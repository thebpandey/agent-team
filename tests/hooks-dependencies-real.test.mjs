import assert from "node:assert/strict";
import { mkdtemp, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { CATALOG_BY_ID } from "../hooks/lib/dependency-catalog.mjs";
import { createDependencyRunner } from "../hooks/lib/dependencies.mjs";

const enabled = process.env.AGENT_TEAM_REAL_DEPS === "1";

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
