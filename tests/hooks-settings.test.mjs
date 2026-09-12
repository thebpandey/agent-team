import assert from "node:assert/strict";
import { mkdtemp, readFile, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

test("settings overview reports configured and enforceable routing for every role", async () => {
  const { inspectSettings } = await import("../hooks/lib/settings.mjs").catch(() => ({}));
  const setup = {
    version: 4,
    settings: {
      profile: "quality",
      runDefaults: { teams: 2, continuous: false },
      hosts: {
        codex: {
          roles: {
            developer: { model: "gpt-dev", effort: "high" },
          },
        },
      },
    },
  };
  const nativeChoices = {
    control: "native",
    enforceable: true,
    models: [{ id: "gpt-dev", label: "GPT Dev", available: true, efforts: ["medium", "high"] }],
  };

  const overview = inspectSettings?.({ setup, host: "codex", nativeChoices });

  assert.equal(overview.host, "codex");
  assert.equal(overview.control, "native");
  assert.equal(overview.roles.length, 6);
  assert.deepEqual(overview.roles.find(({ id }) => id === "developer"), {
    id: "developer",
    name: "Trinity",
    purpose: "Standard development and UI/UX development",
    configured: { model: "gpt-dev", effort: "high" },
    effective: { model: "gpt-dev", effort: "high" },
    source: "override",
    model: { supported: true, available: true },
    effort: { supported: true, available: true },
    enforceable: true,
  });
});

test("settings file inspection is byte-identical and returns friendly numbered choices", async () => {
  const { inspectSettingsFile, buildRoleMenu } = await import("../hooks/lib/settings.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-settings-"));
  const setupPath = path.join(directory, "setup.json");
  const source = '{\n  "version": 2,\n  "settings": {"profile":"quality","hosts":{"codex":{"roles":{"developer":{"model":"gpt-dev","effort":"high"}}}}}\n}\n';
  await writeFile(setupPath, source);
  const nativeChoices = {
    control: "numbered",
    enforceable: false,
    models: [
      { id: "gpt-fast", label: "GPT Fast", description: "Lower latency", available: true, efforts: ["medium"] },
      { id: "gpt-dev", label: "GPT Dev", description: "Best coding quality", available: true, recommended: true, efforts: ["medium", "high"] },
    ],
  };

  const overview = await inspectSettingsFile({ setupPath, host: "codex", nativeChoices });
  const menu = buildRoleMenu({ overview, role: "developer", nativeChoices });

  assert.equal(await readFile(setupPath, "utf8"), source);
  assert.equal(overview.roles.find(({ id }) => id === "developer").enforceable, false);
  assert.deepEqual(menu.models.map(({ number, id, markers }) => ({ number, id, markers })), [
    { number: 1, id: "gpt-fast", markers: [] },
    { number: 2, id: "gpt-dev", markers: ["current", "recommended"] },
  ]);
  assert.equal(menu.efforts.find(({ id }) => id === "high").label, "High");
});

test("targeted role edit preserves the other host, run defaults, active routing, and custom settings", async () => {
  const { updateSettings } = await import("../hooks/lib/settings.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-settings-"));
  const setupPath = path.join(directory, "setup.json");
  const setup = {
    skill: "agent-team",
    projectId: "project-1",
    version: 7,
    tracker: { kind: "beads" },
    vendorExtension: { keep: true },
    settings: {
      profile: "quality",
      runDefaults: { teams: 3, continuous: true },
      hosts: {
        codex: { custom: "keep", roles: { developer: { model: "gpt-old", effort: "medium" } } },
        "claude-code": { roles: { developer: { model: "claude-opus", effort: "high" } } },
      },
    },
    activeRun: { routing: { developer: { model: "gpt-old", effort: "medium" } } },
  };
  await writeFile(setupPath, `${JSON.stringify(setup, null, 2)}\n`);
  const nativeChoices = {
    models: [{ id: "gpt-dev", label: "GPT Dev", available: true, efforts: ["medium", "high"] }],
  };

  const result = await updateSettings({
    setupPath,
    host: "codex",
    expectedVersion: 7,
    writer: { id: "settings-session", role: "project_orchestrator" },
    operationId: "settings-edit-1",
    loadRegistry: async () => ({ projectOwner: "settings-session" }),
    change: { kind: "role", role: "developer", model: "gpt-dev", effort: "high" },
    nativeChoices,
  });
  const saved = JSON.parse(await readFile(setupPath, "utf8"));

  assert.equal(result.status, "applied");
  assert.equal(saved.version, 8);
  assert.deepEqual(saved.settings.hosts.codex.roles.developer, { model: "gpt-dev", effort: "high", source: "override" });
  assert.deepEqual(saved.settings.hosts["claude-code"], setup.settings.hosts["claude-code"]);
  assert.deepEqual(saved.settings.runDefaults, setup.settings.runDefaults);
  assert.deepEqual(saved.activeRun, setup.activeRun);
  assert.deepEqual(saved.vendorExtension, { keep: true });
  assert.equal(saved.settings.hosts.codex.custom, "keep");
});

test("cancel and back do not write settings", async () => {
  const { updateSettings } = await import("../hooks/lib/settings.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-settings-"));
  const setupPath = path.join(directory, "setup.json");
  const source = '{"skill":"agent-team","projectId":"p","version":1,"settings":{"custom":true}}\n';
  await writeFile(setupPath, source);

  for (const action of ["cancel", "back"]) {
    const result = await updateSettings({ setupPath, host: "codex", expectedVersion: 1, writer: { id: "w", role: "project_orchestrator" }, operationId: `${action}-1`, loadRegistry: async () => ({ projectOwner: "w" }), change: { kind: action } });
    assert.equal(result.status, action);
    assert.equal(await readFile(setupPath, "utf8"), source);
  }
});

test("stale settings writer conflicts instead of overwriting a newer version", async () => {
  const { updateSettings } = await import("../hooks/lib/settings.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-settings-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, '{"skill":"agent-team","projectId":"p","version":3,"settings":{"runDefaults":{"teams":1}}}\n');

  const first = await updateSettings({ setupPath, host: "codex", expectedVersion: 3, writer: { id: "w1", role: "project_orchestrator" }, operationId: "run-1", loadRegistry: async () => ({ projectOwner: "w1" }), change: { kind: "run", values: { teams: 2 } } });
  const stale = await updateSettings({ setupPath, host: "codex", expectedVersion: 3, writer: { id: "w2", role: "project_orchestrator" }, operationId: "run-2", loadRegistry: async () => ({ projectOwner: "w2" }), change: { kind: "run", values: { teams: 6 } } });

  assert.equal(first.status, "applied");
  assert.deepEqual(stale, { status: "conflict", reason: "version_changed", expectedVersion: 3, actualVersion: 4 });
  assert.equal(JSON.parse(await readFile(setupPath, "utf8")).settings.runDefaults.teams, 2);
});

test("settings mutations require the registered project owner and are idempotent by operation signature", async () => {
  const { updateSettings } = await import("../hooks/lib/settings.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-settings-"));
  const setupPath = path.join(directory, "setup.json");
  const source = '{"skill":"agent-team","projectId":"p","settings":{"runDefaults":{"teams":1}}}\n';
  await writeFile(setupPath, source);
  const base = {
    setupPath,
    host: "codex",
    expectedVersion: 0,
    writer: { id: "owner-1", role: "project_orchestrator" },
    operationId: "op-1",
    loadRegistry: async () => ({ projectOwner: "owner-1" }),
    change: { kind: "run", values: { teams: 2 } },
  };

  const applied = await updateSettings(base);
  const duplicate = await updateSettings(base);
  const reused = await updateSettings({ ...base, change: { kind: "run", values: { teams: 3 } } });
  const denied = await updateSettings({
    ...base,
    expectedVersion: 1,
    operationId: "op-2",
    writer: { id: "not-owner", role: "project_orchestrator" },
    loadRegistry: async () => ({ projectOwner: "owner-1" }),
  });
  const wrongRole = await updateSettings({
    ...base,
    expectedVersion: 1,
    operationId: "op-3",
    writer: { id: "owner-1", role: "developer" },
  });

  assert.equal(applied.status, "applied");
  assert.deepEqual(duplicate, { status: "duplicate", version: 1, operationId: "op-1" });
  assert.deepEqual(reused, { status: "conflict", reason: "operation_id_reused", operationId: "op-1" });
  assert.deepEqual(denied, { status: "conflict", reason: "project_owner_required" });
  assert.deepEqual(wrongRole, { status: "conflict", reason: "project_owner_required" });
  const saved = JSON.parse(await readFile(setupPath, "utf8"));
  assert.equal(saved.settings.runDefaults.teams, 2);
  assert.equal(saved.setupOperations.length, 1);
});

test("the explicit full wizard covers run defaults and each role with Back and Cancel controls", async () => {
  const { buildSettingsWizard } = await import("../hooks/lib/settings.mjs");
  const nativeChoices = {
    control: "numbered",
    enforceable: true,
    models: [{ id: "quality-model", label: "Quality Model", recommended: true, available: true, efforts: ["medium", "high"] }],
  };
  const wizard = buildSettingsWizard({ setup: { settings: { runDefaults: {} } }, host: "codex", nativeChoices });

  assert.deepEqual(wizard.steps.filter(({ kind }) => kind === "run").map(({ setting }) => setting), [
    "parallel_teams", "continuous", "auto_deploy", "deploy_batch_tasks",
  ]);
  assert.deepEqual(wizard.steps.filter(({ kind }) => kind === "role-model").map(({ role }) => role), [
    "project_orchestrator", "complex_developer", "developer", "routine_developer", "reviewer", "visual_reviewer",
  ]);
  assert.ok(wizard.steps.filter(({ kind }) => kind === "role-effort").every(({ choices }) => choices.some(({ id }) => id === "high")));
  assert.ok(wizard.steps.every(({ choices }) => choices.some(({ id }) => id === "cancel")));
  assert.ok(wizard.steps.every(({ choices }) => choices.some(({ id }) => id === "keep_existing")));
  assert.ok(wizard.steps.slice(1).every(({ choices }) => choices.some(({ id }) => id === "back")));
  assert.equal(wizard.steps.at(-1).kind, "review");
});

test("full wizard effort choices refresh from each role's selected draft model", async () => {
  const { buildSettingsWizard } = await import("../hooks/lib/settings.mjs");
  const setup = { settings: { hosts: { codex: { roles: { developer: { model: "quality", effort: "high" } } } } } };
  const nativeChoices = { models: [
    { id: "quality", efforts: ["high", "max"] },
    { id: "fast", efforts: ["low"] },
  ] };

  const wizard = buildSettingsWizard({ setup, host: "codex", nativeChoices, draft: { roles: { developer: { model: "fast" } } } });
  const effort = wizard.steps.find((step) => step.kind === "role-effort" && step.role === "developer");

  assert.equal(effort.model, "fast");
  assert.deepEqual(effort.choices.filter(({ id }) => !["keep_existing", "back", "cancel"].includes(id)).map(({ id }) => id), ["low"]);
  assert.ok(effort.choices.some(({ id }) => id === "back"));
  assert.ok(effort.choices.some(({ id }) => id === "cancel"));
});

test("recognized legacy routing is shown for its host but never rewrites an unknown host", async () => {
  const { inspectSettings, updateSettings } = await import("../hooks/lib/settings.mjs");
  const nativeChoices = { models: [{ id: "legacy-model", available: true, efforts: ["high"] }] };
  const legacy = inspectSettings({
    setup: { harness: "codex", roleRouting: { developer: { model: "legacy-model", effort: "high" } } },
    host: "codex",
    nativeChoices,
  });
  assert.equal(legacy.roles.find(({ id }) => id === "developer").source, "legacy");

  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-settings-"));
  const setupPath = path.join(directory, "setup.json");
  const source = '{"skill":"agent-team","projectId":"p","harness":"mystery","roleRouting":{"developer":{"model":"custom","effort":"max"}}}\n';
  await writeFile(setupPath, source);
  await assert.rejects(() => updateSettings({
    setupPath, host: "codex", expectedVersion: 0,
    writer: { id: "owner", role: "project_orchestrator" }, operationId: "legacy-edit",
    loadRegistry: async () => ({ projectOwner: "owner" }),
    change: { kind: "run", values: { parallel_teams: 2 } },
  }), /Unknown legacy settings host/);
  assert.equal(await readFile(setupPath, "utf8"), source);
});

test("native recommended routes supply effective defaults without inventing enforcement", async () => {
  const { inspectSettings } = await import("../hooks/lib/settings.mjs");
  const overview = inspectSettings({
    setup: {}, host: "claude-code",
    nativeChoices: { models: [{ id: "native-default", recommended: true, available: "unknown", efforts: ["high"], recommendedEffort: "high" }] },
  });
  const developer = overview.roles.find(({ id }) => id === "developer");

  assert.deepEqual(developer.configured, { model: null, effort: null });
  assert.deepEqual(developer.effective, { model: "native-default", effort: "high" });
  assert.equal(developer.source, "host_default");
  assert.equal(developer.enforceable, "unknown");
});

test("targeted updates validate run defaults and preserve custom role metadata", async () => {
  const { updateSettings } = await import("../hooks/lib/settings.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-settings-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, `${JSON.stringify({
    skill: "agent-team", projectId: "p", version: 1,
    settings: { hosts: { codex: { roles: { developer: { model: "old", effort: "medium", customNote: "keep" } } } } },
  })}\n`);
  const common = {
    setupPath, host: "codex", expectedVersion: 1,
    writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
  };

  await assert.rejects(() => updateSettings({ ...common, operationId: "bad-run", change: { kind: "run", values: { parallel_teams: 7 } } }), /parallel_teams/);
  const result = await updateSettings({
    ...common, operationId: "role-update",
    change: { kind: "role", role: "developer", model: "new", effort: "high" },
    nativeChoices: { models: [{ id: "new", available: true, efforts: ["high"] }] },
  });

  assert.equal(result.setup.settings.hosts.codex.roles.developer.customNote, "keep");
});

test("fallback routes persist only approved supported choices for the selected host", async () => {
  const { updateSettings } = await import("../hooks/lib/settings.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-settings-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, '{"skill":"agent-team","projectId":"p","version":1,"settings":{"hosts":{"claude-code":{"roles":{}},"codex":{"roles":{}}}}}\n');
  const common = {
    setupPath, host: "codex", expectedVersion: 1,
    writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
    nativeChoices: { models: [{ id: "primary", efforts: ["high"] }, { id: "fallback", efforts: ["medium"] }] },
  };

  const result = await updateSettings({
    ...common, operationId: "fallback-1",
    change: { kind: "fallback", role: "developer", routes: [{ model: "fallback", effort: "medium" }], escalation: true },
  });

  assert.deepEqual(result.setup.settings.hosts.codex.fallbacks.developer, {
    routes: [{ model: "fallback", effort: "medium" }], escalation: true, source: "approved",
  });
  assert.deepEqual(result.setup.settings.hosts["claude-code"], { roles: {} });
  await assert.rejects(() => updateSettings({
    ...common, expectedVersion: 2, operationId: "fallback-2",
    change: { kind: "fallback", role: "developer", routes: [{ model: "missing", effort: "medium" }], escalation: true },
  }), /Unsupported fallback model/);
});

test("simultaneous settings writers produce one applied update and one version conflict", async () => {
  const { updateSettings } = await import("../hooks/lib/settings.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-settings-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, '{"skill":"agent-team","projectId":"p","version":1,"settings":{"runDefaults":{"parallel_teams":1}}}\n');
  const request = (operationId, parallel_teams) => updateSettings({
    setupPath, host: "codex", expectedVersion: 1, operationId,
    writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
    change: { kind: "run", values: { parallel_teams } },
  });

  const results = await Promise.all([request("concurrent-1", 2), request("concurrent-2", 3)]);
  assert.deepEqual(results.map(({ status }) => status).sort(), ["applied", "conflict"]);
  assert.ok([2, 3].includes(JSON.parse(await readFile(setupPath, "utf8")).settings.runDefaults.parallel_teams));
});

function completeSettingsFixture() {
  return {
    skill: "agent-team", projectId: "settings-project", version: 9,
    tracker: { kind: "markdown", path: ".agent-team/TASKS.md", root: "." },
    initialization: { schemaVersion: 1, operationId: "initialize-settings", status: "complete",
      initialTaskIds: ["T-1"], trackerFingerprint: "a".repeat(64),
      handoff: { generatedBy: { name: "project-kickoff", version: "0.4.1" }, testedAgainst: { name: "agent-team", version: "7.2.0" } } },
    ownership: { epoch: 3, current: { host: "codex", sessionId: "owner", since: "2026-09-12T00:00:00.000Z", operationId: "recover-3" } },
    ownerRecoveryHistory: [{ operationId: "recover-3", previousEpoch: 2 }],
    dependencies: { hosts: { codex: { scope: "project", receipts: [{ id: "serena", status: "ready" }] } } },
    settings: {
      profile: "quality", custom: { retain: true },
      runDefaults: { parallel_teams: 2, continuous: false, auto_deploy: false, deploy_batch_tasks: 4 },
      hosts: {
        codex: { custom: "codex", roles: {
          project_orchestrator: { model: "quality", effort: "high", source: "override", note: "keep" },
          complex_developer: { model: "quality", effort: "high", source: "override" },
          developer: { model: "quality", effort: "high", source: "override" },
          routine_developer: { model: "fast", effort: "low", source: "override" },
          reviewer: { model: "quality", effort: "high", source: "override" },
          visual_reviewer: { model: "quality", effort: "high", source: "override" },
        }, fallbacks: { developer: { routes: [{ model: "fast", effort: "low" }], escalation: true } } },
        "claude-code": { custom: "claude", roles: { developer: { model: "opus", effort: "high", source: "override" } },
          fallbacks: { developer: { routes: [{ model: "sonnet", effort: "medium" }], escalation: false } } },
      },
    },
    activeRun: { routing: { developer: { model: "quality", effort: "high" } }, taskIds: ["T-1"] },
    dashboard: { snapshot: true, graph: { enabled: false, termsAcknowledged: false }, extension: "keep" },
    deployment: { auto: false, target: { kind: "origin", ref: "main" }, authority: { source: "user" } },
    setupOperations: [{ id: "dependency-9", signature: "b".repeat(64), kind: "dependencies", writer: "owner", version: 9 }],
    vendorExtension: { nested: { keep: true } },
  };
}

const settingsNativeChoices = { models: [
  { id: "quality", efforts: ["medium", "high"] },
  { id: "fast", efforts: ["low"] },
] };

function draftCall(setupPath, overrides = {}) {
  return {
    setupPath, host: "codex", expectedVersion: 9,
    writer: { id: "owner", role: "project_orchestrator", host: "codex", ownershipEpoch: 3 },
    operationId: "settings-draft-10",
    loadRegistry: async () => ({ projectOwner: "owner", projectOwnerHost: "codex", ownershipEpoch: 3 }),
    nativeChoices: settingsNativeChoices,
    draft: { runDefaults: { parallel_teams: 3, auto_deploy: true },
      roles: { developer: { model: "fast", effort: "low" }, reviewer: { model: "quality", effort: "medium" } } },
    ...overrides,
  };
}

test("cancel and back preserve complete setup bytes and report kept existing", async () => {
  const { updateSettings } = await import("../hooks/lib/settings.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-settings-complete-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, `${JSON.stringify(completeSettingsFixture(), null, 2)}\n`);
  const before = await readFile(setupPath);
  for (const kind of ["cancel", "back"]) {
    const result = await updateSettings({ ...draftCall(setupPath), operationId: `settings-${kind}`, change: { kind } });
    assert.deepEqual(result, { status: kind, settingsOutcome: "kept_existing" });
    assert.deepEqual(await readFile(setupPath), before);
  }
});

test("reviewed settings draft saves as one versioned atomic mutation", async () => {
  const { saveSettingsDraft } = await import("../hooks/lib/settings.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-settings-draft-"));
  const setupPath = path.join(directory, "setup.json");
  const original = completeSettingsFixture();
  await writeFile(setupPath, `${JSON.stringify(original, null, 2)}\n`);
  const result = await saveSettingsDraft(draftCall(setupPath));
  const saved = JSON.parse(await readFile(setupPath, "utf8"));
  assert.equal(result.status, "applied");
  assert.equal(saved.version, 10);
  assert.equal(saved.setupOperations.length, original.setupOperations.length + 1);
  assert.deepEqual(saved.setupOperations.at(-1), { id: "settings-draft-10", signature: saved.setupOperations.at(-1).signature,
    kind: "settings", writer: "owner", version: 10 });
  assert.deepEqual(saved.settings.runDefaults, { ...original.settings.runDefaults, parallel_teams: 3, auto_deploy: true });
  assert.deepEqual(saved.settings.hosts.codex.roles.developer, { ...original.settings.hosts.codex.roles.developer, model: "fast", effort: "low", source: "override" });
  assert.deepEqual(saved.settings.hosts.codex.roles.reviewer, { ...original.settings.hosts.codex.roles.reviewer, effort: "medium", source: "override" });
  assert.deepEqual(saved.settings.hosts["claude-code"], original.settings.hosts["claude-code"]);
  assert.deepEqual(saved.activeRun, original.activeRun);
  assert.deepEqual(saved.vendorExtension, original.vendorExtension);
});

test("empty or unchanged reviewed settings draft does not write", async () => {
  const { saveSettingsDraft } = await import("../hooks/lib/settings.mjs");
  for (const [label, draft] of [["empty", {}], ["unchanged", {
    runDefaults: { parallel_teams: 2, continuous: false },
    roles: { developer: { model: "quality", effort: "high" } },
  }]]) {
    const directory = await mkdtemp(path.join(os.tmpdir(), `agent-team-settings-${label}-`));
    const setupPath = path.join(directory, "setup.json");
    await writeFile(setupPath, `${JSON.stringify(completeSettingsFixture(), null, 2)}\n`);
    const before = await readFile(setupPath);
    const result = await saveSettingsDraft(draftCall(setupPath, { operationId: `settings-${label}`, draft }));
    assert.deepEqual(result, { status: "kept_existing", settingsOutcome: "kept_existing" });
    assert.deepEqual(await readFile(setupPath), before);
  }
});

test("reviewed settings draft equal to profile or native effective values does not write overrides", async () => {
  const { saveSettingsDraft } = await import("../hooks/lib/settings.mjs");
  for (const [label, setup, route] of [
    ["profile", { ...completeSettingsFixture(), settings: {
      profile: "quality", profiles: { quality: { roles: { developer: { model: "quality", effort: "high" } } } },
      runDefaults: { parallel_teams: 2 }, hosts: { codex: { roles: {} }, "claude-code": { roles: {} } },
    } }, { model: "quality", effort: "high" }],
    ["native", { ...completeSettingsFixture(), settings: {
      profile: "quality", runDefaults: { parallel_teams: 2 }, hosts: { codex: { roles: {} }, "claude-code": { roles: {} } },
    } }, { model: "quality", effort: "medium" }],
  ]) {
    const directory = await mkdtemp(path.join(os.tmpdir(), `agent-team-settings-effective-${label}-`));
    const setupPath = path.join(directory, "setup.json");
    await writeFile(setupPath, `${JSON.stringify(setup, null, 2)}\n`);
    const before = await readFile(setupPath);
    const result = await saveSettingsDraft(draftCall(setupPath, {
      operationId: `settings-effective-${label}`, draft: { roles: { developer: route } },
    }));
    assert.deepEqual(result, { status: "kept_existing", settingsOutcome: "kept_existing" });
    assert.deepEqual(await readFile(setupPath), before);
  }
});

test("confirmed settings draft replay and conflicts preserve bytes", async () => {
  const { saveSettingsDraft } = await import("../hooks/lib/settings.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-settings-replay-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, `${JSON.stringify(completeSettingsFixture(), null, 2)}\n`);
  const call = draftCall(setupPath);
  assert.equal((await saveSettingsDraft(call)).status, "applied");
  const committed = await readFile(setupPath);
  assert.equal((await saveSettingsDraft(call)).status, "duplicate");
  assert.deepEqual(await readFile(setupPath), committed);
  assert.equal((await saveSettingsDraft({ ...call, draft: { runDefaults: { parallel_teams: 4 } } })).reason, "operation_id_reused");
  assert.deepEqual(await readFile(setupPath), committed);
  assert.equal((await saveSettingsDraft({ ...call, expectedVersion: 9, operationId: "stale-settings-11" })).reason, "version_changed");
  assert.deepEqual(await readFile(setupPath), committed);
  await assert.rejects(saveSettingsDraft({ ...call, expectedVersion: 10, operationId: "invalid-settings-11",
    draft: { roles: { developer: { model: "fast", effort: "low", fallback: true } } } }), /Unsupported settings draft role/);
  assert.deepEqual(await readFile(setupPath), committed);
});
