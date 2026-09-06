import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { promisify } from "node:util";

import { policyFixture } from "./hook-test-helpers.mjs";

const run = promisify(execFile);
const sourceRoot = path.resolve(import.meta.dirname, "..");
const cli = path.join(sourceRoot, "hooks", "agent-team-cli.mjs");

test("CLI returns structured health and source-only artifact status", async () => {
  // This test catches human-only command output and a missing source-only result.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-cli-home-"));
  const sourceOnly = await mkdtemp(path.join(os.tmpdir(), "agent-team-cli-source-"));
  try {
    const health = JSON.parse((await run(process.execPath, [cli, "health", "--home", home])).stdout);
    const artifacts = JSON.parse((await run(process.execPath, [cli, "check-artifacts", "--source", sourceOnly])).stdout);
    assert.equal(typeof health.runtimes.codex.installed, "boolean");
    assert.equal(artifacts.status, "not_applicable");
  } finally {
    await rm(home, { force: true, recursive: true });
    await rm(sourceOnly, { force: true, recursive: true });
  }
});

test("CLI health reports missing, invalid, and current project mapping caches", async () => {
  // This test catches health output that hides unavailable fallback mapping protection.
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-cli-project-"));
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-cli-home-"));
  try {
    const value = await policyFixture(root);
    const cache = path.join(value.root, ".agent-team", "operation-mappings.json");
    await rm(cache);
    const missing = JSON.parse((await run(process.execPath, [cli, "health", "--home", home, "--project", value.feature])).stdout);
    await writeFile(cache, "{bad json\n");
    const invalid = JSON.parse((await run(process.execPath, [cli, "health", "--home", home, "--project", value.feature])).stdout);
    await writeFile(cache, JSON.stringify({
      schemaVersion: 1,
      kind: "agent-team-operation-mapping-cache",
      projectId: "project-1",
      sourcePath: ".agent-team/state.json",
      operationMappings: value.operationMappings,
    }));
    const current = JSON.parse((await run(process.execPath, [cli, "health", "--home", home, "--project", value.feature])).stdout);

    assert.deepEqual(missing.operationMappings, { status: "missing", fallbackProtection: "unavailable" });
    assert.deepEqual(invalid.operationMappings, { status: "invalid", fallbackProtection: "unavailable" });
    assert.deepEqual(current.operationMappings, { status: "current", fallbackProtection: "available" });
  } finally {
    await rm(root, { force: true, recursive: true });
    await rm(`${root}-feature`, { force: true, recursive: true });
    await rm(`${root}-remote`, { force: true, recursive: true });
    await rm(home, { force: true, recursive: true });
  }
});
