import assert from "node:assert/strict";
import { execFile, execFileSync, spawn } from "node:child_process";
import { access, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { promisify } from "node:util";

import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { resolveProject } from "../hooks/lib/project.mjs";
import { policyFixture } from "./hook-test-helpers.mjs";

const run = promisify(execFile);
const cli = path.resolve(import.meta.dirname, "../hooks/agent-team-cli.mjs");
const temporary = [];

test.afterEach(async () => Promise.all(temporary.splice(0).map((target) => rm(target, { force: true, recursive: true }))));

async function fixture() {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-workflow-cli-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  const value = await policyFixture(root);
  const requests = path.join(root, ".agent-team", "requests");
  await mkdir(requests, { recursive: true });
  return { ...value, requests, project: await resolveProject(value.feature) };
}

async function requestFile(value, name, envelope) {
  const file = path.join(value.requests, `${name}.json`);
  await writeFile(file, `${JSON.stringify(envelope, null, 2)}\n`);
  return file;
}

async function invoke(command, ...args) {
  const { stdout } = await run(process.execPath, [cli, command, ...args], { encoding: "utf8", timeout: 5000 });
  return JSON.parse(stdout);
}

async function invokeFailure(command, ...args) {
  try {
    await run(process.execPath, [cli, command, ...args], { encoding: "utf8", timeout: 2000 });
  } catch (error) {
    return JSON.parse(error.stdout);
  }
  assert.fail("CLI command unexpectedly succeeded");
}

function envelope(actorSessionId, expectedVersion, request) {
  return { schemaVersion: 1, actorSessionId, expectedVersion, request };
}

test("real CLI exposes read-only status, usage, recovery, and eligibility without creating a dashboard", async () => {
  const value = await fixture();
  const checkpoint = await requestFile(value, "checkpoint-read", envelope("developer-session", 0, {
    eventId: "checkpoint-read",
    sessionId: "developer-session",
    taskIds: ["AT-001"],
    worktree: value.feature,
    revision: value.revision,
    nextAction: "Continue the assigned task.",
  }));
  assert.equal((await invoke("checkpoint", "--project", value.feature, "--request", checkpoint)).status, "applied");

  const status = await invoke("status", "--project", value.feature);
  const usage = await invoke("usage", "--project", value.feature);
  const recovery = await invoke("recovery", "--project", value.feature, "--session", "developer-session", "--task", "AT-001", "--worktree", value.feature);
  const eligibility = await invoke("eligibility", "--project", value.feature);

  assert.equal(status.project.id, "project-1");
  assert.equal(usage.status, "completed");
  assert.equal(usage.source.kind, "operational_state");
  assert.equal(recovery.nextAction, "Continue the assigned task.");
  assert.deepEqual(eligibility.held.map(({ id }) => id), ["AT-001"]);
  await assert.rejects(access(path.join(value.root, ".agent-team", "dashboard", "index.html")), { code: "ENOENT" });
});

test("real CLI forwards versioned transition, gate-evidence, and cleanup requests to canonical APIs", async () => {
  const value = await fixture();
  const canonical = await loadCanonicalState(value.project);
  const transition = await requestFile(value, "pause", envelope("owner-session", 0, {
    operationId: "cli-pause",
    taskId: "AT-001",
    expectedFingerprint: canonical.tracker.fingerprint,
    expectedOwner: "TEAM-001",
    action: "pause",
  }));
  const paused = await invoke("task-transition", "--project", value.feature, "--request", transition);
  assert.equal(paused.status, "applied");
  assert.equal((await invoke("task-transition", "--project", value.feature, "--request", transition)).status, "duplicate");

  const afterPause = await loadCanonicalState(value.project);
  const evidencePath = path.join(value.root, ".agent-team", "evidence", "cli-gate.json");
  await mkdir(path.dirname(evidencePath), { recursive: true });
  await writeFile(evidencePath, JSON.stringify({ status: "passed", revision: value.revision, taskIds: ["AT-001"] }));
  const gate = await requestFile(value, "gate", envelope("owner-session", afterPause.state.stateVersion, {
    operationId: "cli-gate",
    gate: "completion",
    taskIds: ["AT-001"],
    expectedFingerprint: afterPause.tracker.fingerprint,
    expectedRevision: value.revision,
    evidencePath,
  }));
  assert.equal((await invoke("gate-evidence", "--project", value.feature, "--request", gate)).status, "applied");

  const current = await loadCanonicalState(value.project);
  const cleanup = await requestFile(value, "cleanup", envelope("owner-session", current.state.stateVersion, {
    operationId: "cli-cleanup",
    taskId: "AT-001",
    expectedRevision: value.revision,
    worktree: value.feature,
    expectedWriter: { pid: 99999999, startTime: "1", bootId: "not-current", host: os.hostname() },
  }));
  const retained = await invoke("cleanup", "--project", value.feature, "--request", cleanup);
  assert.deepEqual(retained, { status: "conflict", reason: "retain_identity_mismatch" });
  await access(value.feature);
});

test("request JSON must be a bounded regular file with valid schema, actor, version, and command fields", async () => {
  const value = await fixture();
  const missingActor = await requestFile(value, "missing-actor", { schemaVersion: 1, expectedVersion: 0, request: {} });
  assert.match((await invokeFailure("checkpoint", "--project", value.feature, "--request", missingActor)).error, /actorSessionId/);
  const wrongSchema = await requestFile(value, "wrong-schema", envelope("owner-session", 0, {}));
  await writeFile(wrongSchema, JSON.stringify({ ...JSON.parse(await readFile(wrongSchema, "utf8")), schemaVersion: 2 }));
  assert.match((await invokeFailure("checkpoint", "--project", value.feature, "--request", wrongSchema)).error, /schemaVersion/);
  const wrongVersion = await requestFile(value, "wrong-version", envelope("owner-session", "latest", {}));
  assert.match((await invokeFailure("checkpoint", "--project", value.feature, "--request", wrongVersion)).error, /expectedVersion/);
  const missingEvent = await requestFile(value, "missing-event", envelope("owner-session", 0, { sessionId: "owner-session" }));
  assert.match((await invokeFailure("checkpoint", "--project", value.feature, "--request", missingEvent)).error, /eventId/);
  assert.match((await invokeFailure("task-transition", "--project", value.feature, "--request", missingActor)).error, /actorSessionId/);
  const canonical = await loadCanonicalState(value.project);
  const missingOwner = await requestFile(value, "missing-owner", envelope("owner-session", 0, {
    operationId: "missing-owner",
    taskId: "AT-001",
    expectedFingerprint: canonical.tracker.fingerprint,
    expectedOwner: "TEAM-001",
    action: "claim",
  }));
  assert.match((await invokeFailure("task-transition", "--project", value.feature, "--request", missingOwner)).error, /request.owner/);
  const invalidWriter = await requestFile(value, "invalid-writer", envelope("owner-session", 0, {
    operationId: "invalid-writer",
    taskId: "AT-001",
    expectedRevision: value.revision,
    worktree: value.feature,
    expectedWriter: {},
  }));
  assert.match((await invokeFailure("cleanup", "--project", value.feature, "--request", invalidWriter)).error, /expectedWriter/);
  assert.match((await invokeFailure("status", "--project", value.feature, "--unknown", "value")).error, /Unsupported flag/);

  const oversized = path.join(value.requests, "oversized.json");
  await writeFile(oversized, JSON.stringify(envelope("owner-session", 0, { eventId: "large", sessionId: "owner-session", padding: "x".repeat(256 * 1024) })));
  assert.match((await invokeFailure("checkpoint", "--project", value.feature, "--request", oversized)).error, /bounded regular JSON file/);

  const fifo = path.join(value.requests, "request.fifo");
  execFileSync("mkfifo", [fifo]);
  const started = Date.now();
  assert.match((await invokeFailure("checkpoint", "--project", value.feature, "--request", fifo)).error, /regular JSON file/);
  assert.ok(Date.now() - started < 1500, "FIFO validation must not wait for a writer");
});

test("health forwards explicit project scope", async () => {
  const value = await fixture();
  const result = await invoke("health", "--home", value.root, "--project", value.feature, "--scope", "project");
  assert.equal(result.installation.scope, "project");
  assert.equal(result.installation.root, value.root);
});

test("dashboard snapshot uses the fixed path and configured refresh remains opt-in", async () => {
  const value = await fixture();
  const destination = path.join(value.root, ".agent-team", "dashboard", "index.html");
  const result = await invoke("dashboard-snapshot", "--project", value.feature);
  assert.equal(result.destination, destination);
  assert.match(await readFile(destination, "utf8"), /LOCAL STATUS SNAPSHOT/);

  const module = await import("../hooks/lib/workflow-cli.mjs").catch(() => ({}));
  assert.equal(typeof module.refreshConfiguredDashboard, "function", "configured dashboard refresh helper is required");
  const unconfigured = await fixture();
  assert.deepEqual(await module.refreshConfiguredDashboard(unconfigured.project), { status: "skipped", reason: "not_configured" });
  await assert.rejects(access(path.join(unconfigured.root, ".agent-team", "dashboard", "index.html")), { code: "ENOENT" });
  const setupPath = path.join(unconfigured.root, ".agent-team", "setup.json");
  const setup = JSON.parse(await readFile(setupPath, "utf8"));
  setup.dashboard = { snapshot: true };
  await writeFile(setupPath, JSON.stringify(setup));
  const configured = await module.refreshConfiguredDashboard(await resolveProject(unconfigured.feature));
  assert.equal(configured.status, "published");
});

test("dashboard start reports its loopback URL and stops cleanly on SIGINT or SIGTERM", async (t) => {
  for (const signal of ["SIGINT", "SIGTERM"]) {
    await t.test(signal, async () => {
      const value = await fixture();
      const child = spawn(process.execPath, [cli, "dashboard-start", "--project", value.feature, "--port", "0"], { stdio: ["ignore", "pipe", "pipe"] });
      let stdout = "";
      let stderr = "";
      child.stdout.on("data", (chunk) => { stdout += chunk; });
      child.stderr.on("data", (chunk) => { stderr += chunk; });
      const listening = await new Promise((resolve, reject) => {
        const timeout = setTimeout(() => reject(new Error(`dashboard did not start: ${stderr}`)), 3000);
        child.stdout.on("data", () => {
          const line = stdout.split("\n").find(Boolean);
          if (line) { clearTimeout(timeout); resolve(JSON.parse(line)); }
        });
      });
      assert.equal(listening.status, "listening");
      assert.match(listening.url, /^http:\/\/127\.0\.0\.1:\d+\/$/);
      const body = await new Promise((resolve, reject) => {
        http.get(listening.url, (response) => {
          let source = "";
          response.setEncoding("utf8");
          response.on("data", (chunk) => { source += chunk; });
          response.on("end", () => resolve(source));
        }).on("error", reject);
      });
      assert.match(body, /LOCAL LIVE STATUS/);
      child.kill(signal);
      const code = await new Promise((resolve) => child.once("exit", resolve));
      assert.equal(code, 0, stderr);
      const records = stdout.trim().split("\n").map(JSON.parse);
      assert.equal(records.at(-1).status, "stopped");
      assert.equal(records.at(-1).url, listening.url);
    });
  }
});
