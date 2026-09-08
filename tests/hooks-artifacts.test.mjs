import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { mkdtemp, readFile, rm, stat } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { promisify } from "node:util";

import { buildArtifacts, checkArtifacts, readZip, writeZip } from "../hooks/lib/artifacts.mjs";
import { installPackage } from "../hooks/lib/install.mjs";

const sourceRoot = path.resolve(import.meta.dirname, "..");
const run = promisify(execFile);
const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { force: true, recursive: true }))));

async function artifacts() {
  const outputDirectory = await mkdtemp(path.join(os.tmpdir(), "agent-team-artifacts-"));
  temporary.push(outputDirectory);
  const built = await buildArtifacts({ sourceRoot, outputDirectory, sourceRevision: "fixture-revision" });
  return { outputDirectory, built };
}

test("the one universal ZIP installs for one selected host from outside the source checkout", async () => {
  // Removing either adapter, executable mode, or target filtering must break this consumer journey.
  const { built } = await artifacts();
  const extracted = await mkdtemp(path.join(os.tmpdir(), "agent team extracted "));
  const home = await mkdtemp(path.join(os.tmpdir(), "agent team consumer home "));
  temporary.push(extracted, home);

  assert.equal(built.archives.length, 1);
  assert.equal(path.basename(built.archives[0]), "agent-team-6.5.0.zip");
  await run("unzip", ["-q", built.archives[0], "-d", extracted]);
  const packageRoot = path.join(extracted, "agent-team");
  await readFile(path.join(packageRoot, "hooks", "codex-hooks.json"));
  await readFile(path.join(packageRoot, "hooks", "claude-hooks.json"));
  assert.notEqual((await stat(path.join(packageRoot, "hooks", "agent-team-cli.mjs"))).mode & 0o111, 0);

  const installed = await installPackage({ sourceRoot: packageRoot, home, host: "codex", scope: "user" });
  assert.equal(installed.validation.status, "passed");
  await readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md"));
  await readFile(path.join(home, ".codex", "hooks.json"));
  await assert.rejects(readFile(path.join(home, ".claude", "settings.json")), { code: "ENOENT" });
});

test("artifact validation reports source-only checks as not applicable", async () => {
  // This test catches invented archive success when no archive was supplied.
  const result = await checkArtifacts({ sourceRoot, archives: [], expectedRevision: "fixture-revision" });
  assert.equal(result.status, "not_applicable");
});

test("the reproducible universal archive matches the manifest and source", async () => {
  // This test catches platform payload drift and nondeterministic archive output.
  const first = await artifacts();
  const second = await artifacts();
  const result = await checkArtifacts({ sourceRoot, archives: first.built.archives, expectedRevision: "fixture-revision" });
  const firstBytes = await Promise.all(first.built.archives.map((file) => readZip(file).then((entries) => entries.map(({ name, data }) => [name, data.toString("hex")]))));
  const secondBytes = await Promise.all(second.built.archives.map((file) => readZip(file).then((entries) => entries.map(({ name, data }) => [name, data.toString("hex")]))));
  const firstArchives = await Promise.all(first.built.archives.map((file) => readFile(file)));
  const secondArchives = await Promise.all(second.built.archives.map((file) => readFile(file)));

  assert.equal(result.status, "passed", result.errors.join("\n"));
  assert.deepEqual(firstBytes, secondBytes);
  assert.deepEqual(firstArchives, secondArchives);
});

test("archives identify the canonical repository and pinned release source", async () => {
  const value = await artifacts();
  for (const archive of value.built.archives) {
    const entries = await readZip(archive);
    const source = JSON.parse(entries.find(({ name }) => name === "agent-team/.agent-team-source.json").data.toString("utf8"));
    assert.deepEqual(source.hosts, ["codex", "claude-code"]);
    assert.equal(source.repository, "https://github.com/thebpandey/agent-team");
    assert.equal(source.releaseTag, "v6.5.0");
    assert.equal(source.sourceRevision, "fixture-revision");
  }
});

test("artifact validation rejects stale, omitted, unexpected, duplicate, absolute, and path-escaping entries", async (context) => {
  // This table protects every archive boundary named by the package contract.
  const cases = [
    ["stale", (entries) => entries.map((entry) => entry.name === "agent-team/SKILL.md" ? { ...entry, data: Buffer.from("stale") } : entry)],
    ["omitted", (entries) => entries.filter((entry) => entry.name !== "agent-team/SKILL.md")],
    ["unexpected", (entries) => [...entries, { name: "agent-team/unexpected.txt", data: Buffer.from("unexpected") }]],
    ["duplicate", (entries) => [...entries, { ...entries[0] }]],
    ["entrypoint mode", (entries) => entries.map((entry) => entry.name === "agent-team/hooks/agent-team-cli.mjs" ? { ...entry, mode: 0o100644 } : entry)],
    ["absolute", (entries) => [...entries, { name: "/absolute.txt", data: Buffer.from("unsafe") }]],
    ["path escape", (entries) => [...entries, { name: "../escape.txt", data: Buffer.from("unsafe") }]],
  ];

  for (const [name, mutate] of cases) {
    await context.test(name, async () => {
      const value = await artifacts();
      const archive = value.built.archives[0];
      await writeZip(archive, mutate(await readZip(archive)));
      const result = await checkArtifacts({ sourceRoot, archives: value.built.archives, expectedRevision: "fixture-revision" });
      assert.equal(result.status, "failed");
    });
  }
});
