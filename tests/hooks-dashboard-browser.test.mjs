import assert from "node:assert/strict";
import test from "node:test";

import { renderDashboard } from "../hooks/lib/dashboard.mjs";

const fixture = Object.freeze({
  project: { id: "browser-fixture" }, mode: "snapshot",
  freshness: { status: "stale", source: "TASKS.md", observedAt: "2026-09-08T12:00:00.000Z" },
  progress: { status: "not_applicable", total: 0, completed: 0, remaining: 0, percentage: null, excluded: { cancelled: 0, deferred: 0 } },
  activity: { active: 0, parked: 0, paused: 0, ready: 0, capacity: null }, run: { paused: null }, teams: [], tasks: [], state: {},
});

test("standalone browser fixture keeps keyboard controls, stale state, and mobile safeguards in one file", () => {
  const html = renderDashboard(fixture);
  assert.match(html, /Source: TASKS\.md · stale/);
  assert.match(html, /N\/A/);
  assert.match(html, /<label for="task-search">/);
  assert.match(html, /<label for="task-status">/);
  assert.match(html, /skip-link/);
  assert.match(html, /prefers-reduced-motion/);
  assert.match(html, /max-width: 600px/);
  assert.match(html, /Recorded compute state is not live process liveness/);
  assert.match(html, /Task-count completion only; not estimated effort/);
  assert.match(html, /Integration: unknown/);
  assert.match(html, /Release: unknown/);
  assert.match(html, /Progress confidence: not_applicable/);
});
