# Pro feature preview and integration approval

`start <name> with-preview` requires a user-reviewed preview before integration. This gate survives pauses, crashes, handoffs, new sessions, missing tools, and successful automated checks. The absence of the flag on a later invocation does not remove it. Only an explicit user change to that requirement can waive it; record the change and its scope.

On counted and continuous starts, apply `with-preview` separately to every admitted top-level task, including later replacements. Each team needs its own submitted version and explicit approval. Waiting for approval retains a development slot; another team's approval or an auto-deploy batch does not cover it. A slot is freed only after that task is verified and integrated into main, as [runs](runs.md) specifies.

## Record the gate before starting work

Store the gate in the team's parent task or equivalent canonical tracker record: required, Awaiting preview / Awaiting approval / Approved / Changes requested; review version; preview environment identity; approval author/message reference and time; approved version. Use the installed tracker's supported notes/fields instead of assuming a custom schema. CONTEXT.md and the team directory point to this record.

The review version identifies the actual served source, dependencies, and relevant configuration. Prefer an exact commit with clean relevant files. If project rules prevent a commit, record an unambiguous source snapshot/digest and dirty-file scope; do not claim commit-level approval. Obtain normal commit authority before integration. Do not include secret configuration values in identity records.

## Start a local preview

1. Build and test in the feature worktree. Inspect the project's scripts, package manager, lockfile, environment needs, and established preview command. Use `npm run dev` only when the project defines it; use another existing supported command when appropriate. A dev server is a development preview, not production deployment.
2. Use approved development data and services. Worktrees alone do not isolate a database. Do not connect a preview to production writes by default, copy secrets into the tracker, or make real purchases/messages as a preview check.
3. Reserve an available team-specific port through existing host controls. Record the actual process/session ID, start identity, command, worktree, selected port, URL, log path, and environment in the team resource record. Check readiness and the relevant page. A started process alone does not prove a usable preview. Detect any port fallback and report the actual URL.
4. Use a foreground/background mechanism supported by the host. Do not install a daemon or promise persistence after the host closes. Use loopback by default. For another device, use an already approved private address/tunnel; explain that localhost refers to the server machine. Do not expose a public endpoint or change firewall rules without applicable authority.
5. Submit the inspected version, URL, feature summary, check evidence, and useful review steps. Mark Awaiting approval. A missing server or inaccessible preview remains blocked; it does not bypass the gate.

Freeze the submitted feature source while it awaits review so hot reload cannot silently change what the user approves. Use an isolated served snapshot when continued implementation is necessary. If code, dependencies, or relevant configuration change, create a new review version and tell the user. Do not reuse the old approval for a materially different result.

## Approve or request changes

Resolve `approve <name-or-ID>` to exactly one team in the current project. Only the user's explicit approval counts; an agent report, passing test, screenshot, or elapsed time does not. If no review version was submitted, or the served version changed or cannot be identified, prepare the correct preview and request approval of that version. Record approval against the submitted version with its source. An approval request with an unambiguous version may be recorded if that reviewed version is still intact, even if its server has since stopped; do not claim it is currently live.

Approval permits that feature version to enter integration. It does not directly deploy it, grant a new production destination, waive project checks, or approve other teams. Notify the project owner or leave an acknowledged handoff; do not start a competing integration owner. Keep unrelated teams working.

Feedback makes the gate Changes requested. Repair in the feature worktree, rerun affected checks, submit a new review version, and wait for renewed approval. Material changes during integration also require a new review version and approval before main is updated. Preserve valid approval across a restart when the approved source/environment identity is unchanged. Never reset a required gate while reconstructing missing state; missing approval evidence means approval is unknown.

## Preview lifecycle

Status reads recorded preview information without starting or probing it. Resume inspects actual process ownership and served version before reusing or restarting it. Never kill a process based only on a port or stale PID. Preserve the preview while review is pending where the host permits; no automatic timeout may imply approval.

After the feature is verified in production, the project owner must stop its task-owned preview/processes and remove eligible worktrees and temporary files through [release cleanup](release.md). Keep no idle per-feature preview or disposable checkout merely because development finished. If the user requests longer retention, record the reason and expiry/review point; do not promise cleanup after the host stops without a supported scheduler.
