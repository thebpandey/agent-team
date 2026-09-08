import { createHash, randomUUID } from "node:crypto";
import { access, chmod, copyFile, mkdir, readFile, readdir, realpath, rename, rm, stat, writeFile } from "node:fs/promises";
import path from "node:path";
import { withDirectoryLock } from "./lock.mjs";
import { checkInstalledPackage } from "./package-validator.mjs";

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

function same(left, right) {
  return JSON.stringify(left) === JSON.stringify(right);
}

function handlerDigest(handler) {
  return hashBytes(Buffer.from(JSON.stringify(handler)));
}

function handlerId(runtime, event, groupIndex, handlerIndex) {
  return `${runtime}:${event}:${groupIndex}:${handlerIndex}`;
}

function exactLocations(config, event, handler) {
  const matches = [];
  for (const [groupIndex, group] of (config.hooks?.[event] ?? []).entries()) {
    for (const [handlerIndex, candidate] of (group.hooks ?? []).entries()) {
      if (same(candidate, handler)) matches.push({ groupIndex, handlerIndex });
    }
  }
  return matches;
}

function managedCommandIdentity(handler) {
  if (handler?.type !== "command" || typeof handler.command !== "string") return undefined;
  if (!/(?:^|[\\/])agent-team-hook\.mjs(?=["'\s]|$)/.test(handler.command)) return undefined;
  const runtime = handler.command.match(/(?:^|\s)--runtime\s+(codex|claude)(?=\s|$)/)?.[1];
  const event = handler.command.match(/(?:^|\s)--event\s+([^\s"']+)(?=\s|$)/)?.[1];
  return runtime && event ? `${runtime}:${event}` : undefined;
}

function identityLocations(config, event, handler) {
  const identity = managedCommandIdentity(handler);
  if (!identity) return [];
  const matches = [];
  for (const [groupIndex, group] of (config.hooks?.[event] ?? []).entries()) {
    for (const [handlerIndex, candidate] of (group.hooks ?? []).entries()) {
      if (managedCommandIdentity(candidate) === identity) matches.push({ groupIndex, handlerIndex });
    }
  }
  return matches;
}

function recordAt(runtime, event, handlerIdValue, location, handler, preexisting = false) {
  return {
    runtime,
    event,
    handlerId: handlerIdValue,
    groupIndex: location.groupIndex,
    handlerIndex: location.handlerIndex,
    digest: handlerDigest(handler),
    handler: structuredClone(handler),
    preexisting,
  };
}

function mergeHooks(config, declaration, { runtime, previousHandlers = [], adoptExisting = false }) {
  const output = structuredClone(config);
  output.hooks ??= {};
  const handlers = [];
  const conflicts = [];
  let changed = false;

  for (const [event, declaredGroups] of Object.entries(declaration.hooks ?? {})) {
    output.hooks[event] ??= [];
    for (const [declaredGroupIndex, declaredGroup] of declaredGroups.entries()) {
      let appendedGroup;
      for (const [declaredHandlerIndex, desired] of (declaredGroup.hooks ?? []).entries()) {
        const id = handlerId(runtime, event, declaredGroupIndex, declaredHandlerIndex);
        const previous = previousHandlers.find((entry) => entry.handlerId === id);
        if (previous) {
          const exact = exactLocations(output, event, previous.handler);
          if (exact.length === 1) {
            const location = exact[0];
            if (!same(previous.handler, desired)) {
              output.hooks[event][location.groupIndex].hooks[location.handlerIndex] = structuredClone(desired);
              changed = true;
            }
            handlers.push(recordAt(runtime, event, id, location, desired, previous.preexisting));
            continue;
          }
          if (exact.length > 1) {
            conflicts.push({ kind: "handler", runtime, event, handlerId: id, reason: "ambiguous_handler" });
            handlers.push(previous);
            continue;
          }
          const candidate = output.hooks[event]?.[previous.groupIndex]?.hooks?.[previous.handlerIndex];
          if (candidate) conflicts.push({ kind: "handler", runtime, event, handlerId: id, reason: "customized_handler" });
          else conflicts.push({ kind: "handler", runtime, event, handlerId: id, reason: "managed_handler_missing" });
          handlers.push(previous);
          continue;
        }

        const existing = exactLocations(output, event, desired);
        if (existing.length === 1) {
          handlers.push(recordAt(runtime, event, id, existing[0], desired, !adoptExisting));
          continue;
        }
        if (existing.length > 1) {
          conflicts.push({ kind: "handler", runtime, event, handlerId: id, reason: "ambiguous_handler" });
          continue;
        }
        if (identityLocations(output, event, desired).length) {
          conflicts.push({ kind: "handler", runtime, event, handlerId: id, reason: "ambiguous_handler" });
          continue;
        }
        if (!appendedGroup) {
          appendedGroup = structuredClone(declaredGroup);
          appendedGroup.hooks = [];
          output.hooks[event].push(appendedGroup);
        }
        appendedGroup.hooks.push(structuredClone(desired));
        const location = {
          groupIndex: output.hooks[event].length - 1,
          handlerIndex: appendedGroup.hooks.length - 1,
        };
        handlers.push(recordAt(runtime, event, id, location, desired));
        changed = true;
      }
    }
  }
  return { config: output, handlers, conflicts, changed };
}

function removeOwnedHooks(config, handlerRecords) {
  const output = structuredClone(config);
  const conflicts = [];
  let changed = false;
  for (const record of [...handlerRecords].reverse()) {
    if (record.preexisting) {
      conflicts.push({
        kind: "handler",
        runtime: record.runtime,
        event: record.event,
        handlerId: record.handlerId,
        reason: "preexisting_handler",
      });
      continue;
    }
    const exact = exactLocations(output, record.event, record.handler);
    if (exact.length !== 1) {
      const candidate = output.hooks?.[record.event]?.[record.groupIndex]?.hooks?.[record.handlerIndex];
      conflicts.push({
        kind: "handler",
        runtime: record.runtime,
        event: record.event,
        handlerId: record.handlerId,
        reason: exact.length > 1 ? "ambiguous_handler" : candidate ? "customized_handler" : "managed_handler_missing",
      });
      continue;
    }
    const { groupIndex, handlerIndex } = exact[0];
    const groups = output.hooks[record.event];
    groups[groupIndex].hooks.splice(handlerIndex, 1);
    if (!groups[groupIndex].hooks.length) groups.splice(groupIndex, 1);
    if (!groups.length) delete output.hooks[record.event];
    changed = true;
  }
  return { config: output, conflicts, changed };
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

function selection({ host, scope, projectRoot, trustedHost }) {
  const selectedHost = host ?? trustedHost;
  if (!selectedHost) throw new Error("Installation host is required: choose codex, claude-code, or both.");
  if (!host && trustedHost === "both") throw new Error("Trusted host metadata is ambiguous; choose both explicitly to configure both hosts.");
  if (!["codex", "claude-code", "both"].includes(selectedHost)) throw new Error(`Unsupported installation host: ${selectedHost}`);
  if (!["user", "project"].includes(scope)) throw new Error("Installation scope is required: choose user or project.");
  if (scope === "project" && !projectRoot) throw new Error("projectRoot is required for project scope.");
  return {
    host: selectedHost,
    scope,
    projectRoot: scope === "project" ? path.resolve(projectRoot) : undefined,
    runtimes: selectedHost === "both" ? ["codex", "claude"] : [selectedHost === "claude-code" ? "claude" : selectedHost],
  };
}

async function hookDeclaration(sourceRoot, runtime, selected) {
  const declaration = await readJson(path.join(sourceRoot, "hooks", `${runtime}-hooks.json`));
  if (selected.scope !== "project") return declaration;
  const serialized = JSON.stringify(declaration);
  const userSkill = runtime === "codex"
    ? "$HOME/.agents/skills/agent-team"
    : "$HOME/.claude/skills/agent-team";
  const projectSkill = runtime === "codex"
    ? "$(git rev-parse --show-toplevel)/.agents/skills/agent-team"
    : "${CLAUDE_PROJECT_DIR}/.claude/skills/agent-team";
  return JSON.parse(serialized.replaceAll(userSkill, projectSkill));
}

function runtimePaths({ home, scope, projectRoot }, runtime) {
  const root = scope === "project" ? projectRoot : home;
  if (runtime === "codex") return {
    target: path.join(root, ".agents", "skills", "agent-team"),
    configPath: path.join(root, ".codex", "hooks.json"),
  };
  return {
    target: path.join(root, ".claude", "skills", "agent-team"),
    configPath: path.join(root, ".claude", scope === "project" ? "settings.local.json" : "settings.json"),
    agentsRoot: path.join(root, ".claude", "agents"),
  };
}

async function installLocked({ sourceRoot, home, now, selected }, stateRoot) {
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
    const previousTargets = (previousReceipt?.targets ?? []).map((entry, index) => typeof entry === "string"
      ? { runtime: index === 0 ? "codex" : "claude", path: entry, mode: "legacy", digest: previousReceipt.digest }
      : entry);
    const targets = previousTargets.filter((entry) => !selected.runtimes.includes(entry.runtime));
    const sourceIdentity = await realpath(sourceRoot);
    for (const [index, runtime] of selected.runtimes.entries()) {
      const { target } = runtimePaths({ home, ...selected }, runtime);
      const targetPresent = await present(target);
      const sourceIsTarget = targetPresent && await realpath(target) === sourceIdentity;
      const previous = targetRecord(previousReceipt, runtime, target);
      if (sourceIsTarget) {
        targets.push({ runtime, path: target, mode: "source", digest, files: manifest.files });
        continue;
      }
      const previousDigest = targetPresent && previous?.mode === "copied"
        ? await managedPackageDigest(target, previous.files ?? manifest.files)
        : undefined;
      if (targetPresent && previous?.mode === "copied" && previousDigest !== previous.digest) {
        conflicts.push({ kind: "skill", target, reason: "managed_target_changed" });
        targets.push(previous);
        continue;
      }
      const currentDigest = targetPresent ? await managedPackageDigest(target, manifest.files) : undefined;
      if (targetPresent && currentDigest === digest) {
        targets.push({ runtime, path: target, mode: "copied", digest, files: manifest.files });
        continue;
      }
      await mkdir(path.dirname(target), { recursive: true });
      if (targetPresent) {
        const backup = path.join(backupRoot, "skills", runtime === "codex" ? "agents-agent-team" : "claude-agent-team");
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
    if (selected.scope === "user" && selected.runtimes.includes("codex") && await present(legacy)) {
      const backup = path.join(backupRoot, "skills", "codex-legacy-agent-team");
      await mkdir(path.dirname(backup), { recursive: true });
      await rename(legacy, backup);
      addUndo(async () => { if (await present(backup)) await rename(backup, legacy); });
      backups.push({ kind: "legacy", target: legacy, backup });
      changed = true;
    }

    const claudeAgents = selected.runtimes.includes("claude") ? [] : [...(previousReceipt?.claudeAgents ?? [])];
    const handlerReceipts = (previousReceipt?.handlers ?? []).filter((entry) => !selected.runtimes.includes(entry.runtime));
    const handlerConflictReceipts = (previousReceipt?.handlerConflicts ?? []).filter((entry) => !selected.runtimes.includes(entry.runtime));
    for (const sourceFile of selected.runtimes.includes("claude") ? manifest.files.filter((file) => file.startsWith("assets/claude-agents/") && file.endsWith(".md")) : []) {
      const target = path.join(runtimePaths({ home, ...selected }, "claude").agentsRoot, path.basename(sourceFile));
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

    for (const runtime of selected.runtimes) {
      const { configPath } = runtimePaths({ home, ...selected }, runtime);
      const declaration = await hookDeclaration(sourceRoot, runtime, selected);
      const configPresent = await present(configPath);
      const original = configPresent ? await readFile(configPath, "utf8") : undefined;
      const config = configPresent ? JSON.parse(original) : {};
      const merged = mergeHooks(config, declaration, {
        runtime,
        previousHandlers: (previousReceipt?.handlers ?? []).filter((entry) => entry.runtime === runtime),
        adoptExisting: Boolean(previousReceipt && !Array.isArray(previousReceipt.handlers)),
      });
      const mergedConflicts = merged.conflicts.map((entry) => ({ ...entry, target: configPath }));
      conflicts.push(...mergedConflicts);
      handlerConflictReceipts.push(...mergedConflicts.filter((conflict) => !merged.handlers.some((handler) => handler.handlerId === conflict.handlerId)));
      handlerReceipts.push(...merged.handlers.map((entry) => ({ ...entry, configPath })));
      if (merged.changed) {
        if (configPresent) {
          const backup = path.join(backupRoot, "config", runtime === "codex" ? "hooks.json" : "settings.json");
          await mkdir(path.dirname(backup), { recursive: true });
          await copyFile(configPath, backup);
          backups.push({ kind: "config", target: configPath, backup });
        }
        await atomicJson(configPath, merged.config);
        addUndo(async () => {
          if (original === undefined) await rm(configPath, { force: true });
          else await atomicText(configPath, original);
        });
        changed = true;
      }
    }

    const installedRuntimes = [...new Set([...targets.map(({ runtime }) => runtime), ...handlerReceipts.map(({ runtime }) => runtime)])];
    const receipt = {
      schemaVersion: 3,
      installedAt: now.toISOString(),
      sourceRoot,
      digest,
      host: installedRuntimes.length === 2 ? "both" : installedRuntimes[0] === "claude" ? "claude-code" : installedRuntimes[0],
      scope: selected.scope,
      projectRoot: selected.projectRoot,
      targets,
      claudeAgents,
      handlers: handlerReceipts,
      handlerConflicts: handlerConflictReceipts,
      backups: [...(previousReceipt?.backups ?? []), ...backups],
    };
    if (changed || !previousReceipt || previousReceipt.schemaVersion !== 3 || !Array.isArray(previousReceipt.handlers)) {
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

/** Install only the selected host/scope, preserving exact handler ownership under one scope lock. */
export async function installPackage({ sourceRoot, home, host, scope, projectRoot, trustedHost, now = new Date() }) {
  const selected = selection({ host, scope, projectRoot, trustedHost });
  const validation = await checkInstalledPackage(sourceRoot);
  if (validation.status !== "passed") throw new Error(`Invalid installable package: ${validation.errors.join("; ")}`);
  const stateRoot = path.join(selected.scope === "project" ? selected.projectRoot : home, ".agent-team-hooks");
  const result = await withDirectoryLock(path.join(stateRoot, "install.lock"), {
    pid: process.pid,
    operation: "install",
    acquiredAt: now.toISOString(),
  }, () => installLocked({ sourceRoot, home, now, selected }, stateRoot), { timeoutMs: 5000 });
  return {
    ...result,
    validation,
    selection: {
      host: selected.host,
      scope: selected.scope,
      ...(selected.projectRoot ? { projectRoot: selected.projectRoot } : {}),
    },
    configured: selected.runtimes.map((runtime) => ({
      runtime,
      configPath: runtimePaths({ home, ...selected }, runtime).configPath,
      trust: "required",
    })),
  };
}

async function uninstallLocked({ home, selected }, stateRoot) {
  const receiptPath = path.join(stateRoot, "install.json");
  const receipt = await readJson(receiptPath, null);
  if (!receipt) return { status: "not_installed", changed: false, conflicts: [] };
  const conflicts = (receipt.handlerConflicts ?? []).filter((entry) => selected.runtimes.includes(entry.runtime));
  const removalRoot = path.join(stateRoot, "removals", randomUUID());

  const result = await transaction(async (addUndo) => {
    for (const runtime of selected.runtimes) {
      const { configPath } = runtimePaths({ home, ...selected }, runtime);
      const configPresent = await present(configPath);
      const original = configPresent ? await readFile(configPath, "utf8") : undefined;
      const config = configPresent ? JSON.parse(original) : {};
      const cleaned = removeOwnedHooks(config, (receipt.handlers ?? []).filter((entry) => entry.runtime === runtime && (!entry.configPath || entry.configPath === configPath)));
      conflicts.push(...cleaned.conflicts.map((entry) => ({ ...entry, target: configPath })));
      if (cleaned.changed && !cleaned.conflicts.length) {
        await atomicJson(configPath, cleaned.config);
        addUndo(async () => {
          if (original === undefined) await rm(configPath, { force: true });
          else await atomicText(configPath, original);
        });
      }
    }

    const normalizedTargets = (receipt.targets ?? []).map((entry, index) => typeof entry === "string"
      ? { runtime: index === 0 ? "codex" : "claude", path: entry, mode: "legacy", digest: receipt.digest }
      : entry);
    const blockedRuntimes = new Set(conflicts.filter(({ kind }) => kind === "handler").map(({ runtime }) => runtime));
    let sourceIdentity;
    try {
      sourceIdentity = await realpath(receipt.sourceRoot);
    } catch {
      sourceIdentity = undefined;
    }
    for (const target of normalizedTargets.filter((entry) => selected.runtimes.includes(entry.runtime) && !blockedRuntimes.has(entry.runtime))) {
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
        conflicts.push({ kind: "skill", runtime: target.runtime, target: target.path, reason: "managed_target_changed" });
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

    for (const agent of selected.runtimes.includes("claude") && !blockedRuntimes.has("claude") ? receipt.claudeAgents ?? [] : []) {
      if (!(await present(agent.path))) continue;
      if (await fileDigest(agent.path) !== agent.digest) {
        conflicts.push({ kind: "claude_agent", runtime: "claude", target: agent.path, reason: "custom_definition" });
        continue;
      }
      const removed = path.join(removalRoot, "claude-agents", path.basename(agent.path));
      await mkdir(path.dirname(removed), { recursive: true });
      await rename(agent.path, removed);
      addUndo(async () => { if (await present(removed)) await rename(removed, agent.path); });
    }

    for (const backup of selected.runtimes.includes("codex") ? [...(receipt.backups ?? [])].reverse().filter(({ kind }) => kind === "legacy") : []) {
      if (!(await present(backup.target)) && await present(backup.backup)) {
        await mkdir(path.dirname(backup.target), { recursive: true });
        await rename(backup.backup, backup.target);
        addUndo(async () => { if (await present(backup.target)) await rename(backup.target, backup.backup); });
      }
    }

    const unselectedRuntimes = new Set(["codex", "claude"].filter((runtime) => !selected.runtimes.includes(runtime)));
    const handlerConflictRuntimes = new Set(conflicts.filter(({ kind }) => kind === "handler").map(({ runtime }) => runtime));
    const targetConflictRuntimes = new Set(conflicts.filter(({ kind }) => ["handler", "skill"].includes(kind)).map(({ runtime }) => runtime));
    const claudeConflict = conflicts.some(({ kind, runtime }) => runtime === "claude" && ["handler", "claude_agent"].includes(kind));
    const remainingTargets = normalizedTargets.filter((entry) => unselectedRuntimes.has(entry.runtime) || targetConflictRuntimes.has(entry.runtime));
    const remainingHandlers = (receipt.handlers ?? []).filter((entry) => unselectedRuntimes.has(entry.runtime) || handlerConflictRuntimes.has(entry.runtime));
    const remainingHandlerConflicts = (receipt.handlerConflicts ?? []).filter((entry) => unselectedRuntimes.has(entry.runtime) || handlerConflictRuntimes.has(entry.runtime));
    const remainingClaudeAgents = unselectedRuntimes.has("claude") || claudeConflict ? receipt.claudeAgents ?? [] : [];
    const remainingRuntimes = [...new Set([
      ...remainingTargets.map(({ runtime }) => runtime),
      ...remainingHandlers.map(({ runtime }) => runtime),
      ...(remainingClaudeAgents.length ? ["claude"] : []),
    ])];
    const remaining = {
      ...receipt,
      host: remainingRuntimes.length === 2 ? "both" : remainingRuntimes[0] === "claude" ? "claude-code" : remainingRuntimes[0],
      targets: remainingTargets,
      handlers: remainingHandlers,
      handlerConflicts: remainingHandlerConflicts,
      claudeAgents: remainingClaudeAgents,
      backups: (receipt.backups ?? []).filter((entry) => {
        if (entry.kind === "legacy") return remainingRuntimes.includes("codex");
        if (entry.kind === "claude_agent") return remainingClaudeAgents.some((agent) => agent.path === entry.target);
        if (entry.kind === "skill") return remainingTargets.some((target) => target.path === entry.target);
        if (entry.kind === "config") return remainingHandlers.some((handler) => handler.configPath === entry.target);
        return true;
      }),
    };
    const hasRemaining = (remaining.targets?.length ?? 0) || (remaining.handlers?.length ?? 0) || (remaining.handlerConflicts?.length ?? 0) || (remaining.claudeAgents?.length ?? 0);
    if (conflicts.length || hasRemaining) {
      await atomicJson(receiptPath, conflicts.length ? { ...remaining, status: "uninstall_conflicts", conflicts } : remaining);
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
export async function uninstallPackage({ home, host, scope, projectRoot, trustedHost }) {
  const selected = selection({ host, scope, projectRoot, trustedHost });
  const stateRoot = path.join(selected.scope === "project" ? selected.projectRoot : home, ".agent-team-hooks");
  return withDirectoryLock(path.join(stateRoot, "install.lock"), {
    pid: process.pid,
    operation: "uninstall",
    acquiredAt: new Date().toISOString(),
  }, () => uninstallLocked({ home, selected }, stateRoot), { timeoutMs: 5000 });
}

/** Roll back the selected installed host/scope using the same ownership receipt. */
export async function rollbackPackage(options) {
  return uninstallPackage(options);
}

export { mergeHooks, removeOwnedHooks };
