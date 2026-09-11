import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { runLintChecks } from "../hooks/lib/lint.mjs";
import { createEventBudget } from "../hooks/lib/budget.mjs";

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

test("mixed lint batches preserve passed, failed, missing and timeout evidence with recoverable diagnostics", async () => {
  // Catches early returns discarding completed batches and failed batches reported as checked.
  const directory = await root();
  for (const name of ["passed", "failed", "missing", "timeout"]) {
    await mkdir(path.join(directory, name), { recursive: true });
    await writeFile(path.join(directory, name, "eslint.config.js"), "export default [];\n");
  }
  await executable(path.join(directory, "passed"), "process.stdout.write('ok');");
  await executable(path.join(directory, "failed"), "process.stdout.write('bad.js:2:3 no-undef brokenName\\n' + 'detail '.repeat(1000)); process.exitCode = 1;");
  await executable(path.join(directory, "timeout"), "setTimeout(() => {}, 5000);");
  const result = await runLintChecks(directory, ["passed/ok.js", "failed/bad.js", "missing/unknown.js", "timeout/slow.js"], { timeoutMs: 500, maxOutputBytes: 120 });
  assert.equal(result.status, "failed");
  assert.deepEqual(result.batches.map(({ status }) => status), ["passed", "failed", "skipped", "timeout"]);
  assert.match(result.batches[1].output, /bad.js:2:3.*brokenName/);
  assert.ok(Buffer.byteLength(result.output) <= 120);
  assert.match(await readFile(result.batches[1].logPath, "utf8"), /detail detail detail/);
  assert.ok((await readFile(result.batches[1].logPath, "utf8")).length > 6000);
});

test("successful batches plus missing lint remain incomplete", async () => {
  const directory = await root();
  for (const name of ["passed", "missing"]) {
    await mkdir(path.join(directory, name));
    await writeFile(path.join(directory, name, "eslint.config.js"), "export default [];\n");
  }
  await executable(path.join(directory, "passed"), "process.stdout.write('ok');");
  const result = await runLintChecks(directory, ["passed/ok.js", "missing/no.js"]);
  assert.equal(result.status, "incomplete");
  assert.deepEqual(result.batches.map(({ status }) => status), ["passed", "skipped"]);
});

test("slow config discovery respects the event budget without starting a linter", async () => {
  const directory = await root();
  const budget = createEventBudget(30);
  const started = performance.now();
  try {
    const result = await runLintChecks(directory, ["index.js"], { budget, filesystem: {
      access: async () => { await new Promise((resolve) => setTimeout(resolve, 180)); },
    } });
    assert.equal(result.status, "timeout");
    assert.ok(performance.now() - started < 130);
    assert.equal(result.batches[0].reason, "event_deadline");
  } finally { budget.close(); }
});

test("delayed log-directory creation cannot start a log write after event deadline", async () => {
  const directory = await root();
  await executable(directory, "process.stdout.write('index.js:1:1 broken'); process.exitCode=1;");
  const budget = createEventBudget(150);
  let wroteAfterDelay = false;
  const started = performance.now();
  try {
    const result = await runLintChecks(directory, ["index.js"], { budget, filesystem: {
      mkdir: async () => { await new Promise((resolve) => setTimeout(resolve, 200)); },
      writeFile: async () => { wroteAfterDelay = true; },
    }, runCommand: async () => {
      throw Object.assign(new Error("lint failed"), {
        code: 1,
        stdout: "index.js:1:1 broken",
        stderr: "",
      });
    } });
    assert.ok(performance.now() - started < 180);
    assert.equal(result.status, "failed");
    assert.equal(result.batches[0].logStatus, "unavailable");
    await new Promise((resolve) => setTimeout(resolve, 220));
    assert.equal(wroteAfterDelay, false);
  } finally { budget.close(); }
});
