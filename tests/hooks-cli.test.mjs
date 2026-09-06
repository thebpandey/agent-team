import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { promisify } from "node:util";

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
