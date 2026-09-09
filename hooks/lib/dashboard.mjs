import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { execFile } from "node:child_process";
import path from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { promisify } from "node:util";

const runFile = promisify(execFile);

function jsonForScript(value) {
  return JSON.stringify(value).replace(/</g, "\\u003c").replace(/>/g, "\\u003e").replace(/&/g, "\\u0026").replace(/\u2028/g, "\\u2028").replace(/\u2029/g, "\\u2029");
}

function escapeHtml(value) {
  return String(value ?? "").replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;").replace(/'/g, "&#39;");
}

function taskRows(tasks) {
  return tasks.map((task) => `<tr data-status="${escapeHtml(task.status)}" data-search="${escapeHtml([task.id, task.label, task.owner, task.priority, task.dependencies.join(" "), task.nextAction, task.evidence].join(" ").toLowerCase())}"><th scope="row">${escapeHtml(task.id)}</th><td>${escapeHtml(task.label)}</td><td>${escapeHtml(task.status)}</td><td>${escapeHtml(task.runtime?.compute || "not recorded")}</td><td>${escapeHtml(task.priority)}</td><td>${escapeHtml(task.owner)}</td><td>${escapeHtml(task.dependencies.join(", ") || "None")}</td><td>${escapeHtml(task.nextAction || "Unknown")}</td><td><details><summary>View</summary>${escapeHtml(task.evidence || task.cleanup?.evidencePaths?.join(", ") || "Unknown")}</details></td></tr>`).join("");
}

function graphSvg(dot) {
  const edges = [...String(dot).matchAll(/["']?([A-Za-z0-9_.:-]+)["']?\s*->\s*["']?([A-Za-z0-9_.:-]+)["']?/g)].slice(0, 48).map((match) => [match[1], match[2]]);
  const nodes = [...new Set(edges.flat())].slice(0, 32);
  if (!nodes.length) return "<p>Graph data has no safe, renderable edges; the textual graph remains available below.</p>";
  const width = 760;
  const height = Math.max(180, Math.ceil(nodes.length / 4) * 120);
  const point = (node) => { const index = nodes.indexOf(node); return { x: 110 + (index % 4) * 210, y: 70 + Math.floor(index / 4) * 110 }; };
  return `<svg viewBox="0 0 ${width} ${height}" role="img" aria-label="Interactive dependency graph. Use the task table for keyboard filtering and full dependency text."><defs><marker id="arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 z" fill="#75ff57"/></marker></defs>${edges.map(([from, to]) => { const a = point(from); const b = point(to); return `<line x1="${a.x}" y1="${a.y}" x2="${b.x}" y2="${b.y}" stroke="#5da892" stroke-width="2" marker-end="url(#arrow)"/>`; }).join("")}${nodes.map((node) => { const p = point(node); return `<g tabindex="0"><circle cx="${p.x}" cy="${p.y}" r="35" fill="#07513b" stroke="#75ff57"/><text x="${p.x}" y="${p.y + 5}" text-anchor="middle" fill="#fff">${escapeHtml(node)}</text><title>${escapeHtml(node)}</title></g>`; }).join("")}</svg>`;
}

// Keep the distributed assets/dashboard files in sync with these snapshot-safe copies.
const snapshotCss = `:root { color-scheme: dark; --surface: #063b2d; --surface-raised: #07513b; --ink: #fff; --muted: #c9ded6; --accent: #75ff57; --line: #5da892; --focus: #fff36b; font-family: ui-sans-serif, system-ui, sans-serif; }
* { box-sizing: border-box; } body { margin: 0; min-width: 0; background: #021f18; color: var(--ink); font-size: 16px; line-height: 1.5; } main { width: min(1120px, calc(100% - 2rem)); margin: auto; padding: 2rem 0 3rem; } .hero, section, details { background: var(--surface); border: 1px solid var(--line); border-radius: 1rem; padding: 1.25rem; margin-block: 1rem; } .eyebrow { color: var(--accent); font-size: .8rem; font-weight: 800; letter-spacing: .12em; margin: 0; } h1, h2 { line-height: 1.15; } h1 { font-size: clamp(2rem, 7vw, 3.6rem); margin: .25rem 0; } h2 { margin-top: 0; } .summary { display: grid; grid-template-columns: repeat(3, 1fr); gap: 1rem; } .summary article { background: var(--surface-raised); border-radius: .75rem; padding: 1rem; } .summary span, small { display: block; color: var(--muted); } .summary strong { color: var(--accent); display: block; font-size: 2rem; } .section-heading { display: flex; justify-content: space-between; align-items: start; gap: 1rem; } .controls { display: flex; flex-wrap: wrap; gap: .5rem; align-items: center; margin-block: 1rem; } .controls label { font-weight: 700; } .controls input, .controls select, button { min-height: 44px; border: 1px solid var(--line); border-radius: .4rem; padding: .55rem .7rem; font: inherit; } .controls input, .controls select { background: #fff; color: #10221b; } button { background: var(--accent); color: #062519; font-weight: 800; } button:disabled { opacity: .75; } .table-wrap { overflow-x: auto; border: 1px solid var(--line); border-radius: .5rem; } table { min-width: 700px; width: 100%; border-collapse: collapse; } th, td { text-align: left; padding: .75rem; vertical-align: top; border-bottom: 1px solid color-mix(in srgb, var(--line), transparent 50%); } th { color: var(--accent); } details summary { cursor: pointer; font-weight: 800; } .helper, #results { color: var(--muted); } .skip-link { position: fixed; left: .5rem; top: .5rem; transform: translateY(-200%); background: var(--focus); color: #000; padding: .5rem; z-index: 1; } .skip-link:focus { transform: translateY(0); } :focus-visible { outline: 3px solid var(--focus); outline-offset: 3px; } @media (max-width: 600px) { main { width: min(100% - 1rem, 1120px); padding-top: 1rem; } .summary { grid-template-columns: 1fr; } .section-heading { display: block; } .section-heading button { width: 100%; } } @media (prefers-reduced-motion: reduce) { *, *::before, *::after { scroll-behavior: auto !important; transition: none !important; animation: none !important; } }`;

const snapshotScript = `(() => { const search = document.querySelector("#task-search"); const filter = document.querySelector("#task-status"); const rows = [...document.querySelectorAll("#task-rows tr")]; const results = document.querySelector("#results"); for (const status of [...new Set(rows.map((row) => row.dataset.status))].sort()) { const option = document.createElement("option"); option.value = status; option.textContent = status; filter?.append(option); } function apply() { const query = search?.value.trim().toLowerCase() || ""; const status = filter?.value || ""; let visible = 0; for (const row of rows) { const show = (!query || row.dataset.search.includes(query)) && (!status || row.dataset.status === status); row.hidden = !show; if (show) visible += 1; } if (results) results.textContent = visible + " task" + (visible === 1 ? "" : "s") + " shown."; } search?.addEventListener("input", apply); filter?.addEventListener("change", apply); document.querySelector("#refresh")?.addEventListener("click", () => location.reload()); apply(); })();`;

/** Render a standalone, local-only snapshot. The browser reads embedded data and never reads the tracker. */
export function renderDashboard(model) {
  const progress = model.progress || {};
  const freshness = model.freshness || {};
  const percentage = progress.percentage === null ? "N/A" : `${progress.percentage}%`;
  const live = model.mode === "live";
  const graph = model.graph?.status === "available" && typeof model.graph?.content === "string" ? model.graph : null;
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Agent-Team status</title><style>${snapshotCss}</style></head>
<body><a class="skip-link" href="#tasks">Skip to tasks</a><main id="dashboard" tabindex="-1">
<header class="hero"><p class="eyebrow">${live ? "LOCAL LIVE STATUS" : "LOCAL STATUS SNAPSHOT"}</p><h1>${escapeHtml(model.project?.id || "Agent-Team")}</h1><p id="freshness" role="status">Source: ${escapeHtml(freshness.source || "unknown")} · ${escapeHtml(freshness.status || "unknown")}${freshness.observedAt ? ` · ${escapeHtml(freshness.observedAt)}` : ""}</p></header>
<section aria-labelledby="overview-title"><h2 id="overview-title">Overview</h2><div class="summary"><article><span>Completion</span><strong>${percentage}</strong><small>${escapeHtml(progress.completed ?? "?")} complete · ${escapeHtml(progress.remaining ?? "?")} remaining</small></article><article><span>Recorded work</span><strong>${escapeHtml(model.activity?.active ?? "?")}</strong><small>${escapeHtml(model.activity?.parked ?? "?")} parked · ${escapeHtml(model.activity?.paused ?? "?")} paused · ${escapeHtml(model.activity?.ready ?? "?")} ready</small></article><article><span>Capacity</span><strong>${escapeHtml(model.activity?.capacity ?? "Unknown")}</strong><small>Supplied scheduler capacity; not inferred here.</small></article></div><p>Progress confidence: ${escapeHtml(progress.status || "unknown")}. Task-count completion only; not estimated effort. Excluded: ${escapeHtml(progress.excluded?.cancelled ?? "?")} cancelled, ${escapeHtml(progress.excluded?.deferred ?? "?")} approved-deferred.</p><p>Current run: ${escapeHtml(model.run?.current || "unknown")} (${escapeHtml(model.run?.scope?.status || "unknown")} scope). Admissions are ${model.run?.paused === true ? "paused" : model.run?.paused === false ? "not paused" : "unknown"}. Integration: ${escapeHtml(model.state?.integration?.status || "unknown")}. Release: ${escapeHtml(model.state?.release?.status || "unknown")}. Recorded compute state is not live process liveness.</p><p>Blockers: ${escapeHtml(model.run?.blockerStatus === "unknown" ? "unknown" : model.run?.blockers?.join(", ") || "none recorded")}.</p></section>
<section id="tasks" aria-labelledby="tasks-title"><div class="section-heading"><div><h2 id="tasks-title">All tasks</h2><p>${escapeHtml(model.tasks?.length ?? 0)} rows; counts exclude cancelled and approved-deferred work.</p></div><button type="button" id="refresh" aria-describedby="refresh-help">${live ? "Refresh local status" : "Reload saved snapshot"}</button></div><p id="refresh-help" class="helper">${live ? "Refresh reads current local records through the enabled loopback helper." : "A file snapshot reloads its saved data; it cannot query project records."}</p><div class="controls"><label for="task-search">Search tasks</label><input id="task-search" type="search" autocomplete="off" placeholder="ID, owner, status…"><label for="task-status">Status</label><select id="task-status"><option value="">All statuses</option></select></div><div class="table-wrap" tabindex="0"><table><thead><tr><th>ID</th><th>Task</th><th>Status</th><th>Recorded state</th><th>Priority</th><th>Owner</th><th>Dependencies</th><th>Next action</th><th>Evidence</th></tr></thead><tbody id="task-rows">${taskRows(model.tasks || [])}</tbody></table></div><p id="results" aria-live="polite"></p></section>
<details><summary>Teams and roles</summary><ul>${(model.teams || []).map((team) => `<li><strong>${escapeHtml(team.name)}</strong> · ${escapeHtml(team.role)} · ${escapeHtml(team.model)} / ${escapeHtml(team.effort)} · ${escapeHtml(team.status)} · assignments: ${escapeHtml(team.assignments || "unknown")} · updated: ${escapeHtml(team.updatedAt || "unknown")}</li>`).join("") || "<li>Team metadata is unknown.</li>"}</ul></details>
<details ${graph ? "open" : ""}><summary>Dependency representation</summary>${graph ? `<p>Fresh optional Beads graph (${escapeHtml(graph.format)}); it is derived from the canonical export and does not control tasks.</p>${graphSvg(graph.content)}<pre aria-label="Dependency graph text" style="max-width:100%;overflow-x:auto;overflow-wrap:anywhere;white-space:pre-wrap">${escapeHtml(graph.content)}</pre>` : "<p>Each task row lists prerequisite task IDs in the Dependencies column. A graph is unavailable unless separately generated from a current Beads export.</p>"}</details>
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

function stableContent(value) {
  if (Array.isArray(value)) return value.map(stableContent);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(Object.keys(value).sort().filter((key) => key !== "observedAt" && key !== "refreshedAt").map((key) => [key, stableContent(value[key])]));
}

function contentFingerprint(model) {
  return createHash("sha256").update(JSON.stringify(stableContent(model))).digest("hex");
}

/** Explicit dashboard refresh writer. It publishes only derived files and shares concurrent refresh work. */
export function createSnapshotPublisher({ destination, derive, render = renderDashboard }) {
  if (!destination || typeof derive !== "function" || typeof render !== "function") throw new Error("Snapshot publisher requires destination, derive, and render.");
  let fingerprint = null;
  let inFlight = null;
  let lastGood = null;
  let lastModel = null;
  let stale = false;
  let initialized = false;
  const metadata = `${destination}.dashboard-meta.json`;
  async function restoreFingerprint() {
    if (initialized) return;
    initialized = true;
    try {
      const saved = JSON.parse(await readFile(metadata, "utf8"));
      if (typeof saved.fingerprint === "string" && await readFile(destination, "utf8")) {
        fingerprint = saved.fingerprint;
        lastGood = saved.lastGood || null;
        lastModel = saved.lastModel && typeof saved.lastModel === "object" ? saved.lastModel : null;
        stale = saved.stale === true;
      }
    } catch {
      // First generation and invalid derived metadata are both safely regenerated.
    }
  }
  async function refresh() {
    if (inFlight) return inFlight;
    inFlight = (async () => {
      try {
        await restoreFingerprint();
        const model = await derive();
        if (model instanceof Error) throw model;
        if (["unavailable", "unknown"].includes(model?.freshness?.status)) throw new Error(model.freshness.reason || `Canonical tracker is ${model.freshness.status}.`);
        const source = contentFingerprint(model);
        if (source === fingerprint && !stale) return { status: "unchanged", destination, lastGood };
        const html = render(model);
        await atomicWrite(destination, html);
        fingerprint = source;
        lastModel = model;
        stale = false;
        lastGood = { destination, refreshedAt: new Date().toISOString() };
        await atomicWrite(metadata, `${JSON.stringify({ fingerprint, lastGood, lastModel, stale })}\n`);
        return { status: "published", destination, lastGood };
      } catch (error) {
        stale = true;
        if (lastModel) {
          try {
            const staleModel = { ...lastModel, freshness: { ...lastModel.freshness, status: "stale", reason: String(error.message || error) } };
            await atomicWrite(destination, render(staleModel));
            await atomicWrite(metadata, `${JSON.stringify({ fingerprint, lastGood, lastModel, stale })}\n`);
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

async function boundedCommand(command, args, { cwd, timeoutMs, maxOutputBytes }) {
  try {
    const { stdout, stderr } = await runFile(command, args, { cwd, encoding: "utf8", timeout: timeoutMs, maxBuffer: maxOutputBytes });
    return { status: "completed", output: stdout.slice(0, maxOutputBytes), diagnostics: stderr.slice(0, maxOutputBytes) };
  } catch (error) {
    return { status: error.killed ? "timeout" : error.code === "ENOENT" ? "unavailable" : "failed", output: `${error.stdout || ""}${error.stderr || error.message || ""}`.slice(0, maxOutputBytes) };
  }
}

/**
 * Optional documented bd → bv graph boundary. Selection and terms acknowledgement
 * are caller-controlled; this does not install, vendor, or invoke interactive bv.
 */
export function createBeadsGraphCommandAdapter({ projectRoot, tracker, stagingDirectory = path.join(projectRoot || ".", ".agent-team", "dashboard", "beads"), selected = false, termsAcknowledged = false, runCommand = boundedCommand, ensureDirectory = mkdir, bdPath = "bd", bvPath = "bv", timeoutMs = 5000, maxOutputBytes = 256 * 1024 }) {
  if (!projectRoot || typeof runCommand !== "function") throw new Error("Beads command adapter requires projectRoot and runCommand.");
  const attribution = Object.freeze({
    repository: "https://github.com/Dicklesworthstone/beads_viewer",
    license: "https://github.com/Dicklesworthstone/beads_viewer/blob/main/LICENSE",
    provider: "Jeffrey Emanuel",
  });
  const trackerExecutable = tracker?.executable || bdPath;
  const trackerId = tracker?.id || null;
  const options = { cwd: projectRoot, timeoutMs: Math.min(Math.max(1, timeoutMs), 5000), maxOutputBytes: Math.min(Math.max(1024, maxOutputBytes), 256 * 1024), env: { PATH: process.env.PATH || "" } };
  const graphOptions = { ...options, cwd: stagingDirectory };
  const exportFile = path.join(stagingDirectory, ".beads", "issues.jsonl");
  async function capability() {
    if (!selected || !termsAcknowledged) return { status: "not_selected", reason: !selected ? "Optional Beads graph is not selected." : "Operator terms acknowledgement is required.", attribution };
    if (tracker && tracker.kind !== "beads") return { status: "unavailable", reason: "Selected tracker is not Beads.", attribution };
    const trackerVersion = await runCommand(trackerExecutable, ["--version"], options);
    if (trackerVersion.status !== "completed") return { status: "unavailable", reason: `Selected tracker version check: ${trackerVersion.status}`, attribution };
    const version = await runCommand(bvPath, ["--version"], options);
    if (version.status !== "completed") return { status: "unavailable", reason: `bv version check: ${version.status}`, attribution };
    const robot = await runCommand(bvPath, ["--robot-help"], options);
    if (robot.status !== "completed" || !/robot-graph/i.test(robot.output) || !/graph-format/i.test(robot.output) || !/no-hooks/i.test(robot.output)) return { status: "unavailable", reason: "bv robot graph capability is unavailable.", attribution };
    return { status: "available", version: version.output.trim(), trackerVersion: trackerVersion.output.trim(), attribution };
  }
  return Object.freeze({
    attribution,
    capability,
    async refresh() {
      const available = await capability();
      if (available.status !== "available") return { ...available, graph: null };
      await ensureDirectory(path.dirname(exportFile), { recursive: true, mode: 0o700 });
      const exported = await runCommand(trackerExecutable, ["export", "-o", exportFile], options);
      if (exported.status !== "completed") return { status: "unavailable", reason: `Canonical bd export: ${exported.status}`, graph: null, attribution };
      const rendered = await runCommand(bvPath, ["--robot-graph", "--graph-format=dot", "--no-hooks"], graphOptions);
      if (rendered.status !== "completed") return { status: "unavailable", reason: `bv graph: ${rendered.status}`, graph: null, attribution };
      try {
        const payload = JSON.parse(rendered.output);
        if (typeof payload.graph !== "string" || Buffer.byteLength(payload.graph) > options.maxOutputBytes) throw new Error("Missing or oversized graph output.");
        return { status: "available", graph: { status: "available", format: "dot", content: payload.graph, source: { trackerId, exportFile } }, attribution };
      } catch (error) {
        return { status: "unavailable", reason: String(error.message || error), graph: null, attribution };
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
export function createLoopbackDashboard({ assetsDirectory, readModel, render = renderDashboard, port: requestedPort = 0, deadlineMs: requestedDeadlineMs = 1000, maxOutputBytes: requestedMaxOutputBytes = 512 * 1024 }) {
  if (!assetsDirectory || typeof readModel !== "function") throw new Error("Loopback dashboard requires assetsDirectory and readModel.");
  let server;
  let port;
  let collection;
  const deadlineMs = Number.isFinite(requestedDeadlineMs) ? Math.max(1, requestedDeadlineMs) : 1000;
  const maxOutputBytes = Number.isFinite(requestedMaxOutputBytes) ? Math.max(1024, requestedMaxOutputBytes) : 512 * 1024;
  const assets = new Map([["/dashboard.css", ["dashboard.css", "text/css; charset=utf-8"]], ["/dashboard.js", ["dashboard.js", "application/javascript; charset=utf-8"]]]);
  const respond = (response, status, body, type = "text/plain; charset=utf-8") => response.writeHead(status, { "content-type": type, "cache-control": "no-store", "x-content-type-options": "nosniff" }).end(body);
  const collect = async () => {
    if (collection) return collection.result;
    const entry = { controller: new AbortController() };
    entry.work = Promise.resolve().then(async () => {
      const model = await readModel({ signal: entry.controller.signal, deadlineMs, maxOutputBytes });
      const html = render({ ...model, mode: "live" });
      if (Buffer.byteLength(html) > maxOutputBytes) throw new Error("Status output exceeds limit.");
      return html;
    });
    entry.work.catch(() => {});
    entry.result = new Promise((resolve, reject) => {
      const timer = setTimeout(() => { entry.controller.abort(); reject(new Error("Status collection deadline exceeded.")); }, deadlineMs);
      entry.work.then((value) => { clearTimeout(timer); resolve(value); }, (error) => { clearTimeout(timer); reject(error); });
    });
    entry.result.catch(() => {});
    collection = entry;
    entry.work.finally(() => { if (collection === entry) collection = undefined; }).catch(() => {});
    return entry.result;
  };
  return Object.freeze({
    async start() {
      if (server) return { port };
      server = createServer(async (request, response) => {
        if (!validHost(request.headers.host || "", port) || !validOrigin(request.headers.origin, port)) return respond(response, 403, "Forbidden");
        if (request.method !== "GET") return respond(response, 405, "Method Not Allowed");
        const rawUrl = request.url || "/";
        if (!rawUrl.startsWith("/") || rawUrl.startsWith("//")) return respond(response, 400, "Malformed request target");
        let pathname;
        try {
          pathname = new URL(rawUrl, "http://localhost").pathname;
        } catch {
          return respond(response, 400, "Malformed request target");
        }
        try {
          if (pathname === "/") return respond(response, 200, await collect(), "text/html; charset=utf-8");
          const asset = assets.get(pathname);
          if (!asset) return respond(response, 404, "Not Found");
          return respond(response, 200, await readFile(path.join(assetsDirectory, asset[0]), "utf8"), asset[1]);
        } catch {
          return respond(response, 503, "Status snapshot unavailable");
        }
      });
      server.keepAliveTimeout = deadlineMs;
      await new Promise((resolve, reject) => { server.once("error", reject); server.listen(requestedPort, "127.0.0.1", resolve); });
      port = server.address().port;
      return { port };
    },
    async stop() {
      if (!server) return;
      const closing = server;
      server = undefined;
      collection?.controller.abort();
      await new Promise((resolve, reject) => {
        const timeout = setTimeout(() => { closing.closeAllConnections?.(); resolve(); }, deadlineMs);
        closing.close((error) => { clearTimeout(timeout); error ? reject(error) : resolve(); });
      });
    },
  });
}
