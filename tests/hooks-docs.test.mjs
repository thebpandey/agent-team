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

test("landing-page banner alt text does not misidentify retained artwork as the release version", async () => {
  const index = await read("index.html");
  const banner = index.match(/<img class="banner"[^>]*alt="([^"]+)"/);
  assert.ok(banner);
  assert.match(banner[1], /^Blueprint banner for Agent-Team\./);
  assert.doesNotMatch(banner[1], /version\s+\d+\.\d+\.\d+/i);
});

// Text contracts catch instruction drift; behavioral/consumer/native tests remain separate gates.
test("release version and public guidance stay consistent", async () => {
  const [raw, skill, readme, changelog, guide, gettingStarted, index] = await readMany(["hooks/manifest.json", "SKILL.md", "README.md", "CHANGELOG.md", "references/hooks.md", "GETTING_STARTED.md", "index.html"]);
  const manifest = JSON.parse(raw);
  assert.equal(manifest.version, "7.2.0");
  assert.equal(manifest.repository, "https://github.com/thebpandey/agent-team");
  assert.ok(skill.includes('version: "' + manifest.version + '"'));
  assert.ok(readme.includes("current skill version is **" + manifest.version + "**"));
  assert.ok(changelog.includes("## " + manifest.version + " - "));
  assert.match(index, /Version 7\.2\.0/);
  for (const publicDoc of [readme, gettingStarted]) {
    assert.match(publicDoc, /agent-team-7\.2\.0\.zip/);
    assert.match(publicDoc, /SHA256SUMS/);
    assert.match(publicDoc, /releases\/latest/);
    assert.match(publicDoc, /Linux.*WSL.*\/proc\/self\/fd/is);
    assert.match(publicDoc, /unsupported_platform/is);
    assert.match(publicDoc, /update_requires_manual_replacement/is);
    assert.match(publicDoc, /quiesc.*rollback-backed move.*fresh/is);
    assert.doesNotMatch(publicDoc, /project-kickoff\/releases\/tag\/v0\.4\.1/i);
    assert.match(publicDoc, /project-kickoff\/releases\/latest/i);
  }
  for (const source of [skill, readme]) assert.match(source, /\(references\/hooks\.md\)/);
  assert.match(guide, /Requirements 1.?15/i);
});

test("Graphify guidance accepts AST-origin inferred structural leads only", async () => {
  const [guide, readme] = await readMany(["references/graphify.md", "README.md"]);
  for (const required of [
    "`_origin` distinguishes AST from semantic extraction",
    "confidence describes resolution strength",
    "AST-origin INFERRED relationships are valid offline structural leads",
    "Semantic or missing provenance is invalid for Agent-Team's offline readiness evidence",
    "must not adopt a graph that may contain a prior semantic layer",
  ]) assert.ok(guide.includes(required), required);
  assert.match(readme, /https:\/\/github\.com\/thebpandey\/project-kickoff\/releases\/latest/);
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

test("settings keep targeted edits distinct from the mandatory setup wizard", async () => {
  const [settings, setup, actions] = await readMany(["references/settings.md", "references/setup.md", "references/actions.md"]);
  for (const pattern of [/every available role/i, /numbered choices/i, /model.*compatible effort/i,
    /Quality-first is the default/i, /Back and Cancel/i, /Cancelled\/invalid drafts cause no settings write/i,
    /detect concurrent changes/i, /bare.*settings.*targeted/i, /future dispatches use them/i]) assert.match(settings, pattern);
  for (const pattern of [/every state-changing setup invocation/i, /current effective/i,
    /settingsOutcome.*kept_existing/i, /no answer|timeout|interruption/i, /Read-only.*status.*health.*help.*version/is]) assert.match(setup, pattern);
  assert.match(actions, /state-changing setup.*settings wizard/i);
  assert.match(settings, /neither kind waives tests or required review/i);
});

test("active orchestration follows one ordered question continuation loop", async () => {
  const [skill, runs, actions, team] = await readMany(["SKILL.md", "references/runs.md", "references/actions.md", "references/team.md"]);
  const loop = skill + runs;
  for (const pattern of [
    /consume.*user.*worker.*completion.*handoff.*review.*provider/is,
    /replacement.*addition.*status\/question/is,
    /status\/question.*commentary/is,
    /reconcile.*live worker.*completed handoff/is,
    /repair.*independent review.*serial.*integrat/is,
    /stopped.*ownership transferred.*compute.*free/is,
    /reserve.*review.*admit.*eligible/is,
    /wait.*(?:at most|no more than).*60 seconds/is,
    /heartbeat.*known.*reconcile.*repeat/is,
  ]) assert.match(loop, pattern);
  assert.match(actions, /question.*interrupt.*not.*pause.*cancel.*terminal/is);
  assert.match(actions + team, /commentary.*reconcile.*review.*integrat.*refill.*wait/is);
  assert.match(team, /completion.*independent review.*accepted.*serial.*integrat.*refill/is);
});

test("heartbeats and blockers stay in-turn scoped and finite", async () => {
  const [skill, runs, team, recovery, codex, claude] = await readMany([
    "SKILL.md", "references/runs.md", "references/team.md", "references/recovery.md",
    "references/platform-codex.md", "references/platform-claude.md",
  ]);
  const all = [skill, runs, team, recovery, codex, claude].join("\n");
  for (const pattern of [
    /scoped blocker.*continue.*independent/is,
    /unchanged blocker.*heartbeat/is,
    /global blocker.*no authorized.*safe/is,
    /active workers.*occupied.*free.*unknown.*slots/is,
    /pending decision.*uncertain operation.*pending.*tail.*next eligible/is,
    /one final.*question/is,
    /ordinary.*model.*output.*cost.*unknown/is,
    /no (?:cron|daemon|timer|hosted monitor|nested scheduler)/i,
    /no.*activity.*after.*final.*interruption/is,
  ]) assert.match(all, pattern);
  assert.doesNotMatch(all, /guarantee.*(?:after.*final|after.*host.*interrupt)/i);
});

test("run decisions and underfilled release batches use exact canonical evidence", async () => {
  const [runs, release, status] = await readMany(["references/runs.md", "references/release.md", "references/status.md"]);
  const all = runs + release;
  for (const value of ["unknown", "paused", "unreconciled_completion", "progress_possible",
    "finite_exhausted", "continuous_scope_exhausted", "blocked_tail"]) assert.ok(all.includes(value), value);
  for (const pattern of [
    /unique.*top-level.*delivery.*IDs/i,
    /progress_possible.*refill/is,
    /unknown.*paused.*unreconciled_completion.*never.*select/is,
    /terminal.*cardinality.*never waives.*review.*checks.*preview.*target.*authority.*recovery/is,
    /task-keyed.*source.*integration.*review.*checks.*preview.*target.*recovery/is,
    /outside.*run.*never.*widen.*continuous/is,
  ]) assert.match(all, pattern);
  assert.match(status, /run-decision.*read-only/i);
});

test("dependency instructions separate compatibility ownership and prerequisite edges", async () => {
  const [dependencies, leanctx, graphify] = await readMany(["references/dependencies.md", "references/lean-ctx.md", "references/graphify.md"]);
  const all = dependencies + leanctx + graphify;
  for (const pattern of [
    /reused_unowned/i, /lifecycleOwnership.*unowned/is, /no.*sidecar.*copy.*overwrite/is,
    /rollback.*uninstall.*exclude/is, /required files.*unrelated.*regular/is,
    /LeanCTX.*(?:does not|cannot).*Graphify/is, /prerequisite.*catalog.*edge/is,
    /AST.*_origin.*INFERRED.*structural/is, /semantic.*missing.*(?:invalid|fail)/is,
    /absolute.*canonical.*ast-grep/is, /\/usr\/bin\/sg.*(?:collision|reject|fail)/is,
    /same.*package.*identity/is,
  ]) assert.match(all, pattern);
});

test("native project and migration instructions expose only accepted authority", async () => {
  const [projects, state, hooks] = await readMany(["references/projects.md", "references/state.md", "references/hooks.md"]);
  const all = projects + state + hooks;
  for (const required of ["project-owner-recover", "evidence-store-register", "completion-history-reconcile",
    "run-reconcile", "run-scope-extend", "completion-quarantine", "completion-rebind", "run-decision"]) assert.ok(all.includes(required), required);
  for (const pattern of [
    /native.*host.*session.*cwd/is, /request.*cannot.*(?:assert|mint).*identity/is,
    /four.*record|TEAMS\.md.*state\.json.*setup\.json.*owner-history\.json/is,
    /initialization.*immutable.*scope.*additive.*version/is,
    /completion.*integration.*publication.*task-keyed/is,
    /generatedBy.*testedAgainst.*loadedRuntime.*sourceCandidate.*readiness/is,
    /local_only/i, /enabled_but_held.*target_required/is,
  ]) assert.match(all, pattern);
});

test("artifact instructions match the accepted Linux descriptor-root installer", async () => {
  const hooks = await read("references/hooks.md");
  for (const pattern of [
    /install --archive.*--checksums.*--host.*--scope/is,
    /reject.*install --source/is,
    /Linux.*WSL.*\/proc\/self\/fd/is,
    /unsupported_platform.*changed.*false.*descriptor_root/is,
    /schema.?4.*archiveFileMap.*installedFileMaps/is,
    /\.agent-team-source\.json/i,
    /byte.*mode.*size.*identical.*changed.*false/is,
    /update_requires_manual_replacement.*changed.*false/is,
    /descriptor.*reserved.*never.*canonical/is,
    /quiesc.*rollback.*move.*fresh/is,
    /current.*drifted.*unverified_legacy.*missing_receipt.*not_installed/is,
  ]) assert.match(hooks, pattern);
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
