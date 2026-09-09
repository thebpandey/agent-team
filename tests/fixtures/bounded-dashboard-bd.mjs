#!/usr/bin/env node
// Bounded test command: models only the selected Beads list/version/export calls used by dashboard tests.
import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";

const tasks = [
  { id: "AT-GRAPH-A", title: "Graph source task", status: "open", assignee: "", priority: 1, dependency_count: 0, dependencies: [] },
  { id: "AT-GRAPH-B", title: "Graph dependent task", status: "open", assignee: "TEAM-001", priority: 2, dependency_count: 1, dependencies: [{ depends_on_id: "AT-GRAPH-A", type: "blocks" }] },
];

if (process.argv[2] === "--version") {
  process.stdout.write("bounded-dashboard-bd 1\n");
} else if (process.argv[2] === "list") {
  process.stdout.write(JSON.stringify(tasks));
} else if (process.argv[2] === "export" && process.argv.includes("-o")) {
  const destination = process.argv[process.argv.indexOf("-o") + 1];
  mkdirSync(path.dirname(destination), { recursive: true });
  writeFileSync(destination, `${tasks.map((task) => JSON.stringify(task)).join("\n")}\n`);
} else {
  process.stderr.write("Unsupported bounded dashboard bd fixture command.\n");
  process.exitCode = 2;
}
