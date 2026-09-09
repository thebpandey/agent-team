import path from "node:path";
import { createLoopbackDashboard } from "../hooks/lib/dashboard.mjs";

let revision = 0;
const server = createLoopbackDashboard({
  port: 4173,
  assetsDirectory: path.join(import.meta.dirname, "..", "assets", "dashboard"),
  readModel: async () => ({
    project: { id: `live-${++revision}` }, mode: "live",
    freshness: { status: "current", source: "TASKS.md" },
    progress: { status: "exact", total: 1, completed: 0, remaining: 1, percentage: 0, excluded: { cancelled: 0, deferred: 0 } },
    activity: { active: 1, parked: 0, paused: 0, ready: 0, capacity: 0 }, run: { paused: false, current: "run-live", scope: { status: "full_project" }, blockers: [], blockerStatus: "known" }, teams: [],
    tasks: [{ id: "LIVE", label: "Live task", status: "in_progress", priority: "P1", owner: "T1", dependencies: [], counted: true }], graph: { status: "available", format: "json", adjacency: { nodes: [{ id: "LIVE", title: "LIVE" }, { id: "NEXT", title: "NEXT" }], edges: [{ from: "LIVE", to: "NEXT", type: "blocks" }] } }, state: { integration: { status: "passed" }, release: { status: "pending" } },
  }),
});
await server.start();
process.on("SIGTERM", async () => { await server.stop(); process.exit(0); });
