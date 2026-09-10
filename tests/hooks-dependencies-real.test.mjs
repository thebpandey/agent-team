import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { access, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { promisify } from "node:util";
import { CATALOG_BY_ID } from "../hooks/lib/dependency-catalog.mjs";
import { createDependencyRunner } from "../hooks/lib/dependencies.mjs";

const enabled = process.env.AGENT_TEAM_REAL_DEPS === "1";
const exec = promisify(execFile);

test("real isolated ast-grep package passes positive and negative structural fixtures", { skip: !enabled }, async (t) => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-real-deps-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
  const dependency = CATALOG_BY_ID.get("ast-grep");
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths });

  assert.equal((await runner({ dependency, phase: "install" })).status, "passed");
  const probe = await runner({ dependency, phase: "probe" });
  const functional = await runner({ dependency, phase: "functional", check: dependency.functionalCheck });

  assert.equal(probe.version, "0.45.3");
  assert.deepEqual(functional, { status: "passed", evidence: "Positive and negative structural fixtures passed." });
});

test("real pinned bv release preserves its license and renders fresh selected Beads dependencies", { skip: !enabled || !process.env.AGENT_TEAM_REAL_BD }, async (t) => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-real-bv-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
  const dependency = { ...CATALOG_BY_ID.get("beads-viewer"), trackerExecutable: process.env.AGENT_TEAM_REAL_BD };
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths });
  const installed = await runner({ dependency, phase: "install" });
  assert.equal(installed.status, "passed", installed.evidence);
  assert.equal((await runner({ dependency, phase: "probe" })).version, dependency.version);
  const license = await readFile(path.join(paths.toolRoot, `bv-${dependency.version}`, "LICENSE"), "utf8");
  assert.match(license, /Jeffrey Emanuel/);
  assert.match(license, /OpenAI.*Anthropic/);
  const functional = await runner({ dependency, phase: "functional", check: dependency.functionalCheck });
  assert.equal(functional.status, "passed", functional.evidence);
});

test("real pinned Superpowers source prepares only selected complete skills", { skip: !enabled }, async (t) => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-real-skills-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
  const dependency = CATALOG_BY_ID.get("superpowers");
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths });

  assert.equal((await runner({ dependency, phase: "install" })).status, "passed");
  assert.equal((await runner({ dependency, phase: "probe" })).version, dependency.version);
  assert.equal((await runner({ dependency, phase: "functional", check: dependency.functionalCheck })).status, "passed");
});

test("real selected LeanCTX executable passes its isolated narrow-read gate", { skip: !enabled || !process.env.AGENT_TEAM_REAL_LEAN_CTX }, async (t) => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-real-leanctx-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
  const dependency = { ...CATALOG_BY_ID.get("lean-ctx"), executable: process.env.AGENT_TEAM_REAL_LEAN_CTX };
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths });

  const functional = await runner({ dependency, phase: "functional", check: dependency.functionalCheck });

  assert.equal(functional.status, "passed", functional.evidence);
});

test("real selected Beads executable passes isolated concurrent write and export", { skip: !enabled || !process.env.AGENT_TEAM_REAL_BD }, async (t) => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-real-beads-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
  const dependency = { ...CATALOG_BY_ID.get("beads"), executable: process.env.AGENT_TEAM_REAL_BD };
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths });

  const functional = await runner({ dependency, phase: "functional", check: dependency.functionalCheck });

  assert.equal(functional.status, "passed", functional.evidence);
});

test("real Beads verification ignores contaminated tracker and Git routing", { skip: !enabled || !process.env.AGENT_TEAM_REAL_BD }, async (t) => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-real-beads-routing-"));
  const sentinel = path.join(root, "sentinel");
  await mkdir(sentinel, { recursive: true });
  await writeFile(path.join(sentinel, "sentinel.txt"), "preserve\n");
  await exec("git", ["init", "--quiet"], { cwd: sentinel });
  t.after(() => rm(root, { recursive: true, force: true }));
  const keys = ["BEADS_DIR", "BD_DB", "BD_DOLT_HOST", "GT_DOLT_DATA", "GIT_DIR", "GIT_WORK_TREE"];
  const previous = Object.fromEntries(keys.map((key) => [key, process.env[key]]));
  Object.assign(process.env, {
    BEADS_DIR: path.join(sentinel, ".beads"),
    BD_DB: path.join(sentinel, "redirect.db"),
    BD_DOLT_HOST: "sentinel.invalid",
    GT_DOLT_DATA: path.join(sentinel, "dolt-data"),
    GIT_DIR: path.join(sentinel, ".git"),
    GIT_WORK_TREE: sentinel,
  });
  const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
  const dependency = { ...CATALOG_BY_ID.get("beads"), executable: process.env.AGENT_TEAM_REAL_BD };
  try {
    const runner = createDependencyRunner({ host: "codex", scope: "project", paths });
    const functional = await runner({ dependency, phase: "functional", check: dependency.functionalCheck });
    assert.equal(functional.status, "passed", functional.evidence);
  } finally {
    for (const [key, value] of Object.entries(previous)) {
      if (value === undefined) delete process.env[key];
      else process.env[key] = value;
    }
  }

  assert.equal(await readFile(path.join(sentinel, "sentinel.txt"), "utf8"), "preserve\n");
  await assert.rejects(access(path.join(sentinel, ".beads")), { code: "ENOENT" });
});

test("real isolated Impeccable package passes its documented detector exit contract", { skip: !enabled }, async (t) => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-real-impeccable-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
  const dependency = CATALOG_BY_ID.get("impeccable");
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths });

  assert.equal((await runner({ dependency, phase: "install" })).status, "passed");
  assert.equal((await runner({ dependency, phase: "probe" })).version, dependency.version);
  const functional = await runner({ dependency, phase: "functional", check: dependency.functionalCheck });

  assert.equal(functional.status, "passed", functional.evidence);
});

test("real isolated Serena package performs an MCP symbol lookup", { skip: !enabled || process.env.AGENT_TEAM_REAL_SERENA !== "1" }, async (t) => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-real-serena-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths });
  const uv = CATALOG_BY_ID.get("uv");
  const serena = CATALOG_BY_ID.get("serena");

  assert.equal((await runner({ dependency: uv, phase: "install" })).status, "passed");
  assert.equal((await runner({ dependency: uv, phase: "probe" })).version, uv.version);
  assert.equal((await runner({ dependency: serena, phase: "install" })).status, "passed");
  assert.equal((await runner({ dependency: serena, phase: "probe" })).version, serena.version);
  const functional = await runner({ dependency: serena, phase: "functional", check: serena.functionalCheck });

  assert.equal(functional.status, "passed", functional.evidence);
});

test("real isolated Graphify package builds and traverses an offline code graph", { skip: !enabled }, async (t) => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-real-graphify-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths });
  const uv = CATALOG_BY_ID.get("uv");
  const graphify = CATALOG_BY_ID.get("graphify");

  assert.equal((await runner({ dependency: uv, phase: "install" })).status, "passed");
  const installed = await runner({ dependency: graphify, phase: "install" });
  assert.equal(installed.status, "passed", installed.evidence);
  assert.equal((await runner({ dependency: graphify, phase: "probe" })).version, graphify.version);
  const functional = await runner({ dependency: graphify, phase: "functional", check: graphify.functionalCheck });

  assert.equal(functional.status, "passed", functional.evidence);
});

test("real pinned Project Kickoff source prepares its complete independent skill", { skip: !enabled }, async (t) => {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-real-kickoff-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
  const dependency = CATALOG_BY_ID.get("project-kickoff");
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths });

  assert.equal((await runner({ dependency, phase: "install" })).status, "passed");
  assert.equal((await runner({ dependency, phase: "probe" })).version, dependency.version);
  assert.equal((await runner({ dependency, phase: "functional", check: dependency.functionalCheck })).status, "passed");
});

test("real pinned Ponytail and React sources prepare only their selected complete skills", { skip: !enabled }, async (t) => {
  for (const id of ["ponytail", "react-best-practices"]) {
    await t.test(id, async (t) => {
      const root = await mkdtemp(path.join(os.tmpdir(), `agent-team-real-${id}-`));
      t.after(() => rm(root, { recursive: true, force: true }));
      const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
      const dependency = CATALOG_BY_ID.get(id);
      const runner = createDependencyRunner({ host: "codex", scope: "project", paths });

      assert.equal((await runner({ dependency, phase: "install" })).status, "passed");
      assert.equal((await runner({ dependency, phase: "probe" })).version, dependency.version);
      assert.equal((await runner({ dependency, phase: "functional", check: dependency.functionalCheck })).status, "passed");
    });
  }
});
