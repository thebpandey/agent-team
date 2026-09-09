# Project Kickoff + Agent-Team: first-time guide

Local implementation preview: final native-host qualification and publication are pending. The [standalone dark-green HTML guide](Getting_Started_with_Agent-Team.html) contains the complete prompts, collapsible instructions and reference library.

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
Install the complete Project Kickoff package from https://github.com/thebpandey/project-kickoff at tag v0.3.0, under its applicable license, in this confirmed project root. Use .agents/skills/project-kickoff for Codex or .claude/skills/project-kickoff for Claude Code.

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
| 3 | [uv](https://docs.astral.sh/uv/getting-started/installation/), managed Python, [Serena](https://github.com/oraios/serena), selected language-server prerequisites | Mandatory; supported missing prerequisites prepared in order and functionally checked. |
| 4 | [Microsoft Playwright CLI](https://github.com/microsoft/playwright-cli), browser binaries and required OS libraries | Mandatory even for backend projects; system libraries may need administrator access. |
| 5 | Selected default skills and CLIs below | Prepared automatically on first run; compatible existing copies reused. |
| 6 | [Beads](https://github.com/gastownhall/beads), if selected | Verify its actual backend; do not assume an external database server is always required. |
| 7 | Optional additions | Install only after selection. |

Default-selected profiles: [ast-grep CLI](https://github.com/ast-grep/ast-grep), narrowed [LeanCTX](https://github.com/yvgude/lean-ctx), selective [Superpowers](https://github.com/obra/superpowers), local [Ponytail](https://github.com/DietrichGebert/ponytail), [Impeccable](https://github.com/pbakaus/impeccable) skill/detector, and individual [React Best Practices](https://github.com/vercel-labs/agent-skills/tree/main/skills/react-best-practices).

Setup shows grouped progress, not a separate approval question for each already-selected tool. Required missing capabilities remain pending until useful functional checks pass. Optional [Context7](https://github.com/upstash/context7) supplies library docs; the dashboard is also opt-in.

Preparation is not universal instruction loading. Each role reads only the complete instructions needed for its assignment. No second tracker, proxy, blanket plugin hook set or paid JetBrains dependency is introduced by these profiles.

## 6. Inspect and change settings

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

Use the full wizard only if you want to review everything. Settings apply to future starts, not silently to active agents.

## 7. Start development

```text
Use Agent-Team from https://github.com/thebpandey/agent-team to implement the approved plan. Preserve its decisions and selected tracker. Start with one team, continuous mode off and auto-deploy off. Use bounded sub-agent assignments, repair ordinary lint/test/review findings automatically, and verify requirements before completion. Do not deploy.
```

For sustained execution, ask for “up to 2 teams in continuous mode.” The orchestrator assigns independent work, reserves review capacity, serializes integration and safely cleans eligible worktrees even with deployment off.

Credential or external blockers can be safely parked while independent tasks continue. Claims and evidence remain; unknown writers do not free capacity. Explicit pauses require your resume.

## 8. Optional dashboard and safe recovery

Ask Agent-Team to enable a local saved HTML dashboard and report its path. It shows teams, overall progress and all task statuses. File reload reads the latest saved snapshot. A separately enabled loopback Node helper provides on-open/button refresh; no build framework or public hosting is required.

Optional Beads graph provider: [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer), by Jeffrey Emanuel, under its [complete license including the OpenAI/Anthropic rider](https://github.com/Dicklesworthstone/beads_viewer/blob/main/LICENSE). It is referenced externally, not white-labeled or vendored as unrestricted MIT. Attribution is not blanket license eligibility. TASKS-only views work without it.

Ask “Resume Agent-Team” after interruption. Source-linked checkpoints preserve approved/rejected decisions, ownership, verification and pending operations. Native compaction stays enabled as fallback; the skill cannot guarantee zero compactions or autonomous parent replacement after host exit.

If something fails, request the specific failed prerequisite and its supported recovery. Never bypass policy, silently switch trackers, treat absent metrics as zero cost, or call an unrun test passed.
