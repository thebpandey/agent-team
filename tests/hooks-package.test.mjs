import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { cp, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { promisify } from "node:util";

import { buildArtifacts } from "../hooks/lib/artifacts.mjs";
import * as validators from "../hooks/lib/package-validator.mjs";

const sourceRoot = path.resolve(import.meta.dirname, "..");
const run = promisify(execFile);
const { checkPackage } = validators;
const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { force: true, recursive: true }))));

async function packageFixture() {
  const destination = await mkdtemp(path.join(os.tmpdir(), "agent-team-package-"));
  temporary.push(destination);
  await cp(sourceRoot, destination, {
    recursive: true,
    filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) && !source.includes(`${path.sep}.superpowers${path.sep}`),
  });
  return destination;
}

test("package validator accepts the current source package", async () => {
  // This test catches validator rules that disagree with the package they protect.
  const result = await checkPackage(sourceRoot);
  assert.equal(result.status, "passed", result.errors.join("\n"));
});

test("installed-package validation accepts the extracted consumer without source-only files", async () => {
  // Requiring CI, tests, or legacy files in an installed package must break this consumer check.
  assert.equal(typeof validators.checkInstalledPackage, "function");
  const output = await mkdtemp(path.join(os.tmpdir(), "agent-team-package-output-"));
  const extracted = await mkdtemp(path.join(os.tmpdir(), "agent-team-package-extracted-"));
  temporary.push(output, extracted);
  const built = await buildArtifacts({ sourceRoot, outputDirectory: output, sourceRevision: "consumer-revision" });
  await run("unzip", ["-q", built.archives[0], "-d", extracted]);
  const root = path.join(extracted, "agent-team");

  await assert.rejects(readFile(path.join(root, "tests", "hooks-package.test.mjs")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(root, ".github", "workflows", "release.yml")), { code: "ENOENT" });
  const result = await validators.checkInstalledPackage(root);
  assert.equal(result.status, "passed", result.errors.join("\n"));
});

test("installed-package validation rejects a self-consistent manifest that drops one host adapter", async () => {
  // Trusting only manifest-declared runtimes would let a one-host archive claim to be the universal package.
  const root = await packageFixture();
  const manifestPath = path.join(root, "hooks", "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  manifest.files = manifest.files.filter((file) => file !== "hooks/claude-hooks.json");
  delete manifest.runtimeDeclarations.claude;
  delete manifest.requiredEvents.claude;
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
  await rm(path.join(root, "hooks", "claude-hooks.json"));

  const result = await validators.checkInstalledPackage(root);
  assert.equal(result.status, "failed");
  assert.match(result.errors.join("\n"), /claude/i);
});

test("package validator detects missing files, version drift, broken links, missing adapters, registrations, manifest omissions, and parity defects", async (context) => {
  // Each mutation names a realistic packaging defect that CI must reject.
  const cases = [
    ["missing file", async (root) => rm(path.join(root, "hooks", "agent-team-hook.mjs"))],
    ["version drift", async (root) => writeFile(path.join(root, "SKILL.md"), (await readFile(path.join(root, "SKILL.md"), "utf8")).replace(/version: "[^"]+"/, 'version: "9.9.9"'))],
    ["changelog version drift", async (root) => writeFile(path.join(root, "CHANGELOG.md"), (await readFile(path.join(root, "CHANGELOG.md"), "utf8")).replace(/^## \d+\.\d+\.\d+/m, "## 9.9.9"))],
    ["repository source drift", async (root) => {
      const file = path.join(root, "hooks", "manifest.json");
      const manifest = JSON.parse(await readFile(file, "utf8"));
      manifest.repository = "https://github.com/example/wrong";
      await writeFile(file, JSON.stringify(manifest));
    }],
    ["broken link", async (root) => writeFile(path.join(root, "README.md"), `${await readFile(path.join(root, "README.md"), "utf8")}\n[broken](references/not-present.md)\n`)],
    ["missing adapter", async (root) => rm(path.join(root, "hooks", "claude-hooks.json"))],
    ["registration", async (root) => {
      const file = path.join(root, "hooks", "claude-hooks.json");
      const declaration = JSON.parse(await readFile(file, "utf8"));
      delete declaration.hooks.TaskCompleted;
      await writeFile(file, JSON.stringify(declaration));
    }],
    ["manifest omission", async (root) => {
      const file = path.join(root, "hooks", "manifest.json");
      const manifest = JSON.parse(await readFile(file, "utf8"));
      manifest.files = manifest.files.filter((entry) => entry !== "references/state.md");
      await writeFile(file, JSON.stringify(manifest));
    }],
    ["parity", async (root) => {
      const file = path.join(root, "hooks", "manifest.json");
      const manifest = JSON.parse(await readFile(file, "utf8"));
      manifest.policies[0].platforms = ["codex"];
      await writeFile(file, JSON.stringify(manifest));
    }],
  ];

  for (const [name, mutate] of cases) {
    await context.test(name, async () => {
      const root = await packageFixture();
      await mutate(root);
      const result = await checkPackage(root);
      assert.equal(result.status, "failed");
      assert.ok(result.errors.length > 0);
    });
  }
});
