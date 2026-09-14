import { createHash, randomUUID } from "node:crypto";
import { lstat, mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { withDirectoryLock } from "./lock.mjs";

const HOSTS = new Set(["codex", "claude-code"]);
const KINDS = new Set(["mcp", "plugin"]);

function stable(value) {
  if (Array.isArray(value)) return `[${value.map(stable).join(",")}]`;
  if (value && typeof value === "object") return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stable(value[key])}`).join(",")}}`;
  return JSON.stringify(value);
}

function digest(value) {
  return createHash("sha256").update(typeof value === "string" || Buffer.isBuffer(value) ? value : stable(value)).digest("hex");
}

function exactKeys(value, keys) {
  return value && typeof value === "object" && !Array.isArray(value)
    && Object.keys(value).sort().join("\0") === [...keys].sort().join("\0");
}

function canonicalStatus(result, receipt) {
  if (!exactKeys(result, ["status", "receiptDigest"]) || result.receiptDigest !== receipt.digest
    || !["committed", "not_committed"].includes(result.status)) return "unknown";
  return result.status;
}

function absolute(value, label) {
  if (typeof value !== "string" || !path.isAbsolute(value) || path.normalize(value) !== value) throw new Error(`${label} must be absolute`);
  return value;
}

function candidateKey(candidate) {
  if (!candidate || !KINDS.has(candidate.kind) || typeof candidate.id !== "string" || !candidate.id) {
    throw new Error("visible capability requires kind mcp or plugin and a nonempty id");
  }
  return `${candidate.kind}:${candidate.id}`;
}

export function proposeContextReduction({ host, selectedProfile = {}, visible = [] } = {}) {
  if (!HOSTS.has(host)) throw new Error("host must select codex or claude-code");
  if (!Array.isArray(visible)) throw new Error("visible must be an array");
  const required = new Set(selectedProfile.required ?? []);
  const candidates = visible.filter((candidate) => candidate.visible === true && candidate.enabled === true)
    .map((candidate) => ({ key: candidateKey(candidate), kind: candidate.kind, id: candidate.id,
      label: candidate.label ?? candidate.id, source: candidate.source ?? "visible_host_inventory" }))
    .filter(({ key }) => !required.has(key))
    .sort((left, right) => left.key.localeCompare(right.key));
  const core = { schemaVersion: 1, host, profile: selectedProfile.id ?? null, required: [...required].sort(), candidates };
  return { ...core, proposalId: digest(core) };
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

async function atomicWrite(file, bytes, root, expectedSha256) {
  await ensureSafeDirectory(root, path.dirname(file));
  const current = await maybeRead(file, root);
  if ((current ? digest(current) : null) !== expectedSha256) throw new Error(`write_conflict: ${file}`);
  const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
  try {
    await writeFile(temporary, bytes, { mode: 0o600, flag: "wx" });
    await rename(temporary, file);
  } finally { await rm(temporary, { force: true }); }
}

async function removeIfCurrent(file, expectedSha256, root) {
  const current = await maybeRead(file, root);
  if ((current ? digest(current) : null) !== expectedSha256) throw new Error(`write_conflict: ${file}`);
  if (current) await rm(file);
}

async function updateJournalStatus(file, journal, bytes, status, root) {
  const next = { ...journal, status };
  const nextBytes = jsonBytes(next);
  await atomicWrite(file, nextBytes, root, digest(bytes));
  return { journal: next, bytes: nextBytes };
}

function transactionDigestFor(journal) {
  return digest(jsonBytes({ ...journal, status: "prepared" }));
}

function makeRecoveryAuditReceipt(journal, receiptBefore, scope) {
  let restoredReceiptDigest = null;
  if (receiptBefore) {
    const restored = parseJson(receiptBefore, receiptLocations(scope.projectRoot).receiptPath);
    if (!validateReceipt(restored, scope)) throw new Error("context recovery journal is invalid");
    restoredReceiptDigest = restored.digest;
  }
  return makeReceipt({ schemaVersion: 1, kind: "context-reduction-recovery", action: "rolled_back",
    host: scope.host, projectRoot: scope.projectRoot, transactionDigest: transactionDigestFor(journal),
    priorReceiptDigest: journal.setupReceipt.digest, restoredReceiptDigest });
}

function jsonBytes(value) {
  return Buffer.from(`${JSON.stringify(value, null, 2)}\n`);
}

function parseJson(bytes, file) {
  if (!bytes) return {};
  try {
    const parsed = JSON.parse(bytes);
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) throw new Error();
    return parsed;
  } catch { throw new Error(`configuration must be a JSON object: ${file}`); }
}

function parseHeader(line) {
  const match = line.match(/^\s*\[(mcp_servers|plugins)\.(?:"((?:[^"\\]|\\.)*)"|([A-Za-z0-9_-]+))\]\s*(?:#.*)?$/);
  if (!match) return null;
  return { group: match[1], id: match[2] === undefined ? match[3] : JSON.parse(`"${match[2]}"`) };
}

function findTomlSetting(source, kind, id) {
  const group = kind === "mcp" ? "mcp_servers" : "plugins";
  const lines = source.split("\n");
  const starts = [];
  for (let index = 0; index < lines.length; index += 1) {
    const header = parseHeader(lines[index]);
    if (header?.group === group && header.id === id) starts.push(index);
  }
  if (starts.length > 1) throw new Error(`ambiguous TOML section for ${kind}:${id}`);
  if (!starts.length) return { present: false, value: undefined, sectionPresent: false, lines };
  const start = starts[0];
  let end = lines.length;
  for (let index = start + 1; index < lines.length; index += 1) {
    if (/^\s*\[/.test(lines[index])) { end = index; break; }
  }
  const assignments = [];
  for (let index = start + 1; index < end; index += 1) {
    if (/^\s*enabled\s*=/.test(lines[index])) assignments.push(index);
  }
  if (assignments.length > 1) throw new Error(`ambiguous TOML enabled setting for ${kind}:${id}`);
  if (!assignments.length) return { present: false, value: undefined, sectionPresent: true, lines, start, end };
  const index = assignments[0];
  const match = lines[index].match(/^(\s*)enabled\s*=\s*(true|false)(\s*(?:#.*)?)$/);
  if (!match) throw new Error(`ambiguous TOML enabled setting for ${kind}:${id}`);
  return { present: true, value: match[2] === "true", sectionPresent: true, line: lines[index], index,
    indent: match[1], suffix: match[3], lines, start, end };
}

function disableTomlSetting(source, kind, id) {
  const setting = findTomlSetting(source, kind, id);
  const group = kind === "mcp" ? "mcp_servers" : "plugins";
  if (!setting.sectionPresent) {
    const separator = source && !source.endsWith("\n\n") ? (source.endsWith("\n") ? "\n" : "\n\n") : "";
    const insertedBlock = `${separator}[${group}.${JSON.stringify(id)}]\nenabled = false\n`;
    return { source: `${source}${insertedBlock}`, prior: { present: false, sectionPresent: false, insertedBlock } };
  }
  if (!setting.present) {
    setting.lines.splice(setting.start + 1, 0, "enabled = false");
    return { source: setting.lines.join("\n"), prior: { present: false, sectionPresent: true } };
  }
  setting.lines[setting.index] = `${setting.indent}enabled = false${setting.suffix}`;
  return { source: setting.lines.join("\n"), prior: { present: true, value: setting.value,
    sectionPresent: true, line: setting.line } };
}

function restoreTomlSetting(source, change) {
  const setting = findTomlSetting(source, change.kind, change.id);
  if (!setting.present || setting.value !== false) throw new Error("installed_value_changed");
  if (change.prior.present) {
    setting.lines[setting.index] = change.prior.line;
    return setting.lines.join("\n");
  }
  if (change.prior.sectionPresent) {
    setting.lines.splice(setting.index, 1);
    return setting.lines.join("\n");
  }
  const block = change.prior.insertedBlock;
  if (typeof block !== "string" || !source.includes(block) || source.indexOf(block) !== source.lastIndexOf(block)) {
    throw new Error("installed_value_changed");
  }
  return source.replace(block, "");
}

function validateProposal(proposal) {
  if (!exactKeys(proposal, ["schemaVersion", "host", "profile", "required", "candidates", "proposalId"])
    || proposal.schemaVersion !== 1 || !HOSTS.has(proposal.host) || !(proposal.profile === null || typeof proposal.profile === "string")
    || !Array.isArray(proposal.required) || proposal.required.some((item) => typeof item !== "string")
    || !Array.isArray(proposal.candidates) || typeof proposal.proposalId !== "string") throw new Error("context reduction proposal is invalid");
  const keys = new Set();
  for (const candidate of proposal.candidates) {
    if (!exactKeys(candidate, ["key", "kind", "id", "label", "source"]) || candidate.key !== candidateKey(candidate)
      || typeof candidate.label !== "string" || typeof candidate.source !== "string" || keys.has(candidate.key)) {
      throw new Error("context reduction proposal is invalid");
    }
    keys.add(candidate.key);
  }
  const { proposalId, ...core } = proposal;
  if (proposalId !== digest(core)) throw new Error("context reduction proposal identity is invalid");
}

function selectedCandidates(proposal, decision) {
  validateProposal(proposal);
  if (!exactKeys(decision, ["action", "reviewed", "proposalId", "selected"]) || decision.action !== "apply"
    || decision.reviewed !== true) throw new Error("context reduction requires reviewed approval");
  if (decision.proposalId !== proposal.proposalId) throw new Error("reviewed approval does not bind this proposal identity");
  if (!Array.isArray(decision.selected)) throw new Error("approved selection must be an array");
  const available = new Map(proposal.candidates.map((candidate) => [candidate.key, candidate]));
  return [...new Set(decision.selected)].map((key) => {
    const candidate = available.get(key);
    if (!candidate) throw new Error(`selection was not offered: ${key}`);
    return candidate;
  });
}

function receiptLocations(projectRoot, receiptPath) {
  const canonicalReceipt = path.join(projectRoot, ".agent-team", "context-reduction.json");
  if (receiptPath !== undefined && path.resolve(receiptPath) !== canonicalReceipt) throw new Error("context receipt path must be canonical");
  return { receiptPath: canonicalReceipt,
    journalPath: path.join(projectRoot, ".agent-team", "context-reduction-transaction.json"),
    lockPath: path.join(projectRoot, ".agent-team", ".locks", "context-reduction.lock") };
}

async function applyWrites({ files, receiptPath, receiptAfter, journalPath, setupReceipt, recordSetupReceipt, projectRoot }) {
  const records = [];
  for (const [file, value] of files) {
    const before = await maybeRead(file, value.root);
    records.push({ file, root: value.root, before, beforeSha256: before ? digest(before) : null,
      after: value.bytes, afterSha256: digest(value.bytes) });
  }
  const receiptBefore = await maybeRead(receiptPath, projectRoot);
  const receiptBeforeSha256 = receiptBefore ? digest(receiptBefore) : null;
  const receiptAfterSha256 = receiptAfter ? digest(receiptAfter) : null;
  let journal = { schemaVersion: 1, kind: "context-reduction", status: "prepared", setupReceipt,
    files: records.map(({ file, root, before, beforeSha256, after, afterSha256 }) => ({ file, root,
      before: before?.toString("base64") ?? null, beforeSha256, after: after.toString("base64"), afterSha256 })),
    receipt: { before: receiptBefore?.toString("base64") ?? null, beforeSha256: receiptBeforeSha256,
      after: receiptAfter?.toString("base64") ?? null, afterSha256: receiptAfterSha256 } };
  let journalBytes = jsonBytes(journal);
  await atomicWrite(journalPath, journalBytes, projectRoot, null);
  const applied = [];
  let receiptApplied = false;
  try {
    for (const record of records) {
      await atomicWrite(record.file, record.after, record.root, record.beforeSha256);
      applied.push(record);
    }
    if (receiptAfter) await atomicWrite(receiptPath, receiptAfter, projectRoot, receiptBeforeSha256);
    else await removeIfCurrent(receiptPath, receiptBeforeSha256, projectRoot);
    receiptApplied = true;
    ({ journal, bytes: journalBytes } = await updateJournalStatus(journalPath, journal, journalBytes,
      "local_applied", projectRoot));
  } catch (error) {
    try {
      for (const record of applied) {
        const current = await maybeRead(record.file, record.root);
        if ((current ? digest(current) : null) !== record.afterSha256) throw new Error(`write_conflict: ${record.file}`);
      }
      if (receiptApplied) {
        const currentReceipt = await maybeRead(receiptPath, projectRoot);
        if ((currentReceipt ? digest(currentReceipt) : null) !== receiptAfterSha256) throw new Error(`write_conflict: ${receiptPath}`);
      }
      for (const record of applied.toReversed()) {
        if (record.before) await atomicWrite(record.file, record.before, record.root, record.afterSha256);
        else await removeIfCurrent(record.file, record.afterSha256, record.root);
      }
      if (receiptApplied) {
        if (receiptBefore) await atomicWrite(receiptPath, receiptBefore, projectRoot, receiptAfterSha256);
        else await removeIfCurrent(receiptPath, receiptAfterSha256, projectRoot);
      }
      await removeIfCurrent(journalPath, digest(journalBytes), projectRoot);
    } catch (rollbackError) {
      throw new Error(`recovery_conflict: context rollback stopped safely: ${rollbackError.message}`, { cause: error });
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
      ({ journal, bytes: journalBytes } = await updateJournalStatus(journalPath, journal, journalBytes,
        "canonical_not_committed", projectRoot));
      for (const record of applied) {
        const current = await maybeRead(record.file, record.root);
        if ((current ? digest(current) : null) !== record.afterSha256) throw new Error(`write_conflict: ${record.file}`);
      }
      const currentReceipt = await maybeRead(receiptPath, projectRoot);
      if ((currentReceipt ? digest(currentReceipt) : null) !== receiptAfterSha256) throw new Error(`write_conflict: ${receiptPath}`);
      for (const record of applied.toReversed()) {
        if (record.before) await atomicWrite(record.file, record.before, record.root, record.afterSha256);
        else await removeIfCurrent(record.file, record.afterSha256, record.root);
      }
      if (receiptBefore) await atomicWrite(receiptPath, receiptBefore, projectRoot, receiptAfterSha256);
      else await removeIfCurrent(receiptPath, receiptAfterSha256, projectRoot);
      await removeIfCurrent(journalPath, digest(journalBytes), projectRoot);
    } catch (rollbackError) {
      throw new Error(`recovery_conflict: context rollback stopped safely: ${rollbackError.message}`);
    }
    throw new Error("canonical setup receipt was not committed");
  }
  try {
    ({ journal, bytes: journalBytes } = await updateJournalStatus(journalPath, journal, journalBytes,
      "canonical_committed", projectRoot));
    await removeIfCurrent(journalPath, digest(journalBytes), projectRoot);
  } catch (error) {
    throw new Error(`recovery_required: canonical receipt committed but context finalization failed: ${error.message}`, { cause: error });
  }
}

function configPathFor(host, kind, projectRoot, home) {
  if (host === "codex") return { file: path.join(projectRoot, ".codex", "config.toml"), root: projectRoot };
  if (kind === "plugin") return { file: path.join(projectRoot, ".claude", "settings.json"), root: projectRoot };
  absolute(home, "home for Claude MCP project settings");
  return { file: path.join(home, ".claude.json"), root: home };
}

function makeReceipt(core) {
  return { ...core, digest: digest(core) };
}

function validPrior(change, host) {
  const prior = change.prior;
  if (host === "codex") {
    if (prior.present === true) {
      if (!exactKeys(prior, ["present", "value", "sectionPresent", "line"]) || prior.sectionPresent !== true
        || typeof prior.value !== "boolean" || typeof prior.line !== "string" || prior.line.includes("\n")) return false;
      const match = prior.line.match(/^\s*enabled\s*=\s*(true|false)\s*(?:#.*)?$/);
      return Boolean(match) && (match[1] === "true") === prior.value;
    }
    if (prior.sectionPresent === true) return exactKeys(prior, ["present", "sectionPresent"]) && prior.present === false;
    if (!exactKeys(prior, ["present", "sectionPresent", "insertedBlock"]) || prior.present !== false
      || prior.sectionPresent !== false || typeof prior.insertedBlock !== "string") return false;
    const group = change.kind === "mcp" ? "mcp_servers" : "plugins";
    const block = `[${group}.${JSON.stringify(change.id)}]\nenabled = false\n`;
    return [block, `\n${block}`, `\n\n${block}`].includes(prior.insertedBlock);
  }
  if (change.kind === "plugin") {
    return prior.present === true
      ? exactKeys(prior, ["present", "value"]) && typeof prior.value === "boolean"
      : exactKeys(prior, ["present"]) && prior.present === false;
  }
  return exactKeys(prior, ["present", "value"]) && typeof prior.present === "boolean"
    && prior.value === prior.present;
}

export async function applyContextReduction({ projectRoot, home, proposal, decision, receiptPath, recordSetupReceipt } = {}) {
  if (["no_answer", "cancel"].includes(decision?.action)) return { status: decision.action, changed: false };
  if (decision?.action !== "apply") throw new Error("decision action must select apply, cancel, or no_answer");
  absolute(projectRoot, "projectRoot");
  const selected = selectedCandidates(proposal, decision);
  if (!selected.length) return { status: "no_selection", changed: false };
  if (typeof recordSetupReceipt !== "function") throw new Error("recordSetupReceipt is required before context reduction");
  const locations = receiptLocations(projectRoot, receiptPath);
  await ensureSafeDirectory(projectRoot, path.dirname(locations.lockPath));
  return withDirectoryLock(locations.lockPath, { kind: "context-reduction", pid: process.pid }, async () => {
    if (await maybeRead(locations.journalPath, projectRoot)) throw new Error("recovery_required: context transaction is pending");
    if (await maybeRead(locations.receiptPath, projectRoot)) return { status: "conflict", reason: "context_reduction_already_applied", changed: false };
    const changes = [];
    const files = new Map();

    if (proposal.host === "codex") {
      const location = configPathFor("codex", "mcp", projectRoot, home);
      let source = (await maybeRead(location.file, location.root))?.toString("utf8") ?? "";
      for (const { key, kind, id, source: candidateSource } of selected) {
        const edited = disableTomlSetting(source, kind, id);
        source = edited.source;
        changes.push({ key, kind, id, source: candidateSource, configPath: location.file, prior: edited.prior, installed: false });
      }
      files.set(location.file, { root: location.root, bytes: Buffer.from(source) });
    } else {
      const plugins = selected.filter(({ kind }) => kind === "plugin");
      if (plugins.length) {
        const location = configPathFor(proposal.host, "plugin", projectRoot, home);
        const settings = parseJson(await maybeRead(location.file, location.root), location.file);
        if (settings.enabledPlugins !== undefined && (!settings.enabledPlugins || typeof settings.enabledPlugins !== "object" || Array.isArray(settings.enabledPlugins))) {
          throw new Error(`enabledPlugins must be an object: ${location.file}`);
        }
        settings.enabledPlugins ??= {};
        for (const { key, kind, id, source } of plugins) {
          const present = Object.hasOwn(settings.enabledPlugins, id);
          if (present && typeof settings.enabledPlugins[id] !== "boolean") throw new Error(`enabledPlugins value must be boolean: ${id}`);
          const prior = present ? { present, value: settings.enabledPlugins[id] } : { present };
          changes.push({ key, kind, id, source, configPath: location.file, prior, installed: false });
          settings.enabledPlugins[id] = false;
        }
        files.set(location.file, { root: location.root, bytes: jsonBytes(settings) });
      }
      const mcps = selected.filter(({ kind }) => kind === "mcp");
      if (mcps.length) {
        const location = configPathFor(proposal.host, "mcp", projectRoot, home);
        const settings = parseJson(await maybeRead(location.file, location.root), location.file);
        settings.projects ??= {};
        if (!settings.projects || typeof settings.projects !== "object" || Array.isArray(settings.projects)) throw new Error(`projects must be an object: ${location.file}`);
        settings.projects[projectRoot] ??= {};
        const project = settings.projects[projectRoot];
        if (!project || typeof project !== "object" || Array.isArray(project)) throw new Error(`project settings must be an object: ${location.file}`);
        project.disabledMcpServers ??= [];
        if (!Array.isArray(project.disabledMcpServers) || project.disabledMcpServers.some((id) => typeof id !== "string")) {
          throw new Error(`disabledMcpServers must be a string array: ${location.file}`);
        }
        for (const { key, kind, id, source } of mcps) {
          const present = project.disabledMcpServers.includes(id);
          changes.push({ key, kind, id, source, configPath: location.file, prior: { present, value: present }, installed: true });
          if (!present) project.disabledMcpServers.push(id);
        }
        files.set(location.file, { root: location.root, bytes: jsonBytes(settings) });
      }
    }
    const receipt = makeReceipt({ schemaVersion: 1, kind: "context-reduction", host: proposal.host, projectRoot,
      proposalId: proposal.proposalId, changes });
    await applyWrites({ files, receiptPath: locations.receiptPath, receiptAfter: jsonBytes(receipt),
      journalPath: locations.journalPath, setupReceipt: receipt, recordSetupReceipt, projectRoot });
    return { status: "applied", changed: true, receiptPath: locations.receiptPath, setupReceipt: receipt };
  });
}

function validateReceipt(receipt, { projectRoot, home, host }) {
  if (!exactKeys(receipt, ["schemaVersion", "kind", "host", "projectRoot", "proposalId", "changes", "digest"])
    || receipt.schemaVersion !== 1 || receipt.kind !== "context-reduction" || receipt.host !== host
    || receipt.projectRoot !== projectRoot || typeof receipt.proposalId !== "string" || !Array.isArray(receipt.changes)) return false;
  const { digest: recorded, ...core } = receipt;
  if (recorded !== digest(core)) return false;
  const seen = new Set();
  for (const change of receipt.changes) {
    let derivedKey; let expectedPath;
    try { derivedKey = candidateKey(change); expectedPath = configPathFor(host, change.kind, projectRoot, home).file; }
    catch { return false; }
    if (!exactKeys(change, ["key", "kind", "id", "source", "configPath", "prior", "installed"])
      || change.key !== derivedKey || seen.has(change.key) || change.installed !== (change.kind === "mcp" && host === "claude-code")
      || typeof change.source !== "string" || !change.source || change.configPath !== expectedPath || !validPrior(change, host)) return false;
    seen.add(change.key);
  }
  return receipt.changes.length > 0;
}

async function readReceipt(file, root) {
  const bytes = await maybeRead(file, root);
  if (!bytes) return null;
  return parseJson(bytes, file);
}

export async function inspectContextReduction({ projectRoot, home, host, receiptPath } = {}) {
  absolute(projectRoot, "projectRoot");
  const locations = receiptLocations(projectRoot, receiptPath);
  const receipt = await readReceipt(locations.receiptPath, projectRoot);
  const journal = await maybeRead(locations.journalPath, projectRoot);
  const pendingTransaction = Boolean(journal);
  let receiptValid = !receipt;
  if (receipt) {
    const receiptHost = host ?? receipt.host;
    try { receiptValid = HOSTS.has(receiptHost) && validateReceipt(receipt, { projectRoot, home, host: receiptHost }); }
    catch { receiptValid = false; }
  }
  return { status: pendingTransaction ? "recovery_required" : receiptValid ? (receipt ? "applied" : "not_applied") : "conflict",
    reason: receiptValid ? undefined : "receipt_invalid", receiptPath: locations.receiptPath, receipt,
    pendingTransaction, pendingTransactionDigest: journal ? digest(journal) : null };
}

export async function revertContextReduction({ projectRoot, home, host, receiptPath, expectedReceiptDigest, recordSetupReceipt } = {}) {
  absolute(projectRoot, "projectRoot");
  if (!HOSTS.has(host)) throw new Error("host must select codex or claude-code");
  if (typeof recordSetupReceipt !== "function") throw new Error("recordSetupReceipt is required before context reduction revert");
  const locations = receiptLocations(projectRoot, receiptPath);
  await ensureSafeDirectory(projectRoot, path.dirname(locations.lockPath));
  return withDirectoryLock(locations.lockPath, { kind: "context-reduction-revert", pid: process.pid }, async () => {
    if (await maybeRead(locations.journalPath, projectRoot)) throw new Error("recovery_required: context transaction is pending");
    const receipt = await readReceipt(locations.receiptPath, projectRoot);
    if (!receipt) return { status: "not_applied", changed: false };
    if (typeof expectedReceiptDigest !== "string" || expectedReceiptDigest !== receipt.digest
      || !validateReceipt(receipt, { projectRoot, home, host })) {
      return { status: "conflict", reason: "receipt_invalid", changed: false };
    }
    const grouped = new Map();
    for (const change of receipt.changes) {
      const location = configPathFor(host, change.kind, projectRoot, home);
      if (!grouped.has(location.file)) {
        const original = await maybeRead(location.file, location.root);
        grouped.set(location.file, { root: location.root, original, next: original });
      }
    }
    const conflicts = [];
    for (const change of receipt.changes) {
      const group = grouped.get(change.configPath);
      if (host === "codex") {
        try {
          const source = group.next?.toString("utf8") ?? "";
          group.next = Buffer.from(restoreTomlSetting(source, change));
        } catch { conflicts.push({ key: change.key, reason: "installed_value_changed" }); }
      } else {
        const config = parseJson(group.next, change.configPath);
        const current = change.kind === "plugin" ? config.enabledPlugins?.[change.id]
          : config.projects?.[projectRoot]?.disabledMcpServers?.includes(change.id) ?? false;
        if (current !== change.installed) { conflicts.push({ key: change.key, reason: "installed_value_changed" }); continue; }
        if (change.kind === "plugin") {
          config.enabledPlugins ??= {};
          if (change.prior.present) config.enabledPlugins[change.id] = change.prior.value;
          else delete config.enabledPlugins[change.id];
        } else {
          const list = config.projects?.[projectRoot]?.disabledMcpServers ?? [];
          config.projects[projectRoot].disabledMcpServers = change.prior.present
            ? [...new Set([...list, change.id])] : list.filter((id) => id !== change.id);
        }
        group.next = jsonBytes(config);
      }
    }
    if (conflicts.length) return { status: "conflict", changed: false, conflicts };
    const revertReceipt = makeReceipt({ schemaVersion: 1, kind: "context-reduction-revert", host, projectRoot,
      priorReceiptDigest: receipt.digest, changes: receipt.changes });
    const files = new Map([...grouped].map(([file, value]) => [file, { root: value.root, bytes: value.next }]));
    await applyWrites({ files, receiptPath: locations.receiptPath, receiptAfter: null, journalPath: locations.journalPath,
      setupReceipt: revertReceipt, recordSetupReceipt, projectRoot });
    return { status: "reverted", changed: true, setupReceipt: revertReceipt };
  });
}

function decodeJournalBytes(value, expectedSha256) {
  if (value === null) return expectedSha256 === null ? null : undefined;
  if (typeof value !== "string") return undefined;
  const bytes = Buffer.from(value, "base64");
  return digest(bytes) === expectedSha256 ? bytes : undefined;
}

function validRecoveryReceipt(receipt, scope) {
  if (receipt?.kind === "context-reduction") return validateReceipt(receipt, scope);
  if (!exactKeys(receipt, ["schemaVersion", "kind", "host", "projectRoot", "priorReceiptDigest", "changes", "digest"])
    || receipt.schemaVersion !== 1 || receipt.kind !== "context-reduction-revert" || receipt.host !== scope.host
    || receipt.projectRoot !== scope.projectRoot || !/^[0-9a-f]{64}$/i.test(receipt.priorReceiptDigest)
    || !Array.isArray(receipt.changes)) return false;
  const { digest: recorded, ...core } = receipt;
  if (recorded !== digest(core)) return false;
  const synthetic = { schemaVersion: 1, kind: "context-reduction", host: receipt.host, projectRoot: receipt.projectRoot,
    proposalId: "recovery", changes: receipt.changes };
  synthetic.digest = digest(synthetic);
  return validateReceipt(synthetic, scope);
}

function validRecoveryAuditReceipt(receipt, scope) {
  if (!exactKeys(receipt, ["schemaVersion", "kind", "action", "host", "projectRoot", "transactionDigest",
    "priorReceiptDigest", "restoredReceiptDigest", "digest"])
    || receipt.schemaVersion !== 1 || receipt.kind !== "context-reduction-recovery" || receipt.action !== "rolled_back"
    || receipt.host !== scope.host || receipt.projectRoot !== scope.projectRoot
    || !/^[0-9a-f]{64}$/.test(receipt.transactionDigest) || !/^[0-9a-f]{64}$/.test(receipt.priorReceiptDigest)
    || !(receipt.restoredReceiptDigest === null || /^[0-9a-f]{64}$/.test(receipt.restoredReceiptDigest))) return false;
  const { digest: recorded, ...core } = receipt;
  return recorded === digest(core);
}

/** Bind the canonical setup receipt to the validated local applied state after apply, revert, or recovery. */
export function contextReceiptBinding({ canonicalReceipt, localReceipt, projectRoot, home, host } = {}) {
  const scope = { projectRoot, home, host };
  if (!canonicalReceipt && !localReceipt) return { status: "empty", appliedReceipt: null };
  if (canonicalReceipt?.kind === "context-reduction" && localReceipt
    && validateReceipt(canonicalReceipt, scope) && validateReceipt(localReceipt, scope)
    && stable(canonicalReceipt) === stable(localReceipt)) return { status: "linked", appliedReceipt: structuredClone(localReceipt) };
  if (canonicalReceipt?.kind === "context-reduction-revert" && !localReceipt
    && validRecoveryReceipt(canonicalReceipt, scope)) return { status: "linked", appliedReceipt: null };
  if (canonicalReceipt?.kind === "context-reduction-recovery" && validRecoveryAuditReceipt(canonicalReceipt, scope)) {
    if (canonicalReceipt.restoredReceiptDigest === null && !localReceipt) return { status: "linked", appliedReceipt: null };
    if (localReceipt && validateReceipt(localReceipt, scope)
      && localReceipt.digest === canonicalReceipt.restoredReceiptDigest) return { status: "linked", appliedReceipt: structuredClone(localReceipt) };
  }
  return { status: "mismatch", appliedReceipt: null };
}

export async function recoverContextReductionTransaction({ projectRoot, home, host, receiptPath,
  expectedJournalDigest, recordSetupReceipt } = {}) {
  absolute(projectRoot, "projectRoot");
  if (!HOSTS.has(host)) throw new Error("host must select codex or claude-code");
  if (!/^[0-9a-f]{64}$/i.test(expectedJournalDigest ?? "")) throw new Error("expected journal digest is required");
  if (typeof recordSetupReceipt !== "function") throw new Error("recordSetupReceipt is required before context recovery");
  const locations = receiptLocations(projectRoot, receiptPath);
  await ensureSafeDirectory(projectRoot, path.dirname(locations.lockPath));
  return withDirectoryLock(locations.lockPath, { kind: "context-reduction-recovery", pid: process.pid }, async () => {
    let journalBytes = await maybeRead(locations.journalPath, projectRoot);
    if (!journalBytes) return { status: "not_pending", changed: false };
    if (digest(journalBytes) !== expectedJournalDigest) throw new Error("journal digest changed");
    let journal;
    try { journal = JSON.parse(journalBytes); } catch { throw new Error("context recovery journal is invalid"); }
    const scope = { projectRoot, home, host };
    if (!exactKeys(journal, ["schemaVersion", "kind", "status", "setupReceipt", "files", "receipt"])
      || journal.schemaVersion !== 1 || journal.kind !== "context-reduction"
      || !["prepared", "local_applied", "canonical_not_committed", "canonical_committed",
        "recovery_local_applied", "recovery_canonical_committed"].includes(journal.status)
      || !Array.isArray(journal.files) || !validRecoveryReceipt(journal.setupReceipt, scope)
      || !exactKeys(journal.receipt, ["before", "beforeSha256", "after", "afterSha256"])) {
      throw new Error("context recovery journal is invalid");
    }
    const expectedFiles = new Map(journal.setupReceipt.changes.map((change) => {
      const location = configPathFor(host, change.kind, projectRoot, home);
      return [location.file, location.root];
    }));
    if (journal.files.length !== expectedFiles.size) throw new Error("context recovery journal is invalid");
    const records = [];
    const seen = new Set();
    for (const record of journal.files) {
      const expectedRoot = expectedFiles.get(record.file);
      if (!exactKeys(record, ["file", "root", "before", "beforeSha256", "after", "afterSha256"])
        || expectedRoot === undefined || record.root !== expectedRoot || seen.has(record.file)) throw new Error("context recovery journal is invalid");
      seen.add(record.file);
      const before = decodeJournalBytes(record.before, record.beforeSha256);
      const after = decodeJournalBytes(record.after, record.afterSha256);
      if (before === undefined || after === undefined || after === null) throw new Error("context recovery journal is invalid");
      records.push({ file: record.file, root: expectedRoot, before, beforeSha256: record.beforeSha256,
        after, afterSha256: record.afterSha256 });
    }
    const receiptBefore = decodeJournalBytes(journal.receipt.before, journal.receipt.beforeSha256);
    const receiptAfter = decodeJournalBytes(journal.receipt.after, journal.receipt.afterSha256);
    if (receiptBefore === undefined || receiptAfter === undefined) throw new Error("context recovery journal is invalid");
    for (const record of records) {
      const current = await maybeRead(record.file, record.root);
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
        setupReceipt: recoveryCommitted ? makeRecoveryAuditReceipt(journal, receiptBefore, scope) : journal.setupReceipt };
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
      if (record.before) await atomicWrite(record.file, record.before, record.root, record.afterSha256);
      else await removeIfCurrent(record.file, record.afterSha256, record.root);
      changed.push(record);
    }
    if (currentReceiptSha256 !== journal.receipt.beforeSha256) {
      if (receiptBefore) await atomicWrite(locations.receiptPath, receiptBefore, projectRoot, journal.receipt.afterSha256);
      else await removeIfCurrent(locations.receiptPath, journal.receipt.afterSha256, projectRoot);
      receiptChanged = true;
    }
    ({ journal, bytes: journalBytes } = await updateJournalStatus(locations.journalPath, journal, journalBytes,
      "recovery_local_applied", projectRoot));
    const recoveryReceipt = makeRecoveryAuditReceipt(journal, receiptBefore, scope);
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

export const __contextShrinkTest = Object.freeze({ findTomlSetting, disableTomlSetting, restoreTomlSetting });
