import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import path from "node:path";
import test from "node:test";

const root = path.resolve(import.meta.dirname, "..");
const read = file => readFile(path.join(root, file), "utf8");
const readMany = files => Promise.all(files.map(read));

test("field guide embeds compact WebP artwork without a local image dependency", async () => {
  const [guide, readme, rawManifest] = await readMany([
    "agent-team-guide-v7.1.0.html", "README.md", "hooks/manifest.json",
  ]);
  const manifest = JSON.parse(rawManifest);
  const imagePaths = [
    "assets/guide/agent-team-essence-16x9.webp",
    "assets/guide/agent-team-harness-flow-3x4.webp",
  ];
  const embedded = [...guide.matchAll(/src="data:image\/webp;base64,([^"]+)"/g)]
    .map(match => Buffer.from(match[1], "base64"));

  assert.equal(embedded.length, 2);
  assert.doesNotMatch(guide, /src="assets\/guide\//);
  assert.match(guide, /modalImage\.src = sourceImage\.src/);
  for (const [index, imagePath] of imagePaths.entries()) {
    const source = await readFile(path.join(root, imagePath));
    assert.deepEqual(embedded[index], source);
    assert.ok(manifest.files.includes(imagePath));
    assert.ok(readme.includes(imagePath));
  }
});

test("every field-guide image opens through one borderless modal", async () => {
  const guide = await read("agent-team-guide-v7.1.0.html");
  assert.equal((guide.match(/class="image-open"/g) || []).length, 2);
  assert.equal((guide.match(/<dialog id="image-dialog"/g) || []).length, 1);
  assert.match(guide, /dialog \{[^}]*border:0;[^}]*background:transparent;/);
  assert.match(guide, /querySelectorAll\("\.image-open"\)/);
  assert.match(guide, /modalImage\.src = sourceImage\.src/);
});

test("field guide explains bounded memory and context management", async () => {
  const guide = await read("agent-team-guide-v7.1.0.html");
  for (const required of [
    'id="context"', "Small task packets", "One source of truth", "Relevant reading",
    "Bounded checkpoints", "Native auto-compaction", "LeanCTX", "under 600 words",
    "Tracker", "Project documents", "MISTAKES.md",
  ]) assert.ok(guide.includes(required), required);
  assert.match(guide, /does not make the context window larger/i);
  assert.match(guide, /does not become a second task tracker/i);
});

test("7.1 field guide retains complete onboarding and documents its new operating model", async () => {
  const [guide, index, readme, rawManifest] = await readMany([
    "agent-team-guide-v7.1.0.html", "index.html", "README.md", "hooks/manifest.json",
  ]);
  const manifest = JSON.parse(rawManifest);
  for (const required of [
    "Version 7.1.0", "GPT-5.6-Sol", "medium effort", "Opus 5 fallback",
    "pure orchestrator", "parallel set", "continuous supervision", "Graphify",
    "Serena", 'shell_security = "warn"', "LeanCTX",
  ]) assert.match(guide, new RegExp(required, "i"), required);
  for (const required of [
    "Install the complete package", "Choose settings", "Launch a run",
    "Inspect the optional local dashboard", "Use Project Kickoff first",
  ]) assert.ok(guide.includes(required), required);
  assert.ok(manifest.rootFiles.includes("agent-team-guide-v7.1.0.html"));
  assert.ok(manifest.files.includes("agent-team-guide-v7.1.0.html"));
  assert.match(index, /agent-team-guide-v7\.1\.0\.html/);
  assert.match(readme, /agent-team-guide-v7\.1\.0\.html/);
});

test("7.0.2 guide remains a reachable historical edition", async () => {
  const [historical, current, rawManifest] = await readMany([
    "agent-team-guide-v7.0.2.html", "agent-team-guide-v7.1.0.html", "hooks/manifest.json",
  ]);
  const manifest = JSON.parse(rawManifest);
  assert.match(historical, /Version 7\.0\.2/);
  assert.match(current, /agent-team-guide-v7\.0\.2\.html/);
  assert.ok(manifest.rootFiles.includes("agent-team-guide-v7.0.2.html"));
  assert.ok(manifest.files.includes("agent-team-guide-v7.0.2.html"));
});

// Text contracts catch instruction drift; behavioral/consumer/native tests remain separate gates.
test("release version and public guidance stay consistent", async () => {
  const [raw, skill, readme, changelog, guide] = await readMany(["hooks/manifest.json", "SKILL.md", "README.md", "CHANGELOG.md", "references/hooks.md"]);
  const manifest = JSON.parse(raw);
  assert.match(manifest.version, /^\d+\.\d+\.\d+$/);
  assert.equal(manifest.repository, "https://github.com/thebpandey/agent-team");
  assert.ok(skill.includes('version: "' + manifest.version + '"'));
  assert.ok(readme.includes("current skill version is **" + manifest.version + "**"));
  assert.ok(changelog.includes("## " + manifest.version + " - "));
  for (const source of [skill, readme]) assert.match(source, /\(references\/hooks\.md\)/);
  assert.match(guide, /Requirements 1.?15/i);
});

test("GitHub release publication remains tag-driven and checked", async () => {
  const workflow = await read(".github/workflows/release.yml");
  for (const pattern of [
    /tags:\s*\n\s*- ['"]v\*['"]/, /permissions:\s*\n\s*contents:\s*write/,
    /node --test tests\/hooks-\*\.test\.mjs/, /check-package/, /build-artifacts/,
    /check-artifacts/, /gh release create/, /sha256sum \*\.zip > SHA256SUMS/,
  ]) assert.match(workflow, pattern);
});

test("installation docs keep canonical release provenance and complete checksum examples", async () => {
  const manifest = JSON.parse(await read("hooks/manifest.json"));
  const readme = await read("README.md");
  if (readme.includes("**unpublished local preview**")) {
    assert.match(readme, /native-host qualification and publication are pending/i);
    assert.match(readme, /Published v6\.5\.0.*releases\/tag\/v6\.5\.0/);
  } else assert.ok(readme.includes("github.com/thebpandey/agent-team/releases/tag/v" + manifest.version));
  assert.match(readme, /github\.com\/thebpandey\/agent-team\/releases\/latest/);
  assert.match(readme, /\.agent-team-source\.json/);
  assert.match(readme, /shasum -a 256/);
  assert.doesNotMatch(readme, /\$package_archive/);
});

test("prepared capabilities load selectively in each fresh worker context", async () => {
  const [skill, dependencies, team, projects, state, ...roles] = await readMany([
    "SKILL.md", "references/dependencies.md", "references/team.md", "references/projects.md", "references/state.md",
    ...["complex", "developer", "reviewer", "routine", "text", "visual-tester"].map(role => "assets/claude-agents/agent-team-" + role + ".md"),
  ]);
  assert.match(skill, /Do not fork the full conversation or load every prepared skill/i);
  assert.match(dependencies, /complete SKILL\.md/i);
  assert.match(dependencies, /parent.*receipt.*not a read/i);
  assert.match(dependencies, /retained context/i);
  assert.match(dependencies, /Mandatory.*|Serena/);
  assert.match(dependencies, /Microsoft Playwright CLI/);
  assert.match(dependencies, /do not require every worker to print a four-tool matrix/i);
  assert.match(team, /exact|full|raw/i);
  assert.match(projects, /LeanCTX is supplemental/);
  assert.match(projects, /must not create another tracker.*prove completion.*override a current record/i);
  assert.match(state, /Skill availability never selects or migrates the tracker/);
  for (const role of roles) {
    assert.match(role, /complete applicable skills\/references/i);
    assert.match(role, /parent receipt is not a read/i);
    assert.match(role, /Do not load every prepared skill/i);
    assert.match(role, /exact source/i);
    assert.match(role, /selected tracker owns task state/i);
    assert.match(role, /do not install, spawn agents or expand permissions/i);
  }
});

test("LeanCTX retains exact recovery without broad initialization or permission bypass", async () => {
  const [leanctx, codex, claude] = await readMany(["references/lean-ctx.md", "references/platform-codex.md", "references/platform-claude.md"]);
  for (const required of [
    'tool_profile = "standard"', "shadow_mode = false", 'prompt_reinject = "off"',
    "prefer_native_editor = true", "proxy_enabled = false", 'tee_mode = "always"',
    'response_verbosity = "full"', "ctx_session", "ctx_knowledge", "ctx_handoff",
    "ctx_call", "ctx_execute", "ctx_expand", "autoApprove", "permissions.allow",
  ]) assert.ok(leanctx.includes(required), required);
  assert.match(leanctx, /Do not invoke broad/);
  assert.match(leanctx, /command denial is not permission to retry/i);
  assert.doesNotMatch(leanctx + codex + claude, /then run .lean-ctx init/);
  assert.match(leanctx, /Do not install RTK, Headroom, or another automatic context compressor/);
  assert.match(leanctx, /Do not create a LeanCTX ledger/);
  assert.match(claude, /Do not import full-catalog/);
});

test("hook guide documents runtime, safety, ownership and universal archive contracts", async () => {
  const guide = await read("references/hooks.md");
  for (const required of [
    "PreToolUse", "PostToolBatch", "TaskCompleted", "UserPromptExpansion",
    ".agents/skills/agent-team", ".claude/skills/agent-team", ".claude/agents",
    "health", "audit", "install", "uninstall", "check-package", "check-artifacts",
    "unsupported", "trusted", "Node.js 24", "standard library", "legacy/claude-v3",
    "6c4e5ad39220f50f1059f2cde77f046a61158f6a", "Apache-2.0", "universal ZIP",
    "Handler-level ownership", "--host codex --scope user",
  ]) assert.ok(guide.includes(required), required);
  for (const pattern of [
    /heuristic.*not proof/i, /explicit.*mapping.*not.*universal security boundary/i,
    /validated separate mapping cache.*fails closed/i,
    /unmapped read-only provider.*continue.*unavailable advice/i,
    /read-only.*recovery snapshot/i, /factual correlation.*does not prove.*effectiveness/i,
  ]) assert.match(guide, pattern);
});

test("Claude batches post-tool checks without duplicate per-edit hooks", async () => {
  const declaration = JSON.parse(await read("hooks/claude-hooks.json"));
  assert.equal(declaration.hooks.PostToolBatch.length, 1);
  assert.equal(declaration.hooks.PostToolUse, undefined);
});

test("feature requests and named starts retain distinct task-creation rules", async () => {
  const actions = await read("references/actions.md");
  assert.match(actions, /full natural-language.*feature request.*create.*canonical.*task/i);
  assert.match(actions, /start <name>.*already-defined.*tracker/i);
  assert.match(actions, /bare .start..*existing ready/i);
  assert.match(actions, /hooks.*do not create task records.*arbitrary prompts/i);
  assert.match(actions, /unavailable Beads backend never activates a fallback tracker/);
});

test("mapping cache and hook identities never become authority", async () => {
  const guide = await read("references/hooks.md");
  assert.doesNotMatch(guide, /migrate-mappings|health[^\n]*--session|mapping[^\n]*--session/i);
  assert.match(guide, /recovery[^\n]*--session/i);
  for (const pattern of [
    /health --project/i, /missing or invalid.*fallback protection.*unavailable/i,
    /cache.*not.*task ledger/i, /non-authoritative.*validated canonical state/i,
    /unsigned.*runtime.*event.*session/i, /never grants authority/i,
    /Read-only and status events do not write it/i,
  ]) assert.match(guide, pattern);
});

test("settings preserve independent host routes and distinguish configured from enforced", async () => {
  const [settings, codex, claude] = await readMany(["references/settings.md", "references/platform-codex.md", "references/platform-claude.md"]);
  assert.match(settings, /trusted host metadata/);
  assert.match(settings, /Persist independent routing per host/);
  assert.match(settings, /without deleting the other profile/);
  assert.match(settings, /Never discard custom routing because the host changed/);
  assert.match(settings, /configured versus actually enforced/);
  assert.match(settings, /cannot switch its parent process\/model/);
  assert.match(codex, /preserving Claude Code routing and all run defaults/);
  assert.match(claude, /preserving Codex routing and all run defaults/);
  assert.doesNotMatch(settings, /remove.*role_routing.*adapter defaults/i);
});

test("settings offer targeted friendly choices and an optional complete wizard", async () => {
  const [settings, setup, actions] = await readMany(["references/settings.md", "references/setup.md", "references/actions.md"]);
  for (const pattern of [/every available role/i, /numbered choices/i, /model.*compatible effort/i,
    /Quality-first is the default/i, /Back and Cancel/i, /Cancelled\/invalid drafts cause no settings write/i,
    /detect concurrent changes/i, /full wizard is opt-in/i, /future dispatches use them/i]) assert.match(settings, pattern);
  assert.match(setup, /grouped choice/i);
  assert.match(actions, /complete wizard is opt-in/i);
  assert.match(settings, /neither kind waives tests or required review/i);
});

test("setup automatically prepares defaults while keeping optional and manual authority boundaries", async () => {
  const [setup, actions, release] = await readMany(["references/setup.md", "references/actions.md", "references/release.md"]);
  for (const pattern of [/automatically install missing mandatory and selected default/i,
    /optional choices remain user decisions/i, /gh auth status/, /gh auth login/,
    /Never ask for a token in chat/i, /administrator approval/i, /never activate a temporary Markdown tracker/i,
    /Project Kickoff is not a prerequisite/i, /Restart\/reload/i, /cannot fabricate trust/i,
    /Read-only status and health do not enter setup/i]) assert.match(setup, pattern);
  assert.match(actions, /Saved auto-deploy does not prompt for confirmation/i);
  assert.match(release, /authority|authorization/i);
  assert.doesNotMatch(setup, /each missing dependency.*one at a time.*numbered/is);
  assert.doesNotMatch(actions + release, /ask whether to keep auto-deploy or use no auto-deploy/i);
});

test("continuity instructions preserve facts and native compaction as fallback", async () => {
  const [skill, state, recovery] = await readMany(["SKILL.md", "references/state.md", "references/recovery.md"]);
  assert.match(skill, /Ordinary lint, test and review failures trigger automatic in-scope repair/);
  assert.match(state, /Required in-scope repair: assign, fix and verify automatically/);
  assert.match(state, /Out-of-scope improvement: record proposed\/deferred, not authorized implementation/);
  assert.match(recovery, /not simply the newest file in the project/);
  assert.match(recovery, /Keep native auto-compaction enabled as fallback/);
  assert.match(recovery, /cannot guarantee automatic replacement of its parent conversation/);
  assert.match(recovery, /Unknown activity does not count as stopped/);
});
