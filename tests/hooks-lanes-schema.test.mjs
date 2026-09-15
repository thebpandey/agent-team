import assert from "node:assert/strict";
import test from "node:test";

async function lanesModule() {
  try {
    return await import("../hooks/lib/lanes.mjs");
  } catch {
    return {};
  }
}

const worker = {
  host: "codex",
  sessionId: "worker-session",
  generation: 1,
};

const lane = (extra = {}) => ({
  schemaVersion: 1,
  id: "build-a",
  teamId: "TEAM-BUILD-A",
  status: "prepared",
  role: "developer",
  model: "gpt-6-astra",
  effort: "high",
  queue: ["AT-001", "AT-002"],
  currentTaskId: null,
  worker,
  worktree: "/tmp/project-lane-build-a",
  branch: "lane/build-a",
  brief: { path: ".agent-team/lanes/build-a/BRIEF.md", sha256: "a".repeat(64) },
  ownershipEvidence: { path: ".agent-team/lanes/build-a/evidence/ownership.json", sha256: "b".repeat(64),
    revision: "c".repeat(40), pathSetHash: "d".repeat(64) },
  rotationCount: 0,
  handover: null,
  factSheets: [],
  assignments: [],
  results: [],
  ...extra,
});

const collection = (records = [lane()]) => ({ schemaVersion: 1, records });

test("legacy state has no lanes while malformed present lane state fails closed", async () => {
  const { readLaneCollection, validateLaneCollection } = await lanesModule();
  assert.equal(typeof readLaneCollection, "function");
  assert.equal(typeof validateLaneCollection, "function");
  assert.deepEqual(readLaneCollection({}), []);
  assert.equal(validateLaneCollection(collection()), undefined);

  const invalid = [
    null,
    [],
    { schemaVersion: 1, records: [], extra: true },
    { schemaVersion: 2, records: [] },
    collection([lane(), lane()]),
    collection([{ ...lane(), extra: true }]),
    collection([lane({ id: "../escape", branch: "lane/../escape" })]),
    collection([lane({ branch: "feature/build-a" })]),
    collection([lane({ queue: ["AT-001", "AT-001"] })]),
    collection([lane({ currentTaskId: "AT-999" })]),
    collection([lane({ worktree: "/tmp/project/../escape" })]),
    collection([lane({ brief: { path: "../BRIEF.md", sha256: "a".repeat(64) } })]),
    collection([lane({ ownershipEvidence: { ...lane().ownershipEvidence, revision: "current" } })]),
    collection([lane({ role: "orchestrator" })]),
    collection([lane({ worker: { ...worker, host: "local-hostname" } })]),
    collection([lane(), lane({ id: "build-b", teamId: "TEAM-BUILD-B", queue: ["AT-003"], branch: "lane/build-b",
      brief: { path: ".agent-team/lanes/build-b/BRIEF.md", sha256: "a".repeat(64) },
      ownershipEvidence: { path: ".agent-team/lanes/build-b/evidence/ownership.json", sha256: "b".repeat(64), revision: "c".repeat(40), pathSetHash: "d".repeat(64) } })]),
  ];
  for (const value of invalid) assert.equal(validateLaneCollection(value), "invalid_lanes");
});

test("fact sheets have one owning lane and assignments bind exact cited facts", async () => {
  const { validateLaneCollection } = await lanesModule();
  const sheet = {
    id: "api-contract",
    ownerLaneId: "build-a",
    path: ".agent-team/lanes/build-a/facts/api-contract.json",
    sha256: "6".repeat(64),
    facts: [{ id: "api-version", checkedOn: "2026-09-14T12:00:00.000Z", sourceUrl: "https://example.test/api" }],
  };
  const assignment = {
    id: "assignment-at-001-1", taskId: "AT-001", attempt: 1,
    packet: { path: ".agent-team/lanes/build-a/packets/AT-001-1.md", sha256: "7".repeat(64) },
    briefSha256: "a".repeat(64), revision: "f".repeat(40), worker,
    decisions: [], factSheets: [{ id: sheet.id, sha256: sheet.sha256, factIds: ["api-version"],
      citation: { path: ".agent-team/lanes/build-a/evidence/citations/AT-001-api-contract.json", sha256: "8".repeat(64) } }],
    status: "prepared", createdAt: "2026-09-14T13:00:00.000Z", dispatchedAt: null, dispatch: null,
  };
  assert.equal(validateLaneCollection(collection([lane({ factSheets: [sheet], assignments: [assignment] })])), undefined);
  assert.equal(validateLaneCollection(collection([lane({ factSheets: [sheet] }), lane({
    id: "build-b", teamId: "TEAM-BUILD-B", queue: ["AT-003"], worktree: "/tmp/project-lane-build-b", branch: "lane/build-b",
    brief: { path: ".agent-team/lanes/build-b/BRIEF.md", sha256: "9".repeat(64) },
    ownershipEvidence: { path: ".agent-team/lanes/build-b/evidence/ownership.json", sha256: "b".repeat(64),
      revision: "c".repeat(40), pathSetHash: "d".repeat(64) },
    factSheets: [{ ...sheet, ownerLaneId: "build-b", path: ".agent-team/lanes/build-b/facts/api-contract.json" }],
  })])), "invalid_lanes");
});

test("canonical lane history rejects duplicate attempts results decisions and contradictory lifecycle state", async () => {
  const { validateLaneCollection } = await lanesModule();
  const assignment = {
    id: "assignment-at-001-1", taskId: "AT-001", attempt: 1,
    packet: { path: ".agent-team/lanes/build-a/packets/AT-001-1.md", sha256: "7".repeat(64) },
    briefSha256: "a".repeat(64), revision: "f".repeat(40), worker,
    decisions: [{ id: "DEC-001", kind: "new", supersedes: null }], factSheets: [], status: "resolved",
    createdAt: "2026-09-14T13:00:00.000Z", dispatchedAt: "2026-09-14T13:01:00.000Z",
    dispatch: { status: "observed", source: "codex", eventId: "dispatch-1", observedAt: "2026-09-14T13:01:00.000Z" },
  };
  const result = (kind, status = "passed") => ({ assignmentId: assignment.id, taskId: assignment.taskId, kind, status,
    revision: assignment.revision, worker, evidence: { path: `.agent-team/lanes/build-a/evidence/${kind}.json`, sha256: "8".repeat(64) },
    recordedAt: "2026-09-14T13:02:00.000Z" });
  const base = lane({ status: "active", currentTaskId: "AT-001", assignments: [assignment], results: [result("worker")] });
  const duplicateAttempt = { ...assignment, id: "assignment-at-001-duplicate", packet: {
    path: ".agent-team/lanes/build-a/packets/AT-001-1.md", sha256: "9".repeat(64) } };
  const duplicateDecision = { ...assignment, id: "assignment-at-002-1", taskId: "AT-002", packet: {
    path: ".agent-team/lanes/build-a/packets/AT-002-1.md", sha256: "9".repeat(64) } };
  const invalid = [
    { ...base, assignments: [assignment, duplicateAttempt] },
    { ...base, results: [result("worker"), result("worker")] },
    { ...base, assignments: [assignment, duplicateDecision], results: [result("worker")] },
    { ...base, assignments: [{ ...assignment, decisions: [{ id: "DEC-002", kind: "supersedes", supersedes: "DEC-404" }] }] },
    { ...base, assignments: [{ ...assignment, status: "prepared", dispatchedAt: null, dispatch: null }], results: [result("worker")] },
    { ...base, assignments: [{ ...assignment, dispatch: { status: "unknown", source: "unavailable", eventId: null,
      observedAt: assignment.dispatchedAt } }], results: [result("worker")] },
  ];
  for (const record of invalid) assert.equal(validateLaneCollection(collection([record])), "invalid_lanes");
});

test("lane reads clone state and canonical fingerprint binds all durable fields", async () => {
  const { laneCollectionFingerprint, laneFingerprint, laneOwnsTask, readLaneCollection } = await lanesModule();
  assert.equal(typeof laneFingerprint, "function");
  assert.equal(typeof laneOwnsTask, "function");
  assert.equal(typeof laneCollectionFingerprint, "function");
  const state = { lanes: collection() };
  const read = readLaneCollection(state);
  read[0].queue.push("AT-003");
  assert.deepEqual(state.lanes.records[0].queue, ["AT-001", "AT-002"]);
  assert.match(laneFingerprint(lane()), /^[a-f0-9]{64}$/);
  assert.equal(laneFingerprint(lane()), laneFingerprint(structuredClone(lane())));
  assert.notEqual(laneFingerprint(lane()), laneFingerprint(lane({ rotationCount: 1 })));
  assert.match(laneCollectionFingerprint(collection()), /^[a-f0-9]{64}$/);
  assert.equal(laneOwnsTask(lane(), "AT-002"), true);
  assert.equal(laneOwnsTask(lane(), "AT-999"), false);
});

test("lane summary separates logical concurrency from native reviewer capacity and unknown occupancy", async () => {
  const { laneFingerprint, summarizeLanes } = await lanesModule();
  assert.equal(typeof summarizeLanes, "function");
  const review = lane({
    id: "review-a",
    teamId: "TEAM-REVIEW-A",
    role: "reviewer",
    queue: ["AT-001"],
    worker: { host: "claude-code", sessionId: "review-session", generation: 2 },
    worktree: "/tmp/project-lane-review-a",
    branch: "lane/review-a",
    brief: { path: ".agent-team/lanes/review-a/BRIEF.md", sha256: "b".repeat(64) },
    ownershipEvidence: { path: ".agent-team/lanes/review-a/evidence/ownership.json", sha256: "c".repeat(64),
      revision: "d".repeat(40), pathSetHash: "e".repeat(64) },
  });
  const summary = summarizeLanes({
    state: { lanes: collection([lane(), review]), run: { teamLimit: 2 } },
    git: { headRevision: "c".repeat(40) },
  }, {
    writerLiveness: { "build-a": { ...worker, status: "active" } },
    effectiveSettings: { lanes: { rotation: { tasks: 2 } } },
    nativeCapacity: { limit: 4, active: 2, reservedReview: 1 },
  });

  assert.deepEqual(summary.logical, { limit: 2, occupied: 1, free: 1 });
  assert.deepEqual(summary.native, { limit: 4, occupied: 2, reservedReview: 1, unknown: 1, free: 0 });
  assert.deepEqual(summary.rows.map(({ id, liveness, remainingTaskIds, rotationDue }) => ({ id, liveness, remainingTaskIds, rotationDue })), [
    { id: "build-a", liveness: "active", remainingTaskIds: ["AT-001", "AT-002"], rotationDue: false },
    { id: "review-a", liveness: "unknown", remainingTaskIds: ["AT-001"], rotationDue: false },
  ]);
  assert.deepEqual(summary.rows[0].brief, lane().brief);
  assert.equal(summary.rows[0].laneFingerprint, laneFingerprint(lane()));
  assert.equal(summarizeLanes({ state: { lanes: collection([lane()]), run: { teamLimit: 2 } } }, {
    effectiveSettings: { lanes: { rotation: { tasks: 2 } } },
  }).native, null);
});

test("lane cleanup blocks an open or incomplete shared lane and releases only a closed verified history", async () => {
  const { laneCleanupBlocker } = await lanesModule();
  assert.equal(typeof laneCleanupBlocker, "function");
  const worktree = "/tmp/project-lane-build-a";
  assert.equal(laneCleanupBlocker({ state: {} }, { taskId: "AT-001", worktree }), null);
  assert.equal(laneCleanupBlocker({ state: { lanes: collection([lane()]) } }, { taskId: "AT-001", worktree }), "lane_open");
  const assignment = {
    id: "assignment-at-001-1",
    taskId: "AT-001",
    attempt: 1,
    packet: { path: ".agent-team/lanes/build-a/packets/AT-001-1.md", sha256: "e".repeat(64) },
    briefSha256: "a".repeat(64),
    revision: "f".repeat(40),
    worker,
    decisions: [],
    factSheets: [],
    status: "resolved",
    createdAt: "2026-09-14T13:00:00.000Z",
    dispatchedAt: "2026-09-14T13:01:00.000Z",
    dispatch: { status: "observed", source: "codex", eventId: "dispatch-event-1", observedAt: "2026-09-14T13:01:00.000Z" },
  };
  const result = (kind, resultWorker = worker) => ({
    assignmentId: assignment.id,
    taskId: assignment.taskId,
    kind,
    status: "passed",
    revision: assignment.revision,
    worker: resultWorker,
    evidence: { path: `.agent-team/lanes/${kind === "independent_review" ? "review-a" : "build-a"}/evidence/${kind}.json`, sha256: "1".repeat(64) },
    recordedAt: "2026-09-14T14:00:00.000Z",
  });
  const workerResult = result("worker");
  const verifier = { host: "codex", sessionId: "verifier-session", generation: 1 };
  const reviewer = { host: "codex", sessionId: "reviewer-session", generation: 1 };
  const closed = lane({
    status: "closed",
    queue: ["AT-001"],
    worker: null,
    assignments: [assignment],
    results: [workerResult, result("verification", verifier), result("independent_review", reviewer), result("integration")],
  });
  const reviewLaneFor = (author) => {
    const linked = author.results.find(({ kind }) => kind === "independent_review");
    const source = linked && author.assignments.find(({ id }) => id === linked.assignmentId);
    const reviewAssignment = source && { id: `review-${source.id}`, taskId: source.taskId, attempt: 1,
      packet: { path: `.agent-team/lanes/review-a/packets/${source.taskId}-1.md`, sha256: "3".repeat(64) },
      briefSha256: "4".repeat(64), revision: source.revision, worker: reviewer, decisions: [], factSheets: [],
      sourceAssignment: { laneId: author.id, assignmentId: source.id, packetSha256: source.packet.sha256, revision: source.revision },
      status: "resolved", createdAt: "2026-09-14T13:00:00.000Z", dispatchedAt: "2026-09-14T13:01:00.000Z",
      dispatch: { status: "observed", source: "codex", eventId: `review-${source.id}`, observedAt: "2026-09-14T13:01:00.000Z" } };
    return lane({ id: "review-a", teamId: "TEAM-REVIEW-A", status: "closed", role: "reviewer", queue: ["AT-001"], currentTaskId: null,
      worker: null, worktree: "/tmp/project-lane-review-a", branch: "lane/review-a",
      brief: { path: ".agent-team/lanes/review-a/BRIEF.md", sha256: "4".repeat(64) }, ownershipEvidence: {
        path: ".agent-team/lanes/review-a/evidence/ownership.json", sha256: "5".repeat(64), revision: "c".repeat(40), pathSetHash: "6".repeat(64) },
      assignments: reviewAssignment ? [reviewAssignment] : [], results: reviewAssignment ? [{ ...linked, assignmentId: reviewAssignment.id }] : [] });
  };
  const canonical = (author, deliveryEvidence) => ({ state: { lanes: collection([author, reviewLaneFor(author)]) }, deliveryEvidence });
  const accepted = { sourceRevision: assignment.revision, integratedRevision: assignment.revision };
  assert.equal(laneCleanupBlocker(canonical(closed, { "AT-001": accepted }), { taskId: "AT-001", worktree }), null);
  assert.equal(laneCleanupBlocker(canonical(closed, {}), { taskId: "AT-001", worktree }), "lane_tasks_not_integrated");
  assert.equal(laneCleanupBlocker(canonical({ ...closed, results: closed.results.filter(({ kind }) => !["independent_review", "integration"].includes(kind)) }, { "AT-001": accepted }),
    { taskId: "AT-001", worktree }), "lane_review_incomplete");
  const later = { ...assignment, id: "assignment-at-001-2", attempt: 2,
    packet: { path: ".agent-team/lanes/build-a/packets/AT-001-2.md", sha256: "2".repeat(64) } };
  const unresolvedOld = { ...closed, assignments: [{ ...assignment, status: "prepared", dispatchedAt: null, dispatch: null }, later],
    results: closed.results.map((entry) => ({ ...entry, assignmentId: later.id })) };
  assert.equal(laneCleanupBlocker(canonical(unresolvedOld, { "AT-001": accepted }),
    { taskId: "AT-001", worktree }), "lane_assignment_unresolved");
  const unknownResult = { ...result("unresolved"), status: "unresolved", assignmentId: assignment.id };
  const uncertainOld = { ...closed, assignments: [{ ...assignment, status: "dispatched" }, later],
    results: [unknownResult, ...closed.results.map((entry) => ({ ...entry, assignmentId: later.id }))] };
  assert.equal(laneCleanupBlocker(canonical(uncertainOld, { "AT-001": accepted }),
    { taskId: "AT-001", worktree }), "lane_assignment_unresolved");
  const failedAssignment = { ...assignment, status: "resolved" };
  const failed = { ...closed, assignments: [failedAssignment, later],
    results: [{ ...result("worker"), status: "failed" }, ...closed.results.map((entry) => ({ ...entry, assignmentId: later.id }))] };
  assert.equal(laneCleanupBlocker(canonical(failed, { "AT-001": accepted }),
    { taskId: "AT-001", worktree }), "lane_assignment_unresolved");
});

const packet = `# Lane task packet
Assignment: assignment-at-001-1
Task: AT-001
Attempt: 1
Revision: ${"f".repeat(40)}
Tracker fingerprint: ${"2".repeat(64)}
Lane fingerprint: ${"3".repeat(64)}
Brief fingerprint: ${"a".repeat(64)}

## Acceptance
Implement the exact bounded behavior and preserve existing gates.

## Delta
No prior lane result. Start from the immutable brief context.

## New decisions
- DEC-001 | new | none

## Revision pointers
Use the source, tracker, lane, and brief fingerprints above.
`;

test("task packets bind assignment revision and stable decision changes without repeating the brief", async () => {
  const { readLanePacket, validateLanePacket } = await lanesModule();
  assert.equal(typeof readLanePacket, "function");
  assert.equal(typeof validateLanePacket, "function");
  const expected = { assignmentId: "assignment-at-001-1", taskId: "AT-001", attempt: 1, revision: "f".repeat(40),
    trackerFingerprint: "2".repeat(64), laneFingerprint: "3".repeat(64), briefSha256: "a".repeat(64) };
  assert.equal(validateLanePacket(packet, { expected }), undefined);
  assert.deepEqual(readLanePacket(packet, { expected }).decisions, [{ id: "DEC-001", kind: "new", supersedes: null }]);
  assert.equal(validateLanePacket(packet.replace("## Delta", "## Brief"), { expected }), "invalid_lane_packet");
  assert.equal(validateLanePacket(packet.replace("DEC-001 | new | none", "DEC-001 | supersedes | DEC-404"), { expected }), "decision_reversal");
  assert.equal(validateLanePacket(packet, { expected, priorDecisions: [{ id: "DEC-001", kind: "new", supersedes: null }] }), "decision_reversal");
  const noDecision = packet.replace("- DEC-001 | new | none", "None.");
  assert.equal(validateLanePacket(noDecision, { expected }), undefined);
  assert.deepEqual(readLanePacket(noDecision, { expected }).decisions, []);
  assert.equal(validateLanePacket(`${packet}\n${"word ".repeat(400)}`, { expected }), "lane_packet_too_large");
});

const brief = `# Build lane A

## Role / model / effort
developer / gpt-6-astra / high

## Queue
AT-001, AT-002

## Shared rules
Preserve tracker authority and exact revision evidence.

## Writable paths
hooks/lib/lanes.mjs and its focused tests.

## Required skills
/home/server/.agents/skills/test-driven-development/SKILL.md ${"f".repeat(64)}

## Applicable instructions
AGENTS.md ${"e".repeat(64)}

## Context revision
${"c".repeat(40)}

## Evidence destination
.agent-team/lanes/build-a/evidence/

## Handoff format
State revision, checks, remaining risk, and one next action.
`;

test("lane brief requires every nonempty contract section and enforces the configured cap", async () => {
  const { validateLaneBrief } = await lanesModule();
  assert.equal(typeof validateLaneBrief, "function");
  assert.equal(validateLaneBrief(brief, { maxWords: 80 }), undefined);
  assert.equal(validateLaneBrief(brief.replace("Preserve tracker authority and exact revision evidence.", ""), { maxWords: 80 }), "invalid_lane_brief");
  assert.equal(validateLaneBrief(brief.replace(`${"e".repeat(64)}`, "unhashed"), { maxWords: 80 }), "invalid_lane_brief");
  assert.equal(validateLaneBrief(brief.replace(`${"c".repeat(40)}`, "unknown"), { maxWords: 80 }), "invalid_lane_brief");
  assert.equal(validateLaneBrief(brief.replace(/\n## Handoff format[\s\S]*/, ""), { maxWords: 80 }), "invalid_lane_brief");
  assert.equal(validateLaneBrief(`${brief}\n${"word ".repeat(80)}`, { maxWords: 80 }), "lane_brief_too_large");
});

const handover = `# Lane handover

## Voice
Continue as the retained implementation lane and report exact evidence.

## Gotchas
The tracker is the sole task authority.

## Open threads
Resume the current assignment without a second claim.

## Decisions
DEC-001 remains in force.

## Exact revision
${"f".repeat(40)}
`;

test("lane handover binds sequence revision and digest under the word cap", async () => {
  const { validateLaneHandover } = await lanesModule();
  assert.equal(typeof validateLaneHandover, "function");
  const expected = { laneId: "build-a", sequence: 1, revision: "f".repeat(40) };
  assert.equal(validateLaneHandover(handover, { expected }), undefined);
  assert.equal(validateLaneHandover(handover.replace("## Decisions", "## Notes"), { expected }), "invalid_lane_handover");
  assert.equal(validateLaneHandover(handover.replace("f".repeat(40), "e".repeat(40)), { expected }), "invalid_lane_handover");
  assert.equal(validateLaneHandover(`${handover}\n${"word ".repeat(500)}`, { expected }), "lane_handover_too_large");
});
