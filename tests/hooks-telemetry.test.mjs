import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readFile, readdir, rm, stat, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { activationCapability, activationRecordFor, appendActivationLog, auditEffectiveness, readActivationLogs } from "../hooks/lib/telemetry.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { force: true, recursive: true }))));

async function directory() {
  const value = await mkdtemp(path.join(os.tmpdir(), "agent-team-telemetry-"));
  temporary.push(value);
  return value;
}

function record(correlationId) {
  return {
    runtime: "claude",
    skill: "agent-team",
    sessionId: "session-1",
    eventKind: "PreToolUse",
    projectId: "project-1",
    teamId: "TEAM-001",
    correlationId,
    prompt: "private prompt",
    rawSql: "DELETE FROM customers",
    credential: "secret-value",
  };
}

test("activation logging is concurrent-safe, user-only, allowlisted, and deduplicated", async () => {
  // This test catches interleaved JSON, broad payload logging, and duplicate activation records.
  const root = await directory();
  await chmod(root, 0o755);
  const results = await Promise.all(Array.from({ length: 20 }, (_, index) => appendActivationLog(root, record(`event-${index}`), {
    now: new Date(`2026-09-06T12:00:${String(index).padStart(2, "0")}.000Z`),
  })));
  const duplicate = await appendActivationLog(root, record("event-0"), { now: new Date("2026-09-06T12:01:00.000Z") });
  const logs = await readActivationLogs(root);
  const source = await readFile(path.join(root, "activation.jsonl"), "utf8");
  const mode = (await stat(path.join(root, "activation.jsonl"))).mode & 0o777;
  const directoryMode = (await stat(root)).mode & 0o777;

  assert.equal(results.every(({ recorded }) => recorded), true);
  assert.equal(duplicate.recorded, false);
  assert.equal(logs.records.length, 20);
  assert.equal(logs.malformed, 0);
  assert.equal(mode, 0o600);
  assert.equal(directoryMode, 0o700);
  for (const forbidden of ["private prompt", "DELETE FROM", "secret-value"]) assert.equal(source.includes(forbidden), false);
});

test("activation logs rotate within bounds and malformed records stay unavailable", async () => {
  // This test catches unbounded logs and malformed lines being counted as evidence.
  const root = await directory();
  for (let index = 0; index < 12; index += 1) {
    await appendActivationLog(root, record(`long-correlation-${index}-${"x".repeat(40)}`), {
      now: new Date(`2026-09-06T12:00:${String(index).padStart(2, "0")}.000Z`),
      maxBytes: 500,
      maxFiles: 2,
    });
  }
  await writeFile(path.join(root, "activation.jsonl"), "{bad json}\n", { flag: "a" });
  const names = await readdir(root);
  const logs = await readActivationLogs(root);

  assert.ok(names.includes("activation.jsonl.1"));
  assert.equal(names.includes("activation.jsonl.3"), false);
  assert.equal(logs.malformed, 1);
});

test("effectiveness audit is bounded and states activation coverage limits", async () => {
  // This test catches Codex activation inference and an audit that reads an unbounded history.
  const root = await directory();
  const tracker = path.join(root, "TASKS.md");
  const mistakes = path.join(root, "MISTAKES.md");
  await writeFile(tracker, `# Agent-Team Tasks
| ID | Owner | Status |
| --- | --- | --- |
| AT-001 | TEAM-001 | verified |
| AT-002 | TEAM-002 | in_progress |
`);
  await writeFile(mistakes, `# Agent-Team Mistakes
## M-001: Keep hook failures visible
Source: AT-001, review evidence
`);
  for (let index = 0; index < 5; index += 1) await appendActivationLog(root, record(`audit-${index}`));
  const audit = await auditEffectiveness({ logDirectory: root, trackerPath: tracker, mistakesPath: mistakes, maxRecords: 2 });

  assert.equal(audit.recordsProcessed, 2);
  assert.equal(audit.truncated, true);
  assert.equal(audit.references.tracker, tracker);
  assert.deepEqual(audit.sources.tracker.taskIds, ["AT-001", "AT-002"]);
  assert.deepEqual(audit.sources.mistakes.lessonIds, ["M-001"]);
  assert.deepEqual(audit.correlations.activatedTaskIds, ["AT-001"]);
  assert.deepEqual(audit.correlations.mistakeTaskIds, ["AT-001"]);
  assert.equal(activationCapability("codex").status, "unsupported");
  assert.equal(activationCapability("claude").status, "supported");
});

test("effectiveness audit distinguishes unavailable and malformed canonical sources", async () => {
  // This test catches an audit that echoes paths without reading bounded canonical evidence.
  const root = await directory();
  const malformed = path.join(root, "MISTAKES.md");
  await writeFile(malformed, "not a canonical mistakes record\n");
  const audit = await auditEffectiveness({
    logDirectory: root,
    trackerPath: path.join(root, "missing-TASKS.md"),
    mistakesPath: malformed,
    maxRecords: 2,
  });

  assert.equal(audit.sources.tracker.status, "unavailable");
  assert.equal(audit.sources.mistakes.status, "malformed");
  assert.deepEqual(audit.correlations.activatedTaskIds, []);
});

test("only reliable Claude Skill and slash-expansion paths create activation records", () => {
  // This test catches inferred Codex activation and unrelated Claude tool events.
  const base = { sessionId: "s", eventId: "e", operation: { kind: "skill", skill: "agent-team" } };
  const team = activationRecordFor({ ...base, runtime: "claude", event: "PreToolUse" }, { projectId: "p" }, {
    role: "team", team: { "team id": "TEAM-001" },
  });
  const owner = activationRecordFor({ ...base, runtime: "claude", event: "PreToolUse" }, { projectId: "p" }, { role: "project_owner" });
  assert.equal(team.runtime, "claude");
  assert.equal(team.teamId, "TEAM-001");
  assert.equal(team.identityKind, "team");
  assert.equal(owner.identityKind, "project_owner");
  assert.equal(owner.teamId, undefined);
  assert.equal(activationRecordFor({ ...base, runtime: "claude", event: "UserPromptExpansion" }, { projectId: "p" }).eventKind, "UserPromptExpansion");
  assert.equal(activationRecordFor({ ...base, runtime: "codex", event: "PreToolUse" }, { projectId: "p" }), undefined);
  assert.equal(activationRecordFor({ ...base, runtime: "claude", event: "PostToolUse" }, { projectId: "p" }), undefined);
});
