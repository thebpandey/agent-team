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
    "Historical v7.1.0 guide", "historical guide; it is not the current native release", "GPT-5.6-Sol", "medium effort", "Opus 5 fallback",
    "pure orchestrator", "parallel set", "continuous supervision", "Graphify",
    "Serena", 'shell_security = "warn"', "LeanCTX",
  ]) assert.match(guide, new RegExp(required, "i"), required);
  for (const required of [
    "Install the complete package", "Choose settings", "Launch a run",
    "Inspect the optional local dashboard", "Use Project Kickoff first",
  ]) assert.ok(guide.includes(required), required);
  assert.ok(manifest.rootFiles.includes("agent-team-guide-v7.1.0.html"));
  assert.ok(manifest.files.includes("agent-team-guide-v7.1.0.html"));
  assert.match(index, /GETTING_STARTED\.md/);
  assert.match(index, /github\.com\/thebpandey\/agent-team/);
  assert.match(readme, /GETTING_STARTED\.md/);
  assert.doesNotMatch(readme, /agent-team-guide-v7\.[0-2]\.\d+\.html/);
});

test("7.0.2 guide remains a reachable historical edition", async () => {
  const [historical, current, rawManifest] = await readMany([
    "agent-team-guide-v7.0.2.html", "agent-team-guide-v7.1.0.html", "hooks/manifest.json",
  ]);
  const manifest = JSON.parse(rawManifest);
  assert.match(historical, /Historical v7\.0\.2 guide/);
  assert.match(historical, /historical guide; it is not the current native release/i);
  assert.match(current, /agent-team-guide-v7\.0\.2\.html/);
  assert.ok(manifest.rootFiles.includes("agent-team-guide-v7.0.2.html"));
  assert.ok(manifest.files.includes("agent-team-guide-v7.0.2.html"));
});

test("landing-page workflow board is current and has a useful accessible description", async () => {
  const index = await read("index.html");
  assert.match(index, /class="hero-board" role="img" aria-labelledby="hero-board-title hero-board-desc"/);
  assert.match(index, /same Git project, either host/);
  assert.match(index, /Agree on the work and files to change/);
  assert.match(index, /Build \+ review/);
  assert.doesNotMatch(index, /agent-team-banner-ultrawide\.webp/);
  const version = (await read("vnext/VERSION")).trim();
  assert.ok(index.includes(`Agent-Team ${version} development flow`));
  assert.match(index, /aria-labelledby="flow-title flow-desc"/);
  assert.match(index, /<text x="1120" y="80">Release<\/text>/);
  assert.match(index, /with your approval/);
  assert.match(index, /Review findings go back to the agent that made the change/);
});

test("landing page links current source and the versioned native release", async () => {
  const index = await read("index.html");
  const version = (await read("vnext/VERSION")).trim();
  assert.match(index, /href="https:\/\/github\.com\/thebpandey\/agent-team">View source<\/a>/);
  assert.ok(index.includes(`href="https://github.com/thebpandey/agent-team/releases/tag/v${version}"`));
  assert.ok(index.includes(`docs/releases/${version}-readiness.md`));
});

// Text contracts catch instruction drift; behavioral/consumer/native tests remain separate gates.
test("release version and public guidance stay consistent", async () => {
  const [raw, skill, readme, changelog, guide, release, gettingStarted, index] = await readMany(["hooks/manifest.json", "SKILL.md", "README.md", "CHANGELOG.md", "references/HOOKS.md", "references/RELEASE.md", "GETTING_STARTED.md", "index.html"]);
  const manifest = JSON.parse(raw);
  assert.equal(manifest.version, "7.3.1");
  assert.equal(manifest.repository, "https://github.com/thebpandey/agent-team");
  const nativeVersion = (await read("vnext/VERSION")).trim();
  assert.ok(skill.includes('version: "' + nativeVersion + '"'));
  assert.match(skill, /Route `setup`, `status`, and `start` through the installed native `agent-teamctl` contract\./);
  assert.match(skill, /Historical Node package v7\.3\.1 materials are historical context only and are not native authority\./);
  assert.ok(readme.includes(`**[v${nativeVersion}]`));
  assert.ok(changelog.includes(`## ${nativeVersion} - `));
  assert.ok(changelog.includes("## " + manifest.version + " - "));
  assert.ok(index.includes(`Agent<span>-Team</span> / ${nativeVersion}`));
  for (const publicDoc of [readme, gettingStarted]) {
    assert.match(publicDoc, /agent-team-7\.3\.1\.zip/);
    assert.match(publicDoc, /SHA256SUMS/);
    assert.match(publicDoc, /releases\/latest/);
    assert.match(publicDoc, /Linux.*WSL.*\/proc\/self\/fd/is);
    assert.match(publicDoc, /unsupported_platform/is);
    assert.match(publicDoc, /update_requires_manual_replacement/is);
    assert.match(publicDoc, /quiesc.*rollback-backed move.*fresh/is);
    assert.doesNotMatch(publicDoc, /project-kickoff\/releases\/tag\/v0\.4\.1/i);
    assert.match(publicDoc, /project-kickoff\/releases\/latest/i);
  }
  assert.match(release, /manual.*origin:refs\/heads\/main.*git-push.*exact revision.*task/i);
  for (const source of [skill, readme]) assert.match(source, /\(references\/HOOKS\.md\)/);
  assert.match(guide, /Requirements 1.?15/i);
});

test("displayed native versions use the release authority and historical versions are labeled", async () => {
  const nativeVersion = (await read("vnext/VERSION")).trim();
  const [release, rootSkill, codexSkill, claudeSkill, readme, gettingStarted, changelog, readiness, index, wordmark, workerContract, guide70, guide71] = await readMany([
    "vnext/RELEASE.json", "SKILL.md", "vnext/codex/SKILL.md", "vnext/claude/SKILL.md", "README.md", "GETTING_STARTED.md", "CHANGELOG.md", `docs/releases/${nativeVersion}-readiness.md`, "index.html", "references/WORDMARK.md", "vnext/WORKER-CONTRACT", "agent-team-guide-v7.0.2.html", "agent-team-guide-v7.1.0.html",
  ]);

  assert.equal(JSON.parse(release).version, nativeVersion);
  assert.equal(JSON.parse(workerContract).version, nativeVersion);
  for (const entrypoint of [rootSkill, codexSkill, claudeSkill]) {
    assert.match(entrypoint, new RegExp(`metadata:\\n  version: "${nativeVersion}"`));
  }
  assert.match(readme, new RegExp(`native version is \\*\\*\\[v${nativeVersion}\\]`));
  assert.match(gettingStarted, new RegExp(`agent-teamctl-${nativeVersion}-windows-amd64\\.zip`));
  assert.match(changelog, new RegExp(`^## ${nativeVersion} - `, "m"));
  assert.match(readiness, new RegExp(`^# Agent-Team ${nativeVersion} release readiness`, "m"));
  for (const displayedVersion of index.matchAll(/\\b8\\.\\d+\\.\\d+\\b/g)) {
    assert.equal(displayedVersion[0], nativeVersion, "Pages must not display a stale native version");
  }
  assert.match(index, new RegExp(`Agent<span>-Team</span> / ${nativeVersion}`));
  assert.match(index, new RegExp(`AGENT-TEAM / ${nativeVersion}`));
  assert.match(index, new RegExp(`Agent-Team ${nativeVersion} · Created by`));
  assert.match(wordmark, /from metadata\.version in the installed SKILL\.md/);
  assert.match(wordmark, /Use the installed version, not a guessed latest version\./);

  assert.match(readme, /historical Node package \*\*v7\.3\.1\*\*/i);
  assert.match(gettingStarted, /Node-based Agent-Team 7\.3\.1 instructions below are legacy/i);
  for (const guide of [guide70, guide71]) {
    assert.match(guide, /Historical Agent-Team v7\.\d+\.\d+ Field Guide/);
    assert.match(guide, /historical guide; it is not the current native release/i);
  }
});

test("Graphify guidance accepts AST-origin inferred structural leads only", async () => {
  const [guide, readme] = await readMany(["references/GRAPHIFY.md", "README.md"]);
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
  const [workflow, packageWorkflow] = await readMany([
    ".github/workflows/release.yml",
    ".github/workflows/check-package.yml",
  ]);
  for (const pattern of [
    /tags:\s*\n\s*- ['"]v7\.\*['"]/, /permissions:\s*\n\s*contents:\s*write/,
    /node --test tests\/hooks-\*\.test\.mjs/, /check-package/, /build-artifacts/,
    /check-artifacts/, /gh release create/, /sha256sum \*\.zip > SHA256SUMS/,
  ]) assert.match(workflow, pattern);
  assert.doesNotMatch(workflow, /- ['"]v\*['"]/);
  assert.match(packageWorkflow, /tags:\s*\n\s*- ['"]v7\.\*['"]/);
  assert.doesNotMatch(packageWorkflow, /on:\s*\n\s*pull_request:/);
});

test("published static site and native CI use portable repository paths", async () => {
  await read(".nojekyll");
  const attributes = await read(".gitattributes");
  assert.match(attributes, /\*\.go\s+text\s+eol=lf/);
  assert.match(attributes, /\*\.ya?ml\s+text\s+eol=lf/);
  for (const path of [
    ".github/workflows/vnext-host.yml",
    ".github/workflows/vnext-native.yml",
    ".github/workflows/vnext-install.yml",
  ]) {
    const workflow = await read(path);
    assert.doesNotMatch(workflow, /\$\{\{\s*runner\.temp\s*\}\}/, path);
    assert.match(workflow, /- name: Use canonical test temp\s+shell: bash\s+run: \|/, path);
    assert.ok(workflow.includes(`printf 'TMPDIR=%s\\nTMP=%s\\nTEMP=%s\\n' "$RUNNER_TEMP" "$RUNNER_TEMP" "$RUNNER_TEMP" >> "$GITHUB_ENV"`), path);
  }
});

test("installation docs keep canonical release provenance and complete checksum examples", async () => {
  const manifest = JSON.parse(await read("hooks/manifest.json"));
  const [readme, gettingStarted] = await readMany(["README.md", "GETTING_STARTED.md"]);
  if (readme.includes("**unpublished local preview**")) {
    assert.match(readme, /native-host qualification and publication are pending/i);
    assert.match(readme, /Published v6\.5\.0.*releases\/tag\/v6\.5\.0/);
  } else assert.ok(readme.includes("github.com/thebpandey/agent-team/releases/tag/v" + manifest.version));
  assert.match(readme, /github\.com\/thebpandey\/agent-team\/releases\/latest/);
  assert.match(readme, /\.agent-team-source\.json/);
  assert.match(readme, /shasum -a 256/);
  assert.doesNotMatch(readme, /\$package_archive/);
  for (const document of [readme, gettingStarted]) {
    assert.match(document, /\(cd \/absolute\/download && sha256sum -c SHA256SUMS\)/);
    assert.doesNotMatch(document, /sha256sum -c \/absolute\/download\/SHA256SUMS/);
  }
});

test("prepared capabilities load selectively in each fresh worker context", async () => {
  const [skill, dependencies, team, projects, state, ...roles] = await readMany([
    "SKILL.md", "references/DEPENDENCIES.md", "references/TEAM.md", "references/PROJECTS.md", "references/STATE.md",
    ...["complex", "developer", "reviewer", "routine", "text", "visual-tester"].map(role => `assets/claude-agents/AGENT-TEAM-${role.toUpperCase()}.md`),
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
  const [leanctx, codex, claude] = await readMany(["references/LEAN-CTX.md", "references/PLATFORM-CODEX.md", "references/PLATFORM-CLAUDE.md"]);
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
  const guide = await read("references/HOOKS.md");
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
  const actions = await read("references/ACTIONS.md");
  assert.match(actions, /full natural-language.*feature request.*create.*canonical.*task/i);
  assert.match(actions, /start <name>.*already-defined.*tracker/i);
  assert.match(actions, /bare .start..*existing ready/i);
  assert.match(actions, /hooks.*do not create task records.*arbitrary prompts/i);
  assert.match(actions, /unavailable Beads backend never activates a fallback tracker/);
});

test("mapping cache and hook identities never become authority", async () => {
  const guide = await read("references/HOOKS.md");
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
  const [settings, codex, claude] = await readMany(["references/SETTINGS.md", "references/PLATFORM-CODEX.md", "references/PLATFORM-CLAUDE.md"]);
  assert.match(settings, /Hosts are `codex` and `claude`/);
  for (const role of ["orchestrator", "developer", "reviewer", "visual_reviewer"]) assert.ok(settings.includes("`" + role + "`"));
  assert.match(settings, /`coder` is an alias for `developer`/);
  assert.match(settings, /without deleting the first profile or taking over its live workers/);
  assert.match(settings, /Preserve and flag unavailable saved choices rather than silently\s+substituting another model/);
  assert.match(settings, /saving it does not prove account availability or native enforcement/);
  assert.match(settings, /no setting switches the current parent\s+model or retroactively changes an active worker/);
  assert.match(codex, /preserving Claude Code routing and all run defaults/);
  assert.match(claude, /preserving Codex routing and all run defaults/);
  assert.doesNotMatch(settings, /remove.*role_routing.*adapter defaults/i);
});

test("native setup separates observations from readiness and historical hook limits", async () => {
  const [settings, setup, codex, claude, hooks, gettingStarted] = await readMany([
    "references/SETTINGS.md", "references/SETUP.md", "references/PLATFORM-CODEX.md", "references/PLATFORM-CLAUDE.md",
    "references/HOOKS.md", "GETTING_STARTED.md",
  ]);
  assert.match(setup, /actual host from runtime metadata/);
  assert.match(setup, /canonical Git project/);
  assert.match(setup, /Native v8 requires no external hooks/);
  assert.match(setup, /Never claim executable discovery proves\s+MCP registration, worker access, a successful workload, or readiness/);
  assert.match(setup, /Record unavailable or unobserved facts honestly/);
  assert.match(settings, /Missing\s+usage or parent-model metadata stays unknown/);
  assert.match(codex, /same Git project.*continue/is);
  assert.match(claude, /trusted.*home.*process/i);
  assert.match(hooks, /helpers-install/);
  assert.match(hooks, /context-reduction-apply/);
  assert.match(gettingStarted, /canonical records at 16777216 bytes/);
  assert.match(gettingStarted, /subprocess output is bounded at 2097152 bytes/);
  assert.match(gettingStarted, /lane worker updates at 2000 characters/);
});

test("workflow guides route lane state and evidence through the single lane protocol", async () => {
  const names = ["TEAM", "RUNS", "STATE", "RECOVERY", "RELEASE", "OUTPUT", "ACTIONS", "STATUS"];
  const guides = await readMany(names.map((name) => `references/${name}.md`));
  for (const [index, guide] of guides.entries()) {
    assert.match(guide, /\[lanes?\]\(LANES\.md(?:#[^)]+)?\)/i, names[index]);
  }
  assert.match(guides[0], /brief.*packet.*handover/is);
  assert.match(guides[1], /open lane.*reserve.*capacity/is);
  assert.match(guides[2], /tracker remains.*task authority/i);
  assert.match(guides[3], /unknown liveness.*occupied/i);
  assert.match(guides[4], /queue.*empty.*integrated/is);
  assert.match(guides[5], /2000-character.*worker update/i);
  assert.match(guides[6], /lane-(?:create|next|rotate|close)/);
  assert.match(guides[7], /logical.*native.*capacity/is);
});

test("native first-use settings require an accepted save and preserve targeted edits", async () => {
  const [settings, setup, codex, claude] = await readMany(["references/SETTINGS.md", "references/SETUP.md", "vnext/codex/SKILL.md", "vnext/claude/SKILL.md"]);
  assert.match(settings, /next_action: settings.*revision is zero/s);
  assert.match(settings, /verbal acceptance or a read-only inspection does\s+not advance the revision/);
  assert.match(settings, /successful save advances\s+revision even when the effective preference remains inherited/);
  assert.match(settings, /Cancel, no answer, or interruption saves nothing/);
  assert.match(settings, /request to change one role asks only for its relevant choices/);
  assert.match(settings, /re-reads settings under the project mutation lock/);
  assert.match(settings, /review, tests, explicit pauses, task scope, or destination approval/);
  for (const [host, skill] of [["codex", codex], ["claude", claude]]) {
    const save = `settings ${host}.<role>.model=inherit ${host}.<role>.effort=inherit --json`;
    assert.ok(settings.includes(save));
    assert.ok(setup.includes(save));
    assert.ok(skill.includes(`settings ${host}.<role>.model=<mapped-ID-or-inherit> ${host}.<role>.effort=<mapped-effort-or-inherit> ... --json`));
    assert.match(skill, /every role the user wants inherited, actually save both fields as `inherit`/);
    assert.match(skill, /Keep on first use, persist the shown preference.*revision advances/);
    assert.match(skill, /all requested model\/effort answers/);
    assert.match(skill, /never pin an observed runtime model or effort/);
    for (const guide of [skill, setup, settings]) {
      assert.match(guide, /every role the user wants inherited/i);
      assert.match(guide, /preserve other\s+explicit choices/i);
    }
    for (const guide of [setup, settings]) assert.match(guide, /all requested roles/);
  }
});

test("active orchestration follows one ordered question continuation loop", async () => {
  const [skill, runs, actions, team] = await readMany(["SKILL.md", "references/RUNS.md", "references/ACTIONS.md", "references/TEAM.md"]);
  const loop = skill + runs;
  for (const pattern of [
    /consume.*user.*worker.*completion.*handoff.*review.*provider/is,
    /replacement.*addition.*status\/question/is,
    /status\/question.*commentary/is,
    /reconcile.*live worker.*completed handoff/is,
    /repair.*independent review.*serial.*integrat/is,
    /stopped.*handed off.*worker slot.*compute.*free/is,
    /reserve.*review.*admit.*eligible/is,
    /supervision\.heartbeatSeconds.*default.*600.*minimum.*60/is,
    /heartbeat.*known.*reconcile.*repeat/is,
  ]) assert.match(loop, pattern);
  assert.match(actions, /question.*interrupt.*not.*pause.*cancel.*terminal/is);
  assert.match(actions + team, /commentary.*reconcile.*review.*integrat.*refill.*wait/is);
  assert.match(team, /completion.*independent review.*accepted.*serial.*integrat.*refill/is);
});

test("heartbeats and blockers stay in-turn scoped and finite", async () => {
  const [skill, runs, team, recovery, codex, claude, help] = await readMany([
    "SKILL.md", "references/RUNS.md", "references/TEAM.md", "references/RECOVERY.md",
    "references/PLATFORM-CODEX.md", "references/PLATFORM-CLAUDE.md", "references/HELP.md",
  ]);
  const all = [skill, runs, team, recovery, codex, claude].join("\n");
  for (const source of [skill, runs, team, codex, claude, help]) {
    assert.match(source, /supervision\.heartbeatSeconds.*default.*600.*minimum.*60/is);
    assert.match(source, /lower.*(?:interval|value).*consume.*turn/is);
    assert.doesNotMatch(source, /wait no more than 60 seconds/i);
  }
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
  const [runs, release, status] = await readMany(["references/RUNS.md", "references/RELEASE.md", "references/STATUS.md"]);
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

test("accepted integration queues only complete top-level nondeployed deliveries", async () => {
  const [skill, hooks, readme, gettingStarted] = await readMany([
    "SKILL.md", "references/HOOKS.md", "README.md", "GETTING_STARTED.md",
  ]);
  const all = [skill, hooks, readme, gettingStarted].join("\n");
  for (const pattern of [
    /accepted integration evidence.*completed top-level.*nondeployed.*integration order/is,
    /recovered completions.*(?:already scoped|within the run scope)/is,
    /incomplete top-level.*fail(?:s)? closed/is,
    /subtasks.*epics.*(?:never|do not).*queue/is,
  ]) assert.match(all, pattern);
});

test("dependency instructions separate compatibility ownership and prerequisite edges", async () => {
  const [dependencies, leanctx, graphify] = await readMany(["references/DEPENDENCIES.md", "references/LEAN-CTX.md", "references/GRAPHIFY.md"]);
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

test("native project instructions preserve evidence tools without session authority gates", async () => {
  const [projects, state, hooks] = await readMany(["references/PROJECTS.md", "references/STATE.md", "references/HOOKS.md"]);
  const all = projects + state + hooks;
  for (const required of ["evidence-store-register", "completion-history-reconcile",
    "run-reconcile", "run-scope-extend", "completion-quarantine", "completion-rebind", "run-decision"]) assert.ok(all.includes(required), required);
  for (const pattern of [
    /native.*host.*session.*cwd/is, /request.*cannot.*(?:assert|mint).*identity/is,
    /Legacy owner records.*historical compatibility data/is,
    /initialization.*immutable.*scope.*additive.*version/is,
    /completion.*integration.*publication.*task-keyed/is,
    /generatedBy.*testedAgainst.*loadedRuntime.*sourceCandidate.*readiness/is,
    /local_only/i, /enabled_but_held.*target_required/is,
  ]) assert.match(all, pattern);
  assert.match(hooks, /project-owner-recover.*Deprecated compatibility\/maintenance/is);
  assert.match(projects, /requires no coordination transfer/i);
});

test("artifact instructions match the accepted Linux descriptor-root installer", async () => {
  const hooks = await read("references/HOOKS.md");
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
    /lowercase.*uppercase.*role/is,
    /schema.?4.*receipt.*byte.*mode.*size/is,
    /current.*drifted.*unverified_legacy.*missing_receipt.*not_installed/is,
  ]) assert.match(hooks, pattern);
});

test("native setup prepares one approved dependency bundle and preserves planning authority", async () => {
  const [setup, actions, release] = await readMany(["references/SETUP.md", "references/ACTIONS.md", "references/RELEASE.md"]);
  for (const pattern of [/Inspect before installing and offer the selected missing bundle once/i,
    /setup --install <comma-separated-names> --approve --host <host> --json/,
    /project scope and add no external hooks/i, /`tasks-md` or `beads`/,
    /--prepare-only.*does not generate governance or\s+attach a handoff/s,
    /late handoff.*without rewriting its setup receipt, settings, tracker contents,\s+or active run packets/s,
    /Kickoff remains optional/i, /status --json.*read-only.*does not prepare,\s+install, repair, or dispatch/s,
    /required\s+capabilities still gate affected work/i]) assert.match(setup, pattern);
  for (const action of ["choose_tracker", "approve_artifacts", "approve_kickoff", "initialize_beads", "approve_dependencies", "resolve_dependencies", "prepare_dependencies", "project_kickoff", "settings", "start", "inspect_tracker"]) {
    assert.ok(setup.includes("| `" + action + "` |"), action);
  }
  assert.match(setup, /--refuse-kickoff.*rejected \(CLI exit 2\) or cancelled.*without setup writes/s);
  assert.match(setup, /do not continue initialization or dispatch/);
  assert.match(actions, /Saved auto-deploy does not prompt for confirmation/i);
  assert.match(release, /authority|authorization/i);
  assert.doesNotMatch(setup, /each missing dependency.*one at a time.*numbered/is);
  assert.doesNotMatch(actions + release, /ask whether to keep auto-deploy or use no auto-deploy/i);
});

test("native host guidance requires real dispatch and observes uncertain cross-host replay", async () => {
  const [codex, claude] = await readMany(["vnext/codex/SKILL.md", "vnext/claude/SKILL.md"]);
  assert.match(codex, /collaboration\.spawn_agent/);
  assert.match(codex, /collaboration\.followup_task/);
  assert.match(claude, /actual Claude `Agent` tool/);
  assert.match(claude, /same acknowledged agent.*supported Agent resume/s);
  for (const [host, skill] of [["codex", codex], ["claude", claude]]) {
    assert.ok(skill.includes(`start --host ${host} --json`));
    assert.ok(skill.includes(`--host ${host} --identity`));
    for (const flag of ["host_dispatch_required", "already_admitted", "host_followup_required", "actual_host", "observation_required"]) assert.ok(skill.includes(flag), `${host}: ${flag}`);
    assert.match(skill, /missing\s+ack after a possible launch is uncertain/i);
    assert.match(skill, /observe the original host rather\s+than treating the missing ack as permission to spawn again/);
    assert.match(skill, /independent CLEAN review/);
    assert.match(skill, /observed\s+(?:host\s+)?idle state/);
    assert.match(skill, /beads,serena,graphify,rg,ast-grep,lean-ctx --approve/);
    assert.match(skill, /scoped installer\/Python preparation in that consent/);
    assert.match(skill, /never\s+rewrite global PATH or registry/);
    assert.match(skill, /do not resume setup or dispatch after `--refuse-kickoff` is rejected\/cancelled/);
  }
});

test("native task and lifecycle guidance separates intent from actual host results", async () => {
  for (const host of ["codex", "claude"]) {
    const skill = await read(`vnext/${host}/SKILL.md`);
    for (const action of ["observe_run", "repair_tracker", "provide_task_details", "resume", "observe_control"]) assert.ok(skill.includes("`" + action + "`"), `${host}: ${action}`);
    assert.match(skill, /start retry does not cancel a pause/);
    assert.match(skill, /deferred.*source files.*Git commit/is);
    assert.ok(skill.includes(`task add --queue --from <task.json> --host ${host} --json`));
    assert.ok(skill.includes(`task add --execute --from <task.json> --host ${host} --json`));
    assert.ok(skill.includes(`one-off <feature|audit|review> --from <task.json> --host ${host} --json`));
    assert.match(skill, /start.*,.*task add --execute.*,.*one-off.*same.*dispatch/is);
    assert.match(skill, /read_only.*no writes/is);
    assert.match(skill, /pending_handles.*actual host/is);
    assert.match(skill, /unacknowledged_teams.*reconcile.*repeat/is);
    assert.match(skill, /blocking_scopes.*remain held/is);
    assert.match(skill, /--action ack --control-id <control_id> --run <run> --team <team> --task <task>/);
    assert.match(skill, /paused.*stopped.*running/is);
    assert.match(skill, /unsupported.*control.*report.*blocker/is);
  }
});

test("continuity instructions preserve facts and native compaction as fallback", async () => {
  const [skill, state, recovery] = await readMany(["SKILL.md", "references/STATE.md", "references/RECOVERY.md"]);
  assert.match(skill, /Ordinary lint, test and review failures trigger automatic in-scope repair/);
  assert.match(state, /Required in-scope repair: assign, fix and verify automatically/);
  assert.match(state, /Out-of-scope improvement: record proposed\/deferred, not authorized implementation/);
  assert.match(recovery, /not simply the newest file in the project/);
  assert.match(recovery, /Keep native auto-compaction enabled as fallback/);
  assert.match(recovery, /cannot guarantee automatic replacement of its parent conversation/);
  assert.match(recovery, /Unknown activity does not count as stopped/);
});

test("7.3.1 README is current-first and host switching is a first-class action", async () => {
  const [readme, skill, actions, setup, projects, recovery, help, hooks, codex, claude] = await readMany([
    "README.md", "SKILL.md", "references/ACTIONS.md", "references/SETUP.md", "references/PROJECTS.md",
    "references/RECOVERY.md", "references/HELP.md", "references/HOOKS.md", "references/PLATFORM-CODEX.md",
    "references/PLATFORM-CLAUDE.md",
  ]);
  assert.match(readme, /Agent-Team 7\.3\.1/);
  assert.match(readme, /\$agent-team setup.*\/agent-team setup/is);
  assert.match(readme, /switch.*Claude Code.*Codex|switch.*Codex.*Claude Code/is);
  assert.doesNotMatch(readme, /agent-team-guide-v7\.[0-2]|Version 7\.[0-2]|Version 7\.3\.0|releases\/tag\/v7\.[0-2]|releases\/tag\/v7\.3\.0/i);

  const continuity = [skill, actions, setup, projects, recovery, help, hooks, codex, claude].join("\n");
  for (const pattern of [
    /same Git project.*continue/is,
    /no coordination transfer/i,
    /preserv.*tracker.*claims.*settings.*evidence/is,
    /legacy owner.*(?:does not|do not).*block/is,
    /model.*fallback.*separate.*session continuity/is,
  ]) assert.match(continuity, pattern);
  assert.doesNotMatch(continuity, /requires? (?:an? )?(?:ownership|coordination) transfer/i);
  assert.match(codex, /standalone.*agent.*profile.*does not prove.*native.*subagent.*model.*available/is);
  assert.match(codex, /env_key.*present.*host process/is);
  assert.match(codex, /configured.*provider.*native.*model catalog.*fresh spawn/is);
});
