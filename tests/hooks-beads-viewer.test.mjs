import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import fsPromises from "node:fs/promises";
import { access, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { syncBuiltinESMExports } from "node:module";
import test from "node:test";
import { CATALOG_BY_ID } from "../hooks/lib/dependency-catalog.mjs";
import { buildPreparationPlan, createDependencyRunner, inspectDependencies, prepareDependencies } from "../hooks/lib/dependencies.mjs";
import { createBeadsGraphCommandAdapter } from "../hooks/lib/dashboard.mjs";

async function fixture(t) {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-bv-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  return { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
}

test("bv capability requires the no-hooks option before exporting canonical data", async (t) => {
  const paths = await fixture(t);
  const calls = [];
  const result = await createBeadsGraphCommandAdapter({ projectRoot: paths.projectRoot,
    tracker: { kind: "beads", id: "selected", executable: "bd" }, selected: true, termsAcknowledged: true,
    runCommand: async (_file, args) => { calls.push(args); return { status: "completed", output: args[0] === "--help" ? "--robot-graph --graph-format string" : "version" }; },
  }).refresh();
  assert.equal(result.status, "unavailable");
  assert.deepEqual(calls, [["--version"], ["--version"], ["--help"]]);
});

test("bv preparation rejects absent selection prerequisites before running dependencies", async (t) => {
  const paths = await fixture(t);
  const setupPath = path.join(paths.projectRoot, "setup.json");
  for (const setup of [
    { tracker: { kind: "markdown", path: "TASKS.md" }, dashboard: { graph: { enabled: true, termsAcknowledged: true } } },
    { tracker: { kind: "beads" }, dashboard: { graph: { enabled: true, termsAcknowledged: false } } },
    { tracker: { kind: "beads" }, dashboard: { graph: { enabled: false, termsAcknowledged: true } } },
  ]) {
    await writeFile(setupPath, JSON.stringify({ skill: "agent-team", projectId: "p", version: 0, ...setup }));
    let calls = 0;
    await assert.rejects(prepareDependencies({ setupPath, expectedVersion: 0, writer: { id: "owner", role: "project_orchestrator" },
      operationId: "prepare-bv", loadRegistry: async () => ({ projectOwner: "owner" }), host: "codex", scope: "project", paths,
      selections: { defaults: [], optionals: ["beads-viewer"] }, runner: async () => { calls++; return { status: "passed" }; },
    }), /Beads|graph|terms/i);
    assert.equal(calls, 0);
    assert.equal(JSON.parse(await readFile(setupPath, "utf8")).version, 0);
  }
  await writeFile(setupPath, JSON.stringify({ skill: "agent-team", projectId: "p", version: 0, tracker: { kind: "beads" } }));
  let calls = 0;
  await assert.rejects(prepareDependencies({ setupPath, expectedVersion: 0, writer: { id: "owner", role: "project_orchestrator" },
    operationId: "bv-via-defaults", loadRegistry: async () => ({ projectOwner: "owner" }), host: "codex", scope: "project", paths,
    selections: { defaults: ["beads-viewer"] }, runner: async () => { calls++; return { status: "passed" }; },
  }), /terms/i);
  assert.equal(calls, 0);
});

test("a missing explicitly configured bv path is preserved without running its installer", async (t) => {
  const paths = await fixture(t);
  const setupPath = path.join(paths.projectRoot, "setup.json");
  const executable = path.join(paths.projectRoot, "custom", "bv");
  await writeFile(setupPath, JSON.stringify({ skill: "agent-team", projectId: "p", version: 0, tracker: { kind: "beads" },
    dashboard: { graph: { enabled: true, termsAcknowledged: true, executable } },
  }));
  const bvPhases = [];
  const result = await prepareDependencies({ setupPath, expectedVersion: 0, writer: { id: "owner", role: "project_orchestrator" },
    operationId: "preserve-bv", loadRegistry: async () => ({ projectOwner: "owner" }), host: "codex", scope: "project", paths,
    selections: { defaults: [], optionals: ["beads-viewer"] }, runner: async ({ dependency, phase }) => {
      if (dependency.id === "beads-viewer") { bvPhases.push(phase); return { status: "not_found" }; }
      return { status: "passed", version: dependency.version };
    },
  });
  assert.deepEqual(bvPhases, ["probe"]);
  assert.equal(result.receipts.find(({ id }) => id === "beads-viewer").installed, "preserved");
  await assert.rejects(access(path.dirname(executable)), { code: "ENOENT" });
});

test("selected bv preparation binds its scoped executable without inventing fresh-worker readiness", async (t) => {
  const paths = await fixture(t);
  const setupPath = path.join(paths.projectRoot, "setup.json");
  await writeFile(setupPath, JSON.stringify({ skill: "agent-team", projectId: "p", version: 0,
    tracker: { kind: "beads", executable: "/selected/bd" }, dashboard: { custom: "keep", graph: { enabled: true, termsAcknowledged: true } },
  }));
  const result = await prepareDependencies({ setupPath, expectedVersion: 0, writer: { id: "owner", role: "project_orchestrator" },
    operationId: "prepare-bv", loadRegistry: async () => ({ projectOwner: "owner" }), host: "codex", scope: "project", paths,
    selections: { defaults: [], optionals: ["beads-viewer"] }, runner: async ({ dependency, phase }) => {
      if (dependency.id === "beads-viewer") {
        assert.equal(dependency.trackerExecutable, "/selected/bd");
        if (phase === "worker") return { status: "unverified" };
      }
      return { status: "passed", version: dependency.version };
    },
  });
  const saved = JSON.parse(await readFile(setupPath, "utf8"));
  const receipt = result.receipts.find(({ id }) => id === "beads-viewer");
  assert.equal(receipt.functional, "passed");
  assert.equal(receipt.availableToWorker, "unknown");
  assert.equal(saved.dashboard.graph.executable, receipt.executable);
  assert.ok(receipt.executable.startsWith(paths.toolRoot + path.sep));
  assert.equal(path.basename(receipt.executable), process.platform === "win32" ? "bv.exe" : "bv");
  assert.equal(saved.dashboard.custom, "keep");
  assert.equal(inspectDependencies({ setup: saved, host: "codex" }).groups.find(({ id }) => id === "skills").failed, 0);
});

test("bv installer verifies pinned bytes, preserves license and refuses existing release paths", async (t) => {
  const paths = await fixture(t);
  const dependency = CATALOG_BY_ID.get("beads-viewer");
  assert.equal(dependency.install.kind, "github-release");
  assert.equal(dependency.version, "0.24.1");
  assert.equal(buildPreparationPlan({ dependencyId: dependency.id, host: "codex", scope: "project", paths }).install[0].file, "download-verified-release");
  const stage = path.join(paths.projectRoot, "archive");
  await mkdir(stage);
  await writeFile(path.join(stage, "bv"), "#!/bin/sh\necho bv 0.24.1\n", { mode: 0o755 });
  await writeFile(path.join(stage, "LICENSE"), "Complete fixture license and rider\n");
  const archivePath = path.join(paths.projectRoot, "fixture.tar.gz");
  execFileSync("tar", ["-czf", archivePath, "-C", stage, "bv", "LICENSE"]);
  const bytes = await readFile(archivePath);
  const digest = createHash("sha256").update(bytes).digest("hex");
  const originalFetch = globalThis.fetch;
  t.after(() => { globalThis.fetch = originalFetch; });
  let corrupt = true;
  let requests = 0;
  globalThis.fetch = async (url) => {
    requests++;
    const asset = path.basename(String(url).replace(/\.sha256$/, ""));
    return new Response(String(url).endsWith(".sha256") ? `${corrupt ? "0".repeat(64) : digest}  ${asset}\n` : bytes);
  };
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths });
  const failed = await runner({ dependency, phase: "install" });
  assert.equal(failed.status, "failed");
  assert.match(failed.evidence, /checksum/i);
  await assert.rejects(access(path.join(paths.toolRoot, "bv-0.24.1")), { code: "ENOENT" });
  corrupt = false;
  const originalLink = fsPromises.link;
  fsPromises.link = async (source, target) => {
    if (path.basename(target) === "bv") throw Object.assign(new Error("injected binary publication failure"), { code: "EIO" });
    return originalLink(source, target);
  };
  syncBuiltinESMExports();
  try {
    const interrupted = await runner({ dependency, phase: "install" });
    assert.equal(interrupted.status, "failed");
    assert.match(interrupted.evidence, /publication failure/);
    await assert.rejects(access(path.join(paths.toolRoot, "bv-0.24.1")), { code: "ENOENT" });
  } finally {
    fsPromises.link = originalLink;
    syncBuiltinESMExports();
  }
  const installed = await runner({ dependency, phase: "install" });
  assert.equal(installed.status, "passed", installed.evidence);
  assert.equal(await readFile(path.join(paths.toolRoot, "bv-0.24.1", "LICENSE"), "utf8"), "Complete fixture license and rider\n");
  const before = requests;
  assert.equal((await runner({ dependency, phase: "install" })).status, "customized");
  assert.equal(requests, before);
});
