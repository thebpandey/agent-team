import assert from "node:assert/strict";
import { mkdtemp, readFile, rm } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { buildArtifacts, checkArtifacts, readZip, writeZip } from "../hooks/lib/artifacts.mjs";

const sourceRoot = path.resolve(import.meta.dirname, "..");
const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { force: true, recursive: true }))));

async function artifacts() {
  const outputDirectory = await mkdtemp(path.join(os.tmpdir(), "agent-team-artifacts-"));
  temporary.push(outputDirectory);
  const built = await buildArtifacts({ sourceRoot, outputDirectory, sourceRevision: "fixture-revision" });
  return { outputDirectory, built };
}

test("artifact validation reports source-only checks as not applicable", async () => {
  // This test catches invented archive success when no archive was supplied.
  const result = await checkArtifacts({ sourceRoot, archives: [], expectedRevision: "fixture-revision" });
  assert.equal(result.status, "not_applicable");
});

test("reproducible Codex and Claude archives match the manifest and source", async () => {
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

test("artifact validation rejects stale, omitted, unexpected, duplicate, absolute, and path-escaping entries", async (context) => {
  // This table protects every archive boundary named by the package contract.
  const cases = [
    ["stale", (entries) => entries.map((entry) => entry.name === "agent-team/SKILL.md" ? { ...entry, data: Buffer.from("stale") } : entry)],
    ["omitted", (entries) => entries.filter((entry) => entry.name !== "agent-team/SKILL.md")],
    ["unexpected", (entries) => [...entries, { name: "agent-team/unexpected.txt", data: Buffer.from("unexpected") }]],
    ["duplicate", (entries) => [...entries, { ...entries[0] }]],
    ["absolute", (entries) => [...entries, { name: "/absolute.txt", data: Buffer.from("unsafe") }]],
    ["path escape", (entries) => [...entries, { name: "../escape.txt", data: Buffer.from("unsafe") }]],
  ];

  for (const [name, mutate] of cases) {
    await context.test(name, async () => {
      const value = await artifacts();
      const codex = value.built.archives.find((file) => file.includes("codex"));
      await writeZip(codex, mutate(await readZip(codex)));
      const result = await checkArtifacts({ sourceRoot, archives: value.built.archives, expectedRevision: "fixture-revision" });
      assert.equal(result.status, "failed");
    });
  }
});
