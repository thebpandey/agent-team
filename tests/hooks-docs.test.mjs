import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import path from "node:path";
import test from "node:test";

const root = path.resolve(import.meta.dirname, "..");
const read = (file) => readFile(path.join(root, file), "utf8");

test("hook release version and guide links stay consistent", async () => {
  // This test catches a hook release published with stale version or missing public guidance.
  const [manifest, skill, readme, changelog, guide] = await Promise.all([
    read("hooks/manifest.json").then(JSON.parse),
    read("SKILL.md"),
    read("README.md"),
    read("CHANGELOG.md"),
    read("references/hooks.md"),
  ]);

  assert.equal(manifest.version, "6.2.0");
  assert.match(skill, /version: "6\.2\.0"/);
  assert.match(readme, /current skill version is \*\*6\.2\.0\*\*/i);
  assert.match(changelog, /^## 6\.2\.0 - 2026-09-06/m);
  for (const source of [skill, readme]) assert.match(source, /\(references\/hooks\.md\)/);
  assert.match(guide, /Requirements 1.?15/i);
});

test("hook guide documents runtime, safety, install, audit, and archive contracts", async () => {
  // This test catches removal of a required operator contract from the shared guide.
  const guide = await read("references/hooks.md");
  for (const required of [
    "PreToolUse", "PostToolBatch", "TaskCompleted", "UserPromptExpansion", ".agents/skills/agent-team", ".claude/skills/agent-team",
    "health", "audit", "install", "uninstall", "check-package", "check-artifacts", "unsupported", "trusted",
    "Node.js 24", "standard library", "legacy/claude-v3", "6c4e5ad39220f50f1059f2cde77f046a61158f6a", "Apache-2.0",
  ]) assert.ok(guide.includes(required), `Missing hook guide term: ${required}`);
});

test("Claude declaration batches post-tool checks without a duplicate per-edit hook", async () => {
  // This test catches Claude lint registration on both per-tool and batch events.
  const declaration = JSON.parse(await read("hooks/claude-hooks.json"));
  assert.equal(declaration.hooks.PostToolBatch.length, 1);
  assert.equal(declaration.hooks.PostToolUse, undefined);
});

test("hook guide states factual coverage and installed Claude role behavior", async () => {
  // This test catches claims that heuristic advice or explicit mappings form a complete safety boundary.
  const guide = await read("references/hooks.md");
  for (const pattern of [
    /heuristic.*not proof/i,
    /explicit.*mapping.*not.*universal security boundary/i,
    /validated separate mapping cache.*fails closed/i,
    /unmapped read-only provider.*continue.*unavailable advice/i,
    /read-only.*recovery snapshot/i,
    /\.claude\/agents/i,
    /factual correlation.*does not prove.*effectiveness/i,
  ]) assert.match(guide, pattern);
});

test("feature requests and start actions keep distinct task-creation rules", async () => {
  // This assertion protects the requested distinction between scoped creation and tracker selection.
  const actions = await read("references/actions.md");
  assert.match(actions, /full natural-language.*feature request.*create.*canonical.*task/i);
  assert.match(actions, /start <name>.*already-defined.*tracker/i);
  assert.match(actions, /bare `start`.*existing ready/i);
  assert.match(actions, /hooks.*do not create task records.*arbitrary prompts/i);
});

test("README gives a complete macOS checksum command", async () => {
  // This test catches a checksum example that refers to an undefined shell variable.
  const readme = await read("README.md");
  assert.match(readme, /On macOS, use `shasum -a 256 \.\.\/agent-team-artifacts\/\*\.zip`/);
  assert.doesNotMatch(readme, /\$package_archive/);
});

test("mapping cache guidance states its non-authoritative threat model and visible health", async () => {
  // This test catches documentation that presents unsigned hook identity as cache authority.
  const [guide, setup] = await Promise.all([read("references/hooks.md"), read("references/setup.md")]);
  assert.doesNotMatch(guide, /migrate-mappings|--session/i);
  assert.match(guide, /health --project/i);
  assert.match(guide, /missing or invalid.*fallback protection.*unavailable/i);
  assert.match(guide, /cache.*not.*task ledger/i);
  assert.match(guide, /non-authoritative.*validated canonical state/i);
  assert.match(guide, /unsigned.*runtime.*event.*session/i);
  assert.match(guide, /never grants authority/i);
  assert.match(setup, /any session identity.*SessionStart.*healthy canonical state/i);
  assert.match(setup, /read-only.*status.*do not.*cache/i);
});
