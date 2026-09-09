import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readFile, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

test("catalog selects mandatory, defaults, tracker dependency, and explicit optionals while excluding rejected tools", async () => {
  const { resolveCatalogSelection } = await import("../hooks/lib/dependencies.mjs").catch(() => ({}));
  const result = resolveCatalogSelection?.({ tracker: { kind: "beads" }, optionals: ["context7"] });

  assert.deepEqual(result.selected.map(({ id }) => id), [
    "uv", "serena", "playwright-cli", "ast-grep", "lean-ctx", "superpowers", "ponytail", "impeccable", "react-best-practices", "beads", "context7",
  ]);
  assert.deepEqual(result.optional.map(({ id }) => id), ["project-kickoff", "beads-viewer"]);
  assert.deepEqual(result.excluded.map(({ id }) => id), [
    "vercel-agent-browser", "rtk", "beads-rust", "backlog-md", "gsd", "ralph", "caveman-runtime", "matt-tdd", "matt-diagnosing-bugs", "matt-code-review", "ast-grep-companion", "ponytail-gain",
  ]);
  assert.equal(new Set(result.selected.map(({ id }) => id)).size, result.selected.length);
  assert.deepEqual(result.optional.find(({ id }) => id === "project-kickoff").install.paths, ["."]);
});

test("preparation follows prerequisite order and saves truthful component receipts", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-deps-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, '{"skill":"agent-team","projectId":"p","version":2,"tracker":{"kind":"markdown","path":"TASKS.md"},"custom":{"keep":true}}\n');
  const calls = [];
  const installed = new Set();
  const runner = async ({ dependency, phase, check }) => {
    calls.push(`${dependency.id}:${phase}${check ? `:${check}` : ""}`);
    if (phase === "probe") return installed.has(dependency.id) ? { status: "passed", version: dependency.version } : { status: "not_found" };
    if (phase === "install") installed.add(dependency.id);
    return { status: "passed", version: dependency.version, evidence: `${dependency.id}-${phase}` };
  };

  const result = await prepareDependencies({
    setupPath,
    expectedVersion: 2,
    writer: { id: "setup-owner", role: "project_orchestrator" },
    operationId: "prepare-1",
    loadRegistry: async () => ({ projectOwner: "setup-owner" }),
    host: "codex",
    scope: "project",
    selections: { defaults: ["ast-grep"], optionals: [], declined: ["context7"] },
    paths: { projectRoot: directory, toolRoot: path.join(directory, ".agent-team", "tools"), skillRoot: path.join(directory, ".agents", "skills") },
    runner,
  });
  const saved = JSON.parse(await readFile(setupPath, "utf8"));

  assert.equal(result.status, "ready");
  assert.deepEqual(result.receipts.map(({ id }) => id), ["uv", "serena", "playwright-cli", "ast-grep"]);
  assert.deepEqual(calls.filter((call) => call.endsWith(":install")), ["uv:install", "serena:install", "playwright-cli:install", "ast-grep:install"]);
  assert.ok(calls.indexOf("uv:functional:command") < calls.indexOf("serena:install"));
  assert.ok(calls.indexOf("playwright-cli:functional:browser-interaction") < calls.indexOf("ast-grep:install"));
  for (const receipt of result.receipts) {
    assert.equal(receipt.detected, true);
    assert.equal(receipt.installed, "installed");
    assert.equal(receipt.functional, "passed");
    assert.equal(receipt.availableToWorker, "passed");
    assert.equal(receipt.status, "ready");
  }
  assert.equal(saved.version, 3);
  assert.deepEqual(saved.custom, { keep: true });
  assert.equal(saved.dependencies.hosts.codex.scope, "project");
  assert.deepEqual(saved.dependencies.hosts.codex.declined, ["context7"]);
});

test("compatible installs are reused only after fresh functional and worker discovery checks", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-deps-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, '{"skill":"agent-team","projectId":"p","version":1,"tracker":{"kind":"markdown","path":"TASKS.md"}}\n');
  const calls = [];
  const runner = async ({ dependency, phase }) => {
    calls.push(`${dependency.id}:${phase}`);
    if (phase === "probe") return { status: "passed", version: dependency.version, evidence: "found on selected path" };
    return { status: "passed", version: dependency.version, evidence: `${phase} passed` };
  };

  const result = await prepareDependencies({
    setupPath,
    expectedVersion: 1,
    writer: { id: "setup-owner", role: "project_orchestrator" },
    operationId: "prepare-2",
    loadRegistry: async () => ({ projectOwner: "setup-owner" }),
    host: "claude-code",
    scope: "user",
    selections: { defaults: [], optionals: [] },
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") },
    runner,
  });

  assert.ok(!calls.some((call) => call.endsWith(":install")));
  assert.ok(result.receipts.every(({ installed }) => installed === "reused"));
  assert.ok(result.receipts.every(({ functional, availableToWorker }) => functional === "passed" && availableToWorker === "passed"));
});

test("a version probe or install cannot hide a failed functional gate", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-deps-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, '{"skill":"agent-team","projectId":"p","version":1,"tracker":{"kind":"markdown","path":"TASKS.md"}}\n');
  const runner = async ({ dependency, phase, check }) => {
    if (phase === "probe") return { status: "passed", version: dependency.version };
    if (phase === "functional" && dependency.id === "serena") return { status: "failed", evidence: "symbol lookup returned no result" };
    return { status: "passed", version: dependency.version, evidence: `${check ?? phase} passed` };
  };

  const result = await prepareDependencies({
    setupPath,
    expectedVersion: 1,
    writer: { id: "setup-owner", role: "project_orchestrator" },
    operationId: "prepare-3",
    loadRegistry: async () => ({ projectOwner: "setup-owner" }),
    host: "codex",
    scope: "project",
    selections: { defaults: [], optionals: [] },
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") },
    runner,
  });
  const serena = result.receipts.find(({ id }) => id === "serena");

  assert.equal(result.status, "incomplete");
  assert.equal(serena.detected, true);
  assert.equal(serena.installed, "reused");
  assert.equal(serena.functional, "failed");
  assert.equal(serena.availableToWorker, "not_run");
  assert.equal(serena.status, "failed");
  assert.match(serena.boundary, /symbol lookup returned no result/);
});

test("customized skill paths are preserved and reported as an authority boundary", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-deps-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, '{"skill":"agent-team","projectId":"p","version":1,"tracker":{"kind":"markdown","path":"TASKS.md"}}\n');
  const installs = [];
  const runner = async ({ dependency, phase }) => {
    if (phase === "probe" && dependency.id === "superpowers") return { status: "customized", evidence: "unmanaged local edits" };
    if (phase === "probe") return { status: "passed", version: dependency.version };
    if (phase === "install") installs.push(dependency.id);
    return { status: "passed", version: dependency.version };
  };

  const result = await prepareDependencies({
    setupPath,
    expectedVersion: 1,
    writer: { id: "setup-owner", role: "project_orchestrator" },
    operationId: "prepare-4",
    loadRegistry: async () => ({ projectOwner: "setup-owner" }),
    host: "codex",
    scope: "project",
    selections: { defaults: ["superpowers"], optionals: [] },
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") },
    runner,
  });
  const receipt = result.receipts.find(({ id }) => id === "superpowers");

  assert.ok(!installs.includes("superpowers"));
  assert.equal(receipt.status, "cannot_use");
  assert.equal(receipt.functional, "not_run");
  assert.match(receipt.boundary, /unmanaged local edits/);
});

test("Beads preparation uses the selected tracker executable and the current pinned package", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-deps-"));
  const setupPath = path.join(directory, "setup.json");
  const executable = "/opt/project-tools/bd";
  await writeFile(setupPath, `${JSON.stringify({ skill: "agent-team", projectId: "p", version: 1, tracker: { kind: "beads", executable } })}\n`);
  const seen = [];
  const runner = async ({ dependency, phase }) => {
    if (dependency.id === "beads") seen.push({ phase, executable: dependency.executable, version: dependency.version });
    return { status: "passed", version: dependency.version };
  };

  await prepareDependencies({
    setupPath,
    expectedVersion: 1,
    writer: { id: "owner", role: "project_orchestrator" },
    operationId: "beads-prepare",
    loadRegistry: async () => ({ projectOwner: "owner" }),
    host: "codex",
    scope: "project",
    selections: { defaults: [], optionals: [] },
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") },
    runner,
  });

  assert.ok(seen.length > 0);
  assert.ok(seen.every((item) => item.executable === executable));
  assert.ok(seen.every((item) => item.version === "1.2.2"));
});

test("instruction selection is task-specific and never reopens an approved plan", async () => {
  const { selectInstructions } = await import("../hooks/lib/dependencies.mjs");

  assert.deepEqual(selectInstructions({ role: "developer", task: { kind: "backend", planning: "approved" } }), [
    "superpowers:test-driven-development", "superpowers:systematic-debugging", "ponytail",
  ]);
  assert.deepEqual(selectInstructions({ role: "visual_reviewer", task: { kind: "react", planning: "approved" } }), [
    "superpowers:verification-before-completion", "impeccable", "react-best-practices", "playwright-cli",
  ]);
  assert.ok(!selectInstructions({ role: "developer", task: { kind: "backend", planning: "approved" } }).some((id) => /brainstorm|impeccable|react|playwright/.test(id)));
  assert.ok(selectInstructions({ role: "project_orchestrator", task: { kind: "planning", planning: "unresolved" } }).includes("superpowers:brainstorming"));
});

test("dependency inspection groups recorded evidence without writing or running preparation", async () => {
  const { inspectDependenciesFile } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-deps-"));
  const setupPath = path.join(directory, "setup.json");
  const source = `${JSON.stringify({
    skill: "agent-team", projectId: "p", version: 8,
    dependencies: { hosts: { codex: { scope: "project", selected: ["serena", "playwright-cli", "superpowers"], receipts: [
      { id: "serena", status: "ready", functional: "passed", availableToWorker: "passed" },
      { id: "playwright-cli", status: "failed", functional: "failed", availableToWorker: "not_run", boundary: "browser launch failed" },
      { id: "superpowers", status: "ready", functional: "passed", availableToWorker: "passed" },
    ] } } },
  }, null, 2)}\n`;
  await writeFile(setupPath, source);

  const overview = await inspectDependenciesFile({ setupPath, host: "codex" });

  assert.equal(await readFile(setupPath, "utf8"), source);
  assert.deepEqual(overview.groups.map(({ id, ready, failed }) => ({ id, ready, failed })), [
    { id: "runtimes_tools", ready: 1, failed: 1 },
    { id: "skills", ready: 1, failed: 0 },
    { id: "project_readiness", ready: 0, failed: 1 },
  ]);
  assert.deepEqual(overview.unresolved, [{ id: "playwright-cli", boundary: "browser launch failed" }]);
});

test("pinned preparation plans target the selected scope and avoid broad initializers", async () => {
  const { buildPreparationPlan } = await import("../hooks/lib/dependencies.mjs");
  const paths = { projectRoot: "/work/project", toolRoot: "/work/project/.agent-team/tools", skillRoot: "/work/project/.agents/skills" };

  const playwright = buildPreparationPlan({ dependencyId: "playwright-cli", host: "codex", scope: "project", paths });
  const lean = buildPreparationPlan({ dependencyId: "lean-ctx", host: "codex", scope: "project", paths });
  const impeccable = buildPreparationPlan({ dependencyId: "impeccable", host: "claude-code", scope: "project", paths });

  assert.deepEqual(playwright.install[0], {
    file: "npm",
    args: ["install", "--global", "--prefix", paths.toolRoot, "--no-audit", "--no-fund", "@playwright/cli@0.1.19"],
  });
  assert.deepEqual(playwright.install.slice(1).map(({ args }) => args), [
    ["--help"], ["install", "--skills=agents"], ["install-browser", "chromium"],
  ]);
  assert.equal(playwright.functional.id, "browser-interaction");
  assert.ok(playwright.functional.steps.some((step) => step.includes("click")));
  assert.deepEqual(impeccable.install.at(-1).args, ["install", "-y", "--providers=claude", "--scope=project", "--no-hooks"]);
  assert.equal(lean.install.length, 1);
  assert.ok(!JSON.stringify(lean.install).match(/\b(init|wrap|onboard|setup|proxy)\b/));
  assert.equal(lean.profile.coordination, false);
});

test("successful installer output is not ready until a post-install version probe passes", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-deps-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, '{"skill":"agent-team","projectId":"p","version":1,"tracker":{"kind":"markdown","path":"TASKS.md"}}\n');
  const probes = new Map();
  const runner = async ({ dependency, phase }) => {
    if (phase === "probe") {
      const count = (probes.get(dependency.id) ?? 0) + 1;
      probes.set(dependency.id, count);
      if (count === 1) return { status: "not_found" };
      if (dependency.id === "serena") return { status: "passed", version: "0.0.1", evidence: "wrong binary" };
      return { status: "passed", version: dependency.version };
    }
    return { status: "passed", version: dependency.version };
  };

  const result = await prepareDependencies({
    setupPath, expectedVersion: 1, operationId: "post-probe",
    writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
    host: "codex", scope: "project", selections: { defaults: [], optionals: [] },
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") }, runner,
  });
  const serena = result.receipts.find(({ id }) => id === "serena");

  assert.ok([...probes.values()].every((count) => count === 2));
  assert.equal(serena.status, "failed");
  assert.equal(serena.detected, true);
  assert.equal(serena.functional, "not_run");
  assert.match(serena.boundary, /expected 1\.7\.0.*0\.0\.1/);
});

test("an incompatible selected Beads executable is preserved instead of installing a shadow tracker", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-deps-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, `${JSON.stringify({ skill: "agent-team", projectId: "p", version: 1, tracker: { kind: "beads", executable: "/custom/bd" } })}\n`);
  const installed = [];
  const runner = async ({ dependency, phase }) => {
    if (phase === "probe") return { status: "passed", version: dependency.id === "beads" ? "0.9.0" : dependency.version };
    if (phase === "install") installed.push(dependency.id);
    return { status: "passed", version: dependency.version };
  };

  const result = await prepareDependencies({
    setupPath, expectedVersion: 1, operationId: "beads-mismatch",
    writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
    host: "codex", scope: "project", selections: { defaults: [], optionals: [] },
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") }, runner,
  });
  const beads = result.receipts.find(({ id }) => id === "beads");

  assert.ok(!installed.includes("beads"));
  assert.equal(beads.status, "cannot_use");
  assert.match(beads.boundary, /selected executable.*0\.9\.0.*1\.2\.2/);
});

test("invalid mandatory declines fail before any dependency side effect", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-deps-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, '{"skill":"agent-team","projectId":"p","version":1,"tracker":{"kind":"markdown","path":"TASKS.md"}}\n');
  let calls = 0;

  await assert.rejects(() => prepareDependencies({
    setupPath, expectedVersion: 1, operationId: "bad-decline",
    writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
    host: "codex", scope: "project", selections: { defaults: [], optionals: [], declined: ["serena"] },
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") },
    runner: async () => { calls += 1; return { status: "passed" }; },
  }), /cannot be declined/);
  assert.equal(calls, 0);
});

test("default Serena gate performs an MCP symbol operation in an isolated fixture", async () => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const { CATALOG_BY_ID } = await import("../hooks/lib/dependency-catalog.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-serena-"));
  const server = path.join(directory, "serena-fixture-server.mjs");
  await writeFile(server, `#!/usr/bin/env node
import readline from "node:readline";
const lines = readline.createInterface({ input: process.stdin });
lines.on("line", line => {
  const message = JSON.parse(line);
  if (message.method === "initialize") process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id: message.id, result: { protocolVersion: "2025-06-18", capabilities: { tools: {} }, serverInfo: { name: "fixture", version: "1" } } }) + "\\n");
  if (message.method === "tools/list") process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id: message.id, result: { tools: [{ name: "find_symbol" }] } }) + "\\n");
  if (message.method === "tools/call") process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id: message.id, result: { content: [{ type: "text", text: "readinessFixtureSymbol" }] } }) + "\\n");
});
`);
  await chmod(server, 0o755);
  const base = CATALOG_BY_ID.get("serena");
  const dependency = { ...base, executable: server };
  const paths = { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") };
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths });

  const result = await runner({ dependency, phase: "functional", check: "symbol-operation" });

  assert.equal(result.status, "passed");
  assert.match(result.evidence, /find_symbol.*readinessFixtureSymbol/);
});

test("selective skill installation preflights every destination before copying", async () => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-skill-preflight-"));
  const skillRoot = path.join(directory, "skills");
  await mkdir(path.join(skillRoot, "second"), { recursive: true });
  await writeFile(path.join(skillRoot, "second", "SKILL.md"), "# Local customization\n");
  const dependency = {
    id: "fixture-skills",
    version: "abc123",
    install: {
      kind: "git-skill",
      repository: "https://invalid.example/should-not-be-contacted.git",
      source: "https://invalid.example/should-not-be-contacted",
      revision: "abc123",
      paths: ["skills/first", "skills/second"],
    },
  };
  const runner = createDependencyRunner({
    host: "codex",
    scope: "project",
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot },
  });

  const result = await runner({ dependency, phase: "install" });

  assert.equal(result.status, "customized");
  assert.match(result.evidence, /second/);
  await assert.rejects(readFile(path.join(skillRoot, "first", "SKILL.md")), { code: "ENOENT" });
});

test("default LeanCTX gate performs only a narrow isolated read", async () => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const { CATALOG_BY_ID } = await import("../hooks/lib/dependency-catalog.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-leanctx-"));
  const executable = path.join(directory, "lean-ctx-fixture.mjs");
  await writeFile(executable, `#!/usr/bin/env node
if (process.argv[2] !== "read") process.exit(9);
process.stdout.write("readinessLeanCtxMarker\\n");
`);
  await chmod(executable, 0o755);
  const runner = createDependencyRunner({
    host: "codex",
    scope: "project",
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") },
  });

  const result = await runner({ dependency: { ...CATALOG_BY_ID.get("lean-ctx"), executable }, phase: "functional", check: "narrow-read-recovery" });

  assert.equal(result.status, "passed");
  assert.match(result.evidence, /narrow read.*readinessLeanCtxMarker/i);
});

test("default Impeccable gate verifies all three documented detector exits", async () => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const { CATALOG_BY_ID } = await import("../hooks/lib/dependency-catalog.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-impeccable-"));
  const executable = path.join(directory, "impeccable-fixture.mjs");
  await writeFile(executable, `#!/usr/bin/env node
const target = process.argv.at(-1);
if (target.includes("finding")) process.exit(2);
if (target.includes("missing")) process.exit(1);
process.stdout.write("clean\\n");
`);
  await chmod(executable, 0o755);
  const runner = createDependencyRunner({
    host: "codex",
    scope: "project",
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") },
  });

  const result = await runner({ dependency: { ...CATALOG_BY_ID.get("impeccable"), executable }, phase: "functional", check: "detector-exit-contract" });

  assert.equal(result.status, "passed");
  assert.match(result.evidence, /0.*2.*1/);
});

test("selected Beads gate initializes, writes concurrently, and exports an isolated tracker", async () => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const { CATALOG_BY_ID } = await import("../hooks/lib/dependency-catalog.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-beads-"));
  const executable = path.join(directory, "bd-fixture.mjs");
  await writeFile(executable, `#!/usr/bin/env node
const args = process.argv.slice(2);
if (args[0] === "init") process.exit(0);
if (args[0] === "create") { process.stdout.write(JSON.stringify({ id: args.includes("writer-a") ? "A" : "B" })); process.exit(0); }
if (args[0] === "export") { process.stdout.write('{"title":"writer-a"}\\n{"title":"writer-b"}\\n'); process.exit(0); }
process.exit(9);
`);
  await chmod(executable, 0o755);
  const runner = createDependencyRunner({
    host: "codex",
    scope: "project",
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") },
  });

  const result = await runner({ dependency: { ...CATALOG_BY_ID.get("beads"), executable }, phase: "functional", check: "atomic-tracker-write" });

  assert.equal(result.status, "passed");
  assert.match(result.evidence, /concurrent.*export/i);
});

test("malformed external runner results become failed receipts instead of aborting setup", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-malformed-runner-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, '{"skill":"agent-team","projectId":"p","version":1,"tracker":{"kind":"markdown","path":"TASKS.md"}}\n');

  const result = await prepareDependencies({
    setupPath,
    expectedVersion: 1,
    writer: { id: "owner", role: "project_orchestrator" },
    operationId: "malformed-runner",
    loadRegistry: async () => ({ projectOwner: "owner" }),
    host: "codex",
    scope: "project",
    selections: { defaults: [], optionals: [] },
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") },
    runner: async () => undefined,
  });

  assert.equal(result.status, "incomplete");
  assert.ok(result.receipts.every(({ status }) => status === "failed"));
  assert.match(result.receipts[0].boundary, /malformed/i);
});

test("Serena preparation returns an exact owned host-registration contract", async () => {
  const { buildPreparationPlan } = await import("../hooks/lib/dependencies.mjs");
  const paths = { projectRoot: "/workspace/project", toolRoot: "/workspace/project/.agent-team/tools", skillRoot: "/workspace/project/.agents/skills" };

  const codex = buildPreparationPlan({ dependencyId: "serena", host: "codex", scope: "project", paths });
  const claude = buildPreparationPlan({ dependencyId: "serena", host: "claude-code", scope: "project", paths });

  assert.deepEqual(codex.registration, {
    kind: "mcp-server",
    name: "serena",
    command: "/workspace/project/.agent-team/tools/bin/serena",
    args: ["start-mcp-server", "--context", "codex", "--project", "/workspace/project"],
    scope: "project",
    ownership: "agent-team-entry-only",
    status: "host-approval-required",
    boundary: "Merge only the owned Serena entry, complete host trust/reload, then verify it from a fresh worker.",
  });
  assert.equal(claude.registration.args[2], "claude-code");
});

test("Playwright functional gate uses the documented headless default and closes its owned session", async () => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const { CATALOG_BY_ID } = await import("../hooks/lib/dependency-catalog.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-playwright-"));
  const executable = path.join(directory, "playwright-fixture.mjs");
  await writeFile(executable, `#!/usr/bin/env node
const args = process.argv.slice(2);
if (args.includes("--headless")) process.exit(9);
if (args.includes("eval")) process.stdout.write("yes\\n");
`);
  await chmod(executable, 0o755);
  const runner = createDependencyRunner({
    host: "codex",
    scope: "project",
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") },
  });

  const dependency = { ...CATALOG_BY_ID.get("playwright-cli"), executable };
  const result = await runner({ dependency, phase: "functional", check: dependency.functionalCheck });

  assert.equal(result.status, "passed");
  assert.match(result.evidence, /clicked.*state change/i);
});
