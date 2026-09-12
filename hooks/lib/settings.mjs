import { createHash, randomUUID } from "node:crypto";
import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";
import { ROLE_DEFINITIONS } from "./dependency-profiles.mjs";
import { withDirectoryLock } from "./lock.mjs";
import { assertNoOwnerRecoveryJournal, repairOwnerRecovery } from "./owner-recovery.mjs";

const HOSTS = new Set(["codex", "claude-code"]);

function configuredRole(setup, host, id) {
  const override = setup?.settings?.hosts?.[host]?.roles?.[id];
  if (override) return { value: override, source: override.source ?? "override" };
  const legacy = setup?.harness === host ? setup?.roleRouting?.[id] : undefined;
  return legacy ? { value: legacy, source: "legacy" } : null;
}

function choiceFor(nativeChoices, id) {
  return nativeChoices?.models?.find((choice) => choice.id === id);
}

export function inspectSettings({ setup = {}, host, nativeChoices = {} }) {
  if (!HOSTS.has(host)) throw new Error(`Unknown settings host: ${host ?? "missing"}.`);
  const defaults = setup?.settings?.profiles?.[setup?.settings?.profile]?.roles ?? {};
  const roles = ROLE_DEFINITIONS.map((definition) => {
    const route = configuredRole(setup, host, definition.id);
    const profileDefault = defaults[definition.id];
    const configured = route?.value ?? profileDefault ?? { model: null, effort: null };
    const nativeDefault = nativeChoices.models?.find(({ recommended }) => recommended) ?? nativeChoices.models?.[0];
    const effective = configured.model ? configured : {
      model: nativeDefault?.id ?? null,
      effort: nativeDefault?.recommendedEffort ?? nativeDefault?.efforts?.[0] ?? null,
    };
    const modelChoice = choiceFor(nativeChoices, effective.model);
    const effortSupported = modelChoice?.efforts?.includes(effective.effort) ?? false;
    return {
      ...definition,
      configured: { model: configured.model ?? null, effort: configured.effort ?? null },
      effective: { model: effective.model ?? null, effort: effective.effort ?? null },
      source: route?.source ?? (profileDefault ? "profile_default" : "host_default"),
      model: {
        supported: Boolean(modelChoice),
        available: modelChoice?.available ?? "unknown",
      },
      effort: {
        supported: effortSupported,
        available: modelChoice ? (effortSupported && modelChoice.available !== false) : "unknown",
      },
      enforceable: nativeChoices.enforceable ?? "unknown",
    };
  });
  return {
    host,
    profile: setup?.settings?.profile ?? "quality",
    control: nativeChoices.control ?? "numbered",
    runDefaults: structuredClone(setup?.settings?.runDefaults ?? {}),
    roles,
  };
}

export async function inspectSettingsFile({ setupPath, host, nativeChoices }) {
  const setup = JSON.parse(await readFile(setupPath, "utf8"));
  return inspectSettings({ setup, host, nativeChoices });
}

function title(value) {
  return value ? `${value[0].toUpperCase()}${value.slice(1)}` : value;
}

export function buildRoleMenu({ overview, role, nativeChoices = {} }) {
  const current = overview.roles.find(({ id }) => id === role);
  if (!current) throw new Error(`Unknown role: ${role}.`);
  const models = (nativeChoices.models ?? []).map((choice, index) => ({
    number: index + 1,
    id: choice.id,
    label: choice.label ?? choice.id,
    description: choice.description ?? "",
    available: choice.available ?? "unknown",
    markers: [
      ...(choice.id === current.configured.model ? ["current"] : []),
      ...(choice.recommended ? ["recommended"] : []),
    ],
  }));
  const selected = nativeChoices.models?.find(({ id }) => id === current.configured.model);
  return {
    control: nativeChoices.control ?? "numbered",
    role: { id: current.id, name: current.name, purpose: current.purpose },
    models,
    efforts: (selected?.efforts ?? []).map((id, index) => ({
      number: index + 1,
      id,
      label: title(id),
      markers: id === current.configured.effort ? ["current"] : [],
    })),
  };
}

function navigationChoices(values, includeBack = true) {
  return [
    ...values,
    ...(includeBack ? [{ id: "back", label: "Back" }] : []),
    { id: "cancel", label: "Cancel" },
  ];
}

export function buildSettingsWizard({ setup = {}, host, nativeChoices = {}, draft = {} }) {
  const overview = inspectSettings({ setup, host, nativeChoices });
  const runSettings = ["parallel_teams", "continuous", "auto_deploy", "deploy_batch_tasks"];
  const steps = runSettings.map((setting, index) => ({
    kind: "run",
    setting,
    current: overview.runDefaults[setting] ?? null,
    choices: navigationChoices([{ id: "set", label: "Set value" }], index > 0),
  }));
  for (const role of ROLE_DEFINITIONS) {
    const menu = buildRoleMenu({ overview, role: role.id, nativeChoices });
    const current = overview.roles.find(({ id }) => id === role.id);
    const draftRoute = draft.roles?.[role.id] ?? {};
    const selectedModel = draftRoute.model ?? current.configured.model ?? current.effective.model;
    const selectedEffort = draftRoute.effort ?? (selectedModel === current.configured.model ? current.configured.effort : null);
    const models = menu.models.map((choice) => ({
      ...choice,
      markers: [...choice.markers.filter((marker) => marker !== "current"), ...(choice.id === selectedModel ? ["current"] : [])],
    }));
    steps.push({ kind: "role-model", role: role.id, name: role.name, choices: navigationChoices(models) });
    const efforts = (nativeChoices.models?.find(({ id }) => id === selectedModel)?.efforts ?? [])
      .map((id, index) => ({ id, number: index + 1, label: title(id) }));
    steps.push({
      kind: "role-effort", role: role.id, name: role.name, model: selectedModel,
      choices: navigationChoices(efforts.map((choice) => ({ ...choice, markers: choice.id === selectedEffort ? ["current"] : [] }))),
    });
  }
  steps.push({ kind: "review", choices: navigationChoices([{ id: "save", label: "Save changes" }]) });
  return { host, control: overview.control, steps };
}

function validWriter(writer) {
  return writer && typeof writer.id === "string" && writer.id && typeof writer.role === "string" && writer.role;
}

function stable(value) {
  if (Array.isArray(value)) return `[${value.map(stable).join(",")}]`;
  if (value && typeof value === "object") return `{${Object.keys(value).sort().map((key) => `${JSON.stringify(key)}:${stable(value[key])}`).join(",")}}`;
  return JSON.stringify(value);
}

function signature(operation) {
  return createHash("sha256").update(stable(operation)).digest("hex");
}

async function atomicJson(file, value, budget) {
  budget?.check();
  await mkdir(path.dirname(file), { recursive: true, mode: 0o700 });
  const temporary = `${file}.${process.pid}.${randomUUID()}.tmp`;
  try {
    budget?.check();
    await writeFile(temporary, `${JSON.stringify(value, null, 2)}\n`, { mode: 0o600, signal: budget?.signal });
    budget?.check();
    await rename(temporary, file);
  } catch (error) {
    budget?.check();
    throw error;
  } finally { await rm(temporary, { force: true }); }
}

function registryOwner(registry) {
  return typeof registry?.projectOwner === "string" ? registry.projectOwner : registry?.projectOwner?.id;
}

/** Serialize setup.json changes under one project-owner/version/idempotency contract. */
export async function mutateSetup({ setupPath, expectedVersion, writer, operationId, operation, loadRegistry, mutate, budget }) {
  if (!validWriter(writer)) throw new Error("A setup writer identity and role are required.");
  if (typeof operationId !== "string" || !operationId) throw new Error("A setup operationId is required.");
  if (typeof loadRegistry !== "function") throw new Error("A setup registry loader is required.");
  const operationSignature = signature(operation);
  const stateRoot = path.dirname(setupPath);
  const locks = path.join(stateRoot, ".locks");
  const project = { root: path.dirname(stateRoot), paths: { stateRoot, setup: setupPath, state: path.join(stateRoot, "state.json"),
    teams: path.join(stateRoot, "TEAMS.md"), ownerHistory: path.join(stateRoot, "owner-history.json"),
    ownerRecoveryJournal: path.join(stateRoot, ".owner-recovery.json"), ownerRecoveryLock: path.join(locks, "owner-recovery.lock"), locks } };
  await repairOwnerRecovery(project, { budget });
  const lockPath = path.join(locks, "setup.lock");
  return withDirectoryLock(lockPath, { operation: operation?.kind ?? "setup", operationId, writer }, async () => {
    await assertNoOwnerRecoveryJournal(project);
    const setup = JSON.parse(await readFile(setupPath, "utf8"));
    const registry = await loadRegistry();
    const owner = registryOwner(registry);
    const ownerHost = typeof registry?.projectOwnerHost === "string" ? registry.projectOwnerHost : undefined;
    const writerHost = writer.host === "claude" ? "claude-code" : writer.host;
    if (!owner || owner !== writer.id || ownerHost && writerHost !== ownerHost
      || setup.ownership?.epoch !== undefined && writer.ownershipEpoch !== setup.ownership.epoch
      || writer.role !== "project_orchestrator") {
      return { status: "conflict", reason: "project_owner_required" };
    }
    const previous = setup.setupOperations?.find(({ id }) => id === operationId);
    if (previous) {
      return previous.signature === operationSignature
        ? { status: "duplicate", version: previous.version, operationId }
        : { status: "conflict", reason: "operation_id_reused", operationId };
    }
    const actualVersion = Number.isInteger(setup.version) ? setup.version : 0;
    if (actualVersion !== expectedVersion) return { status: "conflict", reason: "version_changed", expectedVersion, actualVersion };
    const outcome = await mutate(structuredClone(setup));
    budget?.check();
    const next = outcome.setup;
    next.version = actualVersion + 1;
    next.setupOperations = [
      ...(setup.setupOperations ?? []),
      { id: operationId, signature: operationSignature, kind: operation?.kind ?? "setup", writer: writer.id, version: next.version },
    ].slice(-50);
    await atomicJson(setupPath, next, budget);
    return { status: "applied", version: next.version, setup: next, ...(outcome.result ?? {}) };
  }, { budget });
}

function applyChange(setup, host, change, nativeChoices) {
  const next = structuredClone(setup);
  if (next.harness !== undefined && !HOSTS.has(next.harness)) throw new Error(`Unknown legacy settings host: ${next.harness}.`);
  next.settings ??= {};
  if (change.kind === "run") {
    const validators = {
      teams: (value) => Number.isInteger(value) && value >= 1 && value <= 6,
      parallel_teams: (value) => Number.isInteger(value) && value >= 1 && value <= 6,
      continuous: (value) => typeof value === "boolean",
      auto_deploy: (value) => typeof value === "boolean",
      deploy_batch_tasks: (value) => value === null || (Number.isInteger(value) && value > 0),
    };
    for (const [setting, value] of Object.entries(change.values ?? {})) {
      if (!validators[setting] || !validators[setting](value)) throw new Error(`Invalid run setting ${setting}.`);
    }
    next.settings.runDefaults = { ...(next.settings.runDefaults ?? {}), ...(change.values ?? {}) };
    return next;
  }
  if (change.kind === "fallback") {
    if (!ROLE_DEFINITIONS.some(({ id }) => id === change.role)) throw new Error(`Unknown role: ${change.role}.`);
    if (!Array.isArray(change.routes)) throw new Error("Fallback routes must be an array.");
    for (const route of change.routes) {
      const model = nativeChoices.models?.find(({ id }) => id === route.model);
      if (!model) throw new Error(`Unsupported fallback model for ${host}: ${route.model}.`);
      if (!model.efforts?.includes(route.effort)) throw new Error(`Unsupported fallback effort for ${route.model}: ${route.effort}.`);
    }
    next.settings.hosts ??= {};
    next.settings.hosts[host] ??= {};
    next.settings.hosts[host].fallbacks ??= {};
    next.settings.hosts[host].fallbacks[change.role] = {
      routes: structuredClone(change.routes),
      escalation: change.escalation === true,
      source: "approved",
    };
    return next;
  }
  if (change.kind !== "role") throw new Error(`Unknown settings change: ${change.kind}.`);
  if (!ROLE_DEFINITIONS.some(({ id }) => id === change.role)) throw new Error(`Unknown role: ${change.role}.`);
  const model = nativeChoices?.models?.find(({ id }) => id === change.model);
  if (!model) throw new Error(`Unsupported model for ${host}: ${change.model}.`);
  if (!model.efforts?.includes(change.effort)) throw new Error(`Unsupported effort for ${change.model}: ${change.effort}.`);
  next.settings.hosts ??= {};
  next.settings.hosts[host] ??= {};
  next.settings.hosts[host].roles ??= {};
  next.settings.hosts[host].roles[change.role] = {
    ...(next.settings.hosts[host].roles[change.role] ?? {}),
    model: change.model,
    effort: change.effort,
    source: "override",
  };
  return next;
}

export async function updateSettings({ setupPath, host, expectedVersion, writer, operationId, loadRegistry, change, nativeChoices = {}, budget }) {
  if (!HOSTS.has(host)) throw new Error(`Unknown settings host: ${host ?? "missing"}.`);
  if (!validWriter(writer)) throw new Error("A settings writer identity and role are required.");
  if (["cancel", "back"].includes(change?.kind)) return { status: change.kind };
  return mutateSetup({
    setupPath, expectedVersion, writer, operationId, loadRegistry, budget,
    operation: { kind: "settings", host, change },
    mutate: async (setup) => {
      const next = applyChange(setup, host, change, nativeChoices);
      return { setup: next };
    },
  });
}
