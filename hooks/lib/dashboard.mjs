import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import path from "node:path";
import { randomUUID } from "node:crypto";

function jsonForScript(value) {
  return JSON.stringify(value).replace(/</g, "\\u003c").replace(/>/g, "\\u003e").replace(/&/g, "\\u0026").replace(/\u2028/g, "\\u2028").replace(/\u2029/g, "\\u2029");
}

function escapeHtml(value) {
  return String(value ?? "").replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#39;");
}

function taskRows(tasks) {
  return tasks.map((task) => `<tr data-status="${escapeHtml(task.status)}" data-search="${escapeHtml([task.id, task.label, task.owner, task.priority, task.dependencies.join(" ")].join(" ").toLowerCase())}"><th scope="row">${escapeHtml(task.id)}</th><td>${escapeHtml(task.label)}</td><td>${escapeHtml(task.status)}</td><td>${escapeHtml(task.runtime?.compute || "not recorded")}</td><td>${escapeHtml(task.priority)}</td><td>${escapeHtml(task.owner)}</td><td>${escapeHtml(task.dependencies.join(", ") || "None")}</td></tr>`).join("");
}

// Keep the distributed assets/dashboard files in sync with these snapshot-safe copies.
const snapshotCss = `:root { color-scheme: dark; --surface: #063b2d; --surface-raised: #07513b; --ink: #fff; --muted: #c9ded6; --accent: #75ff57; --line: #5da892; --focus: #fff36b; font-family: ui-sans-serif, system-ui, sans-serif; }
* { box-sizing: border-box; } body { margin: 0; min-width: 0; background: #021f18; color: var(--ink); font-size: 16px; line-height: 1.5; } main { width: min(1120px, calc(100% - 2rem)); margin: auto; padding: 2rem 0 3rem; } .hero, section, details { background: var(--surface); border: 1px solid var(--line); border-radius: 1rem; padding: 1.25rem; margin-block: 1rem; } .eyebrow { color: var(--accent); font-size: .8rem; font-weight: 800; letter-spacing: .12em; margin: 0; } h1, h2 { line-height: 1.15; } h1 { font-size: clamp(2rem, 7vw, 3.6rem); margin: .25rem 0; } h2 { margin-top: 0; } .summary { display: grid; grid-template-columns: repeat(3, 1fr); gap: 1rem; } .summary article { background: var(--surface-raised); border-radius: .75rem; padding: 1rem; } .summary span, small { display: block; color: var(--muted); } .summary strong { color: var(--accent); display: block; font-size: 2rem; } .section-heading { display: flex; justify-content: space-between; align-items: start; gap: 1rem; } .controls { display: flex; flex-wrap: wrap; gap: .5rem; align-items: center; margin-block: 1rem; } .controls label { font-weight: 700; } .controls input, .controls select, button { min-height: 44px; border: 1px solid var(--line); border-radius: .4rem; padding: .55rem .7rem; font: inherit; } .controls input, .controls select { background: #fff; color: #10221b; } button { background: var(--accent); color: #062519; font-weight: 800; } button:disabled { opacity: .75; } .table-wrap { overflow-x: auto; border: 1px solid var(--line); border-radius: .5rem; } table { min-width: 700px; width: 100%; border-collapse: collapse; } th, td { text-align: left; padding: .75rem; vertical-align: top; border-bottom: 1px solid color-mix(in srgb, var(--line), transparent 50%); } th { color: var(--accent); } details summary { cursor: pointer; font-weight: 800; } .helper, #results { color: var(--muted); } .skip-link { position: fixed; left: .5rem; top: .5rem; transform: translateY(-200%); background: var(--focus); color: #000; padding: .5rem; z-index: 1; } .skip-link:focus { transform: translateY(0); } :focus-visible { outline: 3px solid var(--focus); outline-offset: 3px; } @media (max-width: 600px) { main { width: min(100% - 1rem, 1120px); padding-top: 1rem; } .summary { grid-template-columns: 1fr; } .section-heading { display: block; } .section-heading button { width: 100%; } } @media (prefers-reduced-motion: reduce) { *, *::before, *::after { scroll-behavior: auto !important; transition: none !important; animation: none !important; } }`;

const snapshotScript = `(() => { const search = document.querySelector("#task-search"); const filter = document.querySelector("#task-status"); const rows = [...document.querySelectorAll("#task-rows tr")]; const results = document.querySelector("#results"); for (const status of [...new Set(rows.map((row) => row.dataset.status))].sort()) { const option = document.createElement("option"); option.value = status; option.textContent = status; filter?.append(option); } function apply() { const query = search?.value.trim().toLowerCase() || ""; const status = filter?.value || ""; let visible = 0; for (const row of rows) { const show = (!query || row.dataset.search.includes(query)) && (!status || row.dataset.status === status); row.hidden = !show; if (show) visible += 1; } if (results) results.textContent = visible + " task" + (visible === 1 ? "" : "s") + " shown."; } search?.addEventListener("input", apply); filter?.addEventListener("change", apply); apply(); })();`;

/** Render a standalone, local-only snapshot. The browser reads embedded data and never reads the tracker. */
export function renderDashboard(model) {
  const progress = model.progress || {};
  const freshness = model.freshness || {};
  const percentage = progress.percentage === null ? "N/A" : `${progress.percentage}%`;
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Agent-Team status</title><style>${snapshotCss}</style></head>
<body><a class="skip-link" href="#tasks">Skip to tasks</a><main id="dashboard" tabindex="-1">
<header class="hero"><p class="eyebrow">LOCAL STATUS SNAPSHOT</p><h1>${escapeHtml(model.project?.id || "Agent-Team")}</h1><p id="freshness" role="status">Source: ${escapeHtml(freshness.source || "unknown")} · ${escapeHtml(freshness.status || "unknown")}${freshness.observedAt ? ` · ${escapeHtml(freshness.observedAt)}` : ""}</p></header>
<section aria-labelledby="overview-title"><h2 id="overview-title">Overview</h2><div class="summary"><article><span>Completion</span><strong>${percentage}</strong><small>${escapeHtml(progress.completed ?? "?")} complete · ${escapeHtml(progress.remaining ?? "?")} remaining</small></article><article><span>Recorded work</span><strong>${escapeHtml(model.activity?.active ?? "?")}</strong><small>${escapeHtml(model.activity?.parked ?? "?")} parked · ${escapeHtml(model.activity?.paused ?? "?")} paused · ${escapeHtml(model.activity?.ready ?? "?")} ready</small></article><article><span>Capacity</span><strong>${escapeHtml(model.activity?.capacity ?? "Unknown")}</strong><small>Supplied scheduler capacity; not inferred here.</small></article></div><p>Admissions are ${model.run?.paused === true ? "paused" : model.run?.paused === false ? "not paused" : "unknown"}. Recorded compute state is not live process liveness.</p></section>
<section id="tasks" aria-labelledby="tasks-title"><div class="section-heading"><div><h2 id="tasks-title">All tasks</h2><p>${escapeHtml(model.tasks?.length ?? 0)} rows; counts exclude cancelled and approved-deferred work.</p></div><button type="button" id="refresh" disabled aria-describedby="refresh-help">Reload saved snapshot</button></div><p id="refresh-help" class="helper">A file snapshot cannot query project records. Reload opens the latest saved file.</p><div class="controls"><label for="task-search">Search tasks</label><input id="task-search" type="search" autocomplete="off" placeholder="ID, owner, status…"><label for="task-status">Status</label><select id="task-status"><option value="">All statuses</option></select></div><div class="table-wrap" tabindex="0"><table><thead><tr><th>ID</th><th>Task</th><th>Status</th><th>Recorded state</th><th>Priority</th><th>Owner</th><th>Dependencies</th></tr></thead><tbody id="task-rows">${taskRows(model.tasks || [])}</tbody></table></div><p id="results" aria-live="polite"></p></section>
<details><summary>Teams and roles</summary><ul>${(model.teams || []).map((team) => `<li><strong>${escapeHtml(team.name)}</strong> · ${escapeHtml(team.role)} · ${escapeHtml(team.model)} / ${escapeHtml(team.effort)} · ${escapeHtml(team.status)}</li>`).join("") || "<li>Team metadata is unknown.</li>"}</ul></details>
<details><summary>Dependency representation</summary><p>Each task row lists prerequisite task IDs in the Dependencies column. A graph is unavailable unless separately generated from a current Beads export.</p></details>
<script id="dashboard-data" type="application/json">${jsonForScript(model)}</script><script>${snapshotScript}</script></main></body></html>`;
}

async function atomicWrite(destination, content) {
  await mkdir(path.dirname(destination), { recursive: true });
  const temporary = `${destination}.${randomUUID()}.tmp`;
  try {
    await writeFile(temporary, content, "utf8");
    await rename(temporary, destination);
  } finally {
    await rm(temporary, { force: true });
  }
}

/** Explicit dashboard refresh writer. It publishes only derived files and shares concurrent refresh work. */
export function createSnapshotPublisher({ destination, derive, render = renderDashboard }) {
  if (!destination || typeof derive !== "function" || typeof render !== "function") throw new Error("Snapshot publisher requires destination, derive, and render.");
  let fingerprint = null;
  let inFlight = null;
  let lastGood = null;
  let lastModel = null;
  let stale = false;
  async function refresh() {
    if (inFlight) return inFlight;
    inFlight = (async () => {
      try {
        const model = await derive();
        if (model instanceof Error) throw model;
        const source = JSON.stringify(model);
        if (source === fingerprint && !stale) return { status: "unchanged", destination, lastGood };
        const html = render(model);
        await atomicWrite(destination, html);
        fingerprint = source;
        lastModel = model;
        stale = false;
        lastGood = { destination, refreshedAt: new Date().toISOString() };
        return { status: "published", destination, lastGood };
      } catch (error) {
        stale = true;
        if (lastModel) {
          try {
            const staleModel = { ...lastModel, freshness: { ...lastModel.freshness, status: "stale", reason: String(error.message || error) } };
            await atomicWrite(destination, render(staleModel));
          } catch {
            // A failed stale-marker write must never replace the last usable snapshot.
          }
        }
        return { status: "stale", destination, lastGood, error: String(error.message || error) };
      } finally {
        inFlight = null;
      }
    })();
    return inFlight;
  }
  return Object.freeze({ refresh, snapshot: () => ({ destination, lastGood, status: lastGood ? stale ? "stale" : "current" : "unavailable" }) });
}

/**
 * Optional adapter boundary for the separately installed upstream beads_viewer.
 * Callers must obtain a fresh export from the selected canonical Beads database;
 * this module neither invokes, vendors, nor licenses the external program.
 */
export function createBeadsGraphAdapter({ exportCurrent, renderGraph }) {
  if (typeof exportCurrent !== "function" || typeof renderGraph !== "function") throw new Error("Beads graph adapter requires exportCurrent and renderGraph.");
  const attribution = Object.freeze({
    repository: "https://github.com/Dicklesworthstone/beads_viewer",
    license: "https://github.com/Dicklesworthstone/beads_viewer/blob/main/LICENSE",
    provider: "Jeffrey Emanuel",
  });
  return Object.freeze({
    attribution,
    async refresh() {
      try {
        const exported = await exportCurrent();
        if (!exported?.jsonlPath || exported.source !== "canonical_beads") throw new Error("A fresh canonical Beads export is required before graph rendering.");
        const graph = await renderGraph({ jsonlPath: exported.jsonlPath, observedAt: exported.observedAt || null, noninteractive: true, disableExportHooks: true });
        if (!graph?.path) throw new Error("External graph renderer returned no output path.");
        return { status: "available", path: graph.path, observedAt: exported.observedAt || null, attribution };
      } catch (error) {
        return { status: "unavailable", path: null, reason: String(error.message || error), attribution };
      }
    },
  });
}

function validHost(host, port) {
  return host === `127.0.0.1:${port}` || host === `localhost:${port}` || host === "127.0.0.1" || host === "localhost";
}

function validOrigin(origin, port) {
  return !origin || origin === `http://127.0.0.1:${port}` || origin === `http://localhost:${port}`;
}

/** Create, but do not start, an opt-in loopback-only status viewer with a fixed route allowlist. */
export function createLoopbackDashboard({ assetsDirectory, readModel, render = renderDashboard }) {
  if (!assetsDirectory || typeof readModel !== "function") throw new Error("Loopback dashboard requires assetsDirectory and readModel.");
  let server;
  let port;
  const assets = new Map([["/dashboard.css", ["dashboard.css", "text/css; charset=utf-8"]], ["/dashboard.js", ["dashboard.js", "application/javascript; charset=utf-8"]]]);
  const respond = (response, status, body, type = "text/plain; charset=utf-8") => response.writeHead(status, { "content-type": type, "cache-control": "no-store", "x-content-type-options": "nosniff" }).end(body);
  return Object.freeze({
    async start() {
      if (server) return { port };
      server = createServer(async (request, response) => {
        if (!validHost(request.headers.host || "", port) || !validOrigin(request.headers.origin, port)) return respond(response, 403, "Forbidden");
        if (request.method !== "GET") return respond(response, 405, "Method Not Allowed");
        const pathname = new URL(request.url || "/", "http://localhost").pathname;
        try {
          if (pathname === "/") return respond(response, 200, render(await readModel()), "text/html; charset=utf-8");
          const asset = assets.get(pathname);
          if (!asset) return respond(response, 404, "Not Found");
          return respond(response, 200, await readFile(path.join(assetsDirectory, asset[0]), "utf8"), asset[1]);
        } catch {
          return respond(response, 503, "Status snapshot unavailable");
        }
      });
      await new Promise((resolve, reject) => { server.once("error", reject); server.listen(0, "127.0.0.1", resolve); });
      port = server.address().port;
      return { port };
    },
    async stop() {
      if (!server) return;
      const closing = server;
      server = undefined;
      await new Promise((resolve, reject) => closing.close((error) => error ? reject(error) : resolve()));
    },
  });
}
