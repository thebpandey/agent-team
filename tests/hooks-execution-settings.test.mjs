import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

const defaults = {
  lanes: {
    enabled: true,
    rotation: { tasks: 2, onPressure: true },
    factSheetStaleDays: 7,
    workerUpdateMaxChars: 2000,
    briefMaxWords: 6000,
  },
  supervision: { heartbeatSeconds: 600 },
  limits: { subprocessMaxBufferBytes: 2097152, maxPlanTasks: 1000, canonicalRecordMaxBytes: 16 * 1024 * 1024 },
};

test("execution settings resolve literal operational defaults", async () => {
  const { resolveExecutionSettings } = await import("../hooks/lib/settings.mjs");

  assert.deepEqual(resolveExecutionSettings({}, "codex"), defaults);
  assert.deepEqual(resolveExecutionSettings({}, "claude-code"), defaults);
});

test("execution settings merge only the selected host without sharing nested defaults", async () => {
  const { resolveExecutionSettings } = await import("../hooks/lib/settings.mjs");
  const setup = { settings: { hosts: {
    codex: { execution: { lanes: { enabled: false, rotation: { tasks: 4 } }, supervision: { heartbeatSeconds: 60 } } },
    "claude-code": { execution: { lanes: { factSheetStaleDays: 14 }, limits: { maxPlanTasks: 750 } } },
  } } };

  assert.deepEqual(resolveExecutionSettings(setup, "codex"), {
    ...defaults,
    lanes: { ...defaults.lanes, enabled: false, rotation: { tasks: 4, onPressure: true } },
    supervision: { heartbeatSeconds: 60 },
  });
  assert.deepEqual(resolveExecutionSettings(setup, "claude-code"), {
    ...defaults,
    lanes: { ...defaults.lanes, factSheetStaleDays: 14 },
    limits: { ...defaults.limits, maxPlanTasks: 750 },
  });
  const first = resolveExecutionSettings({}, "codex");
  first.lanes.rotation.tasks = 99;
  assert.equal(resolveExecutionSettings({}, "codex").lanes.rotation.tasks, 2);
});

test("execution settings reject heartbeat below 60 and accept the boundary", async () => {
  const { resolveExecutionSettings } = await import("../hooks/lib/settings.mjs");
  const setup = (heartbeatSeconds) => ({ settings: { hosts: { codex: { execution: { supervision: { heartbeatSeconds } } } } } });

  assert.throws(() => resolveExecutionSettings(setup(59), "codex"), /heartbeatSeconds/);
  assert.equal(resolveExecutionSettings(setup(60), "codex").supervision.heartbeatSeconds, 60);
});

test("execution settings reject malformed and open-ended values", async (t) => {
  const { resolveExecutionSettings } = await import("../hooks/lib/settings.mjs");
  assert.equal(typeof resolveExecutionSettings, "function");
  const cases = [
    ["unknown host", {}, "other", /Unknown settings host/],
    ["non-object execution", { settings: { hosts: { codex: { execution: [] } } } }, "codex", /execution settings must be an object/],
    ["unknown field", { settings: { hosts: { codex: { execution: { limits: { invented: 1 } } } } } }, "codex", /Unsupported .*limits field/],
    ["non-boolean lane", { settings: { hosts: { codex: { execution: { lanes: { enabled: "yes" } } } } } }, "codex", /lanes.enabled/],
    ["zero rotation", { settings: { hosts: { codex: { execution: { lanes: { rotation: { tasks: 0 } } } } } } }, "codex", /rotation.tasks/],
    ["fractional stale days", { settings: { hosts: { codex: { execution: { lanes: { factSheetStaleDays: 1.5 } } } } } }, "codex", /factSheetStaleDays/],
    ["zero update cap", { settings: { hosts: { codex: { execution: { lanes: { workerUpdateMaxChars: 0 } } } } } }, "codex", /workerUpdateMaxChars/],
    ["zero brief cap", { settings: { hosts: { codex: { execution: { lanes: { briefMaxWords: 0 } } } } } }, "codex", /briefMaxWords/],
    ["zero subprocess cap", { settings: { hosts: { codex: { execution: { limits: { subprocessMaxBufferBytes: 0 } } } } } }, "codex", /subprocessMaxBufferBytes/],
    ["zero plan cap", { settings: { hosts: { codex: { execution: { limits: { maxPlanTasks: 0 } } } } } }, "codex", /maxPlanTasks/],
    ["zero canonical cap", { settings: { hosts: { codex: { execution: { limits: { canonicalRecordMaxBytes: 0 } } } } } }, "codex", /canonicalRecordMaxBytes/],
    ["unbounded canonical cap", { settings: { hosts: { codex: { execution: { limits: { canonicalRecordMaxBytes: 16 * 1024 * 1024 + 1 } } } } } }, "codex", /canonicalRecordMaxBytes/],
  ];
  for (const [label, setup, host, message] of cases) await t.test(label, () => {
    assert.throws(() => resolveExecutionSettings(setup, host), message);
  });
});

test("targeted execution mutation preserves roles custom fields and the other host", async (t) => {
  const { updateSettings } = await import("../hooks/lib/settings.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-execution-settings-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const setupPath = path.join(directory, "setup.json");
  const setup = {
    skill: "agent-team", projectId: "p", version: 4, vendor: { keep: true },
    settings: { custom: "keep", hosts: {
      codex: { custom: "codex", roles: { developer: { model: "gpt", effort: "high" } }, execution: { lanes: { enabled: true } } },
      "claude-code": { custom: "claude", execution: { supervision: { heartbeatSeconds: 120 } } },
    } },
  };
  await writeFile(setupPath, `${JSON.stringify(setup, null, 2)}\n`);

  const result = await updateSettings({
    setupPath, host: "codex", expectedVersion: 4, writer: { id: "owner", role: "project_orchestrator" },
    operationId: "execution-1", loadRegistry: async () => ({ projectOwner: "owner" }),
    change: { kind: "execution", values: { lanes: { rotation: { tasks: 3 } }, supervision: { heartbeatSeconds: 60 } } },
  });

  assert.equal(result.status, "applied");
  assert.deepEqual(result.setup.settings.hosts.codex.execution, {
    lanes: { enabled: true, rotation: { tasks: 3 } }, supervision: { heartbeatSeconds: 60 },
  });
  assert.deepEqual(result.setup.settings.hosts.codex.roles, setup.settings.hosts.codex.roles);
  assert.deepEqual(result.setup.settings.hosts["claude-code"], setup.settings.hosts["claude-code"]);
  assert.deepEqual(result.setup.vendor, setup.vendor);
  assert.equal(JSON.parse(await readFile(setupPath, "utf8")).settings.custom, "keep");
});

test("direct execution mutation rejects nested empty objects without writing or incrementing version", async (t) => {
  const { updateSettings } = await import("../hooks/lib/settings.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-execution-empty-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const setupPath = path.join(directory, "setup.json");
  const before = `${JSON.stringify({ skill: "agent-team", projectId: "p", version: 1 }, null, 2)}\n`;
  await writeFile(setupPath, before);

  for (const [index, values] of [{ lanes: {} }, { lanes: { rotation: {} } }, { supervision: {} }, { limits: {} }].entries()) {
    await assert.rejects(updateSettings({
      setupPath, host: "codex", expectedVersion: 1, writer: { id: "owner", role: "project_orchestrator" },
      operationId: `execution-empty-${index}`, loadRegistry: async () => ({ projectOwner: "owner" }),
      change: { kind: "execution", values },
    }), /change.values must include at least one setting/);
    assert.equal(await readFile(setupPath, "utf8"), before);
  }
});

test("wizard execution steps show configured effective and draft sources through shared validation", async () => {
  const { buildSettingsWizard } = await import("../hooks/lib/settings.mjs");
  const setup = { settings: { hosts: { codex: { execution: { supervision: { heartbeatSeconds: 120 } } } } } };
  const wizard = buildSettingsWizard({ setup, host: "codex", draft: { execution: { supervision: { heartbeatSeconds: 60 } } } });
  const steps = wizard.steps.filter(({ kind }) => kind === "execution");

  assert.deepEqual(steps.map(({ setting }) => setting), [
    "lanes.enabled", "lanes.rotation.tasks", "lanes.rotation.onPressure", "lanes.factSheetStaleDays",
    "lanes.workerUpdateMaxChars", "lanes.briefMaxWords", "supervision.heartbeatSeconds",
    "limits.subprocessMaxBufferBytes", "limits.maxPlanTasks", "limits.canonicalRecordMaxBytes",
  ]);
  assert.deepEqual(steps.find(({ setting }) => setting === "supervision.heartbeatSeconds"), {
    kind: "execution", setting: "supervision.heartbeatSeconds", configured: 120, effective: 60, source: "draft",
    choices: [
      { id: "set", label: "Set value" }, { id: "keep_existing", label: "Keep Existing" },
      { id: "back", label: "Back" }, { id: "cancel", label: "Cancel" },
    ],
  });
  assert.throws(() => buildSettingsWizard({ setup, host: "codex", draft: { execution: { supervision: { heartbeatSeconds: 59 } } } }), /heartbeatSeconds/);
});
