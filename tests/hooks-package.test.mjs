import assert from "node:assert/strict";
import { cp, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { checkPackage } from "../hooks/lib/package-validator.mjs";

const sourceRoot = path.resolve(import.meta.dirname, "..");
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

test("package validator detects missing files, version drift, broken links, missing adapters, registrations, manifest omissions, and parity defects", async (context) => {
  // Each mutation names a realistic packaging defect that CI must reject.
  const cases = [
    ["missing file", async (root) => rm(path.join(root, "hooks", "agent-team-hook.mjs"))],
    ["version drift", async (root) => writeFile(path.join(root, "SKILL.md"), (await readFile(path.join(root, "SKILL.md"), "utf8")).replace(/version: "[^"]+"/, 'version: "9.9.9"'))],
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
