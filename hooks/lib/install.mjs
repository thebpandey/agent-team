import { createHash, randomUUID } from "node:crypto";
import { access, chmod, copyFile, mkdir, readFile, readdir, realpath, rename, rm, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import { withDirectoryLock } from "./lock.mjs";

async function present(file) {
  try {
    await access(file);
    return true;
  } catch {
    return false;
  }
}

async function readJson(file, fallback = {}) {
  try {
    return JSON.parse(await readFile(file, "utf8"));
  } catch (error) {
    if (error.code === "ENOENT") return fallback;
    throw error;
  }
}

async function atomicText(file, value, mode = 0o600) {
  await mkdir(path.dirname(file), { recursive: true, mode: 0o700 });
  const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
  await writeFile(temporary, value, { mode });
  await rename(temporary, file);
}

async function atomicJson(file, value) {
  await atomicText(file, `${JSON.stringify(value, null, 2)}\n`);
}

function owned(group) {
  return group.hooks?.some(({ command = "" }) => command.includes("agent-team-hook.mjs"));
}

function mergeHooks(config, declaration) {
  const output = structuredClone(config);
  output.hooks ??= {};
  for (const [event, groups] of Object.entries(declaration.hooks)) {
    output.hooks[event] = [...(output.hooks[event] ?? []).filter((group) => !owned(group)), ...groups];
  }
  for (const [event, groups] of Object.entries(output.hooks)) {
    if (!declaration.hooks[event]) output.hooks[event] = groups.filter((group) => !owned(group));
    if (!output.hooks[event].length) delete output.hooks[event];
  }
  return output;
}

function removeOwnedHooks(config) {
  const output = structuredClone(config);
  for (const [event, groups] of Object.entries(output.hooks ?? {})) {
    output.hooks[event] = groups.filter((group) => !owned(group));
    if (!output.hooks[event].length) delete output.hooks[event];
  }
  return output;
}

function hashBytes(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

async function fileDigest(file) {
  try {
    return hashBytes(await readFile(file));
  } catch {
    return undefined;
  }
}

async function packageDigest(sourceRoot, files) {
  const hash = createHash("sha256");
  for (const file of [...files].sort()) {
    hash.update(file);
    hash.update(await readFile(path.join(sourceRoot, file)));
  }
  return hash.digest("hex");
}

async function listFiles(root, relative = "", output = []) {
  for (const entry of await readdir(path.join(root, relative), { withFileTypes: true })) {
    const name = path.posix.join(relative.replaceAll("\\", "/"), entry.name);
    if (entry.isDirectory()) await listFiles(root, name, output);
    else if (entry.isFile()) output.push(name);
  }
  return output;
}

async function managedPackageDigest(root, files) {
  try {
    const actual = (await listFiles(root)).sort();
    const expected = [...files].sort();
    if (actual.length !== expected.length || actual.some((file, index) => file !== expected[index])) return undefined;
    return packageDigest(root, files);
  } catch {
    return undefined;
  }
}

async function copyPackage(sourceRoot, target, files) {
  const parent = path.dirname(target);
  const staging = path.join(parent, `.agent-team-install-${randomUUID()}`);
  await mkdir(staging, { recursive: true, mode: 0o700 });
  try {
    for (const file of files) {
      const source = path.join(sourceRoot, file);
      const destination = path.join(staging, file);
      await mkdir(path.dirname(destination), { recursive: true });
      await copyFile(source, destination);
      const mode = (await stat(source)).mode & 0o777;
      if (mode & 0o111) await chmod(destination, mode);
    }
    await rename(staging, target);
  } finally {
    await rm(staging, { force: true, recursive: true });
  }
}

function stamp(now) {
  return now.toISOString().replace(/[-:.]/g, "");
}

async function transaction(callback) {
  const undo = [];
  try {
    return await callback((action) => undo.push(action));
  } catch (error) {
    const rollbackErrors = [];
    for (const action of undo.reverse()) {
      try {
        await action();
      } catch (rollbackError) {
        rollbackErrors.push(rollbackError.message);
      }
    }
    if (rollbackErrors.length) error.message = `${error.message}; rollback errors: ${rollbackErrors.join("; ")}`;
    throw error;
  }
}

function targetRecord(receipt, runtime, target) {
  return (receipt?.targets ?? []).map((entry, index) => typeof entry === "string"
    ? { runtime: index === 0 ? "codex" : "claude", path: entry, mode: "legacy", digest: receipt.digest }
    : entry).find((entry) => entry.runtime === runtime || entry.path === target);
}

async function installLocked({ sourceRoot, home, now }, stateRoot) {
  const manifest = await readJson(path.join(sourceRoot, "hooks", "manifest.json"));
  const digest = await packageDigest(sourceRoot, manifest.files);
  const receiptPath = path.join(stateRoot, "install.json");
  const previousReceipt = await readJson(receiptPath, null);
  const backupRoot = path.join(stateRoot, "backups", `${stamp(now)}-${randomUUID().slice(0, 8)}`);
  const backups = [];
  const conflicts = [];
  let changed = false;

  return transaction(async (addUndo) => {
    addUndo(() => rm(backupRoot, { force: true, recursive: true }));
    const targets = [];
    const sourceIdentity = await realpath(sourceRoot);
    for (const [index, [runtime, target]] of [
      ["codex", path.join(home, ".agents", "skills", "agent-team")],
      ["claude", path.join(home, ".claude", "skills", "agent-team")],
    ].entries()) {
      const targetPresent = await present(target);
      const sourceIsTarget = targetPresent && await realpath(target) === sourceIdentity;
      const previous = targetRecord(previousReceipt, runtime, target);
      if (sourceIsTarget) {
        targets.push({ runtime, path: target, mode: "source", digest, files: manifest.files });
        continue;
      }
      const currentDigest = targetPresent ? await managedPackageDigest(target, manifest.files) : undefined;
      if (targetPresent && previous?.mode === "copied" && currentDigest !== previous.digest) {
        conflicts.push({ kind: "skill", target, reason: "managed_target_changed" });
        targets.push(previous);
        continue;
      }
      if (targetPresent && currentDigest === digest) {
        targets.push({ runtime, path: target, mode: "copied", digest, files: manifest.files });
        continue;
      }
      await mkdir(path.dirname(target), { recursive: true });
      if (targetPresent) {
        const backup = path.join(backupRoot, "skills", index === 0 ? "agents-agent-team" : "claude-agent-team");
        await mkdir(path.dirname(backup), { recursive: true });
        await rename(target, backup);
        addUndo(async () => { if (await present(backup)) await rename(backup, target); });
        backups.push({ kind: "skill", target, backup });
      }
      await copyPackage(sourceRoot, target, manifest.files);
      addUndo(() => rm(target, { force: true, recursive: true }));
      targets.push({ runtime, path: target, mode: "copied", digest, files: manifest.files });
      changed = true;
    }

    const legacy = path.join(home, ".codex", "skills", "agent-team");
    if (await present(legacy)) {
      const backup = path.join(backupRoot, "skills", "codex-legacy-agent-team");
      await mkdir(path.dirname(backup), { recursive: true });
      await rename(legacy, backup);
      addUndo(async () => { if (await present(backup)) await rename(backup, legacy); });
      backups.push({ kind: "legacy", target: legacy, backup });
      changed = true;
    }

    const claudeAgents = [];
    for (const sourceFile of manifest.files.filter((file) => file.startsWith("assets/claude-agents/") && file.endsWith(".md"))) {
      const target = path.join(home, ".claude", "agents", path.basename(sourceFile));
      const source = path.join(sourceRoot, sourceFile);
      const desiredDigest = await fileDigest(source);
      const currentDigest = await fileDigest(target);
      const previous = previousReceipt?.claudeAgents?.find((entry) => entry.path === target);
      if (currentDigest === desiredDigest) {
        claudeAgents.push({ path: target, source: sourceFile, digest: desiredDigest });
        continue;
      }
      if (currentDigest && (!previous || previous.digest !== currentDigest)) {
        conflicts.push({ kind: "claude_agent", target, reason: "custom_definition" });
        if (previous) claudeAgents.push(previous);
        continue;
      }
      let prior;
      if (currentDigest) {
        prior = await readFile(target);
        const backup = path.join(backupRoot, "claude-agents", path.basename(target));
        await mkdir(path.dirname(backup), { recursive: true });
        await copyFile(target, backup);
        backups.push({ kind: "claude_agent", target, backup });
      }
      await atomicText(target, await readFile(source, "utf8"));
      addUndo(async () => {
        if (prior) await atomicText(target, prior);
        else await rm(target, { force: true });
      });
      claudeAgents.push({ path: target, source: sourceFile, digest: desiredDigest });
      changed = true;
    }

    for (const runtime of ["codex", "claude"]) {
      const configPath = runtime === "codex" ? path.join(home, ".codex", "hooks.json") : path.join(home, ".claude", "settings.json");
      const declaration = await readJson(path.join(sourceRoot, "hooks", `${runtime}-hooks.json`));
      const configPresent = await present(configPath);
      const original = configPresent ? await readFile(configPath, "utf8") : undefined;
      const config = configPresent ? JSON.parse(original) : {};
      const merged = mergeHooks(config, declaration);
      if (JSON.stringify(config) !== JSON.stringify(merged)) {
        if (configPresent) {
          const backup = path.join(backupRoot, "config", runtime === "codex" ? "hooks.json" : "settings.json");
          await mkdir(path.dirname(backup), { recursive: true });
          await copyFile(configPath, backup);
          backups.push({ kind: "config", target: configPath, backup });
        }
        await atomicJson(configPath, merged);
        addUndo(async () => {
          if (original === undefined) await rm(configPath, { force: true });
          else await atomicText(configPath, original);
        });
        changed = true;
      }
    }

    const receipt = {
      schemaVersion: 2,
      installedAt: now.toISOString(),
      sourceRoot,
      digest,
      targets,
      claudeAgents,
      backups: [...(previousReceipt?.backups ?? []), ...backups],
    };
    if (changed || !previousReceipt) {
      const original = previousReceipt ? `${JSON.stringify(previousReceipt, null, 2)}\n` : undefined;
      await atomicJson(receiptPath, receipt);
      addUndo(async () => {
        if (original === undefined) await rm(receiptPath, { force: true });
        else await atomicText(receiptPath, original);
      });
    }
    return { status: "installed", changed, receipt: receiptPath, backups, conflicts };
  });
}

/** Install both runtime copies, native Claude roles, and owned hook groups under one user lock. */
export async function installPackage({ sourceRoot, home, now = new Date() }) {
  const stateRoot = path.join(home, ".agent-team-hooks");
  return withDirectoryLock(path.join(stateRoot, "install.lock"), {
    pid: process.pid,
    operation: "install",
    acquiredAt: now.toISOString(),
  }, () => installLocked({ sourceRoot, home, now }, stateRoot), { timeoutMs: 5000 });
}

async function uninstallLocked({ home }, stateRoot) {
  const receiptPath = path.join(stateRoot, "install.json");
  const receipt = await readJson(receiptPath, null);
  if (!receipt) return { status: "not_installed", changed: false, conflicts: [] };
  const conflicts = [];
  const removalRoot = path.join(stateRoot, "removals", randomUUID());

  const result = await transaction(async (addUndo) => {
    for (const runtime of ["codex", "claude"]) {
      const configPath = runtime === "codex" ? path.join(home, ".codex", "hooks.json") : path.join(home, ".claude", "settings.json");
      const configPresent = await present(configPath);
      const original = configPresent ? await readFile(configPath, "utf8") : undefined;
      const config = configPresent ? JSON.parse(original) : {};
      const cleaned = removeOwnedHooks(config);
      if (JSON.stringify(config) !== JSON.stringify(cleaned)) {
        await atomicJson(configPath, cleaned);
        addUndo(async () => {
          if (original === undefined) await rm(configPath, { force: true });
          else await atomicText(configPath, original);
        });
      }
    }

    const normalizedTargets = (receipt.targets ?? []).map((entry, index) => typeof entry === "string"
      ? { runtime: index === 0 ? "codex" : "claude", path: entry, mode: "legacy", digest: receipt.digest }
      : entry);
    let sourceIdentity;
    try {
      sourceIdentity = await realpath(receipt.sourceRoot);
    } catch {
      sourceIdentity = undefined;
    }
    for (const target of normalizedTargets) {
      if (!(await present(target.path))) continue;
      let targetIdentity;
      try {
        targetIdentity = await realpath(target.path);
      } catch {
        targetIdentity = undefined;
      }
      if (target.mode === "source" || (sourceIdentity && targetIdentity === sourceIdentity)) continue;
      let files = target.files;
      if (!files) files = (await readJson(path.join(target.path, "hooks", "manifest.json"), { files: [] })).files;
      const currentDigest = await managedPackageDigest(target.path, files ?? []);
      if (!currentDigest || currentDigest !== target.digest) {
        conflicts.push({ kind: "skill", target: target.path, reason: "managed_target_changed" });
        continue;
      }
      const removed = path.join(removalRoot, "skills", target.runtime);
      await mkdir(path.dirname(removed), { recursive: true });
      await rename(target.path, removed);
      addUndo(async () => { if (await present(removed)) await rename(removed, target.path); });
      const backup = [...(receipt.backups ?? [])].reverse().find((entry) => entry.kind === "skill" && entry.target === target.path && entry.backup);
      if (backup && await present(backup.backup)) {
        await mkdir(path.dirname(target.path), { recursive: true });
        await rename(backup.backup, target.path);
        addUndo(async () => { if (await present(target.path)) await rename(target.path, backup.backup); });
      }
    }

    for (const agent of receipt.claudeAgents ?? []) {
      if (!(await present(agent.path))) continue;
      if (await fileDigest(agent.path) !== agent.digest) {
        conflicts.push({ kind: "claude_agent", target: agent.path, reason: "custom_definition" });
        continue;
      }
      const removed = path.join(removalRoot, "claude-agents", path.basename(agent.path));
      await mkdir(path.dirname(removed), { recursive: true });
      await rename(agent.path, removed);
      addUndo(async () => { if (await present(removed)) await rename(removed, agent.path); });
    }

    for (const backup of [...(receipt.backups ?? [])].reverse().filter(({ kind }) => kind === "legacy")) {
      if (!(await present(backup.target)) && await present(backup.backup)) {
        await mkdir(path.dirname(backup.target), { recursive: true });
        await rename(backup.backup, backup.target);
        addUndo(async () => { if (await present(backup.target)) await rename(backup.target, backup.backup); });
      }
    }

    if (conflicts.length) {
      await atomicJson(receiptPath, { ...receipt, status: "uninstall_conflicts", conflicts });
      addUndo(() => atomicJson(receiptPath, receipt));
    } else {
      await rm(receiptPath, { force: true });
      addUndo(() => atomicJson(receiptPath, receipt));
    }
    return { status: conflicts.length ? "uninstalled_with_conflicts" : "uninstalled", changed: true, conflicts };
  });
  await rm(removalRoot, { force: true, recursive: true });
  return result;
}

/** Unregister hooks and remove only unchanged files owned by the install receipt. */
export async function uninstallPackage({ home }) {
  const stateRoot = path.join(home, ".agent-team-hooks");
  return withDirectoryLock(path.join(stateRoot, "install.lock"), {
    pid: process.pid,
    operation: "uninstall",
    acquiredAt: new Date().toISOString(),
  }, () => uninstallLocked({ home }, stateRoot), { timeoutMs: 5000 });
}

export { mergeHooks, removeOwnedHooks };
