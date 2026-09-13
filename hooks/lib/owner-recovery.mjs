import { createHash, randomUUID } from "node:crypto";
import { execFile } from "node:child_process";
import { constants } from "node:fs";
import { mkdir, open, readFile, readlink, realpath, rename, rm, unlink } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { promisify } from "node:util";
import { withDirectoryLock } from "./lock.mjs";
import { readTracker } from "./tracker.mjs";

const MAX_REQUEST_BYTES = 256 * 1024;
const validId = (value) => typeof value === "string" && /^[\w.:-]{1,128}$/.test(value)
  && !["none", "unknown", "unassigned", "-"].includes(value.toLowerCase());
const hex = (value, length) => typeof value === "string" && new RegExp(`^[a-f0-9]{${length}}$`).test(value);
const stable = (value) => JSON.stringify(value && typeof value === "object"
  ? Array.isArray(value) ? value.map((entry) => JSON.parse(stable(entry)))
    : Object.fromEntries(Object.keys(value).sort().map((key) => [key, JSON.parse(stable(value[key]))])) : value);
const exactKeys = (value, keys) => value && typeof value === "object" && !Array.isArray(value)
  && stable(Object.keys(value).sort()) === stable([...keys].sort());
const refusal = (reason) => ({ status: "conflict", reason });
const run = promisify(execFile);
const runtimes = new Set(["codex", "claude-code"]);
const timestamp = (value) => typeof value === "string" && value.length <= 128 && Number.isFinite(Date.parse(value));
const qualifiedIdentity = (value) => exactKeys(value, ["host", "sessionId"])
  && runtimes.has(value.host) && validId(value.sessionId);
let recoveryTestObserver;
function setOwnerRecoveryTestObserver(observer) { recoveryTestObserver = observer; }
const observeRecoveryIo = (event) => recoveryTestObserver?.(event);

export function validateOwnershipWriter(writer) {
  if (exactKeys(writer, ["pid", "startTime", "bootId", "host", "pidNamespace"])) {
    return Number.isSafeInteger(writer.pid) && writer.pid > 0 && typeof writer.startTime === "string" && /^\d+$/.test(writer.startTime)
      && typeof writer.bootId === "string" && /^[a-f0-9-]{16,64}$/i.test(writer.bootId)
      && typeof writer.host === "string" && writer.host.length > 0 && writer.host.length <= 255
      && typeof writer.pidNamespace === "string" && /^pid:\[\d+\]$/.test(writer.pidNamespace);
  }
  return exactKeys(writer, ["kind", "host", "invocationId", "approvalId"])
    && writer.kind === "host-bootstrap" && runtimes.has(writer.host) && validId(writer.invocationId) && validId(writer.approvalId);
}

export function validateQualifiedOwnership(ownership) {
  const current = ownership?.current;
  return exactKeys(ownership, ["epoch", "current"]) && Number.isSafeInteger(ownership.epoch) && ownership.epoch > 0
    && exactKeys(current, ["host", "sessionId", "since", "operationId", "writer"])
    && runtimes.has(current.host) && validId(current.sessionId) && timestamp(current.since) && validId(current.operationId)
    && validateOwnershipWriter(current.writer);
}

const receiptKeys = ["operationId", "ownershipEpoch", "oldOwner", "newOwner", "appliedAt"];
export function validateOwnerRecoveryReceipt(receipt) {
  return exactKeys(receipt, receiptKeys) && validId(receipt.operationId) && Number.isSafeInteger(receipt.ownershipEpoch)
    && receipt.ownershipEpoch > 1 && qualifiedIdentity(receipt.oldOwner) && qualifiedIdentity(receipt.newOwner)
    && stable(receipt.oldOwner) !== stable(receipt.newOwner) && timestamp(receipt.appliedAt);
}

const historyEntryKeys = ["operationId", "signature", "receipt", "epoch", "oldOwner", "newOwner", "reason", "approvalId",
  "writer", "livenessEvidence", "priorStateFingerprint", "priorTeamsFingerprint", "priorSetupFingerprint", "priorOwnerHistoryFingerprint", "appliedAt"];
function validStoppedEvidence(value) {
  return exactKeys(value, ["status", "observedAt"]) && value.status === "stopped" && timestamp(value.observedAt);
}

export function validateOwnerHistory(history, currentOwnership) {
  if (!exactKeys(history, ["schemaVersion", "version", "ownership", "entries"]) || history.schemaVersion !== 1
    || !Number.isSafeInteger(history.version) || history.version < 1 || !exactKeys(history.ownership, ["epoch"])
    || !Number.isSafeInteger(history.ownership.epoch) || history.ownership.epoch < 1 || !Array.isArray(history.entries)
    || history.version !== history.ownership.epoch || history.entries.length !== history.ownership.epoch - 1
    || currentOwnership && history.ownership.epoch !== currentOwnership.epoch) return false;
  let priorOwner;
  const operations = new Set();
  for (let index = 0; index < history.entries.length; index += 1) {
    const entry = history.entries[index];
    if (!exactKeys(entry, historyEntryKeys) || !validId(entry.operationId) || operations.has(entry.operationId) || !hex(entry.signature, 64)
      || entry.epoch !== index + 1 || !qualifiedIdentity(entry.oldOwner) || !qualifiedIdentity(entry.newOwner)
      || !validateOwnerRecoveryReceipt(entry.receipt) || entry.receipt.operationId !== entry.operationId
      || entry.receipt.ownershipEpoch !== entry.epoch + 1 || stable(entry.receipt.oldOwner) !== stable(entry.oldOwner)
      || stable(entry.receipt.newOwner) !== stable(entry.newOwner) || entry.receipt.appliedAt !== entry.appliedAt
      || typeof entry.reason !== "string" || !entry.reason || entry.reason.length > 4096 || !validId(entry.approvalId)
      || !validateOwnershipWriter(entry.writer) || entry.writer.kind !== "host-bootstrap" || entry.writer.host !== entry.newOwner.host
      || entry.writer.approvalId !== entry.approvalId
      || !validStoppedEvidence(entry.livenessEvidence) || !hex(entry.priorStateFingerprint, 64)
      || !hex(entry.priorTeamsFingerprint, 64) || !hex(entry.priorSetupFingerprint, 64)
      || !(entry.priorOwnerHistoryFingerprint === null || hex(entry.priorOwnerHistoryFingerprint, 64)) || !timestamp(entry.appliedAt)
      || priorOwner && stable(entry.oldOwner) !== stable(priorOwner)) return false;
    operations.add(entry.operationId);
    priorOwner = entry.newOwner;
  }
  return !currentOwnership || !priorOwner
    || stable(priorOwner) === stable({ host: currentOwnership.current.host, sessionId: currentOwnership.current.sessionId });
}

async function gitIdentity(location) {
  const cwd = await realpath(location);
  const git = async (...args) => (await run("git", ["-C", cwd, ...args], { encoding: "utf8", timeout: 1000, maxBuffer: 16384 })).stdout.trim();
  const nativeTop = await realpath(await git("rev-parse", "--show-toplevel"));
  const commonRaw = await git("rev-parse", "--git-common-dir");
  const commonDirectory = await realpath(path.isAbsolute(commonRaw) ? commonRaw : path.resolve(cwd, commonRaw));
  const canonicalTop = path.basename(commonDirectory) === ".git" ? path.dirname(commonDirectory) : nativeTop;
  return { canonicalTop: await realpath(canonicalTop), commonDirectory, nativeTop, nativeCwd: cwd };
}

export async function validateNativeOwnerAuthority(project, nativeIdentity, ownership, expectedSessionId) {
  if (!ownership) return true;
  const host = nativeIdentity?.host === "claude" ? "claude-code" : nativeIdentity?.host;
  if (!validateQualifiedOwnership(ownership) || nativeIdentity?.observed !== true || host !== ownership.current.host
    || nativeIdentity?.sessionId !== expectedSessionId || expectedSessionId !== ownership.current.sessionId
    || nativeIdentity?.ownershipEpoch !== ownership.epoch || typeof nativeIdentity?.cwd !== "string") return false;
  let derived;
  try { derived = await gitIdentity(nativeIdentity.cwd); }
  catch { return false; }
  return project.root === project.worktreeRoot && project.root === derived.canonicalTop && derived.nativeTop === derived.canonicalTop
    && project.commonDirectory === derived.commonDirectory;
}

const requestKeys = ["operationId", "projectId", "expectedOwnerSessionId", "expectedOwnershipEpoch", "expectedSetupVersion",
  "expectedStateVersion", "expectedStateFingerprint", "expectedTeamsFingerprint", "expectedSetupFingerprint",
  "expectedOwnerHistoryFingerprint", "reason"];

export function validateOwnerRecoveryEnvelope(envelope) {
  if (!exactKeys(envelope, ["schemaVersion", "request"]) || envelope.schemaVersion !== 1
    || Buffer.byteLength(JSON.stringify(envelope)) > MAX_REQUEST_BYTES || !exactKeys(envelope.request, requestKeys)) return "invalid_request";
  const request = envelope.request;
  if (!validId(request.operationId) || !validId(request.projectId) || !validId(request.expectedOwnerSessionId)
    || !Number.isSafeInteger(request.expectedOwnershipEpoch) || request.expectedOwnershipEpoch < 1
    || !Number.isSafeInteger(request.expectedSetupVersion) || request.expectedSetupVersion < 1
    || !Number.isSafeInteger(request.expectedStateVersion) || request.expectedStateVersion < 0
    || ![request.expectedStateFingerprint, request.expectedTeamsFingerprint, request.expectedSetupFingerprint].every((value) => hex(value, 64))
    || !(request.expectedOwnerHistoryFingerprint === null || hex(request.expectedOwnerHistoryFingerprint, 64))
    || typeof request.reason !== "string" || request.reason.trim() !== request.reason || !request.reason
    || Buffer.byteLength(request.reason) > 4096) return "invalid_request";
  return undefined;
}

export async function readOwnerRecoveryEnvelope(file) {
  let handle;
  try {
    handle = await open(path.resolve(file), constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
    const stat = await handle.stat();
    if (!stat.isFile() || stat.size > MAX_REQUEST_BYTES) throw new Error("Request must be a bounded regular JSON file.");
    const bytes = Buffer.alloc(stat.size);
    let offset = 0;
    while (offset < bytes.length) {
      const { bytesRead } = await handle.read(bytes, offset, bytes.length - offset, offset);
      if (!bytesRead) break;
      offset += bytesRead;
    }
    if (offset !== bytes.length) throw new Error("Request changed while reading.");
    return JSON.parse(bytes);
  } catch (error) {
    if (error instanceof SyntaxError) throw new Error("Request must contain valid JSON.");
    throw error;
  } finally { await handle?.close(); }
}

const grants = new WeakMap();
const consumed = new WeakSet();

// Intentionally private. Current host adapters do not call this; source-only tests may instrument this module.
async function mintOwnerRecoveryCapabilityFromHostBootstrap(binding) {
  const keys = ["expiresAt", "expectedOwnershipEpoch", "invocationId", "nativeCwd", "projectRoot", "replacementRuntime", "replacementSessionId"];
  if (!exactKeys(binding, keys) || !validId(binding.invocationId) || !validId(binding.replacementSessionId)
    || !runtimes.has(binding.replacementRuntime) || !Number.isSafeInteger(binding.expectedOwnershipEpoch) || binding.expectedOwnershipEpoch < 1
    || typeof binding.nativeCwd !== "string" || typeof binding.projectRoot !== "string"
    || !Number.isSafeInteger(binding.expiresAt) || binding.expiresAt <= Date.now() || binding.expiresAt > Date.now() + 60_000) {
    throw new Error("invalid_owner_recovery_binding");
  }
  let derived;
  try { derived = await gitIdentity(binding.nativeCwd); }
  catch { throw new Error("invalid_owner_recovery_binding"); }
  const projectRoot = await realpath(binding.projectRoot);
  if (projectRoot !== derived.canonicalTop || derived.nativeTop !== derived.canonicalTop) throw new Error("invalid_owner_recovery_binding");
  const capability = Object.freeze(Object.create(null));
  grants.set(capability, Object.freeze({ ...structuredClone(binding), ...derived, projectRoot }));
  return capability;
}

export async function validateNativeRecoveryContext(context, project, expectedOwnershipEpoch, now = Date.now()) {
  const capability = context?.capability;
  const grant = capability && typeof capability === "object" ? grants.get(capability) : undefined;
  const identity = context?.identity;
  if (!grant || consumed.has(capability) || grant.expiresAt <= now || !exactKeys(identity,
    ["host", "sessionId", "invocationId", "projectRoot", "worktreeRoot", "cwd", "ownershipEpoch"])
    || grant.invocationId !== identity.invocationId || grant.expectedOwnershipEpoch !== expectedOwnershipEpoch
    || identity.ownershipEpoch !== grant.expectedOwnershipEpoch
    || grant.replacementRuntime !== identity.host || grant.replacementSessionId !== identity.sessionId
    || identity.projectRoot !== grant.projectRoot || identity.worktreeRoot !== grant.canonicalTop || identity.cwd !== grant.nativeCwd
    || project?.root !== grant.canonicalTop || project?.worktreeRoot !== grant.canonicalTop || project?.commonDirectory !== grant.commonDirectory
    || typeof context.inspectSession !== "function" || typeof context.confirm !== "function") return false;
  // Claim the opaque token synchronously. No concurrent caller may cross the first await with the same capability.
  consumed.add(capability);
  let derived;
  try { derived = await gitIdentity(identity.cwd); }
  catch { return false; }
  if (stable(derived) !== stable({ canonicalTop: grant.canonicalTop, commonDirectory: grant.commonDirectory,
    nativeTop: grant.nativeTop, nativeCwd: grant.nativeCwd })) return false;
  return true;
}

export const sha256 = (source) => createHash("sha256").update(source).digest("hex");
export const encodeJson = (value) => Buffer.from(`${JSON.stringify(value, null, 2)}\n`);

async function fsyncDirectory(directory) {
  const handle = await open(directory, constants.O_RDONLY | constants.O_DIRECTORY);
  try { await handle.sync(); } finally { await handle.close(); }
}

async function safeBytes(file, { absent = false } = {}) {
  let handle;
  try {
    handle = await open(file, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
    const stat = await handle.stat();
    if (!stat.isFile() || stat.size > 1024 * 1024) throw new Error("unsafe_owner_recovery_record");
    const bytes = Buffer.alloc(stat.size);
    let offset = 0;
    while (offset < bytes.length) {
      const { bytesRead } = await handle.read(bytes, offset, bytes.length - offset, offset);
      if (!bytesRead) break;
      offset += bytesRead;
    }
    if (offset !== bytes.length) throw new Error("owner_recovery_record_changed");
    return bytes;
  } catch (error) {
    if (absent && error.code === "ENOENT") return null;
    throw error;
  } finally { await handle?.close(); }
}

export async function durableWrite(file, source) {
  await mkdir(path.dirname(file), { recursive: true, mode: 0o700 });
  const temporary = path.join(path.dirname(file), `.${path.basename(file)}.${process.pid}.${randomUUID()}.tmp`);
  let handle;
  try {
    handle = await open(temporary, constants.O_WRONLY | constants.O_CREAT | constants.O_EXCL | constants.O_NOFOLLOW, 0o600);
    await handle.writeFile(source);
    await handle.sync();
    observeRecoveryIo({ kind: "file_fsync", file });
    await handle.close(); handle = undefined;
    await rename(temporary, file);
    observeRecoveryIo({ kind: "rename", file });
    await fsyncDirectory(path.dirname(file));
    observeRecoveryIo({ kind: "directory_fsync", directory: path.dirname(file), file });
  } finally { await handle?.close(); await rm(temporary, { force: true }); }
}

export const durableReplace = durableWrite;
export async function durableUnlink(file) {
  try { await unlink(file); observeRecoveryIo({ kind: "unlink", file }); await fsyncDirectory(path.dirname(file));
    observeRecoveryIo({ kind: "directory_fsync", directory: path.dirname(file), file }); }
  catch (error) { if (error.code !== "ENOENT") throw error; }
}

export function journalRecord(name, file, before, after) {
  return { name, path: file, before, after, priorSha256: before === null ? null : sha256(before), postSha256: sha256(after) };
}

export async function verifyFourRecordPostimages(records) {
  for (const record of records) if (sha256(await safeBytes(record.path)) !== record.postSha256) throw new Error("owner_recovery_postimage_mismatch");
}

function recoveryPaths(project) {
  return [
    ["teams", project.paths.teams], ["state", project.paths.state], ["setup", project.paths.setup],
    ["owner-history", project.paths.ownerHistory],
  ];
}

const journalKeys = ["schemaVersion", "operationId", "signature", "phase", "nextRecord", "expectedSetupVersion",
  "expectedStateVersion", "expectedOwnerHistoryFingerprint", "receipt", "records"];
const journalRecordKeys = ["name", "path", "priorSha256", "priorimageBase64", "postSha256", "postimageBase64"];
const manual = () => { throw new Error("owner_recovery_manual_reconciliation_required"); };
const decodeCanonicalBase64 = (source) => {
  if (typeof source !== "string") return undefined;
  const decoded = Buffer.from(source, "base64");
  return decoded.toString("base64") === source ? decoded : undefined;
};

function validateJournal(journal, project) {
  if (!exactKeys(journal, journalKeys) || journal.schemaVersion !== 1 || !validId(journal.operationId) || !hex(journal.signature, 64)
    || !Array.isArray(journal.records) || journal.records.length !== 4 || !Number.isSafeInteger(journal.expectedSetupVersion)
    || journal.expectedSetupVersion < 1 || !Number.isSafeInteger(journal.expectedStateVersion) || journal.expectedStateVersion < 0
    || !(journal.expectedOwnerHistoryFingerprint === null || hex(journal.expectedOwnerHistoryFingerprint, 64))
    || !validateOwnerRecoveryReceipt(journal.receipt) || journal.receipt.operationId !== journal.operationId
    || !Number.isInteger(journal.nextRecord) || journal.nextRecord < 0 || journal.nextRecord > 4) manual();
  const expectedPhase = journal.nextRecord === 0 ? "prepared" : journal.nextRecord === 4 && journal.phase === "committed"
    ? "committed" : `renamed:${journal.nextRecord}`;
  if (journal.phase !== expectedPhase) manual();
  const expected = recoveryPaths(project);
  for (let index = 0; index < expected.length; index += 1) {
    const record = journal.records[index];
    const prior = record?.priorimageBase64 === null ? null : decodeCanonicalBase64(record?.priorimageBase64);
    const post = decodeCanonicalBase64(record?.postimageBase64);
    if (!exactKeys(record, journalRecordKeys) || record.name !== expected[index][0] || record.path !== expected[index][1]
      || !(record.priorSha256 === null || hex(record.priorSha256, 64)) || !hex(record.postSha256, 64)
      || post === undefined || sha256(post) !== record.postSha256
      || (prior === null) !== (record.priorSha256 === null) || prior !== null && sha256(prior) !== record.priorSha256
      || record.priorSha256 === null && (record.name !== "owner-history" || journal.expectedOwnerHistoryFingerprint !== null)
      || record.name === "owner-history" && record.priorSha256 !== journal.expectedOwnerHistoryFingerprint) manual();
  }
  return journal;
}

async function readJournal(project) {
  const source = await safeBytes(project.paths.ownerRecoveryJournal, { absent: true });
  if (source === null) return undefined;
  try { return validateJournal(JSON.parse(source), project); }
  catch { return manual(); }
}

function parseJsonBytes(source) {
  try { return JSON.parse(source); } catch { return manual(); }
}

const recordObject = (value) => value !== null && typeof value === "object" && !Array.isArray(value);

function without(value, keys) {
  const copy = structuredClone(value);
  for (const key of keys) delete copy[key];
  return copy;
}

function semanticJournalPostimages(journal, project) {
  const records = Object.fromEntries(journal.records.map((record) => [record.name, {
    prior: record.priorimageBase64 === null ? null : Buffer.from(record.priorimageBase64, "base64"),
    post: Buffer.from(record.postimageBase64, "base64"),
  }]));
  const priorState = parseJsonBytes(records.state.prior); const postState = parseJsonBytes(records.state.post);
  const priorSetup = parseJsonBytes(records.setup.prior); const postSetup = parseJsonBytes(records.setup.post);
  const priorTeams = records.teams.prior?.toString("utf8"); const postTeams = records.teams.post.toString("utf8");
  const priorIdentity = parseTeams(priorTeams); const postIdentity = parseTeams(postTeams);
  const priorHistory = records["owner-history"].prior === null ? undefined : parseJsonBytes(records["owner-history"].prior);
  const postHistory = parseJsonBytes(records["owner-history"].post);
  const receipt = journal.receipt;
  if (![priorState, postState, priorSetup, postSetup, priorHistory, postHistory,
    priorState?.ownership, priorState?.ownership?.current, priorState?.integration, priorState?.release,
    postState?.ownership, postState?.ownership?.current, postState?.integration, postState?.release,
    priorSetup?.ownership, priorSetup?.ownership?.current, postSetup?.ownership, postSetup?.ownership?.current,
    priorHistory?.ownership, postHistory?.ownership].every(recordObject)
    || !Array.isArray(priorHistory.entries) || !priorHistory.entries.every(recordObject)
    || !Array.isArray(postHistory.entries) || !postHistory.entries.every(recordObject)) manual();
  if (!priorState || !priorSetup || !priorTeams || priorIdentity.projectId !== project.projectId
    || postIdentity.projectId !== priorIdentity.projectId || postIdentity.owner !== receipt.newOwner.sessionId
    || postIdentity.ownerHost !== receipt.newOwner.host || postIdentity.integrationOwner !== receipt.newOwner.sessionId
    || postIdentity.integrationOwnerHost !== receipt.newOwner.host || priorIdentity.owner !== receipt.oldOwner.sessionId
    || priorIdentity.ownerHost !== receipt.oldOwner.host || postState.stateVersion !== journal.expectedStateVersion + 1
    || priorState.stateVersion !== journal.expectedStateVersion || postSetup.version !== journal.expectedSetupVersion + 1
    || priorSetup.version !== journal.expectedSetupVersion || !validateQualifiedOwnership(postState.ownership)
    || stable(postState.ownership) !== stable(postSetup.ownership) || postState.ownership.epoch !== receipt.ownershipEpoch
    || stable({ host: postState.ownership.current.host, sessionId: postState.ownership.current.sessionId }) !== stable(receipt.newOwner)
    || postState.ownership.current.operationId !== journal.operationId || postState.ownership.current.since !== receipt.appliedAt
    || stable(postState.integration && { ownerSessionId: postState.integration.ownerSessionId, ownerHost: postState.integration.ownerHost,
      ownershipEpoch: postState.integration.ownershipEpoch }) !== stable({ ownerSessionId: receipt.newOwner.sessionId,
      ownerHost: receipt.newOwner.host, ownershipEpoch: receipt.ownershipEpoch })
    || stable(postState.release && { ownerSessionId: postState.release.ownerSessionId, ownerHost: postState.release.ownerHost,
      ownershipEpoch: postState.release.ownershipEpoch }) !== stable({ ownerSessionId: receipt.newOwner.sessionId,
      ownerHost: receipt.newOwner.host, ownershipEpoch: receipt.ownershipEpoch })
    || stable(without(postState, ["stateVersion", "ownership", "integration", "release"])) !== stable(without(priorState, ["stateVersion", "ownership", "integration", "release"]))
    || stable(without(postState.integration, ["ownerSessionId", "ownerHost", "ownershipEpoch"])) !== stable(without(priorState.integration, ["ownerSessionId", "ownerHost", "ownershipEpoch"]))
    || stable(without(postState.release, ["ownerSessionId", "ownerHost", "ownershipEpoch"])) !== stable(without(priorState.release, ["ownerSessionId", "ownerHost", "ownershipEpoch"]))
    || stable(without(postSetup, ["version", "ownership"])) !== stable(without(priorSetup, ["version", "ownership"]))
    || !validateOwnerHistory(postHistory, postState.ownership)) manual();
  const expectedTeams = replaceLabel(replaceLabel(replaceLabel(replaceLabel(priorTeams, "Project owner", receipt.newOwner.sessionId),
    "Project owner host", receipt.newOwner.host), "Integration owner", receipt.newOwner.sessionId), "Integration owner host", receipt.newOwner.host);
  if (postTeams !== expectedTeams) manual();
  const oldEpoch = receipt.ownershipEpoch - 1;
  if (!validateQualifiedOwnership(priorState.ownership) || stable(priorState.ownership) !== stable(priorSetup.ownership)
    || priorState.ownership.epoch !== oldEpoch || journal.expectedOwnerHistoryFingerprint === null || !priorHistory
    || stable({ host: priorState.ownership.current.host, sessionId: priorState.ownership.current.sessionId }) !== stable(receipt.oldOwner)
    || priorIdentity.integrationOwner !== receipt.oldOwner.sessionId || priorIdentity.integrationOwnerHost !== receipt.oldOwner.host
    || stable(priorState.integration && { ownerSessionId: priorState.integration.ownerSessionId, ownerHost: priorState.integration.ownerHost,
      ownershipEpoch: priorState.integration.ownershipEpoch }) !== stable({ ownerSessionId: receipt.oldOwner.sessionId,
      ownerHost: receipt.oldOwner.host, ownershipEpoch: oldEpoch })
    || stable(priorState.release && { ownerSessionId: priorState.release.ownerSessionId, ownerHost: priorState.release.ownerHost,
      ownershipEpoch: priorState.release.ownershipEpoch }) !== stable({ ownerSessionId: receipt.oldOwner.sessionId,
      ownerHost: receipt.oldOwner.host, ownershipEpoch: oldEpoch })
    || !validateOwnerHistory(priorHistory, priorState.ownership) || postHistory.version !== priorHistory.version + 1
    || postHistory.entries.length !== priorHistory.entries.length + 1
    || stable(postHistory.entries.slice(0, -1)) !== stable(priorHistory.entries)) manual();
  const entry = postHistory.entries.at(-1);
  if (entry.operationId !== journal.operationId || entry.signature !== journal.signature || stable(entry.receipt) !== stable(receipt)
    || entry.epoch !== oldEpoch || stable(entry.oldOwner) !== stable(receipt.oldOwner) || stable(entry.newOwner) !== stable(receipt.newOwner)
    || entry.priorStateFingerprint !== journal.records[1].priorSha256 || entry.priorTeamsFingerprint !== journal.records[0].priorSha256
    || entry.priorSetupFingerprint !== journal.records[2].priorSha256
    || entry.priorOwnerHistoryFingerprint !== journal.expectedOwnerHistoryFingerprint
    || stable(entry.writer) !== stable(postState.ownership.current.writer)) manual();
  return records;
}

async function rollForward(project, journal, options = {}) {
  validateJournal(journal, project);
  semanticJournalPostimages(journal, project);
  for (let index = 0; index < journal.records.length; index += 1) {
    const record = journal.records[index];
    const current = await safeBytes(record.path, { absent: true });
    const currentHash = current === null ? null : sha256(current);
    if (index < journal.nextRecord && currentHash !== record.postSha256) manual();
    if (index > journal.nextRecord && currentHash !== record.priorSha256) manual();
    if (currentHash === record.priorSha256) await durableReplace(record.path, Buffer.from(record.postimageBase64, "base64"));
    else if (currentHash !== record.postSha256) manual();
    if (index < journal.nextRecord) continue;
    journal = { ...journal, phase: `renamed:${index + 1}`, nextRecord: index + 1 };
    await durableWrite(project.paths.ownerRecoveryJournal, encodeJson(journal));
    if (options.failAfterRename === index + 1) throw new Error(`injected_owner_recovery_crash_${index + 1}`);
  }
  const records = journal.records.map((record) => ({ ...record, path: record.path }));
  await verifyFourRecordPostimages(records);
  semanticJournalPostimages(journal, project);
  journal = { ...journal, phase: "committed", nextRecord: 4 };
  await durableWrite(project.paths.ownerRecoveryJournal, encodeJson(journal));
  if (options.failAfterCommitted) throw new Error("injected_owner_recovery_crash_committed");
  semanticJournalPostimages(journal, project);
  await verifyFourRecordPostimages(journal.records);
  await durableUnlink(project.paths.ownerRecoveryJournal);
  if (options.failAfterUnlink) throw new Error("injected_owner_recovery_crash_unlinked");
  return journal.receipt;
}

export async function assertNoOwnerRecoveryJournal(project) {
  if (!project?.paths?.ownerRecoveryJournal) return;
  if (await safeBytes(project.paths.ownerRecoveryJournal, { absent: true }) !== null) throw new Error("owner_recovery_in_progress");
  if (await safeBytes(path.join(project.paths.stateRoot, ".legacy-owner-adoption.json"), { absent: true }) !== null) {
    throw new Error("legacy_owner_adoption_in_progress");
  }
}

/** Repair the four-record barrier before any canonical reader exposes state. */
export async function repairOwnerRecovery(project, options = {}) {
  if (!project?.paths?.ownerRecoveryJournal) return { status: "none" };
  const source = await safeBytes(project.paths.ownerRecoveryJournal, { absent: true });
  if (source === null) return { status: "none" };
  if (await safeBytes(path.join(project.paths.stateRoot, ".legacy-owner-adoption.json"), { absent: true }) !== null) {
    throw new Error("legacy_owner_adoption_manual_reconciliation_required");
  }
  return withDirectoryLock(project.paths.ownerRecoveryLock, { kind: "owner_recovery_repair", pid: process.pid }, async () =>
    withDirectoryLock(path.join(project.paths.locks, "setup.lock"), { kind: "owner_recovery_repair", pid: process.pid }, async () =>
      withDirectoryLock(path.join(project.paths.locks, "state.lock"), { kind: "owner_recovery_repair", pid: process.pid }, async () => {
        const journal = await readJournal(project);
        const receipt = await rollForward(project, journal, options);
        return { status: "repaired", receipt };
      }, { budget: options.budget }), { budget: options.budget }), { budget: options.budget });
}

function parseTeams(source) {
  const label = (name) => source.match(new RegExp(`^${name}:\\s*(.+)$`, "m"))?.[1].trim();
  return { projectId: label("Project"), owner: label("Project owner"), ownerHost: label("Project owner host"),
    integrationOwner: label("Integration owner"), integrationOwnerHost: label("Integration owner host") };
}

function replaceLabel(source, name, value) {
  const expression = new RegExp(`^${name}:.*$`, "m");
  if (!expression.test(source)) throw new Error("owner_recovery_record_invalid");
  return source.replace(expression, `${name}: ${value}`);
}

/** Closed transaction. Positive authority can only originate from this module's private capability registry. */
export async function recoverProjectOwner(project, envelope, options = {}) {
  if (validateOwnerRecoveryEnvelope(envelope)) return refusal("invalid_request");
  const native = options.nativeOwnerRecovery;
  if (!await validateNativeRecoveryContext(native, project, envelope.request.expectedOwnershipEpoch, options.nowMs?.() ?? Date.now())) {
    return { status: "validated", ready: false, reason: "native_owner_recovery_required" };
  }
  const request = envelope.request;
  return withDirectoryLock(project.paths.ownerRecoveryLock, { kind: "project_owner_recovery", operationId: request.operationId, pid: process.pid }, async () =>
    withDirectoryLock(path.join(project.paths.locks, "setup.lock"), { kind: "project_owner_recovery", operationId: request.operationId, pid: process.pid }, async () =>
      withDirectoryLock(path.join(project.paths.locks, "state.lock"), { kind: "project_owner_recovery", operationId: request.operationId, pid: process.pid }, async () => {
        const existingJournal = await readJournal(project);
        if (existingJournal) {
          if (existingJournal.operationId !== request.operationId || existingJournal.signature !== sha256(stable(request))) return refusal("operation_identity_reused");
          const receipt = await rollForward(project, existingJournal, options);
          return { status: "applied", result: receipt };
        }
        const entries = await Promise.all(recoveryPaths(project).map(async ([name, file]) => [name, file, await safeBytes(file, { absent: name === "owner-history" })]));
        const byName = Object.fromEntries(entries.map(([name, , bytes]) => [name, bytes]));
        const hashes = Object.fromEntries(entries.map(([name, , bytes]) => [name, bytes === null ? null : sha256(bytes)]));
        const requestSignature = sha256(stable(request));
        let parsedHistory;
        try { parsedHistory = byName["owner-history"] === null ? undefined : JSON.parse(byName["owner-history"]); }
        catch { return refusal("owner_history_invalid"); }
        const replay = parsedHistory?.entries?.find((entry) => entry.operationId === request.operationId);
        if (replay) return replay.signature === requestSignature
          ? { status: "duplicate", result: structuredClone(replay.receipt) }
          : refusal("operation_identity_reused");
        if (hashes.state !== request.expectedStateFingerprint || hashes.teams !== request.expectedTeamsFingerprint
          || hashes.setup !== request.expectedSetupFingerprint || hashes["owner-history"] !== request.expectedOwnerHistoryFingerprint) return refusal("stale_fingerprint");
        let state; let setup;
        try { state = JSON.parse(byName.state); setup = JSON.parse(byName.setup); }
        catch { return refusal("owner_records_invalid"); }
        const teamsText = byName.teams.toString("utf8");
        const identity = parseTeams(teamsText);
        const qualified = state.ownership !== undefined || setup.ownership !== undefined || identity.ownerHost !== undefined
          || identity.integrationOwnerHost !== undefined || state.integration?.ownerHost !== undefined || state.integration?.ownershipEpoch !== undefined
          || state.release?.ownerHost !== undefined || state.release?.ownershipEpoch !== undefined;
        if (!qualified && parsedHistory !== undefined || qualified && parsedHistory === undefined) return refusal(parsedHistory ? "owner_history_invalid" : "owner_history_missing");
        if (qualified && (!validateOwnershipWriter(state.ownership?.current?.writer)
          || !validateOwnershipWriter(setup.ownership?.current?.writer))) return refusal("owner_liveness_unknown");
        if (qualified && (!validateQualifiedOwnership(state.ownership) || stable(state.ownership) !== stable(setup.ownership)
          || !validateOwnerHistory(parsedHistory, state.ownership))) return refusal("owner_history_invalid");
        if (!qualified) return refusal("owner_liveness_unknown");
        const history = parsedHistory;
        const epoch = state.ownership.epoch;
        if (identity.projectId !== request.projectId || project.projectId !== request.projectId || identity.owner !== request.expectedOwnerSessionId
          || epoch !== request.expectedOwnershipEpoch || setup.version !== request.expectedSetupVersion || state.stateVersion !== request.expectedStateVersion
          || history.ownership?.epoch !== epoch) return refusal("stale_owner_or_version");
        const oldOwner = { host: identity.ownerHost, sessionId: identity.owner };
        const newOwner = { host: native.identity.host, sessionId: native.identity.sessionId };
        if (stable(oldOwner) === stable(newOwner)) return refusal("owner_identity_unchanged");
        const liveness = await native.inspectSession({ ...oldOwner, writer: structuredClone(state.ownership.current.writer) });
        if (!liveness || typeof liveness !== "object" || !["active", "stopped", "unknown"].includes(liveness.status)) return refusal("owner_liveness_unknown");
        if (liveness.status === "active") return refusal("owner_active");
        if (!validStoppedEvidence(liveness)) return refusal("owner_liveness_unknown");
        const approval = await native.confirm({ projectId: request.projectId, oldOwner: { host: identity.ownerHost, sessionId: identity.owner },
          newOwner, expectedSetupVersion: request.expectedSetupVersion,
          expectedStateVersion: request.expectedStateVersion, fingerprints: hashes, pendingOperations: state.pendingOperations ?? {}, holds: {
            integration: state.integration?.hold === true, release: state.release?.hold === true } });
        if (!exactKeys(approval, approval?.approved === true ? ["approved", "approvalId"] : ["approved"])
          || approval.approved !== true || !validId(approval.approvalId)) return refusal("owner_recovery_cancelled");
        const now = (options.now ?? (() => new Date().toISOString()))();
        if (!timestamp(now)) return refusal("owner_recovery_time_invalid");
        const nextEpoch = epoch + 1;
        const writer = { kind: "host-bootstrap", host: native.identity.host, invocationId: native.identity.invocationId, approvalId: approval.approvalId };
        const nextState = structuredClone(state);
        nextState.stateVersion += 1;
        nextState.ownership = { epoch: nextEpoch, current: { ...newOwner, since: now, operationId: request.operationId, writer } };
        nextState.integration = { ...nextState.integration, ownerSessionId: newOwner.sessionId, ownerHost: newOwner.host, ownershipEpoch: nextEpoch };
        nextState.release = { ...nextState.release, ownerSessionId: newOwner.sessionId, ownerHost: newOwner.host, ownershipEpoch: nextEpoch };
        const nextSetup = { ...setup, version: setup.version + 1, ownership: structuredClone(nextState.ownership) };
        let nextTeams = replaceLabel(teamsText, "Project owner", newOwner.sessionId);
        nextTeams = replaceLabel(nextTeams, "Project owner host", newOwner.host);
        nextTeams = replaceLabel(nextTeams, "Integration owner", newOwner.sessionId);
        nextTeams = replaceLabel(nextTeams, "Integration owner host", newOwner.host);
        const receipt = { operationId: request.operationId, ownershipEpoch: nextEpoch, oldOwner: { host: identity.ownerHost, sessionId: identity.owner }, newOwner, appliedAt: now };
        const entry = { operationId: request.operationId, signature: requestSignature, receipt, epoch,
          oldOwner: { host: identity.ownerHost, sessionId: identity.owner }, newOwner, reason: request.reason,
          approvalId: approval.approvalId, writer: structuredClone(writer), livenessEvidence: structuredClone(liveness),
          priorStateFingerprint: hashes.state, priorTeamsFingerprint: hashes.teams, priorSetupFingerprint: hashes.setup,
          priorOwnerHistoryFingerprint: hashes["owner-history"], appliedAt: now };
        const nextHistory = { schemaVersion: 1, version: history.version + 1, ownership: { epoch: nextEpoch }, entries: [...history.entries, entry] };
        const nextBytes = { teams: Buffer.from(nextTeams), state: encodeJson(nextState), setup: encodeJson(nextSetup), "owner-history": encodeJson(nextHistory) };
        const records = entries.map(([name, file, before]) => journalRecord(name, file, before, nextBytes[name]));
        const journal = { schemaVersion: 1, operationId: request.operationId, signature: requestSignature, phase: "prepared", nextRecord: 0,
          expectedSetupVersion: request.expectedSetupVersion, expectedStateVersion: request.expectedStateVersion,
          expectedOwnerHistoryFingerprint: request.expectedOwnerHistoryFingerprint, receipt,
          records: records.map((record) => ({ name: record.name, path: record.path, priorSha256: record.priorSha256,
            priorimageBase64: record.before === null ? null : record.before.toString("base64"), postSha256: record.postSha256,
            postimageBase64: record.after.toString("base64") })) };
        if (options.failBeforeJournal) throw new Error("injected_owner_recovery_crash_before_prepared");
        await durableWrite(project.paths.ownerRecoveryJournal, encodeJson(journal));
        if (options.failAfterJournal === true) throw new Error("injected_owner_recovery_crash_prepared");
        const result = await rollForward(project, journal, options);
        return { status: "applied", result };
      }, { budget: options.budget }), { budget: options.budget }), { budget: options.budget });
}

const legacyAdoptionRequestKeys = ["operationId", "projectId", "expectedLegacyOwnerSessionId", "expectedProjectRoot",
  "expectedRevision", "expectedTracker", "expectedSetupVersion", "expectedStateVersion", "expectedTeamsFingerprint",
  "expectedSetupFingerprint", "expectedStateFingerprint", "expectedOwnerHistoryFingerprint", "authorization", "reason"];
const adoptionAuthorizationKeys = ["status", "scope", "source", "approvalId", "grantedAt", "projectId", "revision", "trackerFingerprint", "host"];

/** Validate the one-time migration request without accepting any asserted actor identity. */
export function validateLegacyOwnerAdoptionEnvelope(envelope) {
  if (!exactKeys(envelope, ["schemaVersion", "request"]) || envelope.schemaVersion !== 1
    || Buffer.byteLength(JSON.stringify(envelope)) > MAX_REQUEST_BYTES || !exactKeys(envelope.request, legacyAdoptionRequestKeys)) return "invalid_request";
  const request = envelope.request;
  const tracker = request.expectedTracker;
  const authorization = request.authorization;
  if (!validId(request.operationId) || !validId(request.projectId) || request.expectedLegacyOwnerSessionId !== "root"
    || typeof request.expectedProjectRoot !== "string" || !path.isAbsolute(request.expectedProjectRoot)
    || path.normalize(request.expectedProjectRoot) !== request.expectedProjectRoot || !hex(request.expectedRevision, 40)
    || !exactKeys(tracker, ["kind", "path", "fingerprint"]) || !["markdown", "beads"].includes(tracker.kind)
    || typeof tracker.path !== "string" || !path.isAbsolute(tracker.path) || path.normalize(tracker.path) !== tracker.path || !hex(tracker.fingerprint, 64)
    || !Number.isSafeInteger(request.expectedSetupVersion) || request.expectedSetupVersion < 1
    || !Number.isSafeInteger(request.expectedStateVersion) || request.expectedStateVersion < 0
    || ![request.expectedTeamsFingerprint, request.expectedSetupFingerprint, request.expectedStateFingerprint].every((value) => hex(value, 64))
    || request.expectedOwnerHistoryFingerprint !== null || !exactKeys(authorization, adoptionAuthorizationKeys)
    || authorization.status !== "approved" || authorization.scope !== "legacy_owner_adoption"
    || typeof authorization.source !== "string" || !authorization.source.trim() || authorization.source.length > 256
    || !validId(authorization.approvalId) || !timestamp(authorization.grantedAt) || authorization.projectId !== request.projectId
    || authorization.revision !== request.expectedRevision || authorization.trackerFingerprint !== tracker.fingerprint
    || !runtimes.has(authorization.host) || typeof request.reason !== "string" || request.reason.trim() !== request.reason
    || !request.reason || Buffer.byteLength(request.reason) > 4096) return "invalid_request";
  return undefined;
}

const adoptionReceiptKeys = ["schemaVersion", "operationId", "signature", "projectId", "expectedLegacyOwnerSessionId",
  "ownershipEpoch", "newOwner", "appliedAt", "authorization", "prior", "nativeEvidence", "reason"];
const priorKeys = ["stateFingerprint", "teamsFingerprint", "setupFingerprint", "ownerHistoryFingerprint"];
const nativeEvidenceKeys = ["host", "sessionId", "cwd", "invocationId", "writer"];

function validateLegacyOwnerAdoptionReceipt(receipt) {
  return exactKeys(receipt, adoptionReceiptKeys) && receipt.schemaVersion === 1 && validId(receipt.operationId) && hex(receipt.signature, 64)
    && validId(receipt.projectId) && receipt.expectedLegacyOwnerSessionId === "root" && receipt.ownershipEpoch === 1
    && qualifiedIdentity(receipt.newOwner) && timestamp(receipt.appliedAt) && exactKeys(receipt.authorization, adoptionAuthorizationKeys)
    && receipt.authorization.status === "approved" && receipt.authorization.scope === "legacy_owner_adoption"
    && typeof receipt.authorization.source === "string" && receipt.authorization.source.trim() && receipt.authorization.source.length <= 256
    && validId(receipt.authorization.approvalId) && timestamp(receipt.authorization.grantedAt)
    && receipt.authorization.projectId === receipt.projectId && hex(receipt.authorization.revision, 40)
    && hex(receipt.authorization.trackerFingerprint, 64) && receipt.authorization.host === receipt.newOwner.host
    && exactKeys(receipt.prior, priorKeys) && hex(receipt.prior.stateFingerprint, 64) && hex(receipt.prior.teamsFingerprint, 64)
    && hex(receipt.prior.setupFingerprint, 64) && receipt.prior.ownerHistoryFingerprint === null
    && exactKeys(receipt.nativeEvidence, nativeEvidenceKeys) && receipt.nativeEvidence.host === receipt.newOwner.host
    && receipt.nativeEvidence.sessionId === receipt.newOwner.sessionId && typeof receipt.nativeEvidence.cwd === "string"
    && validId(receipt.nativeEvidence.invocationId) && validateOwnershipWriter(receipt.nativeEvidence.writer)
    && typeof receipt.reason === "string" && receipt.reason.length > 0 && receipt.reason.length <= 4096;
}

const adoptionPaths = (project) => ({
  journal: path.join(project.paths.stateRoot, ".legacy-owner-adoption.json"),
  lock: path.join(project.paths.locks, "legacy-owner-adoption.lock"),
  receipt: path.join(project.paths.stateRoot, "legacy-owner-adoption.json"),
});

/** Bare CLI path: validate intent and report a sealed replay without acquiring mutation authority. */
export async function inspectLegacyOwnerAdoption(project, envelope) {
  if (validateLegacyOwnerAdoptionEnvelope(envelope)) return refusal("invalid_request");
  const request = envelope.request;
  const source = await safeBytes(adoptionPaths(project).receipt, { absent: true });
  if (source === null) return { status: "validated", ready: false, reason: "native_legacy_owner_adoption_required" };
  let receipt;
  try { receipt = JSON.parse(source); } catch { return refusal("legacy_owner_adoption_manual_reconciliation_required"); }
  if (!validateLegacyOwnerAdoptionReceipt(receipt)) return refusal("legacy_owner_adoption_manual_reconciliation_required");
  if (receipt.operationId !== request.operationId) return refusal("legacy_owner_adoption_already_completed");
  return receipt.signature === sha256(stable(request)) ? { status: "duplicate", result: receipt } : refusal("operation_identity_reused");
}

async function captureAdoptionWriter() {
  const [stat, bootId, pidNamespace] = await Promise.all([
    readFile(`/proc/${process.pid}/stat`, "utf8"), readFile("/proc/sys/kernel/random/boot_id", "utf8"), readlink("/proc/self/ns/pid"),
  ]);
  const startTime = stat.slice(stat.lastIndexOf(")") + 2).split(/\s+/)[19];
  const writer = { pid: process.pid, startTime, bootId: bootId.trim(), host: os.hostname(), pidNamespace };
  if (!validateOwnershipWriter(writer)) throw new Error("legacy_owner_adoption_writer_unavailable");
  return writer;
}

async function adoptionGitIdentity(location) {
  const identity = await gitIdentity(location);
  const git = async (...args) => (await run("git", ["-C", identity.nativeCwd, ...args], { encoding: "utf8", timeout: 1000, maxBuffer: 16384 })).stdout.trim();
  return { ...identity, revision: await git("rev-parse", "HEAD"), branch: await git("symbolic-ref", "--short", "HEAD"),
    clean: await git("status", "--porcelain", "--untracked-files=no") === "" };
}

function addLegacyHostLabels(source, host) {
  if (/^Project owner host:/m.test(source) || /^Integration owner host:/m.test(source)) throw new Error("legacy_owner_shape_invalid");
  let next = source.replace(/^Project owner: root$/m, `Project owner: root\nProject owner host: ${host}`);
  next = next.replace(/^Integration owner: root$/m, `Integration owner: root\nIntegration owner host: ${host}`);
  if (next === source || !/^Project owner host:/m.test(next) || !/^Integration owner host:/m.test(next)) throw new Error("legacy_owner_shape_invalid");
  return next;
}

function exactLegacyTeamsShape(source, projectId) {
  const values = (label) => source.match(new RegExp(`^${label}:\\s*([^\\r\\n]+)$`, "gm")) ?? [];
  const exact = (label, expected) => {
    const matches = values(label);
    return matches.length === 1 && matches[0].slice(label.length + 1).trim() === expected;
  };
  return exact("Project", projectId) && exact("Project owner", "root") && exact("Integration owner", "root")
    && values("Project owner host").length === 0 && values("Integration owner host").length === 0;
}

function validateAdoptionJournal(journal, project) {
  const paths = adoptionPaths(project);
  const names = ["teams", "state", "setup", "owner-history", "receipt"];
  const files = [project.paths.teams, project.paths.state, project.paths.setup, project.paths.ownerHistory, paths.receipt];
  if (!exactKeys(journal, ["schemaVersion", "operationId", "signature", "phase", "nextRecord", "receipt", "records"])
    || journal.schemaVersion !== 1 || !validId(journal.operationId) || !hex(journal.signature, 64)
    || !validateLegacyOwnerAdoptionReceipt(journal.receipt) || journal.receipt.operationId !== journal.operationId
    || journal.receipt.signature !== journal.signature
    || !Array.isArray(journal.records) || journal.records.length !== names.length || !Number.isInteger(journal.nextRecord)
    || journal.nextRecord < 0 || journal.nextRecord > names.length) throw new Error("legacy_owner_adoption_manual_reconciliation_required");
  const expectedPhase = journal.nextRecord === 0 ? "prepared" : journal.nextRecord === names.length && journal.phase === "committed"
    ? "committed" : `renamed:${journal.nextRecord}`;
  if (journal.phase !== expectedPhase) throw new Error("legacy_owner_adoption_manual_reconciliation_required");
  for (let index = 0; index < names.length; index += 1) {
    const record = journal.records[index];
    const prior = record?.priorimageBase64 === null ? null : decodeCanonicalBase64(record?.priorimageBase64);
    const post = decodeCanonicalBase64(record?.postimageBase64);
    if (!exactKeys(record, journalRecordKeys) || record.name !== names[index] || record.path !== files[index]
      || !(record.priorSha256 === null || hex(record.priorSha256, 64)) || !hex(record.postSha256, 64)
      || post === undefined || sha256(post) !== record.postSha256 || (prior === null) !== (record.priorSha256 === null)
      || prior !== null && sha256(prior) !== record.priorSha256) throw new Error("legacy_owner_adoption_manual_reconciliation_required");
  }
  return journal;
}

function semanticAdoptionJournal(journal, project) {
  const records = Object.fromEntries(journal.records.map((record) => [record.name, {
    prior: record.priorimageBase64 === null ? null : Buffer.from(record.priorimageBase64, "base64"),
    post: Buffer.from(record.postimageBase64, "base64"),
  }]));
  let priorState; let postState; let priorSetup; let postSetup; let postHistory; let postReceipt;
  try {
    priorState = JSON.parse(records.state.prior); postState = JSON.parse(records.state.post);
    priorSetup = JSON.parse(records.setup.prior); postSetup = JSON.parse(records.setup.post);
    postHistory = JSON.parse(records["owner-history"].post); postReceipt = JSON.parse(records.receipt.post);
  } catch { throw new Error("legacy_owner_adoption_manual_reconciliation_required"); }
  const priorTeams = records.teams.prior?.toString("utf8");
  const postTeams = records.teams.post.toString("utf8");
  const receipt = journal.receipt;
  const identity = parseTeams(priorTeams);
  const owner = receipt.newOwner;
  const expectedOwnership = { epoch: 1, current: { ...owner, since: receipt.appliedAt, operationId: receipt.operationId,
    writer: receipt.nativeEvidence.writer } };
  const stripGate = (gate) => without(gate, ["ownerSessionId", "ownerHost", "ownershipEpoch", "authorized", "hold"]);
  if (!priorState || !postState || !priorSetup || !postSetup || !priorTeams
    || records["owner-history"].prior !== null || records.receipt.prior !== null
    || journal.records.find(({ name }) => name === "state").priorSha256 !== receipt.prior.stateFingerprint
    || journal.records.find(({ name }) => name === "teams").priorSha256 !== receipt.prior.teamsFingerprint
    || journal.records.find(({ name }) => name === "setup").priorSha256 !== receipt.prior.setupFingerprint
    || receipt.prior.ownerHistoryFingerprint !== null || stable(postReceipt) !== stable(receipt)
    || !exactLegacyTeamsShape(priorTeams, receipt.projectId)
    || identity.projectId !== receipt.projectId || identity.owner !== "root" || identity.integrationOwner !== "root"
    || identity.ownerHost !== undefined || identity.integrationOwnerHost !== undefined
    || priorState.ownership !== undefined || priorSetup.ownership !== undefined
    || priorState.integration?.ownerSessionId !== "root" || priorState.release?.ownerSessionId !== "root"
    || priorState.integration?.ownerHost !== undefined || priorState.integration?.ownershipEpoch !== undefined
    || priorState.release?.ownerHost !== undefined || priorState.release?.ownershipEpoch !== undefined
    || priorState.pendingOperations && Object.keys(priorState.pendingOperations).length
    || postState.stateVersion !== priorState.stateVersion + 1 || postSetup.version !== priorSetup.version + 1
    || stable(postState.ownership) !== stable(expectedOwnership) || stable(postSetup.ownership) !== stable(expectedOwnership)
    || stable(without(postState, ["stateVersion", "ownership", "integration", "release"])) !== stable(without(priorState, ["stateVersion", "ownership", "integration", "release"]))
    || stable(without(postSetup, ["version", "ownership"])) !== stable(without(priorSetup, ["version", "ownership"]))
    || stable(stripGate(postState.integration)) !== stable(stripGate(priorState.integration))
    || stable(stripGate(postState.release)) !== stable(stripGate(priorState.release))
    || postState.integration.ownerSessionId !== owner.sessionId || postState.integration.ownerHost !== owner.host
    || postState.integration.ownershipEpoch !== 1 || postState.integration.authorized !== false || postState.integration.hold !== true
    || postState.release.ownerSessionId !== owner.sessionId || postState.release.ownerHost !== owner.host
    || postState.release.ownershipEpoch !== 1 || postState.release.authorized !== false || postState.release.hold !== true
    || stable(postHistory) !== stable({ schemaVersion: 1, version: 1, ownership: { epoch: 1 }, entries: [] })) {
    throw new Error("legacy_owner_adoption_manual_reconciliation_required");
  }
  let expectedTeams;
  try {
    expectedTeams = addLegacyHostLabels(priorTeams, owner.host);
    expectedTeams = replaceLabel(expectedTeams, "Project owner", owner.sessionId);
    expectedTeams = replaceLabel(expectedTeams, "Integration owner", owner.sessionId);
  } catch { throw new Error("legacy_owner_adoption_manual_reconciliation_required"); }
  if (postTeams !== expectedTeams || receipt.nativeEvidence.cwd !== project.root) {
    throw new Error("legacy_owner_adoption_manual_reconciliation_required");
  }
  return records;
}

async function readAdoptionJournal(project) {
  const source = await safeBytes(adoptionPaths(project).journal, { absent: true });
  if (source === null) return undefined;
  try {
    const journal = validateAdoptionJournal(JSON.parse(source), project);
    semanticAdoptionJournal(journal, project);
    return journal;
  }
  catch { throw new Error("legacy_owner_adoption_manual_reconciliation_required"); }
}

async function rollForwardAdoption(project, journal, options = {}) {
  validateAdoptionJournal(journal, project);
  semanticAdoptionJournal(journal, project);
  const journalPath = adoptionPaths(project).journal;
  for (let index = 0; index < journal.records.length; index += 1) {
    const record = journal.records[index];
    const current = await safeBytes(record.path, { absent: true });
    const currentHash = current === null ? null : sha256(current);
    if (index < journal.nextRecord && currentHash !== record.postSha256) throw new Error("legacy_owner_adoption_manual_reconciliation_required");
    if (index > journal.nextRecord && currentHash !== record.priorSha256) throw new Error("legacy_owner_adoption_manual_reconciliation_required");
    if (currentHash === record.priorSha256) await durableReplace(record.path, Buffer.from(record.postimageBase64, "base64"));
    else if (currentHash !== record.postSha256) throw new Error("legacy_owner_adoption_manual_reconciliation_required");
    if (index < journal.nextRecord) continue;
    journal = { ...journal, phase: `renamed:${index + 1}`, nextRecord: index + 1 };
    await durableWrite(journalPath, encodeJson(journal));
    if (options.failAfterRename === index + 1) throw new Error(`injected_legacy_owner_adoption_crash_${index + 1}`);
  }
  for (const record of journal.records) if (sha256(await safeBytes(record.path)) !== record.postSha256) {
    throw new Error("legacy_owner_adoption_manual_reconciliation_required");
  }
  journal = { ...journal, phase: "committed", nextRecord: journal.records.length };
  await durableWrite(journalPath, encodeJson(journal));
  if (options.failAfterCommitted) throw new Error("injected_legacy_owner_adoption_crash_committed");
  await durableUnlink(journalPath);
  return journal.receipt;
}

/** One-time migration for the historically known synthetic `root` owner. */
export async function adoptLegacyProjectOwner(project, envelope, context = {}, options = {}) {
  if (validateLegacyOwnerAdoptionEnvelope(envelope)) return refusal("invalid_request");
  const request = envelope.request;
  const native = context.nativeIdentity;
  const nativeHost = native?.host === "claude" ? "claude-code" : native?.host;
  if (native?.observed !== true || !runtimes.has(nativeHost) || !validId(native.sessionId) || native.sessionId === "root"
    || !validId(native.invocationId) || typeof native.cwd !== "string" || request.authorization.host !== nativeHost) {
    return { status: "validated", ready: false, reason: "native_legacy_owner_adoption_required" };
  }
  let git;
  try { git = await adoptionGitIdentity(native.cwd); }
  catch { return refusal("native_project_cwd_mismatch"); }
  if (project.root !== project.worktreeRoot || project.root !== git.canonicalTop || git.nativeTop !== git.canonicalTop
    || project.commonDirectory !== git.commonDirectory || git.nativeCwd !== project.root || request.expectedProjectRoot !== project.root
    || git.branch !== "main") return refusal("native_project_cwd_mismatch");
  if (!git.clean) return refusal("dirty_revision");
  const writer = await captureAdoptionWriter();
  const paths = adoptionPaths(project);
  return withDirectoryLock(project.paths.ownerRecoveryLock, { kind: "legacy_owner_adoption", operationId: request.operationId, pid: process.pid }, async () =>
    withDirectoryLock(paths.lock, { kind: "legacy_owner_adoption", operationId: request.operationId, pid: process.pid }, async () =>
      withDirectoryLock(path.join(project.paths.locks, "setup.lock"), { kind: "legacy_owner_adoption", operationId: request.operationId, pid: process.pid }, async () =>
        withDirectoryLock(path.join(project.paths.locks, "state.lock"), { kind: "legacy_owner_adoption", operationId: request.operationId, pid: process.pid }, async () => {
          const existingJournal = await readAdoptionJournal(project);
          if (existingJournal && await safeBytes(project.paths.ownerRecoveryJournal, { absent: true }) !== null) {
            throw new Error("legacy_owner_adoption_manual_reconciliation_required");
          }
          if (existingJournal) {
            if (existingJournal.operationId !== request.operationId || existingJournal.signature !== sha256(stable(request))) return refusal("operation_identity_reused");
            return { status: "applied", result: await rollForwardAdoption(project, existingJournal, options) };
          }
          if (await safeBytes(project.paths.ownerRecoveryJournal, { absent: true }) !== null) return refusal("owner_recovery_in_progress");
          const requestSignature = sha256(stable(request));
          const existingReceiptBytes = await safeBytes(paths.receipt, { absent: true });
          if (existingReceiptBytes !== null) {
            let receipt;
            try { receipt = JSON.parse(existingReceiptBytes); } catch { throw new Error("legacy_owner_adoption_manual_reconciliation_required"); }
            if (!validateLegacyOwnerAdoptionReceipt(receipt)) throw new Error("legacy_owner_adoption_manual_reconciliation_required");
            if (receipt.operationId === request.operationId) return receipt.signature === requestSignature
              ? { status: "duplicate", result: receipt } : refusal("operation_identity_reused");
            return refusal("legacy_owner_adoption_already_completed");
          }
          const entries = await Promise.all([
            ["teams", project.paths.teams], ["state", project.paths.state], ["setup", project.paths.setup],
            ["owner-history", project.paths.ownerHistory], ["receipt", paths.receipt],
          ].map(async ([name, file]) => [name, file, await safeBytes(file, { absent: ["owner-history", "receipt"].includes(name) })]));
          const byName = Object.fromEntries(entries.map(([name, , bytes]) => [name, bytes]));
          const hashes = Object.fromEntries(entries.map(([name, , bytes]) => [name, bytes === null ? null : sha256(bytes)]));
          if (hashes.state !== request.expectedStateFingerprint || hashes.teams !== request.expectedTeamsFingerprint
            || hashes.setup !== request.expectedSetupFingerprint || hashes["owner-history"] !== null) return refusal("stale_fingerprint");
          let state; let setup;
          try { state = JSON.parse(byName.state); setup = JSON.parse(byName.setup); } catch { return refusal("owner_records_invalid"); }
          const identity = parseTeams(byName.teams.toString("utf8"));
          const tracker = await readTracker(project, options);
          if (tracker.tracker.status !== "current" || tracker.tracker.kind !== request.expectedTracker.kind
            || tracker.tracker.path !== request.expectedTracker.path || tracker.tracker.fingerprint !== request.expectedTracker.fingerprint) return refusal("stale_tracker");
          const refreshedGit = await adoptionGitIdentity(native.cwd);
          if (refreshedGit.revision !== request.expectedRevision || refreshedGit.branch !== "main" || !refreshedGit.clean || refreshedGit.canonicalTop !== project.root
            || identity.projectId !== request.projectId || project.projectId !== request.projectId
            || setup.version !== request.expectedSetupVersion || state.stateVersion !== request.expectedStateVersion) return refusal("stale_owner_or_version");
          const gateShape = (gate) => gate && gate.ownerSessionId === "root" && gate.ownerHost === undefined && gate.ownershipEpoch === undefined;
          if (!exactLegacyTeamsShape(byName.teams.toString("utf8"), request.projectId)
            || identity.owner !== "root" || identity.integrationOwner !== "root" || identity.ownerHost !== undefined || identity.integrationOwnerHost !== undefined
            || state.ownership !== undefined || setup.ownership !== undefined || !gateShape(state.integration) || !gateShape(state.release)
            || byName["owner-history"] !== null || state.pendingOperations && Object.keys(state.pendingOperations).length) return refusal("legacy_owner_shape_invalid");
          const appliedAt = (options.now ?? (() => new Date().toISOString()))();
          if (!timestamp(appliedAt)) return refusal("legacy_owner_adoption_time_invalid");
          const newOwner = { host: nativeHost, sessionId: native.sessionId };
          const ownership = { epoch: 1, current: { ...newOwner, since: appliedAt, operationId: request.operationId, writer } };
          const nextState = structuredClone(state);
          nextState.stateVersion += 1;
          nextState.ownership = ownership;
          nextState.integration = { ...nextState.integration, ownerSessionId: newOwner.sessionId, ownerHost: newOwner.host,
            ownershipEpoch: 1, authorized: false, hold: true };
          nextState.release = { ...nextState.release, ownerSessionId: newOwner.sessionId, ownerHost: newOwner.host,
            ownershipEpoch: 1, authorized: false, hold: true };
          const nextSetup = { ...setup, version: setup.version + 1, ownership: structuredClone(ownership) };
          let nextTeams = addLegacyHostLabels(byName.teams.toString("utf8"), nativeHost);
          nextTeams = replaceLabel(nextTeams, "Project owner", newOwner.sessionId);
          nextTeams = replaceLabel(nextTeams, "Integration owner", newOwner.sessionId);
          const nextHistory = { schemaVersion: 1, version: 1, ownership: { epoch: 1 }, entries: [] };
          const receipt = { schemaVersion: 1, operationId: request.operationId, signature: requestSignature, projectId: request.projectId,
            expectedLegacyOwnerSessionId: "root", ownershipEpoch: 1, newOwner, appliedAt,
            authorization: structuredClone(request.authorization), prior: { stateFingerprint: hashes.state, teamsFingerprint: hashes.teams,
              setupFingerprint: hashes.setup, ownerHistoryFingerprint: null },
            nativeEvidence: { host: nativeHost, sessionId: native.sessionId, cwd: git.nativeCwd, invocationId: native.invocationId, writer }, reason: request.reason };
          const nextBytes = { teams: Buffer.from(nextTeams), state: encodeJson(nextState), setup: encodeJson(nextSetup),
            "owner-history": encodeJson(nextHistory), receipt: encodeJson(receipt) };
          const records = entries.map(([name, file, before]) => journalRecord(name, file, before, nextBytes[name]));
          const journal = { schemaVersion: 1, operationId: request.operationId, signature: requestSignature, phase: "prepared", nextRecord: 0,
            receipt, records: records.map((record) => ({ name: record.name, path: record.path, priorSha256: record.priorSha256,
              priorimageBase64: record.before === null ? null : record.before.toString("base64"), postSha256: record.postSha256,
              postimageBase64: record.after.toString("base64") })) };
          await durableWrite(paths.journal, encodeJson(journal));
          if (options.failAfterJournal) throw new Error("injected_legacy_owner_adoption_crash_prepared");
          return { status: "applied", result: await rollForwardAdoption(project, journal, options) };
        }, { budget: options.budget }), { budget: options.budget }), { budget: options.budget }), { budget: options.budget });
}
