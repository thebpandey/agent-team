import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import http from "node:http";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { createBeadsGraphAdapter, createLoopbackDashboard, createSnapshotPublisher, renderDashboard } from "../hooks/lib/dashboard.mjs";

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
