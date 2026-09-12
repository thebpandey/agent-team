import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { createHash } from "node:crypto";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { createDependencyRunner } from "../hooks/lib/dependencies.mjs";
import { CATALOG_BY_ID } from "../hooks/lib/dependency-catalog.mjs";

async function fixture(t, { corrupt = false } = {}) {
  const catalog = CATALOG_BY_ID.get("lean-ctx");
  assert.equal(catalog.install.kind, "github-release", "never execute LeanCTX npm lifecycle scripts");
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-lean-install-"));
  t.after(() => rm(root, { recursive: true, force: true }));
  const source = path.join(root, "source");
  await mkdir(source);
  await writeFile(path.join(source, "lean-ctx"), `#!${process.execPath}\nconsole.log('lean-ctx 3.10.1');\n`, { mode: 0o755 });
  const archiveFile = path.join(root, "fixture.tar.gz");
  execFileSync("tar", ["-czf", archiveFile, "-C", source, "lean-ctx"]);
  const archive = await readFile(archiveFile);
  const skill = Buffer.from("---\nname: lean-ctx\n---\nComplete fixture skill.\n");
  const blob = createHash("sha1").update(`blob ${skill.length}\0`).update(skill).digest("hex");
  const dependency = { ...catalog, install: { ...catalog.install, skill: { ...catalog.install.skill, gitBlob: blob } } };
  const urls = [];
  const previousFetch = globalThis.fetch;
  globalThis.fetch = async (url) => {
    urls.push(String(url));
    if (url === dependency.install.skill.source) return new Response(skill);
    assert.ok(String(url).startsWith(`${dependency.install.source}/`), `unpinned URL: ${url}`);
    if (String(url).endsWith("/SHA256SUMS")) {
      const asset = urls.find((value) => value.endsWith(".tar.gz"))?.split("/").at(-1);
      const digest = corrupt ? "0".repeat(64) : createHash("sha256").update(archive).digest("hex");
      return new Response(`${digest}  ${asset}\n`);
    }
    assert.match(String(url), /\/lean-ctx-[\w-]+\.tar\.gz$/);
    return new Response(archive);
  };
  t.after(() => { globalThis.fetch = previousFetch; });
  const paths = { projectRoot: root, toolRoot: path.join(root, "tools"), skillRoot: path.join(root, "skills") };
  const runner = createDependencyRunner({ host: "codex", scope: "project", paths });
  return { root, paths, dependency, runner, urls, skill };
}

test("LeanCTX installs a pinned verified native release without touching an existing npm shim", async (t) => {
  const { paths, dependency, runner, urls, skill } = await fixture(t);
  const shim = path.join(paths.toolRoot, "bin", "lean-ctx");
  await mkdir(path.dirname(shim), { recursive: true });
  await writeFile(shim, "preserve existing npm wrapper");
  assert.equal((await runner({ dependency, phase: "install" })).status, "passed");
  const probe = await runner({ dependency, phase: "probe" });
  assert.equal(probe.status, "passed");
  assert.equal(probe.version, "3.10.1");
  assert.equal(await readFile(shim, "utf8"), "preserve existing npm wrapper");
  assert.equal((await runner({ dependency, phase: "companion" })).status, "passed");
  assert.deepEqual(await readFile(path.join(paths.skillRoot, "lean-ctx", "SKILL.md")), skill);
  const count = urls.length;
  assert.equal((await runner({ dependency, phase: "companion" })).status, "passed");
  assert.equal(urls.length, count, "compatible complete skill is reused without downloading");
});

test("LeanCTX checksum failure publishes no executable", async (t) => {
  const { paths, dependency, runner } = await fixture(t, { corrupt: true });
  const result = await runner({ dependency, phase: "install" });
  assert.equal(result.status, "failed");
  assert.match(result.evidence, /checksum/i);
  assert.equal((await runner({ dependency, phase: "probe" })).status, "not_found");
  await assert.rejects(readFile(path.join(paths.toolRoot, `lean-ctx-${dependency.version}`, "lean-ctx")), { code: "ENOENT" });
});

test("LeanCTX production receipt keeps native executable and skill ownership separate", async (t) => {
  const { prepareDependencies } = await import("../hooks/lib/dependencies.mjs");
  const { root, paths, dependency, runner } = await fixture(t);
  const setupPath = path.join(root, "setup.json");
  await writeFile(setupPath, `${JSON.stringify({ skill: "agent-team", projectId: "p", version: 1,
    tracker: { kind: "markdown", path: "TASKS.md" } })}\n`);
  const result = await prepareDependencies({ setupPath, expectedVersion: 1, operationId: "lean-components",
    writer: { id: "owner", role: "project_orchestrator" }, loadRegistry: async () => ({ projectOwner: "owner" }),
    host: "codex", scope: "project", selections: { defaults: ["lean-ctx"] }, paths,
    runner: async (request) => request.dependency.id !== "lean-ctx"
      ? { status: "passed", version: request.dependency.version }
      : ["functional", "worker"].includes(request.phase) ? { status: "passed" } : runner({ ...request, dependency }) });
  const receipt = result.receipts.find(({ id }) => id === "lean-ctx");
  assert.deepEqual(receipt.components.map(({ id }) => id), ["executable", "skill"]);
  assert.equal(receipt.components[0].lifecycleOwnership, "managed");
  assert.equal(receipt.components[1].lifecycleOwnership, "managed");
  assert.equal(receipt.lifecycleOwnership, "managed");
});

test("LeanCTX reuses an exact sidecar-free skill with unrelated files without fetching or claiming it", async (t) => {
  const { paths, dependency, runner, urls, skill } = await fixture(t);
  const destination = path.join(paths.skillRoot, "lean-ctx");
  await mkdir(destination, { recursive: true });
  await writeFile(path.join(destination, "SKILL.md"), skill);
  await writeFile(path.join(destination, "NOTES.md"), "operator notes\n");
  const before = createHash("sha256").update(Buffer.concat([skill, Buffer.from("operator notes\n")])).digest("hex");

  const result = await runner({ dependency, phase: "companion" });

  const after = createHash("sha256").update(Buffer.concat([
    await readFile(path.join(destination, "SKILL.md")), await readFile(path.join(destination, "NOTES.md")),
  ])).digest("hex");
  assert.equal(result.status, "passed");
  assert.equal(result.installed, "reused_unowned");
  assert.equal(result.lifecycleOwnership, "unowned");
  assert.equal(result.path, destination);
  assert.equal(result.components[0].compatibility.unrelatedRegularFilesPreserved, true);
  assert.equal(before, after);
  assert.equal(urls.length, 0);
  await assert.rejects(readFile(path.join(destination, ".agent-team-source.json")), { code: "ENOENT" });
});

test("LeanCTX preserves customized executable and skill paths", async (t) => {
  const { paths, dependency, runner, urls } = await fixture(t);
  const binary = path.join(paths.toolRoot, `lean-ctx-${dependency.version}`, "lean-ctx");
  const skill = path.join(paths.skillRoot, "lean-ctx", "SKILL.md");
  await mkdir(path.dirname(binary), { recursive: true });
  await mkdir(path.dirname(skill), { recursive: true });
  await writeFile(binary, "custom executable");
  await writeFile(skill, "custom skill");
  assert.equal((await runner({ dependency, phase: "install" })).status, "customized");
  const companion = await runner({ dependency, phase: "companion" });
  assert.equal(companion.status, "manual_action");
  assert.equal(companion.lifecycleOwnership, "unowned");
  assert.equal(companion.path, path.dirname(skill));
  assert.equal(companion.components[0].compatibility.status, "incompatible");
  assert.equal(urls.length, 0);
  assert.equal(await readFile(binary, "utf8"), "custom executable");
  assert.equal(await readFile(skill, "utf8"), "custom skill");
});
