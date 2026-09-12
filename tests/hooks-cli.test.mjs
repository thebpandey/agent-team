import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { promisify } from "node:util";

import { policyFixture } from "./hook-test-helpers.mjs";
import { buildArtifacts } from "../hooks/lib/artifacts.mjs";

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
      authoritative: false,
      projectId: "project-1",
      sourcePath: ".agent-team/state.json",
      operationMappings: value.operationMappings,
    }));
    const current = JSON.parse((await run(process.execPath, [cli, "health", "--home", home, "--project", value.feature])).stdout);

    const threatModel = {
      authoritative: false,
      source: "validated_canonical_state",
      purpose: "classification_fallback",
    };
    assert.deepEqual(missing.operationMappings, { ...threatModel, status: "missing", fallbackProtection: "unavailable" });
    assert.deepEqual(invalid.operationMappings, { ...threatModel, status: "invalid", fallbackProtection: "unavailable" });
    assert.deepEqual(current.operationMappings, { ...threatModel, status: "current", fallbackProtection: "available" });
  } finally {
    await rm(root, { force: true, recursive: true });
    await rm(`${root}-feature`, { force: true, recursive: true });
    await rm(`${root}-remote`, { force: true, recursive: true });
    await rm(home, { force: true, recursive: true });
  }
});

test("CLI install and rollback require effective target flags and reject unknown input", async () => {
  // Ignored flags or positional text could make the CLI configure a different target than the caller selected.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-cli-install-home-"));
  const projectRoot = await mkdtemp(path.join(os.tmpdir(), "agent-team-cli-project-root-"));
  try {
    const { archives: [archive] } = await buildArtifacts({ sourceRoot, outputDirectory: home, sourceRevision: "d".repeat(40) });
    const checksums = path.join(home, "SHA256SUMS");
    await writeFile(checksums, `${createHash("sha256").update(await readFile(archive)).digest("hex")}  ${path.basename(archive)}\n`);
    const failures = [
      ["health", "--unknown", "value"],
      ["install", "--source", sourceRoot, "--home", home, "--host", "codex", "--scope", "user", "--project", projectRoot],
      ["install", "--source", sourceRoot, "--home", home, "--host"],
      ["install", "unexpected", "--source", sourceRoot, "--home", home, "--host", "codex", "--scope", "user"],
      ["install", "--checksums", checksums, "--home", home, "--host", "codex", "--scope", "user"],
      ["install", "--archive", archive, "--home", home, "--host", "codex", "--scope", "user"],
      ["install", "--archive", archive, "--archive", archive, "--checksums", checksums, "--home", home, "--host", "codex", "--scope", "user"],
      ["install", "--archive", archive, "--checksums", checksums, "--checksums", checksums, "--home", home, "--host", "codex", "--scope", "user"],
    ];
    for (const args of failures) {
      await assert.rejects(run(process.execPath, [cli, ...args]), (error) => {
        const result = JSON.parse(error.stdout);
        assert.equal(result.status, "failed");
        assert.match(result.error, /unsupported|ineffective|missing|unexpected|duplicate/i);
        return true;
      });
    }

    const installed = JSON.parse((await run(process.execPath, [cli, "install", "--archive", archive, "--checksums", checksums, "--home", home, "--host", "codex", "--scope", "user"])).stdout);
    assert.equal(installed.status, "installed");
    await readFile(path.join(home, ".codex", "hooks.json"));
    await assert.rejects(readFile(path.join(home, ".claude", "settings.json")), { code: "ENOENT" });
    const rolledBack = JSON.parse((await run(process.execPath, [cli, "rollback", "--home", home, "--host", "codex", "--scope", "user"])).stdout);
    assert.equal(rolledBack.status, "uninstalled");
  } finally {
    await rm(home, { force: true, recursive: true });
    await rm(projectRoot, { force: true, recursive: true });
  }
});
