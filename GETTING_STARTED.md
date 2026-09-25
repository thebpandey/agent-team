# Project Kickoff + Agent-Team: first-time guide

The Node-based Agent-Team 7.3.1 instructions below are legacy during the vNext transition. The [standalone dark-green HTML guide](Getting_Started_with_Agent-Team.html) is historical; the [README](README.md) is the current release and workflow reference.

## Native quick start

Agent-Team 8.0.15 is a release candidate repairing Project Kickoff task admission and large setup responses. After publication, use the complete [8.0.15 package](https://github.com/thebpandey/agent-team/releases/tag/v8.0.15). The [release readiness record](docs/releases/8.0.15-readiness.md) distinguishes fixture acceptance from publication and installation.

Open your Git project and invoke `$agent-team setup` in Codex or `/agent-team setup` in Claude Code. You can also ask to `start`; the skill completes missing setup before dispatch. Existing tracker choices, task identities, governance, settings, and Project Kickoff facts are reused. Native v8 requires no external hooks.

Setup offers numbered model and effort choices for each role from the active host's available options, with Keep and Inherit choices; no typed model IDs are required.

The skill resolves `agent-teamctl` with `command -v` (`Get-Command` in PowerShell), then uses the owned binary path in `install-manifest.json`. The Linux default is `~/.config/agent-team/bin/agent-teamctl`; when set, `XDG_DATA_HOME` supplies the base instead, otherwise `XDG_CONFIG_HOME` replaces `~/.config`. macOS uses `~/Library/Application Support/agent-team`, and Windows uses `%LOCALAPPDATA%\AgentTeam` with `bin\agent-teamctl.exe`. The manifest is in that same data directory. Use the absolute binary path if it is absent from PATH; no shell edits are required.

### If LeanCTX blocks the controller

Native Agent-Team does not require LeanCTX hooks. If your separately installed
LeanCTX shell guard rejects `agent-teamctl`, use its supported
`lean-ctx allow agent-teamctl` command to extend `shell_allowlist_extra`.
Keep the guard enabled and preserve unrelated settings.

To preserve the controller's complete JSON output, add `agent-teamctl` and its
resolved absolute installed path to LeanCTX's `excluded_commands`, retaining
existing entries. Its matcher uses a literal command prefix: invoke the
controller as a separate simple command with the tool's working directory set
to the project. A compound `cd ... && ...` or quoted executable prefix does not
match that exclusion. For unusual compound commands, `lean-ctx raw` provides
supported output passthrough while retaining security checks.

### Inspect, approve missing pieces, and resume

Run `agent-teamctl setup --host codex --json`, replacing `codex` with `claude` for Claude Code. A `needs_input` response names `next_action`:

| Next action | What happens next |
| --- | --- |
| `choose_tracker` | Select `tasks-md` or `beads` when existing project authority does not resolve the choice. |
| `approve_artifacts` | Review missing governance/tracker files and approve their creation; existing files are preserved. |
| `approve_kickoff` | Review the discovered nested Project Kickoff 0.5.2 handoff (0.5.0/0.5.1 also supported) and approve using its existing facts. |
| `initialize_beads` | Approve initialization for the selected Beads tracker; retain existing data. |
| `settings` | Review saved role preferences and resolve only missing or requested choices. |

After the relevant consent, continue with the selected host and tracker:

```text
agent-teamctl setup --tracker tasks-md --approve --host codex --json
agent-teamctl setup --approve-kickoff --approve --host codex --json
```

The second command consumes a discovered handoff; add `--kickoff <path>` for an explicit path. The skill does not repeat the Kickoff interview. Project Kickoff is optional: approved setup without a handoff can create a minimal scaffold. `TASKS.md` can remain the live tracker; Beads is an alternative, not a mandatory conversion. `BLOCKERS.md` and `DECISIONS.md` remain human-readable projections.

The skill offers one dependency choice for selected missing tools. After approval, it executes the chosen names, for example:

```text
agent-teamctl setup --install beads,serena,graphify --approve --host codex --json
```

Include Beads when it is the selected tracker. Installs are project scoped, add no external hooks, and resume setup afterward. Prior approval carries forward. An optional helper failure reports its effect and available native fallback; it does not silently change the selected tracker.

### Choose role preferences

Inspect `agent-teamctl settings --json`. Both hosts support `orchestrator`, `developer` (alias `coder`), `reviewer`, and `visual_reviewer`. Each role accepts `model` and `effort`, including `inherit`:

```text
agent-teamctl settings codex.developer.model=inherit --json
agent-teamctl settings claude.reviewer.effort=inherit --json
```

Only the named preference changes. It applies to future dispatch, does not switch the current parent model, and does not prove host enforcement. The skill passes supported profile fields to the real host tools and reports unsupported choices. See [Settings](references/SETTINGS.md).

### Start and inspect real work

Use `$agent-team start` or `/agent-team start`. The skill runs `agent-teamctl start --host codex|claude --json`; the explicit host selects its saved profile and the project retains its tracker. The controller reserves a packet. The skill must then call Codex's collaboration tool or Claude's Agent tool and acknowledge the exact returned handle before reporting a launch. A standalone terminal reservation is not a launched team.

FIX/CLEAN is an internal review loop. Retained queues require actual completion, an independent CLEAN review, and observed idle state before a fresh follow-up packet can be delivered to the same handle. Claude uses its supported Agent resume facility; if unavailable, the skill reports that limit. Already admitted retries are observed without duplicate dispatch or acknowledgement. Foreground host switching does not transfer worker ownership; foreign live handles remain occupied. `agent-teamctl status --json` reads actual persisted state without setup, installation, or dispatch.

External Ed25519 trust is not needed for normal v8 setup or for a migration backed by a valid schema-4 receipt. It is only the one-time fallback for stale or unverifiable v7 cutover without a current canonical approval. One machine trust key may sign separate short-lived, project-bound approvals.

For that fallback, an authorized Linux operator can provision a new non-overwriting machine trust anchor with `sudo bash scripts/provision-cutover-trust.sh`. The script prints only the public key ID and paths; it never signs an approval or runs cutover.

Linux amd64 users download `agent-teamctl-8.0.15.zip`, `RELEASE.json`, `SBOM.cdx.json`, and `SHA256SUMS` into one empty folder, run `sha256sum -c SHA256SUMS`, extract the archive there, then run `./agent-teamctl install --host both --json`. Windows amd64 users download the one `agent-teamctl-8.0.15-windows-amd64.zip` bundle and its `.sha256` sidecar, verify the sidecar with `Get-FileHash -Algorithm SHA256`, extract the bundle once, then run `.\agent-teamctl.exe install --host both --json` in PowerShell. Use `codex` or `claude` instead of `both` for one host. macOS has source verification but no published native bundle. By default Codex uses `~/.agents` and Claude uses `~/.claude` on Windows, macOS, and Linux. For a custom location, set `CODEX_HOME` or `CLAUDE_HOME` on that host's first install only; later lifecycle commands reuse the manifest-recorded home and reject a conflicting override. The installer does not mutate shell configuration, hooks, MCP registrations, credentials, or unrelated host settings. Use `rollback --version <version>` or `uninstall` for the reversible manifest-owned lifecycle.

For an explicit task series after native admission, use `agent-teamctl start --run <returned-run> --task <task-id> --task <another-task-id> --json`. The existing team queue holds at most eight tasks. Each selected task must be ready in that run's unchanged tracker snapshot; retained workers are not rebound across runs or silently refreshed against changed tracker authority. The installed host skill handles acknowledgement, independent review, idle observation, and sequential follow-up. A final `next` consumes the last reviewed task and leaves the retained team idle; adding another eligible task can then request a fresh follow-up to the same handle.

Use the complete checksum-verified v8.0.15 distribution. For an existing native installation, run the **new distribution controller** with `update --version 8.0.15 --json` instead of `install`. The old installed controller cannot parse the new release metadata. Use an absolute path, for example `/absolute/verified-8.0.15/agent-teamctl update --version 8.0.15 --json`; afterward the skill resolves the installed binary through its manifest.

In PowerShell, verify the Windows download before extraction:

```powershell
$bundle = "agent-teamctl-8.0.15-windows-amd64.zip"
$expected = (Get-Content -Raw "$bundle.sha256").Trim()
$actual = "{0}  {1}" -f (Get-FileHash -Algorithm SHA256 $bundle).Hash.ToLowerInvariant(), $bundle
if ($actual -cne $expected) { throw "Windows bundle checksum mismatch" }
$distribution = "agent-teamctl-8.0.15-windows-amd64"
Expand-Archive -LiteralPath $bundle -DestinationPath $distribution
& (Join-Path $distribution "agent-teamctl.exe") install --host both --json
```

If native install or update reports a conflicting Codex Agent-Team skill, it has not changed the target installation. Move the reported whole root to a recoverable backup outside `~/.codex/skills` and `~/.agents/skills`, then retry. Do not merge files from an unknown root into the native installation.

Run `setup`, then use `status`, `start`, `task add`, or `one-off`. Operational controls are `pause`, `stop`, `cancel`, and `resume`. FIX/CLEAN denotes internal independent review and is not a public `review` action. After approved tracker cutover, Beads is authoritative and `TASKS.md` is retained as legacy provenance; `BLOCKERS.md` and `DECISIONS.md` remain bounded projections.

Do not overwrite a v7 project or top-level skill manually. The current v7 state/receipt chain cannot immutably authorize the different Beads scope. An operator must provision the fixed trust store out of band at `/etc/agent-team/cutover-trust.json` on Unix or `C:\ProgramData\Agent-Team\cutover-trust.json` on Windows as a schema-1 document: `{"schema":1,"keys":[{"algorithm":"ed25519","keyId":"<sha256-of-raw-key>","publicKey":"<base64-raw-key>"}]}`; multiple keys are sorted uniquely by key ID. With OpenSSL 3, `openssl genpkey -algorithm Ed25519 -out operator-private.pem` creates the operator-held key, and `openssl pkey -in operator-private.pem -pubout -outform DER | tail -c 32 > operator-public.raw` extracts the 32 raw public-key bytes used by `sha256sum operator-public.raw` and `base64 -w0 operator-public.raw`. Unix requires root ownership with no group/world writes. Windows requires a regular non-reparse file owned by Local System or Administrators whose DACL grants writes only to those identities; unknown ACLs fail closed. This package has no API that writes the trust store or private key.

Create a schema-1 request with `action: "prepare"`, the absolute project, operation ID, tracker export and parent ID, review/test/readiness IDs and paths, and `prepare` containing the approval ID, trusted signer key ID, RFC3339 issue/expiry (at most 24 hours), cause, recovery disposition, remote name/base/target refs, `remoteMainDeploys`, absolute payload/request/signature output paths, native manifest, and both absolute `hostRoots`. Relative evidence paths are confined to the canonical project root and cannot traverse or use symlinked parents. An explicitly absolute external evidence file remains supported, is read through the same stable bounded no-follow path, and its absolute path and digest are signed. Run `agent-teamctl cutover --request /absolute/prepare.json`. It only creates the two new mode-0600 output files with exclusive creation and prints the canonical payload SHA-256; it does not sign, grant authority, or modify project/host state. The payload includes independently observed remote facts, the exact closed tracker scope and evidence hashes, and exact source metadata, top-level skill, config-handler, and staged-native inventory for both hosts.

Sign the payload bytes without editing them. With an Ed25519 private key held by the operator, OpenSSL 3 uses `openssl pkeyutl -sign -rawin -inkey operator-private.pem -in /absolute/unsigned-approval.json -out /absolute/unsigned-approval.sig`. The generated cutover request already references that raw signature file and key ID; base64-encoded signature files are also accepted. Run it unchanged with `agent-teamctl cutover --request /absolute/cutover.json`. A project request then uses `status`, an approval- and receipt-bound `reconcile`, or an exact `rollback`. A host `host-cutover` request may use the exact schema-4 legacy receipt, or the explicit canonical `project` plus `authorityReceiptSha256` for that project's fixed `.agent-team/v8/authority.json`; the latter is required for actual v7.3.1 hosts that have only `.agent-team-source.json`. No caller-supplied receipt path or inventory is accepted. Signed-inventory activation revalidates the receipt signature, project binding, every host byte, and only replaces top-level `SKILL.md`, removes its staged nested duplicate, and retires the signed handler spans. `host-status` and `host-rollback` retain the durable exact rollback path. Unknown, changed, symlinked, foreign, or cross-project files fail closed.

Host switching stays in the foreground and transfers no lease. The dashboard is local-only, capacity caps still apply, and optional semantic, graph, compression, browser, and visual tools fall back to native Git/Go/file operations. Review the [v8 benchmark evidence](https://github.com/thebpandey/agent-team/blob/main/docs/benchmarks/vnext-optional-8.0.0.md) and [v8.0.15 revision-bound release checks](docs/releases/8.0.15-readiness.md) before installation.

These are prompts to paste into Codex or Claude Code—not Bash commands.

## Legacy v7 step 1: Prepare the prerequisites and GitHub access

You need a signed-in host with suitable model access, Git, [GitHub CLI](https://cli.github.com/), a browser, and [Node.js 24](https://nodejs.org/) with npm. Host login and GitHub login are separate.

Paste:

```text
Check Git, GitHub CLI (gh), Node.js 24, npm and this host. Show the official installation method and scope for missing prerequisites. Preserve existing accounts and settings.

Verify GitHub CLI authentication and access to my project, https://github.com/thebpandey/project-kickoff and https://github.com/thebpandey/agent-team. Guide me through browser/device sign-in and any organization SSO step. Never ask me to paste a token, password or secret into chat. Report gh auth status without showing credentials.
```

Complete the browser sign-in yourself. Repository visibility does not grant license permission; read each package's license. Private project access may require its owner or organization administrator. See [GitHub authentication](https://cli.github.com/manual/gh_auth_login).

## Legacy v7 step 2: Install Project Kickoff and plan first

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

## Legacy v7 step 3: Install the complete Agent-Team package

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

For the historical Node package only, download `agent-team-7.3.1.zip` and its matching one-entry `SHA256SUMS` from [v7.3.1](https://github.com/thebpandey/agent-team/releases/tag/v7.3.1), with update discovery at [releases/latest](https://github.com/thebpandey/agent-team/releases/latest). Verify and install the sealed bytes:

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

## Legacy v7 step 4: Reload and review hook trust

Close/reopen the host or use its supported reload action. Type `/hooks` yourself, inspect the registered paths and commands, and approve the intended entries through native controls. Use “Trust all” only if you have reviewed every affected entry.

Telling the agent “trust all hooks” cannot bypass native consent or organization policy. Reload again if requested. Installation, registration, trust and observed execution are different facts.

Confirm discovery without starting work:

```text
Codex:  $agent-team help
Claude: /agent-team help
```

## Legacy v7 step 5: Run setup once

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

## Legacy v7 step 6: Inspect and change settings

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

## Legacy v7 step 7: Start development

```text
Use Agent-Team from https://github.com/thebpandey/agent-team to implement the approved plan. Preserve its decisions and selected tracker. Start with one team, continuous mode off and auto-deploy off. Use bounded sub-agent assignments, repair ordinary lint/test/review findings automatically, and verify requirements before completion. Do not deploy.
```

For sustained execution, ask for “up to 2 teams in continuous mode.” The orchestrator assigns independent work, reserves review capacity, serializes integration and safely cleans eligible worktrees even with deployment off.

Credential or external blockers can be safely parked while independent tasks continue. Claims and evidence remain; unknown writers do not free capacity. Explicit pauses require your resume.

## Legacy v7 step 8: Optional dashboard and safe recovery

Ask Agent-Team to enable a local saved HTML dashboard and report its path. It shows teams, overall progress and all task statuses. File reload reads the latest saved snapshot. A separately enabled loopback Node helper provides on-open/button refresh; no build framework or public hosting is required.

Optional Beads graph provider: [beads_viewer](https://github.com/Dicklesworthstone/beads_viewer), by Jeffrey Emanuel, under its [complete license including the OpenAI/Anthropic rider](https://github.com/Dicklesworthstone/beads_viewer/blob/main/LICENSE). It is referenced externally, not white-labeled or vendored as unrestricted MIT. Attribution is not blanket license eligibility. TASKS-only views work without it.

Ask “Resume Agent-Team” after interruption, or open the same checkout in the other host and ask it to continue. No ownership release or takeover file is needed. Source-linked checkpoints preserve approved/rejected decisions, writer assignments, verification and pending operations. Native compaction stays enabled as fallback; the skill cannot guarantee zero compactions or autonomous parent replacement after host exit.

If something fails, request the specific failed prerequisite and its supported recovery. Never bypass policy, silently switch trackers, treat absent metrics as zero cost, or call an unrun test passed.
