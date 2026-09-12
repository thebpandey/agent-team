# Setup, Handoff, and Orchestrator Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Release Agent-Team 7.2.0 and Project Kickoff 0.4.1 with trusted setup identity, immutable handoff provenance, scoped runtime transitions, evidence-based orchestration, transactional artifact installation, and verified BrainVault migration.

**Architecture:** Agent-Team keeps its dependency-free Node/Markdown package and adds trusted native context at the CLI boundary, atomic owner-only runtime transitions, a focused run-state module, compatibility-aware dependency reuse, and receipt-bound installation. Project Kickoff keeps handoff schema version 1, changes project.revision from exact-tip equality to generation-baseline ancestry, emits the observed current tip through Agent-Team 7.2.0's released request contract, and adds a standard-library transactional installer. Runtime enforcement, instruction pressure tests, deterministic artifacts, host installation, and BrainVault migration are separate gates.

**Tech Stack:** Node.js 24 standard library and node:test; Python 3 standard library and unittest; Markdown; JSON; Git worktrees; GitHub Actions/CLI; SHA-256 artifacts; Codex and Claude Code native sessions.

**Spec:** docs/superpowers/specs/2026-09-11-setup-handoff-orchestrator-reliability-design.md

**Exact contract supplement:** /tmp/at-720-exact-contracts.md (copy its closed envelopes and field names verbatim; this plan includes the release-critical portions so implementation does not depend on the temporary file surviving).

## Global Constraints

- Release Agent-Team as 7.2.0 and keep Project Kickoff at unpublished 0.4.1.
- Publish and verify Agent-Team 7.2.0 before Project Kickoff 0.4.1; install both packages on both hosts before BrainVault migration.
- Use isolated task worktrees and one serial integration worktree per repository. No feature worker writes canonical main or the shared ledger.
- The project orchestrator alone updates .agent-team/TASKS.md, integrates accepted revisions, publishes, installs, and performs migration transitions.
- Before dispatch and after each developer handoff, route exact-source search, acceptance verification, requirements review, and code-quality review to an independent GPT-5.6-Sol verifier at medium effort. The owning developer repairs findings; integrate only a verifier-accepted revision.
- Workers do not spawn children. Each assignment names exclusive paths, task IDs, base revision, worktree, acceptance checks, evidence destination, model, effort, and prohibitions.
- Follow code TDD and writing-skills RED/GREEN. Watch each new automated test fail for the intended reason. Capture verbatim pressure-test rationalizations before changing instruction text.
- Preserve unrelated tracked, untracked, ignored, customized, and reused-unowned resources. Never force-remove a worktree, reset user work, or manufacture ownership.
- Native identity is { host: codex | claude-code, sessionId: string, observed: true, cwd: string }. Request JSON, CLI --project, and subprocess workdir cannot assert it.
- Setup Cancel/Keep Existing preserves complete settings bytes, setup version, roles, fallbacks, run defaults, dashboard/deployment values, and settings operations; earlier valid dependency preparation remains.
- Initialization validates one exact tracker snapshot. Later work uses state.run.taskIds; completion, integration, release, and cleanup IDs must be subsets.
- Current BrainVault repair is gated first by runtime-qualified owner liveness. While the historical owner is active, make no mutation or team interruption; because neither current host can mint the opaque recovery capability, record positive recovery as unavailable rather than claiming a live `owner_active` receipt. After cooperative handoff or authoritative stopped proof plus a safe checkpoint and a capable native bootstrap, register the declared external evidence store, reconcile generic 6zf and 0o4 completion histories while preserving n8s, then reconcile the legacy run; every transition is versioned, fingerprinted, replay-safe, and never hand-edits JSON.
- Preserve compatibility only for Project Kickoff 0.3.1/0.4.0 with Agent-Team 7.0.2, existing 0.4.1 with 7.1.0, and new 0.4.1 with 7.2.0. Reject crossed/unknown pairs.
- Compatible referenced skills become reused_unowned only after pinned structural/content, functional, and fresh-worker checks. Never copy them or write a sidecar to claim them.
- LeanCTX does not gate Graphify. Graphify accepts deterministic AST-origin INFERRED relationships and rejects semantic/missing origin.
- Use the absolute managed ast-grep executable. Never accept bare /usr/bin/sg; normalize a sibling only inside the same verified pinned package root.
- Active run state records mode, taskIds, teamLimit, autoDeploy, batchSize, and source; never re-infer them from changed defaults.
- Underfilled release batches are allowed only for finite exhaustion, continuous scoped exhaustion, or an all-blocked tail whose ordinary release gates pass.
- A user question interrupts but does not terminate an active run. While workers remain, wait at most 60 seconds, emit one compact heartbeat from known state, reconcile/refill, and repeat. This creates no daemon, post-turn schedule, or extra worker call. Exact incremental cost is unknown.
- Status separates generatedBy/testedAgainst, loadedRuntime, sourceCandidate, and readiness. Beads without a Dolt remote is local_only; missing production target is enabled_but_held: target_required.
- Preserve BrainVault's authorized origin/main publication target separately from Vercel, database, DNS, credentials, and production-check holds. Authority never crosses targets.
- Install only freshly downloaded release archives that pass published SHA256SUMS. Receipts bind release tag/URL, source revision, archive SHA-256, installed file digests, host/scope, transaction/recovery identity, and time.
- Agent-Team consumes archive .agent-team-source.json. Project Kickoff provides an equivalent transactional installer. Reinstall is idempotent; downgrade requires explicit compatible authority; partial both-host failure rolls back.
- Explicitly invalidate the old Project Kickoff a296ce8 / 43b6b85ede01bbc63b0a85bad25a5e017b228b3f7789fae060e8c8ebc5b73aab candidate.
- No force-push, tag replacement, ambiguous release retry, direct runtime-state edit, historical provenance rewrite, unapproved production action, or optional-hook activation.

## SDD worktrees, ledger, and review route

Before dispatch, run `scripts/sdd-workspace docs/superpowers/plans/2026-09-11-setup-handoff-orchestrator-reliability.md`, seed REL-001 through REL-018 in the plan-specific ignored `progress.md` and existing ignored `.agent-team/TASKS.md`, and give each task an evidence path under `.agent-team/evidence/reliability/`. Only the orchestrator writes those ledgers. Each implementer uses branch `feat/reliability-rel-NNN` and one isolated worktree derived from verified canonical main. One integration worktree per repository combines verifier-accepted commits serially.

Run exactly one implementation subagent at a time in numeric order. REL-001 through REL-011 implement, qualify, and publish Agent-Team; REL-012 installs it on both hosts; REL-013 and REL-014 correct and package Project Kickoff; REL-015 qualifies and publishes it; REL-016 installs it on both hosts; REL-017 performs BrainVault native acceptance; REL-018 performs the broad final review. Shared files (`hooks/agent-team-cli.mjs`, `hooks/lib/initialization.mjs`, `hooks/lib/workflow-cli.mjs`, tests, READMEs, manifests, and release evidence) therefore have one writer at any instant and always start from the previously accepted integrated revision.

For every task, record BASE, generate the SDD task brief, dispatch one implementer with no child agents, and require its commit/report. Then generate the SDD review package, dispatch the fixed Agent-Team verifier route (GPT-5.6-Sol, medium effort) for requirements and code-quality review, return every finding to the owning implementer, and dispatch a scoped Sol-medium re-review. Only a clean re-review permits integration and the `Task N: complete` ledger line. Ledger the ruling verbatim: `Ruling: use GPT-5.6-Sol at medium effort for task and broad final review — the user-selected Agent-Team SKILL.md orchestrator-conduct verifier route and references/team.md/platform adapter contract are more specific than subagent-driven-development's generic most-capable-final-review advice — if wrong, cost is one additional most-capable broad review; implementation and evidence remain unchanged.` REL-018 repeats that broad whole-branch Sol-medium review after all tasks.

At every boundary run:

    git status --short
    git diff --check
    git rev-parse HEAD

---

### Task 1: Trusted native identity and immutable initialization provenance (REL-001)

**Files:**
- Create: hooks/lib/owner-recovery.mjs
- Modify: hooks/agent-team-cli.mjs:56-101
- Modify: hooks/lib/initialization.mjs:20-289
- Modify: hooks/lib/canonical-state.mjs:18-63,214-218
- Modify: hooks/lib/project.mjs:24-83
- Modify: hooks/lib/task-transitions.mjs:110-149
- Modify: hooks/lib/policy.mjs:155-282,398-417
- Modify: hooks/lib/setup-cli.mjs:126-202
- Create: tests/hooks-owner-recovery.test.mjs
- Test: tests/hooks-initialization.test.mjs
- Test: tests/hooks-project-journey.test.mjs
- Test: tests/hooks-artifacts.test.mjs

**Interfaces:**
- Consumes: native host observation supplied only through runCommand context.
- Produces: `runCommand(command, options, { nativeIdentity, ...context })`; internal `initializeProject(projectPath, request, { actorSessionId, nativeIdentity, validateOnly = false, ...options })`, called only after closed-envelope validation; `initialization.initialTaskIds`; `initialization.trackerFingerprint`; and the exact optional request/receipt contract below. These are the only cross-skill wire keys Project Kickoff may emit.
- Produces: runtime-qualified current ownership, `identityFor(registry, host, sessionId)`, and native-only `recoverProjectOwner(project, envelope, { nativeOwnerRecovery })`; the routed command is exactly `project-owner-recover`. Owner recovery is separate from settings/resume and uses `.agent-team/.owner-recovery.json` as a write-ahead fencing journal over exactly four canonical records: `TEAMS.md`, `.agent-team/state.json`, `.agent-team/setup.json`, and `.agent-team/owner-history.json`.

```js
request.handoff = {
  schemaVersion: 1,
  path: "AGENT_TEAM_HANDOFF.json",
  sha256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  generatedBy: { name: "project-kickoff", version: "0.4.1" },
  testedAgainst: { name: "agent-team", version: "7.2.0" },
  generationBaseline: "1111111111111111111111111111111111111111", // project.revision
  observedRevision: "2222222222222222222222222222222222222222" // tip observed at emission
};
setup.initialization.handoff = {
  ...request.handoff,
  consumptionOperationId: request.operationId,
  consumedAt: "2026-09-11T12:00:00.000Z"
};
```

The complete `project-initialize` body fixture is:

```json
{
  "operationId": "brainvault-project-initialize-1",
  "projectId": "brainvault-system",
  "source": "existing",
  "tracker": { "kind": "beads", "root": ".", "executable": "/home/server/.agent-team/tools/bin/bd" },
  "plan": {
    "scope": "approved BrainVault delivery scope",
    "acceptance": ["approved acceptance statement"],
    "verification": ["approved verification command"],
    "branch": "main",
    "authority": { "ownedPaths": ["src"] },
    "tasks": [{ "id": "brainvault-system-abc" }]
  },
  "handoff": {
    "schemaVersion": 1,
    "path": "AGENT_TEAM_HANDOFF.json",
    "sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "generatedBy": { "name": "project-kickoff", "version": "0.4.1" },
    "testedAgainst": { "name": "agent-team", "version": "7.2.0" },
    "generationBaseline": "1111111111111111111111111111111111111111",
    "observedRevision": "2222222222222222222222222222222222222222"
  }
}
```

- [ ] **Step 1: Write failing identity tests.**

Add no-identity, observed-false, unsupported-host, mismatched-session, nested-repository CWD, and linked-worktree CWD cases. Assert no .agent-team directory or tracker is created.

```js
async function requestFile(value) {
  const file = path.join(await mkdtemp(path.join(tmpdir(), "agent-team-request-")), "request.json");
  await writeFile(file, `${JSON.stringify(value)}\n`, { mode: 0o600 });
  return file;
}
```

    const nativeIdentity = { host: "codex", sessionId: "native-owner", observed: true, cwd: root };
    const envelope = { schemaVersion: 1, actorSessionId: "different-owner", expectedVersion: 0, request };
    const result = await runCommand("project-initialize", { project: root, request: await requestFile(envelope) }, { nativeIdentity });
    assert.equal(result.reason, "native_identity_mismatch");
    await assert.rejects(access(path.join(root, ".agent-team")), { code: "ENOENT" });

    const passing = { schemaVersion: 1, actorSessionId: "native-owner", expectedVersion: 0, request };
    assert.equal(Object.hasOwn(passing.request, "ownerSessionId"), false);
    assert.equal((await runCommand("project-initialize", { project: root, request: await requestFile(passing) }, { nativeIdentity })).status, "initialized");

    const forbidden = { ...passing, request: { ...request, ownerSessionId: "native-owner" } };
    assert.equal((await runCommand("project-initialize", { project: root, request: await requestFile(forbidden) }, { nativeIdentity })).reason, "invalid_request");

Add the provenance fixture and assertion:

```js
const handoffBytes = Buffer.from('{"schemaVersion":1,"kind":"project-kickoff-agent-team-handoff"}\n');
const generationBaseline = await git(root, ["rev-parse", "HEAD"]);
await writeFile(path.join(root, "AGENT_TEAM_HANDOFF.json"), handoffBytes);
await git(root, ["add", "AGENT_TEAM_HANDOFF.json"]);
await git(root, ["commit", "-m", "fixture handoff"]);
const observedRevision = await git(root, ["rev-parse", "HEAD"]);
const handoff = {
  schemaVersion: 1, path: "AGENT_TEAM_HANDOFF.json", sha256: createHash("sha256").update(handoffBytes).digest("hex"),
  generatedBy: { name: "project-kickoff", version: "0.4.1" },
  testedAgainst: { name: "agent-team", version: "7.2.0" },
  generationBaseline, observedRevision,
};
const envelope = { schemaVersion: 1, actorSessionId: nativeIdentity.sessionId, expectedVersion: 0,
  request: { ...request, handoff } };
const result = await runCommand("project-initialize", { project: root, request: await requestFile(envelope) }, { nativeIdentity });
assert.equal(result.setup.initialization.handoff.consumptionOperationId, request.operationId);
assert.match(result.setup.initialization.handoff.consumedAt, /^\d{4}-\d{2}-\d{2}T/);
```

- [ ] **Step 2: Run RED.**

Run: node --test --test-name-pattern='native identity|shell validation|nested repository|linked worktree' tests/hooks-initialization.test.mjs tests/hooks-project-journey.test.mjs tests/hooks-artifacts.test.mjs

Expected: FAIL because request-controlled identity currently reaches initialization and extracted shell CLI can own a project.

- [ ] **Step 3: Validate native identity before filesystem access.**

Implement a closed check that returns native_identity_required, native_host_unsupported, native_identity_mismatch, or native_project_cwd_mismatch. Plain shell invocation returns { status: "validated", ready: false, reason: "native_identity_required" } without ownership mutation. Preserve existing initialized owners.

```js
async function gitProjectIdentity(location) {
  const cwd = await realpath(location);
  const top = await realpath((await run("git", ["-C", cwd, "rev-parse", "--show-toplevel"], { encoding: "utf8", timeout: 1000 })).stdout.trim());
  const commonRaw = (await run("git", ["-C", cwd, "rev-parse", "--git-common-dir"], { encoding: "utf8", timeout: 1000 })).stdout.trim();
  const common = await realpath(path.resolve(cwd, commonRaw));
  return { top, common };
}
async function nativeIdentityProblem(projectPath, actorSessionId, nativeIdentity) {
  if (nativeIdentity?.observed !== true) return "native_identity_required";
  if (!["codex", "claude-code"].includes(nativeIdentity.host)) return "native_host_unsupported";
  if (!validId(actorSessionId) || !validId(nativeIdentity.sessionId) || nativeIdentity.sessionId !== actorSessionId) return "native_identity_mismatch";
  const expected = await gitProjectIdentity(projectPath);
  const observed = await gitProjectIdentity(nativeIdentity.cwd);
  if (observed.top !== expected.top || observed.common !== expected.common) return "native_project_cwd_mismatch";
}
```

The router validates the closed envelope first, rejects `request.ownerSessionId` as an unknown body field, and calls `nativeIdentityProblem(projectPath, envelope.actorSessionId, nativeIdentity)`. The validated envelope actor—not any request-body owner claim—is the session persisted by initialization.

This permits a normal subdirectory whose Git top/common identity is the active project, but rejects a nested repository and a sibling linked worktree even when the sibling shares the Git common directory.

- [ ] **Step 4: Persist immutable provenance.**

Store exact initial task IDs and tracker fingerprint. For a Kickoff handoff require 64-hex digest, semantic producer/tested versions, full 40-hex baseline/current revisions, and consumption operation. Keep exact tracker identity at adoption.

```js
const supportedPairs = new Set(["0.3.1/7.0.2", "0.4.0/7.0.2", "0.4.1/7.1.0", "0.4.1/7.2.0"]);
async function validateHandoff(projectRoot, branch, handoff, { readSafeFile, git }) {
  if (handoff === undefined) return undefined;
  if (stable(Object.keys(handoff).sort()) !== stable(["generatedBy", "generationBaseline", "observedRevision", "path", "schemaVersion", "sha256", "testedAgainst"])) return "invalid_handoff_provenance";
  if (handoff.schemaVersion !== 1 || !relative(handoff.path)) return "invalid_handoff_provenance";
  if (!/^[a-f0-9]{64}$/.test(handoff.sha256) || ![handoff.generationBaseline, handoff.observedRevision].every((v) => /^[a-f0-9]{40}$/.test(v))) return "invalid_handoff_provenance";
  if (handoff.generatedBy?.name !== "project-kickoff" || handoff.testedAgainst?.name !== "agent-team"
    || !supportedPairs.has(`${handoff.generatedBy.version}/${handoff.testedAgainst.version}`)) return "unsupported_handoff_contract";
  const bytes = await readSafeFile(path.join(projectRoot, handoff.path), { regular: true, symlink: false, maxBytes: 250 * 1024 });
  if (createHash("sha256").update(bytes).digest("hex") !== handoff.sha256) return "handoff_digest_changed";
  if (await git(projectRoot, ["merge-base", "--is-ancestor", handoff.generationBaseline, handoff.observedRevision], { status: true }) !== 0) return "invalid_handoff_ancestry";
  if (await git(projectRoot, ["rev-parse", `refs/heads/${branch}^{commit}`]) !== handoff.observedRevision) return "handoff_observed_revision_changed";
}
```

- [ ] **Step 5: Update extracted-artifact tests.**

Test shell validation separately, then import extracted runCommand with an observed exact native identity to initialize.

- [ ] **Step 6: Run GREEN.**

Run: node --test tests/hooks-initialization.test.mjs tests/hooks-project-journey.test.mjs tests/hooks-artifacts.test.mjs tests/hooks-setup-cli.test.mjs

Expected: PASS; mismatches make zero mutations, exact native identity initializes once, and replay is idempotent.

- [ ] **Step 7: Run the identity/provenance GREEN checkpoint.**

Run: `node --test tests/hooks-initialization.test.mjs tests/hooks-project-journey.test.mjs tests/hooks-artifacts.test.mjs`

Expected: PASS before extending the same independently reviewed Task 1 deliverable with owner recovery.

- [ ] **Step 8: Write owner-recovery RED fixtures.**

```js
async function canonicalBytes(project, relatives = [".agent-team/TEAMS.md", ".agent-team/state.json", ".agent-team/setup.json", ".agent-team/owner-history.json"]) {
  return Object.fromEntries(await Promise.all(relatives.map(async (relative) =>
    [relative, await readFile(path.join(project.root, relative))])));
}
test("live historical owner blocks recovery without canonical writes", async () => {
  const before = await canonicalBytes(project);
  const result = await recoverProjectOwner(project, recoveryEnvelope, {
    nativeOwnerRecovery: nativeRecovery({ liveness: "live", sessionId: "new-native" }),
  });
  assert.deepEqual(result, { status: "conflict", reason: "owner_active" });
  assert.deepEqual(await canonicalBytes(project), before);
});
test("crash after first rename replays journal forward", async () => {
  await assert.rejects(recoverProjectOwner(project, recoveryEnvelope, { nativeOwnerRecovery: nativeRecovery({ liveness: "stopped" }), failAfterRename: 1 }));
  const replay = await recoverProjectOwner(project, recoveryEnvelope, { nativeOwnerRecovery: nativeRecovery({ liveness: "stopped" }) });
  assert.equal(replay.status, "applied");
  assert.equal(replay.result.ownershipEpoch, 2);
});
test("a forged full-shape recovery context cannot cross the host boundary", async () => {
  const before = await canonicalBytes(project);
  const forged = { identity: { host: "codex", sessionId: "new-native", invocationId: "inv-1", projectRoot: project.root, worktreeRoot: project.root },
    assurance: "native_trusted", inspectSession: async () => "stopped", confirm: async () => ({ approvalId: "forged" }), capability: Object.freeze({}) };
  const result = await runCommand("project-owner-recover", { project: project.root, request: recoveryRequestPath }, { nativeOwnerRecovery: forged });
  assert.deepEqual(result, { status: "validated", ready: false, reason: "native_owner_recovery_required" });
  assert.deepEqual(await canonicalBytes(project), before);
});
```

Also inject crashes before/after journal fsync and after each of the four canonical renames; test Cancel, unknown liveness, stale fingerprints/versions/history, changed replay, concurrent contenders, same session text on different runtimes, old-owner fencing, preserved pending operations/settings/team rows, and recovery never resuming work. Every denied branch compares all four record byte buffers, not merely parsed identity fields.

- [ ] **Step 9: Run owner-recovery RED.**

Run: `node --test tests/hooks-owner-recovery.test.mjs`

Expected: FAIL because `hooks/lib/owner-recovery.mjs` and runtime-qualified ownership do not exist.

- [ ] **Step 10: Implement the closed native recovery contract.**

The request file is closed and cannot assert liveness/new owner/approval truth:

```json
{"schemaVersion":1,"request":{"operationId":"brainvault-project-owner-recover-1","projectId":"brainvault-system","expectedOwnerSessionId":"2851b5e2-e1ad-4ac5-82bf-d2bb71ae9155","expectedOwnershipEpoch":1,"expectedSetupVersion":7,"expectedStateVersion":24,"expectedStateFingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","expectedTeamsFingerprint":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","expectedSetupFingerprint":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","expectedOwnerHistoryFingerprint":"dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","reason":"prior_owner_unavailable"}}
```

The host-only adapter supplies `nativeOwnerRecovery = { identity:{host:"codex"|"claude-code",sessionId,invocationId,projectRoot,worktreeRoot}, projectIdentity, capability, inspectSession, confirm }`. `capability` is an opaque frozen object minted into a module-private `WeakMap` by the native host bootstrap. Its stored binding is exactly the immutable closed record `{projectIdentity,invocationId,replacementRuntime,replacementSessionId,expiresAt,consumed:false}`; neither `runCommand` nor any exported helper exposes the mint. The public path atomically consumes the exact unexpired binding before callbacks, locks, journals, or canonical reads. Missing, expired, replayed, cross-project, cross-session/runtime, mismatched-invocation, or caller-constructed capability context receives the single public reason `native_owner_recovery_required`. `inspectSession` returns only `active|stopped|unknown`; `confirm` returns a host-issued approval bound to project, old/new qualified identities, both expected versions, all four prior hashes, pending operations and forced holds. Neither fact is accepted from the envelope. Only authoritative `stopped` plus that confirmation may transfer. Active returns `owner_active`; unknown returns `owner_liveness_unknown`. Runtime-qualified state is:

```js
state.ownership = { epoch: previousEpoch + 1, current: { host: native.host, sessionId: native.sessionId, since: now, operationId } };
ownerHistory = { schemaVersion: 1, version: previousHistory.version + 1,
  entries: [...previousHistory.entries, { epoch: previousEpoch, oldOwner, newOwner, reason, approvalId: confirmation.approvalId,
    livenessEvidence, priorStateFingerprint, priorTeamsFingerprint, priorSetupFingerprint,
    priorOwnerHistoryFingerprint, appliedAt: now }] };
```

Keep the capability registry in the CLI closure, not in an exported object or string-valued assurance flag:

```js
const ownerRecoveryCapabilities = new WeakMap();
const consumedOwnerRecoveryCapabilities = new WeakSet();
function mintOwnerRecoveryCapabilityFromHostBootstrap(binding) {
  const expectedKeys = ["expiresAt", "invocationId", "projectIdentity", "replacementRuntime", "replacementSessionId"];
  if (stable(Object.keys(binding).sort()) !== stable(expectedKeys)
    || !validProjectIdentity(binding.projectIdentity) || !validId(binding.invocationId)
    || !["codex", "claude-code"].includes(binding.replacementRuntime) || !validId(binding.replacementSessionId)
    || !Number.isSafeInteger(binding.expiresAt) || binding.expiresAt <= Date.now()
    || binding.expiresAt > Date.now() + 60_000) throw new Error("invalid_owner_recovery_binding");
  const capability = Object.freeze(Object.create(null));
  ownerRecoveryCapabilities.set(capability, Object.freeze({ ...structuredClone(binding), consumed: false }));
  return capability;
}
function consumeOwnerRecoveryCapability(context, now = Date.now()) {
  const grant = typeof context?.capability === "object" && context.capability !== null
    ? ownerRecoveryCapabilities.get(context.capability) : undefined;
  const observed = { projectIdentity: context?.projectIdentity, invocationId: context?.identity?.invocationId,
    replacementRuntime: context?.identity?.host, replacementSessionId: context?.identity?.sessionId,
    expiresAt: grant?.expiresAt, consumed: false };
  if (!grant || consumedOwnerRecoveryCapabilities.has(context.capability) || grant.expiresAt <= now
    || stable(grant) !== stable(observed)) return false;
  consumedOwnerRecoveryCapabilities.add(context.capability);
  return true;
}
```

`mintOwnerRecoveryCapabilityFromHostBootstrap` is private and has no CLI route or package export. `runCommand` derives the current project identity independently and passes it with the current invocation and replacement runtime/session to `consumeOwnerRecoveryCapability`; callers cannot choose comparison values separately from those live contexts. Consumption is a synchronous `WeakSet` insertion before any awaited work, so concurrent reuse has one winner; every losing or mismatched call returns `native_owner_recovery_required` without invoking callbacks. Tests cover expiry, forged full shape, one-shot replay, cross-project reuse, cross-session/runtime reuse, changed invocation and two concurrent calls, and compare all four canonical records byte-for-byte after every rejection. Until a Codex or Claude Code bootstrap is explicitly wired to call it, the installed product cannot perform a positive recovery. The transaction module accepts a private boolean/callback from this closure solely for deterministic unit testing; the public `runCommand` path never accepts that seam from caller context.

The journal generator is exact and ordered:

```js
const records = [
  journalRecord("teams", project.teamsPath, priorTeamsBytes, nextTeamsBytes),
  journalRecord("state", project.statePath, priorStateBytes, encodeJson(nextState)),
  journalRecord("setup", project.setupPath, priorSetupBytes, encodeJson(nextSetup)),
  journalRecord("owner-history", project.ownerHistoryPath, priorHistoryBytes, encodeJson(ownerHistory)),
];
const journal = { schemaVersion: 1, operationId: request.operationId, phase: "prepared", nextRecord: 0,
  expectedSetupVersion: request.expectedSetupVersion, expectedStateVersion: request.expectedStateVersion,
  expectedOwnerHistoryFingerprint: request.expectedOwnerHistoryFingerprint,
  records: records.map(({ name, path, before, after }) => ({ name, path,
    priorSha256: sha256(before), postSha256: sha256(after), postimageBase64: after.toString("base64") })) };
await durableWrite(project.ownerRecoveryJournalPath, encodeJson(journal));
for (let index = journal.nextRecord; index < records.length; index += 1) {
  await durableReplace(records[index].path, records[index].after);
  await durableWrite(project.ownerRecoveryJournalPath, encodeJson({ ...journal,
    phase: `renamed:${index + 1}`, nextRecord: index + 1 }));
}
await verifyFourRecordPostimages(records);
await durableWrite(project.ownerRecoveryJournalPath, encodeJson({ ...journal, phase: "committed", nextRecord: 4 }));
await durableUnlink(project.ownerRecoveryJournalPath);
```

Implement the private helpers in `owner-recovery.mjs`: `journalRecord` retains byte buffers and exact canonical paths; `encodeJson` emits canonical JSON plus newline; `sha256` hashes bytes; `durableWrite` and `durableReplace` use 0600 same-directory temporary regular files, file fsync, atomic rename and parent-directory fsync; `verifyFourRecordPostimages` reopens all four records no-follow and compares live hashes; `durableUnlink` unlinks and fsyncs the journal directory.

Add `Project owner host: claude-code|codex` and `Integration owner host: claude-code|codex` beside the existing session labels, add `integration.ownerHost` and `release.ownerHost`, and compare `(host, sessionId, ownership.epoch)` at every writer/policy boundary. `resolveProject` returns `ownerHistoryPath` and its live fingerprint, and `loadCanonicalState` validates the owner-history schema/version and the state/setup/history epoch agreement. Acquire `.locks/owner-recovery.lock`, `setup.lock`, then `state.lock`; re-read setup/state/TEAMS/owner-history, both versions, all four fingerprints and liveness under lock. Generate all four postimages in memory. Write `.agent-team/.owner-recovery.json` with `schemaVersion`, operation ID, exact ordered record names, each record's path/prior fingerprint/post fingerprint/base64 postimage, `expectedSetupVersion`, `expectedStateVersion`, `expectedOwnerHistoryFingerprint`, and phase `prepared`; fsync the journal and directory. Rename in the fixed order `TEAMS.md`, `state.json`, `setup.json`, `owner-history.json`, writing/fsyncing journal phase `renamed:1` through `renamed:4` and fsyncing each destination directory after every rename. Verify all four post-fingerprints plus cross-record qualified identity/epoch, write/fsync `committed`, then unlink/fsync the journal. On startup, each writer invokes replay: a record at its prior hash receives its journal postimage, a record at its post hash is retained, and any third hash returns `owner_recovery_manual_reconciliation_required`. Crash tests cover prepare plus renames 1, 2, 3 and 4, and every replay converges to byte-identical four-record output. Preserve historical authors, writers, evidence, pending operations, gate evidence, scope/task runtime and both host settings; change only current-owner fields and append owner history. Existing owner-bound authority remains historical and becomes inoperative through the ownership-epoch comparison rather than being erased or transferred. Recovery never resumes, pauses, stops, or reassigns work.

The current Codex and Claude Code integrations do not expose this opaque native recovery capability, so a positive installed-host recovery invocation is unavailable in 7.2.0 and is explicitly not a release blocker. Unit tests exercise the transaction/replay engine through its internal capability-validation seam; installed-host acceptance must prove forged full-shape rejection and, while BrainVault's owner is live, `owner_active` only if a future host bootstrap actually supplies a valid capability.

- [ ] **Step 11: Run owner-recovery GREEN.**

Run: `node --test tests/hooks-owner-recovery.test.mjs tests/hooks-initialization.test.mjs tests/hooks-transitions.test.mjs tests/hooks-policy.test.mjs tests/hooks-settings.test.mjs`

Expected: PASS including crash fencing, qualified identity, no-write stop branches, and separate settings Cancel.

- [ ] **Step 12: Commit Task 1.**

    git add hooks/lib/owner-recovery.mjs hooks/lib/canonical-state.mjs hooks/lib/project.mjs hooks/lib/task-transitions.mjs hooks/lib/policy.mjs hooks/agent-team-cli.mjs hooks/manifest.json tests/hooks-owner-recovery.test.mjs tests/hooks-initialization.test.mjs tests/hooks-transitions.test.mjs tests/hooks-policy.test.mjs tests/hooks-settings.test.mjs
    git commit -m "feat: bind native ownership and recovery"

### Task 2: Setup always enters settings and Cancel preserves bytes (REL-002)

**Files:**
- Modify: hooks/lib/settings.mjs:96-133,169-201,256-268
- Modify: hooks/lib/setup-cli.mjs:185-264
- Test: tests/hooks-settings.test.mjs
- Test: tests/hooks-setup-cli.test.mjs
- Test: tests/hooks-project-journey.test.mjs

**Interfaces:**
- Consumes: existing settings wizard, dependency result, readiness result, native identity from REL-001.
- Produces: cancel result { status: "cancel", settingsOutcome: "kept_existing" }; pure buildSetupSummary({ project, dependencies, readiness, settings, settingsOutcome, nativeIdentity }).

- [ ] **Step 1: Write failing byte-preservation tests.**

Hash exact setup bytes before Cancel, Back, absent answer, and simulated interruption. Assert no version/settings-operation change while an earlier dependency receipt remains.

```js
test("cancel preserves bytes and save is one versioned atomic mutation", async () => {
  const before = await readFile(setupPath);
  const cancelled = await updateSettings({ setupPath, host: "codex", expectedVersion: 4, writer, operationId: "settings-cancel", change: { kind: "cancel" } });
  assert.deepEqual(cancelled, { status: "cancel", settingsOutcome: "kept_existing" });
  assert.deepEqual(await readFile(setupPath), before);
  const saved = await updateSettings({ setupPath, host: "codex", expectedVersion: 4, writer, operationId: "settings-save-5", loadRegistry, change: { kind: "run", values: { teams: 2, deploy_batch_tasks: 1, auto_deploy: false } } });
  assert.equal(saved.status, "applied");
  assert.equal(saved.setup.version, 5);
  assert.equal(saved.setup.setupOperations.at(-1).id, "settings-save-5");
});
```

- [ ] **Step 2: Run RED.**

Run: node --test --test-name-pattern='cancel|repeated setup|state-changing setup|read-only action' tests/hooks-settings.test.mjs tests/hooks-setup-cli.test.mjs tests/hooks-project-journey.test.mjs

Expected: FAIL because Cancel lacks settingsOutcome and setup does not always enter settings.

- [ ] **Step 3: Implement the non-writing outcome.**

Return before mutateSetup for Cancel/Back. Sequence state-changing setup as canonical inspection, dependency inspection/preparation, current-effective wizard, readiness summary. Do not add a fake shell setup command.

```js
if (["cancel", "back"].includes(change?.kind)) {
  return { status: change.kind, settingsOutcome: "kept_existing" };
}
return mutateSetup({
  setupPath, expectedVersion, writer, operationId, loadRegistry, budget,
  operation: { kind: "settings", host, change },
  mutate: async (setup) => ({ setup: applyChange(setup, host, change, nativeChoices) }),
});
```

- [ ] **Step 4: Add read-only negative cases.**

Assert status, health, help, and version never launch a wizard or mutate records.

- [ ] **Step 5: Run GREEN.**

Run: node --test --test-name-pattern='cancel preserves bytes|save is one versioned atomic mutation|setup enters settings' tests/hooks-settings.test.mjs tests/hooks-setup-cli.test.mjs tests/hooks-project-journey.test.mjs

Expected: PASS with byte-exact cancellation and retained dependency preparation.

- [ ] **Step 6: Commit.**

    git add hooks/lib/settings.mjs hooks/lib/setup-cli.mjs tests/hooks-settings.test.mjs tests/hooks-setup-cli.test.mjs tests/hooks-project-journey.test.mjs
    git commit -m "feat: make setup settings cancellation explicit"

### Task 3: Effective run state and terminal batch selection (REL-003)

**Files:**
- Create: hooks/lib/run-state.mjs
- Modify: hooks/lib/canonical-state.mjs:45-64
- Modify: hooks/lib/workflow-cli.mjs:21-32,106-147,258-295
- Modify: hooks/manifest.json:6-116
- Create: tests/hooks-run-state.test.mjs
- Test: tests/hooks-transitions.test.mjs

**Interfaces:**
- Produces: validateEffectiveRun(run, canonicalTasks); startRun(project, request, options = {}); reconcileRun(project, request, options = {}); classifyRun(canonical, { writerLiveness = {} } = {}); selectReleaseBatch(canonical, classification); commands run-start, run-reconcile, and read-only run-decision. `loadCanonicalState()` also produces `canonical.deliveryEvidence[taskId]` by joining the exact completion, integration, review, check, preview, target-authority, and recovery receipts for that task; it never reads the aggregate release receipt.

- [ ] **Step 1: Write failing pure tests.**

Cover complete run fields, defaults drift, missing/duplicate IDs, hierarchy normalization, finite/continuous exhaustion, active eligible work, blocked tail, and one-of-two underfilled selection.

```js
const releaseEvidence = (id) => ({
  taskId: id, revision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", integratedRevision: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  review: { status: "passed" }, checks: [{ name: "unit", status: "passed" }],
  preview: { status: "passed" }, target: { status: "authorized" }, recovery: { status: "ready" },
});
test("finite exhausted run permits an underfilled terminal batch", () => {
  const run = { mode: "finite", taskIds: ["T-1"], teamLimit: 2, autoDeploy: true, batchSize: 2, source: "explicit_run" };
  const canonical = { state: { run: { ...run, id: "run-1", ownerSessionId: "owner", settingSources: { mode: "explicit_run", taskIds: "explicit_run", teamLimit: "explicit_run", autoDeploy: "explicit_run", batchSize: "explicit_run" }, paused: false, operationalVersion: 7, blockers: [], pendingDeliveryIds: ["T-1"], deployedTaskIds: [], terminalClassification: "finite_exhausted" } }, deliveryEvidence: { "T-1": releaseEvidence("T-1") }, tasks: [{ id: "T-1", status: "done", parentId: null }] };
  assert.deepEqual(classifyRun(canonical), { kind: "finite_exhausted", eligibleTaskIds: [], blockedTaskIds: [] });
  assert.deepEqual(selectReleaseBatch(canonical, classifyRun(canonical)), ["T-1"]);
});
test("legacy active run without id receives request-bound compatibility id", async () => {
  const legacy = { mode: "finite", taskIds: ["T-1"], paused: false };
  const request = {
    operationId: "reconcile-legacy-run", expectedTrackerFingerprint: trackerFingerprint,
    expectedRunFingerprint: stableDigest(legacy), authoritativeSource: "compatibility_migration",
    reason: "Reconcile the incomplete Agent-Team 7.1 active run record.", affectedTaskIds: ["T-1"],
    run: { id: "compat-run-1", mode: "finite", taskIds: ["T-1"], teamLimit: 3, autoDeploy: false, batchSize: 4, source: "compatibility_migration", settingSources: migrationSources },
  };
  const result = await reconcileRun(projectWithRun(legacy), request, ownerOptions);
  assert.equal(result.status, "applied");
  assert.equal(result.result.run.id, "compat-run-1");
  assert.equal((await reconcileRun(projectWithRun(undefined), request, ownerOptions)).reason, "run_not_active");
});
const explicitRun = () => ({ id: "run-1", mode: "finite", taskIds: ["T-1"], teamLimit: 2, autoDeploy: false, batchSize: 2,
  source: "explicit_run", settingSources: { mode: "explicit_run", taskIds: "explicit_run", teamLimit: "explicit_run", autoDeploy: "explicit_run", batchSize: "explicit_run" } });
const reconciliation = (previousRun) => ({ operationId: "reconcile-1", expectedTrackerFingerprint: trackerFingerprint,
  expectedRunFingerprint: createHash("sha256").update(stable(previousRun)).digest("hex"), authoritativeSource: "compatibility_migration",
  reason: "Populate the complete effective run without widening scope.", affectedTaskIds: [...previousRun.taskIds],
  run: { id: previousRun.id, mode: previousRun.mode, taskIds: [...previousRun.taskIds], teamLimit: previousRun.teamLimit,
    autoDeploy: false, batchSize: previousRun.batchSize, source: "compatibility_migration",
    settingSources: { mode: "compatibility_migration", taskIds: "compatibility_migration", teamLimit: "compatibility_migration", autoDeploy: "compatibility_migration", batchSize: "compatibility_migration" } } });

test("run mutations are versioned, replay-safe, fingerprinted, and scope preserving", async () => {
  const start = { operationId: "start-1", expectedTrackerFingerprint: trackerFingerprint,
    reason: "Start the approved finite run.", run: explicitRun() };
  const options = { actorSessionId: owner, expectedVersion: 7 };
  const applied = await startRun(project, start, options);
  assert.equal(applied.status, "applied");
  assert.deepEqual(applied.result.run.taskIds, ["T-1"]);
  assert.equal((await startRun(project, start, options)).status, "duplicate");
  assert.equal((await startRun(project, { ...start, reason: "changed" }, options)).reason, "operation_identity_reused");
  assert.equal((await startRun(project, { ...start, operationId: "stale" }, { ...options, expectedVersion: 6 })).reason, "stale_version");

  const before = structuredClone(applied.result.run);
  const reconcile = reconciliation(before);
  for (const [name, change] of [
    ["stale tracker", (value) => ({ ...value, operationId: "bad-tracker", expectedTrackerFingerprint: "c".repeat(64) })],
    ["stale run", (value) => ({ ...value, operationId: "bad-run", expectedRunFingerprint: "d".repeat(64) })],
    ["widened scope", (value) => ({ ...value, operationId: "bad-scope", run: { ...value.run, taskIds: [...value.run.taskIds, "T-2"] } })],
    ["migration deploy enablement", (value) => ({ ...value, operationId: "bad-deploy", run: { ...value.run, autoDeploy: true } })],
  ]) {
    const bytes = await readFile(project.paths.state);
    assert.equal((await reconcileRun(project, change(reconcile), { actorSessionId: owner, expectedVersion: applied.version })).status, "conflict", name);
    assert.deepEqual(await readFile(project.paths.state), bytes);
  }
  const reconciled = await reconcileRun(project, reconcile, { actorSessionId: owner, expectedVersion: applied.version });
  assert.equal(reconciled.status, "applied");
  assert.deepEqual(reconciled.result.previousRun, before);
  assert.deepEqual(reconciled.result.run.taskIds, before.taskIds);
  assert.equal(reconciled.result.deploymentHeld, true);
  assert.equal((await reconcileRun(project, reconcile, { actorSessionId: owner, expectedVersion: applied.version })).status, "duplicate");
});
```

- [ ] **Step 2: Run RED.**

Run: node --test tests/hooks-run-state.test.mjs

Expected: FAIL because run-state.mjs does not exist.

- [ ] **Step 3: Implement the closed active-run record.**

Persist id, ownerSessionId, mode, taskIds, teamLimit, autoDeploy, batchSize, source, settingSources, paused, operationalVersion, blockers, pendingDeliveryIds, deployedTaskIds, and terminalClassification. Allow source explicit_run, saved_default, or compatibility_migration. Unknown/incomplete state holds deployment but not safe independent development.

```js
export function validateEffectiveRun(run, canonicalTasks) {
  const ids = new Set(canonicalTasks.map(({ id }) => id));
  const sources = ["explicit_run", "saved_default", "compatibility_migration"];
  const arrayFields = ["taskIds", "blockers", "pendingDeliveryIds", "deployedTaskIds"];
  if (!run || !validId(run.id) || !validId(run.ownerSessionId) || !["finite", "continuous"].includes(run.mode)
    || arrayFields.some((field) => !Array.isArray(run[field]) || new Set(run[field]).size !== run[field].length)
    || new Set(run.taskIds).size !== run.taskIds.length || run.taskIds.some((id) => !ids.has(id))
    || !Number.isInteger(run.teamLimit) || run.teamLimit < 1 || typeof run.autoDeploy !== "boolean"
    || !Number.isInteger(run.batchSize) || run.batchSize < 1
    || !sources.includes(run.source) || typeof run.paused !== "boolean" || !Number.isSafeInteger(run.operationalVersion)
    || typeof run.terminalClassification !== "string"
    || [...run.pendingDeliveryIds, ...run.deployedTaskIds].some((id) => !run.taskIds.includes(id))
    || run.blockers.some((item) => !item || typeof item !== "object" || !run.taskIds.includes(item.taskId) || typeof item.reason !== "string")
    || stable(Object.keys(run.settingSources).sort()) !== stable(["autoDeploy", "batchSize", "mode", "taskIds", "teamLimit"])
    || Object.values(run.settingSources).some((source) => !sources.includes(source))) return "invalid_effective_run";
}

const runConflict = (reason) => ({ status: "conflict", reason });
const runSources = new Set(["explicit_run", "saved_default", "compatibility_migration"]);
const fingerprintRun = (run) => createHash("sha256").update(stable(run)).digest("hex");
const taskDependencies = (task) => Array.isArray(task.dependencies) ? task.dependencies : [];

function runInputProblem(run, tasks, { allowMigration }) {
  const sources = allowMigration ? runSources : new Set(["explicit_run", "saved_default"]);
  const ids = new Set(tasks.map(({ id }) => id));
  const completed = new Set(tasks.filter(({ status }) => ["done", "closed", "cancelled"].includes(status)).map(({ id }) => id));
  if (!run || stable(Object.keys(run).sort()) !== stable(["autoDeploy", "batchSize", "id", "mode", "settingSources", "source", "taskIds", "teamLimit"])
    || !validId(run.id) || !["finite", "continuous"].includes(run.mode) || !Array.isArray(run.taskIds) || !run.taskIds.length
    || new Set(run.taskIds).size !== run.taskIds.length || run.taskIds.some((id) => !ids.has(id))
    || !Number.isInteger(run.teamLimit) || run.teamLimit < 1 || run.teamLimit > 64
    || !Number.isInteger(run.batchSize) || run.batchSize < 1 || run.batchSize > 64 || typeof run.autoDeploy !== "boolean"
    || !sources.has(run.source) || stable(Object.keys(run.settingSources ?? {}).sort()) !== stable(["autoDeploy", "batchSize", "mode", "taskIds", "teamLimit"])
    || Object.values(run.settingSources ?? {}).some((source) => !sources.has(source))) return "invalid_effective_run";
  const scope = new Set(run.taskIds);
  if (tasks.filter(({ id }) => scope.has(id)).some((task) => taskDependencies(task).some((dependency) => !scope.has(dependency) && !completed.has(dependency)))) return "unresolved_scope_dependency";
}

export async function startRun(project, request, options = {}) {
  if (stable(Object.keys(request ?? {}).sort()) !== stable(["expectedTrackerFingerprint", "operationId", "reason", "run"])
    || !/^[a-f0-9]{64}$/.test(request.expectedTrackerFingerprint) || typeof request.reason !== "string"
    || !request.reason.length || Buffer.byteLength(request.reason) > 4096) return runConflict("invalid_request");
  const effectiveRequest = { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion };
  return mutateOperationalState(project, effectiveRequest, async (state) => {
    const canonical = await loadCanonicalState(project, { includeTasks: true, budget: options.budget });
    if (state.run?.id) return runConflict("run_already_active");
    if (canonical.tracker.status !== "current") return runConflict("tracker_unavailable");
    if (canonical.tracker.fingerprint !== request.expectedTrackerFingerprint) return runConflict("stale_tracker");
    const problem = runInputProblem(request.run, canonical.tasks, { allowMigration: false });
    if (problem) return runConflict(problem);
    const run = { ...request.run, ownerSessionId: options.actorSessionId, paused: false,
      operationalVersion: (state.stateVersion ?? 0) + 1, blockers: [], pendingDeliveryIds: [], deployedTaskIds: [], terminalClassification: "progress_possible" };
    return { state: { ...state, run }, result: { run: structuredClone(run), trackerFingerprint: canonical.tracker.fingerprint } };
  }, options);
}

export async function reconcileRun(project, request, options = {}) {
  if (stable(Object.keys(request ?? {}).sort()) !== stable(["affectedTaskIds", "authoritativeSource", "expectedRunFingerprint", "expectedTrackerFingerprint", "operationId", "reason", "run"])
    || ![request.expectedTrackerFingerprint, request.expectedRunFingerprint].every((value) => /^[a-f0-9]{64}$/.test(value))
    || typeof request.reason !== "string" || !request.reason.length || Buffer.byteLength(request.reason) > 4096) return runConflict("invalid_request");
  const effectiveRequest = { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion };
  return mutateOperationalState(project, effectiveRequest, async (state) => {
    const canonical = await loadCanonicalState(project, { includeTasks: true, budget: options.budget });
    const previousRun = structuredClone(state.run);
    if (!previousRun) return runConflict("run_not_active");
    const legacyCompatibility = !previousRun.id;
    if (previousRun.paused) return runConflict("paused");
    if (canonical.tracker.status !== "current") return runConflict("tracker_unavailable");
    if (canonical.tracker.fingerprint !== request.expectedTrackerFingerprint) return runConflict("stale_tracker");
    if (fingerprintRun(previousRun) !== request.expectedRunFingerprint) return runConflict("stale_run");
    if (!runSources.has(request.authoritativeSource) || request.run?.source !== request.authoritativeSource) return runConflict("invalid_authoritative_source");
    if (legacyCompatibility && request.authoritativeSource !== "compatibility_migration") return runConflict("legacy_run_requires_compatibility_migration");
    if ((!legacyCompatibility && request.run?.id !== previousRun.id) || (legacyCompatibility && !validId(request.run?.id))
      || !Array.isArray(request.affectedTaskIds) || !request.affectedTaskIds.length || new Set(request.affectedTaskIds).size !== request.affectedTaskIds.length
      || request.affectedTaskIds.some((id) => !previousRun.taskIds.includes(id)) || stable(request.run.taskIds) !== stable(previousRun.taskIds)) return runConflict("scope_change_requires_extension");
    if (request.authoritativeSource === "compatibility_migration" && previousRun.autoDeploy === false && request.run.autoDeploy === true) return runConflict("migration_cannot_enable_deployment");
    const problem = runInputProblem(request.run, canonical.tasks, { allowMigration: true });
    if (problem) return runConflict(problem);
    const run = { ...request.run, ownerSessionId: previousRun.ownerSessionId ?? options.actorSessionId, paused: previousRun.paused,
      ownershipEpoch: previousRun.ownershipEpoch ?? state.ownership?.epoch ?? 1,
      operationalVersion: (state.stateVersion ?? 0) + 1, blockers: previousRun.blockers ?? [],
      pendingDeliveryIds: previousRun.pendingDeliveryIds ?? [], deployedTaskIds: previousRun.deployedTaskIds ?? [],
      terminalClassification: previousRun.terminalClassification ?? "unknown" };
    const result = { previousRun, run: structuredClone(run), authoritativeSource: request.authoritativeSource,
      reason: request.reason, affectedTaskIds: [...request.affectedTaskIds], trackerFingerprint: canonical.tracker.fingerprint,
      deploymentHeld: request.authoritativeSource === "compatibility_migration" || run.autoDeploy === false };
    return { state: { ...state, run }, result };
  }, options);
}
```

- [ ] **Step 4: Implement classification/batch selection.**

Count unique top-level deliveries from canonical parent relationships. Exclude epics, subtasks, commits, tests, workers, and prose. Underfill only for finite_exhausted, continuous_scope_exhausted, or blocked_tail after ordinary gates.

In `canonical-state.mjs`, change the current direct return object to `const canonical = { state, registry, tasks, tracker };`, attach the fields below, and `return canonical`. Join only exact task-keyed receipts and require their revision to be the same full Git revision integrated into the loaded repository:

```js
import { execFile } from "node:child_process";
import { promisify } from "node:util";
const exec = promisify(execFile);

async function loadHeadRevision(project, budget) {
  const result = await exec("git", ["-C", project.root, "rev-parse", "HEAD"], {
    encoding: "utf8", timeout: budget?.timeout(1500) ?? 1500, ...(budget ? { signal: budget.signal } : {}),
  });
  const revision = result.stdout.trim();
  if (!/^[a-f0-9]{40}$/.test(revision)) throw new Error("invalid_head_revision");
  return revision;
}

async function loadDeliveryEvidence(project, state, gitHeadRevision, budget) {
  const isAncestor = async (ancestor, descendant) => {
    try { await exec("git", ["-C", project.root, "merge-base", "--is-ancestor", ancestor, descendant], { timeout: budget?.timeout(1500) ?? 1500 }); return true; }
    catch (error) { if (error.code === 1) return false; throw error; }
  };
  const categories = ["completion", "integration", "review", "checks", "preview", "target", "recovery"];
  const ids = new Set(categories.flatMap((category) => Object.keys(state.deliveryReceipts?.[category] ?? {})));
  const entries = [];
  for (const taskId of ids) {
    const parts = Object.fromEntries(categories.map((category) => [category, state.deliveryReceipts?.[category]?.[taskId]]));
    const sourceRevision = parts.completion?.sourceRevision;
    const boundaryRevision = parts.integration?.boundaryRevision;
    if (Object.values(parts).some((part) => !part) || ![sourceRevision, boundaryRevision].every((revision) => /^[a-f0-9]{40}$/.test(revision))
      || parts.review.revision !== sourceRevision || parts.checks.revision !== sourceRevision
      || parts.integration.sourceRevision !== sourceRevision
      || [parts.preview.revision, parts.target.revision, parts.recovery.revision].some((revision) => revision !== boundaryRevision)) continue;
    if (!await isAncestor(sourceRevision, boundaryRevision) || !await isAncestor(boundaryRevision, gitHeadRevision)) continue;
    entries.push([taskId, { taskId, sourceRevision, revision: boundaryRevision, integratedRevision: boundaryRevision,
      review: parts.review, checks: parts.checks.results, preview: parts.preview,
      target: parts.target, recovery: parts.recovery }]);
  }
  return Object.fromEntries(entries);
}
const gitHeadRevision = await loadHeadRevision(project, options.budget);
canonical.git = { headRevision: gitHeadRevision };
canonical.deliveryEvidence = await loadDeliveryEvidence(project, canonical.state, gitHeadRevision, options.budget);
```

```js
export function classifyRun(canonical, { writerLiveness = {} } = {}) {
  const run = canonical.state?.run;
  if (validateEffectiveRun(run, canonical.tasks)) return { kind: "unknown", eligibleTaskIds: [], blockedTaskIds: [] };
  if (run.paused) return { kind: "paused", eligibleTaskIds: [], blockedTaskIds: [] };
  const scoped = canonical.tasks.filter((task) => run.taskIds.includes(task.id));
  const affectedByUnknownWriter = (task) => writerLiveness[task.ownerSessionId] === "unknown" || writerLiveness[task.id] === "unknown";
  const eligibleTaskIds = scoped.filter((task) => task.status === "ready" && !task.parentId && !affectedByUnknownWriter(task)).map(({ id }) => id);
  const blockedTaskIds = [...new Set(scoped.filter((task) => (task.status === "blocked" || affectedByUnknownWriter(task)) && !task.parentId).map(({ id }) => id))];
  const unreconciled = scoped.filter((task) => task.status === "done" && !run.pendingDeliveryIds.includes(task.id) && !run.deployedTaskIds.includes(task.id));
  if (unreconciled.length) return { kind: "unreconciled_completion", eligibleTaskIds, blockedTaskIds };
  const unfinished = scoped.filter((task) => !["done", "cancelled"].includes(task.status));
  const kind = eligibleTaskIds.length ? "progress_possible" : !unfinished.length && run.mode === "finite" ? "finite_exhausted"
    : !unfinished.length ? "continuous_scope_exhausted" : blockedTaskIds.length === unfinished.length ? "blocked_tail" : "progress_possible";
  return { kind, eligibleTaskIds, blockedTaskIds };
}
export function selectReleaseBatch(canonical, classification) {
  const ready = canonical.state.run.pendingDeliveryIds.filter((id) => {
    const evidence = canonical.deliveryEvidence?.[id];
    return evidence?.taskId === id && evidence.integratedRevision === evidence.revision
      && evidence.review?.status === "passed" && evidence.checks?.length > 0 && evidence.checks.every(({ status }) => status === "passed")
      && ["passed", "not_required"].includes(evidence.preview?.status)
      && evidence.target?.status === "authorized" && evidence.recovery?.status === "ready";
  });
  const batchSize = canonical.state.run.batchSize;
  if (ready.length >= batchSize) return ready.slice(0, batchSize); // full first B of N
  return ["finite_exhausted", "continuous_scope_exhausted", "blocked_tail"].includes(classification.kind) ? ready : [];
}
```

`selectReleaseBatch` is advisory and cannot consume `state.release.recordedEvidence`. The later `gate-evidence` release mutation receives the exact selected IDs and binds them into release evidence only after this pure selection.

- [ ] **Step 5: Route closed versioned commands.**

Require native owner, expected state version/fingerprint, complete values, authoritative source, reason, and affected IDs. run-decision is read-only.

```js
export const workflowCommandFlags = Object.freeze({
  status: new Set(["project"]),
  usage: new Set(["project", "receipt", "soft-tokens", "hard-tokens"]),
  recovery: new Set(["project", "session", "task", "worktree", "stale-ms", "include-probes", "include-git"]),
  eligibility: new Set(["project"]),
  checkpoint: new Set(["project", "request"]),
  "task-transition": new Set(["project", "request"]),
  "gate-evidence": new Set(["project", "request"]),
  cleanup: new Set(["project", "request"]),
  "dashboard-snapshot": new Set(["project"]),
  "dashboard-start": new Set(["project", "port"]),
  "run-start": new Set(["project", "request"]),
  "run-reconcile": new Set(["project", "request"]),
  "run-decision": new Set(["project"]),
  "project-owner-recover": new Set(["project", "request"]),
  "evidence-store-register": new Set(["project", "request"]),
  "completion-history-reconcile": new Set(["project", "request"]),
});
if (command === "run-start") return startRun(project, envelope.request, { ...context, actorSessionId: envelope.actorSessionId, expectedVersion: envelope.expectedVersion });
if (command === "run-reconcile") return reconcileRun(project, envelope.request, { ...context, actorSessionId: envelope.actorSessionId, expectedVersion: envelope.expectedVersion });
if (command === "run-decision") return readRunDecision(await loadCanonicalState(project));
if (command === "project-owner-recover") return recoverProjectOwner(project, envelope, { nativeOwnerRecovery: context.nativeOwnerRecovery });
if (command === "evidence-store-register") return registerEvidenceStore(project, envelope.request, { ...context, actorSessionId: envelope.actorSessionId, expectedVersion: envelope.expectedVersion });
if (command === "completion-history-reconcile") return reconcileCompletionHistory(project, envelope.request, { ...context, actorSessionId: envelope.actorSessionId, expectedVersion: envelope.expectedVersion });
```

Router tests invoke all three new names. Shell calls to `project-owner-recover` return `native_owner_recovery_required`; only a native adapter holding the opaque capability can supply `nativeOwnerRecovery`. Unknown aliases including `owner-recover` and the superseded `evidence-root-register` fail as unknown commands without reading or writing canonical state.

`reconcileRun` treats a present legacy `state.run` without `id` as active-but-incomplete, not `run_not_active`; it atomically assigns only the request's valid compatibility ID and explicit effective values while preserving exact scope, pause, blockers, pending/deployed IDs and ownership epoch. It never copies setup defaults.

- [ ] **Step 6: Run GREEN.**

Run: node --test tests/hooks-run-state.test.mjs tests/hooks-transitions.test.mjs tests/hooks-project-journey.test.mjs

Expected: PASS for the twelve design run/batch cases.

- [ ] **Step 7: Commit.**

    git add hooks/lib/run-state.mjs hooks/lib/canonical-state.mjs hooks/lib/workflow-cli.mjs hooks/manifest.json tests/hooks-run-state.test.mjs tests/hooks-transitions.test.mjs
    git commit -m "feat: persist and classify effective runs"

### Task 4: Scope extension, quarantine/rebind, and subset gates (REL-004)

**Files:**
- Modify: hooks/lib/task-transitions.mjs:80-459
- Modify: hooks/lib/workflow-cli.mjs:21-32,106-147,258-295
- Modify: hooks/lib/initialization.mjs:69-90,223-280
- Modify: hooks/lib/cleanup.mjs:13-76
- Test: tests/hooks-transitions.test.mjs
- Test: tests/hooks-initialization.test.mjs
- Test: tests/hooks-cleanup.test.mjs
- Test: tests/hooks-project-journey.test.mjs
- Test: tests/hooks-setup-cli.test.mjs

**Interfaces:**
- Consumes: REL-001 immutable snapshot and REL-003 run validation.
- Produces: extendRunScope, quarantineCompletion, rebindCompletion, `registerEvidenceStore`, and `reconcileCompletionHistory`; commands run-scope-extend, completion-quarantine, completion-rebind, evidence-store-register, and completion-history-reconcile; scopeExtensions, evidenceStores, completionHistory, and quarantinedEvidence histories; task-keyed `state.deliveryReceipts.completion[taskId]` and `.integration[taskId]` records consumed by REL-003 selection. Existing singleton compatibility records may remain read-only migration input but never drive a new batch.

Closed bodies copied from `/tmp/at-720-exact-contracts.md` (the shared envelope supplies actorSessionId and expectedVersion):

```json
{"operationId":"brainvault-migrate-6zf-scope-1","expectedTrackerFingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","taskIds":["brainvault-system-6zf"],"reason":"Admit the already tracked reviewed delivery into the active run."}
{"operationId":"brainvault-migrate-6zf-quarantine-1","taskId":"brainvault-system-6zf","reason":"task_outside_admitted_run_scope","expectedEvidence":{"path":".agent-team/evidence/brainvault-system-6zf/completion.json","fingerprint":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","revision":"cccccccccccccccccccccccccccccccccccccccc","taskIds":["brainvault-system-6zf"]}}
{"operationId":"brainvault-migrate-6zf-rebind-1","taskId":"brainvault-system-6zf","quarantineId":"completion:brainvault-system-6zf:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","expectedTrackerFingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","evidencePath":".agent-team/evidence/brainvault-system-6zf/completion.json","expectedEvidenceFingerprint":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","expectedSourceRevision":"cccccccccccccccccccccccccccccccccccccccc","expectedBoundaryRevision":"dddddddddddddddddddddddddddddddddddddddd"}
```

- [ ] **Step 1: Write failing migration tests.**

Create a 33-ID initialization fixture, later tracker ID brainvault-system-6zf, and invalid outside-scope completion receipt. Cover exact replay, changed replay, stale version/fingerprint, pause, missing row, disguised removal, evidence substitution, and crash boundaries.

```js
test("scope extension is strict-additive and replay safe", async () => {
  const request = { operationId: "extend-6zf", expectedTrackerFingerprint: trackerFingerprint, reason: "admit reviewed tracker addition", taskIds: ["brainvault-system-6zf"] };
  const options = { nativeIdentity, actorSessionId: owner, expectedVersion: 7 };
  const applied = await extendRunScope(project, request, options);
  assert.equal(applied.status, "applied");
  assert.deepEqual(applied.result.addedTaskIds, ["brainvault-system-6zf"]);
  assert.deepEqual(applied.result.previousTaskIds, initialTaskIds);
  assert.deepEqual(applied.result.taskIds, [...initialTaskIds, "brainvault-system-6zf"]);
  assert.equal(applied.result.ownerSessionId, owner);
  assert.equal(applied.result.trackerFingerprint, trackerFingerprint);
  assert.equal((await extendRunScope(project, request, options)).status, "duplicate");
  assert.equal((await extendRunScope(project, { ...request, taskIds: ["other"] }, options)).reason, "operation_identity_reused");
});

test("distinct task sources bind to one shared integration boundary", () => {
  const sourceA = "a".repeat(40);
  const sourceB = "b".repeat(40);
  const boundaryRevision = "c".repeat(40);
  const state = { deliveryReceipts: {} };
  for (const [taskId, sourceRevision] of [["T-1", sourceA], ["T-2", sourceB]]) {
    bindTaskDeliveryReceipts(state, "completion", [taskId], sourceRevision,
      { review: { status: "passed" }, checks: [{ name: "unit", status: "passed" }] },
      { path: `.agent-team/evidence/${taskId}.json`, fingerprint: (taskId === "T-1" ? "d" : "e").repeat(64) });
  }
  bindTaskDeliveryReceipts(state, "integration", ["T-1", "T-2"], boundaryRevision,
    { sourceRevisions: { "T-1": sourceA, "T-2": sourceB }, preview: { required: false },
      targetAuthorization: { status: "authorized", target: "refs/heads/main" },
      recovery: { artifactId: "batch-1", action: "revert" } },
    { path: ".agent-team/evidence/batch-1.json", fingerprint: "f".repeat(64) });
  assert.deepEqual(state.deliveryReceipts.integration["T-1"], {
    taskId: "T-1", sourceRevision: sourceA, boundaryRevision, status: "passed", integratedRevision: boundaryRevision,
    evidencePath: ".agent-team/evidence/batch-1.json", evidenceFingerprint: "f".repeat(64),
  });
  assert.deepEqual(state.deliveryReceipts.integration["T-2"], {
    taskId: "T-2", sourceRevision: sourceB, boundaryRevision, status: "passed", integratedRevision: boundaryRevision,
    evidencePath: ".agent-team/evidence/batch-1.json", evidenceFingerprint: "f".repeat(64),
  });
  assert.equal(state.deliveryReceipts.completion["T-1"].sourceRevision, sourceA);
  assert.equal(state.deliveryReceipts.completion["T-2"].sourceRevision, sourceB);
});

async function commitFixture(root, relativePath, contents, message) {
  await mkdir(path.dirname(path.join(root, relativePath)), { recursive: true });
  await writeFile(path.join(root, relativePath), contents);
  await git(root, ["add", relativePath]);
  await git(root, ["commit", "-m", message]);
  return (await git(root, ["rev-parse", "HEAD"])).trim();
}
async function seedQuarantineAndIntegration(project, { taskId, sourceRevision, boundaryRevision, evidencePath }) {
  const evidence = { schemaVersion: 1, taskId, taskIds: [taskId], revision: sourceRevision, status: "passed",
    requirementsReconciled: true, review: { status: "passed" }, checks: [{ name: "unit", status: "passed" }] };
  const bytes = Buffer.from(`${JSON.stringify(evidence)}\n`);
  await mkdir(path.dirname(path.join(project.root, evidencePath)), { recursive: true });
  await writeFile(path.join(project.root, evidencePath), bytes);
  const evidenceFingerprint = createHash("sha256").update(bytes).digest("hex");
  const quarantineId = `completion:${taskId}:${evidenceFingerprint}`;
  const statePath = path.join(project.root, ".agent-team", "state.json");
  const state = JSON.parse(await readFile(statePath, "utf8"));
  state.run.taskIds = [...new Set([...state.run.taskIds, taskId])];
  state.quarantinedEvidence = [{ quarantineId, evidence: { path: evidencePath, fingerprint: evidenceFingerprint,
    revision: sourceRevision, taskIds: [taskId] }, ownerSessionId: state.project.ownerSessionId }];
  state.deliveryReceipts ??= {};
  state.deliveryReceipts.integration = { ...(state.deliveryReceipts.integration ?? {}),
    [taskId]: { taskId, sourceRevision, boundaryRevision, status: "passed", integratedRevision: boundaryRevision } };
  await writeFile(statePath, `${JSON.stringify(state, null, 2)}\n`);
  return { evidenceFingerprint, quarantineId };
}
test("completion rebind permits HEAD to advance beyond the integration boundary", async () => {
  const sourceRevision = await commitFixture(root, "src/task.txt", "source\n", "task source");
  const boundaryRevision = await commitFixture(root, "integration.txt", "boundary\n", "integration boundary");
  const currentRevision = await commitFixture(root, "later.txt", "later\n", "advance main");
  assert.notEqual(sourceRevision, currentRevision);
  assert.notEqual(boundaryRevision, currentRevision);
  const { evidenceFingerprint, quarantineId } = await seedQuarantineAndIntegration(project,
    { taskId: "T-1", sourceRevision, boundaryRevision, evidencePath });
  const result = await rebindCompletion(project, { operationId: "rebind-advanced-head", taskId: "T-1", quarantineId,
    expectedTrackerFingerprint: trackerFingerprint, evidencePath, expectedEvidenceFingerprint: evidenceFingerprint,
    expectedSourceRevision: sourceRevision, expectedBoundaryRevision: boundaryRevision }, options);
  assert.equal(result.status, "applied");
  assert.deepEqual(result.result, { taskId: "T-1", quarantineId, sourceRevision, boundaryRevision,
    currentRevision, evidenceFingerprint, trackerFingerprint, rebound: true });
});
```

Add sibling RED cases where source is not an ancestor of boundary and boundary is not an ancestor of current HEAD; both return `invalid_completion_lineage` with byte-identical state. The fixture helper is test-only; production writes still go exclusively through `mutateOperationalState`.

- [ ] **Step 2: Write failing subset tests.**

Submit an existing tracker ID outside state.run.taskIds to completion, integration, release, and cleanup. Assert unchanged state/evidence and no cleanup probe/removal.

```js
for (const gate of ["completion", "integration", "release"]) {
  const before = await readFile(statePath);
  const result = await recordGateEvidence(project, { gate, taskIds: ["outside-scope"], operationId: `reject-${gate}` }, options);
  assert.equal(result.reason, "outside_scope");
  assert.deepEqual(await readFile(statePath), before);
}
let cleanupProbes = 0;
const cleanup = await cleanupDevelopmentWorktree(project, { taskId: "outside-scope" }, { ...options, git: async () => { cleanupProbes += 1; } });
assert.equal(cleanup.reason, "outside_scope");
assert.equal(cleanupProbes, 0);
```

- [ ] **Step 3: Run RED.**

Run: node --test --test-name-pattern='scope|quarantine|rebind|outside.*scope|tracker addition|shared integration boundary' tests/hooks-transitions.test.mjs tests/hooks-initialization.test.mjs tests/hooks-cleanup.test.mjs tests/hooks-project-journey.test.mjs

Expected: FAIL because the transitions do not exist and gates accept tracker membership without run membership.

- [ ] **Step 4: Decouple readiness.**

Keep exact tracker snapshot checks at adoption. After initialization, validate receipt/selector and require all admitted IDs to remain in the live tracker; allow unrelated tracker additions.

- [ ] **Step 5: Implement extension.**

Under mutateOperationalState and a fresh tracker read, require strict additive existing IDs with resolved dependencies. Record operationId, ownerSessionId, reason, previousTaskIds, added taskIds, newTaskIds, trackerFingerprint, and appliedAt.

```js
export async function extendRunScope(project, request, options = {}) {
  const now = options.now ?? (() => new Date().toISOString());
  const effectiveRequest = { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion };
  return mutateOperationalState(project, effectiveRequest, async (state) => {
    const canonical = await loadCanonicalState(project, { includeTasks: true, budget: options.budget });
    if (state.run.paused) return { status: "conflict", reason: "paused" };
    if (canonical.tracker.status !== "current") return { status: "conflict", reason: "tracker_unavailable" };
    if (request.expectedTrackerFingerprint !== canonical.tracker.fingerprint) return { status: "conflict", reason: "stale_tracker" };
    const previousTaskIds = [...state.run.taskIds];
    if (!request.taskIds.length || new Set(request.taskIds).size !== request.taskIds.length
      || request.taskIds.some((id) => previousTaskIds.includes(id) || !canonical.tasks.some((task) => task.id === id))) return { status: "conflict", reason: "scope_extension_not_strict_addition" };
    const taskIds = [...previousTaskIds, ...request.taskIds];
    if (canonical.tasks.filter(({ id }) => request.taskIds.includes(id)).some((task) => (task.dependencies ?? []).some((id) => !taskIds.includes(id)))) return { status: "conflict", reason: "unresolved_scope_dependency" };
    const extension = { operationId: request.operationId, previousTaskIds, addedTaskIds: [...request.taskIds], taskIds,
      reason: request.reason, ownerSessionId: options.actorSessionId, trackerFingerprint: canonical.tracker.fingerprint, appliedAt: now() };
    return { state: { ...state, run: { ...state.run, taskIds, scopeExtensions: [...(state.run.scopeExtensions ?? []), extension] } },
      result: { previousTaskIds, addedTaskIds: [...request.taskIds], taskIds, reason: request.reason,
        ownerSessionId: options.actorSessionId, trackerFingerprint: canonical.tracker.fingerprint } };
  }, options);
}
```

- [ ] **Step 6: Implement quarantine and rebind.**

Quarantine preserves exact receipt/pointers in immutable history and clears only active consideration atomically. Rebind requires the quarantined identity, admitted task, exact evidence revision/fingerprints, and valid review/checks.

```js
async function readValidatedCompletionEvidence(project, evidencePath) {
  if (typeof evidencePath !== "string" || !evidencePath || evidencePath.length > 256 || path.isAbsolute(evidencePath)
    || evidencePath.split(/[\\/]/).includes("..") || /[\r\n\0|]/.test(evidencePath)) throw new Error("unsafe_evidence_path");
  const file = path.join(project.root, evidencePath);
  const metadata = await lstat(file);
  if (!metadata.isFile() || metadata.isSymbolicLink() || metadata.size > 256 * 1024) throw new Error("unsafe_evidence_file");
  const bytes = await readFile(file);
  return { parsed: JSON.parse(bytes), fingerprint: createHash("sha256").update(bytes).digest("hex") };
}
function completionIdentity(record) {
  return record && { path: record.path, fingerprint: record.fingerprint, revision: record.revision, taskIds: record.taskIds };
}
async function readCleanHead(project, options = {}) {
  const git = options.git ?? ((args) => run("git", args, { cwd: project.root, encoding: "utf8", timeout: 1500 }));
  const output = (result) => typeof result === "string" ? result : result.stdout;
  const revision = output(await git(["rev-parse", "HEAD"])).trim();
  const dirty = output(await git(["status", "--porcelain"]));
  return { revision, clean: dirty.length === 0 };
}
export const quarantineCompletion = (project, request, options = {}) => mutateOperationalState(project,
  { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion }, async (state) => {
  const canonical = await loadCanonicalState(project, { includeTasks: true, budget: options.budget });
  if (state.run.paused) return { status: "conflict", reason: "paused" };
  if (canonical.tracker.status !== "current") return { status: "conflict", reason: "tracker_unavailable" };
  if (request.reason !== "task_outside_admitted_run_scope" || stable(request.expectedEvidence) !== stable(completionIdentity(state.completion.recordedEvidence))) return { status: "conflict", reason: "completion_evidence_changed" };
  if (state.run.taskIds.includes(request.taskId) || !canonical.tasks.some(({ id }) => id === request.taskId)) return { status: "conflict", reason: "quarantine_scope_mismatch" };
  const observed = await readValidatedCompletionEvidence(project, request.expectedEvidence.path);
  if (observed.fingerprint !== request.expectedEvidence.fingerprint || observed.parsed.revision !== request.expectedEvidence.revision
    || !sameIds(observed.parsed.taskIds ?? [observed.parsed.taskId], [request.taskId])) return { status: "conflict", reason: "completion_evidence_changed" };
  const quarantineId = `completion:${request.taskId}:${request.expectedEvidence.fingerprint}`;
  const completionReceipts = { ...(state.deliveryReceipts?.completion ?? {}) };
  delete completionReceipts[request.taskId];
  return { state: { ...state, completion: { ...state.completion, recordedEvidence: undefined },
    deliveryReceipts: { ...state.deliveryReceipts, completion: completionReceipts },
    quarantinedEvidence: [...(state.quarantinedEvidence ?? []), { quarantineId, evidence: structuredClone(state.completion.recordedEvidence),
      ownerSessionId: state.completion.ownerSessionId, operationId: request.operationId, reason: request.reason,
      quarantinedAt: (options.now ?? (() => new Date().toISOString()))() }] },
    result: { quarantineId, taskId: request.taskId, reason: request.reason, evidence: request.expectedEvidence, activeCompletionCleared: true } };
}, options);
export const rebindCompletion = (project, request, options = {}) => mutateOperationalState(project,
  { ...request, actorSessionId: options.actorSessionId, expectedVersion: options.expectedVersion }, async (state) => {
  const canonical = await loadCanonicalState(project, { includeTasks: true, budget: options.budget });
  const quarantined = state.quarantinedEvidence?.find(({ quarantineId }) => quarantineId === request.quarantineId);
  if (state.run.paused) return { status: "conflict", reason: "paused" };
  const requestedIdentity = { path: request.evidencePath, fingerprint: request.expectedEvidenceFingerprint,
    revision: request.expectedSourceRevision, taskIds: [request.taskId] };
  if (!quarantined || quarantined.reboundByOperationId || !state.run.taskIds.includes(request.taskId)
    || stable(completionIdentity(quarantined.evidence)) !== stable(requestedIdentity)) return { status: "conflict", reason: "invalid_quarantine_identity" };
  const observed = await readValidatedCompletionEvidence(project, request.evidencePath);
  const evidence = observed.parsed;
  const repository = await readCleanHead(project, options);
  const integration = state.deliveryReceipts?.integration?.[request.taskId];
  if (observed.fingerprint !== request.expectedEvidenceFingerprint || evidence.revision !== request.expectedSourceRevision
    || !sameIds(evidence.taskIds ?? [evidence.taskId], [request.taskId]) || evidence.status !== "passed" || evidence.requirementsReconciled !== true
    || evidence.review?.status !== "passed" || !evidence.checks?.length || evidence.checks.some(({ status }) => status !== "passed")
    || canonical.tracker.status !== "current" || canonical.tracker.fingerprint !== request.expectedTrackerFingerprint
    || integration?.status !== "passed" || integration.sourceRevision !== request.expectedSourceRevision
    || integration.boundaryRevision !== request.expectedBoundaryRevision || integration.integratedRevision !== request.expectedBoundaryRevision
    || !repository.clean) return { status: "conflict", reason: "completion_evidence_changed" };
  if (!await isAncestor(project, request.expectedSourceRevision, request.expectedBoundaryRevision, options)
    || !await isAncestor(project, request.expectedBoundaryRevision, repository.revision, options)) {
    return { status: "conflict", reason: "invalid_completion_lineage" };
  }
  const rebound = { ...requestedIdentity, observedAt: (options.now ?? (() => new Date().toISOString()))() };
  const completionReceipt = { taskId: request.taskId, sourceRevision: request.expectedSourceRevision, status: "passed",
    evidencePath: request.evidencePath, evidenceFingerprint: request.expectedEvidenceFingerprint };
  return { state: { ...state, completion: { ...state.completion, recordedEvidence: rebound },
    deliveryReceipts: { ...state.deliveryReceipts, completion: { ...state.deliveryReceipts?.completion, [request.taskId]: completionReceipt } },
    quarantinedEvidence: state.quarantinedEvidence.map((item) => item.quarantineId === request.quarantineId
      ? { ...item, reboundByOperationId: request.operationId, reboundAt: rebound.observedAt } : item) },
    result: { taskId: request.taskId, quarantineId: request.quarantineId, sourceRevision: request.expectedSourceRevision,
      boundaryRevision: request.expectedBoundaryRevision, currentRevision: repository.revision,
      evidenceFingerprint: request.expectedEvidenceFingerprint, trackerFingerprint: canonical.tracker.fingerprint, rebound: true } };
}, options);
```

`isAncestor(project, ancestor, descendant, options)` executes `git -C project.root merge-base --is-ancestor <ancestor> <descendant>` with the existing bounded Git runner and returns true only for exit status zero. Rebind deliberately never requires `sourceRevision === HEAD` or `boundaryRevision === HEAD`: the required chain is source → integration boundary → current clean HEAD, and ordinary completion-gate validation remains the source-evidence validator.

- [ ] **Step 7: Enforce scope before side effects.**

Use an admitted Set before evidence/Git reads and cleanup probes; return outside_scope on any missing ID.

```js
function requireAdmittedTaskIds(state, taskIds) {
  const admitted = new Set(state.run.taskIds);
  return taskIds.every((id) => admitted.has(id)) ? undefined : "outside_scope";
}
```

After the existing gate-specific validator has accepted the source evidence, the same `mutateOperationalState` write creates the task-keyed inputs used by `loadDeliveryEvidence`. Completion produces completion/review/checks; integration produces integration/preview/target/recovery. Extend integration evidence with a closed `targetAuthorization` object and require `status:"authorized"`, exact `revision`, exact `taskIds`, exact `target === evidence.remote.targetRef`, the integration owner, and its explicit authority source before writing a target receipt. The existing integration-scoped `authorization` object is not target authority and cannot be reused as one. No status prose or later aggregate release receipt can populate these records.

```js
function bindTaskDeliveryReceipts(state, gate, taskIds, revision, evidence, recordedEvidence) {
  state.deliveryReceipts ??= {};
  const put = (category, taskId, value) => {
    state.deliveryReceipts[category] = { ...(state.deliveryReceipts[category] ?? {}), [taskId]: { taskId, ...value } };
  };
  for (const taskId of taskIds) {
    if (gate === "completion") {
      const sourceRevision = revision;
      put("completion", taskId, { sourceRevision, status: "passed", evidencePath: recordedEvidence.path, evidenceFingerprint: recordedEvidence.fingerprint });
      put("review", taskId, { revision: sourceRevision, status: evidence.review.status });
      put("checks", taskId, { revision: sourceRevision, results: evidence.checks.map(({ name, status }) => ({ name, status })) });
    }
    if (gate === "integration") {
      const sourceRevision = evidence.sourceRevisions?.[taskId];
      const recordedSource = state.deliveryReceipts.completion?.[taskId]?.sourceRevision;
      if (!/^[a-f0-9]{40}$/.test(sourceRevision) || sourceRevision !== recordedSource) throw new Error("integration_source_revision_mismatch");
      const boundaryRevision = revision;
      put("integration", taskId, { sourceRevision, boundaryRevision, status: "passed", integratedRevision: boundaryRevision,
        evidencePath: recordedEvidence.path, evidenceFingerprint: recordedEvidence.fingerprint });
      put("preview", taskId, { revision: boundaryRevision, status: evidence.preview.required ? "passed" : "not_required" });
      put("target", taskId, { revision: boundaryRevision, status: evidence.targetAuthorization.status, target: evidence.targetAuthorization.target,
        authority: structuredClone(evidence.targetAuthorization) });
      put("recovery", taskId, { revision: boundaryRevision, status: "ready", artifactId: evidence.recovery.artifactId,
        action: evidence.recovery.action });
    }
  }
}
```

Construct `recordedEvidence` once, call `bindTaskDeliveryReceipts(state, request.gate, request.taskIds, revision, evidence, recordedEvidence)`, and only then assign the same object to `state[request.gate].recordedEvidence`. Tests must delete or alter each category independently and prove `selectReleaseBatch` rejects the task.

- [ ] **Step 8: Route closed request envelopes.**

Reject unknown fields, FIFO/nonregular/oversized input, stale values, native-owner mismatch, and changed-signature replay before mutation.

Add closed `evidence-store-register` fields `{operationId,storeId,realpath,expectedTeamsFingerprint,declaration,projectId,expectedOwnershipEpoch}` and closed `completion-history-reconcile` fields `{operationId,taskId,intent,completionOperationId,completionResultFingerprint,evidenceStoreId,completionRelativePath,completionSha256,sourceRevision,integrationOperationId,integrationRelativePath,integrationSha256,boundaryRevision,expectedTrackerFingerprint,expectedStateFingerprint,expectedRunFingerprint,expectedTeamsFingerprint,expectedOwnershipEpoch,expectedActiveCompletion,publicationTarget,observedRemoteRevision}`. `intent` is exactly `admit_and_record|record_existing_scope|decline_admission`. Both commands use the native qualified owner/epoch and fresh locked reads. An explicit router test invokes `evidence-root-register` and requires `unknown_command` plus four-record byte identity.

`registerEvidenceStore` verifies the exact fingerprinted TEAMS declaration, realpath/uid/directory, no symlink component, and stores the registration. `reconcileCompletionHistory` opens only normalized relative regular files beneath the registered store with no-follow/size/hash/re-containment checks, resolves the historical completion/integration operations, preserves an unrelated active singleton, and records source/boundary receipts only when `sourceRevision` is an ancestor of `boundaryRevision` and the boundary is an ancestor of HEAD. If authorized origin/main already contains the boundary, add the task to deployed IDs rather than pending IDs. Add runnable BrainVault-shaped tests for 6zf and 0o4 plus n8s preservation, store escape/symlink/uid/mode drift, source/boundary non-ancestry, active singleton mismatch, pending-operation/writer/owner-epoch drift, exact replay and decline intent.

- [ ] **Step 9: Run GREEN.**

Run: node --test tests/hooks-transitions.test.mjs tests/hooks-initialization.test.mjs tests/hooks-cleanup.test.mjs tests/hooks-project-journey.test.mjs tests/hooks-setup-cli.test.mjs

Expected: PASS with replay/crash safety and unchanged initialization provenance.

- [ ] **Step 10: Commit.**

    git add hooks/lib/task-transitions.mjs hooks/lib/workflow-cli.mjs hooks/lib/initialization.mjs hooks/lib/cleanup.mjs tests/hooks-transitions.test.mjs tests/hooks-initialization.test.mjs tests/hooks-cleanup.test.mjs tests/hooks-project-journey.test.mjs tests/hooks-setup-cli.test.mjs
    git commit -m "feat: add scoped migration transitions"

### Task 5: Policy, status, recovery, and target-scoped holds (REL-005)

**Files:**
- Modify: hooks/lib/policy.mjs:155-282,398-417
- Modify: hooks/lib/status.mjs:38-65,134-225
- Modify: hooks/lib/recovery.mjs:94-230
- Test: tests/hooks-policy.test.mjs
- Test: tests/hooks-status.test.mjs
- Test: tests/hooks-recovery.test.mjs
- Test: tests/hooks-project-journey.test.mjs

**Interfaces:**
- Consumes: REL-003 run validation/classification/batch selection and REL-004 admitted scope/quarantine history.
- Produces: policy subset defense; status effective-run snapshot and four provenance labels; recovery workers/slots/blockers/decisions/tails/next-dispatch fields; validation of REL-004 task-keyed delivery receipts and target-keyed authority/holds; and the exact migration-readable fields `.stateVersion`, `.tracker.fingerprint`, `.run.fingerprint`, and `.project.ownerSessionId`. The later release gate receives `selectedTaskIds` from REL-003 and atomically binds exactly those IDs into `state.release.recordedEvidence`; selection never consumes that aggregate receipt.

- [ ] **Step 1: Write failing defense tests.**

Corrupt completion/integration/release evidence with an out-of-scope ID and assert actual policy operations reject. Add simultaneous authorized origin/main publication and held Vercel/database production.

```js
test("publication authority does not clear production holds", () => {
  const result = releaseGate(canonical({ runTaskIds: ["T-1"], releaseTaskIds: ["T-1"], authority: { "origin/main": "authorized" }, holds: { vercel: "target_required", database: "target_required" } }));
  assert.equal(result.targets["origin/main"].ready, true);
  assert.equal(result.targets.vercel.reason, "target_required");
  assert.equal(result.targets.database.reason, "target_required");
});
```

- [ ] **Step 2: Write failing status/recovery tests.**

Require teamLimit, autoDeploy, batchSize, source, terminal classification, pending delivery IDs, local_only, enabled_but_held: target_required, the four provenance labels, active worker/slot inventory, scoped blockers, pending decision, uncertainty, tail, and next eligible dispatch.

```js
test("status exposes exact migration snapshot keys without mutation", async () => {
  const before = await readFile(project.paths.state);
  const status = await readStatus(project);
  assert.equal(status.stateVersion, 18);
  assert.match(status.tracker.fingerprint, /^[a-f0-9]{64}$/);
  assert.match(status.run.fingerprint, /^[a-f0-9]{64}$/);
  assert.equal(status.project.ownerSessionId, "native-owner-session");
  assert.deepEqual(await readFile(project.paths.state), before);
});
```

- [ ] **Step 3: Run RED.**

Run: node --test tests/hooks-policy.test.mjs tests/hooks-status.test.mjs tests/hooks-recovery.test.mjs

Expected: FAIL because policy checks tracker membership only and status/recovery omit effective run and target distinctions.

- [ ] **Step 4: Implement policy run/scope checks.**

Require every evidence ID in admitted scope and a valid effective run. Unknown run state holds release only.

```js
const scopeProblem = validateEffectiveRun(canonical.state.run, canonical.tasks)
  ?? requireAdmittedTaskIds(canonical.state, evidence.taskIds);
if (scopeProblem) return hold(scopeProblem);
```

- [ ] **Step 5: Implement target-keyed holds.**

Ensure a production hold cannot erase/block authorized non-deploying origin/main publication and publication authority cannot authorize production.

- [ ] **Step 6: Extend read-only status/recovery.**

Reuse pure run-state helpers. Never reconcile, refill, checkpoint, dispatch, install, or release from status.

```js
return {
  ...model,
  stateVersion: canonical.state.stateVersion,
  tracker: { ...model.tracker, fingerprint: canonical.tracker.fingerprint },
  run: { ...model.run, fingerprint: createHash("sha256").update(stable(canonical.state.run)).digest("hex") },
  project: { ...model.project, ownerSessionId: canonical.registry.projectOwner },
};
```

- [ ] **Step 7: Run GREEN.**

Run: node --test tests/hooks-policy.test.mjs tests/hooks-status.test.mjs tests/hooks-recovery.test.mjs tests/hooks-project-journey.test.mjs

Expected: PASS with zero read-only mutation.

- [ ] **Step 8: Commit.**

    git add hooks/lib/policy.mjs hooks/lib/status.mjs hooks/lib/recovery.mjs tests/hooks-policy.test.mjs tests/hooks-status.test.mjs tests/hooks-recovery.test.mjs tests/hooks-project-journey.test.mjs
    git commit -m "feat: enforce run scope across status and policy"

### Task 6: Reuse unowned skills and canonicalize ast-grep (REL-006)

**Files:**
- Modify: hooks/lib/dependency-catalog.mjs:1-118
- Modify: hooks/lib/dependencies.mjs:16-82,158-540,699-842,1129-1197
- Test: tests/hooks-dependencies.test.mjs
- Test: tests/hooks-dependencies-real.test.mjs
- Test: tests/hooks-lean-ctx-install.test.mjs
- Test: tests/hooks-install-health.test.mjs

**Interfaces:**
- Produces: lifecycleOwnership managed|unowned; installed reused_unowned; compatibility and explicit prerequisites fields; absolute canonical ast-grep selection.

- [ ] **Step 1: Write failing unowned-reuse tests.**

Cover exact no-sidecar skill, required files plus unrelated file, edited/missing file, symlink/special file, conflicting entrypoint, no writes, functional/fresh-worker continuation, and rollback/uninstall preservation.

```js
test("compatible skill without sidecar is reused unowned", async () => {
  const before = await treeDigest(existingSkill);
  const result = await qualifyDependency({ ...leanCtx, installPath: existingSkill }, { workerDiscovery });
  assert.equal(result.status, "passed");
  assert.equal(result.installed, "reused_unowned");
  assert.equal(result.lifecycleOwnership, "unowned");
  assert.equal(await treeDigest(existingSkill), before);
});
```

- [ ] **Step 2: Write failing independence/binary tests.**

Fail LeanCTX in probe/function/worker phases and require unchanged Graphify receipt. Require absolute ast-grep, reject /usr/bin/sg, and accept sibling alias only within verified @ast-grep/cli 0.45.3 root.

- [ ] **Step 3: Run RED.**

Run: node --test tests/hooks-dependencies.test.mjs tests/hooks-lean-ctx-install.test.mjs tests/hooks-install-health.test.mjs

Expected: FAIL because sidecar absence is customized/cannot_use and the catalog selects sg.

- [ ] **Step 4: Separate compatibility from ownership.**

Verify pinned required paths/hashes for LeanCTX, Superpowers, Ponytail, Impeccable guidance, and React guidance. Allow unrelated regular files. Compatible unowned paths skip copy/sidecar/install/remove and continue functional/worker checks.

```js
if (!sidecar && await compatibleRequiredFiles(candidate, dependency.compatibility)) {
  await functionalCheck(candidate);
  await freshWorkerCheck(candidate);
  return { status: "passed", installed: "reused_unowned", lifecycleOwnership: "unowned", path: candidate };
}
```

- [ ] **Step 5: Emit prerequisite edges independently.**

Derive blocking only from catalog prerequisites. Graphify depends on uv only.

- [ ] **Step 6: Canonicalize ast-grep.**

Change the catalog command to ast-grep and bind workers to its absolute managed path. Contain optional alias normalization within verified package realpaths.

- [ ] **Step 7: Run GREEN including real dependencies.**

Run: node --test tests/hooks-dependencies.test.mjs tests/hooks-dependencies-real.test.mjs tests/hooks-lean-ctx-install.test.mjs tests/hooks-install-health.test.mjs

Expected: PASS with no reused path changes and independent Graphify evidence.

- [ ] **Step 8: Commit.**

    git add hooks/lib/dependency-catalog.mjs hooks/lib/dependencies.mjs tests/hooks-dependencies.test.mjs tests/hooks-dependencies-real.test.mjs tests/hooks-lean-ctx-install.test.mjs tests/hooks-install-health.test.mjs
    git commit -m "feat: reuse compatible skills without ownership"

### Task 7: Bind Agent-Team installs to release artifacts (REL-007)

**Accepted base:** `38b6ff5b2bc63f0b308fec9a555d6eca59c053e4`
**Current review candidate:** `a336ce07bb61087f0cafc5e08d5f5303ca4427d6`

**Files:**
- Modify: hooks/lib/artifacts.mjs:110-196
- Modify: hooks/lib/install.mjs:485-788,791-966
- Modify: hooks/lib/health.mjs:84-122
- Modify: hooks/agent-team-cli.mjs:20-99
- Test: tests/hooks-artifacts.test.mjs
- Test: tests/hooks-install-health.test.mjs
- Test: tests/hooks-package.test.mjs
- Test: tests/hooks-cli.test.mjs

**Interfaces:**
- Consumes: verified archive path, SHA256SUMS, embedded .agent-team-source.json, host codex|claude-code|both, scope user|project.
- Produces: install options --archive and --checksums; receipt artifact fields releaseTag, releaseUrl, sourceRevision, archiveSha256, archiveContentDigest, archiveFileMap, and per-host installedFileMaps; transaction/recovery identity independent of temporary extraction.
- Private helpers defined and directly unit-tested in this task: artifact helpers `parseSha256Sums(source)`, `readArchiveOnce(path) -> { archiveName, bytes }`, `inspectArchiveClosedBytes(bytes, { prefix, metadata })`, and `verifyReleaseArtifactBytes({ archiveName, bytes, checksums })`; install helpers `withInstallLock(receiptPath, callback)`, `extractVerifiedArchiveBytesToSameFilesystem(bytes, targetParent, artifact)`, `fileMap(root)`, `fileMapDigest(map)`, and `installBothFromSealedTree(sealed, targets, options)`. Each file-map entry is exactly `{sha256,mode,size}`. `readArchiveOnce` opens the caller path once with no-follow regular-file checks, reads bounded bytes, verifies a stable pre/post stat identity, and closes it; no later helper receives that replaceable path. `inspectArchiveClosedBytes` rejects duplicate names, unsafe paths, unexpected directories, symlinks, and special entries before deriving the canonical archive map. The extraction helper creates every regular file exclusively with its canonical mode in an owner-only transaction directory. The installer preflights every selected target before mutation and admits only absent targets or exact byte/mode/size-identical no-target-mutation reinstalls.

**Feasibility ruling:** Node 24's standard filesystem API has no inode-conditional directory replace. REL-007 therefore does not automatically replace a differing present package target. Any differing target, including an otherwise-owned or schema-3 target, returns `update_requires_manual_replacement`, `changed: false`, before any target, role, configuration, backup, or receipt mutation. A later release update is an explicit quiesced maintenance operation outside this automatic command: retain the verified old receipt, move the old target to a rollback backup, invoke the fresh absent-target installer, and restore the backup if the fresh transaction does not commit. Exact identical reinstall performs no target mutation. Fresh absent targets, including explicit `both`, retain journaled all-or-restored transaction behavior.

**Platform ruling:** Qualify artifact installation only on Linux and WSL where `/proc/self/fd/<directory-fd>/<child>` traversal works. Probe that capability read-only before the install lock; unsupported platforms or missing/nonfunctional descriptor roots return `unsupported_platform` before any mutation. After exclusive reservation, every package write, copy, and verification uses the open directory-descriptor root and never the canonical replaceable target pathname. A late rename/replacement therefore receives no installer bytes. Final canonical-name/descriptor and exact-map mismatch prevents roles, configuration, and receipt publication; clean the invocation-owned descriptor tree only when its identity is still proven, otherwise retain durable recovery evidence for the moved namespace. REL-009 must disclose the Linux/WSL qualification and `unsupported_platform` behavior in README and release installation documentation.

- [ ] **Step 1: Write failing provenance tests.**

Cover missing/tampered metadata, checksum mismatch, duplicate/unsafe/symlink/special entries, wrong mode/size, extraction deletion, exact byte/mode/size-identical reinstall without target mutation, `update_requires_manual_replacement` and zero mutations for differing present owned/schema-3/unowned targets, downgrade denial, installed drift, uninstall, injected fresh second-host failure/rollback, archive/checksum replacement after precheck, staged-byte mutation before publication, unsupported platform/descriptor-root behavior, and a late canonical-name replacement while the reserved descriptor remains open. Prove every reserved-tree write/copy/read uses the descriptor root and the operator replacement remains unpolluted. Preserve review-round-3 findings 2 and 3: schema-4 health rejects semantically invalid versions/checksum digests, unequal archive/package maps, and a forged metadata entry; a final-map mismatch never removes same-inode content changed after verification and instead retains the operator bytes plus durable recovery/old-backup evidence.

```js
const originalEntries = await readZip(validArchive);
const duplicateArchive = path.join(temp, "duplicate.zip");
await writeZip(duplicateArchive, [...originalEntries, { ...originalEntries[0] }]);
await assert.rejects(verifyReleaseArtifact({ archive: duplicateArchive, checksums: await sumsFor(duplicateArchive) }), /duplicate_archive_entry/);

for (const [label, mutate, reason] of [
  ["mode", (entry) => ({ ...entry, mode: 0o777 }), /archive_mode_mismatch/],
  ["size", (entry) => ({ ...entry, declaredSize: entry.data.length + 1 }), /archive_size_mismatch/],
]) {
  const changed = path.join(temp, `${label}.zip`);
  await writeZip(changed, originalEntries.map((entry, index) => index === 0 ? mutate(entry) : entry));
  await assert.rejects(verifyReleaseArtifact({ archive: changed, checksums: await sumsFor(changed) }), reason);
}
```

`sumsFor` is the test helper that hashes exactly its archive argument and writes a one-entry `SHA256SUMS`; the size case uses the ZIP fixture writer's raw-header override so declared and opened byte sizes disagree.

    assert.equal(receipt.artifact.releaseTag, "v7.2.0");
    assert.match(receipt.artifact.sourceRevision, /^[0-9a-f]{40}$/);
    assert.equal(receipt.artifact.archiveSha256, expectedArchiveSha256);

    await rm(extractedRoot, { recursive: true });
    assert.equal((await checkInstalledPackage(target, { receiptPath })).status, "current");

    const prechecked = await verifyReleaseArtifact({ archive, checksums });
    await writeFile(mutableExtractionFile, "tampered after precheck");
    const installed = await installPackage({ archive, checksums, host: "both", scope: "user" });
    assert.equal(installed.status, "installed");
    assert.notEqual(await readFile(installed.targets[0].file, "utf8"), "tampered after precheck");

    await copyFile(otherArchive, archive); // replace after precheck, before install lock
    assert.equal((await installPackage({ archive, checksums, host: "both", scope: "user" })).reason, "archive_checksum_mismatch");

    const replacementBetweenVerifyAndExtract = await installPackage({ archive, checksums, host: "both", scope: "user" }, {
      afterArchiveVerified: async () => {
        await copyFile(otherArchive, archive);
        await copyFile(otherChecksums, checksums);
      },
    });
    assert.equal(replacementBetweenVerifyAndExtract.status, "installed");
    assert.deepEqual(replacementBetweenVerifyAndExtract.receipt.artifact.archiveFileMap, expectedArchiveFileMap);
    assert.equal(replacementBetweenVerifyAndExtract.receipt.artifact.archiveContentDigest, fileMapDigest(expectedArchiveFileMap));
    assert.deepEqual(replacementBetweenVerifyAndExtract.receipt.installedFileMaps, expectedInstalledFileMaps);

- [ ] **Step 2: Run RED.**

Run: node --test --test-name-pattern='artifact|checksum|temporary|both.*rollback|downgrade|reinstall' tests/hooks-artifacts.test.mjs tests/hooks-install-health.test.mjs tests/hooks-package.test.mjs

Expected: FAIL because current receipt lacks downloaded artifact identity.

- [ ] **Step 3: Validate before transaction.**

Require the checksum file to name exactly the archive; verify embedded version/tag/repository/revision and reject duplicate names, unsafe paths, symlinks, special entries, extras, or noncanonical modes. Derive exact `{sha256,mode,size}` entries and their canonical aggregate digest from the opened archive bytes.

When building, `expectedEntries` first derives `packageFileMap` from the manifest-listed non-metadata entries, adds `packageContentDigest:fileMapDigest(packageFileMap)` and that exact map to `.agent-team-source.json`, then writes the deterministic archive. The metadata file is excluded only from its own embedded map; verification adds its independently observed `{sha256,mode,size}` entry to the complete `archiveFileMap` used by staging, receipts, and installed-tree comparison.

```js
export async function verifyReleaseArtifact({ archive, checksums }) {
  const opened = await readArchiveOnce(archive);
  return verifyReleaseArtifactBytes({ ...opened, checksums });
}
export async function verifyReleaseArtifactBytes({ archiveName, bytes, checksums }) {
  const entries = parseSha256Sums(await readFile(checksums, "utf8"));
  if (entries.length !== 1 || entries[0].name !== archiveName) throw new Error("checksum_manifest_mismatch");
  if (createHash("sha256").update(bytes).digest("hex") !== entries[0].digest) throw new Error("archive_checksum_mismatch");
  const inspected = inspectArchiveClosedBytes(bytes, { prefix: "agent-team/", metadata: ".agent-team-source.json" });
  return { ...inspected, archiveFileMap: inspected.fileMap, archiveContentDigest: fileMapDigest(inspected.fileMap) };
}
function inspectArchiveClosedBytes(bytes, { prefix, metadata }) {
  const entries = readZipBytes(bytes); // ordered central-directory records; duplicates retained
  const seen = new Set();
  const fileMap = {};
  for (const entry of entries) {
    if (seen.has(entry.name)) throw new Error("duplicate_archive_entry");
    seen.add(entry.name);
    if (!safeArchivePath(entry.name, prefix) || entry.type !== "file") throw new Error("unsafe_archive_entry");
    if (![0o644, 0o755].includes(entry.mode)) throw new Error("archive_mode_mismatch");
    if (entry.declaredSize !== entry.data.length) throw new Error("archive_size_mismatch");
    fileMap[entry.name.slice(prefix.length)] = {
      sha256: createHash("sha256").update(entry.data).digest("hex"), mode: entry.mode, size: entry.data.length,
    };
  }
  const source = JSON.parse(entryBytes(entries, `${prefix}${metadata}`));
  const packageFileMap = Object.fromEntries(Object.entries(fileMap).filter(([name]) => name !== metadata));
  if (stable(source.packageFileMap) !== stable(packageFileMap)
    || source.packageContentDigest !== fileMapDigest(packageFileMap)) throw new Error("archive_file_map_mismatch");
  return { metadata: source, fileMap, packageFileMap, packageContentDigest: source.packageContentDigest };
}
```

`readZipBytes`, `safeArchivePath`, and `entryBytes` are private byte-buffer helpers in `artifacts.mjs`: the parser retains ordered duplicate central-directory records, validates local/central path-size-mode agreement, and never accepts a filesystem archive path. The embedded metadata binds every non-metadata package path to its `{sha256,mode,size}` record and aggregate; the verifier independently adds the metadata file to `archiveFileMap`. That non-self-referential complete archive map is the sole immutable authority later passed to extraction.

`inspectArchiveClosedBytes` iterates the ordered raw entries before constructing a map: reject a second occurrence of any normalized path, require the exact manifest allowlist, and compare each entry's normalized path, opened byte length, regular-file mode and SHA-256 with embedded metadata. Its returned `fileMap` is keyed by the full normalized relative path and each immutable value is exactly `{sha256,mode,size}`. ZIP header size disagreement, noncanonical mode and digest disagreement have distinct errors. The checksum entry, embedded metadata and derived file map are copied into the returned frozen artifact; extraction consumes only the same `opened.bytes` plus that artifact.

- [ ] **Step 4: Persist source-independent identity.**

Inside the exclusive install lock, open/read the caller's archive exactly once, verify those immutable bytes, and extract only those same bytes into a transaction-owned same-filesystem directory using exclusive regular-file creation and canonical modes. Before any selected-scope mutation, compare every present target's complete `{sha256,mode,size}` map with the archive-derived package map. An exact match is an unchanged reinstall with no target mutation. Any differing present target returns `update_requires_manual_replacement`, `changed: false`, with every target, role, configuration, backup, and receipt byte unchanged. Only absent targets enter fresh publication. Compare every staged and freshly installed map to archive authority and persist the archive aggregate/map plus each installed-host map in the receipt. Never install from a prior extraction or moving source root, and never reopen the caller's replaceable archive path after verification. Archive/checksum/staged drift aborts and restores every fresh mutation and the prior receipt.

```js
return withInstallLock(receiptPath, async () => {
  const opened = await readArchiveOnce(archive);
  const artifact = await verifyReleaseArtifactBytes({ ...opened, checksums });
  await options.afterArchiveVerified?.(); // test-only replacement of archive/checksums cannot affect opened.bytes or frozen artifact
  const sealed = await extractVerifiedArchiveBytesToSameFilesystem(opened.bytes, targetParent, artifact);
  if (stable(await fileMap(sealed)) !== stable(artifact.archiveFileMap)) throw new Error("staged_artifact_changed");
  return installBothFromSealedTree(sealed, targets, { artifact, receiptPath, failAfterFirstSwap });
});
```

- [ ] **Step 5: Preserve ownership during rollback/uninstall.**

Use installed digests to touch only unchanged owned resources. Never touch customized or reused_unowned paths. Automatic install never performs a changed-target update. A separately authorized later release update first quiesces the runtime and performs a rollback-backed move of the old target, then uses this command only as a fresh absent-target install.

- [ ] **Step 6: Run GREEN.**

Run: node --test tests/hooks-artifacts.test.mjs tests/hooks-install-health.test.mjs tests/hooks-package.test.mjs

Expected: PASS after deleting staged extraction.

- [ ] **Step 7: Commit.**

    git add hooks/lib/artifacts.mjs hooks/lib/install.mjs hooks/lib/health.mjs hooks/agent-team-cli.mjs tests/hooks-artifacts.test.mjs tests/hooks-install-health.test.mjs tests/hooks-package.test.mjs tests/hooks-cli.test.mjs
    git commit -m "feat: bind installs to release artifacts"

### Task 8: Agent-Team orchestration instructions and pressure tests (REL-008)

**Files:**
- Modify: SKILL.md
- Modify: references/actions.md
- Modify: references/dependencies.md
- Modify: references/graphify.md
- Modify: references/help.md
- Modify: references/hooks.md
- Modify: references/lean-ctx.md
- Modify: references/platform-claude.md
- Modify: references/platform-codex.md
- Modify: references/projects.md
- Modify: references/recovery.md
- Modify: references/release.md
- Modify: references/runs.md
- Modify: references/settings.md
- Modify: references/setup.md
- Modify: references/state.md
- Modify: references/status.md
- Modify: references/team.md
- Modify: tests/command-scenarios.md
- Modify: tests/hooks-docs.test.mjs

**Interfaces:**
- Consumes: finalized REL-001 through REL-007 public names and shapes.
- Produces: setup flow, native identity, project-owner-recover, evidence-store-register, generic completion-history-reconcile, compatibility run-reconcile, effective runs, status labels, dependency reuse, canonical binary, ordered liveness loop, termination rules, and release-tail behavior.

- [ ] **Step 1: Run writing-skills RED controls.**

Use `/tmp/at-720-pressure-scenarios.md` exactly: S1 repeated setup/Cancel, S2 missing answer, S3 read-only actions, L1 status interruption, L2 completion/review/refill, L3 scoped blocker, L4 60-second heartbeat, L5 global blocker, plus every Dependency and Release scenario ID in that report. For each ID run five fresh RED samples against the 7.1.x package, asking `State the next actions in order; do not execute them.` Save prompt, full response, package revision, host/model/effort, sample, choice, required decisions, forbidden rationalizations, and observed usage under the concrete naming rule `.agent-team/evidence/reliability/rel-008/S1/01.json` through `05.json`, substituting the report's exact scenario ID for `S1`.

The scoring fixture copied from the report is: S1 must order canonical inspection → dependency preparation → current-effective wizard → Cancel/Keep Existing → readiness and preserve all settings bytes while retaining the dependency receipt; L1 must answer in commentary then reconcile/review/refill; L4 must wait no more than 60 seconds inside the active turn without daemon/scheduler/new worker; L5 may ask finally only after recording recovery facts and exhausting safe independent work. A sample passes only when every required decision appears and none of the report's anti-rationalizations appears. A RED family with 5/5 compliance receives no speculative wording change.

- [ ] **Step 2: Add failing text assertions.**

Assert ordered event loop, setup-only mandatory wizard, kept_existing, owner-active no-write behavior, project-owner-recover, evidence-store-register, completion-history-reconcile, compatibility run-reconcile, native CWD, four status labels, local_only, target hold, reused_unowned, absolute ast-grep, and heartbeat cost/no-daemon caveat.

- [ ] **Step 3: Run RED.**

Run: node --test tests/hooks-docs.test.mjs

Expected: FAIL on optional-wizard and incomplete orchestration/migration language.

- [ ] **Step 4: Write minimal corrected runtime instructions.**

Use exact implemented names. Describe syntax checks as hygiene, status as read-only, consumed handoff as history, and migration as supported commands only.

- [ ] **Step 5: Write the ordered active-turn loop.**

Order: consume messages; classify replacement/addition/status-question; answer in commentary; reconcile workers/handoffs; route repair/review/integration; refill while reserving review; wait no more than 60 seconds; emit one compact known-state heartbeat; repeat.

- [ ] **Step 6: Write release/termination rules.**

Only a global blocker or no safe independent work permits a final question. Underfilled tails use terminal classification. Keep publication and production targets separate.

- [ ] **Step 7: Run writing-skills GREEN.**

Repeat the useful controls five times with the complete candidate. Require 5/5 and no hybrid workaround. Add counters only for observed rationalizations. This finite campaign has no recurring runtime cost; record actual sample usage.

- [ ] **Step 8: Run GREEN docs tests.**

Run: node --test tests/hooks-docs.test.mjs

Expected: PASS; historical guides remain unchanged.

- [ ] **Step 9: Commit.**

    git add SKILL.md references tests/command-scenarios.md tests/hooks-docs.test.mjs
    git commit -m "docs: define reliable setup and orchestration"

### Task 9: Agent-Team 7.2.0 version and package surfaces (REL-009)

**Files:**
- Modify: SKILL.md:3-5
- Modify: hooks/manifest.json:2-116
- Modify: CHANGELOG.md:1-10
- Modify: README.md:5,61-64,112-177,179-216,256-273
- Modify: GETTING_STARTED.md
- Modify: index.html:44
- Modify: tests/hooks-artifacts.test.mjs:178-203
- Modify: tests/hooks-docs.test.mjs:93-137

**Interfaces:**
- Consumes: final runtime file list/behavior.
- Produces: coherent 7.2.0 package, manifest including run-state.mjs and owner-recovery.mjs, stable Project Kickoff releases/latest link, archive/checksum install examples.

- [ ] **Step 1: Change tests to expect 7.2.0.**

Assert metadata, manifest, README, changelog, site, archive tag/name, and embedded metadata; assert Project Kickoff link is https://github.com/thebpandey/project-kickoff/releases/latest.

- [ ] **Step 2: Run RED.**

Run: node --test tests/hooks-artifacts.test.mjs tests/hooks-docs.test.mjs tests/hooks-package.test.mjs

Expected: FAIL because current identity is 7.1.1 and the new module is not packaged.

- [ ] **Step 3: Update version/changelog.**

Add dated 7.2.0 behavior and compatibility. Preserve historical entries/guides.

- [ ] **Step 4: Update manifest/public docs.**

Add hooks/lib/run-state.mjs and hooks/lib/owner-recovery.mjs. Require archive/checksum provenance. Do not link a future Project Kickoff tag.

- [ ] **Step 5: Run GREEN package suite.**

    node --test tests/hooks-*.test.mjs
    node hooks/agent-team-cli.mjs check-package
    git diff --check

Expected: PASS with exact manifest and no historical-guide rewrite.

- [ ] **Step 6: Commit.**

    git add SKILL.md hooks/manifest.json CHANGELOG.md README.md GETTING_STARTED.md index.html tests/hooks-artifacts.test.mjs tests/hooks-docs.test.mjs tests/hooks-package.test.mjs
    git commit -m "chore: prepare Agent-Team 7.2.0 package"

### Task 10: Integrate and qualify Agent-Team 7.2.0 (REL-010)

**Files:**
- Create: docs/release-checks-7.2.0.md
- Repair only through original task owners: files from REL-001 through REL-009

**Interfaces:**
- Consumes: Sol-medium accepted commits REL-001 through REL-009.
- Produces: one clean exact candidate, deterministic ZIPs, complete review/check evidence.

- [ ] **Step 1: Verify every pending commit.**

Sol-medium returns exact revision, requirements/code-quality verdicts, affected paths, and evidence. Route findings to the original developer.

- [ ] **Step 2: Integrate serially.**

Use one integration worktree. Recheck conflicts in initialization.mjs, workflow-cli.mjs, SKILL.md, manifest, and docs tests after each commit.

- [ ] **Step 3: Run full GREEN.**

    node --test tests/hooks-*.test.mjs
    node hooks/agent-team-cli.mjs check-package
    git diff --check

Expected: PASS; no required qualification is skipped.

- [ ] **Step 4: Build twice from exact HEAD.**

    at_release_revision="$(git rev-parse HEAD)"
    at_build_one="$(mktemp -d)"
    at_build_two="$(mktemp -d)"
    node hooks/agent-team-cli.mjs build-artifacts --revision "$at_release_revision" --output "$at_build_one"
    node hooks/agent-team-cli.mjs build-artifacts --revision "$at_release_revision" --output "$at_build_two"
    sha256sum "$at_build_one/agent-team-7.2.0.zip" "$at_build_two/agent-team-7.2.0.zip"
    node hooks/agent-team-cli.mjs check-artifacts --revision "$at_release_revision" --archive "$at_build_one/agent-team-7.2.0.zip"

Expected: identical hashes and exact metadata/files/modes/bytes.

- [ ] **Step 5: Run final Sol-medium candidate review.**

Review the full amended spec, security, atomic migration, installer rollback, and pressure evidence. Repair findings and repeat Steps 3-4.

- [ ] **Step 6: Record exact qualification evidence.**

Write only the qualification method, required commands, invariant expectations, and pre-final baseline to `docs/release-checks-7.2.0.md`. Do not record the candidate SHA, final ZIP hash, tag, workflow, or asset digest in this tracked file because committing it changes the candidate.

- [ ] **Step 7: Commit evidence and rebuild.**

    git add docs/release-checks-7.2.0.md
    git commit -m "docs: record Agent-Team 7.2.0 qualification"

Repeat Step 4 against this final commit. Write its final commit SHA and artifact hashes only to `.agent-team/evidence/reliability/rel-010/final-artifacts.json`, then bind them through tag/release assets in REL-011; the ignored ledger must not be added to Git.

### Task 11: Publish Agent-Team 7.2.0 (REL-011)

**Files/resources:**
- External: origin/main, refs/tags/v7.2.0, GitHub Actions/release/Pages
- Evidence: .agent-team/evidence/reliability/rel-011/

**Interfaces:**
- Consumes: exact REL-010 candidate and explicit publication authority.
- Produces: verified v7.2.0 with agent-team-7.2.0.zip and SHA256SUMS.

- [ ] **Step 1: Re-probe authority/state.**

After explicit publication authority has written the ignored closed receipt `{ "schemaVersion": 1, "repository": "thebpandey/agent-team", "releaseTag": "v7.2.0", "remoteBase": "40-lowercase-hex", "authorized": true }` at `.agent-team/evidence/reliability/rel-011/authority.json`, consume that exact base and prove tag/release absence:

```bash
test -z "$(git status --porcelain)"
test -f .agent-team/evidence/reliability/rel-011/authority.json
jq -e '((keys|sort)==["authorized","releaseTag","remoteBase","repository","schemaVersion"]) and .schemaVersion==1 and .repository=="thebpandey/agent-team" and .releaseTag=="v7.2.0" and .authorized==true and (.remoteBase|test("^[a-f0-9]{40}$"))' .agent-team/evidence/reliability/rel-011/authority.json >/dev/null
EXPECTED_AT_REMOTE_SHA="$(jq -er '.remoteBase' .agent-team/evidence/reliability/rel-011/authority.json)"
git fetch origin main --tags
test "$(git rev-parse origin/main)" = "$EXPECTED_AT_REMOTE_SHA"
test "$EXPECTED_AT_REMOTE_SHA" = "$(git merge-base HEAD "$EXPECTED_AT_REMOTE_SHA")"
test -z "$(git ls-remote --tags origin refs/tags/v7.2.0)"
! gh release view v7.2.0 --repo thebpandey/agent-team >/dev/null 2>&1
test -f .agent-team/evidence/reliability/rel-010/final-artifacts.json
```

Do not rewrite the authority receipt. A missing, malformed, or changed base stops publication.

- [ ] **Step 2: Push exact HEAD to origin/main without force.**

```bash
at_release_revision="$(git rev-parse HEAD)"
EXPECTED_AT_REMOTE_SHA="$(jq -er '.remoteBase' .agent-team/evidence/reliability/rel-011/authority.json)"
git fetch origin main --tags
test "$(git rev-parse origin/main)" = "$EXPECTED_AT_REMOTE_SHA"
test "$EXPECTED_AT_REMOTE_SHA" = "$(git merge-base HEAD "$EXPECTED_AT_REMOTE_SHA")"
test -z "$(git ls-remote --tags origin refs/tags/v7.2.0)"
! gh release view v7.2.0 --repo thebpandey/agent-team >/dev/null 2>&1
git push origin HEAD:refs/heads/main
test "$(git ls-remote origin refs/heads/main | awk '{print $1}')" = "$at_release_revision"
```

- [ ] **Step 3: Create/push annotated v7.2.0 once.**

```bash
git tag -a v7.2.0 "$at_release_revision" -m "Agent-Team 7.2.0"
git push origin refs/tags/v7.2.0
test "$(git ls-remote origin 'refs/tags/v7.2.0^{}' | awk '{print $1}')" = "$at_release_revision"
```

Never replace/retry an ambiguous tag.

- [ ] **Step 4: Verify tag workflow/release.**

```bash
AT_RUN_ID="$(gh run list --repo thebpandey/agent-team --workflow release.yml --commit "$at_release_revision" --limit 20 --json databaseId,headSha,workflowName | jq -er --arg sha "$at_release_revision" '[.[]|select(.headSha==$sha and .workflowName=="Release")]|sort_by(.databaseId)|last|.databaseId')"
gh run watch "$AT_RUN_ID" --repo thebpandey/agent-team --exit-status
gh run view "$AT_RUN_ID" --repo thebpandey/agent-team --json conclusion,headSha,workflowName | jq -e --arg sha "$at_release_revision" '.conclusion=="success" and .headSha==$sha and .workflowName=="Release"'
gh release view v7.2.0 --repo thebpandey/agent-team --json isDraft,isPrerelease,tagName,assets | jq -e '.tagName=="v7.2.0" and (.isDraft|not) and (.isPrerelease|not) and ([.assets[].name]|sort)==["SHA256SUMS","agent-team-7.2.0.zip"]'
```

- [ ] **Step 5: Download and verify official assets.**

    at_download="$(mktemp -d)"
    gh release download v7.2.0 --repo thebpandey/agent-team --dir "$at_download"
    (cd "$at_download" && sha256sum -c SHA256SUMS)
    node hooks/agent-team-cli.mjs check-artifacts --revision "$(git rev-parse v7.2.0^{commit})" --archive "$at_download/agent-team-7.2.0.zip"

```bash
at_tag_parent="$(mktemp -d)"
at_tag_checkout="$at_tag_parent/checkout"
at_archive_extract="$(mktemp -d)"
git worktree add --detach "$at_tag_checkout" v7.2.0
unzip -q "$at_download/agent-team-7.2.0.zip" -d "$at_archive_extract"
while IFS= read -r relative; do cmp "$at_tag_checkout/$relative" "$at_archive_extract/agent-team/$relative"; done < <(node -e 'const m=require("./hooks/manifest.json"); for (const f of m.files) console.log(f)')
curl -fsSL https://thebpandey.github.io/agent-team/ | grep -F '7.2.0'
git worktree remove "$at_tag_checkout"
```

- [ ] **Step 6: Record release evidence.**

Persist exact tag/revision, workflow run, asset URLs/digests, and Pages observation in the ledger. Do not issue a duplicate release operation.

### Task 12: Install Agent-Team 7.2.0 on both user hosts (REL-012)

**Files/resources:**
- Transactional target: /home/server/.agents/skills/agent-team
- Transactional target: /home/server/.claude/skills/agent-team
- Managed receipt/registrations: /home/server/.agent-team-hooks/install.json and selected host hooks/roles

**Interfaces:**
- Consumes: downloaded verified official v7.2.0 assets and explicit both-host user authority.
- Produces: receipt-bound 7.2.0 installs and fresh native loaded-runtime evidence.

- [ ] **Step 1: Download, extract, and validate the official artifact.**

    at_install_download="$(mktemp -d)"
    gh release download v7.2.0 --repo thebpandey/agent-team --dir "$at_install_download"
    (cd "$at_install_download" && sha256sum -c SHA256SUMS)
    unzip -q "$at_install_download/agent-team-7.2.0.zip" -d "$at_install_download/extracted"
    node "$at_install_download/extracted/agent-team/hooks/agent-team-cli.mjs" check-artifacts --revision "$(git -C /home/server/dev/skills/agent-team rev-parse v7.2.0^{commit})" --archive "$at_install_download/agent-team-7.2.0.zip"

Require safe paths, exact metadata, check-package, check-installed-package, and check-artifacts.

- [ ] **Step 2: Snapshot current owned and unrelated state.**

    find /home/server/.agents/skills/agent-team /home/server/.claude/skills/agent-team -type f -print0 | sort -z | xargs -0 sha256sum > "$at_install_download/preinstall-files.sha256"
    cp -a /home/server/.agent-team-hooks/install.json "$at_install_download/install.pre.json"

- [ ] **Step 3: Install once from extracted released CLI.**

    node "$at_install_download/extracted/agent-team/hooks/agent-team-cli.mjs" install --host both --scope user --archive "$at_install_download/agent-team-7.2.0.zip" --checksums "$at_install_download/SHA256SUMS"

Require conflict-free installed status and exact receipt artifact fields.

- [ ] **Step 4: Delete extraction and validate installed packages.**

    at_extracted_real="$(realpath "$at_install_download/extracted")"
    test "$at_extracted_real" = "$at_install_download/extracted"
    rm -rf -- "$at_extracted_real"
    test ! -e "$at_install_download/extracted"
    node /home/server/.agents/skills/agent-team/hooks/agent-team-cli.mjs check-installed-package
    node /home/server/.claude/skills/agent-team/hooks/agent-team-cli.mjs check-installed-package

- [ ] **Step 5: Reload and verify native hosts.**

Fresh Codex and Claude sessions invoke version/help and BrainVault user-scope health. Verify loaded path/version, registration/trust/exercise, and actual native events.

- [ ] **Step 6: Record installation evidence.**

Save official digest, transaction/recovery IDs, host paths/versions, preservation hashes, and reload observations in .agent-team/evidence/reliability/rel-012/; this ignored evidence is not committed.

### Task 13: Correct Project Kickoff handoff and request semantics (REL-013)

**Repository:** /home/server/dev/skills/project-kickoff

**Files:**
- Modify: scripts/check_agent_team_handoff.py:59-86,174-416
- Modify: assets/templates/AGENT_TEAM_HANDOFF.json:5-18
- Test: tests/test_agent_team_handoff.py:18-295

**Interfaces:**
- Consumes: released Agent-Team 7.2.0 request/provenance schema.
- Produces: centralized supported-pair classification; generation-baseline ancestry; emit-time branch/tracker revalidation and current-tip binding.

- [ ] **Step 1: Write failing regressions.**

Add committed descendant, unrelated branch, invalid approved ancestry, four supported/crossed pairs, emit rebind, emit tracker drift/no stdout, unchanged input bytes, and real released 7.2.0 consumption.

```python
def test_emit_request_rebinds_the_current_descendant_tip(self):
    handoff = valid_handoff(self.root)
    baseline = handoff["project"]["revision"]
    handoff_path = self.root / "AGENT_TEAM_HANDOFF.json"
    handoff_path.write_text(json.dumps(handoff))
    subprocess.run(["git", "-C", str(self.root), "add", handoff_path.name], check=True)
    subprocess.run(["git", "-C", str(self.root), "commit", "-qm", "commit handoff"], check=True)
    tip = subprocess.check_output(["git", "-C", str(self.root), "rev-parse", "HEAD"], text=True).strip()
    result = self.emit_request(handoff)
    self.assertEqual(result.returncode, 0, result.stderr)
    emitted = json.loads(result.stdout)
    self.assertEqual(emitted["request"]["handoff"]["generationBaseline"], baseline)
    self.assertEqual(emitted["request"]["handoff"]["observedRevision"], tip)
    self.assertEqual(json.loads((self.root / "AGENT_TEAM_HANDOFF.json").read_text())["project"]["revision"], baseline)
```

- [ ] **Step 2: Run RED.**

    cd /home/server/dev/skills/project-kickoff
    PYTHONPATH=tests PYTHONDONTWRITEBYTECODE=1 python3 -m unittest -v test_agent_team_handoff.AgentTeamHandoffTests.test_checker_accepts_a_committed_handoff_descendant test_agent_team_handoff.AgentTeamHandoffTests.test_checker_rejects_an_unrelated_generation_revision test_agent_team_handoff.AgentTeamHandoffTests.test_checker_enforces_the_supported_compatibility_matrix test_agent_team_handoff.AgentTeamHandoffTests.test_emit_request_rebinds_the_current_descendant_tip test_agent_team_handoff.AgentTeamHandoffTests.test_emit_request_rejects_tracker_drift

Expected: FAIL on exact-tip and missing matrix/rebind behavior.

- [ ] **Step 3: Implement compatibility matrix.**

Accept only 0.3.1/7.0.2, 0.4.0/7.0.2, 0.4.1/7.1.0, 0.4.1/7.2.0. New template uses 7.2.0. Failures name checker 0.4.1 and migration.

```python
SUPPORTED_PAIRS = {("0.3.1", "7.0.2"), ("0.4.0", "7.0.2"), ("0.4.1", "7.1.0"), ("0.4.1", "7.2.0")}

def compatibility_pair(project_kickoff_version, agent_team_version):
    pair = (project_kickoff_version, agent_team_version)
    if pair not in SUPPORTED_PAIRS:
        raise ValueError(f"unsupported handoff compatibility {pair}; checker 0.4.1 requires an approved migration")
    return pair
```

- [ ] **Step 4: Implement Git ancestry.**

Keep exact root/checked-out local branch; require commit resolution, generation baseline ancestor of branch, and approvedRevision ancestor of baseline. Keep subprocess bounds.

```python
def resolved_branch_tip(root, branch):
    symbolic = git(root, "symbolic-ref", "--short", "HEAD")
    if symbolic != branch:
        raise ValueError("project.branch must be the checked-out local branch")
    tip = git(root, "rev-parse", "--verify", f"refs/heads/{branch}^{{commit}}")
    return tip

def require_ancestor(root, ancestor, descendant, label):
    result = subprocess.run(["git", "-C", str(root), "merge-base", "--is-ancestor", ancestor, descendant], timeout=3)
    if result.returncode != 0:
        raise ValueError(f"{label} is not an ancestor")
```

- [ ] **Step 5: Revalidate at emission.**

Immediately before serialization reread tip/tracker, enforce exact unconsumed tracker identity/baseline ancestry, populate released 7.2.0 request fields, and leave input unchanged.

```python
root = Path(value["project"]["root"])
observed_tip = resolved_branch_tip(root, value["project"]["branch"])
require_ancestor(root, value["project"]["revision"], observed_tip, "project.revision generation baseline")
tracker = value["tracker"]
current_rows = (markdown_tracker(root / tracker["path"])
                if tracker["kind"] == "markdown"
                else beads_tracker(str(root), tracker["executable"]))
if [row["id"] for row in current_rows] != [task["id"] for task in value["plan"]["tasks"]]:
    raise ValueError("tracker changed after handoff validation")
request["request"]["handoff"] = {
    "schemaVersion": 1,
    "path": handoff.relative_to(root).as_posix(),
    "sha256": hashlib.sha256(source).hexdigest(),
    "generatedBy": {"name": "project-kickoff", "version": value["projectKickoff"]["version"]},
    "testedAgainst": {"name": "agent-team", "version": value["agentTeam"]["testedVersion"]},
    "generationBaseline": value["project"]["revision"],
    "observedRevision": observed_tip,
}
```

- [ ] **Step 6: Preserve security gates.**

Retain regular-file/symlink/size, sanitized Beads, timeout, safe IDs/capabilities/paths, tracker identity, dependencies/cycles/actionability, runtime IDs, and request size.

- [ ] **Step 7: Run GREEN with released Agent-Team.**

    at_checker_download="$(mktemp -d)"
    gh release download v7.2.0 --repo thebpandey/agent-team --dir "$at_checker_download"
    (cd "$at_checker_download" && sha256sum -c SHA256SUMS)
    unzip -q "$at_checker_download/agent-team-7.2.0.zip" -d "$at_checker_download/extracted"
    AGENT_TEAM_ROOT="$at_checker_download/extracted/agent-team" PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v

Expected: all tests pass; 7.2 consumption runs, not skips.

- [ ] **Step 8: Commit.**

    git add scripts/check_agent_team_handoff.py assets/templates/AGENT_TEAM_HANDOFF.json tests/test_agent_team_handoff.py
    git commit -m "fix: bind handoffs to generation baselines"

### Task 14: Add Project Kickoff transactional installer (REL-014)

**Repository:** /home/server/dev/skills/project-kickoff

**Files:**
- Create: scripts/manage_install.py
- Create: tests/test_install_manager.py
- Modify: tests/test_package_manifest.py
- Modify: README.md:192-336,565-575

**Interfaces:**
- Consumes: official ZIP/SHA256SUMS, host codex|claude-code|both, scope user|project, home, and project root for project scope. The extracted CLI may launch the command, but its tree is never an installation input.
- Produces: build-artifact, check-artifact, install, rollback, uninstall, check-installed commands; generated project-kickoff/.project-kickoff-source.json archive metadata; user receipt at HOME/.project-kickoff-installer/install.json; project receipt under the Git common directory at project-kickoff-installer/install.json; atomic targets/backups.

Closed CLI schemas are `build-artifact --source-root PATH --repository thebpandey/project-kickoff --release-tag v0.4.1 --release-url https://github.com/thebpandey/project-kickoff/releases/tag/v0.4.1 --source-revision 40HEX --output ZIP`, `check-artifact --archive ZIP --checksums FILE --repository thebpandey/project-kickoff --release-tag v0.4.1 --release-url https://github.com/thebpandey/project-kickoff/releases/tag/v0.4.1 --source-revision 40HEX [--source-root EXACT_TAG_CHECKOUT]`, `install --archive ZIP --checksums FILE --host codex|claude-code|both --scope user|project --home PATH [--project PATH] [--allow-compatible-downgrade]`, and `check-installed|rollback|uninstall --host codex|claude-code|both --scope user|project --home PATH [--project PATH]`. Reject unknown arguments. Install never accepts or copies an independently mutable source tree.

Private helper definitions in `manage_install.py` are: `read_archive_once(path) -> {archive_name, bytes}` using no-follow bounded regular-file reads plus stable pre/post stat identity; `parse_single_checksum(path, archive_name) -> {archiveName, archiveSha256, checksumFileSha256}`; `safe_member(name, prefix) -> bool`; `verify_artifact_bytes(...) -> validated metadata dict`; `verify_artifact(...) -> validated metadata dict` as the check-only one-read wrapper; `packageContentMap(path) -> exact 35-entry tracked-package relative-file `{sha256,mode,size}` map, deliberately excluding generated .project-kickoff-source.json and rejecting missing/extra/symlink/special entries`; `installedTreeMap(path) -> complete installed relative-file `{sha256,mode,size}` map including .project-kickoff-source.json and rejecting missing/extra/symlink/special entries`; `fileMapDigest(map) -> canonical SHA-256`; `atomic_json(path, value, mode)` using same-directory `O_EXCL` temp plus `os.replace`; `stage_on_same_filesystem(target, source) -> staged Path`; `atomic_swap(staged, target) -> backup Path`; `build_receipt(validated_artifact, targets, journal) -> dict`; and `InstallJournal.begin(receipt_path, targets)`, `.extract_archive_bytes_same_filesystem(bytes, artifact)`, `.snapshot(target)`, `.restore_all_preimages()`, `.commit()`, `.recovery_identity()`. Archive inspection rejects duplicate names before any dictionary construction, unsafe paths, directories outside the exact package shape, symlinks, special entries, extras, and noncanonical modes. Extraction creates regular files exclusively with canonical modes. No install helper receives or reopens the caller's archive path after `read_archive_once`. Implement these functions in this task; no unnamed primitive is permitted.

- [ ] **Step 1: Write failing installer tests.**

Cover both-host user and project installs, checksum/metadata tamper, duplicate/unsafe/symlink/special archive entries, wrong mode/size, complete 35-tracked-file-plus-metadata package, extraction deletion, exact reinstall, downgrade denial, unowned/custom conflict, injected second-host rollback, installed drift, rollback/uninstall, hooks remaining inactive, archive/checksum replacement after precheck, replacement of the caller archive exactly between verification and extraction, and staged-byte mutation before swap. `test_install_rejects_duplicate_member_before_extraction`, `test_install_rechecks_archive_inside_lock`, `test_install_uses_opened_bytes_when_path_replaced_between_verify_and_extract`, `test_install_rejects_staged_tamper_before_swap`, and `test_install_ignores_deleted_or_modified_precheck_extraction` snapshot both valid prior targets and receipt, inject the named race, and assert exact archive-derived `{sha256,mode,size}` maps and aggregate digests or byte-exact restoration.

```python
def install_valid_release(target, release):
    shutil.copytree(release["source"], target)

def write_valid_receipt(path, release, targets):
    value = {**release["validatedArtifact"], "targets": [str(item) for item in targets], "installedFileMaps": {str(item): installedTreeMap(item) for item in targets}}
    source = (json.dumps(value, sort_keys=True) + "\n").encode()
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_bytes(source)
    return source

def test_second_host_failure_restores_both_preimages(self):
    codex = self.home / ".agents/skills/project-kickoff"
    claude = self.home / ".claude/skills/project-kickoff"
    install_valid_release(codex, self.previous_release)
    install_valid_release(claude, self.previous_release)
    prior_receipt = write_valid_receipt(self.receipt_path, self.previous_release, [codex, claude])
    before = {"codex": installedTreeMap(codex), "claude": installedTreeMap(claude)}
    result = run_manager("install", self.release_args, "--host", "both", "--scope", "user", "--home", str(self.home), env={"PROJECT_KICKOFF_FAIL_AFTER_FIRST_SWAP": "1"})
    self.assertNotEqual(result.returncode, 0)
    self.assertEqual(installedTreeMap(codex), before["codex"])
    self.assertEqual(installedTreeMap(claude), before["claude"])
    self.assertEqual(self.receipt_path.read_bytes(), prior_receipt)

def test_duplicate_mode_size_and_path_manifest_are_closed(self):
    entries = read_zip_entries(self.valid_archive)
    duplicate = self.tmp / "duplicate.zip"
    write_zip_entries(duplicate, entries + [entries[0]])
    self.assert_manager_rejects(duplicate, "duplicate_archive_member")

    wrong_mode = self.tmp / "wrong-mode.zip"
    write_zip_entries(wrong_mode, [entry._replace(mode=0o777) if entry.path == "project-kickoff/SKILL.md" else entry for entry in entries])
    self.assert_manager_rejects(wrong_mode, "unsafe_archive_type_or_mode")

    wrong_size = self.tmp / "wrong-size-manifest.zip"
    metadata = json.loads(next(entry.data for entry in entries if entry.path.endswith(".project-kickoff-source.json")))
    metadata["packageFileMap"]["SKILL.md"]["size"] += 1
    write_zip_entries(wrong_size, replace_metadata(entries, metadata))
    self.assert_manager_rejects(wrong_size, "artifact_file_map_mismatch")

def test_replacing_archive_and_sums_after_open_cannot_change_sealed_authority(self):
    hooks = TestHooks(after_archive_verified=lambda: (os.replace(self.other_archive, self.valid_archive),
      os.replace(self.other_sums, self.checksums)))
    result = install_transaction(self.targets, self.valid_archive, self.checksums, self.receipt_path,
      self.expected_release, hooks)
    self.assertEqual(result["receipt"]["archiveFileMap"], self.expected_archive_file_map)
    self.assertEqual(installedTreeMap(self.targets[0]), self.expected_archive_file_map)
```

Define the test-only `ArchiveEntry(path,data,mode)` named tuple, `read_zip_entries`, `write_zip_entries`, `replace_metadata`, `assert_manager_rejects`, and `TestHooks` in `tests/test_install_manager.py`. The fixture writer preserves order and deliberately permits duplicate names; it recomputes `SHA256SUMS` for each negative archive so failure proves the inner path/mode/size manifest rule rather than the outer checksum.

- [ ] **Step 2: Run RED.**

Run: PYTHONDONTWRITEBYTECODE=1 python3 -m unittest -v tests/test_install_manager.py

Expected: FAIL because manage_install.py does not exist.

- [ ] **Step 3: Implement closed validation.**

Use Python standard library. Require explicit archive/checksum/host/scope; verify one checksum entry, safe ZIP prefix, version/source metadata, unique names, exact allowlist, regular files, canonical modes, sizes, and digests before mutation. `build-artifact` alone consumes a source root to write canonical source metadata from an explicit 40-hex revision and deterministic ZIP bytes; `check-artifact` proves that metadata and every packaged byte.

```python
import io
import stat

def verify_artifact(archive, checksums, repository, release_tag, release_url, source_revision, source_root=None):
    opened = read_archive_once(archive)
    return verify_artifact_bytes(opened["archive_name"], opened["bytes"], checksums, repository, release_tag, release_url, source_revision, source_root)

def verify_artifact_bytes(archive_name, archive_bytes, checksums, repository, release_tag, release_url, source_revision, source_root=None):
    provenance = parse_single_checksum(checksums, archive_name)
    if hashlib.sha256(archive_bytes).hexdigest() != provenance["archiveSha256"]:
        raise InstallError("archive_checksum_mismatch")
    with zipfile.ZipFile(io.BytesIO(archive_bytes)) as package:
        infos = package.infolist()
        names = [info.filename for info in infos]
        if len(names) != len(set(names)):
            raise InstallError("duplicate_archive_member")
        if any(not safe_member(name, prefix="project-kickoff/") for name in names):
            raise InstallError("unsafe_archive_path")
        metadata = json.loads(package.read("project-kickoff/.project-kickoff-source.json"))
        fixed = {"schemaVersion": 1, "version": "0.4.1", "repository": repository, "releaseTag": release_tag, "releaseUrl": release_url, "sourceRevision": source_revision}
        if {key: metadata.get(key) for key in fixed} != fixed or not isinstance(metadata.get("packageFileMap"), dict):
            raise InstallError("artifact_metadata_mismatch")
        archive_file_map = {}
        for info in infos:
            mode = (info.external_attr >> 16) & 0o177777
            if not stat.S_ISREG(mode) or (mode & 0o777) not in (0o644, 0o755):
                raise InstallError("unsafe_archive_type_or_mode")
            data = package.read(info)
            if len(data) != info.file_size:
                raise InstallError("archive_size_mismatch")
            archive_file_map[info.filename.removeprefix("project-kickoff/")] = {
                "sha256": hashlib.sha256(data).hexdigest(), "mode": mode & 0o777, "size": info.file_size}
        package_file_map = {name: value for name, value in archive_file_map.items() if name != ".project-kickoff-source.json"}
        if package_file_map != metadata["packageFileMap"] or fileMapDigest(package_file_map) != metadata.get("packageContentDigest"):
            raise InstallError("artifact_file_map_mismatch")
        if source_root is not None and package_file_map != packageContentMap(source_root):
            raise InstallError("artifact_source_checkout_mismatch")
    return {**metadata, **provenance, "archiveFileMap": archive_file_map, "archiveContentDigest": fileMapDigest(archive_file_map)}

def artifact_metadata(source_root, repository, release_tag, release_url, source_revision):
    package_file_map = packageContentMap(source_root)
    return {"schemaVersion": 1, "version": "0.4.1", "repository": repository, "releaseTag": release_tag, "releaseUrl": release_url, "sourceRevision": source_revision, "packageFileMap": package_file_map, "packageContentDigest": fileMapDigest(package_file_map)}
```

- [ ] **Step 4: Implement transactions.**

Use exclusive lock/journal, same-filesystem staging, owned snapshots, atomic rename, and full preimage restoration on partial both failure. Preserve unowned/custom targets.

```python
def install_transaction(targets, archive, checksums, receipt_path, expected_release, test_hooks=None):
    journal = InstallJournal.begin(receipt_path, targets)
    try:
        opened = read_archive_once(archive)  # exactly once, inside exclusive lock
        artifact = verify_artifact_bytes(opened["archive_name"], opened["bytes"], checksums, **expected_release)
        if test_hooks is not None:
            test_hooks.after_archive_verified()  # race-test injection; replacing caller path is now irrelevant
        sealed_source = journal.extract_archive_bytes_same_filesystem(opened["bytes"], artifact)
        if installedTreeMap(sealed_source) != artifact["archiveFileMap"]:
            raise InstallError("staged_artifact_changed")
        for target in targets:
            journal.snapshot(target)
            staged = stage_on_same_filesystem(target, sealed_source)
            if installedTreeMap(staged) != artifact["archiveFileMap"]:
                raise InstallError("staged_artifact_changed")
            atomic_swap(staged, target)
            if installedTreeMap(target) != artifact["archiveFileMap"]:
                raise InstallError("installed_artifact_changed")
        receipt = build_receipt(artifact, targets, journal)
        atomic_json(receipt_path, receipt, mode=0o600)
        journal.commit()
        return {"status": "installed", "receipt": receipt}
    except BaseException:
        journal.restore_all_preimages()
        raise
```

- [ ] **Step 5: Persist durable receipt.**

Record schema/version, release tag/URL, source revision, archive SHA-256, archive content digest, exact archive `{sha256,mode,size}` map, per-target installed maps, host/scope/targets, transaction ID, backup/recovery identity, and UTC time. Exact reinstall is unchanged; downgrade needs --allow-compatible-downgrade.

```python
receipt = {"schemaVersion": 1, **{key: validated_artifact[key] for key in ("version", "repository", "releaseTag", "releaseUrl", "sourceRevision", "archiveSha256", "checksumFileSha256", "archiveName", "packageContentDigest", "packageFileMap", "archiveContentDigest", "archiveFileMap")}, "host": args.host, "scope": args.scope, "targets": [str(path) for path in targets], "installedFileMaps": {str(path): installedTreeMap(path) for path in targets}, "transactionId": journal.id, "recovery": journal.recovery_identity(), "installedAt": datetime.now(timezone.utc).isoformat()}
```

- [ ] **Step 6: Implement digest-bound rollback/uninstall/check.**

Touch only unchanged receipted targets. Retain/report drift. Never activate hooks or alter settings.

```python
def removable_target(target, receipt):
    expected = receipt["installedFileMaps"].get(str(target))
    return expected is not None and installedTreeMap(target) == expected
```

- [ ] **Step 7: Update package/docs.**

Add manage_install.py to both README allowlists/tree; the tracked package becomes 35 files and its release ZIP has those 35 plus generated .project-kickoff-source.json. Document official artifact install/check/rollback/reload.

- [ ] **Step 8: Run GREEN.**

    PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v
    git diff --check

Expected: PASS including injected rollback/preservation.

- [ ] **Step 9: Commit.**

    git add scripts/manage_install.py tests/test_install_manager.py tests/test_package_manifest.py README.md
    git commit -m "feat: install Project Kickoff transactionally"

### Task 15: Qualify and publish Project Kickoff 0.4.1 (REL-015)

**Repository:** /home/server/dev/skills/project-kickoff

**Files:**
- Modify: CHANGELOG.md:6-23
- Modify: README.md:85,170,188-190,450,487,499-528
- Modify: references/artifacts.md:103-123
- Modify: references/handoff.md:9-29,63-101
- Modify: references/hosts.md:46-59
- Modify: references/setup.md:108-178
- Modify: assets/templates/README.md:82-104
- Create: docs/release-checks-0.4.1.md
- Test: tests/test_agent_team_handoff.py
- Test: tests/test_package_manifest.py
- Test: tests/test_install_manager.py
- External: Project Kickoff origin/main, refs/tags/v0.4.1, GitHub release/Pages

**Interfaces:**
- Consumes: REL-013/014 accepted commits and verified released and installed Agent-Team 7.2.0.
- Produces: current 0.4.1 docs/package, deterministic ZIP with 35 tracked files plus generated source metadata, exact release evidence, published v0.4.1.

- [ ] **Step 1: Run Project Kickoff writing-skills RED controls.**

Use `/tmp/pk-041-pressure-scenarios.md` Scenario 1 generation baseline, Scenario 2 unrelated revision, Scenario 3 provenance/runtime labels, Scenario 4 tracker-safe emit rebind, Scenario 5 released-contract key, Scenario 6 native identity, Scenario 7 preserved security bounds, and Scenario 8 release-artifact installation. Run five fresh RED samples and five fresh GREEN samples per scenario (80 first-pass samples maximum), save the complete prompt/response/package revision/host/model/effort/sample/choice/usage, and record verbatim rationalizations. Each GREEN sample must choose C and satisfy that scenario's success criteria; guessed wire keys, rewritten historical provenance, tracker widening, weakened bounds, shell identity, source-copy install, or any hybrid workaround fails. If a RED scenario is already 5/5, do not add speculative prose; rerun only demonstrated failures.

- [ ] **Step 2: Add/update failing documentation/package assertions.**

Require generation baseline, emit-time tip, matrix, 7.2.0 new template, historical consumed input, installer commands, and 35 tracked files plus generated metadata package parity.

- [ ] **Step 3: Run RED docs/package tests.**

Run: PYTHONDONTWRITEBYTECODE=1 python3 -m unittest -v tests/test_agent_team_handoff.py tests/test_package_manifest.py tests/test_install_manager.py

Expected: FAIL on old 7.1/current-tip prose and pre-installer package count.

- [ ] **Step 4: Update current docs and changelog.**

Amend still-unpublished 0.4.1. Do not rewrite 0.3.1 guides or old release evidence. Keep SKILL.md metadata at 0.4.1.

- [ ] **Step 5: Run GREEN pressure tests.**

Repeat useful scenarios five times against the complete candidate; require 5/5 without guessed wire fields, tracker weakening, provenance rewrite, or source/install confusion.

- [ ] **Step 6: Run full GREEN and Sol-medium review.**

    at_contract_download="$(mktemp -d)"
    gh release download v7.2.0 --repo thebpandey/agent-team --dir "$at_contract_download"
    (cd "$at_contract_download" && sha256sum -c SHA256SUMS)
    unzip -q "$at_contract_download/agent-team-7.2.0.zip" -d "$at_contract_download/extracted"
    AGENT_TEAM_ROOT="$at_contract_download/extracted/agent-team" PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s tests -v
    git diff --check

Require independent requirements/code-quality acceptance at exact revision.

- [ ] **Step 7: Commit docs/evidence candidate.**

    git add CHANGELOG.md README.md references assets/templates/README.md docs/release-checks-0.4.1.md tests/test_agent_team_handoff.py tests/test_package_manifest.py tests/test_install_manager.py
    git commit -m "docs: qualify Project Kickoff 0.4.1"

- [ ] **Step 8: Build the final candidate twice.**

    pk_release_revision="$(git rev-parse HEAD)"
    pk_build_one="$(mktemp -d)"
    pk_build_two="$(mktemp -d)"
    python3 scripts/manage_install.py build-artifact --source-root . --repository thebpandey/project-kickoff --release-tag v0.4.1 --release-url https://github.com/thebpandey/project-kickoff/releases/tag/v0.4.1 --source-revision "$pk_release_revision" --output "$pk_build_one/project-kickoff-0.4.1.zip"
    python3 scripts/manage_install.py build-artifact --source-root . --repository thebpandey/project-kickoff --release-tag v0.4.1 --release-url https://github.com/thebpandey/project-kickoff/releases/tag/v0.4.1 --source-revision "$pk_release_revision" --output "$pk_build_two/project-kickoff-0.4.1.zip"
    (cd "$pk_build_one" && sha256sum project-kickoff-0.4.1.zip > SHA256SUMS)
    python3 scripts/manage_install.py check-artifact --archive "$pk_build_one/project-kickoff-0.4.1.zip" --checksums "$pk_build_one/SHA256SUMS" --repository thebpandey/project-kickoff --release-tag v0.4.1 --release-url https://github.com/thebpandey/project-kickoff/releases/tag/v0.4.1 --source-revision "$pk_release_revision" --source-root .
    sha256sum "$pk_build_one/project-kickoff-0.4.1.zip" "$pk_build_two/project-kickoff-0.4.1.zip"

Expected: identical hashes, exactly 35 tracked regular package files plus canonical generated source metadata, no other extras/symlinks, and tracked-file byte identity to pk_release_revision. The old 43b6 hash must not recur as release evidence.

- [ ] **Step 9: Re-probe and publish without force.**

Require explicit authority and run the exact sequence:

```bash
pk_release_revision="$(git rev-parse HEAD)"
EXPECTED_PK_REMOTE_SHA="$(jq -er '.remoteBase' .agent-team/evidence/reliability/rel-015/authority.json)"
git fetch origin main --tags
test -z "$(git ls-remote --tags origin refs/tags/v0.4.1)"
! gh release view v0.4.1 --repo thebpandey/project-kickoff >/dev/null 2>&1
test "$(git rev-parse origin/main)" = "$EXPECTED_PK_REMOTE_SHA"
test "$EXPECTED_PK_REMOTE_SHA" = "$(git merge-base HEAD "$EXPECTED_PK_REMOTE_SHA")"
git fetch origin main
test "$(git rev-parse origin/main)" = "$EXPECTED_PK_REMOTE_SHA"
git push origin HEAD:refs/heads/main
test "$(git ls-remote origin refs/heads/main | awk '{print $1}')" = "$pk_release_revision"
git tag -a v0.4.1 "$pk_release_revision" -m "Project Kickoff 0.4.1"
git push origin refs/tags/v0.4.1
test "$(git ls-remote origin 'refs/tags/v0.4.1^{}' | awk '{print $1}')" = "$pk_release_revision"
(cd "$pk_build_one" && sha256sum project-kickoff-0.4.1.zip > SHA256SUMS && sha256sum -c SHA256SUMS)
gh release create v0.4.1 --repo thebpandey/project-kickoff --verify-tag --title "Project Kickoff 0.4.1" --notes-file CHANGELOG.md "$pk_build_one/project-kickoff-0.4.1.zip" "$pk_build_one/SHA256SUMS"
gh release view v0.4.1 --repo thebpandey/project-kickoff --json isDraft,isPrerelease,tagName,assets | jq -e '.tagName=="v0.4.1" and (.isDraft|not) and (.isPrerelease|not) and ([.assets[].name]|sort)==["SHA256SUMS","project-kickoff-0.4.1.zip"]'
```

- [ ] **Step 10: Download and independently verify release.**

    pk_download="$(mktemp -d)"
    gh release download v0.4.1 --repo thebpandey/project-kickoff --dir "$pk_download"
    (cd "$pk_download" && sha256sum -c SHA256SUMS)

Extract safely, compare to independent tag checkout, run full package/checker/installer tests from the extracted package, and verify Pages/non-draft/non-prerelease/exact assets.

```bash
test "$(git ls-remote origin 'refs/tags/v0.4.1^{}' | awk '{print $1}')" = "$pk_release_revision"
test "$(sha256sum "$pk_download/project-kickoff-0.4.1.zip" | awk '{print $1}')" = "$(awk '{print $1}' "$pk_download/SHA256SUMS")"
test "$(gh release view v0.4.1 --repo thebpandey/project-kickoff --json assets --jq '.assets|length')" = 2
PK_PAGES_RUN_ID="$(gh run list --repo thebpandey/project-kickoff --commit "$pk_release_revision" --limit 20 --json databaseId,headSha,workflowName | jq -er --arg sha "$pk_release_revision" '[.[]|select(.headSha==$sha and (.workflowName|ascii_downcase)=="pages build and deployment")]|sort_by(.databaseId)|last|.databaseId')"
gh run watch "$PK_PAGES_RUN_ID" --repo thebpandey/project-kickoff --exit-status
gh run view "$PK_PAGES_RUN_ID" --repo thebpandey/project-kickoff --json conclusion,headSha | jq -e --arg sha "$pk_release_revision" '.conclusion=="success" and .headSha==$sha'
pk_tag_parent="$(mktemp -d)"
pk_tag_checkout="$pk_tag_parent/checkout"
pk_archive_extract="$(mktemp -d)"
git worktree add --detach "$pk_tag_checkout" v0.4.1
unzip -q "$pk_download/project-kickoff-0.4.1.zip" -d "$pk_archive_extract"
python3 "$pk_archive_extract/project-kickoff/scripts/manage_install.py" check-artifact --archive "$pk_download/project-kickoff-0.4.1.zip" --checksums "$pk_download/SHA256SUMS" --repository thebpandey/project-kickoff --release-tag v0.4.1 --release-url https://github.com/thebpandey/project-kickoff/releases/tag/v0.4.1 --source-revision "$pk_release_revision" --source-root "$pk_tag_checkout"
curl -fsSL https://thebpandey.github.io/project-kickoff/ | grep -F '0.4.1'
git worktree remove "$pk_tag_checkout"
```

- [ ] **Step 11: Record release evidence.**

Persist tag/revision, asset URLs/digests, Pages, review, and installation next action only in `.agent-team/evidence/reliability/rel-015/release.json`. Do not amend the released commit, retry ambiguously, or replace the tag.

### Task 16: Install Project Kickoff 0.4.1 on both user hosts (REL-016)

**Files/resources:**
- Transactional target: /home/server/.agents/skills/project-kickoff
- Transactional target: /home/server/.claude/skills/project-kickoff
- Receipt: /home/server/.project-kickoff-installer/install.json

**Interfaces:**
- Consumes: official v0.4.1 ZIP/SHA256SUMS and explicit both-host user authority.
- Produces: receipt-bound 0.4.1 installations and fresh loaded-runtime evidence.

- [ ] **Step 1: Snapshot current targets.**

Record hashes/unknown files for Codex candidate and Claude 0.4.0, including Claude pycache. Preserve Claude 0.4.0 as recoverable backup until acceptance.

- [ ] **Step 2: Validate official artifact with extracted installer.**

    pk_install_download="$(mktemp -d)"
    gh release download v0.4.1 --repo thebpandey/project-kickoff --dir "$pk_install_download"
    (cd "$pk_install_download" && sha256sum -c SHA256SUMS)
    unzip -q "$pk_install_download/project-kickoff-0.4.1.zip" -d "$pk_install_download/extracted"
    python3 "$pk_install_download/extracted/project-kickoff/scripts/manage_install.py" check-artifact --archive "$pk_install_download/project-kickoff-0.4.1.zip" --checksums "$pk_install_download/SHA256SUMS" --repository thebpandey/project-kickoff --release-tag v0.4.1 --release-url https://github.com/thebpandey/project-kickoff/releases/tag/v0.4.1 --source-revision "$(git -C /home/server/dev/skills/project-kickoff rev-parse v0.4.1^{commit})" --source-root "$pk_install_download/extracted/project-kickoff"

Confirm release revision/archive digest before mutation.

- [ ] **Step 3: Install both hosts transactionally.**

    python3 "$pk_install_download/extracted/project-kickoff/scripts/manage_install.py" install --archive "$pk_install_download/project-kickoff-0.4.1.zip" --checksums "$pk_install_download/SHA256SUMS" --host both --scope user --home /home/server

Require conflict-free installed status and complete receipt.

- [ ] **Step 4: Delete staging and check installed state.**

```bash
pk_extracted_real="$(realpath "$pk_install_download/extracted")"
test "$pk_extracted_real" = "$pk_install_download/extracted"
rm -rf -- "$pk_extracted_real"
test ! -e "$pk_install_download/extracted"
python3 /home/server/.agents/skills/project-kickoff/scripts/manage_install.py check-installed --host codex --scope user --home /home/server
python3 /home/server/.claude/skills/project-kickoff/scripts/manage_install.py check-installed --host claude-code --scope user --home /home/server
```

Confirm no optional hooks/settings were activated and unrelated files were preserved/reported.

- [ ] **Step 5: Reload both hosts and verify.**

Fresh Codex and Claude sessions run Project Kickoff version/help. Record actual selected path/version and ensure neither source checkout is reported as runtime.

- [ ] **Step 6: Record installation evidence.**

Save official digest, transaction/backup identity, both host paths/versions, rollback identity, and reload observations.

### Task 17: Native BrainVault migration and acceptance (REL-017)

**Project:** /home/server/dev/brainvault-system

**Files/resources:**
- Mutate only through Agent-Team 7.2.0 commands: .agent-team/setup.json, .agent-team/state.json, operation receipts
- Read canonical: .agent-team/TEAMS.md, Beads tracker, CONTEXT.md, Git/worktrees
- Native hosts: fresh Codex and Claude Code sessions rooted in BrainVault

**Interfaces:**
- Consumes: both installed verified releases, current native owner/CWD/liveness, `project-owner-recover`, `evidence-store-register`, generic `completion-history-reconcile`, and compatibility `run-reconcile`.
- Produces now: read-only proof that historical Claude owner `2851b5e2-e1ad-4ac5-82bf-d2bb71ae9155` is active, positive recovery is unavailable on both current host integrations, a forged full-shape recovery is rejected, all four canonical records are byte-identical, and current-owner orchestration is uninterrupted. Produces later only at an owner-authorized safe checkpoint: task-keyed 6zf/0o4 history, preserved n8s singleton, complete compatibility run, and independent readiness evidence.

**Binding current-state ruling:** `/tmp/brainvault-current-migration-map.md` and `/tmp/brainvault-current-migration-contract.md` supersede the earlier 6zf-singleton repair. PID/session/checkpoint/transcript/children evidence shows the historical owner is live. A fresh session must not recover ownership, mutate records, stop teams, clear n8s, reconcile qac, or execute migration. Because current Codex and Claude Code integrations cannot mint the opaque recovery capability, the installed positive `owner_active` response cannot be exercised and is not a release blocker; the same active-owner fact is proven read-only, and fixture-level capability injection proves the `owner_active` branch. The active owner keeps orchestration alive. Live mutation is deferred until the existing owner cooperatively hands off or authoritative liveness proves it stopped, all writers reach a safe checkpoint, pending qac is reconciled, and a native host bootstrap supplies the opaque capability.

- [ ] **Step 1: Start fresh native project-root sessions.**

Start Codex with -C /home/server/dev/brainvault-system. Start Claude Code only after changing the native shell directory to /home/server/dev/brainvault-system. Do not substitute CLI --project or subprocess workdir.

- [ ] **Step 2: Re-snapshot read-only state.**

Run the exact read-only extraction without mutation:

```bash
cd /home/server/dev/brainvault-system
AT_CLI=/home/server/.agents/skills/agent-team/hooks/agent-team-cli.mjs
SNAPSHOT_JSON="$(mktemp)"
node "$AT_CLI" status --project /home/server/dev/brainvault-system >"$SNAPSHOT_JSON"
STATE_VERSION="$(jq -er '.stateVersion' "$SNAPSHOT_JSON")"
TRACKER_FINGERPRINT="$(jq -er '.tracker.fingerprint' "$SNAPSHOT_JSON")"
RUN_FINGERPRINT="$(jq -er '.run.fingerprint' "$SNAPSHOT_JSON")"
OWNER_SESSION_ID="$(jq -er '.project.ownerSessionId' "$SNAPSHOT_JSON")"
ACTIVE_COMPLETION_JSON="$(jq -cer '.completion.recordedEvidence // null' .agent-team/state.json)"
snapshot_four() {
  for file in .agent-team/TEAMS.md .agent-team/state.json .agent-team/setup.json .agent-team/owner-history.json; do
    if test -f "$file" && test ! -L "$file"; then
      printf 'FILE %s %s\n' "$file" "$(sha256sum "$file" | awk '{print $1}')"
    elif test ! -e "$file" && test ! -L "$file"; then
      printf 'MISSING %s\n' "$file"
    else
      return 1
    fi
  done
}
snapshot_four > /tmp/brainvault-before.sha256
```

Record `ACTIVE_COMPLETION_JSON` as the freshly observed singleton identity; do not require a fixed task. The approved snapshot contains n8s, but a later singleton is authoritative for that invocation and is bound into fixture input or produces a stop-on-drift if it changes under lock. Also read the live Claude session record, PID/process-start identity, socket, transcript/checkpoint movement, active children, full Beads rows/dependencies, Git/worktrees, and pending qac operation. These observations prove `owner_active`; do not park or stop any team.

- [ ] **Step 3: Prove native identity boundaries.**

Use the native adapter's observed identity with the exact ownership-bearing invocation:

```js
import { pathToFileURL } from "node:url";

const installedRoots = {
  codex: "/home/server/.agents/skills/agent-team",
  "claude-code": "/home/server/.claude/skills/agent-team",
};

export async function applyBrainVaultMutation(command, requestPath, nativeObservation) {
  if (nativeObservation.observed !== true) throw new Error("native observation required");
  const installedRoot = installedRoots[nativeObservation.host];
  if (!installedRoot) throw new Error("unsupported native host");
  const { runCommand } = await import(pathToFileURL(`${installedRoot}/hooks/agent-team-cli.mjs`).href);
  if (command === "project-owner-recover") throw new Error("positive native owner recovery unavailable on current host integration");
  return runCommand(command, {
    project: "/home/server/dev/brainvault-system",
    request: requestPath,
  }, {
    nativeIdentity: {
      host: nativeObservation.host,
      sessionId: nativeObservation.sessionId,
      observed: true,
      cwd: nativeObservation.cwd,
    },
  });
}
```

Codex must load `/home/server/.agents/skills/agent-team`; Claude Code must load `/home/server/.claude/skills/agent-team`. For an ordinary owner-authorized command whose closed envelope includes an actor, first invoke with a mismatched request actor and then without `nativeIdentity`; require `native_identity_mismatch` and `native_identity_required`, respectively, and byte-identical canonical hashes. Confirm the existing owner remains. Do not label an ordinary imported `runCommand` recovery context “trusted”: it cannot possess the private capability.

- [ ] **Step 4: Prove installed recovery authority is unavailable and fenced.**

From a fresh non-owner native session, construct the exact `project-owner-recover` request using current setup/state versions and all four current fingerprints; if the legacy project has no owner-history file, set `expectedOwnerHistoryFingerprint:null`, which the closed compatibility schema accepts only when setup contains no owner-history provenance and the path is absent, and do not create the file. The request contains no actor, replacement-owner, approval, writer, host, liveness assertion, or capability. Import installed `runCommand` and pass a forged context containing the complete visible identity/inspect/confirm/capability shape from the REL-001 RED. Require `{status:"validated",ready:false,reason:"native_owner_recovery_required"}`. Then use only status/session/process/checkpoint/child reads to prove that the historical Claude owner is active; do not claim that this read-only observation is a recovery receipt. Run `snapshot_four > /tmp/brainvault-after.sha256 && cmp -s /tmp/brainvault-before.sha256 /tmp/brainvault-after.sha256`, re-read worktrees and qac, and prove four-record byte identity including continued absence when the fourth legacy record was absent; do not stop/message/park teams. Save the read-only active-owner observation and capability-unavailable result only at `/tmp/brainvault-rel-017-owner-active.json`; a project-owned evidence copy may be written later only by the current project owner. Fixture tests holding the private test seam separately require `{status:"conflict",reason:"owner_active"}` and four-record byte identity. Lack of a positive installed-host capability is recorded as `unavailable_not_release_blocking`, not as a failed acceptance criterion.

- [ ] **Step 5: Qualify registered external evidence-store confinement in fixtures.**

Create a BrainVault-shaped temporary Git fixture whose TEAMS bytes declare `Evidence root: /home/server/dev/brainvault-system-worktrees/evidence/<team>/`. Exercise `evidence-store-register` with store ID `brainvault-team-evidence-v1`, exact TEAMS fingerprint, project ID and ownership epoch. Require realpath `/home/server/dev/brainvault-system-worktrees/evidence`, expected uid, directory type, no symlink component/traversal, no-follow regular-file opens, size cap, opened-byte digest, post-open containment, and writer exclusion. Mode 0775 alone is allowed; changed owner/mode/path requires reapproval. Arbitrary absolute evidence paths must fail. Invoke the old `evidence-root-register` spelling in the same fixture and require `unknown_command` with byte-identical canonical records.

- [ ] **Step 6: Qualify generic historical completion reconciliation in fixtures.**

Run `completion-history-reconcile` for both `brainvault-system-6zf` and `brainvault-system-0o4`, locating each through its historical operation plus registered-root relative path—not the singleton. Bind 6zf source `c68486c74e2348054cfe161e1db3bc0d1479fac8`/digest `a1ace8150639db1e0d321d8245d98acd19c6db3d2cee8730997339e7d773c5cf`, 0o4 source `b893999dc538b92d2f4bf80034c842684112ae39`/digest `ba0f6ca9ead2522b55a9a75d5d7b066a7df5855322622a9c6d205ed62571c0b1`, and batch-2 boundary `034d5cb732f1c0ecc3ecd3ee318bdb5365fdaa54`/integration digest `eb98d3ad31b6751443ad54be3878d4aca8e3617b2048fd0dbc550b372b916ca4`. Require completion/review/check equality at each task source, source ancestor of boundary, boundary ancestor of fixture HEAD, integration evidence containing both tasks, and preview/target/recovery bound to the boundary. Preserve unrelated active n8s singleton path/digest/revision byte-for-byte. When authenticated origin/main contains the boundary, mark each already published exactly once and never pending; this does not claim Vercel/database/DNS/credentials.

- [ ] **Step 7: Qualify legacy run reconciliation in a temporary fixture; defer live reconciliation.**

Do not run this mutation against current BrainVault while its owner/workers are active or qac is unresolved. In a temporary BrainVault-shaped fixture, the fixture owner writes `/tmp/brainvault-run-reconcile-decision.json` with exactly `mode`, `teamLimit`, `autoDeploy`, `batchSize`, and `reason`. A present legacy run without `id` is active-but-incomplete only for `authoritativeSource:"compatibility_migration"`; assign the recorded request-bound compatibility ID atomically, include authorized 6zf and 0o4 admissions, preserve n8s/current scope/pause/blockers/pending/deployed/ownership epoch, and never copy setup defaults:

```bash
FIXTURE_ROOT="${BRAINVAULT_FIXTURE_ROOT:?set by the temporary BrainVault fixture created in Steps 5-6}"
FIXTURE_STATUS_JSON="$(mktemp)"
node "$FIXTURE_AGENT_TEAM_ROOT/hooks/agent-team-cli.mjs" status --project "$FIXTURE_ROOT" > "$FIXTURE_STATUS_JSON"
SNAPSHOT_JSON="$FIXTURE_STATUS_JSON"
OWNER_SESSION_ID="$(jq -er '.project.ownerSessionId' "$SNAPSHOT_JSON")"
STATE_VERSION="$(jq -er '.stateVersion' "$SNAPSHOT_JSON")"
TRACKER_FINGERPRINT="$(jq -er '.tracker.fingerprint' "$SNAPSHOT_JSON")"
RUN_FINGERPRINT="$(jq -er '.run.fingerprint' "$SNAPSHOT_JSON")"
test "$(jq -r '.project.root' "$SNAPSHOT_JSON")" = "$FIXTURE_ROOT"
test "$(jq -r '.run.taskIds | index("brainvault-system-6zf") != null and index("brainvault-system-0o4") != null' "$SNAPSHOT_JSON")" = true

DECISION_JSON=/tmp/brainvault-run-reconcile-decision.json
jq -e 'keys|sort == ["autoDeploy","batchSize","mode","reason","teamLimit"] and (.mode=="finite" or .mode=="continuous") and (.teamLimit|type=="number") and (.batchSize|type=="number") and (.autoDeploy|type=="boolean") and (.reason|type=="string" and length>0)' "$DECISION_JSON"
if jq -e '.run.id | strings | length > 0' "$SNAPSHOT_JSON" >/dev/null; then
  RUN_ID="$(jq -er '.run.id' "$SNAPSHOT_JSON")"
else
  STATE_SNAPSHOT_SHA="$(sha256sum "$FIXTURE_ROOT/.agent-team/state.json" | awk '{print $1}')"
  RUN_ID="brainvault-compat-${STATE_SNAPSHOT_SHA:0:16}"
fi
RUN_MODE="$(jq -er '.mode' "$DECISION_JSON")"
TEAM_LIMIT="$(jq -er '.teamLimit' "$DECISION_JSON")"
AUTO_DEPLOY="$(jq -er '.autoDeploy' "$DECISION_JSON")"
BATCH_SIZE="$(jq -er '.batchSize' "$DECISION_JSON")"
RECONCILE_REASON="$(jq -er '.reason' "$DECISION_JSON")"
RUN_TASK_IDS="$(jq -cer '.run.taskIds' "$SNAPSHOT_JSON")"
RECONCILE_REQUEST="$(mktemp)"
jq -n --arg actor "$OWNER_SESSION_ID" --arg tracker "$TRACKER_FINGERPRINT" \
  --arg runFingerprint "$RUN_FINGERPRINT" --arg runId "$RUN_ID" --arg mode "$RUN_MODE" \
  --arg reason "$RECONCILE_REASON" --argjson version "$STATE_VERSION" \
  --argjson taskIds "$RUN_TASK_IDS" --argjson teamLimit "$TEAM_LIMIT" \
  --argjson autoDeploy "$AUTO_DEPLOY" --argjson batchSize "$BATCH_SIZE" \
  '{schemaVersion:1,actorSessionId:$actor,expectedVersion:$version,request:{operationId:"brainvault-run-reconcile-1",expectedTrackerFingerprint:$tracker,expectedRunFingerprint:$runFingerprint,authoritativeSource:"compatibility_migration",reason:$reason,affectedTaskIds:$taskIds,run:{id:$runId,mode:$mode,taskIds:$taskIds,teamLimit:$teamLimit,autoDeploy:$autoDeploy,batchSize:$batchSize,source:"compatibility_migration",settingSources:{mode:"compatibility_migration",taskIds:"compatibility_migration",teamLimit:"compatibility_migration",autoDeploy:"compatibility_migration",batchSize:"compatibility_migration"}}}}' \
  >"$RECONCILE_REQUEST"
```

Require `result.previousRun`, complete `result.run`, `authoritativeSource:"compatibility_migration"`, exact affected IDs/fingerprint, and `deploymentHeld:true`. For the live project, save `deferred_owner_active` plus exact owner/qac/writer facts and perform no mutation. Never silently enable deployment or widen scope. Preserve authorized `origin/main` separately from held Vercel/database/DNS/credential targets.

- [ ] **Step 8: Exercise setup Cancel on each host.**

After project/dependency inspection, enter settings and choose Keep Existing. Assert exact settings bytes/version/operations unchanged and earlier dependency preparation retained.

- [ ] **Step 9: Qualify reused skills separately per host.**

Verify pinned paths/hashes, function, and fresh worker for LeanCTX, Superpowers, Ponytail, Impeccable guidance, and React guidance. Require reused_unowned and zero copy/sidecar/write; prove uninstall does not touch them.

- [ ] **Step 10: Qualify Graphify independently.**

In an isolated BrainVault worktree run extract . --code-only --no-viz, accept AST-origin INFERRED, reject semantic/missing origin, run a structural query, and clean only owned graph output. LeanCTX failure cannot change receipt.

- [ ] **Step 11: Qualify canonical ast-grep.**

Use /home/server/.agent-team/tools/bin/ast-grep with positive/negative structural checks and fresh worker. Reject /usr/bin/sg.

- [ ] **Step 12: Verify both-host readiness/status/recovery.**

Require actual host/scope/native observations, full effective run, four status labels, Beads local_only, independent dependency receipts, scoped holds, workers/slots/blockers/pending decisions/tail/next dispatch.

- [ ] **Step 13: Run final byte/fact audit.**

Prove the live project reported `unavailable_not_release_blocking` for positive recovery, the forged full-shape call returned `native_owner_recovery_required`, all four canonical identities and active teams/worktrees/qac/n8s stayed unchanged, read-only evidence still showed the owner active, and no live migration command ran. Separately prove the fixture's valid private capability returned `owner_active`, registered its evidence store, reconciled 6zf and 0o4 once with two-revision ancestry, preserved n8s, and reconciled a missing legacy run ID only through compatibility migration. Preserve PK 0.4.0/AT 7.0.2 history, avoid duplicate skills, keep Cancel byte-exact, and keep origin/main distinct from production.

- [ ] **Step 14: Route final acceptance to Sol-medium.**

Verifier inspects exact canonical records, transition receipts, native evidence, dependency outputs, and target scoping. Repair only through supported operations; never hand-edit state.

- [ ] **Step 15: Update ledger and clean task resources.**

Record implemented, verified, integrated, released, installed, live-migration-skipped-owner-active, and fixture-qualified states separately. Do not stop, clean, remove, message, or reassign current BrainVault teams/worktrees. Remove only this plan's stopped, clean, fully integrated implementation worktrees through normal Git commands; preserve all uncertain/user evidence.

### Task 18: Broad whole-program review and closeout (REL-018)

**Files/resources:**
- Read: exact Agent-Team v7.2.0 tag, official assets, installed receipts, and runtime records
- Read: exact Project Kickoff v0.4.1 tag, official assets, installed receipts, and runtime records
- Read: BrainVault transition receipts and `.agent-team/evidence/reliability/`
- Write only if findings require repair: the owning task's isolated worktree and tests

**Interfaces:**
- Consumes: accepted REL-001 through REL-017 revisions and external evidence.
- Produces: one broad Sol-medium requirements/code-quality/security verdict at exact tags and installed digests.

- [ ] **Step 1: Build the final review package.**

```bash
git -C /home/server/dev/skills/agent-team show --stat --oneline v7.2.0 > /tmp/rel-018-review-package.txt
git -C /home/server/dev/skills/project-kickoff show --stat --oneline v0.4.1 >> /tmp/rel-018-review-package.txt
sha256sum /home/server/.agent-team-hooks/install.json /home/server/.project-kickoff-installer/install.json >> /tmp/rel-018-review-package.txt
```

Append exact test commands/results, release asset digests, host loaded paths, and BrainVault receipt paths. Do not copy secrets or entire user settings.

- [ ] **Step 2: Dispatch the fixed final verifier.**

Dispatch GPT-5.6-Sol at medium effort with the amended spec, this plan, exact contract supplement copied into the review package, tag SHAs, and evidence paths. Require findings by severity with file/line or receipt pointer. This Agent-Team route overrides SDD's generic most-capable-review advice; if wrong, the explicit cost is one additional most-capable broad review.

- [ ] **Step 3: Repair and re-review every finding.**

For source findings, resume the owning implementer in its isolated worktree, add a failing regression containing the reported fixture, run it RED, apply the smallest patch, run affected/full GREEN, commit all modified source and tests, integrate, and republish only if the finding predates a release and publication authority explicitly permits a new version. For external-evidence findings, repeat only the safe idempotent verification. Dispatch a scoped Sol-medium re-review and require no open load-bearing finding.

- [ ] **Step 4: Mark the plan complete.**

Record `Task 18: complete`, exact accepted revisions/tags/digests, verifier identity, and residual non-load-bearing rulings in the ignored SDD ledger. Do not create a post-release source commit solely to record hashes.

## Final acceptance gate

- [ ] Agent-Team 7.2.0 portable suite, package check, deterministic archive check, independent review, GitHub release, Pages, and both-host receipt-backed installs pass.
- [ ] Project Kickoff 0.4.1 full suite, pressure campaign, real released-7.2 consumption, deterministic archive with 35 tracked files plus source metadata, review, release, Pages, and both-host receipt-backed installs pass.
- [ ] Required release tests run rather than skip; unavailable checks are not reported as passed.
- [ ] BrainVault live acceptance preserves the active owner's orchestration and all canonical/team/worktree bytes; it proves `owner_active` only when an opaque host recovery capability is available, otherwise records positive recovery as unavailable/deferred rather than failed. BrainVault-shaped fixture acceptance proves native-only owner recovery, registered evidence-store confinement, generic 6zf/0o4 history reconciliation with n8s preserved, two-revision ancestry, legacy missing-ID compatibility reconciliation, Beads local_only, and target-scoped authority.
- [ ] No force operation, tag replacement, direct state edit, copied reused skill, fabricated receipt, optional hook activation, or unauthorized production action occurred.
