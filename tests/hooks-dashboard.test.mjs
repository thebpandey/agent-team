import assert from "node:assert/strict";
import { chmod, mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { createBeadsGraphAdapter, createBeadsGraphCommandAdapter, createLoopbackDashboard, createSnapshotPublisher, renderDashboard } from "../hooks/lib/dashboard.mjs";

const model = Object.freeze({
  project: { id: "project-1", root: "/project" },
  freshness: { status: "current", source: "TASKS.md", observedAt: "2026-09-08T12:00:00.000Z" },
  progress: { status: "exact", total: 1, completed: 1, remaining: 0, percentage: 100, excluded: { cancelled: 0, deferred: 0 } },
  activity: { active: 0, parked: 0, ready: 0, capacity: 1 },
  teams: [],
  tasks: [{ id: "AT-1", label: "</script><img src=x onerror=alert(1)>", status: "completed", priority: "P1", owner: "T1", dependencies: [], counted: true }],
  state: { integration: { status: "passed" }, release: { status: "pending" } },
});

function request(port, { method = "GET", route = "/", headers = {} } = {}) {
  return new Promise((resolve, reject) => {
    const req = http.request({ host: "127.0.0.1", port, path: route, method, headers }, (response) => {
      let body = "";
      response.setEncoding("utf8");
      response.on("data", (chunk) => { body += chunk; });
      response.on("end", () => resolve({ status: response.statusCode, body }));
    });
    req.on("error", reject);
    req.end();
  });
}

test("dashboard render is a standalone snapshot with escaped embedded data", () => {
  const html = renderDashboard(model);

  assert.match(html, /<main id="dashboard"/);
  assert.match(html, /<style>/);
  assert.match(html, /<script>/);
  assert.doesNotMatch(html, /(?:href|src)="dashboard\.(?:css|js)"/);
  assert.doesNotMatch(html, /<\/script><img/i);
  assert.match(html, /\\u003c\/script\\u003e/);
  assert.doesNotMatch(html, /https?:\/\//);
});

test('dependency graph provides keyboard-operable task selection and labels a bounded visual excerpt', () => {
  const adjacency = { nodes: Array.from({ length: 41 }, (_, i) => ({ id: `AT-${i}`, title: `AT-${i}` })), edges: Array.from({ length: 40 }, (_, i) => ({ from: `AT-${i}`, to: `AT-${i + 1}`, type: 'blocks' })) };
  const html = renderDashboard({ ...model, graph: { status: 'available', format: 'json', adjacency } });
  assert.match(html, /role="button"[^>]*aria-label="Filter task AT-0"/);
  assert.match(html, /data-graph-task="AT-0"/);
  assert.match(html, /Visual excerpt: 32 of 41 nodes/);
  assert.match(html, /keydown/);
});

test('actual command boundary pins canonical export and isolates bv routing from inherited environment', async t => {
  const directory = await mkdtemp(path.join(os.tmpdir(), 'agent-team-graph-boundary-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const beads = path.join(directory, '.beads');
  await mkdir(beads);
  const bd = path.join(directory, 'selected-bd');
  const bv = path.join(directory, 'selected-bv');
  await writeFile(bd, `#!${process.execPath}\nimport { writeFileSync } from 'node:fs';\nif (process.argv[2] === '--version') console.log('fixture-bd'); else writeFileSync(process.argv[process.argv.indexOf('-o') + 1], JSON.stringify({ dir: process.env.BEADS_DIR, db: process.env.BEADS_DB ?? null }));\n`);
  await writeFile(bv, `#!${process.execPath}\nimport { readFileSync } from 'node:fs';\nif (process.argv[2] === '--version') console.log('fixture-bv'); else if (process.argv[2] === '--robot-help') console.log('--robot-graph --graph-format --no-hooks'); else { const input = JSON.parse(readFileSync('.beads/issues.jsonl', 'utf8')); console.log(JSON.stringify({format: 'json', nodes: 2, edges: 1, adjacency: {nodes: [{id: 'A', title: 'A'}, {id: 'B', title: 'B'}], edges: [{from: 'A', to: 'B', type: 'blocks'}]}, probe: input})); }\n`);
  await chmod(bd, 0o700); await chmod(bv, 0o700);
  const oldDir = process.env.BEADS_DIR, oldDb = process.env.BEADS_DB;
  t.after(() => { if (oldDir === undefined) delete process.env.BEADS_DIR; else process.env.BEADS_DIR = oldDir; if (oldDb === undefined) delete process.env.BEADS_DB; else process.env.BEADS_DB = oldDb; });
  process.env.BEADS_DIR = path.join(directory, 'wrong-project'); process.env.BEADS_DB = path.join(directory, 'wrong.db');
  const adapter = createBeadsGraphCommandAdapter({ projectRoot: directory, tracker: { kind: 'beads', id: `beads:${beads}`, path: beads, executable: bd }, bvPath: bv, selected: true, termsAcknowledged: true });
  const result = await adapter.refresh();
  assert.equal(result.status, 'available', result.reason);
  const exported = JSON.parse(await readFile(result.graph.source.exportFile, 'utf8'));
  assert.deepEqual(exported, { dir: beads, db: null });
});

test("snapshot publisher skips stable inputs and retains last good snapshot after a failed refresh", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-dashboard-"));
  const destination = path.join(directory, "index.html");
  try {
    let current = model;
    const publisher = createSnapshotPublisher({ destination, derive: async () => current, render: renderDashboard });
    assert.equal((await publisher.refresh()).status, "published");
    assert.equal((await publisher.refresh()).status, "unchanged");
    const before = await readFile(destination, "utf8");
    current = new Error("tracker unavailable");
    const failed = await publisher.refresh();
    assert.equal(failed.status, "stale");
    const stale = await readFile(destination, "utf8");
    assert.notEqual(stale, before);
    assert.match(stale, /Source: TASKS\.md · stale/);
    assert.match(stale, /AT-1/);
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("snapshot publisher retains last-good task data when canonical state reports unavailable without throwing", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-dashboard-"));
  const destination = path.join(directory, "index.html");
  try {
    let current = model;
    const publisher = createSnapshotPublisher({ destination, derive: async () => current, render: renderDashboard });
    assert.equal((await publisher.refresh()).status, "published");
    current = { ...model, freshness: { status: "unavailable", source: "TASKS.md", reason: "backend timeout" }, tasks: [] };
    assert.equal((await publisher.refresh()).status, "stale");
    const stale = await readFile(destination, "utf8");
    assert.match(stale, /Source: TASKS\.md · stale/);
    assert.match(stale, /AT-1/);
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("snapshot publisher fingerprints content without collection timestamps and reuses persisted metadata after restart", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-dashboard-"));
  const destination = path.join(directory, "index.html");
  try {
    let observedAt = "2026-09-08T12:00:00.000Z";
    const derive = async () => ({ ...model, freshness: { ...model.freshness, observedAt } });
    const first = createSnapshotPublisher({ destination, derive, render: renderDashboard });
    assert.equal((await first.refresh()).status, "published");
    observedAt = "2026-09-08T12:01:00.000Z";
    assert.equal((await first.refresh()).status, "unchanged");
    const restarted = createSnapshotPublisher({ destination, derive, render: renderDashboard });
    assert.equal((await restarted.refresh()).status, "unchanged");
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("publisher restart restores last-good data for stale markers and rewrites the same healthy content as current", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-dashboard-"));
  const destination = path.join(directory, "index.html");
  try {
    let current = model;
    const first = createSnapshotPublisher({ destination, derive: async () => current, render: renderDashboard });
    await first.refresh();
    current = new Error("backend timeout");
    const afterRestartFailure = createSnapshotPublisher({ destination, derive: async () => current, render: renderDashboard });
    await afterRestartFailure.refresh();
    assert.match(await readFile(destination, "utf8"), /Source: TASKS\.md · stale/);
    current = model;
    const recovered = await afterRestartFailure.refresh();
    assert.equal(recovered.status, "published");
    assert.match(await readFile(destination, "utf8"), /Source: TASKS\.md · current/);
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("snapshot publisher serializes concurrent refreshes and atomically replaces output", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-dashboard-"));
  const destination = path.join(directory, "index.html");
  try {
    let calls = 0;
    const publisher = createSnapshotPublisher({
      destination,
      derive: async () => ({ ...model, revision: ++calls }),
      render: (value) => `<html>${value.revision}</html>`,
    });
    const results = await Promise.all([publisher.refresh(), publisher.refresh()]);
    assert.equal(calls, 1);
    assert.equal(results[0].status, "published");
    assert.equal(results[1].status, "published");
    assert.equal(await readFile(destination, "utf8"), "<html>1</html>");
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test('publisher never trusts a content receipt when its HTML was replaced independently', async t => {
  const directory = await mkdtemp(path.join(os.tmpdir(), 'agent-team-dashboard-receipt-'));
  t.after(() => rm(directory, { recursive: true, force: true }));
  const destination = path.join(directory, 'index.html');
  await createSnapshotPublisher({ destination, derive: async () => model }).refresh();
  await writeFile(destination, '<html>unrelated or interrupted publication</html>');
  const restarted = createSnapshotPublisher({ destination, derive: async () => model });
  assert.equal((await restarted.refresh()).status, 'published');
  assert.match(await readFile(destination, 'utf8'), /All tasks/);
});

test("loopback helper rejects unsafe route, host, origin, and methods while serving only fixed assets", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-dashboard-"));
  try {
    await writeFile(path.join(directory, "dashboard.css"), "body{}\n");
    await writeFile(path.join(directory, "dashboard.js"), "\n");
    const server = createLoopbackDashboard({ assetsDirectory: directory, readModel: async () => model, render: renderDashboard });
    const { port } = await server.start();
    try {
      assert.equal((await request(port)).status, 200);
      assert.equal((await request(port, { route: "/dashboard.css" })).status, 200);
      assert.equal((await request(port, { route: "/../../etc/passwd" })).status, 404);
      assert.equal((await request(port, { route: "//malformed" })).status, 400);
      assert.equal((await request(port, { method: "POST" })).status, 405);
      assert.equal((await request(port, { headers: { host: "evil.test" } })).status, 403);
      assert.equal((await request(port, { headers: { origin: "https://evil.test" } })).status, 403);
    } finally {
      await server.stop();
    }
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("dashboard labels snapshot reload and live refresh distinctly", () => {
  assert.match(renderDashboard({ ...model, mode: "snapshot" }), /Reload saved snapshot/);
  const live = renderDashboard({ ...model, mode: "live" });
  assert.match(live, /Refresh local status/);
  assert.doesNotMatch(live, /id="refresh" disabled/);
});

test("loopback helper coalesces bounded concurrent collection and rejects an overlarge rendered response", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-dashboard-"));
  try {
    await writeFile(path.join(directory, "dashboard.css"), "body{}\n");
    await writeFile(path.join(directory, "dashboard.js"), "\n");
    let calls = 0;
    const server = createLoopbackDashboard({ assetsDirectory: directory, deadlineMs: 100, maxOutputBytes: 64, readModel: async () => { calls += 1; await new Promise((resolve) => setTimeout(resolve, 10)); return model; }, render: renderDashboard });
    const { port } = await server.start();
    try {
      const [first, second] = await Promise.all([request(port), request(port)]);
      assert.equal(calls, 1);
      assert.equal(first.status, 503);
      assert.equal(second.status, 503);
    } finally {
      await server.stop();
    }
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("file snapshot stays saved while the explicitly started loopback view reads a new model per request", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-dashboard-"));
  try {
    let sequence = 0;
    await writeFile(path.join(directory, "dashboard.css"), "body{}\n");
    await writeFile(path.join(directory, "dashboard.js"), "\n");
    const server = createLoopbackDashboard({ assetsDirectory: directory, readModel: async () => ({ ...model, project: { id: `live-${++sequence}` } }), render: renderDashboard });
    const { port } = await server.start();
    try {
      assert.match((await request(port)).body, /live-1/);
      assert.match((await request(port)).body, /live-2/);
    } finally {
      await server.stop();
    }
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});

test("optional Beads graph adapter requires a fresh canonical export and degrades without blocking snapshots", async () => {
  const adapter = createBeadsGraphAdapter({
    exportCurrent: async () => ({ jsonlPath: "/tmp/issues.jsonl", source: "canonical_beads", observedAt: "2026-09-08T12:00:00.000Z" }),
    renderGraph: async ({ jsonlPath }) => ({ path: `${jsonlPath}.html` }),
  });
  const graph = await adapter.refresh();
  assert.equal(graph.status, "available");
  assert.equal(graph.path, "/tmp/issues.jsonl.html");
  assert.match(adapter.attribution.repository, /beads_viewer/);
  const unavailable = await createBeadsGraphAdapter({
    exportCurrent: async () => ({ jsonlPath: "/tmp/old.jsonl", source: "cached_jsonl" }),
    renderGraph: async () => { throw new Error("must not execute"); },
  }).refresh();
  assert.equal(unavailable.status, "unavailable");
  assert.match(unavailable.reason, /fresh canonical/i);
});

test("documented Beads command adapter checks capabilities then refreshes canonical bd export before noninteractive bv graph", async () => {
  const calls = [];
  const adapter = createBeadsGraphCommandAdapter({
    projectRoot: "/project", tracker: { kind: "beads", id: "beads:/project/.beads", executable: "/selected/bd" }, stagingDirectory: "/project/.agent-team/dashboard/beads", ensureDirectory: async () => {},
    selected: true,
    termsAcknowledged: true,
    runCommand: async (command, args, options) => {
      calls.push({ command, args, options });
      if (args[0] === "--version") return { status: "completed", output: "bv 0.24.1" };
      if (args[0] === "--robot-help") return { status: "completed", output: "--robot-graph --graph-format --no-hooks" };
      if (command === "/selected/bd") return { status: "completed", output: "" };
      return { status: "completed", output: JSON.stringify({ format: "json", nodes: 2, edges: 1, adjacency: { nodes: [{ id: "A", title: "Title with A -> B" }, { id: "B", title: "Second" }], edges: [{ from: "A", to: "B", type: "blocks" }] } }) };
    },
  });
  const graph = await adapter.refresh();
  assert.equal(graph.status, "available");
  assert.deepEqual(graph.graph.adjacency.edges, [{ from: "A", to: "B", type: "blocks" }]);
  assert.deepEqual(calls.map((entry) => [entry.command, entry.args]), [
    ["/selected/bd", ["--version"]], ["bv", ["--version"]], ["bv", ["--robot-help"]],
    ["/selected/bd", ["export", "-o", "/project/.agent-team/dashboard/beads/.beads/issues.jsonl"]],
    ["bv", ["--robot-graph", "--graph-format=json", "--no-hooks"]],
  ]);
  assert.equal(calls.slice(0, 4).every((entry) => entry.options.cwd === "/project"), true);
  assert.equal(calls[4].options.cwd, "/project/.agent-team/dashboard/beads");
  assert.equal(calls.every((entry) => entry.options.timeoutMs <= 5000 && entry.options.maxOutputBytes <= 256 * 1024 && !Object.hasOwn(entry.options.env, "BEADS_DB")), true);
  assert.equal(calls.slice(0, 4).every(entry => entry.options.env.BEADS_DIR === '/project/.beads'), true);
  assert.equal(calls[4].options.env.BEADS_DIR, '/project/.agent-team/dashboard/beads/.beads');
  assert.equal(graph.graph.source.trackerId, "beads:/project/.beads");
  const graphHtml = renderDashboard({ ...model, graph: graph.graph });
  assert.match(graphHtml, /A --blocks--&gt; B/);
  assert.doesNotMatch(graphHtml, /Title with A --.*--&gt; B/);
  assert.match(graphHtml, /<svg /);
  const unselected = await createBeadsGraphCommandAdapter({ projectRoot: "/project", runCommand: async () => { throw new Error("must not execute"); } }).refresh();
  assert.equal(unselected.status, "not_selected");
});

test("structured graph accepts isolated and empty nodes but rejects invalid adjacency and directory setup failures", async () => {
  const base = { projectRoot: "/project", tracker: { kind: "beads", id: "beads:/project/.beads", executable: "/selected/bd" }, stagingDirectory: "/project/.agent-team/dashboard/beads", selected: true, termsAcknowledged: true, ensureDirectory: async () => {} };
  const run = async (_command, args) => {
    if (args[0] === "--version") return { status: "completed", output: "v1" };
    if (args[0] === "--robot-help") return { status: "completed", output: "--robot-graph --graph-format --no-hooks" };
    if (args[0] === "export") return { status: "completed", output: "" };
    return { status: "completed", output: JSON.stringify({ format: "json", nodes: 1, edges: 0, adjacency: { nodes: [{ id: "SOLO", title: "A -> B is text" }], edges: null } }) };
  };
  const isolated = await createBeadsGraphCommandAdapter({ ...base, runCommand: run }).refresh();
  assert.equal(isolated.status, "available");
  assert.deepEqual(isolated.graph.adjacency.edges, []);
  const empty = await createBeadsGraphCommandAdapter({ ...base, runCommand: async (_command, args) => args[0] === "--robot-graph" ? { status: "completed", output: JSON.stringify({ format: "json", nodes: 0, edges: 0 }) } : run(_command, args) }).refresh();
  assert.equal(empty.status, "available");
  assert.deepEqual(empty.graph.adjacency.nodes, []);
  const invalid = await createBeadsGraphCommandAdapter({ ...base, runCommand: async (_command, args) => args[0] === "--robot-graph" ? { status: "completed", output: JSON.stringify({ format: "json", adjacency: { nodes: [{ title: "missing id" }], edges: [] } }) } : run(_command, args) }).refresh();
  assert.equal(invalid.status, "unavailable");
  const directoryFailure = await createBeadsGraphCommandAdapter({ ...base, ensureDirectory: async () => { throw new Error("read-only"); }, runCommand: run }).refresh();
  assert.equal(directoryFailure.status, "unavailable");
  assert.match(directoryFailure.reason, /staging/i);
});

test("loopback preserves one aborted underlying collection across repeated timeout requests and stop", async () => {
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-dashboard-"));
  try {
    await writeFile(path.join(directory, "dashboard.css"), "body{}\n");
    await writeFile(path.join(directory, "dashboard.js"), "\n");
    let calls = 0;
    let aborted = 0;
    const server = createLoopbackDashboard({ assetsDirectory: directory, deadlineMs: 10, readModel: ({ signal }) => new Promise((resolve) => { calls += 1; signal.addEventListener("abort", () => { aborted += 1; }); }), render: renderDashboard });
    const { port } = await server.start();
    try {
      assert.equal((await request(port)).status, 503);
      assert.equal((await request(port)).status, 503);
      assert.equal(calls, 1);
    } finally {
      await server.stop();
      assert.equal(aborted, 1);
    }
  } finally {
    await rm(directory, { force: true, recursive: true });
  }
});
