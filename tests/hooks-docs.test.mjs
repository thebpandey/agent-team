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
    "PreToolUse", "TaskCompleted", "UserPromptExpansion", ".agents/skills/agent-team", ".claude/skills/agent-team",
    "health", "audit", "install", "uninstall", "check-package", "check-artifacts", "unsupported", "trusted",
    "Node.js 24", "standard library", "legacy/claude-v3", "6c4e5ad39220f50f1059f2cde77f046a61158f6a", "Apache-2.0",
  ]) assert.ok(guide.includes(required), `Missing hook guide term: ${required}`);
});

test("feature requests and start actions keep distinct task-creation rules", async () => {
  // This assertion protects the requested distinction between scoped creation and tracker selection.
  const actions = await read("references/actions.md");
  assert.match(actions, /full natural-language.*feature request.*create.*canonical.*task/i);
  assert.match(actions, /start <name>.*already-defined.*tracker/i);
  assert.match(actions, /bare `start`.*existing ready/i);
  assert.match(actions, /hooks.*do not create task records.*arbitrary prompts/i);
});
