import { createHash, randomUUID } from "node:crypto";
import { chmod, lstat, mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { withDirectoryLock } from "./lock.mjs";

export const HELPER_NAMES = Object.freeze(["completion-evidence.mjs", "gate-rebind.mjs", "with-env.mjs"]);
const DEFAULT_SOURCE = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "../../assets/helpers");

function digest(bytes) {
  return createHash("sha256").update(bytes).digest("hex");
}

function exactKeys(value, keys) {
  return value && typeof value === "object" && !Array.isArray(value)
    && Object.keys(value).sort().join("\0") === [...keys].sort().join("\0");
}

async function lstatOrMissing(file) {
  try { return await lstat(file); }
  catch (error) { if (error.code === "ENOENT") return null; throw error; }
}

async function safeDirectory(root, directory, create) {
  const relative = path.relative(root, directory);
  if (!path.isAbsolute(root) || relative.startsWith("..") || path.isAbsolute(relative)) throw new Error("unsafe directory scope");
  let current = root;
  const rootStat = await lstatOrMissing(root);
  if (!rootStat?.isDirectory() || rootStat.isSymbolicLink()) throw new Error(`unsafe directory ancestor: ${root}`);
  for (const component of relative.split(path.sep).filter(Boolean)) {
    current = path.join(current, component);
    let state = await lstatOrMissing(current);
    if (!state) {
      if (!create) return false;
      try { await mkdir(current, { mode: 0o700 }); }
      catch (error) { if (error.code !== "EEXIST") throw error; }
      state = await lstatOrMissing(current);
    }
    if (!state?.isDirectory() || state.isSymbolicLink()) throw new Error(`unsafe directory ancestor: ${current}`);
  }
  return true;
}

async function ensureSafeDirectory(root, directory) {
  await safeDirectory(root, directory, true);
}

async function maybeRead(file, root) {
  if (!await safeDirectory(root, path.dirname(file), false)) return null;
  const state = await lstatOrMissing(file);
  if (!state) return null;
  if (!state.isFile() || state.isSymbolicLink()) throw new Error(`unsafe file target: ${file}`);
  return readFile(file);
}

async function atomicWrite(file, bytes, mode, root, expectedSha256) {
  await ensureSafeDirectory(root, path.dirname(file));
  const current = await maybeRead(file, root);
  if ((current ? digest(current) : null) !== expectedSha256) throw new Error(`write_conflict: ${file}`);
  const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
  try {
    await writeFile(temporary, bytes, { mode, flag: "wx" });
    await rename(temporary, file);
    await chmod(file, mode);
  } finally {
    await rm(temporary, { force: true });
  }
}

async function removeIfCurrent(file, expectedSha256, root) {
  const current = await maybeRead(file, root);
  if ((current ? digest(current) : null) !== expectedSha256) throw new Error(`write_conflict: ${file}`);
  if (current) await rm(file);
}

function pathsFor(projectRoot, receiptPath) {
  if (!path.isAbsolute(projectRoot)) throw new Error("projectRoot must be absolute");
  const canonicalReceipt = path.join(projectRoot, ".agent-team", "helpers.json");
  if (receiptPath !== undefined && path.resolve(receiptPath) !== canonicalReceipt) throw new Error("helper receipt path must be canonical");
  return {
    targetRoot: path.join(projectRoot, "scripts", "agent-team"),
    receiptPath: canonicalReceipt,
    journalPath: path.join(projectRoot, ".agent-team", "helper-transaction.json"),
    lockPath: path.join(projectRoot, ".agent-team", ".locks", "helpers.lock"),
  };
}

function makeReceipt(locations, files) {
  const core = { schemaVersion: 1, kind: "agent-team-helpers", targetRoot: locations.targetRoot,
    receiptPath: locations.receiptPath, files };
  return { ...core, digest: digest(Buffer.from(JSON.stringify(core))) };
}

function validReceipt(receipt, locations) {
  if (!exactKeys(receipt, ["schemaVersion", "kind", "targetRoot", "receiptPath", "files", "digest"])
    || receipt.schemaVersion !== 1 || receipt.kind !== "agent-team-helpers"
    || receipt.targetRoot !== locations.targetRoot || receipt.receiptPath !== locations.receiptPath
    || !receipt.files || typeof receipt.files !== "object" || Array.isArray(receipt.files)) return false;
  for (const [name, entry] of Object.entries(receipt.files)) {
    if (!HELPER_NAMES.includes(name) || !exactKeys(entry, ["sha256", "sourceSha256", "mode"])
      || !/^[0-9a-f]{64}$/i.test(entry.sha256) || entry.sourceSha256 !== entry.sha256 || entry.mode !== 0o755) return false;
  }
  const { digest: recorded, ...core } = receipt;
  return recorded === digest(Buffer.from(JSON.stringify(core)));
}

function canonicalStatus(result, receipt) {
  if (!exactKeys(result, ["status", "receiptDigest"]) || result.receiptDigest !== receipt.digest
    || !["committed", "not_committed"].includes(result.status)) return "unknown";
  return result.status;
}

async function updateJournalStatus(file, journal, bytes, status, root) {
  const next = { ...journal, status };
  const nextBytes = Buffer.from(`${JSON.stringify(next, null, 2)}\n`);
  await atomicWrite(file, nextBytes, 0o600, root, digest(bytes));
  return { journal: next, bytes: nextBytes };
}

function transactionDigestFor(journal) {
  return digest(Buffer.from(`${JSON.stringify({ ...journal, status: "prepared" }, null, 2)}\n`));
}

function makeRecoveryReceipt(journal, receiptBefore, locations) {
  let restoredReceiptDigest = null;
  if (receiptBefore) {
    try {
      const restored = JSON.parse(receiptBefore);
      if (validReceipt(restored, locations)) restoredReceiptDigest = restored.digest;
    } catch {}
  }
  const core = { schemaVersion: 1, kind: "agent-team-helpers-recovery", action: "rolled_back",
    transactionDigest: transactionDigestFor(journal), priorReceiptDigest: journal.setupReceipt.digest,
    restoredReceiptDigest };
  return { ...core, digest: digest(Buffer.from(JSON.stringify(core))) };
}

async function readReceipt(file, root, locations) {
  const bytes = await maybeRead(file, root);
  if (!bytes) return null;
  try {
    const receipt = JSON.parse(bytes);
    return validReceipt(receipt, locations) ? receipt : null;
  } catch { return null; }
}

function validRecoveryReceipt(receipt) {
  if (!exactKeys(receipt, ["schemaVersion", "kind", "action", "transactionDigest", "priorReceiptDigest",
    "restoredReceiptDigest", "digest"])
    || receipt.schemaVersion !== 1 || receipt.kind !== "agent-team-helpers-recovery" || receipt.action !== "rolled_back"
    || !/^[0-9a-f]{64}$/.test(receipt.transactionDigest) || !/^[0-9a-f]{64}$/.test(receipt.priorReceiptDigest)
    || !(receipt.restoredReceiptDigest === null || /^[0-9a-f]{64}$/.test(receipt.restoredReceiptDigest))) return false;
  const { digest: recorded, ...core } = receipt;
  return recorded === digest(Buffer.from(JSON.stringify(core)));
}

/** Bind the canonical setup receipt to the validated local helper installation or restored state. */
export function helperReceiptBinding({ canonicalReceipt, localReceipt, receiptStatus, projectRoot } = {}) {
  const locations = pathsFor(projectRoot);
  if (!canonicalReceipt && receiptStatus === "missing") return { status: "empty" };
  if (canonicalReceipt?.kind === "agent-team-helpers" && localReceipt
    && validReceipt(canonicalReceipt, locations) && validReceipt(localReceipt, locations)
    && canonicalReceipt.digest === localReceipt.digest) return { status: "linked" };
  if (canonicalReceipt?.kind === "agent-team-helpers-recovery" && validRecoveryReceipt(canonicalReceipt)) {
    if (canonicalReceipt.restoredReceiptDigest === null && receiptStatus === "missing") return { status: "linked" };
    if (receiptStatus === "valid" && localReceipt && validReceipt(localReceipt, locations)
      && localReceipt.digest === canonicalReceipt.restoredReceiptDigest) return { status: "linked" };
  }
  return { status: "mismatch" };
}

export async function inspectHelpers({ projectRoot, sourceRoot = DEFAULT_SOURCE, receiptPath } = {}) {
  const locations = pathsFor(projectRoot, receiptPath);
  const receiptBytes = await maybeRead(locations.receiptPath, projectRoot);
  let receipt = null;
  if (receiptBytes) {
    try {
      const parsed = JSON.parse(receiptBytes);
      if (validReceipt(parsed, locations)) receipt = parsed;
    } catch {}
  }
  const receiptStatus = !receiptBytes ? "missing" : receipt ? "valid" : "invalid";
  const journal = await maybeRead(locations.journalPath, projectRoot);
  const files = [];
  for (const name of HELPER_NAMES) {
    const source = await readFile(path.join(sourceRoot, name));
    const target = path.join(locations.targetRoot, name);
    const targetBytes = await maybeRead(target, projectRoot);
    const expected = digest(source);
    const recorded = receipt?.files?.[name]?.sha256;
    let status = "missing";
    if (targetBytes) {
      const actual = digest(targetBytes);
      status = actual === expected && recorded === actual ? "current"
        : actual === expected ? "unrecorded"
          : "customized";
    }
    files.push({ name, target, status, expectedSha256: expected, recordedSha256: recorded ?? null,
      actualSha256: targetBytes ? digest(targetBytes) : null });
  }
  const current = files.every(({ status }) => status === "current");
  return { status: journal ? "recovery_required" : current ? "current" : "drift", receiptPath: locations.receiptPath,
    targetRoot: locations.targetRoot, receipt: receipt ? structuredClone(receipt) : null, receiptStatus, files, pendingTransaction: Boolean(journal),
    pendingTransactionDigest: journal ? digest(journal) : null };
}

export async function installHelpers({ projectRoot, sourceRoot = DEFAULT_SOURCE, receiptPath, recordSetupReceipt } = {}) {
  const locations = pathsFor(projectRoot, receiptPath);
  if (typeof recordSetupReceipt !== "function") throw new Error("recordSetupReceipt is required before helper installation");
  await ensureSafeDirectory(projectRoot, path.dirname(locations.lockPath));
  return withDirectoryLock(locations.lockPath, { kind: "helper-install", pid: process.pid }, async () => {
    if (await maybeRead(locations.journalPath, projectRoot)) throw new Error("recovery_required: helper transaction is pending");
    const previousReceiptBytes = await maybeRead(locations.receiptPath, projectRoot);
    const previousReceipt = await readReceipt(locations.receiptPath, projectRoot, locations);
    await ensureSafeDirectory(projectRoot, path.dirname(locations.targetRoot));

    const planned = [];
    const conflicts = [];
    const nextFiles = { ...(previousReceipt?.files ?? {}) };
    for (const name of HELPER_NAMES) {
      const source = await readFile(path.join(sourceRoot, name));
      const sourceSha256 = digest(source);
      const target = path.join(locations.targetRoot, name);
      const current = await maybeRead(target, projectRoot);
      const currentSha256 = current ? digest(current) : null;
      const recorded = previousReceipt?.files?.[name]?.sha256;
      if (current && currentSha256 !== sourceSha256 && currentSha256 !== recorded) {
        conflicts.push({ name, target, reason: "customized_target", actualSha256: currentSha256, expectedSha256: sourceSha256 });
        continue;
      }
      nextFiles[name] = { sha256: sourceSha256, sourceSha256, mode: 0o755 };
      if (!current || currentSha256 !== sourceSha256) planned.push({ name, target, source, before: current,
        beforeSha256: currentSha256, afterSha256: sourceSha256 });
    }
    const setupReceipt = makeReceipt(locations, nextFiles);
    const receiptBytes = Buffer.from(`${JSON.stringify(setupReceipt, null, 2)}\n`);
    const receiptChanged = !previousReceiptBytes?.equals(receiptBytes);
    if (!planned.length && !receiptChanged) {
      return { status: conflicts.length ? "conflict" : "current", changed: false, conflicts, setupReceipt };
    }
    let journal = { schemaVersion: 1, kind: "helper-install", status: "prepared",
      targets: planned.map(({ name, target, before, beforeSha256, afterSha256 }) => ({ name, target,
        before: before?.toString("base64") ?? null, beforeSha256, afterSha256 })),
      receipt: { before: previousReceiptBytes?.toString("base64") ?? null,
        beforeSha256: previousReceiptBytes ? digest(previousReceiptBytes) : null, afterSha256: digest(receiptBytes) }, setupReceipt };
    let journalBytes = Buffer.from(`${JSON.stringify(journal, null, 2)}\n`);
    await atomicWrite(locations.journalPath, journalBytes, 0o600, projectRoot, null);
    const applied = [];
    let receiptApplied = false;
    const rollBackLocal = async () => {
      for (const item of applied) {
        const current = await maybeRead(item.target, projectRoot);
        if ((current ? digest(current) : null) !== item.afterSha256) throw new Error(`write_conflict: ${item.target}`);
      }
      if (receiptApplied) {
        const currentReceipt = await maybeRead(locations.receiptPath, projectRoot);
        if ((currentReceipt ? digest(currentReceipt) : null) !== digest(receiptBytes)) {
          throw new Error(`write_conflict: ${locations.receiptPath}`);
        }
      }
      for (const item of applied.toReversed()) {
        if (item.before) await atomicWrite(item.target, item.before, 0o755, projectRoot, item.afterSha256);
        else await removeIfCurrent(item.target, item.afterSha256, projectRoot);
      }
      if (receiptApplied) {
        if (previousReceiptBytes) await atomicWrite(locations.receiptPath, previousReceiptBytes, 0o600, projectRoot, digest(receiptBytes));
        else await removeIfCurrent(locations.receiptPath, digest(receiptBytes), projectRoot);
      }
    };
    try {
      for (const item of planned) {
        await atomicWrite(item.target, item.source, 0o755, projectRoot, item.beforeSha256);
        applied.push(item);
      }
      await atomicWrite(locations.receiptPath, receiptBytes, 0o600, projectRoot,
        previousReceiptBytes ? digest(previousReceiptBytes) : null);
      receiptApplied = true;
      ({ journal, bytes: journalBytes } = await updateJournalStatus(locations.journalPath, journal, journalBytes,
        "local_applied", projectRoot));
    } catch (error) {
      try {
        await rollBackLocal();
        await removeIfCurrent(locations.journalPath, digest(journalBytes), projectRoot);
      } catch (rollbackError) {
        throw new Error(`recovery_conflict: helper rollback stopped safely: ${rollbackError.message}`, { cause: error });
      }
      throw error;
    }
    let result;
    try { result = await recordSetupReceipt(structuredClone(setupReceipt)); }
    catch (error) { throw new Error(`canonical_outcome_unknown: ${error.message}`, { cause: error }); }
    const outcome = canonicalStatus(result, setupReceipt);
    if (outcome === "unknown") throw new Error("canonical_outcome_unknown: invalid setup receipt result");
    if (outcome === "not_committed") {
      try {
        ({ journal, bytes: journalBytes } = await updateJournalStatus(locations.journalPath, journal, journalBytes,
          "canonical_not_committed", projectRoot));
        await rollBackLocal();
        await removeIfCurrent(locations.journalPath, digest(journalBytes), projectRoot);
      } catch (rollbackError) {
        throw new Error(`recovery_conflict: helper rollback stopped safely: ${rollbackError.message}`);
      }
      throw new Error("canonical setup receipt was not committed");
    }
    try {
      ({ journal, bytes: journalBytes } = await updateJournalStatus(locations.journalPath, journal, journalBytes,
        "canonical_committed", projectRoot));
      await removeIfCurrent(locations.journalPath, digest(journalBytes), projectRoot);
    } catch (error) {
      throw new Error(`recovery_required: canonical receipt committed but helper finalization failed: ${error.message}`, { cause: error });
    }
    return { status: conflicts.length ? "conflict" : "installed", changed: true,
      installed: planned.map(({ name }) => name), conflicts, setupReceipt };
  });
}

function decodeBefore(value, expectedSha256) {
  if (value === null) return expectedSha256 === null ? null : undefined;
  if (typeof value !== "string") return undefined;
  const bytes = Buffer.from(value, "base64");
  return digest(bytes) === expectedSha256 ? bytes : undefined;
}

export async function recoverHelperTransaction({ projectRoot, sourceRoot = DEFAULT_SOURCE, receiptPath,
  expectedJournalDigest, recordSetupReceipt } = {}) {
  const locations = pathsFor(projectRoot, receiptPath);
  if (!/^[0-9a-f]{64}$/i.test(expectedJournalDigest ?? "")) throw new Error("expected journal digest is required");
  if (typeof recordSetupReceipt !== "function") throw new Error("recordSetupReceipt is required before helper recovery");
  await ensureSafeDirectory(projectRoot, path.dirname(locations.lockPath));
  return withDirectoryLock(locations.lockPath, { kind: "helper-recovery", pid: process.pid }, async () => {
    let journalBytes = await maybeRead(locations.journalPath, projectRoot);
    if (!journalBytes) return { status: "not_pending", changed: false };
    if (digest(journalBytes) !== expectedJournalDigest) throw new Error("journal digest changed");
    let journal;
    try { journal = JSON.parse(journalBytes); } catch { throw new Error("helper recovery journal is invalid"); }
    if (!exactKeys(journal, ["schemaVersion", "kind", "status", "targets", "receipt", "setupReceipt"])
      || journal.schemaVersion !== 1 || journal.kind !== "helper-install"
      || !["prepared", "local_applied", "canonical_not_committed", "canonical_committed",
        "recovery_local_applied", "recovery_canonical_committed"].includes(journal.status)
      || !Array.isArray(journal.targets) || !validReceipt(journal.setupReceipt, locations)
      || !exactKeys(journal.receipt, ["before", "beforeSha256", "afterSha256"])) {
      throw new Error("helper recovery journal is invalid");
    }
    const records = [];
    const names = new Set();
    for (const target of journal.targets) {
      if (!exactKeys(target, ["name", "target", "before", "beforeSha256", "afterSha256"])
        || !HELPER_NAMES.includes(target.name) || names.has(target.name)
        || target.target !== path.join(locations.targetRoot, target.name)) throw new Error("helper recovery journal is invalid");
      names.add(target.name);
      const before = decodeBefore(target.before, target.beforeSha256);
      const after = await readFile(path.join(sourceRoot, target.name));
      if (before === undefined || digest(after) !== target.afterSha256) throw new Error("helper recovery journal is invalid");
      records.push({ file: target.target, before, beforeSha256: target.beforeSha256, after, afterSha256: target.afterSha256 });
    }
    const receiptBefore = decodeBefore(journal.receipt.before, journal.receipt.beforeSha256);
    const receiptAfter = Buffer.from(`${JSON.stringify(journal.setupReceipt, null, 2)}\n`);
    if (receiptBefore === undefined || digest(receiptAfter) !== journal.receipt.afterSha256) throw new Error("helper recovery journal is invalid");
    for (const record of records) {
      const current = await maybeRead(record.file, projectRoot);
      const currentSha256 = current ? digest(current) : null;
      if (![record.beforeSha256, record.afterSha256].includes(currentSha256)) return { status: "conflict", reason: "postimage_changed", changed: false };
      record.currentSha256 = currentSha256;
    }
    const currentReceipt = await maybeRead(locations.receiptPath, projectRoot);
    const currentReceiptSha256 = currentReceipt ? digest(currentReceipt) : null;
    if (![journal.receipt.beforeSha256, journal.receipt.afterSha256].includes(currentReceiptSha256)) {
      return { status: "conflict", reason: "receipt_changed", changed: false };
    }
    if (["canonical_committed", "recovery_canonical_committed"].includes(journal.status)) {
      const recoveryCommitted = journal.status === "recovery_canonical_committed";
      const expectedRecords = recoveryCommitted ? "beforeSha256" : "afterSha256";
      const expectedReceipt = recoveryCommitted ? journal.receipt.beforeSha256 : journal.receipt.afterSha256;
      if (records.some((record) => record.currentSha256 !== record[expectedRecords])
        || currentReceiptSha256 !== expectedReceipt) {
        return { status: "conflict", reason: "committed_postimage_changed", changed: false };
      }
      await removeIfCurrent(locations.journalPath, digest(journalBytes), projectRoot);
      return { status: "finalized", changed: false,
        setupReceipt: recoveryCommitted ? makeRecoveryReceipt(journal, receiptBefore, locations) : journal.setupReceipt };
    }
    if (journal.status === "local_applied") {
      let result;
      try { result = await recordSetupReceipt(structuredClone(journal.setupReceipt)); }
      catch (error) { throw new Error(`canonical_outcome_unknown: ${error.message}`, { cause: error }); }
      const outcome = canonicalStatus(result, journal.setupReceipt);
      if (outcome === "unknown") throw new Error("canonical_outcome_unknown: invalid setup receipt result");
      if (outcome === "committed") {
        if (records.some((record) => record.currentSha256 !== record.afterSha256)
          || currentReceiptSha256 !== journal.receipt.afterSha256) {
          return { status: "conflict", reason: "committed_postimage_changed", changed: false };
        }
        ({ journal, bytes: journalBytes } = await updateJournalStatus(locations.journalPath, journal, journalBytes,
          "canonical_committed", projectRoot));
        await removeIfCurrent(locations.journalPath, digest(journalBytes), projectRoot);
        return { status: "finalized", changed: false, setupReceipt: journal.setupReceipt };
      }
      ({ journal, bytes: journalBytes } = await updateJournalStatus(locations.journalPath, journal, journalBytes,
        "canonical_not_committed", projectRoot));
    }
    const changed = [];
    let receiptChanged = false;
    for (const record of records) {
      if (record.currentSha256 === record.beforeSha256) continue;
      if (record.before) await atomicWrite(record.file, record.before, 0o755, projectRoot, record.afterSha256);
      else await removeIfCurrent(record.file, record.afterSha256, projectRoot);
      changed.push(record);
    }
    if (currentReceiptSha256 !== journal.receipt.beforeSha256) {
      if (receiptBefore) await atomicWrite(locations.receiptPath, receiptBefore, 0o600, projectRoot, journal.receipt.afterSha256);
      else await removeIfCurrent(locations.receiptPath, journal.receipt.afterSha256, projectRoot);
      receiptChanged = true;
    }
    ({ journal, bytes: journalBytes } = await updateJournalStatus(locations.journalPath, journal, journalBytes,
      "recovery_local_applied", projectRoot));
    const recoveryReceipt = makeRecoveryReceipt(journal, receiptBefore, locations);
    let result;
    try { result = await recordSetupReceipt(structuredClone(recoveryReceipt)); }
    catch (error) { throw new Error(`canonical_outcome_unknown: ${error.message}`, { cause: error }); }
    const outcome = canonicalStatus(result, recoveryReceipt);
    if (outcome === "unknown") throw new Error("canonical_outcome_unknown: invalid recovery receipt result");
    if (outcome === "not_committed") throw new Error("canonical recovery receipt was not committed");
    ({ journal, bytes: journalBytes } = await updateJournalStatus(locations.journalPath, journal, journalBytes,
      "recovery_canonical_committed", projectRoot));
    await removeIfCurrent(locations.journalPath, digest(journalBytes), projectRoot);
    return { status: "rolled_back", changed: changed.length > 0 || receiptChanged, setupReceipt: recoveryReceipt };
  });
}
