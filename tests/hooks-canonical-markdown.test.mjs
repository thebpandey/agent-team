import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { access, mkdtemp, readFile, rename, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { promisify } from "node:util";

import { buildArtifacts } from "../hooks/lib/artifacts.mjs";
import { checkInstalledPackage } from "../hooks/lib/package-validator.mjs";

const run = promisify(execFile);
const sourceRoot = path.resolve(import.meta.dirname, "..");
const temporary = [];
const laneAssets = [
  "references/LANES.md",
  "assets/templates/lanes/BRIEF.md",
  "assets/templates/lanes/NEXT-TASK.md",
  "assets/templates/lanes/HANDOVER.md",
];
const roles = ["complex", "developer", "reviewer", "routine", "text", "visual-tester"];

test.afterEach(async () => Promise.all(temporary.splice(0).map((entry) => rm(entry, { force: true, recursive: true }))));

async function manifestAt(root) {
  return JSON.parse(await readFile(path.join(root, "hooks", "manifest.json"), "utf8"));
}

function noncanonicalMarkdown(files) {
  return files.filter((file) => file.endsWith(".md")
    && path.basename(file, ".md") !== path.basename(file, ".md").toUpperCase());
}

function localLinks(source) {
  return [...source.matchAll(/!?\[[^\]]*\]\(([^)]+)\)/g)]
    .map((match) => match[1].trim().replace(/^<|>$/g, ""));
}

async function brokenPackageLinks(root, manifest) {
  const broken = [];
  for (const file of manifest.files.filter((entry) => entry.endsWith(".md"))) {
    const source = await readFile(path.join(root, file), "utf8");
    for (const target of localLinks(source)) {
      if (/^(?:[a-z]+:|#)/i.test(target)) continue;
      const clean = decodeURIComponent(target.split("#")[0].split("?")[0]);
      const resolved = path.resolve(path.dirname(path.join(root, file)), clean);
      try {
        assert.ok(resolved.startsWith(`${path.resolve(root)}${path.sep}`), `${file}: ${target}`);
        await access(resolved);
      } catch {
        broken.push(`${file}: ${target}`);
      }
    }
  }
  return broken;
}

async function installedFixture() {
  const output = await mkdtemp(path.join(os.tmpdir(), "agent-team-canonical-output-"));
  const extracted = await mkdtemp(path.join(os.tmpdir(), "agent-team-canonical-installed-"));
  temporary.push(output, extracted);
  const built = await buildArtifacts({ sourceRoot, outputDirectory: output, sourceRevision: "d".repeat(40) });
  await run("unzip", ["-q", built.archives[0], "-d", extracted]);
  return path.join(extracted, "agent-team");
}

test("current Pro package uses uppercase canonical Markdown basenames and includes lane assets", async () => {
  const manifest = await manifestAt(sourceRoot);
  assert.deepEqual(noncanonicalMarkdown(manifest.files), []);
  for (const file of laneAssets) assert.ok(manifest.files.includes(file), file);

  for (const role of roles) {
    const file = `assets/claude-agents/AGENT-TEAM-${role.toUpperCase()}.md`;
    assert.ok(manifest.files.includes(file), file);
    assert.match(await readFile(path.join(sourceRoot, file), "utf8"), new RegExp(`^name: agent-team-${role}$`, "m"));
  }
});

test("every packaged local Markdown link resolves within the package", async () => {
  const manifest = await manifestAt(sourceRoot);
  assert.deepEqual(await brokenPackageLinks(sourceRoot, manifest), []);
});

test("SKILL.md remains within the 108-line loading ceiling", async () => {
  const lines = (await readFile(path.join(sourceRoot, "SKILL.md"), "utf8")).split(/\r?\n/);
  if (lines.at(-1) === "") lines.pop();
  assert.ok(lines.length <= 108, `SKILL.md has ${lines.length} lines`);
});

test("installed-package validation rejects a lowercase canonical Markdown basename", async () => {
  const root = await installedFixture();
  const manifest = await manifestAt(root);
  const canonical = "assets/diagrams/README.md";
  const lowercase = "assets/diagrams/readme.md";
  await rename(path.join(root, canonical), path.join(root, lowercase));
  manifest.files = manifest.files.map((file) => file === canonical ? lowercase : file);
  await writeFile(path.join(root, "hooks", "manifest.json"), `${JSON.stringify(manifest, null, 2)}\n`);

  const result = await checkInstalledPackage(root);
  assert.equal(result.status, "failed");
  assert.match(result.errors.join("\n"), /uppercase|canonical Markdown basename/i);
});
