import { createHash, randomUUID } from "node:crypto";
import { access, chmod, copyFile, mkdir, readFile, realpath, rename, rm, stat, writeFile } from "node:fs/promises";
import path from "node:path";

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

async function atomicJson(file, value) {
  await mkdir(path.dirname(file), { recursive: true, mode: 0o700 });
  const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
  await writeFile(temporary, `${JSON.stringify(value, null, 2)}\n`, { mode: 0o600 });
  await rename(temporary, file);
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

async function packageDigest(sourceRoot, files) {
  const hash = createHash("sha256");
  for (const file of [...files].sort()) {
    hash.update(file);
    hash.update(await readFile(path.join(sourceRoot, file)));
  }
  return hash.digest("hex");
}

async function copyPackage(sourceRoot, target, files) {
  const parent = path.dirname(target);
  const staging = path.join(parent, `.agent-team-install-${randomUUID()}`);
  await mkdir(staging, { recursive: true, mode: 0o700 });
  for (const file of files) {
    const source = path.join(sourceRoot, file);
    const destination = path.join(staging, file);
    await mkdir(path.dirname(destination), { recursive: true });
    await copyFile(source, destination);
    const mode = (await stat(source)).mode & 0o777;
    if (mode & 0o111) await chmod(destination, mode);
  }
  await rename(staging, target);
}

function stamp(now) {
  return now.toISOString().replace(/[-:.]/g, "");
}

/** Install both runtime copies and merge only hook groups owned by Agent-Team. */
export async function installPackage({ sourceRoot, home, now = new Date() }) {
  const manifest = await readJson(path.join(sourceRoot, "hooks", "manifest.json"));
  const digest = await packageDigest(sourceRoot, manifest.files);
  const stateRoot = path.join(home, ".agent-team-hooks");
  const receiptPath = path.join(stateRoot, "install.json");
  const previousReceipt = await readJson(receiptPath, null);
  const backupRoot = path.join(stateRoot, "backups", stamp(now));
  const backups = [];
  let changed = false;

  const targets = [
    path.join(home, ".agents", "skills", "agent-team"),
    path.join(home, ".claude", "skills", "agent-team"),
  ];
  const sourceIdentity = await realpath(sourceRoot);
  for (const [index, target] of targets.entries()) {
    const targetPresent = await present(target);
    const sourceIsTarget = targetPresent && await realpath(target) === sourceIdentity;
    const current = previousReceipt?.digest === digest && await present(path.join(target, "SKILL.md"));
    if (sourceIsTarget || current) continue;
    await mkdir(path.dirname(target), { recursive: true });
    if (targetPresent) {
      const backup = path.join(backupRoot, "skills", index === 0 ? "agents-agent-team" : "claude-agent-team");
      await mkdir(path.dirname(backup), { recursive: true });
      await rename(target, backup);
      backups.push({ kind: "skill", target, backup });
    }
    await copyPackage(sourceRoot, target, manifest.files);
    changed = true;
  }

  const legacy = path.join(home, ".codex", "skills", "agent-team");
  if (await present(legacy)) {
    const backup = path.join(backupRoot, "skills", "codex-legacy-agent-team");
    await mkdir(path.dirname(backup), { recursive: true });
    await rename(legacy, backup);
    backups.push({ kind: "legacy", target: legacy, backup });
    changed = true;
  }

  for (const runtime of ["codex", "claude"]) {
    const configPath = runtime === "codex" ? path.join(home, ".codex", "hooks.json") : path.join(home, ".claude", "settings.json");
    const declaration = await readJson(path.join(sourceRoot, "hooks", `${runtime}-hooks.json`));
    const config = await readJson(configPath);
    const merged = mergeHooks(config, declaration);
    if (JSON.stringify(config) !== JSON.stringify(merged)) {
      if (await present(configPath)) {
        const backup = path.join(backupRoot, "config", runtime === "codex" ? "hooks.json" : "settings.json");
        await mkdir(path.dirname(backup), { recursive: true });
        await copyFile(configPath, backup);
        backups.push({ kind: "config", target: configPath, backup });
      }
      await atomicJson(configPath, merged);
      changed = true;
    }
  }

  if (!changed) return { status: "installed", changed: false, receipt: receiptPath };
  const receipt = {
    schemaVersion: 1,
    installedAt: now.toISOString(),
    sourceRoot,
    digest,
    targets,
    backups: [...(previousReceipt?.backups ?? []), ...backups],
  };
  await atomicJson(receiptPath, receipt);
  return { status: "installed", changed: true, receipt: receiptPath, backups };
}

/** Remove only owned hook groups and restore skill copies saved by the installer. */
export async function uninstallPackage({ home }) {
  const receiptPath = path.join(home, ".agent-team-hooks", "install.json");
  const receipt = await readJson(receiptPath, null);
  if (!receipt) return { status: "not_installed", changed: false };
  for (const runtime of ["codex", "claude"]) {
    const configPath = runtime === "codex" ? path.join(home, ".codex", "hooks.json") : path.join(home, ".claude", "settings.json");
    await atomicJson(configPath, removeOwnedHooks(await readJson(configPath)));
  }
  for (const target of receipt.targets ?? []) await rm(target, { force: true, recursive: true });
  for (const backup of [...(receipt.backups ?? [])].reverse().filter(({ kind }) => kind === "skill" || kind === "legacy")) {
    if (!(await present(backup.target)) && await present(backup.backup)) {
      await mkdir(path.dirname(backup.target), { recursive: true });
      await rename(backup.backup, backup.target);
    }
  }
  await rm(receiptPath, { force: true });
  return { status: "uninstalled", changed: true };
}

export { mergeHooks, removeOwnedHooks };
