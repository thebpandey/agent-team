import { constants } from "node:fs";
import { open, readFile, realpath } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { isDeepStrictEqual } from "node:util";
import { loadCanonicalState } from "./canonical-state.mjs";
import { applyContextReduction, contextReceiptBinding, inspectContextReduction, proposeContextReduction,
  recoverContextReductionTransaction, revertContextReduction } from "./context-shrink.mjs";
import { createDependencyRunner, inspectDependencies, prepareDependencies } from "./dependencies.mjs";
import { ROLE_DEFINITIONS } from "./dependency-profiles.mjs";
import { inspectHelpers, installHelpers, recoverHelperTransaction } from "./helpers.mjs";
import { initializationRecordProblem } from "./initialization.mjs";
import { validateNativeOwnerAuthority } from "./owner-recovery.mjs";
import { resolveProject } from "./project.mjs";
import { assessReadiness } from "./readiness.mjs";
import { buildRoleMenu, buildSettingsWizard, inspectSettings, mutateSetup, saveSettingsDraft, updateSettings,
  validateExecutionSettings, validateExecutionSettingsChange } from "./settings.mjs";
import { taskEligibility } from "./task-transitions.mjs";
import { resolveTracker } from "./tracker.mjs";

const selectors = ["project", "host", "scope"];
export const setupCommandFlags = Object.freeze({
  settings: new Set([...selectors, "role"]),
  "settings-wizard": new Set([...selectors, "request"]),
  readiness: new Set([...selectors, "request"]),
  dependencies: new Set(selectors),
  "settings-update": new Set([...selectors, "request"]),
  "dependencies-prepare": new Set([...selectors, "home", "request"]),
  "dashboard-configure": new Set([...selectors, "request"]),
  helpers: new Set(selectors),
  "helpers-install": new Set([...selectors, "request"]),
  "helpers-recover": new Set([...selectors, "request"]),
  "context-reduction": new Set([...selectors, "request"]),
  "context-reduction-apply": new Set([...selectors, "request"]),
  "context-reduction-revert": new Set([...selectors, "request"]),
  "context-reduction-recover": new Set([...selectors, "request"]),
});

const mutations = new Set(["settings-update", "dependencies-prepare", "dashboard-configure", "helpers-install", "helpers-recover",
  "context-reduction-apply", "context-reduction-revert", "context-reduction-recover"]);
const requestLimit = 256 * 1024;
const identityPattern = /^[\w.:-]{1,128}$/;

function object(value, label) {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error(`${label} must be an object.`);
  return value;
}

function fields(value, allowed, label) {
  object(value, label);
  for (const key of Object.keys(value)) if (!allowed.includes(key)) throw new Error(`Unsupported ${label} field: ${key}.`);
  return value;
}

function string(value, label) {
  if (typeof value !== "string" || !value.trim() || value.includes("\0")) throw new Error(`${label} must be a nonempty string.`);
  return value;
}

function identity(value, label) {
  if (!identityPattern.test(string(value, label)) || ["none", "unknown", "unassigned", "-"].includes(value.toLowerCase())) throw new Error(`${label} is invalid.`);
  return value;
}

function stringList(value, label) {
  if (!Array.isArray(value) || value.some((item) => typeof item !== "string" || !item.trim())) throw new Error(`${label} must be a string array.`);
  return value;
}

/** Bound the opened file itself, including growth after stat; never wait on a FIFO. */
async function requestEnvelope(file) {
  let handle;
  try {
    handle = await open(path.resolve(string(file, "--request")), constants.O_RDONLY | constants.O_NONBLOCK);
    const stat = await handle.stat();
    if (!stat.isFile() || stat.size > requestLimit) throw new Error("Request must be a bounded regular JSON file.");
    const bytes = Buffer.alloc(requestLimit + 1);
    let length = 0;
    while (length < bytes.length) {
      const { bytesRead } = await handle.read(bytes, length, bytes.length - length, null);
      if (!bytesRead) break;
      length += bytesRead;
    }
    if (length > requestLimit) throw new Error("Request must be a bounded regular JSON file.");
    const envelope = fields(JSON.parse(bytes.subarray(0, length).toString("utf8")), ["schemaVersion", "expectedVersion", "operationId", "writer", "request"], "envelope");
    if (envelope.schemaVersion !== 1) throw new Error("Request schemaVersion must be 1.");
    object(envelope.request, "request");
    return envelope;
  } catch (error) {
    if (error instanceof SyntaxError) throw new Error("Request must contain valid JSON.");
    throw error;
  } finally {
    await handle?.close();
  }
}

function mutationIdentity(envelope) {
  if (!Number.isSafeInteger(envelope.expectedVersion) || envelope.expectedVersion < 0) throw new Error("Request expectedVersion must be a nonnegative integer.");
  identity(envelope.operationId, "operationId");
  fields(envelope.writer, ["id", "role"], "writer");
  identity(envelope.writer.id, "writer.id");
  if (envelope.writer.role !== "project_orchestrator") throw new Error("writer.role must be project_orchestrator.");
  return { expectedVersion: envelope.expectedVersion, operationId: envelope.operationId, writer: envelope.writer };
}

function validateChange(change) {
  object(change, "change");
  const allowed = { role: ["kind", "role", "model", "effort"], run: ["kind", "values"], execution: ["kind", "values"],
    fallback: ["kind", "role", "routes", "escalation"], cancel: ["kind"], back: ["kind"] };
  if (!allowed[change.kind]) throw new Error("Unknown settings change kind.");
  fields(change, allowed[change.kind], "change");
  if (["run", "execution"].includes(change.kind)) {
    object(change.values, "change.values");
    if (!Object.keys(change.values).length) throw new Error("change.values must not be empty.");
  }
  if (change.kind === "execution") validateExecutionSettingsChange(change.values, "execution settings change");
  if (change.kind === "role") for (const key of ["role", "model", "effort"]) string(change[key], `change.${key}`);
  if (change.kind === "fallback") {
    string(change.role, "change.role");
    if (!Array.isArray(change.routes)) throw new Error("change.routes must be an array.");
    for (const route of change.routes) {
      fields(route, ["model", "effort"], "fallback route");
      string(route.model, "route.model");
      string(route.effort, "route.effort");
    }
    if (change.escalation !== undefined && typeof change.escalation !== "boolean") throw new Error("change.escalation must be boolean.");
  }
  return change;
}

function validateDraft(draft) {
  fields(draft, ["roles", "execution"], "draft");
  if (draft.roles !== undefined) {
    fields(draft.roles, ROLE_DEFINITIONS.map(({ id }) => id), "draft.roles");
    for (const route of Object.values(draft.roles)) {
      fields(route, ["model", "effort"], "draft role");
      for (const [key, value] of Object.entries(route)) string(value, `draft.${key}`);
    }
  }
  if (draft.execution !== undefined) validateExecutionSettings(draft.execution, "settings draft execution");
  return draft;
}

function capabilityInventory(value, host) {
  if (value === undefined) return null;
  fields(value, ["schemaVersion", "host", "observed", "profile", "capabilities"], "capability inventory");
  if (value.schemaVersion !== 1 || value.host !== host || value.observed !== true) return null;
  fields(value.profile, ["id", "required"], "capability inventory profile");
  if (!(value.profile.id === null || typeof value.profile.id === "string" && value.profile.id.trim())) return null;
  stringList(value.profile.required, "capability inventory profile.required");
  if (!Array.isArray(value.capabilities)) throw new Error("capability inventory capabilities must be an array.");
  const seen = new Set();
  for (const entry of value.capabilities) {
    fields(entry, ["kind", "id", "enabled", "visible", "label", "source"], "visible capability");
    if (!["mcp", "plugin"].includes(entry.kind)) throw new Error("visible capability kind must be mcp or plugin.");
    string(entry.id, "visible capability id");
    if (typeof entry.enabled !== "boolean" || typeof entry.visible !== "boolean") throw new Error("visible capability state must be boolean.");
    if (entry.label !== undefined) string(entry.label, "visible capability label");
    if (entry.source !== undefined) string(entry.source, "visible capability source");
    const key = `${entry.kind}:${entry.id}`;
    if (seen.has(key)) throw new Error("visible capability identity must be unique.");
    seen.add(key);
  }
  if (new Set(value.profile.required).size !== value.profile.required.length
    || value.profile.required.some((key) => !/^(mcp|plugin):[^\s]+$/.test(key))) {
    throw new Error("capability inventory profile.required is invalid.");
  }
  return structuredClone(value);
}

function ownerReportedCapabilities(value, host) {
  if (value === undefined) return null;
  fields(value, ["schemaVersion", "host", "capabilities"], "owner-reported capabilities");
  if (value.schemaVersion !== 1 || value.host !== host) throw new Error("owner-reported capability host does not match the selected host.");
  if (!Array.isArray(value.capabilities) || value.capabilities.length > 128) {
    throw new Error("owner-reported capabilities must be a bounded array.");
  }
  const seen = new Set();
  const capabilities = value.capabilities.map((entry) => {
    fields(entry, ["kind", "id", "enabled", "visible", "label"], "owner-reported capability");
    if (!["mcp", "plugin"].includes(entry.kind)) throw new Error("owner-reported capability kind must be mcp or plugin.");
    string(entry.id, "owner-reported capability id");
    if (typeof entry.enabled !== "boolean" || typeof entry.visible !== "boolean") {
      throw new Error("owner-reported capability state must be boolean.");
    }
    if (entry.label !== undefined) string(entry.label, "owner-reported capability label");
    const key = `${entry.kind}:${entry.id}`;
    if (seen.has(key)) throw new Error("owner-reported capability identity must be unique.");
    seen.add(key);
    return { ...structuredClone(entry), source: "owner_reported_unverified" };
  });
  return { schemaVersion: 1, host, capabilities };
}

function canonicalCapabilityProfile(project, host, capabilities) {
  const selected = project.setup.dependencies?.hosts?.[host]?.selected ?? [];
  if (!Array.isArray(selected) || selected.some((id) => typeof id !== "string" || !id)) {
    throw new Error("canonical dependency selection is invalid.");
  }
  const selectedIds = new Set(selected);
  return { id: `dependencies:${host}`, required: capabilities
    .filter(({ id }) => selectedIds.has(id)).map(({ kind, id }) => `${kind}:${id}`).sort() };
}

async function canonicalReadiness(project, host, scope, context) {
  let canonical;
  try { canonical = await loadCanonicalState(project, { budget: context.budget }); }
  catch (error) {
    return { status: "unavailable", reason: error.message, readyForDispatch: false, eligibleTask: null,
      projectInitialization: { required: true, projectId: project.projectId, projectOwner: null, reason: error.message } };
  }
  const eligible = new Set(taskEligibility(canonical, {
    scopeTaskIds: canonical.state.run?.taskIds,
    capacity: canonical.state.capacity,
  }).eligible.map(({ id }) => id));
  const finished = new Set(["verified", "integrated", "deployed", "closed", "done", "completed", "complete"]);
  const tasks = canonical.tasks.map((task) => ({
    ...task,
    status: eligible.has(task.id) ? "ready" : finished.has(task.status) ? "complete" : "blocked",
    dependencies: Array.isArray(task.dependencies) ? task.dependencies
      : String(task["depends on"] ?? "").split(/\s*,\s*/).filter((id) => id && !["none", "-"].includes(id)),
  }));
  const profile = project.setup.dependencies?.hosts?.[host];
  const capabilities = profile?.scope === scope ? Object.fromEntries((profile.receipts ?? []).map((receipt) => [receipt.id, receipt])) : {};
  const result = assessReadiness({ existing: {
    plan: { ...(project.setup.plan ?? {}), tasks }, tracker: canonical.tracker,
  }, capabilities });
  const initializationProblem = initializationRecordProblem(project.setup, canonical, {
    projectRoot: project.root,
    validateTracker: true,
    allowLegacy: true,
  });
  result.projectInitialization = { required: Boolean(initializationProblem), projectId: project.projectId, projectOwner: canonical.registry.projectOwner ?? null,
    ...(initializationProblem ? { reason: initializationProblem } : {}) };
  result.readyForDispatch = result.readyForDispatch && !result.projectInitialization.required;
  return result;
}

async function actualPath(candidate) {
  try { return await realpath(candidate); }
  catch (error) {
    if (error.code !== "ENOENT") throw error;
    const parent = path.dirname(candidate);
    if (parent === candidate) throw error;
    return path.join(await actualPath(parent), path.basename(candidate));
  }
}

async function preparationPaths(project, host, scope, home) {
  const root = await actualPath(scope === "user" ? path.resolve(home ?? os.homedir()) : project.root);
  const paths = { projectRoot: project.root, toolRoot: path.join(root, ".agent-team", "tools"), skillRoot: path.join(root, host === "codex" ? ".agents" : ".claude", "skills") };
  for (const candidate of [paths.toolRoot, paths.skillRoot]) {
    const resolved = await actualPath(candidate);
    if (!resolved.startsWith(`${root}${path.sep}`)) throw new Error(`Managed preparation path escapes selected scope: ${candidate}`);
  }
  return paths;
}

function dashboardChange(value) {
  fields(value, ["snapshot", "graph"], "dashboard");
  if (typeof value.snapshot !== "boolean") throw new Error("dashboard.snapshot must be boolean.");
  fields(value.graph, ["enabled", "termsAcknowledged", "executable"], "dashboard.graph");
  for (const key of ["enabled", "termsAcknowledged"]) if (typeof value.graph[key] !== "boolean") throw new Error(`dashboard.graph.${key} must be boolean.`);
  if (value.graph.executable !== undefined && !path.isAbsolute(string(value.graph.executable, "dashboard.graph.executable"))) throw new Error("dashboard.graph.executable must be an absolute path.");
  if (value.graph.enabled && !value.graph.termsAcknowledged) throw new Error("Graph enabling requires positive terms acknowledgement.");
  return value;
}

function validateOrchestrationInput(input) {
  fields(input, ["project", "host", "scope", "dependencies", "settings"], "setup input");
  for (const key of ["project", "host", "scope", "dependencies", "settings"]) {
    if (!Object.hasOwn(input, key)) throw new Error(`setup input.${key} is required.`);
  }
  if (!path.isAbsolute(string(input.project, "setup input.project"))) throw new Error("setup input.project must be absolute.");
  if (!["codex", "claude-code"].includes(input.host)) throw new Error("setup input.host must select codex or claude-code.");
  if (!["project", "user"].includes(input.scope)) throw new Error("setup input.scope must select project or user.");
  fields(input.settings, ["operationId"], "setup input.settings");
  if (Object.keys(input.settings).length !== 1) throw new Error("setup input.settings must contain only operationId.");
  identity(input.settings.operationId, "setup input.settings.operationId");
  const dependencies = fields(input.dependencies,
    input.dependencies?.action === "prepare" ? ["action", "expectedVersion", "operationId", "selections", "home"] : ["action"],
    "setup input.dependencies");
  if (!Object.hasOwn(dependencies, "action") || !["inspect", "prepare"].includes(dependencies.action)) {
    throw new Error("setup input.dependencies.action must select inspect or prepare.");
  }
  if (dependencies.action === "prepare") {
    if (!Number.isSafeInteger(dependencies.expectedVersion) || dependencies.expectedVersion < 0) throw new Error("setup input.dependencies.expectedVersion must be nonnegative.");
    identity(dependencies.operationId, "setup input.dependencies.operationId");
    if (!Object.hasOwn(dependencies, "selections")) throw new Error("setup input.dependencies.selections is required for prepare.");
    const selections = fields(dependencies.selections, ["defaults", "optionals", "declined"], "setup input.dependencies.selections");
    for (const [key, value] of Object.entries(selections)) {
      stringList(value, `setup input.dependencies.selections.${key}`);
      if (value.length > 64) throw new Error(`setup input.dependencies.selections.${key} is too large.`);
      for (const item of value) identity(item, `setup input.dependencies.selections.${key} item`);
    }
    if (dependencies.home !== undefined) {
      if (input.scope !== "user") throw new Error("setup input.dependencies.home is valid only for user scope.");
      if (!path.isAbsolute(string(dependencies.home, "setup input.dependencies.home"))) throw new Error("setup input.dependencies.home must be absolute.");
    }
  }
  return input;
}

async function qualifySetupOwner(projectPath, host, nativeIdentity, budget) {
  let project;
  let canonical;
  try {
    project = await resolveProject(path.resolve(projectPath), { budget });
    if (!project.active) return null;
    canonical = await loadCanonicalState(project, { includeTasks: false, budget });
  } catch {
    return null;
  }
  if (initializationRecordProblem(project.setup, canonical, { projectRoot: project.root, validateTracker: false, allowLegacy: true })) return null;
  const nativeHost = nativeIdentity?.host === "claude" ? "claude-code" : nativeIdentity?.host;
  if (nativeIdentity?.observed !== true || nativeHost !== host || typeof nativeIdentity?.sessionId !== "string"
    || !nativeIdentity.sessionId || typeof nativeIdentity?.cwd !== "string") return null;
  const epoch = Number.isSafeInteger(canonical.registry.ownershipEpoch) && canonical.registry.ownershipEpoch > 0
    ? canonical.registry.ownershipEpoch : 1;
  const derivedNativeIdentity = { host: nativeHost, sessionId: nativeIdentity.sessionId, observed: true,
    cwd: nativeIdentity.cwd, ownershipEpoch: epoch };
  if (!await validateNativeOwnerAuthority(project, derivedNativeIdentity, canonical.setup.ownership, nativeIdentity.sessionId)) return null;
  return {
    project, canonical,
    identity: { role: "project_coordinator", host: nativeHost, sessionId: nativeIdentity.sessionId, ownershipEpoch: epoch },
    writer: { id: nativeIdentity.sessionId, role: "project_orchestrator", host: nativeHost, ownershipEpoch: epoch },
  };
}

function orchestrationRegistryLoader(projectRoot, budget) {
  return async () => {
    const qualifiedProject = await resolveProject(projectRoot, { budget });
    if (!qualifiedProject.active || qualifiedProject.root !== projectRoot) return null;
    let canonical;
    try { canonical = await loadCanonicalState(qualifiedProject, { includeTasks: false, budget }); }
    catch { return null; }
    if (initializationRecordProblem(qualifiedProject.setup, canonical, {
      projectRoot: qualifiedProject.root, validateTracker: false, allowLegacy: true,
    })) {
      return null;
    }
    return canonical.registry;
  };
}

function normalizedInteraction(value) {
  if (value === undefined) return { kind: "keep_existing", interrupted: false };
  fields(value, value?.kind === "save" ? ["kind", "draft"] : ["kind"], "settings interaction outcome");
  if (value.kind === "save") {
    if (!Object.hasOwn(value, "draft")) throw new Error("settings interaction save requires a reviewed draft.");
    return { kind: "save", draft: value.draft, interrupted: false };
  }
  if (!["keep_existing", "cancel", "back", "no_answer", "timeout", "interrupted"].includes(value.kind)) {
    throw new Error(`Unknown settings interaction outcome: ${value.kind}.`);
  }
  return { kind: "keep_existing", interrupted: value.kind === "interrupted" };
}

function modelRoutingAdvisory(host, nativeChoices) {
  if (nativeChoices?.observed !== true || nativeChoices.host !== host) {
    return { status: "unknown", host, reason: "native_model_inventory_unobserved" };
  }
  const availableModels = [...new Set((Array.isArray(nativeChoices.models) ? nativeChoices.models : [])
    .filter(({ id, available }) => typeof id === "string" && id.trim() && available === true)
    .map(({ id }) => id))];
  if (!availableModels.length) return { status: "unknown", host, reason: "no_available_models_observed" };
  if (host === "codex") return { status: "observed", host, guidance: "explicit_available_model_selection", availableModels };
  const recommendedModels = availableModels.filter((id) => {
    const model = nativeChoices.models.find((candidate) => candidate.id === id);
    return /(^|[-_\s])opus($|[-_\s])/i.test(`${model.id} ${model.label ?? ""}`);
  });
  return recommendedModels.length
    ? { status: "observed", host, guidance: "prefer_opus_for_execution", availableModels, recommendedModels }
    : { status: "unknown", host, reason: "available_opus_model_unobserved", availableModels };
}

/** Build the setup result without reading or mutating project state. */
export function buildSetupSummary({ project, dependencies, readiness, settings, settingsOutcome, nativeIdentity }) {
  return {
    status: readiness?.readyForDispatch === true ? "ready" : "incomplete",
    project: { id: project?.projectId, root: project?.root },
    nativeIdentity: {
      role: nativeIdentity?.role, host: nativeIdentity?.host, sessionId: nativeIdentity?.sessionId,
      ownershipEpoch: nativeIdentity?.ownershipEpoch,
    },
    dependencies: structuredClone(dependencies),
    settings: structuredClone(settings),
    settingsOutcome,
    readiness: structuredClone(readiness),
  };
}

/** Run state-changing setup only from a native in-process owner context. */
export async function orchestrateSetup(input, context = {}) {
  validateOrchestrationInput(input);
  let qualified = await qualifySetupOwner(input.project, input.host, context.nativeIdentity, context.budget);
  if (!qualified) return { status: "conflict", reason: "native_project_context_required" };
  if (typeof context.interactSettings !== "function") throw new Error("A native settings interaction is required.");
  let dependencies = inspectDependencies({ setup: qualified.project.setup, host: input.host });
  if (dependencies.scope !== "unknown" && dependencies.scope !== input.scope) dependencies = {
    status: "unavailable", reason: "dependency_scope_mismatch", host: input.host,
    scope: input.scope, recordedScope: dependencies.scope,
  };
  if (input.dependencies.action === "prepare") {
    const paths = await preparationPaths(qualified.project, input.host, input.scope, input.dependencies.home);
    const runner = (context.createDependencyRunner ?? createDependencyRunner)({ host: input.host, scope: input.scope, paths,
      workerDiscovery: context.workerDiscovery });
    dependencies = await prepareDependencies({
      setupPath: qualified.project.paths.setup, expectedVersion: input.dependencies.expectedVersion,
      writer: qualified.writer, operationId: input.dependencies.operationId,
      loadRegistry: orchestrationRegistryLoader(qualified.project.root, context.budget), host: input.host, scope: input.scope,
      selections: { ...(input.dependencies.selections ?? {}), managedPaths: paths }, paths, runner, budget: context.budget,
    });
    if (["conflict", "failed"].includes(dependencies.status)) return dependencies;
  }
  qualified = await qualifySetupOwner(input.project, input.host, context.nativeIdentity, context.budget);
  if (!qualified) return { status: "conflict", reason: "native_project_context_required" };
  const overview = inspectSettings({ setup: qualified.project.setup, host: input.host, nativeChoices: context.nativeChoices ?? {} });
  const modelRouting = modelRoutingAdvisory(input.host, context.nativeChoices ?? {});
  const wizard = { ...buildSettingsWizard({ setup: qualified.project.setup, host: input.host, nativeChoices: context.nativeChoices ?? {} }),
    modelRouting };
  const settingsCheckpoint = await readFile(qualified.project.paths.setup);
  let interaction;
  try {
    interaction = normalizedInteraction(await context.interactSettings({ overview: structuredClone(overview), wizard: structuredClone(wizard) }));
  } catch (error) {
    if (error?.code !== "SETUP_INTERACTION_INTERRUPTED") throw error;
    interaction = { kind: "keep_existing", interrupted: true };
  }
  let settingsOutcome = "kept_existing";
  if (interaction.kind === "save") {
    const result = await saveSettingsDraft({
      setupPath: qualified.project.paths.setup, host: input.host, expectedVersion: qualified.project.setup.version ?? 0,
      writer: qualified.writer, operationId: input.settings.operationId,
      loadRegistry: orchestrationRegistryLoader(qualified.project.root, context.budget), draft: interaction.draft,
      nativeChoices: context.nativeChoices ?? {}, budget: context.budget,
    });
    if (result.status === "applied" || result.status === "duplicate") settingsOutcome = "saved";
    else if (result.status !== "kept_existing") return result;
  } else if (!settingsCheckpoint.equals(await readFile(qualified.project.paths.setup))) {
    throw new Error("setup changed during non-consenting settings interaction");
  }
  qualified = await qualifySetupOwner(input.project, input.host, context.nativeIdentity, context.budget);
  if (!qualified) return { status: "conflict", reason: "native_project_context_required" };
  const readiness = await canonicalReadiness(qualified.project, input.host, input.scope, context);
  const settings = inspectSettings({ setup: qualified.project.setup, host: input.host, nativeChoices: context.nativeChoices ?? {} });
  const summary = { ...buildSetupSummary({ project: qualified.project, dependencies, readiness, settings, settingsOutcome,
    nativeIdentity: qualified.identity }), modelRouting };
  return interaction.interrupted ? { ...summary, interrupted: true } : summary;
}

async function contextOffer(project, host, context, request = {}) {
  fields(request, ["ownerReportedCapabilities"], "request");
  const reported = ownerReportedCapabilities(request.ownerReportedCapabilities, host);
  const inventory = capabilityInventory(context.capabilityInventory, host);
  const inspection = await inspectContextReduction({ projectRoot: project.root, home: context.contextHome, host });
  if (reported) {
    const proposal = proposeContextReduction({ host,
      selectedProfile: canonicalCapabilityProfile(project, host, reported.capabilities), visible: reported.capabilities });
    return { status: "offered_unverified", visibility: "unknown", automaticDetection: "unavailable", inspection, proposal };
  }
  if (!inventory) return { status: "visibility_unknown", inspection, proposal: null };
  const proposal = proposeContextReduction({ host,
    selectedProfile: canonicalCapabilityProfile(project, host, inventory.capabilities), visible: inventory.capabilities });
  return { status: "offered", visibility: "host_observed", inspection, proposal };
}

function setupReceiptRecorder(common, slot) {
  return async (receipt) => {
    const result = await mutateSetup({ ...common, operation: { kind: `${slot}-receipt`, receipt }, mutate: async (setup) => {
      if (isDeepStrictEqual(setup[slot], receipt)) return { write: false, result: { status: "already_recorded" } };
      setup[slot] = structuredClone(receipt);
      return { setup };
    } });
    if (result.status === "already_recorded") return { status: "committed", receiptDigest: receipt.digest };
    if (["applied", "duplicate"].includes(result.status)) {
      const recorded = result.setup ?? JSON.parse(await readFile(common.setupPath, "utf8"));
      if (!isDeepStrictEqual(recorded[slot], receipt)) throw new Error("canonical setup receipt changed after commit");
      return { status: "committed", receiptDigest: receipt.digest };
    }
    if (result.status === "conflict") return { status: "not_committed", receiptDigest: receipt.digest };
    throw new Error(`canonical setup receipt result is unknown: ${result.status ?? "missing"}`);
  };
}

function absoluteOptional(value, label) {
  if (value === undefined) return undefined;
  const checked = string(value, label);
  if (!path.isAbsolute(checked) || path.normalize(checked) !== checked) throw new Error(`${label} must be absolute.`);
  return checked;
}

/** JSON is user intent; native choices, runners, and worker discovery enter only through trusted caller context. */
export async function runSetupCommand(command, options = {}, context = {}) {
  const allowed = setupCommandFlags[command];
  if (!allowed) throw new Error(`Unknown setup command: ${command ?? "missing"}.`);
  object(options, "options");
  for (const key of Object.keys(options)) {
    if (key === "archive" && Array.isArray(options.archive) && options.archive.length === 0) continue; // Shared CLI parser default.
    if (!allowed.has(key)) throw new Error(`Unsupported flag for ${command}: --${key}.`);
    string(options[key], `--${key}`);
  }
  if (!["codex", "claude-code"].includes(options.host)) throw new Error("--host must explicitly select codex or claude-code.");
  if (!["user", "project"].includes(options.scope)) throw new Error("--scope must explicitly select user or project.");
  if (options.home && options.scope !== "user") throw new Error("--home is ineffective with project scope.");
  if (options.scope === "user" && !["dependencies", "dependencies-prepare", "readiness"].includes(command)) throw new Error(`${command} supports project scope only.`);
  const project = await resolveProject(path.resolve(string(options.project, "--project")), { budget: context.budget });
  const envelope = options.request ? await requestEnvelope(options.request) : null;
  if (mutations.has(command) && !envelope) throw new Error("--request is required for setup mutations.");
  if (!project.active && command !== "readiness") throw new Error(`Active Agent-Team project required: ${project.reason}.`);
  const nativeChoices = context.nativeChoices ?? {};
  if (command === "settings") {
    const overview = inspectSettings({ setup: project.setup, host: options.host, nativeChoices });
    return options.role ? buildRoleMenu({ overview, role: options.role, nativeChoices }) : overview;
  }
  if (command === "settings-wizard") {
    const request = fields(envelope?.request ?? {}, ["draft"], "request");
    return { ...buildSettingsWizard({ setup: project.setup, host: options.host, nativeChoices, draft: validateDraft(request.draft ?? {}) }),
      modelRouting: modelRoutingAdvisory(options.host, nativeChoices) };
  }
  if (command === "dependencies") {
    const result = inspectDependencies({ setup: project.setup, host: options.host });
    if (result.scope !== "unknown" && result.scope !== options.scope) return {
      status: "unavailable", reason: "dependency_scope_mismatch", host: options.host,
      scope: options.scope, recordedScope: result.scope,
    };
    return result;
  }
  if (command === "helpers") return inspectHelpers({ projectRoot: project.root });
  if (command === "context-reduction") return contextOffer(project, options.host, context, envelope?.request ?? {});
  if (command === "readiness") {
    if (project.active) {
      if (envelope) throw new Error("Active project readiness uses canonical records; --request is ineffective.");
      return canonicalReadiness(project, options.host, options.scope, context);
    }
    const request = fields(envelope?.request ?? {}, ["kickoff", "existing", "request", "project"], "request");
    const result = assessReadiness({ ...request, capabilities: {} });
    return { ...result, readyForDispatch: false, projectInitialization: { ...result.projectInitialization, required: true, projectRoot: project.root } };
  }
  const identity = mutationIdentity(envelope);
  const nativeHost = context.nativeIdentity?.host === "claude" ? "claude-code" : context.nativeIdentity?.host;
  if (nativeHost !== options.host || !await validateNativeOwnerAuthority(project, context.nativeIdentity,
    project.setup.ownership, context.nativeIdentity?.sessionId)) return { status: "conflict", reason: "native_project_context_required" };
  identity.writer = { id: context.nativeIdentity.sessionId, role: "project_orchestrator", host: options.host,
    ...(project.setup.ownership?.epoch !== undefined ? { ownershipEpoch: project.setup.ownership.epoch } : {}) };
  const common = {
    setupPath: project.paths.setup, ...identity, budget: context.budget,
    loadRegistry: async () => {
      const fresh = await resolveProject(project.cwd ?? project.root, { budget: context.budget });
      if (!fresh.active || fresh.root !== project.root || fresh.paths.setup !== project.paths.setup || fresh.projectId !== project.projectId) {
        return null;
      }
      let canonical;
      try { canonical = await loadCanonicalState(fresh, { includeTasks: false, budget: context.budget }); }
      catch { return null; }
      return initializationRecordProblem(fresh.setup, canonical, {
        projectRoot: fresh.root,
        validateTracker: false,
        allowLegacy: true,
      }) ? null : canonical.registry;
    },
  };
  if (command === "helpers-install") {
    fields(envelope.request, [], "request");
    const recordSetupReceipt = setupReceiptRecorder(common, "helpers");
    const result = await installHelpers({ projectRoot: project.root, recordSetupReceipt });
    if (!result.changed && result.setupReceipt && !isDeepStrictEqual(project.setup.helpers, result.setupReceipt)) {
      const outcome = await recordSetupReceipt(result.setupReceipt);
      if (outcome.status !== "committed") throw new Error("canonical helper receipt was not committed");
    }
    return result;
  }
  if (command === "helpers-recover") {
    fields(envelope.request, ["expectedJournalDigest"], "request");
    string(envelope.request.expectedJournalDigest, "request.expectedJournalDigest");
    return recoverHelperTransaction({ projectRoot: project.root, expectedJournalDigest: envelope.request.expectedJournalDigest,
      recordSetupReceipt: setupReceiptRecorder(common, "helpers") });
  }
  if (command === "context-reduction-apply") {
    fields(envelope.request, ["proposal", "decision", "ownerReportedCapabilities"], "request");
    const decision = fields(envelope.request.decision, envelope.request.decision?.action === "apply"
      ? ["action", "reviewed", "proposalId", "selected", "visibilityAcknowledged"] : ["action"], "request.decision");
    if (["no_answer", "cancel"].includes(decision.action)) {
      return applyContextReduction({ decision });
    }
    const reported = ownerReportedCapabilities(envelope.request.ownerReportedCapabilities, options.host);
    const inventory = capabilityInventory(context.capabilityInventory, options.host);
    if (reported && decision.visibilityAcknowledged !== true) {
      return { status: "conflict", reason: "visibility_acknowledgement_required", changed: false };
    }
    if (!reported && !inventory) return { status: "conflict", reason: "visibility_unknown", changed: false };
    const capabilities = reported?.capabilities ?? inventory.capabilities;
    const currentProposal = proposeContextReduction({ host: options.host,
      selectedProfile: canonicalCapabilityProfile(project, options.host, capabilities), visible: capabilities });
    if (!isDeepStrictEqual(envelope.request.proposal, currentProposal)) {
      return { status: "conflict", reason: "proposal_stale", changed: false };
    }
    const home = absoluteOptional(context.contextHome, "trusted context home");
    if (options.host === "claude-code" && home === undefined) return { status: "conflict", reason: "trusted_home_unavailable", changed: false };
    const inspection = await inspectContextReduction({ projectRoot: project.root, home, host: options.host });
    const binding = contextReceiptBinding({ canonicalReceipt: project.setup.contextReduction,
      localReceipt: inspection.receipt, projectRoot: project.root, home, host: options.host });
    if (binding.status === "mismatch") return { status: "conflict", reason: "canonical_receipt_mismatch", changed: false };
    if (binding.appliedReceipt) return { status: "conflict", reason: "context_reduction_already_applied", changed: false };
    const applyDecision = { action: decision.action, reviewed: decision.reviewed,
      proposalId: decision.proposalId, selected: decision.selected };
    return applyContextReduction({ projectRoot: project.root, home, proposal: currentProposal, decision: applyDecision,
      recordSetupReceipt: setupReceiptRecorder(common, "contextReduction") });
  }
  if (command === "context-reduction-revert") {
    fields(envelope.request, ["expectedReceiptDigest"], "request");
    string(envelope.request.expectedReceiptDigest, "request.expectedReceiptDigest");
    const home = absoluteOptional(context.contextHome, "trusted context home");
    const inspection = await inspectContextReduction({ projectRoot: project.root, home, host: options.host });
    const binding = contextReceiptBinding({ canonicalReceipt: project.setup.contextReduction,
      localReceipt: inspection.receipt, projectRoot: project.root, home, host: options.host });
    if (binding.status === "empty") return { status: "not_applied", changed: false };
    if (binding.status !== "linked" || !binding.appliedReceipt
      || binding.appliedReceipt.digest !== envelope.request.expectedReceiptDigest) {
      return { status: "conflict", reason: "canonical_receipt_mismatch", changed: false };
    }
    return revertContextReduction({ projectRoot: project.root, home, host: options.host,
      expectedReceiptDigest: envelope.request.expectedReceiptDigest,
      recordSetupReceipt: setupReceiptRecorder(common, "contextReduction") });
  }
  if (command === "context-reduction-recover") {
    fields(envelope.request, ["expectedJournalDigest"], "request");
    string(envelope.request.expectedJournalDigest, "request.expectedJournalDigest");
    const home = absoluteOptional(context.contextHome, "trusted context home");
    return recoverContextReductionTransaction({ projectRoot: project.root, home, host: options.host,
      expectedJournalDigest: envelope.request.expectedJournalDigest,
      recordSetupReceipt: setupReceiptRecorder(common, "contextReduction") });
  }
  if (command === "settings-update") {
    fields(envelope.request, ["change"], "request");
    return updateSettings({ ...common, host: options.host, change: validateChange(envelope.request.change), nativeChoices });
  }
  if (command === "dependencies-prepare") {
    fields(envelope.request, ["selections"], "request");
    const selections = fields(envelope.request.selections ?? {}, ["defaults", "optionals", "declined"], "selections");
    for (const [key, value] of Object.entries(selections)) stringList(value, `selections.${key}`);
    const paths = await preparationPaths(project, options.host, options.scope, options.home);
    const runner = (context.createDependencyRunner ?? createDependencyRunner)({ host: options.host, scope: options.scope, paths, workerDiscovery: context.workerDiscovery });
    return prepareDependencies({ ...common, host: options.host, scope: options.scope, paths, runner,
      // Bind the resolved, non-user-supplied paths into the accepted semantic operation signature.
      selections: { ...selections, managedPaths: paths },
    });
  }
  fields(envelope.request, ["dashboard"], "request");
  const dashboard = dashboardChange(envelope.request.dashboard);
  return mutateSetup({ ...common, operation: { kind: "dashboard-configure", dashboard }, mutate: async (setup) => {
    if (dashboard.graph.enabled && resolveTracker(project.root, setup.tracker).kind !== "beads") throw new Error("Graph enabling requires the selected Beads tracker.");
    setup.dashboard = { ...setup.dashboard, ...dashboard, graph: { ...setup.dashboard?.graph, ...dashboard.graph } };
    return { setup };
  } });
}
