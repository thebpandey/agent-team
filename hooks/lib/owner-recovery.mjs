import { createHash, randomUUID } from "node:crypto";
import { constants } from "node:fs";
import { mkdir, open, readFile, rename, rm, unlink } from "node:fs/promises";
import path from "node:path";
import { withDirectoryLock } from "./lock.mjs";

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
function mintOwnerRecoveryCapabilityFromHostBootstrap(binding) {
  const keys = ["expiresAt", "invocationId", "projectIdentity", "replacementRuntime", "replacementSessionId"];
  if (!exactKeys(binding, keys) || !validId(binding.invocationId) || !validId(binding.replacementSessionId)
    || !["codex", "claude-code"].includes(binding.replacementRuntime)
    || !binding.projectIdentity || typeof binding.projectIdentity !== "object"
    || !Number.isSafeInteger(binding.expiresAt) || binding.expiresAt <= Date.now() || binding.expiresAt > Date.now() + 60_000) {
    throw new Error("invalid_owner_recovery_binding");
  }
  const capability = Object.freeze(Object.create(null));
  grants.set(capability, Object.freeze(structuredClone(binding)));
  return capability;
}

export function validateNativeRecoveryContext(context, project, now = Date.now()) {
  const capability = context?.capability;
  const grant = capability && typeof capability === "object" ? grants.get(capability) : undefined;
  const identity = context?.identity;
  const projectIdentity = { root: project?.root, commonDirectory: project?.commonDirectory };
  if (!grant || consumed.has(capability) || grant.expiresAt <= now || !exactKeys(identity,
    ["host", "sessionId", "invocationId", "projectRoot", "worktreeRoot"])
    || stable(grant.projectIdentity) !== stable(projectIdentity) || grant.invocationId !== identity.invocationId
    || grant.replacementRuntime !== identity.host || grant.replacementSessionId !== identity.sessionId
    || identity.projectRoot !== project.root || ![project.root, project.worktreeRoot].includes(identity.worktreeRoot)
    || typeof context.inspectSession !== "function" || typeof context.confirm !== "function") return false;
  consumed.add(capability);
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
    await handle.close(); handle = undefined;
    await rename(temporary, file);
    await fsyncDirectory(path.dirname(file));
  } finally { await handle?.close(); await rm(temporary, { force: true }); }
}

export const durableReplace = durableWrite;
export async function durableUnlink(file) {
  try { await unlink(file); await fsyncDirectory(path.dirname(file)); }
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

function validateJournal(journal, project) {
  if (journal?.schemaVersion !== 1 || !validId(journal.operationId) || !Array.isArray(journal.records)
    || journal.records.length !== 4 || !Number.isInteger(journal.nextRecord) || journal.nextRecord < 0 || journal.nextRecord > 4) {
    throw new Error("owner_recovery_manual_reconciliation_required");
  }
  const expected = recoveryPaths(project);
  for (let index = 0; index < expected.length; index += 1) {
    const record = journal.records[index];
    if (record.name !== expected[index][0] || record.path !== expected[index][1]
      || !(record.priorSha256 === null || hex(record.priorSha256, 64)) || !hex(record.postSha256, 64)
      || typeof record.postimageBase64 !== "string" || sha256(Buffer.from(record.postimageBase64, "base64")) !== record.postSha256) {
      throw new Error("owner_recovery_manual_reconciliation_required");
    }
  }
  return journal;
}

async function readJournal(project) {
  const source = await safeBytes(project.paths.ownerRecoveryJournal, { absent: true });
  return source === null ? undefined : validateJournal(JSON.parse(source), project);
}

async function rollForward(project, journal, options = {}) {
  for (let index = 0; index < journal.records.length; index += 1) {
    const record = journal.records[index];
    const current = await safeBytes(record.path, { absent: true });
    const currentHash = current === null ? null : sha256(current);
    if (currentHash === record.priorSha256) await durableReplace(record.path, Buffer.from(record.postimageBase64, "base64"));
    else if (currentHash !== record.postSha256) throw new Error("owner_recovery_manual_reconciliation_required");
    journal = { ...journal, phase: `renamed:${index + 1}`, nextRecord: index + 1 };
    await durableWrite(project.paths.ownerRecoveryJournal, encodeJson(journal));
    if (options.failAfterRename === index + 1) throw new Error(`injected_owner_recovery_crash_${index + 1}`);
  }
  const records = journal.records.map((record) => ({ ...record, path: record.path }));
  await verifyFourRecordPostimages(records);
  await durableWrite(project.paths.ownerRecoveryJournal, encodeJson({ ...journal, phase: "committed", nextRecord: 4 }));
  await durableUnlink(project.paths.ownerRecoveryJournal);
  return journal.receipt;
}

export async function assertNoOwnerRecoveryJournal(project) {
  if (!project?.paths?.ownerRecoveryJournal) return;
  if (await safeBytes(project.paths.ownerRecoveryJournal, { absent: true }) !== null) throw new Error("owner_recovery_in_progress");
}

/** Repair the four-record barrier before any canonical reader exposes state. */
export async function repairOwnerRecovery(project, options = {}) {
  if (!project?.paths?.ownerRecoveryJournal) return { status: "none" };
  const source = await safeBytes(project.paths.ownerRecoveryJournal, { absent: true });
  if (source === null) return { status: "none" };
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
  return { projectId: label("Project"), owner: label("Project owner"), ownerHost: label("Project owner host"), integrationOwner: label("Integration owner") };
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
  if (!validateNativeRecoveryContext(native, project, options.nowMs?.() ?? Date.now())) {
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
        const parsedHistory = byName["owner-history"] === null ? undefined : JSON.parse(byName["owner-history"]);
        const replay = parsedHistory?.entries?.find((entry) => entry.operationId === request.operationId);
        if (replay) return replay.signature === requestSignature
          ? { status: "duplicate", result: structuredClone(replay.receipt) }
          : refusal("operation_identity_reused");
        if (hashes.state !== request.expectedStateFingerprint || hashes.teams !== request.expectedTeamsFingerprint
          || hashes.setup !== request.expectedSetupFingerprint || hashes["owner-history"] !== request.expectedOwnerHistoryFingerprint) return refusal("stale_fingerprint");
        const state = JSON.parse(byName.state); const setup = JSON.parse(byName.setup); const teamsText = byName.teams.toString("utf8");
        const identity = parseTeams(teamsText);
        const history = parsedHistory ?? { schemaVersion: 1, version: 0, ownership: { epoch: state.ownership?.epoch ?? 1 }, entries: [] };
        const epoch = state.ownership?.epoch ?? setup.ownership?.epoch;
        if (identity.projectId !== request.projectId || project.projectId !== request.projectId || identity.owner !== request.expectedOwnerSessionId
          || epoch !== request.expectedOwnershipEpoch || setup.version !== request.expectedSetupVersion || state.stateVersion !== request.expectedStateVersion
          || history.ownership?.epoch !== epoch) return refusal("stale_owner_or_version");
        const liveness = await native.inspectSession({ host: identity.ownerHost, sessionId: identity.owner, writer: state.ownership?.current?.writer });
        if (liveness === "active") return refusal("owner_active");
        if (liveness !== "stopped") return refusal("owner_liveness_unknown");
        const approval = await native.confirm({ projectId: request.projectId, oldOwner: { host: identity.ownerHost, sessionId: identity.owner },
          newOwner: { host: native.identity.host, sessionId: native.identity.sessionId }, expectedSetupVersion: request.expectedSetupVersion,
          expectedStateVersion: request.expectedStateVersion, fingerprints: hashes, pendingOperations: state.pendingOperations ?? {}, holds: {
            integration: state.integration?.hold === true, release: state.release?.hold === true } });
        if (!approval || approval.approved !== true || !validId(approval.approvalId)) return refusal("owner_recovery_cancelled");
        const now = (options.now ?? (() => new Date().toISOString()))();
        const nextEpoch = epoch + 1;
        const newOwner = { host: native.identity.host, sessionId: native.identity.sessionId };
        const writer = approval.writer ?? { host: native.identity.host, invocationId: native.identity.invocationId };
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
          approvalId: approval.approvalId, livenessEvidence: approval.livenessEvidence ?? { status: "stopped" },
          priorStateFingerprint: hashes.state, priorTeamsFingerprint: hashes.teams, priorSetupFingerprint: hashes.setup,
          priorOwnerHistoryFingerprint: hashes["owner-history"], appliedAt: now };
        const nextHistory = { schemaVersion: 1, version: history.version + 1, ownership: { epoch: nextEpoch }, entries: [...history.entries, entry] };
        const nextBytes = { teams: Buffer.from(nextTeams), state: encodeJson(nextState), setup: encodeJson(nextSetup), "owner-history": encodeJson(nextHistory) };
        const records = entries.map(([name, file, before]) => journalRecord(name, file, before, nextBytes[name]));
        const journal = { schemaVersion: 1, operationId: request.operationId, signature: requestSignature, phase: "prepared", nextRecord: 0,
          expectedSetupVersion: request.expectedSetupVersion, expectedStateVersion: request.expectedStateVersion,
          expectedOwnerHistoryFingerprint: request.expectedOwnerHistoryFingerprint, receipt,
          records: records.map((record) => ({ name: record.name, path: record.path, priorSha256: record.priorSha256,
            postSha256: record.postSha256, postimageBase64: record.after.toString("base64") })) };
        await durableWrite(project.paths.ownerRecoveryJournal, encodeJson(journal));
        if (options.failAfterJournal === true) throw new Error("injected_owner_recovery_crash_prepared");
        const result = await rollForward(project, journal, options);
        return { status: "applied", result };
      }, { budget: options.budget }), { budget: options.budget }), { budget: options.budget });
}
