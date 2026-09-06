import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { runLintChecks } from "../hooks/lib/lint.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { force: true, recursive: true }))));

async function root() {
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-lint-"));
  temporary.push(directory);
  return directory;
}

async function executable(directory, source) {
  const binary = path.join(directory, "node_modules", ".bin", "eslint");
  await mkdir(path.dirname(binary), { recursive: true });
  await writeFile(binary, `#!/usr/bin/env node\n${source}\n`);
  await chmod(binary, 0o755);
}

test("lint batches deduplicated changed files by nearest monorepo config and excludes generated paths", async () => {
  // This test catches per-file process spawning and use of the wrong package config.
  const directory = await root();
  await mkdir(path.join(directory, "apps/a/src"), { recursive: true });
  await mkdir(path.join(directory, "apps/b/src"), { recursive: true });
  await writeFile(path.join(directory, "apps/a/eslint.config.js"), "export default [];\n");
  await writeFile(path.join(directory, "apps/b/eslint.config.js"), "export default [];\n");
  await executable(directory, "process.stdout.write(process.argv.slice(2).join(','));");

  const result = await runLintChecks(directory, [
    "apps/a/src/a.js", "apps/a/src/a.js", "apps/b/src/b.js", "apps/a/dist/generated.js", "vendor/file.js",
  ]);

  assert.equal(result.status, "checked");
  assert.equal(result.batches.length, 2);
  assert.deepEqual(result.batches.map(({ files }) => files), [["src/a.js"], ["src/b.js"]]);
});

test("missing lint executables skip without installing or changing package files", async () => {
  // This test catches a hook that installs a missing linter.
  const directory = await root();
  await writeFile(path.join(directory, "package.json"), "{\"name\":\"fixture\"}\n");
  const before = await readFile(path.join(directory, "package.json"), "utf8");
  const result = await runLintChecks(directory, ["index.js"]);
  const after = await readFile(path.join(directory, "package.json"), "utf8");

  assert.equal(result.status, "skipped");
  assert.equal(result.reason, "missing_executable");
  assert.equal(after, before);
});

test("lint timeouts and output are bounded", async () => {
  // This test catches a hook that can hang or return unbounded tool output.
  const directory = await root();
  await writeFile(path.join(directory, "eslint.config.js"), "export default [];\n");
  await executable(directory, "process.stdout.write('x'.repeat(10000)); setTimeout(() => {}, 5000);");
  const result = await runLintChecks(directory, ["index.js"], { timeoutMs: 50, maxOutputBytes: 80 });

  assert.equal(result.status, "timeout");
  assert.ok(result.output.length <= 80);
});
