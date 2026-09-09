import { writeFile } from "node:fs/promises";
import { renderDashboard } from "../hooks/lib/dashboard.mjs";

const destination = process.argv[2];
if (!destination) throw new Error("A snapshot destination is required.");
await writeFile(destination, renderDashboard({
  project: { id: "browser-fixture" }, mode: "snapshot",
  freshness: { status: "current", source: "TASKS.md", observedAt: "2026-09-08T12:00:00.000Z" },
  progress: { status: "exact", total: 2, completed: 1, remaining: 1, percentage: 50, excluded: { cancelled: 1, deferred: 1 } },
  activity: { active: 1, parked: 0, paused: 0, ready: 1, capacity: null }, run: { paused: false },
  teams: [{ id: "T1", name: "Platform", role: "developer", model: "model-x", effort: "high", status: "active" }],
  tasks: [
    { id: "AT-ONE", label: "Alpha task", status: "ready", runtime: null, priority: "P1", owner: "T1", dependencies: [], counted: true },
    { id: "AT-TWO", label: "Beta task", status: "completed", runtime: { compute: "active" }, priority: "P2", owner: "T1", dependencies: ["AT-ONE"], counted: true },
  ], state: { integration: { status: "passed" }, release: { status: "pending" } },
}));
