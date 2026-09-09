import assert from "node:assert/strict";
import test from "node:test";

test("approved Kickoff handoff reaches an eligible task without replacing develop or Beads", async () => {
  const { assessReadiness } = await import("../hooks/lib/readiness.mjs").catch(() => ({}));
  const tracker = { kind: "beads", status: "current" };
  const result = assessReadiness?.({
    kickoff: {
      status: "approved",
      handoff: {
        scope: "Add import validation",
        acceptance: ["Invalid rows are rejected"],
        tasks: [{ id: "PK-7", title: "Add validation", status: "ready", dependencies: [] }],
        branch: "develop",
        tracker,
        verification: ["node --test"],
        authority: { writes: ["src/import.mjs"], deployment: false },
        requiredCapabilities: ["node"],
      },
    },
    capabilities: { node: { functional: "passed", availableToWorker: "passed" } },
  });

  assert.equal(result.path, "kickoff");
  assert.equal(result.eligible, true);
  assert.equal(result.branch, "develop");
  assert.strictEqual(result.tracker, tracker);
  assert.deepEqual(result.eligibleTask, { id: "PK-7", title: "Add validation", status: "ready", dependencies: [] });
  assert.equal(result.planning, "reused");
  assert.deepEqual(result.missing, []);
});

test("malformed task data and partially verified capabilities remain specific readiness gaps", async () => {
  const { assessReadiness } = await import("../hooks/lib/readiness.mjs");
  const result = assessReadiness({
    existing: {
      plan: {
        scope: "Inspect symbols", acceptance: ["Symbol is found"], tasks: null, branch: "main",
        verification: ["node --test"], authority: { writes: [] }, requiredCapabilities: ["serena"],
      },
      tracker: { kind: "markdown", path: "TASKS.md", status: "current" },
    },
    capabilities: { serena: { functional: "passed", availableToWorker: "unknown" } },
  });

  assert.equal(result.eligible, false);
  assert.deepEqual(result.missing.map(({ id }) => id), ["actionable_tasks", "capability:serena"]);
});

test("existing non-Kickoff plan is adopted without a second tracker", async () => {
  const { assessReadiness } = await import("../hooks/lib/readiness.mjs");
  const tracker = { kind: "markdown", path: "TASKS.md", status: "current" };
  const result = assessReadiness({
    existing: {
      plan: {
        scope: "Repair parser",
        acceptance: ["Regression fixture passes"],
        tasks: [{ id: "T-2", title: "Repair parser", status: "ready", dependencies: [] }],
        branch: "release/next",
        verification: ["node --test tests/parser.test.mjs"],
        authority: { writes: ["parser.mjs"] },
      },
      tracker,
    },
  });

  assert.equal(result.path, "existing");
  assert.equal(result.planning, "adopted");
  assert.strictEqual(result.tracker, tracker);
  assert.equal(result.branch, "release/next");
  assert.equal(result.eligibleTask.id, "T-2");
});

test("standalone request creates only the proportionate execution state it needs", async () => {
  const { assessReadiness } = await import("../hooks/lib/readiness.mjs");
  const result = assessReadiness({
    request: { summary: "Fix empty email validation", acceptance: ["Empty email returns Email required"] },
    project: { currentBranch: "main", projectOwner: "owner-1", verification: ["node --test tests/email.test.mjs"], authority: { writes: ["email.mjs"] } },
  });

  assert.equal(result.path, "standalone");
  assert.equal(result.planning, "created");
  assert.deepEqual(result.tracker, { kind: "markdown", path: ".agent-team/TASKS.md", status: "current" });
  assert.deepEqual(result.tasks, [{
    id: "AT-001",
    title: "Fix empty email validation",
    status: "ready",
    dependencies: [],
    acceptance: ["Empty email returns Email required"],
  }]);
  assert.equal(result.eligible, true);
  assert.equal(result.readyForDispatch, false);
  assert.deepEqual(result.projectInitialization, {
    required: true,
    projectId: null,
    projectOwner: "owner-1",
    teamsPath: ".agent-team/TEAMS.md",
    tracker: { kind: "markdown", path: ".agent-team/TASKS.md", status: "current" },
    teams: [{ id: "TEAM-001", name: "Morpheus 01", owner: "owner-1", taskIds: ["AT-001"], status: "planned" }],
  });
});

test("a temporary Beads outage remains a specific gap and never migrates the tracker", async () => {
  const { assessReadiness } = await import("../hooks/lib/readiness.mjs");
  const tracker = { kind: "beads", status: "unavailable", reason: "backend_offline" };
  const result = assessReadiness({
    existing: {
      plan: {
        scope: "Repair parser",
        acceptance: [],
        tasks: [{ id: "T-2", title: "Repair parser", status: "ready", dependencies: [] }],
        branch: "develop",
        verification: [],
        authority: {},
      },
      tracker,
    },
  });

  assert.strictEqual(result.tracker, tracker);
  assert.equal(result.eligible, false);
  assert.deepEqual(result.missing.map(({ id }) => id), ["acceptance_conditions", "verification_commands", "authorization_boundaries", "tracker_available"]);
  assert.ok(result.missing.every(({ question }) => !/continue|anything else/i.test(question)));
});
