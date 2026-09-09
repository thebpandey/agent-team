#!/usr/bin/env node
// Bounded test command: models only the selected Beads list/claim calls used by the CLI consumer test.
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import path from "node:path";

const statePath = path.join(process.env.BEADS_DIR, "bounded-cli-fixture.json");
const initial = {
  id: "AT-BEADS",
  title: "Claim an unassigned Beads task",
  status: "open",
  assignee: "",
  priority: 1,
  dependency_count: 0,
  dependencies: [],
  notes: "",
};
const read = () => {
  try { return JSON.parse(readFileSync(statePath, "utf8")); } catch { return initial; }
};
const value = (flag) => process.argv[process.argv.indexOf(flag) + 1];

if (process.argv[2] === "list") {
  process.stdout.write(JSON.stringify([read()]));
} else if (process.argv[2] === "update" && process.argv.includes("--claim")) {
  const task = { ...read(), assignee: value("--actor"), status: "in_progress", notes: value("--append-notes") };
  mkdirSync(path.dirname(statePath), { recursive: true });
  writeFileSync(statePath, JSON.stringify(task));
  process.stdout.write(JSON.stringify(task));
} else {
  process.stderr.write("Unsupported bounded Beads fixture command.\n");
  process.exitCode = 2;
}
