import assert from "node:assert/strict";
import { execFileSync, spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { chmod, lstat, mkdir, mkdtemp, readFile, readdir, rm, symlink, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

test("Serena probes separate its documented Git display suffix from the pinned package version", async (t) => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-version-probe-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const executable = path.join(directory, "version-fixture");
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths: {
    projectRoot: directory, toolRoot: directory, skillRoot: directory,
  } });
  for (const [id, output, expected] of [
    ["serena", "Serena 1.7.0-ef4f7385", "1.7.0"],
    ["serena", "Serena 1.7.0-ef4f7385-dirty", "1.7.0"],
    ["serena", "Serena 1.7.0", "1.7.0"],
    ["serena", "Serena 1.6.0-ef4f7385", "1.6.0"],
    ["serena", "Serena 1.7.0-rc1", "1.7.0-rc1"],
    ["serena", "Serena 1.7.0-ef4f7385-custom", "1.7.0-ef4f7385-custom"],
    ["another-tool", "Version 1.7.0-ef4f7385", "1.7.0-ef4f7385"],
  ]) {
    await writeFile(executable, `#!${process.execPath}\nprocess.stdout.write(${JSON.stringify(`${output}\n`)});\n`, { mode: 0o755 });
    const result = await runner({ dependency: { id, executable }, phase: "probe" });
    assert.equal(result.status, "passed");
    assert.equal(result.version, expected, output);
    assert.equal(result.stdout, `${output}\n`, "retain the complete reported version as evidence");
  }
});

test("catalog selects mandatory, defaults, tracker dependency, and explicit optionals while excluding rejected tools", async () => {
  const { resolveCatalogSelection } = await import("../hooks/lib/dependencies.mjs").catch(() => ({}));
  const result = resolveCatalogSelection?.({ tracker: { kind: "beads" }, optionals: ["context7"] });

  assert.deepEqual(result.selected.map(({ id }) => id), [
    "uv", "serena", "playwright-cli", "ast-grep", "graphify", "lean-ctx", "superpowers", "ponytail", "impeccable", "react-best-practices", "beads", "context7",
  ]);
  assert.deepEqual(result.optional.map(({ id }) => id), ["project-kickoff", "beads-viewer"]);
  assert.deepEqual(result.excluded.map(({ id }) => id), [
    "vercel-agent-browser", "rtk", "beads-rust", "backlog-md", "gsd", "ralph", "caveman-runtime", "matt-tdd", "matt-diagnosing-bugs", "matt-code-review", "ast-grep-companion", "ponytail-gain",
  ]);
  assert.equal(new Set(result.selected.map(({ id }) => id)).size, result.selected.length);
  assert.deepEqual(result.optional.find(({ id }) => id === "project-kickoff").install.paths, ["."]);
});

test("persisted required Graphify survives omitted defaults and an explicit decline remains unresolved", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-required-graphify-"));
  const prepare = async (name, declined = []) => {
    const setupPath = path.join(root, `${name}.json`);
    await writeFile(setupPath, `${JSON.stringify({ skill: "agent-team", projectId: "p", version: 1,
      tracker: { kind: "markdown", path: "TASKS.md" }, plan: { requiredCapabilities: ["graphify", "future-capability"] } })}\n`);
    const calls = [];
    const result = await prepareDependencies({ setupPath, expectedVersion: 1, operationId: name,
      writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
      host: "codex", scope: "project", selections: { defaults: [], declined },
      paths: { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") },
      runner: async ({ dependency, phase }) => { calls.push(`${dependency.id}:${phase}`); return { status: "passed", version: dependency.version }; } });
    return { result, saved: JSON.parse(await readFile(setupPath, "utf8")), calls };
  };

  const selected = await prepare("required");
  assert.deepEqual(selected.saved.dependencies.hosts.codex.selected, ["uv", "serena", "playwright-cli", "graphify"]);
  assert.equal(selected.calls.filter((call) => call === "graphify:probe").length, 1);
  assert.ok(selected.calls.indexOf("uv:worker") < selected.calls.indexOf("graphify:probe"));
  const declined = await prepare("declined", ["graphify"]);
  assert.ok(declined.saved.dependencies.hosts.codex.selected.includes("graphify"));
  assert.equal(declined.result.receipts.find(({ id }) => id === "graphify").status, "required_unavailable");
  assert.ok(!declined.calls.some((call) => call.startsWith("graphify:")));
});

test("only an unavailable declared prerequisite blocks Graphify", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const run = async (name, failure) => {
    const root = await mkdtemp(path.join(os.tmpdir(), `agent-team-prerequisite-${name}-`));
    const setupPath = path.join(root, "setup.json");
    await writeFile(setupPath, `${JSON.stringify({ skill: "agent-team", projectId: "p", version: 1, tracker: { kind: "markdown", path: "TASKS.md" } })}\n`);
    const calls = [];
    const result = await prepareDependencies({ setupPath, expectedVersion: 1, operationId: name,
      writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
      host: "codex", scope: "project", selections: { defaults: ["graphify", "lean-ctx"] },
      paths: { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") },
      runner: async ({ dependency, phase }) => { calls.push(`${dependency.id}:${phase}`);
        if (dependency.id === failure.id && phase === failure.phase) return { status: "failed", evidence: "fixture failure" };
        return { status: "passed", version: dependency.version }; } });
    return { result, calls };
  };
  const leanFailure = await run("lean", { id: "lean-ctx", phase: "functional" });
  assert.equal(leanFailure.result.receipts.find(({ id }) => id === "graphify").status, "ready");
  assert.ok(leanFailure.calls.includes("graphify:worker"));
  const uvFailure = await run("uv", { id: "uv", phase: "functional" });
  const graphify = uvFailure.result.receipts.find(({ id }) => id === "graphify");
  assert.equal(graphify.status, "required_unavailable");
  assert.equal(graphify.availableToWorker, "unknown", "a skipped worker check is unknown, not a fabricated failure");
  assert.match(graphify.boundary, /uv/);
  assert.ok(!uvFailure.calls.some((call) => call.startsWith("graphify:")));
});

test("sidecar-free exact skills are reused unowned and incompatible paths are preserved", async () => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-compatible-skill-"));
  const skillRoot = path.join(root, "skills");
  const destination = path.join(skillRoot, "fixture-skill");
  const bytes = Buffer.from("---\nname: fixture-skill\n---\nExact fixture.\n");
  await mkdir(destination, { recursive: true });
  await writeFile(path.join(destination, "SKILL.md"), bytes);
  await writeFile(path.join(destination, "NOTES.md"), "preserve\n");
  const dependency = { id: "fixture", version: "abc", functionalCheck: "complete-selective-skills", install: {
    kind: "git-skill", source: "https://example.invalid/fixture", revision: "abc", paths: ["skills/fixture-skill"] },
    compatibility: { kind: "required-files", entrypoint: "SKILL.md", allowUnrelatedRegularFiles: true, selectedPaths: [{
      selectedPath: "skills/fixture-skill", requiredFiles: [{ path: "SKILL.md", digest: { algorithm: "sha256",
        value: createHash("sha256").update(bytes).digest("hex") } }] }] } };
  const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot };
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths, workerDiscovery: async () => ({ status: "passed" }) });
  const exact = await runner({ dependency, phase: "probe" });
  assert.equal(exact.status, "passed");
  assert.equal(exact.installed, "reused_unowned");
  assert.equal(exact.lifecycleOwnership, "unowned");
  assert.equal(exact.path, destination);
  assert.equal(exact.components[0].compatibility.status, "exact");
  assert.equal(await readFile(path.join(destination, "NOTES.md"), "utf8"), "preserve\n");

  await mkdir(path.join(destination, "nested"));
  await writeFile(path.join(destination, "nested", "SKILL.md"), bytes);
  const ambiguous = await runner({ dependency, phase: "probe" });
  assert.equal(ambiguous.status, "manual_action");
  assert.match(ambiguous.evidence, /entrypoint/i);
  await rm(path.join(destination, "nested"), { recursive: true });

  const provenance = path.join(destination, "operator-provenance.json");
  await writeFile(provenance, JSON.stringify({ source: dependency.install.source, revision: dependency.version,
    selectedPath: dependency.install.paths[0] }));
  await symlink("operator-provenance.json", path.join(destination, ".agent-team-source.json"));
  const linkedSidecar = await runner({ dependency, phase: "probe" });
  assert.equal(linkedSidecar.status, "manual_action");
  assert.equal(linkedSidecar.lifecycleOwnership, "unowned");
  await rm(path.join(destination, ".agent-team-source.json"));
  await rm(provenance);

  await writeFile(path.join(destination, "SKILL.md"), "edited\n");
  const changed = await runner({ dependency, phase: "probe" });
  assert.equal(changed.status, "manual_action");
  assert.match(changed.evidence, /SKILL\.md/);
  assert.equal(changed.path, destination);
  assert.equal(changed.lifecycleOwnership, "unowned");
  assert.equal(changed.components[0].compatibility.status, "incompatible");
  await rm(path.join(destination, "SKILL.md"));
  await symlink("NOTES.md", path.join(destination, "SKILL.md"));
  assert.equal((await runner({ dependency, phase: "probe" })).status, "manual_action");
});

test("Impeccable guidance is inspected separately and exact existing bytes are reused unowned", async (t) => {
  const { createDependencyRunner, prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-impeccable-guidance-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const skillRoot = path.join(root, "skills");
  const destination = path.join(skillRoot, "impeccable");
  const bytes = Buffer.from("---\nname: impeccable\n---\nPinned guidance.\n");
  await mkdir(destination, { recursive: true });
  await writeFile(path.join(destination, "SKILL.md"), bytes);
  await writeFile(path.join(destination, "NOTES.md"), "preserve\n");
  const toolRoot = path.join(root, "tools");
  const executable = path.join(toolRoot, "bin", "impeccable");
  await mkdir(path.dirname(executable), { recursive: true });
  await writeFile(executable, `#!${process.execPath}\nprocess.stdout.write("impeccable 4.1.0\\n");\n`, { mode: 0o755 });
  const dependency = {
    id: "impeccable", version: "4.1.0", install: { kind: "npm", source: "fixture" },
    guidance: { repository: "fixture", source: "fixture", revision: "guidance-revision",
      selectedPaths: { codex: "skills/impeccable" }, gitBlobs: {
      codex: createHash("sha1").update(Buffer.concat([Buffer.from(`blob ${bytes.length}\0`), bytes])).digest("hex"),
    } },
  };
  const runner = createDependencyRunner({ host: "codex", scope: "project",
    paths: { projectRoot: root, toolRoot, skillRoot } });
  const cli = await runner({ dependency, phase: "probe" });
  assert.equal(cli.status, "passed");
  assert.equal(cli.components[0].id, "executable");
  assert.equal(cli.components[0].lifecycleOwnership, "unowned");
  const exact = await runner({ dependency, phase: "companion" });
  assert.equal(exact.status, "passed");
  assert.equal(exact.installed, "reused_unowned");
  assert.equal(exact.lifecycleOwnership, "unowned");
  assert.equal(exact.components[0].path, destination);
  assert.equal(await readFile(path.join(destination, "NOTES.md"), "utf8"), "preserve\n");

  await writeFile(path.join(destination, "SKILL.md"), "edited\n");
  const edited = await runner({ dependency, phase: "companion" });
  assert.equal(edited.status, "manual_action");
  assert.equal(await readFile(path.join(destination, "SKILL.md"), "utf8"), "edited\n");

  const setupPath = path.join(root, "setup.json");
  await writeFile(setupPath, `${JSON.stringify({ skill: "agent-team", projectId: "p", version: 1,
    tracker: { kind: "markdown", path: "TASKS.md" } })}\n`);
  const prepared = await prepareDependencies({ setupPath, expectedVersion: 1, operationId: "impeccable-components",
    writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
    host: "codex", scope: "project", selections: { defaults: ["impeccable"] },
    paths: { projectRoot: root, toolRoot, skillRoot }, runner: async ({ dependency: selected, phase }) => {
      if (selected.id !== "impeccable") return { status: "passed", version: selected.version };
      if (phase === "probe") return { status: "passed", version: selected.version, installed: "reused", lifecycleOwnership: "managed",
        path: executable, paths: [executable], components: [{ id: "executable", path: executable, installed: "reused", lifecycleOwnership: "managed" }] };
      if (phase === "companion") return { status: "passed", installed: "reused_unowned", lifecycleOwnership: "unowned",
        path: destination, paths: [destination], components: [{ id: "guidance", path: destination, installed: "reused_unowned", lifecycleOwnership: "unowned" }] };
      return { status: "passed" };
    } });
  const receipt = prepared.receipts.find(({ id }) => id === "impeccable");
  assert.deepEqual(receipt.components.map(({ id, lifecycleOwnership }) => ({ id, lifecycleOwnership })), [
    { id: "executable", lifecycleOwnership: "managed" }, { id: "guidance", lifecycleOwnership: "unowned" },
  ]);
  assert.equal(receipt.lifecycleOwnership, "unowned", "a managed CLI must not confer lifecycle authority on unowned guidance");
});

test("only Impeccable receives the closed 256-entry guidance allowance", async (t) => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const bytes = Buffer.from("---\nname: bounded\n---\n");
  for (const id of ["impeccable", "ordinary"]) await t.test(id, async () => {
    const root = await mkdtemp(path.join(os.tmpdir(), `agent-team-${id}-bounds-`));
    t.after(() => rm(root, { recursive: true, force: true }));
    const destination = path.join(root, "skills", id);
    await mkdir(destination, { recursive: true });
    await writeFile(path.join(destination, "SKILL.md"), bytes);
    for (let index = 0; index < 128; index += 1) await writeFile(path.join(destination, `file-${index}.txt`), "x");
    const selectedPath = `skills/${id}`;
    const dependency = { id, version: "pinned", install: { kind: "git-skill", source: "fixture", revision: "pinned", paths: [selectedPath] },
      compatibility: { kind: "required-files", entrypoint: "SKILL.md", allowUnrelatedRegularFiles: true, selectedPaths: [{ selectedPath,
        requiredFiles: [{ path: "SKILL.md", digest: { algorithm: "sha256", value: createHash("sha256").update(bytes).digest("hex") } }] }] } };
    const runner = createDependencyRunner({ host: "codex", scope: "project",
      paths: { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") } });
    assert.equal((await runner({ dependency, phase: "probe" })).status, id === "impeccable" ? "passed" : "manual_action");
    if (id === "impeccable") {
      for (let index = 128; index < 255; index += 1) await writeFile(path.join(destination, `file-${index}.txt`), "x");
      assert.equal((await runner({ dependency, phase: "probe" })).status, "passed");
      await writeFile(path.join(destination, "file-255.txt"), "x");
      assert.equal((await runner({ dependency, phase: "probe" })).status, "manual_action");
    }
  });
});

test("multi-path manual classification retains every selected conservative component", async () => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-multi-manual-"));
  const skillRoot = path.join(root, "skills");
  const bytes = Buffer.from("# exact\n");
  const paths = ["skills/first", "skills/second"];
  for (const selected of paths) {
    const destination = path.join(skillRoot, path.basename(selected));
    await mkdir(destination, { recursive: true });
    await writeFile(path.join(destination, "SKILL.md"), selected.endsWith("first") ? "edited\n" : bytes);
  }
  const dependency = { id: "multi", version: "1", install: { kind: "git-skill", source: "fixture", revision: "1", paths },
    compatibility: { kind: "required-files", entrypoint: "SKILL.md", allowUnrelatedRegularFiles: true, selectedPaths: paths.map((selectedPath) => ({
      selectedPath, requiredFiles: [{ path: "SKILL.md", digest: { algorithm: "sha256", value: createHash("sha256").update(bytes).digest("hex") } }],
    })) } };
  await writeFile(path.join(skillRoot, "second", ".agent-team-source.json"), JSON.stringify({
    source: dependency.install.source, revision: dependency.version, selectedPath: "skills/second",
  }));
  const runner = createDependencyRunner({ host: "codex", scope: "project",
    paths: { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot } });
  const result = await runner({ dependency, phase: "probe" });
  assert.equal(result.status, "manual_action");
  assert.deepEqual(result.paths, paths.map((selected) => path.join(skillRoot, path.basename(selected))));
  assert.equal(result.components.length, 2);
  assert.deepEqual(result.components.map(({ lifecycleOwnership, installed }) => ({ lifecycleOwnership, installed })), [
    { lifecycleOwnership: "unowned", installed: "preserved" },
    { lifecycleOwnership: "managed", installed: "reused" },
  ]);
  assert.equal(result.lifecycleOwnership, "unowned");
});

test("probe ownership metadata survives functional and worker readiness failures", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-receipt-metadata-"));
  const setupPath = path.join(root, "setup.json");
  await writeFile(setupPath, `${JSON.stringify({ skill: "agent-team", projectId: "p", version: 1,
    tracker: { kind: "markdown", path: "TASKS.md" }, plan: { requiredCapabilities: ["graphify"] } })}\n`);
  const selectedPath = path.join(root, "tools", "bin", "graphify");
  const component = { id: "executable", path: selectedPath, realpath: selectedPath, installed: "reused_unowned",
    lifecycleOwnership: "unowned", compatibility: { status: "exact", entrypoint: null, requiredFiles: [], unrelatedRegularFilesPreserved: false } };
  const result = await prepareDependencies({ setupPath, expectedVersion: 1, operationId: "metadata",
    writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
    host: "codex", scope: "project", selections: { defaults: [] },
    paths: { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") },
    runner: async ({ dependency, phase }) => dependency.id === "graphify" && phase === "probe"
      ? { status: "passed", version: dependency.version, installed: "reused_unowned", lifecycleOwnership: "unowned",
          path: selectedPath, paths: [selectedPath], components: [component] }
      : dependency.id === "graphify" && phase === "functional" ? { status: "failed", evidence: "graph traversal failed" }
      : { status: "passed", version: dependency.version } });
  const graphify = result.receipts.find(({ id }) => id === "graphify");
  assert.equal(graphify.status, "failed");
  assert.equal(graphify.installed, "reused_unowned");
  assert.equal(graphify.lifecycleOwnership, "unowned");
  assert.deepEqual(graphify.paths, [selectedPath]);
  assert.deepEqual(graphify.components, [component]);
});

test("an exact canonical Graphify executable is reused unowned without invoking uv install", async () => {
  const { prepareDependencies, createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-canonical-graphify-"));
  const setupPath = path.join(root, "setup.json");
  const toolRoot = path.join(root, "tools");
  const executable = path.join(toolRoot, "bin", "graphify");
  await mkdir(path.dirname(executable), { recursive: true });
  await writeFile(executable, `#!${process.execPath}\nif (process.argv.includes('--version')) console.log('graphify 0.9.57');\n`, { mode: 0o755 });
  await writeFile(setupPath, `${JSON.stringify({ skill: "agent-team", projectId: "p", version: 1,
    tracker: { kind: "markdown", path: "TASKS.md" }, plan: { requiredCapabilities: ["graphify"] } })}\n`);
  const graphCalls = [];
  const actual = createDependencyRunner({ host: "codex", scope: "project", paths: { projectRoot: root, toolRoot, skillRoot: path.join(root, "skills") },
    functionalAdapters: { graphify: async ({ executable: selected }) => { graphCalls.push(`functional:${selected}`); return { status: "passed" }; } },
    workerDiscovery: async ({ dependency, executable: selected }) => { if (dependency.id === "graphify") graphCalls.push(`worker:${selected}`); return { status: "passed" }; } });
  const result = await prepareDependencies({ setupPath, expectedVersion: 1, operationId: "canonical-graphify",
    writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
    host: "codex", scope: "project", selections: { defaults: [] }, paths: { projectRoot: root, toolRoot, skillRoot: path.join(root, "skills") },
    runner: async (request) => { if (request.dependency.id === "graphify") { graphCalls.push(request.phase); return actual(request); }
      return { status: "passed", version: request.dependency.version }; } });
  const receipt = result.receipts.find(({ id }) => id === "graphify");
  assert.ok(!graphCalls.includes("install"));
  assert.equal(receipt.installed, "reused_unowned");
  assert.equal(receipt.lifecycleOwnership, "unowned");
  assert.equal(receipt.path, executable);
  assert.deepEqual(graphCalls.filter((call) => call.startsWith("functional:") || call.startsWith("worker:")), [`functional:${executable}`, `worker:${executable}`]);

  const dependency = { ...(await import("../hooks/lib/dependency-catalog.mjs")).CATALOG_BY_ID.get("graphify") };
  const direct = createDependencyRunner({ host: "codex", scope: "project",
    paths: { projectRoot: root, toolRoot, skillRoot: path.join(root, "skills") },
    functionalAdapters: { graphify: async () => ({ status: "passed" }) } });
  assert.equal((await direct({ dependency, phase: "probe" })).status, "passed");
  await rm(executable);
  await writeFile(executable, `#!${process.execPath}\nconsole.log('replacement');\n`, { mode: 0o755 });
  assert.equal((await direct({ dependency, phase: "functional", check: dependency.functionalCheck })).status, "manual_action");

  const actualRoot = path.join(root, "linked-tools-target");
  const linkedRoot = path.join(root, "linked-tools");
  await mkdir(path.join(actualRoot, "bin"), { recursive: true });
  await writeFile(path.join(actualRoot, "bin", "graphify"), `#!${process.execPath}\nconsole.log('graphify 0.9.57');\n`, { mode: 0o755 });
  await symlink(actualRoot, linkedRoot);
  const linkedRunner = createDependencyRunner({ host: "codex", scope: "project",
    paths: { projectRoot: root, toolRoot: linkedRoot, skillRoot: path.join(root, "skills") } });
  assert.equal((await linkedRunner({ dependency: { ...dependency }, phase: "probe" })).status, "manual_action");
});

test("canonical contained npm and uv shims retain one bound identity across phases", async (t) => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const { CATALOG_BY_ID } = await import("../hooks/lib/dependency-catalog.mjs");
  for (const fixture of [
    { id: "impeccable", packageRoot: ["lib", "node_modules", "impeccable"], target: ["cli", "bin", "cli.js"],
      metadata: ["package.json", JSON.stringify({ name: "impeccable", version: "4.1.0" })], output: "impeccable 4.1.0\n" },
    { id: "graphify", packageRoot: ["uv-tools", "graphifyy"], target: ["bin", "graphify"],
      metadata: ["lib/python3.12/site-packages/graphifyy-0.9.57.dist-info/METADATA", "Name: graphifyy\nVersion: 0.9.57\n"], output: "graphify 0.9.57\n" },
  ]) await t.test(fixture.id, async () => {
    const root = await mkdtemp(path.join(os.tmpdir(), `agent-team-${fixture.id}-shim-`));
    t.after(() => rm(root, { recursive: true, force: true }));
    const toolRoot = path.join(root, "tools");
    const packageRoot = path.join(toolRoot, ...fixture.packageRoot);
    const target = path.join(packageRoot, ...fixture.target);
    const executable = path.join(toolRoot, "bin", fixture.id);
    await mkdir(path.dirname(target), { recursive: true });
    await mkdir(path.dirname(path.join(packageRoot, fixture.metadata[0])), { recursive: true });
    await mkdir(path.dirname(executable), { recursive: true });
    await writeFile(target, `#!${process.execPath}\nprocess.stdout.write(${JSON.stringify(fixture.output)});\n`, { mode: 0o755 });
    await writeFile(path.join(packageRoot, fixture.metadata[0]), fixture.metadata[1]);
    await symlink(path.relative(path.dirname(executable), target), executable);
    const dependency = { ...CATALOG_BY_ID.get(fixture.id) };
    const identities = [];
    const runner = createDependencyRunner({ host: "codex", scope: "project",
      paths: { projectRoot: root, toolRoot, skillRoot: path.join(root, "skills") },
      functionalAdapters: { [fixture.id]: async ({ executableIdentity }) => { identities.push(executableIdentity); return { status: "passed" }; } },
      workerDiscovery: async ({ executableIdentity }) => { identities.push(executableIdentity); return { status: "passed" }; } });
    const probe = await runner({ dependency, phase: "probe" });
    assert.equal(probe.status, "passed", probe.evidence);
    assert.equal(probe.version, dependency.version);
    assert.equal((await runner({ dependency, phase: "functional", check: dependency.functionalCheck })).status, "passed");
    assert.equal((await runner({ dependency, phase: "worker" })).status, "passed");
    assert.ok(identities[0]);
    assert.deepEqual(identities[1], identities[0]);
    await writeFile(target, `#!${process.execPath}\nconsole.log('replacement');\n`, { mode: 0o755 });
    assert.equal((await runner({ dependency, phase: "functional", check: dependency.functionalCheck })).status, "manual_action");

    const outside = path.join(root, "outside", fixture.id);
    await mkdir(path.dirname(outside), { recursive: true });
    await writeFile(outside, `#!${process.execPath}\nconsole.log('escaped');\n`, { mode: 0o755 });
    await rm(executable);
    await symlink(path.relative(path.dirname(executable), outside), executable);
    assert.equal((await createDependencyRunner({ host: "codex", scope: "project",
      paths: { projectRoot: root, toolRoot, skillRoot: path.join(root, "skills") } })
      ({ dependency: { ...dependency }, phase: "probe" })).status, "manual_action");

    const unrelated = path.join(toolRoot, "unrelated", fixture.id);
    await mkdir(path.dirname(unrelated), { recursive: true });
    await writeFile(unrelated, `#!${process.execPath}\nconsole.log('unrelated');\n`, { mode: 0o755 });
    await rm(executable);
    await symlink(path.relative(path.dirname(executable), unrelated), executable);
    assert.equal((await createDependencyRunner({ host: "codex", scope: "project",
      paths: { projectRoot: root, toolRoot, skillRoot: path.join(root, "skills") } })
      ({ dependency: { ...dependency }, phase: "probe" })).status, "manual_action");
  });
});

test("missing Graphify and ast-grep reach their fresh installers before identity binding", async (t) => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const { CATALOG_BY_ID } = await import("../hooks/lib/dependency-catalog.mjs");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-absent-executable-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const toolRoot = path.join(root, "tools");
  const bin = path.join(toolRoot, "bin");
  await mkdir(bin, { recursive: true });
  const uv = path.join(bin, "uv");
  const graphify = path.join(bin, "graphify");
  await writeFile(uv, `#!${process.execPath}\nrequire('node:fs').writeFileSync(${JSON.stringify(graphify)},'#!${process.execPath}\\nconsole.log("graphify 0.9.57")\\n',{mode:0o755});\n`, { mode: 0o755 });
  const wrapper = path.join(root, "wrapper");
  await mkdir(wrapper);
  await writeFile(path.join(wrapper, "npm"), `#!${process.execPath}\nprocess.exit(7);\n`, { mode: 0o755 });
  const priorPath = process.env.PATH;
  process.env.PATH = `${wrapper}${path.delimiter}${priorPath}`;
  t.after(() => { process.env.PATH = priorPath; });
  const paths = { projectRoot: root, toolRoot, skillRoot: path.join(root, "skills") };
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths });
  assert.equal((await runner({ dependency: CATALOG_BY_ID.get("graphify"), phase: "install" })).status, "passed");
  assert.equal((await runner({ dependency: CATALOG_BY_ID.get("ast-grep"), phase: "install" })).status, "failed");
});

test("canonical incompatible Graphify is preserved without invoking its installer", async () => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const { CATALOG_BY_ID } = await import("../hooks/lib/dependency-catalog.mjs");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-incompatible-graphify-"));
  const toolRoot = path.join(root, "tools");
  const executable = path.join(toolRoot, "bin", "graphify");
  await mkdir(path.dirname(executable), { recursive: true });
  await writeFile(executable, `#!${process.execPath}\nconsole.log('graphify 0.1.0');\n`, { mode: 0o755 });
  const dependency = CATALOG_BY_ID.get("graphify");
  const actual = createDependencyRunner({ host: "codex", scope: "project",
    paths: { projectRoot: root, toolRoot, skillRoot: path.join(root, "skills") } });
  const calls = [];
  const setupPath = path.join(root, "setup.json");
  await writeFile(setupPath, `${JSON.stringify({ skill: "agent-team", projectId: "p", version: 1,
    tracker: { kind: "markdown", path: "TASKS.md" }, plan: { requiredCapabilities: ["graphify"] } })}\n`);
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const result = await prepareDependencies({ setupPath, expectedVersion: 1, operationId: "wrong-graphify",
    writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
    host: "codex", scope: "project", selections: { defaults: [] }, paths: { projectRoot: root, toolRoot, skillRoot: path.join(root, "skills") },
    runner: async (request) => { if (request.dependency.id === "graphify") { calls.push(request.phase); return actual(request); }
      return { status: "passed", version: request.dependency.version }; } });
  const receipt = result.receipts.find(({ id }) => id === "graphify");
  assert.equal(receipt.status, "manual_action");
  assert.deepEqual(calls, ["probe"]);
  assert.equal(receipt.path, executable);
  assert.equal(receipt.lifecycleOwnership, "unowned");
  assert.equal(await readFile(executable, "utf8"), `#!${process.execPath}\nconsole.log('graphify 0.1.0');\n`);
});

test("prepareOne preserves exact and manual race-time installation classifications", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-install-classification-"));
  const selectedPath = path.join(root, "skills", "using-superpowers");
  const component = { id: "skill", path: selectedPath, realpath: selectedPath, installed: "reused_unowned",
    lifecycleOwnership: "unowned", compatibility: { status: "exact", entrypoint: "SKILL.md", requiredFiles: [], unrelatedRegularFilesPreserved: true } };
  for (const [name, installation, expected] of [
    ["exact", { status: "passed", installed: "reused_unowned", lifecycleOwnership: "unowned", path: selectedPath, paths: [selectedPath], components: [component] }, "reused_unowned"],
    ["manual", { status: "manual_action", installed: "preserved", lifecycleOwnership: "unowned", path: selectedPath, paths: [selectedPath],
      components: [{ ...component, installed: "preserved", compatibility: { ...component.compatibility, status: "incompatible" } }] }, "manual_action"],
  ]) {
    const setupPath = path.join(root, `${name}.json`);
    await writeFile(setupPath, `${JSON.stringify({ skill: "agent-team", projectId: "p", version: 1, tracker: { kind: "markdown", path: "TASKS.md" } })}\n`);
    let installed = false;
    const result = await prepareDependencies({ setupPath, expectedVersion: 1, operationId: name,
      writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
      host: "codex", scope: "project", selections: { defaults: ["superpowers"] }, paths: { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") },
      runner: async ({ dependency, phase }) => dependency.id !== "superpowers" ? { status: "passed", version: dependency.version }
        : phase === "probe" ? installed ? installation : { status: "not_found" }
        : phase === "install" ? (installed = true, installation) : { status: "passed" } });
    const receipt = result.receipts.find(({ id }) => id === "superpowers");
    assert.equal(name === "exact" ? receipt.installed : receipt.status, expected);
    assert.equal(receipt.path, selectedPath);
    assert.equal(receipt.lifecycleOwnership, "unowned");
    assert.deepEqual(receipt.components, installation.components);
  }
});

test("ast-grep uses its canonical absolute entrypoint and accepts only a verified same-package sg alias", async () => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const { CATALOG_BY_ID } = await import("../hooks/lib/dependency-catalog.mjs");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-ast-grep-"));
  const toolRoot = path.join(root, "tools");
  const packageRoot = path.join(toolRoot, "lib", "node_modules", "@ast-grep", "cli");
  const target = path.join(packageRoot, "bin", "ast-grep");
  const canonical = path.join(toolRoot, "bin", "ast-grep");
  const alias = path.join(toolRoot, "bin", "sg");
  await mkdir(path.dirname(target), { recursive: true });
  await mkdir(path.dirname(canonical), { recursive: true });
  await writeFile(path.join(packageRoot, "package.json"), JSON.stringify({ name: "@ast-grep/cli", version: "0.45.3" }));
  await writeFile(target, `#!${process.execPath}\nconsole.log('ast-grep 0.45.3');\n`, { mode: 0o755 });
  await symlink(path.relative(path.dirname(canonical), target), canonical);
  await symlink(path.relative(path.dirname(alias), target), alias);
  const seen = [];
  const paths = { projectRoot: root, toolRoot, skillRoot: path.join(root, "skills") };
  const base = CATALOG_BY_ID.get("ast-grep");
  assert.equal(base.install.command, "ast-grep");
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths,
    functionalAdapters: { "ast-grep": async ({ executable }) => { seen.push(executable); return { status: "passed" }; } },
    workerDiscovery: async ({ executable }) => { seen.push(executable); return { status: "passed" }; } });
  await writeFile(path.join(packageRoot, "package.json"), JSON.stringify({ name: "other", version: "0.45.3" }));
  assert.equal((await runner({ dependency: { ...base }, phase: "probe" })).status, "manual_action", "canonical requests verify package identity too");
  await writeFile(path.join(packageRoot, "package.json"), JSON.stringify({ name: "@ast-grep/cli", version: "0.45.3" }));
  const selected = { ...base, executable: alias };
  const probe = await runner({ dependency: selected, phase: "probe" });
  assert.equal(probe.version, "0.45.3");
  assert.equal(probe.components[0].compatibility.identity.requested, alias);
  await writeFile(target, `#!${process.execPath}\nconsole.log('replacement 0.45.3');\n`, { mode: 0o755 });
  assert.equal((await runner({ dependency: selected, phase: "functional", check: base.functionalCheck })).status, "manual_action");
  await writeFile(target, `#!${process.execPath}\nconsole.log('ast-grep 0.45.3');\n`, { mode: 0o755 });
  assert.equal((await runner({ dependency: { ...base, executable: alias }, phase: "probe" })).version, "0.45.3");
  assert.equal((await runner({ dependency: { ...base, executable: alias }, phase: "worker" })).status, "passed");
  assert.deepEqual(seen, [canonical]);
  assert.equal((await runner({ dependency: { ...base, executable: "/usr/bin/sg" }, phase: "probe" })).status, "manual_action");
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
    "superpowers:test-driven-development", "superpowers:systematic-debugging", "ponytail", "graphify",
  ]);
  assert.deepEqual(selectInstructions({ role: "visual_reviewer", task: { kind: "react", planning: "approved" } }), [
    "superpowers:verification-before-completion", "impeccable", "react-best-practices", "playwright-cli",
  ]);
  assert.ok(!selectInstructions({ role: "developer", task: { kind: "backend", planning: "approved" } }).some((id) => /brainstorm|impeccable|react|playwright/.test(id)));
  assert.ok(selectInstructions({ role: "project_orchestrator", task: { kind: "planning", planning: "unresolved" } }).includes("superpowers:brainstorming"));
  // The structural graph is for code, and routine single-file work does not need it.
  for (const role of ["reviewer", "project_orchestrator"]) {
    assert.ok(selectInstructions({ role, task: { kind: "backend", planning: "approved" } }).includes("graphify"), role);
  }
  assert.ok(!selectInstructions({ role: "developer", task: { kind: "text", planning: "approved" } }).includes("graphify"));
  assert.ok(!selectInstructions({ role: "routine_developer", task: { kind: "backend", planning: "approved" } }).includes("graphify"));
});

test("dependency inspection groups recorded evidence without writing or running preparation", async () => {
  const { inspectDependenciesFile } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-deps-"));
  const setupPath = path.join(directory, "setup.json");
  const source = `${JSON.stringify({
    skill: "agent-team", projectId: "p", version: 8,
    dependencies: { hosts: { codex: { scope: "project", selected: ["serena", "playwright-cli", "graphify", "superpowers"], receipts: [
      { id: "serena", status: "ready", functional: "passed", availableToWorker: "passed" },
      { id: "playwright-cli", status: "failed", functional: "failed", availableToWorker: "not_run", boundary: "browser launch failed" },
      { id: "graphify", status: "ready", functional: "passed", availableToWorker: "passed" },
      { id: "superpowers", status: "ready", functional: "passed", availableToWorker: "passed" },
    ] } } },
  }, null, 2)}\n`;
  await writeFile(setupPath, source);

  const overview = await inspectDependenciesFile({ setupPath, host: "codex" });

  assert.equal(await readFile(setupPath, "utf8"), source);
  // Graphify is a prepared CLI, so its receipt belongs with the runtimes and tools, not the skills.
  assert.deepEqual(overview.groups.map(({ id, ready, failed }) => ({ id, ready, failed })), [
    { id: "runtimes_tools", ready: 2, failed: 1 },
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
  assert.deepEqual(playwright.install.slice(1), []);
  assert.deepEqual(playwright.browserInstall.args, ["install-browser", "chromium"]);
  assert.equal(playwright.browserInstall.env.PLAYWRIGHT_BROWSERS_PATH, path.join(paths.toolRoot, "playwright-browsers"));
  assert.equal(playwright.functional.id, "browser-interaction");
  assert.ok(playwright.functional.steps.some((step) => step.includes("click")));
  assert.deepEqual(impeccable.install, [{
    file: "npm",
    args: ["install", "--global", "--prefix", paths.toolRoot, "--no-audit", "--no-fund", "impeccable@4.1.0"],
  }], "the CLI install must not run the broad guidance initializer before inspecting its destination");
  assert.equal(lean.install.length, 1);
  assert.ok(!JSON.stringify(lean.install).match(/\b(init|wrap|onboard|setup|proxy)\b/));
  assert.equal(lean.profile.coordination, false);
});

test("Graphify preparation installs the pinned uv tool without any host installer or hook step", async () => {
  const { buildPreparationPlan } = await import("../hooks/lib/dependencies.mjs");
  const paths = { projectRoot: "/work/project", toolRoot: "/work/project/.agent-team/tools", skillRoot: "/work/project/.agents/skills" };

  const plan = buildPreparationPlan({ dependencyId: "graphify", host: "claude-code", scope: "user", paths });

  assert.deepEqual(plan.install, [{
    file: path.join(paths.toolRoot, "bin", "uv"),
    args: ["tool", "install", "--python", "3.12", "graphifyy==0.9.57"],
  }]);
  assert.equal(plan.registration, undefined);
  assert.equal(plan.profile.codeOnly, true);
  assert.equal(plan.profile.llmBackend, false);
  assert.ok(!JSON.stringify(plan.install).match(/\b(hook|claude|codex)\b/));
  assert.equal(plan.functional.id, "code-graph-traversal");
  assert.ok(plan.functional.steps.some((step) => step.includes("--code-only --no-viz")));
});

// Mirrors pinned 0.9.57 output: ids, parenthesised labels, relations and the traversal text all
// come from the graph the fixture wrote, so a wrong operand or a wrong edge cannot still pass.
const GRAPHIFY_FIXTURE = `#!/usr/bin/env node
import { appendFileSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
// A backend key in the environment would make extraction non-deterministic and online.
for (const key of ${JSON.stringify(["ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENAI_API_KEY", "MOONSHOT_API_KEY", "DEEPSEEK_API_KEY", "OLLAMA_BASE_URL"])}) {
  if (process.env[key] !== undefined) process.exit(7);
}
const args = process.argv.slice(2);
appendFileSync(process.argv[1] + ".calls", JSON.stringify(args) + "\\n");
const variant = readFileSync(process.argv[1] + ".variant", "utf8").trim();
const edge = (source, target, relation, {
  confidence = "EXTRACTED",
  confidenceScore = 1.0,
  origin = "ast",
  context,
} = {}) => ({
  source, target, relation, confidence, confidence_score: confidenceScore, _origin: origin,
  ...(context ? { context } : {}),
});
if (args[0] === "extract") {
  mkdirSync("graphify-out", { recursive: true });
  writeFileSync("graphify-out/graph.json", JSON.stringify({
    directed: true, multigraph: false, graph: {}, hyperedges: [],
    nodes: [
      { id: "db_pool", label: "Pool", _origin: "ast" }, { id: "db_pool_connect", label: ".connect()", _origin: "ast" },
      { id: "app_start_server", label: "start_server()", _origin: "ast" }, { id: "app_load_config", label: "load_config()", _origin: "ast" },
      { id: "app_dispatch", label: "dispatch()", _origin: "ast" }, { id: "app_handler", label: "handler()", _origin: "ast" },
    ],
    links: [
      edge("db_pool", "db_pool_connect", "method"),
      ...(variant === "missing-edge" ? [] : [edge("db_pool_connect", "app_start_server", "calls")]),
      edge("app_start_server", "app_load_config", "calls"),
      edge("app_dispatch", "app_handler", "indirect_call", {
        confidence: "INFERRED",
        confidenceScore: 0.85,
        origin: variant === "semantic-origin" ? "semantic" : "ast",
        context: "argument",
      }),
      ...(variant === "missing-origin" ? [{ source: "db_pool", target: "app_load_config", relation: "calls", confidence: "EXTRACTED", confidence_score: 1.0 }] : []),
    ],
  }));
  process.exit(0);
}
const graph = JSON.parse(readFileSync("graphify-out/graph.json", "utf8"));
const labelOf = (id) => graph.nodes.find((node) => node.id === id).label;
const plain = (label) => label.replace(/^\\./, "").replace(/\\(\\)$/, "");
const find = (name) => graph.nodes.find((node) => node.label === name || plain(node.label) === name);
if (args[0] === "path") {
  const [from, to] = [find(args[1]), find(args[2])];
  if (!from || !to) process.exit(3);
  const trails = [[from.id]];
  const seen = new Set([from.id]);
  while (trails.length) {
    const trail = trails.shift();
    const last = trail.at(-1);
    if (last === to.id) {
      const hops = trail.slice(1).map((id, index) => {
        const link = graph.links.find(({ source, target }) => [source, target].includes(trail[index]) && [source, target].includes(id));
        return \`--\${link.relation} [\${link.confidence}]--> \${labelOf(id)}\`;
      });
      process.stdout.write(\`Shortest path (\${hops.length} hops):\\n  \${[labelOf(trail[0]), ...hops].join(" ")}\\n\`);
      process.exit(0);
    }
    for (const { source, target } of graph.links) {
      for (const [a, b] of [[source, target], [target, source]]) {
        if (a === last && !seen.has(b)) { seen.add(b); trails.push([...trail, b]); }
      }
    }
  }
  process.exit(4);
}
if (args[0] === "explain") {
  const node = find(args[1]);
  if (!node) process.exit(3);
  const connections = graph.links.filter(({ source, target }) => [source, target].includes(node.id))
    .map((link) => link.source === node.id
      ? \`  --> \${labelOf(link.target)} [\${link.relation}] [\${link.confidence}]\`
      : \`  <-- \${labelOf(link.source)} [\${link.relation}] [\${link.confidence}]\`);
  process.stdout.write(\`Node: \${node.label}\\n  ID:        \${node.id}\\nConnections (\${connections.length}):\\n\${connections.join("\\n")}\\n\`);
  process.exit(0);
}
process.exit(9);
`;

test("default Graphify gate extracts an offline code graph and traverses it", async (t) => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const { CATALOG_BY_ID } = await import("../hooks/lib/dependency-catalog.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-graphify-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const previous = process.env.ANTHROPIC_API_KEY;
  process.env.ANTHROPIC_API_KEY = "sentinel-must-not-reach-graphify";
  t.after(() => {
    if (previous === undefined) delete process.env.ANTHROPIC_API_KEY;
    else process.env.ANTHROPIC_API_KEY = previous;
  });
  const gate = async (variant) => {
    const root = path.join(directory, variant);
    const executable = path.join(root, "tools", "bin", "graphify");
    await mkdir(path.dirname(executable), { recursive: true });
    await writeFile(executable, GRAPHIFY_FIXTURE);
    await writeFile(`${executable}.variant`, `${variant}\n`);
    await chmod(executable, 0o755);
    const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
    const runner = createDependencyRunner({ host: "codex", scope: "project", paths });
    const result = await runner({ dependency: { ...CATALOG_BY_ID.get("graphify"), executable }, phase: "functional", check: "code-graph-traversal" });
    const calls = (await readFile(`${executable}.calls`, "utf8")).trim().split("\n").map((line) => JSON.parse(line));
    return { result, calls };
  };

  const complete = await gate("complete");
  const semanticOrigin = await gate("semantic-origin");
  const missingOrigin = await gate("missing-origin");
  const missing = await gate("missing-edge");

  assert.equal(complete.result.status, "passed", complete.result.evidence);
  assert.match(complete.result.evidence, /AST-origin.*inferred callback.*Pool -> load_config.*explain start_server/);
  assert.deepEqual(complete.calls, [
    ["extract", ".", "--code-only", "--no-viz"],
    ["path", "Pool", "load_config"],
    ["explain", "start_server"],
  ]);
  // Semantic provenance is never valid in a code-only extraction, even when the edge is inferred.
  assert.equal(semanticOrigin.result.status, "failed");
  assert.match(semanticOrigin.result.evidence, /semantic provenance; expected AST-origin graph content only/);
  assert.deepEqual(semanticOrigin.calls, [["extract", ".", "--code-only", "--no-viz"]]);
  // Missing provenance must fail closed rather than being treated as AST-derived.
  assert.equal(missingOrigin.result.status, "failed");
  assert.match(missingOrigin.result.evidence, /missing provenance; expected AST-origin graph content only/);
  assert.deepEqual(missingOrigin.calls, [["extract", ".", "--code-only", "--no-viz"]]);
  // A graph without the known call edge is not a usable map, however well the CLI exits.
  assert.equal(missing.result.status, "failed");
  assert.match(missing.result.evidence, /\.connect\(\) --calls--> start_server\(\)/);
  assert.deepEqual(missing.calls, [["extract", ".", "--code-only", "--no-viz"]]);
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

test("invalid prerequisite declines fail before any dependency side effect", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-deps-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, '{"skill":"agent-team","projectId":"p","version":1,"tracker":{"kind":"markdown","path":"TASKS.md"}}\n');
  let calls = 0;

  await assert.rejects(() => prepareDependencies({
    setupPath, expectedVersion: 1, operationId: "bad-decline",
    writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
    host: "codex", scope: "project", selections: { defaults: [], optionals: [], declined: ["uv"] },
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

  assert.equal(result.status, "manual_action");
  assert.match(result.evidence, /second/);
  await assert.rejects(readFile(path.join(skillRoot, "first", "SKILL.md")), { code: "ENOENT" });
});

test("skill installation reclassifies destinations that appear after the absent preflight", async (t) => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-skill-race-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = path.join(root, "source");
  const skillRoot = path.join(root, "skills");
  const toolRoot = path.join(root, "tools");
  const destination = path.join(skillRoot, "fixture");
  const bytes = Buffer.from("# exact\n");
  await mkdir(path.join(source, "skills", "fixture"), { recursive: true });
  await writeFile(path.join(source, "skills", "fixture", "SKILL.md"), bytes);
  execFileSync("git", ["init", "--quiet"], { cwd: source });
  execFileSync("git", ["add", "."], { cwd: source });
  execFileSync("git", ["-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "-m", "fixture"], { cwd: source });
  const revision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: source, encoding: "utf8" }).trim();
  const realGit = execFileSync("which", ["git"], { encoding: "utf8" }).trim();
  const wrapperRoot = path.join(root, "wrapper");
  await mkdir(wrapperRoot);
  await writeFile(path.join(wrapperRoot, "git"), `#!${process.execPath}\nconst {spawnSync}=require('node:child_process');const fs=require('node:fs');const r=spawnSync(${JSON.stringify(realGit)},process.argv.slice(2),{stdio:'inherit'});if(process.argv.includes('checkout')){fs.mkdirSync(process.env.RACE_DEST,{recursive:true});fs.writeFileSync(process.env.RACE_DEST+'/SKILL.md',process.env.RACE_BYTES)}process.exit(r.status??1);\n`, { mode: 0o755 });
  const dependency = { id: "fixture", version: revision, install: { kind: "git-skill", repository: source, source, revision,
    paths: ["skills/fixture"] }, compatibility: { kind: "required-files", entrypoint: "SKILL.md", allowUnrelatedRegularFiles: true,
      selectedPaths: [{ selectedPath: "skills/fixture", requiredFiles: [{ path: "SKILL.md", digest: { algorithm: "sha256",
        value: createHash("sha256").update(bytes).digest("hex") } }] }] } };
  const prior = { path: process.env.PATH, dest: process.env.RACE_DEST, bytes: process.env.RACE_BYTES };
  t.after(() => { process.env.PATH = prior.path; for (const [key, value] of [["RACE_DEST", prior.dest], ["RACE_BYTES", prior.bytes]]) {
    if (value === undefined) delete process.env[key]; else process.env[key] = value;
  } });
  process.env.PATH = `${wrapperRoot}${path.delimiter}${prior.path}`;
  process.env.RACE_DEST = destination;
  process.env.RACE_BYTES = bytes.toString();
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths: { projectRoot: root, toolRoot, skillRoot } });
  const exact = await runner({ dependency, phase: "install" });
  assert.equal(exact.installed, "reused_unowned");
  assert.equal(exact.lifecycleOwnership, "unowned");
  assert.equal(await readFile(path.join(destination, "SKILL.md"), "utf8"), bytes.toString());
  await rm(destination, { recursive: true });
  process.env.RACE_BYTES = "operator edit\n";
  const incompatible = await runner({ dependency, phase: "install" });
  assert.equal(incompatible.status, "manual_action");
  assert.equal(incompatible.lifecycleOwnership, "unowned");
  assert.equal(await readFile(path.join(destination, "SKILL.md"), "utf8"), "operator edit\n");
});

test("fresh local git skills publish completely as installed and prevalidate every source", async (t) => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-fresh-git-skill-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = path.join(root, "source");
  const skillRoot = path.join(root, "skills");
  await mkdir(path.join(source, "skills", "first"), { recursive: true });
  await writeFile(path.join(source, "skills", "first", "SKILL.md"), "# first\n");
  execFileSync("git", ["init", "--quiet"], { cwd: source });
  execFileSync("git", ["add", "."], { cwd: source });
  execFileSync("git", ["-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "-m", "fixture"], { cwd: source });
  const revision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: source, encoding: "utf8" }).trim();
  const selected = "skills/first";
  const dependency = { id: "fresh", version: revision, install: { kind: "git-skill", repository: source, source, revision, paths: [selected] },
    compatibility: { kind: "required-files", entrypoint: "SKILL.md", allowUnrelatedRegularFiles: true, selectedPaths: [{ selectedPath: selected,
      requiredFiles: [{ path: "SKILL.md", digest: { algorithm: "sha256", value: createHash("sha256").update("# first\n").digest("hex") } }] }] } };
  const runner = createDependencyRunner({ host: "codex", scope: "project",
    paths: { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot } });
  const installed = await runner({ dependency, phase: "install" });
  assert.equal(installed.status, "passed");
  assert.equal(installed.installed, "installed");
  assert.deepEqual(installed.components.map(({ installed: disposition, lifecycleOwnership }) => ({ disposition, lifecycleOwnership })), [
    { disposition: "installed", lifecycleOwnership: "managed" },
  ]);
  assert.equal(await readFile(path.join(skillRoot, "first", "SKILL.md"), "utf8"), "# first\n");

  const incomplete = { ...dependency, id: "incomplete", install: { ...dependency.install, paths: [selected, "skills/missing"] },
    compatibility: { ...dependency.compatibility, selectedPaths: [...dependency.compatibility.selectedPaths,
      { selectedPath: "skills/missing", requiredFiles: [] }] } };
  const incompleteRoot = path.join(root, "incomplete-skills");
  const incompleteRunner = createDependencyRunner({ host: "codex", scope: "project",
    paths: { projectRoot: root, toolRoot: path.join(root, "incomplete-tools"), skillRoot: incompleteRoot } });
  const failed = await incompleteRunner({ dependency: incomplete, phase: "install" });
  assert.equal(failed.status, "failed");
  await assert.rejects(readFile(path.join(incompleteRoot, "first", "SKILL.md")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(incompleteRoot, "missing", "SKILL.md")), { code: "ENOENT" });

  await t.test("rejects a corrupt required-file digest before publication", async () => {
    const corruptRoot = path.join(root, "corrupt-skills");
    const corruptDependency = { ...dependency, id: "corrupt", compatibility: { ...dependency.compatibility,
      selectedPaths: [{ ...dependency.compatibility.selectedPaths[0], requiredFiles: [{ path: "SKILL.md",
        digest: { algorithm: "sha256", value: createHash("sha256").update("corrupt\n").digest("hex") } }] }] } };
    const corruptRunner = createDependencyRunner({ host: "codex", scope: "project",
      paths: { projectRoot: root, toolRoot: path.join(root, "corrupt-tools"), skillRoot: corruptRoot } });
    const corrupt = await corruptRunner({ dependency: corruptDependency, phase: "install" });
    assert.equal(corrupt.status, "failed");
    await assert.rejects(readFile(path.join(corruptRoot, "first", "SKILL.md")), { code: "ENOENT" });
  });

  await writeFile(path.join(source, selected, ".agent-team-source.json"), `${JSON.stringify({
    source, revision: "fixture-version", selectedPath: selected,
  }, null, 2)}\n`);
  execFileSync("git", ["add", "."], { cwd: source });
  execFileSync("git", ["-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "-m", "partial fixture"], { cwd: source });
  const partialRevision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: source, encoding: "utf8" }).trim();
  const partialDependency = { ...dependency, id: "partial", version: "fixture-version",
    install: { ...dependency.install, revision: partialRevision } };
  const partialRoot = path.join(root, "partial-skills");
  const partialRunner = createDependencyRunner({ host: "codex", scope: "project",
    paths: { projectRoot: root, toolRoot: path.join(root, "partial-tools"), skillRoot: partialRoot } });
  await t.test("rejects reserved provenance before publication", async () => {
    const partial = await partialRunner({ dependency: partialDependency, phase: "install" });
    assert.equal(partial.status, "failed");
    await assert.rejects(readFile(path.join(partialRoot, "first", "SKILL.md")), { code: "ENOENT" });
  });
});

test("multi-path git publication never replaces a later concurrent empty destination", async (t) => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-git-publish-race-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = path.join(root, "source");
  const skillRoot = path.join(root, "skills");
  const selectedPaths = Array.from({ length: 40 }, (_, index) => `skills/fixture-${String(index).padStart(2, "0")}`);
  const bytes = "# exact\n";
  for (const selectedPath of selectedPaths) {
    await mkdir(path.join(source, selectedPath), { recursive: true });
    await writeFile(path.join(source, selectedPath, "SKILL.md"), bytes);
  }
  execFileSync("git", ["init", "--quiet"], { cwd: source });
  execFileSync("git", ["add", "."], { cwd: source });
  execFileSync("git", ["-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "commit", "--quiet", "-m", "fixture"], { cwd: source });
  const revision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: source, encoding: "utf8" }).trim();
  const dependency = { id: "publish-race", version: revision,
    install: { kind: "git-skill", repository: source, source, revision, paths: selectedPaths },
    compatibility: { kind: "required-files", entrypoint: "SKILL.md", allowUnrelatedRegularFiles: true,
      selectedPaths: selectedPaths.map((selectedPath) => ({ selectedPath, requiredFiles: [{ path: "SKILL.md",
        digest: { algorithm: "sha256", value: createHash("sha256").update(bytes).digest("hex") } }] })) } };
  const first = path.join(skillRoot, path.basename(selectedPaths[0]));
  const later = path.join(skillRoot, path.basename(selectedPaths.at(-1)));
  const watcher = spawn(process.execPath, ["-e", `
    const fs = require("node:fs");
    const first = process.env.FIRST_DEST;
    const later = process.env.LATER_DEST;
    const deadline = Date.now() + 5000;
    const timer = setInterval(() => {
      if (!fs.existsSync(first)) {
        if (Date.now() > deadline) { clearInterval(timer); process.exit(2); }
        return;
      }
      clearInterval(timer);
      fs.mkdirSync(later);
      const stat = fs.lstatSync(later);
      process.stdout.write(JSON.stringify({ dev: stat.dev, ino: stat.ino }));
    }, 1);
  `], { env: { ...process.env, FIRST_DEST: first, LATER_DEST: later }, stdio: ["ignore", "pipe", "pipe"] });
  const observed = new Promise((resolve, reject) => {
    let stdout = ""; let stderr = "";
    watcher.stdout.on("data", (chunk) => { stdout += chunk; });
    watcher.stderr.on("data", (chunk) => { stderr += chunk; });
    watcher.on("error", reject);
    watcher.on("close", (code) => code === 0 ? resolve(JSON.parse(stdout)) : reject(new Error(`race watcher failed ${code}: ${stderr}`)));
  });
  const runner = createDependencyRunner({ host: "codex", scope: "project",
    paths: { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot } });
  const result = await runner({ dependency, phase: "install" });
  const operatorIdentity = await observed;

  assert.equal(result.status, "manual_action");
  const current = await lstat(later);
  assert.deepEqual({ dev: current.dev, ino: current.ino }, operatorIdentity);
  for (const selectedPath of selectedPaths.slice(0, -1)) {
    await assert.rejects(readFile(path.join(skillRoot, path.basename(selectedPath), "SKILL.md")), { code: "ENOENT" });
  }
  assert.equal(current.isDirectory(), true);
  assert.deepEqual(await readdir(later), []);
});

test("default LeanCTX gate overrides inherited directory pins for its narrow read", async (t) => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const { CATALOG_BY_ID } = await import("../hooks/lib/dependency-catalog.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-leanctx-"));
  const keys = ["LEAN_CTX_CONFIG_DIR", "LEAN_CTX_DATA_DIR", "LEAN_CTX_STATE_DIR", "LEAN_CTX_CACHE_DIR", "XDG_RUNTIME_DIR"];
  const previous = Object.fromEntries(keys.map((key) => [key, process.env[key]]));
  for (const key of keys) process.env[key] = path.join(directory, "unrelated", key);
  t.after(() => {
    for (const key of keys) {
      if (previous[key] === undefined) delete process.env[key];
      else process.env[key] = previous[key];
    }
  });
  const executable = path.join(directory, "lean-ctx-fixture.mjs");
  await writeFile(executable, `#!/usr/bin/env node
import path from 'node:path';
if (process.argv[2] !== "read") process.exit(9);
for (const key of ${JSON.stringify(["LEAN_CTX_CONFIG_DIR", "LEAN_CTX_DATA_DIR", "LEAN_CTX_STATE_DIR", "LEAN_CTX_CACHE_DIR", "XDG_RUNTIME_DIR"])}) {
  const relative = path.relative(process.cwd(), process.env[key] ?? '/');
  if (!relative || relative.startsWith('..') || path.isAbsolute(relative)) process.exit(8);
}
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
  for (const key of keys) assert.equal(process.env[key], path.join(directory, "unrelated", key));
});

test("default Impeccable gate verifies all three documented detector exits", async () => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const { CATALOG_BY_ID } = await import("../hooks/lib/dependency-catalog.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-impeccable-"));
  const executable = path.join(directory, "impeccable-fixture.mjs");
  const observation = path.join(directory, "environment.json");
  await writeFile(executable, `#!/usr/bin/env node
import { writeFileSync } from "node:fs";
writeFileSync(process.env.AGENT_TEAM_IMPECCABLE_ENV, JSON.stringify({ PWD: process.env.PWD }));
const target = process.argv.at(-1);
if (target.includes("finding")) process.exit(2);
if (target.includes("missing")) process.exit(1);
process.stdout.write("clean\\n");
`);
  await chmod(executable, 0o755);
  const previousObservation = process.env.AGENT_TEAM_IMPECCABLE_ENV;
  process.env.AGENT_TEAM_IMPECCABLE_ENV = observation;
  try {
  const runner = createDependencyRunner({
    host: "codex",
    scope: "project",
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") },
  });

  const result = await runner({ dependency: { ...CATALOG_BY_ID.get("impeccable"), executable }, phase: "functional", check: "detector-exit-contract" });

  assert.equal(result.status, "passed");
  assert.match(result.evidence, /0.*2.*1/);
  assert.ok(JSON.parse(await readFile(observation, "utf8")).PWD
    .startsWith(path.join(directory, "tools", "verification", "impeccable-")));
  } finally {
    if (previousObservation === undefined) delete process.env.AGENT_TEAM_IMPECCABLE_ENV;
    else process.env.AGENT_TEAM_IMPECCABLE_ENV = previousObservation;
  }
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
  assert.equal(result.receipts.find(({ id }) => id === "uv").status, "failed");
  assert.equal(result.receipts.find(({ id }) => id === "serena").status, "required_unavailable");
  assert.equal(result.receipts.find(({ id }) => id === "playwright-cli").status, "failed");
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

test("saved default and optional declines survive preparation until explicitly changed", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-saved-declines-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, `${JSON.stringify({
    skill: "agent-team", projectId: "p", version: 1, tracker: { kind: "markdown", path: "TASKS.md" },
    dependencies: { hosts: { codex: { declined: ["ponytail", "context7"], custom: "keep" } } },
  })}\n`);
  const called = [];
  const runner = async ({ dependency, phase }) => {
    called.push(`${dependency.id}:${phase}`);
    return { status: "passed", version: dependency.version };
  };

  const result = await prepareDependencies({
    setupPath, expectedVersion: 1, writer: { id: "owner", role: "project_orchestrator" }, operationId: "saved-declines",
    loadRegistry: async () => ({ projectOwner: "owner" }), host: "codex", scope: "project", selections: {},
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") }, runner,
  });
  const saved = JSON.parse(await readFile(setupPath, "utf8"));

  assert.equal(result.status, "ready");
  assert.ok(!called.some((entry) => entry.startsWith("ponytail:")));
  assert.deepEqual(saved.dependencies.hosts.codex.declined, ["ponytail", "context7"]);
  assert.equal(saved.dependencies.hosts.codex.custom, "keep");

  called.length = 0;
  const changed = await prepareDependencies({
    setupPath, expectedVersion: 2, writer: { id: "owner", role: "project_orchestrator" }, operationId: "change-saved-decline",
    loadRegistry: async () => ({ projectOwner: "owner" }), host: "codex", scope: "project",
    selections: { defaults: ["ponytail"] },
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") }, runner,
  });
  const changedSetup = JSON.parse(await readFile(setupPath, "utf8"));
  assert.equal(changed.status, "ready");
  assert.ok(called.some((entry) => entry.startsWith("ponytail:")));
  assert.deepEqual(changedSetup.dependencies.hosts.codex.declined, ["context7"]);
});

test("declined mandatory dependencies get unavailable receipts while other preparation continues", async () => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-required-decline-"));
  const setupPath = path.join(directory, "setup.json");
  await writeFile(setupPath, '{"skill":"agent-team","projectId":"p","version":1,"tracker":{"kind":"markdown","path":"TASKS.md"}}\n');
  const called = [];
  const runner = async ({ dependency, phase }) => {
    called.push(`${dependency.id}:${phase}`);
    return { status: "passed", version: dependency.version };
  };

  const result = await prepareDependencies({
    setupPath, expectedVersion: 1, writer: { id: "owner", role: "project_orchestrator" }, operationId: "required-decline",
    loadRegistry: async () => ({ projectOwner: "owner" }), host: "codex", scope: "project",
    selections: { defaults: [], declined: ["serena"] },
    paths: { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") }, runner,
  });
  const serena = result.receipts.find(({ id }) => id === "serena");

  assert.equal(result.status, "incomplete");
  assert.equal(serena.status, "required_unavailable");
  assert.equal(serena.availableToWorker, "failed");
  assert.match(serena.boundary, /mandatory.*declined/i);
  assert.ok(!called.some((entry) => entry.startsWith("serena:")));
  assert.ok(called.some((entry) => entry.startsWith("playwright-cli:")));
});

test("Serena functional verification isolates user state and inherited Git routing", async (t) => {
  const { createDependencyRunner } = await import("../hooks/lib/dependencies.mjs");
  const { CATALOG_BY_ID } = await import("../hooks/lib/dependency-catalog.mjs");
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-serena-env-"));
  const sentinelHome = path.join(directory, "sentinel-serena-home");
  const observation = path.join(directory, "child-environment.json");
  const server = path.join(directory, "serena-environment-server.mjs");
  t.after(() => rm(directory, { recursive: true, force: true }));
  await writeFile(server, `#!/usr/bin/env node
import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";
import readline from "node:readline";
mkdirSync(process.env.SERENA_HOME, { recursive: true });
writeFileSync(path.join(process.env.SERENA_HOME, "probe-state"), "owned by probe");
writeFileSync(process.env.AGENT_TEAM_ENV_OBSERVATION, JSON.stringify({ SERENA_HOME: process.env.SERENA_HOME, PWD: process.env.PWD, GIT_DIR: process.env.GIT_DIR, GIT_WORK_TREE: process.env.GIT_WORK_TREE, GIT_COMMON_DIR: process.env.GIT_COMMON_DIR, GIT_INDEX_FILE: process.env.GIT_INDEX_FILE }));
const lines = readline.createInterface({ input: process.stdin });
lines.on("line", line => {
  const message = JSON.parse(line);
  if (message.method === "initialize") process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id: message.id, result: { capabilities: { tools: {} } } }) + "\\n");
  if (message.method === "tools/list") process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id: message.id, result: { tools: [{ name: "find_symbol" }] } }) + "\\n");
  if (message.method === "tools/call") process.stdout.write(JSON.stringify({ jsonrpc: "2.0", id: message.id, result: { content: [{ type: "text", text: "readinessFixtureSymbol" }] } }) + "\\n");
});
`);
  await chmod(server, 0o755);
  const keys = ["SERENA_HOME", "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "AGENT_TEAM_ENV_OBSERVATION"];
  const previous = Object.fromEntries(keys.map((key) => [key, process.env[key]]));
  Object.assign(process.env, {
    SERENA_HOME: sentinelHome,
    GIT_DIR: "/sentinel/git-dir",
    GIT_WORK_TREE: "/sentinel/git-work-tree",
    GIT_COMMON_DIR: "/sentinel/git-common-dir",
    GIT_INDEX_FILE: "/sentinel/git-index",
    AGENT_TEAM_ENV_OBSERVATION: observation,
  });
  const paths = { projectRoot: directory, toolRoot: path.join(directory, "tools"), skillRoot: path.join(directory, "skills") };
  try {
    const runner = createDependencyRunner({ host: "codex", scope: "project", paths });
    const result = await runner({ dependency: { ...CATALOG_BY_ID.get("serena"), executable: server }, phase: "functional", check: "symbol-operation" });
    assert.equal(result.status, "passed", result.evidence);
  } finally {
    for (const [key, value] of Object.entries(previous)) {
      if (value === undefined) delete process.env[key];
      else process.env[key] = value;
    }
  }
  const observed = JSON.parse(await readFile(observation, "utf8"));
  assert.ok(observed.SERENA_HOME.startsWith(path.join(paths.toolRoot, "verification", "serena-")));
  assert.equal(observed.PWD, path.dirname(observed.SERENA_HOME));
  for (const key of ["GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"]) assert.equal(Object.hasOwn(observed, key), false);
  await assert.rejects(readFile(path.join(sentinelHome, "probe-state")), { code: "ENOENT" });
});
