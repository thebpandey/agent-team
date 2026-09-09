import { createHash, randomUUID } from "node:crypto";
import { access, chmod, copyFile, mkdir, open, readFile, readdir, realpath, rename, rm, stat, writeFile } from "node:fs/promises";
import path from "node:path";
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

function textGuard(value) {
  return { kind: "file", digest: hashBytes(Buffer.from(value)) };
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
  let output = structuredClone(config);
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
              if (previous.preexisting) {
                conflicts.push({ kind: "handler", runtime, event, handlerId: id, reason: "preexisting_handler" });
                handlers.push(previous);
                continue;
              }
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
          conflicts.push({ kind: "handler", runtime, event, handlerId: id, reason: "ambiguous_handler", handler: structuredClone(desired) });
          continue;
        }
        if (identityLocations(output, event, desired).length) {
          conflicts.push({ kind: "handler", runtime, event, handlerId: id, reason: "ambiguous_handler", handler: structuredClone(desired) });
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
  const desiredIds = new Set(Object.entries(declaration.hooks ?? {}).flatMap(([event, groups]) => groups.flatMap((group, groupIndex) =>
    (group.hooks ?? []).map((handler, handlerIndex) => handlerId(runtime, event, groupIndex, handlerIndex)))));
  const retired = previousHandlers.filter((entry) => !desiredIds.has(entry.handlerId));
  if (retired.length) {
    const removed = removeOwnedHooks(output, retired);
    output = removed.config;
    changed ||= removed.changed;
    conflicts.push(...removed.conflicts);
    const conflictedIds = new Set(removed.conflicts.map(({ handlerId: id }) => id));
    handlers.push(...retired.filter(({ handlerId: id }) => conflictedIds.has(id)));
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

async function resourceGuard(file, files) {
  if (files) return { kind: "package", files, digest: await managedPackageDigest(file, files) };
  try {
    const metadata = await stat(file);
    if (metadata.isDirectory()) {
      const entries = (await listFiles(file)).sort();
      return { kind: "directory", files: entries, digest: await packageDigest(file, entries) };
    }
    return { kind: "file", digest: await fileDigest(file) };
  } catch (error) {
    if (error.code === "ENOENT") return { kind: "absent" };
    throw error;
  }
}

async function guardMatches(file, guard) {
  if (!guard) return false;
  if (guard.kind === "absent") return !(await present(file));
  const actual = await resourceGuard(file, guard.kind === "package" ? guard.files : undefined);
  return actual.kind === guard.kind && actual.digest === guard.digest
    && (guard.kind !== "directory" || same(actual.files, guard.files));
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

async function applyUndo(action) {
  if (action.kind === "remove_path") {
    if (!(await present(action.path))) return null;
    if (!action.internal && !(await guardMatches(action.path, action.guard))) {
      return { kind: "recovery", target: action.path, reason: "post_crash_resource_changed" };
    }
    await rm(action.path, { force: true, recursive: Boolean(action.recursive) });
    return null;
  }
  if (action.kind === "move") {
    if (!(await present(action.from))) return null;
    if (!(await guardMatches(action.from, action.fromGuard))) {
      return { kind: "recovery", target: action.from, reason: "recovery_backup_changed" };
    }
    if (await present(action.to)) {
      return { kind: "recovery", target: action.to, reason: "post_crash_resource_created" };
    }
    await mkdir(path.dirname(action.to), { recursive: true });
    await rename(action.from, action.to);
    return null;
  }
  if (action.kind === "restore_file") {
    if (!(await present(action.backup))) return null;
    if (action.preimageGuard && await guardMatches(action.target, action.preimageGuard)) return null;
    if (!(await guardMatches(action.target, action.targetGuard))) {
      return { kind: "recovery", target: action.target, reason: "post_crash_resource_changed" };
    }
    await atomicText(action.target, await readFile(action.backup));
    return null;
  }
  throw new Error(`Unsupported transaction undo action: ${action.kind}`);
}

function permittedRecoveryPath(stateRoot, candidate) {
  if (typeof candidate !== "string" || !path.isAbsolute(candidate)) return false;
  const scopeRoot = path.dirname(stateRoot);
  const relative = path.relative(scopeRoot, candidate);
  if (!relative || relative.startsWith("..") || path.isAbsolute(relative)) return false;
  const internal = path.relative(stateRoot, candidate);
  if (internal && !internal.startsWith("..") && !path.isAbsolute(internal)) return true;
  const normalized = relative.replaceAll("\\", "/");
  return [
    ".agents/skills/agent-team",
    ".claude/skills/agent-team",
    ".codex/skills/agent-team",
    ".codex/hooks.json",
    ".claude/settings.json",
    ".claude/settings.local.json",
  ].includes(normalized) || /^\.claude\/agents\/agent-team-[^/]+\.md$/.test(normalized);
}

function internalRecoveryPath(stateRoot, candidate) {
  const relative = path.relative(stateRoot, candidate);
  return Boolean(relative) && !relative.startsWith("..") && !path.isAbsolute(relative);
}

function validRecoveryJournal(stateRoot, journal) {
  if (journal?.schemaVersion !== 1 || !["install", "uninstall"].includes(journal.operation)
    || typeof journal.transactionId !== "string" || !Array.isArray(journal.undo)) return false;
  return journal.undo.every((action) => {
    if (action?.kind === "remove_path") return permittedRecoveryPath(stateRoot, action.path)
      && (!action.internal || internalRecoveryPath(stateRoot, action.path));
    if (action?.kind === "move") return permittedRecoveryPath(stateRoot, action.from) && permittedRecoveryPath(stateRoot, action.to);
    if (action?.kind === "restore_file") return permittedRecoveryPath(stateRoot, action.target) && permittedRecoveryPath(stateRoot, action.backup);
    return false;
  });
}

async function recoverTransaction(stateRoot) {
  const journalPath = path.join(stateRoot, "transaction.json");
  const journal = await readJson(journalPath, null);
  if (!journal) return null;
  if (!validRecoveryJournal(stateRoot, journal)) throw new Error("Recovery journal is invalid or contains an out-of-scope action.");
  if (journal.status === "committed") {
    await rm(journalPath, { force: true });
    return { transactionId: journal.transactionId, operation: journal.operation, action: "finalized" };
  }
  const rollbackErrors = [];
  const conflicts = [];
  for (const action of [...(journal.undo ?? [])].reverse()) {
    try {
      if (conflicts.length && action.kind === "remove_path" && action.internal) continue;
      const conflict = await applyUndo(action);
      if (conflict) conflicts.push(conflict);
    } catch (error) {
      rollbackErrors.push(error.message);
    }
  }
  if (rollbackErrors.length) throw new Error(`Cannot recover transaction ${journal.transactionId}: ${rollbackErrors.join("; ")}`);
  if (conflicts.length) {
    journal.status = "recovery_conflicts";
    journal.recoveryConflicts = conflicts;
    await atomicJson(journalPath, journal);
    return { transactionId: journal.transactionId, operation: journal.operation, action: "conflicts", conflicts };
  }
  await rm(journalPath, { force: true });
  return { transactionId: journal.transactionId, operation: journal.operation, action: "rolled_back" };
}

function processIsAlive(pid) {
  try {
    process.kill(pid, 0);
    return true;
  } catch (error) {
    if (error.code === "ESRCH") return false;
    throw error;
  }
}

function sameLockOwner(left, right) {
  if (left?.lockToken || right?.lockToken) return left?.lockToken === right?.lockToken;
  return left?.pid === right?.pid && left?.operation === right?.operation && left?.acquiredAt === right?.acquiredAt;
}

function staleLockFence(stateRoot, owner) {
  const identity = typeof owner.lockToken === "string" && /^[a-zA-Z0-9-]+$/.test(owner.lockToken)
    ? owner.lockToken
    : hashBytes(Buffer.from(JSON.stringify(owner))).slice(0, 32);
  return path.join(stateRoot, `install.lock.stale-${identity}`);
}

async function withRecoverableInstallLock(stateRoot, metadata, callback, { timeoutMs = 5000 } = {}) {
  const lockPath = path.join(stateRoot, "install.lock");
  const ownerPath = path.join(lockPath, "owner.json");
  const owner = { ...metadata, lockToken: randomUUID() };
  const recoveryPath = path.join(lockPath, "recovery.json");
  const started = Date.now();
  await mkdir(stateRoot, { recursive: true, mode: 0o700 });
  while (true) {
    try {
      await mkdir(lockPath, { mode: 0o700 });
      const handle = await open(ownerPath, "wx", 0o600);
      await handle.writeFile(`${JSON.stringify(owner)}\n`);
      await handle.close();
      break;
    } catch (error) {
      if (error.code !== "EEXIST") throw error;
      const observed = await readJson(ownerPath, null);
      if (Number.isInteger(observed?.pid) && observed.pid > 0 && !processIsAlive(observed.pid)) {
        const recoveryPresent = await present(recoveryPath);
        const observedRecovery = recoveryPresent ? await readJson(recoveryPath, null) : null;
        if (recoveryPresent && Number.isInteger(observedRecovery?.pid) && observedRecovery.pid > 0) {
          if (!processIsAlive(observedRecovery.pid)) {
            try {
              await rename(lockPath, staleLockFence(stateRoot, observed));
              continue;
            } catch (takeoverError) {
              if (!["EEXIST", "ENOENT", "ENOTEMPTY"].includes(takeoverError.code)) throw takeoverError;
            }
          }
        } else if (!recoveryPresent) {
          let claim;
          try {
            claim = await open(recoveryPath, "wx", 0o600);
            await claim.writeFile(`${JSON.stringify(owner)}\n`);
            await claim.close();
          } catch (claimError) {
            if (claimError.code !== "EEXIST") throw claimError;
          }
          if (claim) {
            const current = await readJson(ownerPath, null);
            if (sameLockOwner(observed, current) && !processIsAlive(current.pid)) {
              await atomicJson(ownerPath, owner);
              await rm(recoveryPath, { force: true });
              break;
            }
            await rm(recoveryPath, { force: true });
          }
        }
      }
      if (Date.now() - started >= timeoutMs) throw error;
      await new Promise((resolve) => setTimeout(resolve, 10));
    }
  }
  try {
    return await callback();
  } finally {
    const current = await readJson(ownerPath, null);
    if (sameLockOwner(owner, current)) await rm(lockPath, { force: true, recursive: true });
  }
}

async function durableTransaction(stateRoot, operation, callback) {
  const journalPath = path.join(stateRoot, "transaction.json");
  const journal = {
    schemaVersion: 1,
    transactionId: randomUUID(),
    operation,
    status: "active",
    undo: [],
  };
  await atomicJson(journalPath, journal);
  const addUndo = async (action) => {
    journal.undo.push(action);
    await atomicJson(journalPath, journal);
  };
  try {
    const result = await callback(addUndo, journal.transactionId);
    journal.status = "committed";
    await atomicJson(journalPath, journal);
    await rm(journalPath, { force: true });
    return result;
  } catch (error) {
    try {
      await recoverTransaction(stateRoot);
    } catch (rollbackError) {
      error.message = `${error.message}; rollback error: ${rollbackError.message}`;
    }
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
  if (scope === "user" && projectRoot) throw new Error("projectRoot is ineffective with user scope.");
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
  const unavailableRuntimes = new Set();
  let changed = false;

  return durableTransaction(stateRoot, "install", async (addUndo, transactionId) => {
    await addUndo({ kind: "remove_path", path: backupRoot, recursive: true, internal: true });
    const previousTargets = (previousReceipt?.targets ?? []).map((entry, index) => typeof entry === "string"
      ? { runtime: index === 0 ? "codex" : "claude", path: entry, mode: "legacy", digest: previousReceipt.digest }
      : entry);
    const targets = previousTargets.filter((entry) => !selected.runtimes.includes(entry.runtime));
    const sourceIdentity = await realpath(sourceRoot);
    for (const runtime of selected.runtimes) {
      const { target } = runtimePaths({ home, ...selected }, runtime);
      const targetPresent = await present(target);
      const sourceIsTarget = targetPresent && await realpath(target) === sourceIdentity;
      const previous = targetRecord(previousReceipt, runtime, target);
      const previousDigest = targetPresent && previous
        ? await managedPackageDigest(target, previous.files ?? manifest.files)
        : undefined;
      if (previous?.preexisting) {
        if (previousDigest !== previous.digest) conflicts.push({ kind: "skill", runtime, target, reason: "preexisting_target_changed" });
        else if (await managedPackageDigest(target, manifest.files) !== digest) conflicts.push({ kind: "skill", runtime, target, reason: "preexisting_target" });
        else {
          targets.push(previous);
          continue;
        }
        targets.push(previous);
        unavailableRuntimes.add(runtime);
        continue;
      }
      if (sourceIsTarget) {
        targets.push(previous?.mode === "copied"
          ? previous
          : { runtime, path: target, mode: "source", digest, files: manifest.files, preexisting: true });
        continue;
      }
      if (targetPresent && previous?.mode === "copied" && previousDigest !== previous.digest) {
        conflicts.push({ kind: "skill", runtime, target, reason: "managed_target_changed" });
        targets.push(previous);
        continue;
      }
      const currentDigest = targetPresent ? await managedPackageDigest(target, manifest.files) : undefined;
      if (targetPresent && currentDigest === digest) {
        targets.push({ runtime, path: target, mode: "copied", digest, files: manifest.files, preexisting: previous?.preexisting ?? !previous });
        continue;
      }
      if (targetPresent && !previous) {
        conflicts.push({ kind: "skill", runtime, target, reason: "unowned_target" });
        unavailableRuntimes.add(runtime);
        continue;
      }
      await mkdir(path.dirname(target), { recursive: true });
      if (targetPresent) {
        const backup = path.join(backupRoot, "skills", runtime === "codex" ? "agents-agent-team" : "claude-agent-team");
        await mkdir(path.dirname(backup), { recursive: true });
        await addUndo({ kind: "move", from: backup, to: target, fromGuard: await resourceGuard(target, previous?.files ?? manifest.files) });
        await rename(target, backup);
        backups.push({ kind: "skill", target, backup, purpose: previous ? "update_snapshot" : "preinstall_restore", transactionId });
      }
      await addUndo({ kind: "remove_path", path: target, recursive: true, guard: { kind: "package", files: manifest.files, digest } });
      await copyPackage(sourceRoot, target, manifest.files);
      targets.push({ runtime, path: target, mode: "copied", digest, files: manifest.files });
      changed = true;
    }

    const legacy = path.join(home, ".codex", "skills", "agent-team");
    if (selected.scope === "user" && selected.runtimes.includes("codex") && await present(legacy)) {
      const backup = path.join(backupRoot, "skills", "codex-legacy-agent-team");
      await mkdir(path.dirname(backup), { recursive: true });
      await addUndo({ kind: "move", from: backup, to: legacy, fromGuard: await resourceGuard(legacy) });
      await rename(legacy, backup);
      backups.push({ kind: "legacy", target: legacy, backup, purpose: "preinstall_restore", transactionId });
      changed = true;
    }

    const claudeAgents = selected.runtimes.includes("claude") && !unavailableRuntimes.has("claude")
      ? []
      : [...(previousReceipt?.claudeAgents ?? [])];
    const retainedRuntime = (runtime) => !selected.runtimes.includes(runtime) || unavailableRuntimes.has(runtime);
    const handlerReceipts = (previousReceipt?.handlers ?? []).filter((entry) => retainedRuntime(entry.runtime));
    const handlerConflictReceipts = (previousReceipt?.handlerConflicts ?? []).filter((entry) => retainedRuntime(entry.runtime));
    const resourceConflictReceipts = (previousReceipt?.resourceConflicts ?? []).filter((entry) => retainedRuntime(entry.runtime));
    for (const sourceFile of selected.runtimes.includes("claude") && !unavailableRuntimes.has("claude") ? manifest.files.filter((file) => file.startsWith("assets/claude-agents/") && file.endsWith(".md")) : []) {
      const target = path.join(runtimePaths({ home, ...selected }, "claude").agentsRoot, path.basename(sourceFile));
      const source = path.join(sourceRoot, sourceFile);
      const desiredDigest = await fileDigest(source);
      const currentDigest = await fileDigest(target);
      const previous = previousReceipt?.claudeAgents?.find((entry) => entry.path === target);
      if (currentDigest === desiredDigest) {
        claudeAgents.push({ path: target, source: sourceFile, digest: desiredDigest, preexisting: previous?.preexisting ?? !previous });
        continue;
      }
      if (previous?.preexisting) {
        conflicts.push({ kind: "claude_agent", runtime: "claude", target, reason: "preexisting_definition" });
        claudeAgents.push(previous);
        continue;
      }
      if (currentDigest && (!previous || previous.digest !== currentDigest)) {
        conflicts.push({ kind: "claude_agent", target, reason: "custom_definition" });
        if (previous) claudeAgents.push(previous);
        continue;
      }
      if (currentDigest) {
        const backup = path.join(backupRoot, "claude-agents", path.basename(target));
        await mkdir(path.dirname(backup), { recursive: true });
        await addUndo({
          kind: "restore_file",
          target,
          backup,
          preimageGuard: await resourceGuard(target),
          targetGuard: { kind: "file", digest: desiredDigest },
        });
        await copyFile(target, backup);
        backups.push({ kind: "claude_agent", target, backup, purpose: "update_snapshot", transactionId });
      } else {
        await addUndo({ kind: "remove_path", path: target, guard: { kind: "file", digest: desiredDigest } });
      }
      await atomicText(target, await readFile(source, "utf8"));
      claudeAgents.push({ path: target, source: sourceFile, digest: desiredDigest });
      changed = true;
    }

    for (const runtime of selected.runtimes.filter((runtime) => !unavailableRuntimes.has(runtime))) {
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
        const mergedText = `${JSON.stringify(merged.config, null, 2)}\n`;
        const mergedGuard = textGuard(mergedText);
        if (configPresent) {
          const backup = path.join(backupRoot, "config", runtime === "codex" ? "hooks.json" : "settings.json");
          await mkdir(path.dirname(backup), { recursive: true });
          await addUndo({ kind: "restore_file", target: configPath, backup, preimageGuard: await resourceGuard(configPath), targetGuard: mergedGuard });
          await copyFile(configPath, backup);
          backups.push({ kind: "config", target: configPath, backup, purpose: "update_snapshot", transactionId });
        } else {
          await addUndo({ kind: "remove_path", path: configPath, guard: mergedGuard });
        }
        await atomicText(configPath, mergedText);
        changed = true;
      }
    }

    const installedRuntimes = [...new Set([...targets.map(({ runtime }) => runtime), ...handlerReceipts.map(({ runtime }) => runtime)])];
    const resourceConflicts = [...new Map([...resourceConflictReceipts, ...conflicts.filter(({ kind }) => kind === "skill")]
      .map((entry) => [`${entry.kind}:${entry.runtime}:${entry.target}:${entry.reason}`, entry])).values()];
    const receipt = {
      schemaVersion: 3,
      transactionId,
      version: manifest.version,
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
      resourceConflicts,
      backups: [...(previousReceipt?.backups ?? []), ...backups],
    };
    if (changed || !previousReceipt || previousReceipt.schemaVersion !== 3 || !Array.isArray(previousReceipt.handlers)) {
      const receiptText = `${JSON.stringify(receipt, null, 2)}\n`;
      const receiptGuard = textGuard(receiptText);
      if (previousReceipt) {
        const backup = path.join(backupRoot, "receipt", "install.json");
        await mkdir(path.dirname(backup), { recursive: true });
        await addUndo({ kind: "restore_file", target: receiptPath, backup, preimageGuard: await resourceGuard(receiptPath), targetGuard: receiptGuard });
        await copyFile(receiptPath, backup);
      } else {
        await addUndo({ kind: "remove_path", path: receiptPath, guard: receiptGuard });
      }
      await atomicText(receiptPath, receiptText);
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
  const result = await withRecoverableInstallLock(stateRoot, {
    pid: process.pid,
    operation: "install",
    acquiredAt: now.toISOString(),
  }, async () => {
    const recovery = await recoverTransaction(stateRoot);
    if (recovery?.conflicts?.length) throw new Error(`Recovery conflicts require manual resolution: ${recovery.conflicts.map(({ target, reason }) => `${target}: ${reason}`).join("; ")}`);
    return { ...await installLocked({ sourceRoot, home, now, selected }, stateRoot), ...(recovery ? { recovery } : {}) };
  }, { timeoutMs: 5000 });
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
  const activeStoredHandlerConflicts = [];
  for (const entry of (receipt.handlerConflicts ?? []).filter((candidate) => selected.runtimes.includes(candidate.runtime))) {
    if (!entry.handler) {
      activeStoredHandlerConflicts.push(entry);
      continue;
    }
    const configPath = entry.target ?? runtimePaths({ home, ...selected }, entry.runtime).configPath;
    const config = await readJson(configPath, {});
    if (exactLocations(config, entry.event, entry.handler).length || identityLocations(config, entry.event, entry.handler).length) {
      activeStoredHandlerConflicts.push(entry);
    }
  }
  const conflicts = [
    ...activeStoredHandlerConflicts,
    ...(receipt.resourceConflicts ?? []).filter((entry) => selected.runtimes.includes(entry.runtime)),
  ];
  const removalRoot = path.join(stateRoot, "removals", randomUUID());

  const result = await durableTransaction(stateRoot, "uninstall", async (addUndo) => {
    for (const runtime of selected.runtimes) {
      const { configPath } = runtimePaths({ home, ...selected }, runtime);
      const configPresent = await present(configPath);
      const original = configPresent ? await readFile(configPath, "utf8") : undefined;
      const config = configPresent ? JSON.parse(original) : {};
      const cleaned = removeOwnedHooks(config, (receipt.handlers ?? []).filter((entry) => entry.runtime === runtime && (!entry.configPath || entry.configPath === configPath)));
      conflicts.push(...cleaned.conflicts.map((entry) => ({ ...entry, target: configPath })));
      if (cleaned.changed) {
        const cleanedText = `${JSON.stringify(cleaned.config, null, 2)}\n`;
        const cleanedGuard = textGuard(cleanedText);
        if (configPresent) {
          const backup = path.join(removalRoot, "config", runtime === "codex" ? "hooks.json" : "settings.json");
          await mkdir(path.dirname(backup), { recursive: true });
          await addUndo({ kind: "restore_file", target: configPath, backup, preimageGuard: await resourceGuard(configPath), targetGuard: cleanedGuard });
          await copyFile(configPath, backup);
        } else {
          await addUndo({ kind: "remove_path", path: configPath, guard: cleanedGuard });
        }
        await atomicText(configPath, cleanedText);
      }
    }

    const normalizedTargets = (receipt.targets ?? []).map((entry, index) => typeof entry === "string"
      ? { runtime: index === 0 ? "codex" : "claude", path: entry, mode: "legacy", digest: receipt.digest }
      : entry);
    const blockedRuntimes = new Set(conflicts.filter(({ kind }) => kind === "handler").map(({ runtime }) => runtime));
    for (const target of normalizedTargets.filter((entry) => selected.runtimes.includes(entry.runtime) && !blockedRuntimes.has(entry.runtime))) {
      if (!(await present(target.path))) continue;
      if (target.preexisting || target.mode === "source") continue;
      let files = target.files;
      if (!files) files = (await readJson(path.join(target.path, "hooks", "manifest.json"), { files: [] })).files;
      const currentDigest = await managedPackageDigest(target.path, files ?? []);
      if (!currentDigest || currentDigest !== target.digest) {
        conflicts.push({ kind: "skill", runtime: target.runtime, target: target.path, reason: "managed_target_changed" });
        continue;
      }
      const removed = path.join(removalRoot, "skills", target.runtime);
      await mkdir(path.dirname(removed), { recursive: true });
      await addUndo({ kind: "move", from: removed, to: target.path, fromGuard: { kind: "package", files: files ?? [], digest: currentDigest } });
      await rename(target.path, removed);
      const backup = (receipt.backups ?? []).find((entry) => entry.kind === "skill"
        && entry.target === target.path
        && entry.backup
        && entry.purpose !== "update_snapshot");
      if (backup && await present(backup.backup)) {
        await mkdir(path.dirname(target.path), { recursive: true });
        await addUndo({ kind: "move", from: target.path, to: backup.backup, fromGuard: await resourceGuard(backup.backup) });
        await rename(backup.backup, target.path);
      }
    }

    for (const agent of selected.runtimes.includes("claude") && !blockedRuntimes.has("claude") ? receipt.claudeAgents ?? [] : []) {
      if (!(await present(agent.path))) continue;
      if (agent.preexisting) {
        conflicts.push({ kind: "claude_agent", runtime: "claude", target: agent.path, reason: "preexisting_definition" });
        continue;
      }
      if (await fileDigest(agent.path) !== agent.digest) {
        conflicts.push({ kind: "claude_agent", runtime: "claude", target: agent.path, reason: "custom_definition" });
        continue;
      }
      const removed = path.join(removalRoot, "claude-agents", path.basename(agent.path));
      await mkdir(path.dirname(removed), { recursive: true });
      await addUndo({ kind: "move", from: removed, to: agent.path, fromGuard: { kind: "file", digest: agent.digest } });
      await rename(agent.path, removed);
    }

    for (const backup of selected.runtimes.includes("codex") ? [...(receipt.backups ?? [])].reverse().filter(({ kind }) => kind === "legacy") : []) {
      if (!(await present(backup.target)) && await present(backup.backup)) {
        await mkdir(path.dirname(backup.target), { recursive: true });
        await addUndo({ kind: "move", from: backup.target, to: backup.backup, fromGuard: await resourceGuard(backup.backup) });
        await rename(backup.backup, backup.target);
      }
    }

    const unselectedRuntimes = new Set(["codex", "claude"].filter((runtime) => !selected.runtimes.includes(runtime)));
    const handlerConflictRuntimes = new Set(conflicts.filter(({ kind }) => kind === "handler").map(({ runtime }) => runtime));
    const handlerConflictIds = new Set(conflicts.filter(({ kind }) => kind === "handler")
      .map(({ runtime, handlerId: id }) => `${runtime}:${id}`));
    const targetConflictRuntimes = new Set(conflicts.filter(({ kind }) => ["handler", "skill"].includes(kind)).map(({ runtime }) => runtime));
    const claudeConflictPaths = new Set(conflicts.filter(({ kind, runtime }) => runtime === "claude" && kind === "claude_agent").map(({ target }) => target));
    const remainingTargets = normalizedTargets.filter((entry) => unselectedRuntimes.has(entry.runtime) || targetConflictRuntimes.has(entry.runtime));
    const remainingHandlers = (receipt.handlers ?? []).filter((entry) => unselectedRuntimes.has(entry.runtime)
      || handlerConflictIds.has(`${entry.runtime}:${entry.handlerId}`));
    const remainingHandlerConflicts = (receipt.handlerConflicts ?? []).filter((entry) => unselectedRuntimes.has(entry.runtime)
      || conflicts.some((conflict) => conflict.kind === "handler" && conflict.runtime === entry.runtime && conflict.handlerId === entry.handlerId));
    const remainingResourceConflicts = (receipt.resourceConflicts ?? []).filter((entry) => unselectedRuntimes.has(entry.runtime)
      || conflicts.some((conflict) => conflict.kind === entry.kind && conflict.runtime === entry.runtime && conflict.target === entry.target && conflict.reason === entry.reason));
    const remainingClaudeAgents = (receipt.claudeAgents ?? []).filter((entry) => unselectedRuntimes.has("claude")
      || blockedRuntimes.has("claude")
      || claudeConflictPaths.has(entry.path));
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
      resourceConflicts: remainingResourceConflicts,
      claudeAgents: remainingClaudeAgents,
      backups: (receipt.backups ?? []).filter((entry) => {
        if (entry.kind === "legacy") return remainingRuntimes.includes("codex");
        if (entry.kind === "claude_agent") return remainingClaudeAgents.some((agent) => agent.path === entry.target);
        if (entry.kind === "skill") return remainingTargets.some((target) => target.path === entry.target);
        if (entry.kind === "config") return remainingHandlers.some((handler) => handler.configPath === entry.target);
        return true;
      }),
    };
    const hasRemaining = (remaining.targets?.length ?? 0) || (remaining.handlers?.length ?? 0) || (remaining.handlerConflicts?.length ?? 0)
      || (remaining.resourceConflicts?.length ?? 0) || (remaining.claudeAgents?.length ?? 0);
    const receiptBackup = path.join(removalRoot, "receipt", "install.json");
    await mkdir(path.dirname(receiptBackup), { recursive: true });
    const nextReceipt = conflicts.length || hasRemaining ? conflicts.length ? { ...remaining, status: "uninstall_conflicts", conflicts } : remaining : null;
    const nextReceiptText = nextReceipt ? `${JSON.stringify(nextReceipt, null, 2)}\n` : undefined;
    await addUndo({
      kind: "restore_file",
      target: receiptPath,
      backup: receiptBackup,
      preimageGuard: await resourceGuard(receiptPath),
      targetGuard: nextReceiptText ? textGuard(nextReceiptText) : { kind: "absent" },
    });
    await copyFile(receiptPath, receiptBackup);
    if (nextReceiptText) await atomicText(receiptPath, nextReceiptText);
    else await rm(receiptPath, { force: true });
    return { status: conflicts.length ? "uninstalled_with_conflicts" : "uninstalled", changed: true, conflicts };
  });
  await rm(removalRoot, { force: true, recursive: true });
  return result;
}

/** Unregister hooks and remove only unchanged files owned by the install receipt. */
export async function uninstallPackage({ home, host, scope, projectRoot, trustedHost }) {
  const selected = selection({ host, scope, projectRoot, trustedHost });
  const stateRoot = path.join(selected.scope === "project" ? selected.projectRoot : home, ".agent-team-hooks");
  return withRecoverableInstallLock(stateRoot, {
    pid: process.pid,
    operation: "uninstall",
    acquiredAt: new Date().toISOString(),
  }, async () => {
    const recovery = await recoverTransaction(stateRoot);
    if (recovery?.conflicts?.length) throw new Error(`Recovery conflicts require manual resolution: ${recovery.conflicts.map(({ target, reason }) => `${target}: ${reason}`).join("; ")}`);
    const result = await uninstallLocked({ home, selected }, stateRoot);
    return { ...result, ...(recovery ? { recovery } : {}) };
  }, { timeoutMs: 5000 });
}

/** Roll back the selected installed host/scope using the same ownership receipt. */
export async function rollbackPackage(options) {
  return uninstallPackage(options);
}

export { mergeHooks, removeOwnedHooks };
