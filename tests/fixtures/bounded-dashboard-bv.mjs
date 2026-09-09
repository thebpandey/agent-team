#!/usr/bin/env node
// Bounded test command: models only the bv capability/JSON graph calls used by dashboard tests.
import { appendFileSync } from "node:fs";

const command = process.argv[2];
const log = process.env.AGENT_TEAM_BOUNDED_GRAPH_LOG;
const mode = process.env.AGENT_TEAM_BOUNDED_GRAPH_MODE;
if (log) appendFileSync(log, `${command}\n`);

function complete(output) {
  if (log) appendFileSync(log, `${command}:completed\n`);
  process.stdout.write(output);
}

if (command === "--version") {
  complete("bounded-dashboard-bv 1\n");
} else if (command === "--robot-help") {
  const output = "--robot-graph --graph-format --no-hooks\n";
  if (mode === "slow") setTimeout(() => complete(output), 2000);
  else complete(output);
} else if (command === "--robot-graph") {
  if (mode === "failed") {
    process.stderr.write("Bounded dashboard graph fixture failure.\n");
    process.exitCode = 2;
  } else {
    const output = `${JSON.stringify({
      format: "json",
      adjacency: {
        nodes: [{ id: "AT-GRAPH-A", title: "Source" }, { id: "AT-GRAPH-B", title: "Dependent" }],
        edges: [{ from: "AT-GRAPH-A", to: "AT-GRAPH-B", type: "blocks" }],
      },
      data_hash: "bounded-fixture",
    })}\n`;
    if (mode === "slow") setTimeout(() => complete(output), 2000);
    else complete(output);
  }
} else {
  process.stderr.write("Unsupported bounded dashboard bv fixture command.\n");
  process.exitCode = 2;
}
