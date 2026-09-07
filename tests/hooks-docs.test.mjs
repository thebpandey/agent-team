import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import path from "node:path";
import test from "node:test";

const root = path.resolve(import.meta.dirname, "..");
const read = (file) => readFile(path.join(root, file), "utf8");

test("release version and guide links stay consistent", async () => {
  // This test catches a hook release published with stale version or missing public guidance.
  const [manifest, skill, readme, changelog, guide] = await Promise.all([
    read("hooks/manifest.json").then(JSON.parse),
    read("SKILL.md"),
    read("README.md"),
    read("CHANGELOG.md"),
    read("references/hooks.md"),
  ]);

  assert.equal(manifest.version, "6.4.0");
  assert.equal(manifest.repository, "https://github.com/thebpandey/agent-team");
  assert.match(skill, /version: "6\.4\.0"/);
  assert.match(readme, /current skill version is \*\*6\.4\.0\*\*/i);
  assert.match(changelog, /^## 6\.4\.0 - 2026-09-07/m);
  for (const source of [skill, readme]) assert.match(source, /\(references\/hooks\.md\)/);
  assert.match(guide, /Requirements 1.?15/i);
});

test("GitHub releases are tag-driven, validated, and publish both runtime archives", async () => {
  const workflow = await read(".github/workflows/release.yml");
  assert.match(workflow, /tags:\s*\n\s*- ['"]v\*['"]/);
  assert.match(workflow, /permissions:\s*\n\s*contents:\s*write/);
  assert.match(workflow, /node --test tests\/hooks-\*\.test\.mjs/);
  assert.match(workflow, /check-package/);
  assert.match(workflow, /build-artifacts/);
  assert.match(workflow, /check-artifacts/);
  assert.match(workflow, /gh release create/);
  assert.match(workflow, /cd release-artifacts\s*\n\s*sha256sum \*\.zip > SHA256SUMS/);
});

test("installation guidance uses the canonical pinned release and update source", async () => {
  const readme = await read("README.md");
  assert.match(readme, /github\.com\/thebpandey\/agent-team\/releases\/tag\/v6\.4\.0/);
  assert.match(readme, /github\.com\/thebpandey\/agent-team\/releases\/latest/);
  assert.match(readme, /\.agent-team-source\.json/);
});

test("LeanCTX stays supplemental and reaches every orchestrator and teammate", async () => {
  // This catches a dispatch contract that omits LeanCTX or lets its memory replace Agent-Team records.
  const rolePaths = [
    "complex", "developer", "reviewer", "routine", "text", "visual-tester",
  ].map((role) => `assets/claude-agents/agent-team-${role}.md`);
  const [skill, dependencies, setup, team, projects, state, codex, claude, leanctx, ...roles] = await Promise.all([
    read("SKILL.md"),
    read("references/dependencies.md"),
    read("references/setup.md"),
    read("references/team.md"),
    read("references/projects.md"),
    read("references/state.md"),
    read("references/platform-codex.md"),
    read("references/platform-claude.md"),
    read("references/lean-ctx.md"),
    ...rolePaths.map(read),
  ]);

  for (const source of [skill, dependencies, setup, team, projects, codex, claude, leanctx, ...roles]) {
    assert.match(source, /LeanCTX|lean-ctx/i);
  }
  assert.match(dependencies, /Project Orchestrator.*Team Orchestrator/s);
  assert.match(dependencies, /loaded.*missing.*unreadable.*disabled.*not applicable/is);
  assert.match(team, /exact|full|raw/i);
  assert.match(projects, /supplemental/i);
  assert.match(projects, /CONTEXT\.md/);
  assert.match(state, /Default to local mode when any of Beads, Ponytail, Using-Superpowers, or Impeccable is skipped\/unusable/);
  assert.match(codex, /init --agent codex/);
  assert.match(claude, /init --agent claude/);
  for (const [index, role] of roles.entries()) {
    assert.match(role, /skill receipt/i, `missing receipt in ${rolePaths[index]}`);
    assert.match(role, /loaded.*missing.*unreadable.*disabled.*not applicable/is, `missing statuses in ${rolePaths[index]}`);
    assert.match(role, /exact|full/i, `missing exact recovery in ${rolePaths[index]}`);
    assert.match(role, /raw|uncompressed/i, `missing raw diagnostics in ${rolePaths[index]}`);
    assert.match(role, /authoritative/i, `missing authority boundary in ${rolePaths[index]}`);
  }
});

test("LeanCTX profile keeps native recovery and Agent-Team authority", async () => {
  const [leanctx, projects, claude] = await Promise.all([
    read("references/lean-ctx.md"),
    read("references/projects.md"),
    read("references/platform-claude.md"),
  ]);

  for (const required of [
    'tool_profile = "standard"',
    'shadow_mode = false',
    'prompt_reinject = "off"',
    'prefer_native_editor = true',
    'proxy_enabled = false',
    'rules_injection = "dedicated"',
    'tee_mode = "always"',
    'response_verbosity = "full"',
    'ctx_session',
    'ctx_knowledge',
    'ctx_handoff',
    'ctx_call',
    'ctx_execute',
    'ctx_expand',
    'fresh=true',
    'lean-ctx -c --raw "command"',
    'LEAN_CTX_RAW=1',
    'LEAN_CTX_DISABLED=1',
    '"shell"',
  ]) assert.ok(leanctx.includes(required), `Missing LeanCTX safeguard: ${required}`);
  assert.doesNotMatch(leanctx, /lean-ctx -c "command" --raw/);
  assert.doesNotMatch(leanctx, /response_verbosity = "normal"/);
  assert.match(leanctx, /autoApprove/);
  assert.match(leanctx, /permissions\.allow/);
  assert.match(claude, /autoApprove/);
  assert.match(claude, /permissions\.allow/);
  assert.match(leanctx, /Do not install RTK, Headroom, or another automatic context compressor/);
  assert.match(leanctx, /do not create.*ledger/i);
  assert.match(projects, /must not create another tracker.*prove completion.*override a current record/i);
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
