# Project Kickoff + Agent-Team: first-time guide

The Node-based Agent-Team 7.3.1 instructions below are legacy during the vNext transition. The [standalone dark-green HTML guide](Getting_Started_with_Agent-Team.html) is historical; the [README](README.md) is the current release and workflow reference.

## vNext quick start

From a verified unpacked release, run `agent-teamctl install --host codex|claude|both`; choose Codex, Claude, or both explicitly. The native package supports Windows, macOS, and Linux without mutating hooks, MCP registrations, credentials, or unrelated host settings. Use `rollback --version <version>` or `uninstall` for the reversible manifest-owned lifecycle.

Run `setup`, then use `status`, `start`, `task add`, or `one-off`. Operational controls are `pause`, `stop`, `cancel`, and `resume`. FIX/CLEAN denotes internal independent review and is not a public `review` action. After approved tracker cutover, Beads is authoritative and `TASKS.md` is retained as legacy provenance; `BLOCKERS.md` and `DECISIONS.md` remain bounded projections.

Do not overwrite a v7 project or top-level skill manually. Prepare a version-1 cutover request and run `agent-teamctl cutover --request /absolute/request.json`. A project request uses `action: "cutover"`, then `status`, a cause-bound `reconcile`, or an exact `rollback`; its evidence paths and SHA-256 values must still resolve to the recorded Git revision. A host request uses `host-cutover`, `host-status`, or `host-rollback`, the exact schema-4 legacy installer receipt and digest, the current native manifest revision, and `hosts: ["codex", "claude"]` as applicable. Keep the request and returned receipt digest: rollback refuses a different receipt, revision, changed legacy file, or customized hook.

Host switching stays in the foreground and transfers no lease. The dashboard is local-only, capacity caps still apply, and optional semantic, graph, compression, browser, and visual tools fall back to native Git/Go/file operations. Review the [benchmark evidence](docs/benchmarks/vnext-optional-8.0.0.md) and revision-bound release checks before installation.

These are prompts to paste into Codex or Claude Code—not Bash commands.

## 1. Prepare the prerequisites and GitHub access

You need a signed-in host with suitable model access, Git, [GitHub CLI](https://cli.github.com/), a browser, and [Node.js 24](https://nodejs.org/) with npm. Host login and GitHub login are separate.

Paste:

```text
Check Git, GitHub CLI (gh), Node.js 24, npm and this host. Show the official installation method and scope for missing prerequisites. Preserve existing accounts and settings.

Verify GitHub CLI authentication and access to my project, https://github.com/thebpandey/project-kickoff and https://github.com/thebpandey/agent-team. Guide me through browser/device sign-in and any organization SSO step. Never ask me to paste a token, password or secret into chat. Report gh auth status without showing credentials.
```

Complete the browser sign-in yourself. Repository visibility does not grant license permission; read each package's license. Private project access may require its owner or organization administrator. See [GitHub authentication](https://cli.github.com/manual/gh_auth_login).

## 2. Install Project Kickoff and plan first (recommended)

[Project Kickoff](https://github.com/thebpandey/project-kickoff) defines a new project, audits an existing one, or replans a major revision. It produces approved planning records and an Agent-Team handoff. It does not implement product features or start Agent-Team automatically.

Its proprietary license requires the owner's permission. Install the whole package, not just SKILL.md:

```text
Install the complete Project Kickoff package from https://github.com/thebpandey/project-kickoff/releases/latest, under its applicable license, in this confirmed project root. Use .agents/skills/project-kickoff for Codex or .claude/skills/project-kickoff for Claude Code. Do not assume an unpublished future version is available.

Preserve customized installations and unrelated files. Keep references, assets, scripts and metadata together. Exclude the proprietary package from application commits through this repository's local Git exclude and verify that exclusion. Do not change global settings or activate hooks. Verify discovery and tell me whether to reload.
```

After reloading if necessary:

```text
Codex:  $project-kickoff start <project idea>
Claude: /project-kickoff start <project idea>
```

Answer one unresolved question at a time and approve each planning stage. Kickoff's own setup asks before dependency installation. Its optional project hooks need separate approval and Python 3.9+ on supported Linux/macOS/WSL hosts; Python is not required merely to read the skill.

Use `audit <path>` for an existing project, `audit-only <path>` for a report without follow-on setup, and `resume <path>` to continue. Prefix each with the host's Project Kickoff invocation.

Already have an approved plan? Skip Kickoff. Agent-Team can adopt it or work from a clearly defined standalone task without installing Kickoff.

## 3. Install the complete Agent-Team package

Choose one host and user scope (available across projects) or project scope (only this repository). One complete package serves both hosts; selecting both must be explicit.

```text
Install the complete Agent-Team package from https://github.com/thebpandey/agent-team using the managed installer and my authorized GitHub access. Inspect its source, exact revision and any existing installation first. Do not overwrite customized or unowned resources.

Use this actual host only: codex or claude-code. Use user scope unless I request project scope at this confirmed Git root. Keep the complete package and register only the selected host's hooks and roles. Preserve unrelated configuration. Verify the installed package and report the receipt, reload/trust steps, and any checks that could not run.
```

| Scope | Codex skill | Claude Code skill |
| --- | --- | --- |
| User | `~/.agents/skills/agent-team` | `~/.claude/skills/agent-team` |
| Project | `.agents/skills/agent-team` | `.claude/skills/agent-team` |

Project hook configuration is `.codex/hooks.json` or `.claude/settings.local.json`. Node.js 24 must be ready before running package helpers and hooks.

Download `agent-team-7.3.1.zip` and its matching one-entry `SHA256SUMS` from [v7.3.1](https://github.com/thebpandey/agent-team/releases/tag/v7.3.1), with update discovery at [releases/latest](https://github.com/thebpandey/agent-team/releases/latest). Verify and install the sealed bytes:

```sh
(cd /absolute/download && sha256sum -c SHA256SUMS)
node /absolute/extracted/agent-team/hooks/agent-team-cli.mjs install \
  --archive /absolute/download/agent-team-7.3.1.zip \
  --checksums /absolute/download/SHA256SUMS \
  --host codex \
  --scope user
```

Use `--host claude-code` for Claude Code or explicit `--host both` for both hosts. Project scope requires `--scope project --project /absolute/project/root`. `install --source` is rejected; source validation or a local build is not official installation evidence.

Automatic artifact installation is qualified only on Linux and WSL where `/proc/self/fd` directory traversal functions. Missing or nonfunctional descriptor-root support returns `unsupported_platform`, `changed: false` before the lock or any mutation. Exact byte/mode/size-identical reinstall returns `installed`, `changed: false` with no target mutation. Every differing present target, owned and schema-3 targets included, returns `update_requires_manual_replacement`, `changed: false`, with zero target, configuration, role, backup, or receipt mutation. A changed release requires separately authorized quiescence and an explicit rollback-backed move of the old target, followed by a normal fresh absent-target install and restoration on failure.

This includes the 7.2.6 lowercase-to-uppercase Markdown migration. Verify every installed package and lowercase Claude role against its schema-4 receipt before a separately authorized replacement, retain rollback backups, and never leave lowercase and uppercase files for the same native role ID active together. Customized or ambiguous files remain in place as conflicts. The normal installer will not infer permission to replace them from the release version.

The embedded `.agent-team-source.json` records the ten-field package source identity and package map. Verified artifact authority and schema-4 receipts separately bind archive/checksum identity, complete archive and installed maps, selected hosts/scope, transaction, recovery, targets, and time. They do not prove publication, host reload/trust, dependency readiness, or live session state.

## 4. Reload and review hook trust

Close/reopen the host or use its supported reload action. Type `/hooks` yourself, inspect the registered paths and commands, and approve the intended entries through native controls. Use “Trust all” only if you have reviewed every affected entry.

Telling the agent “trust all hooks” cannot bypass native consent or organization policy. Reload again if requested. Installation, registration, trust and observed execution are different facts.

Confirm discovery without starting work:

```text
Codex:  $agent-team help
Claude: /agent-team help
```

## 5. Run setup once

```text
Codex:  $agent-team setup
Claude: /agent-team setup
```

Setup reuses the Kickoff handoff, an existing plan, or a standalone task. It preserves the canonical branch and chosen tracker. A missing helper never silently migrates Beads to Markdown.

| Preparation order | Component | Responsibility |
| --- | --- | --- |
| 1 | Host, Git, gh, Node.js 24/npm, account/project access | Agent can install in approved scope; you complete login, access, admin and trust steps. |
| 2 | Complete Agent-Team package and native reload/trust | Managed selected-host installer; native trust remains yours. |
| 3 | [uv](https://docs.astral.sh/uv/getting-started/installation/), managed Python, [Serena](https://github.com/oraios/serena), selected language-server prerequisites | Selected default; prepare when useful or explicitly required. Host registration/reload is reported separately. |
| 4 | [Microsoft Playwright CLI](https://github.com/microsoft/playwright-cli), browser binaries and required OS libraries | Selected default; required only for plans that explicitly need browser interaction. System libraries may need administrator access. |
| 5 | Selected default skills and CLIs below | Prepared automatically on first run; compatible existing copies reused. |
| 6 | [Beads](https://github.com/gastownhall/beads), if selected | Verify its actual backend; do not assume an external database server is always required. |
| 7 | Optional additions | Install only after selection. |

Default-selected profiles: [ast-grep CLI](https://github.com/ast-grep/ast-grep), code-only [Graphify](https://github.com/Graphify-Labs/graphify) for offline repository structure, blast radius and cross-module paths, narrowed [LeanCTX](https://github.com/yvgude/lean-ctx), selective [Superpowers](https://github.com/obra/superpowers), local [Ponytail](https://github.com/DietrichGebert/ponytail), [Impeccable](https://github.com/pbakaus/impeccable) skill/detector, and individual [React Best Practices](https://github.com/vercel-labs/agent-skills/tree/main/skills/react-best-practices).

Setup shows grouped progress, not a separate approval question for each already-selected tool. A companion failure is diagnostic and does not block unrelated work. Only capabilities explicitly named by the active plan remain pending until both functional and worker checks pass. Optional [Context7](https://github.com/upstash/context7) supplies library docs; the dashboard is also opt-in.

The current executable adapters cannot automatically list every visible MCP/plugin capability. If you ask to reduce context, Agent-Team may show an `offered_unverified` proposal based on your reviewed report. Visibility remains unknown, required selected dependencies stay excluded, and applying requires your explicit acknowledgement from a native session in the same project. Cancel or no answer changes nothing. Claude Code derives its configuration home from the running adapter, not from request text.

Preparation is not universal instruction loading. Each role reads only the complete instructions needed for its assignment. No second tracker, proxy, blanket plugin hook set or paid JetBrains dependency is introduced by these profiles.

## 6. Inspect and change settings

Creating the canonical project records is separate from dependency observations and native-host trust. Keep the final setup summary: it identifies the active native session, selected tracker, task-required checks and any reload/trust step. The agent should never call a saved installation preference proof of fresh-worker access.

Ask:

```text
Show Agent-Team's role/model/effort overview and effective setting sources. Help me change only the developer role for this host, using supported choices with short explanations. Preserve the other host's preferences and all unrelated settings. Let me cancel without saving; distinguish saved values from actual host enforcement.
```

| Setting | Meaning | Default |
| --- | --- | --- |
| Parallel teams | Requested concurrent teams, limited by real host and reviewer capacity | 1 |
| Continuous | Refill after verified integration or safe parking while eligible authorized work remains | Off |
| Auto-deploy | Verified batches to an authorized destination; never a waiver of release checks | Off |
| Deployment batch | Completed top-level delivery tasks per release | Effective team limit unless explicitly set |
| Model/effort | Per-host role choices, friendly menus, availability/enforcement shown | Quality-first supported defaults |
| Usage budget | Soft strategy advice; explicit hard limit requests safe checkpointing | No hard limit by default |

Execution limits are separate: subprocess output is bounded at 2097152 bytes, canonical records at 16777216 bytes, plans at 1000 tasks, and lane worker updates at 2000 characters. Raising the canonical-record allowance does not raise the other limits. Parent-model comparison is unknown unless the host provides trustworthy comparable metadata.

Use the full wizard only if you want to review everything. Settings apply to future starts, not silently to active agents.

Accepted integration evidence adds completed top-level, nondeployed deliveries to the deployment queue in integration order, including recovered completions already within the run scope. An incomplete top-level integration is rejected; subtasks and epics do not count as queued deliveries.

## 7. Start development

```text
Use Agent-Team from https://github.com/thebpandey/agent-team to implement the approved plan. Preserve its decisions and selected tracker. Start with one team, continuous mode off and auto-deploy off. Use bounded sub-agent assignments, repair ordinary lint/test/review findings automatically, and verify requirements before completion. Do not deploy.
```

For sustained execution, ask for “up to 2 teams in continuous mode.” The orchestrator assigns independent work, reserves review capacity, serializes integration and safely cleans eligible worktrees even with deployment off.

Credential or external blockers can be safely parked while independent tasks continue. Claims and evidence remain; unknown writers do not free capacity. Explicit pauses require your resume.

## 8. Optional dashboard and safe recovery

Ask Agent-Team to enable a local saved HTML dashboard and report its path. It shows teams, overall progress and all task statuses. File reload reads the latest saved snapshot. A separately enabled loopback Node helper provides on-open/button refresh; no build framework or public hosting is required.

Optional Beads graph provider: [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer), by Jeffrey Emanuel, under its [complete license including the OpenAI/Anthropic rider](https://github.com/Dicklesworthstone/beads_viewer/blob/main/LICENSE). It is referenced externally, not white-labeled or vendored as unrestricted MIT. Attribution is not blanket license eligibility. TASKS-only views work without it.

Ask “Resume Agent-Team” after interruption, or open the same checkout in the other host and ask it to continue. No ownership release or takeover file is needed. Source-linked checkpoints preserve approved/rejected decisions, writer assignments, verification and pending operations. Native compaction stays enabled as fallback; the skill cannot guarantee zero compactions or autonomous parent replacement after host exit.

If something fails, request the specific failed prerequisite and its supported recovery. Never bypass policy, silently switch trackers, treat absent metrics as zero cost, or call an unrun test passed.
