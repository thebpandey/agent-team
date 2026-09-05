# Pro project and named-team coordination

## Identity and shared ownership

Use one canonical project record location in the main checkout. Discover it from the actual Git worktree/common-directory metadata and saved setup receipt; do not assume the current directory is main. Resolve the same canonical tracker across all worktrees, including any worktree-specific Beads configuration. Never initialize a second tracker because the current worktree lacks its files.

Maintain `.agent-team/TEAMS.md` at that location as a directory of identities and resource ownership, not another task ledger. Record a stable project ID, canonical paths, one project orchestrator owner/session, the registry revision, and an integration/release owner. Register each team with a stable ID such as `TEAM-001`, a unique readable name, its feature or parent-task ID, assigned task IDs, actual host/session identity, branch/base revision, worktree and CONTEXT.md paths, and owned preview/process resources. IDs are unique within a project; use the project ID to distinguish repositories. Keep a team ID through session replacement, pause, resume, and name changes. Session IDs identify attempts, not the feature itself.

The project orchestrator is the only writer of TEAMS.md, the canonical local TASKS.md, the main CONTEXT.md, and shared MISTAKES.md. Team leads send updates with team ID, task IDs, revision, evidence, and requested change. Use the host's supported message channel; when sessions cannot message, use uniquely named handoff files under the canonical `.agent-team/handoffs/<team-id>/` directory. These are pending reports, not task authorities. The project owner consumes each report once and records acknowledgement. Never have team leads overwrite shared files independently.

Serialize registration, ownership transfers, and integration. Use the host's exclusive ownership mechanism or a lock obtained through atomic filesystem creation with owner/session identity for shared mutations; atomic file replacement alone does not prevent two writers. If exclusive ownership cannot be established, queue the shared mutation and continue only independent work. Do not elect a second project owner merely because the first is not responding. Transfer ownership only after verifying the previous writer has stopped or obtaining a coordinated handoff. An old timestamp alone is insufficient. The first authorized project initializer records itself as project owner under that same exclusive operation. Register and allocate a team ID under this ownership rule before launching writers. Keep lock operations short and release them after the protected mutation; preserve operation ownership while a merge or release is in flight. Reclaim an orphaned lock only after confirming its owner stopped and inspecting the pending operation.

Beads can accept assigned task updates directly only when its installed backend and ownership controls support concurrent use. Discover version-specific commands instead of inventing fields or claim operations. Keep global task assignment, dependency changes, completion reconciliation, and integration under project control. Otherwise send updates through the project owner. Keep unconsumed handoffs visible as pending reports; do not silently count them as canonical completed tasks.

## Project and team orchestrators

The project orchestrator plans the project, owns shared records, and coordinates integration and release. A team orchestrator manages only its named feature, developer assignments, local evidence, and handoff. Both use the selected platform's orchestrator model/effort. The project orchestrator may also lead one small feature; do not add a supervisory agent just to satisfy a diagram.

Use independent full sessions for multiple team orchestrators when the host supports them. Spawn or attach such sessions only through supported controls. Native subagents that cannot delegate must not be presented as full team orchestrators. If independent sessions are unavailable, one parent can manage separately named developer groups and worktrees; disclose that they share an orchestrator. Developers, reviewers, and testers never spawn teammates. Each full team orchestrator dispatches only its own members; the project owner does not duplicate those assignments.

Assign every actionable task one implementation owner at a time. Link cross-team dependencies. If teams need overlapping changes, agree on the shared contract and serialize the conflicting part. Independent work can continue while a project owner is temporarily unavailable; shared mutations, merges, and releases wait for an authorized owner.

## Main, feature, and integration worktrees

Use the main checkout for project initialization, planning, and coordination. The main branch still contains integrated application code. After initialization, all feature code, tests, feature documentation, and repairs occur in separate feature worktrees, even when a lead handles a small change directly. Never edit application files in main as a shortcut.

Create a feature branch/worktree from an identified base using the project's Git rules. Preserve existing edits. If the repository has no initial commit or commit authority is missing, finish permissible initialization/planning and report that concrete prerequisite; do not silently commit or invent worktree isolation. Adopt an existing suitable worktree after checking ownership instead of duplicating it.

A team lead owns its feature worktree's CONTEXT.md. Developers sharing that checkout must have disjoint writable paths and separate context locations; use further worktrees for independent writable streams. Record every created or adopted task worktree for later cleanup. Worktrees do not isolate databases, ports, credentials, caches, or external services. Allocate separate resources only where concurrent use can conflict.

Use one reusable integration worktree owned by the project orchestrator. Merge branches, not directories. Feature teams do not merge into main or deploy their feature directly to production. A verified team handoff contains the exact commit, task/requirement coverage, checks, documentation and mistake pointers, environment effects, remaining limits, and the [preview gate](preview.md) when enabled. Follow project commit and branch-protection rules; uncommitted edits are preserved but are not a merge-ready handoff.

## Integrate serially

1. Obtain exclusive project integration ownership. Inspect any in-flight merge or release before starting another.
2. Confirm feature evidence and required user preview approval. A success report alone is not approval. Do not begin integrating a gated feature before approval.
3. Read current main and combine the approved feature revision in the integration worktree. Do not reset away unresolved work left by an interrupted integration.
4. Check the combined result, reusing valid evidence and rerunning only affected checks or required project gates. Return feature defects to their owner. Make integration repairs in the integration worktree. Material changes to an approved feature need a new review version and renewed approval before main is updated.
5. Update main through the established authorized merge/PR process only if it still matches the tested base. If main advances, incorporate it and repeat affected checks. Never force an update to bypass a race or protection rule.
6. Follow [release](release.md) for production deployment, live verification, tracker updates, and required disk cleanup. Merged, deployed, and verified-in-production are separate facts. A pending preview approval cannot be bypassed by another team's combined release.

Keep the integration worktree at most one per project. Reuse it while queued work needs it; after verified release and no queued or unfinished integration, remove the clean idle integration worktree using the same cleanup checks. Main is always preserved.
