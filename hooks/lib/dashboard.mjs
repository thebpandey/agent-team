import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { execFile } from "node:child_process";
import path from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { promisify } from "node:util";
import { readFileSync } from 'node:fs';
import { beadsEnvironment } from './tracker.mjs';

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

function graphSvg(adjacency) {
  const allNodes = Array.isArray(adjacency?.nodes) ? adjacency.nodes : [];
  const allEdges = Array.isArray(adjacency?.edges) ? adjacency.edges : [];
  const nodes = allNodes.slice(0, 32);
  const nodeIds = new Set(nodes.map((node) => node.id));
  const edges = allEdges.filter((edge) => nodeIds.has(edge.from) && nodeIds.has(edge.to)).slice(0, 48);
  if (!nodes.length) return "<p>Graph data has no safe, renderable edges; the textual graph remains available below.</p>";
  const width = 800;
  const height = Math.max(180, Math.ceil(nodes.length / 4) * 120);
  const point = (id) => { const index = nodes.findIndex((node) => node.id === id); return { x: 85 + (index % 4) * 205, y: 70 + Math.floor(index / 4) * 110 }; };
  const excerpt = allNodes.length > nodes.length || allEdges.length > edges.length;
  return `<p>${excerpt ? `Visual excerpt: ${nodes.length} of ${allNodes.length} nodes, ${edges.length} of ${allEdges.length} edges. ` : ''}Select a node with click, Enter or Space to filter tasks. Full dependency text is available below.</p><svg style="display:block;width:100%;height:auto" viewBox="0 0 ${width} ${height}" role="group" aria-label="Dependency graph"><defs><marker id="arrow" markerWidth="8" markerHeight="8" refX="7" refY="4" orient="auto"><path d="M0,0 L8,4 L0,8 z" fill="#75ff57"/></marker></defs>${edges.map((edge) => { const a = point(edge.from); const b = point(edge.to); const dx = b.x - a.x; const dy = b.y - a.y; const distance = Math.hypot(dx, dy) || 1; const end = Math.max(0, distance - 43); return `<line x1="${a.x}" y1="${a.y}" x2="${a.x + (dx / distance) * end}" y2="${a.y + (dy / distance) * end}" stroke="#5da892" stroke-width="2" marker-end="url(#arrow)"><title>${escapeHtml(edge.type || "depends on")}</title></line>`; }).join("")}${nodes.map((node) => { const p = point(node.id); const label = node.title || node.id; return `<g tabindex="0" role="button" aria-label="Filter task ${escapeHtml(node.id)}" data-graph-task="${escapeHtml(node.id)}"><circle cx="${p.x}" cy="${p.y}" r="35" fill="#07513b" stroke="#75ff57"/><text x="${p.x}" y="${p.y + 5}" text-anchor="middle" fill="#fff">${escapeHtml(label)}</text><title>${escapeHtml(node.id)}</title></g>`; }).join("")}</svg>`;
}

function graphText(adjacency) {
  const edges = Array.isArray(adjacency?.edges) ? adjacency.edges : [];
  return edges.length ? edges.map((edge) => `${edge.from} --${edge.type || "depends on"}--> ${edge.to}`).join("\n") : "No dependency edges reported.";
}

// Keep the distributed assets/dashboard files in sync with these snapshot-safe copies.
const snapshotCss = readFileSync(new URL('../../assets/dashboard/dashboard.css', import.meta.url), 'utf8');

const snapshotScript = readFileSync(new URL('../../assets/dashboard/dashboard.js', import.meta.url), 'utf8');

/** Render a standalone, local-only snapshot. The browser reads embedded data and never reads the tracker. */
export function renderDashboard(model) {
  const progress = model.progress || {};
  const freshness = model.freshness || {};
  const percentage = Number.isFinite(progress.percentage) && progress.percentage >= 0 && progress.percentage <= 100 ? `${progress.percentage}%` : "N/A";
  const live = model.mode === "live";
  const graph = model.graph?.status === "available" && model.graph?.adjacency ? model.graph : null;
  const graphRepository = graph?.attribution?.repository;
  const graphLicense = graph?.attribution?.license;
  const graphProvider = graph?.attribution?.provider;
  const graphAttribution = graphRepository && graphLicense && graphProvider
    ? ` External graph attribution: <a href="${escapeHtml(graphRepository)}">beads_viewer by ${escapeHtml(graphProvider)}</a> · <a href="${escapeHtml(graphLicense)}">license terms</a>.`
    : "";
  return `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>Agent-Team status</title><style>${snapshotCss}</style></head>
<body><a class="skip-link" href="#tasks">Skip to tasks</a><main id="dashboard" tabindex="-1">
<header class="hero"><p class="eyebrow">${live ? "LOCAL LIVE STATUS" : "LOCAL STATUS SNAPSHOT"}</p><h1>${escapeHtml(model.project?.id || "Agent-Team")}</h1><p id="freshness" role="status">Source: ${escapeHtml(freshness.source || "unknown")} · ${escapeHtml(freshness.status || "unknown")}${freshness.observedAt ? ` · ${escapeHtml(freshness.observedAt)}` : ""}${freshness.reason ? ` · ${escapeHtml(freshness.reason)}` : ''}</p></header>
<section aria-labelledby="overview-title"><h2 id="overview-title">Overview</h2><div class="summary"><article><span>Completion</span><strong>${percentage}</strong><small>${escapeHtml(progress.completed ?? "?")} complete · ${escapeHtml(progress.remaining ?? "?")} remaining</small></article><article><span>Recorded work</span><strong>${escapeHtml(model.activity?.active ?? "?")}</strong><small>${escapeHtml(model.activity?.parked ?? "?")} parked · ${escapeHtml(model.activity?.paused ?? "?")} paused · ${escapeHtml(model.activity?.ready ?? "?")} ready</small></article><article><span>Capacity</span><strong>${escapeHtml(model.activity?.capacity ?? "Unknown")}</strong><small>Supplied scheduler capacity; not inferred here.</small></article></div><p>Progress confidence: ${escapeHtml(progress.status || "unknown")}. Task-count completion only; not estimated effort. Excluded: ${escapeHtml(progress.excluded?.cancelled ?? "?")} cancelled, ${escapeHtml(progress.excluded?.deferred ?? "?")} approved-deferred.</p><p>Current run: ${escapeHtml(model.run?.current || "unknown")} (${escapeHtml(model.run?.scope?.status || "unknown")} scope). Admissions are ${model.run?.paused === true ? "paused" : model.run?.paused === false ? "not paused" : "unknown"}. Integration: ${escapeHtml(model.state?.integration?.status || "unknown")}. Release: ${escapeHtml(model.state?.release?.status || "unknown")}. Recorded compute state is not live process liveness.</p><p>Blockers: ${escapeHtml(model.run?.blockerStatus === "unknown" ? "unknown" : model.run?.blockers?.join(", ") || "none recorded")}.</p></section>
<section id="tasks" aria-labelledby="tasks-title"><div class="section-heading"><div><h2 id="tasks-title">All tasks</h2><p>${escapeHtml(model.tasks?.length ?? 0)} rows; counts exclude cancelled and approved-deferred work.</p></div><button type="button" id="refresh" aria-describedby="refresh-help">${live ? "Refresh local status" : "Reload saved snapshot"}</button></div><p id="refresh-help" class="helper">${live ? "Refresh reads current local records through the enabled loopback helper." : "A file snapshot reloads its saved data; it cannot query project records."}</p><div class="controls"><label for="task-search">Search tasks</label><input id="task-search" type="search" autocomplete="off" placeholder="ID, owner, status…"><label for="task-status">Status</label><select id="task-status"><option value="">All statuses</option></select></div><div class="table-wrap" tabindex="0"><table><thead><tr><th>ID</th><th>Task</th><th>Status</th><th>Recorded state</th><th>Priority</th><th>Owner</th><th>Dependencies</th><th>Next action</th><th>Evidence</th></tr></thead><tbody id="task-rows">${taskRows(model.tasks || [])}</tbody></table></div><p id="results" aria-live="polite"></p></section>
<details><summary>Teams and roles</summary><ul>${(model.teams || []).map((team) => `<li><strong>${escapeHtml(team.name)}</strong> · ${escapeHtml(team.role)} · ${escapeHtml(team.model)} / ${escapeHtml(team.effort)} · ${escapeHtml(team.status)} · assignments: ${escapeHtml(team.assignments || "unknown")} · updated: ${escapeHtml(team.updatedAt || "unknown")}</li>`).join("") || "<li>Team metadata is unknown.</li>"}</ul></details>
<details ${graph ? "open" : ""}><summary>Dependency representation</summary>${graph ? `<p>Optional graph from a canonical Beads export (${escapeHtml(graph.format)}); it does not control tasks.${graphAttribution}</p>${graphSvg(graph.adjacency)}<pre aria-label="Dependency graph text" style="max-width:100%;overflow-x:auto;overflow-wrap:anywhere;white-space:pre-wrap">${escapeHtml(graphText(graph.adjacency))}</pre>` : "<p>Each task row lists prerequisite task IDs in the Dependencies column. A graph is unavailable unless separately generated from a current Beads export.</p>"}</details>
<script id="dashboard-data" type="application/json">${jsonForScript(model)}</script><script>${snapshotScript}</script></main></body></html>`;
}

async function atomicWrite(destination, content, budget) {
  budget?.check();
  await mkdir(path.dirname(destination), { recursive: true, mode: 0o700 });
  const temporary = `${destination}.${randomUUID()}.tmp`;
  try {
    budget?.check();
    await writeFile(temporary, content, { encoding: 'utf8', mode: 0o600, ...(budget ? { signal: budget.signal } : {}) });
    budget?.check();
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
export function createSnapshotPublisher({ destination, derive, render = renderDashboard, budget }) {
  if (!destination || typeof derive !== "function" || typeof render !== "function") throw new Error("Snapshot publisher requires destination, derive, and render.");
  let fingerprint = null;
  let inFlight = null;
  let lastGood = null;
  let lastModel = null;
  let stale = false;
  let initialized = false;
  const metadata = `${destination}.dashboard-meta.json`;
  const bounded = (action) => budget ? budget.run(action) : action();
  const digest = html => createHash('sha256').update(html).digest('hex');
  const metadataFor = html => `${JSON.stringify({ schemaVersion: 1, fingerprint, lastGood, lastModel, stale, htmlDigest: digest(html) })}\n`;
  async function restoreFingerprint() {
    if (initialized) return;
    initialized = true;
    try {
      const saved = JSON.parse(await bounded(() => readFile(metadata, "utf8")));
      if (saved.schemaVersion === 1 && saved.lastModel && typeof saved.lastModel === 'object' && contentFingerprint(saved.lastModel) === saved.fingerprint) {
        const html = await bounded(() => readFile(destination, 'utf8'));
        const matches = typeof saved.htmlDigest === 'string' && digest(html) === saved.htmlDigest;
        fingerprint = matches ? saved.fingerprint : null;
        lastGood = saved.lastGood || null;
        lastModel = saved.lastModel && typeof saved.lastModel === "object" ? saved.lastModel : null;
        stale = saved.stale === true || !matches;
      }
    } catch {
      // First generation and invalid derived metadata are both safely regenerated.
    }
  }
  async function refresh() {
    if (inFlight) return inFlight;
    inFlight = (async () => {
      try {
        await bounded(restoreFingerprint);
        const model = await bounded(derive);
        budget?.check();
        if (model instanceof Error) throw model;
        if (["unavailable", "unknown"].includes(model?.freshness?.status)) throw new Error(model.freshness.reason || `Canonical tracker is ${model.freshness.status}.`);
        const source = contentFingerprint(model);
        if (source === fingerprint && !stale) return { status: "unchanged", destination, lastGood };
        const html = render(model);
        await bounded(() => atomicWrite(destination, html, budget));
        fingerprint = source;
        lastModel = model;
        stale = false;
        lastGood = { destination, refreshedAt: new Date().toISOString() };
        await bounded(() => atomicWrite(metadata, metadataFor(html), budget));
        return { status: "published", destination, lastGood };
      } catch (error) {
        stale = true;
        if (lastModel && !budget?.signal.aborted && (budget?.remaining() ?? 1) > 0) {
          try {
            const staleModel = { ...lastModel, freshness: { ...lastModel.freshness, status: "stale", reason: String(error.message || error) } };
            const html = render(staleModel);
            await bounded(() => atomicWrite(destination, html, budget));
            await bounded(() => atomicWrite(metadata, metadataFor(html), budget));
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

async function boundedCommand(command, args, { cwd, timeoutMs, maxOutputBytes, env, signal }) {
  try {
    const { stdout, stderr } = await runFile(command, args, { cwd, env, signal, encoding: "utf8", timeout: timeoutMs, maxBuffer: maxOutputBytes });
    return { status: "completed", output: stdout.slice(0, maxOutputBytes), diagnostics: stderr.slice(0, maxOutputBytes) };
  } catch (error) {
    return { status: error.killed || error.code === "ABORT_ERR" || signal?.aborted ? "timeout" : error.code === "ENOENT" ? "unavailable" : "failed", output: `${error.stdout || ""}${error.stderr || error.message || ""}`.slice(0, maxOutputBytes) };
  }
}

/**
 * Optional documented bd → bv graph boundary. Selection and terms acknowledgement
 * are caller-controlled; this does not install, vendor, or invoke interactive bv.
 */
export function createBeadsGraphCommandAdapter({ projectRoot, tracker, stagingDirectory = path.join(projectRoot || ".", ".agent-team", "dashboard", "beads"), selected = false, termsAcknowledged = false, runCommand = boundedCommand, ensureDirectory = mkdir, bdPath = "bd", bvPath = "bv", timeoutMs = 5000, maxOutputBytes = 256 * 1024, budget, signal, reserveMs = 0 }) {
  if (!projectRoot || typeof runCommand !== "function") throw new Error("Beads command adapter requires projectRoot and runCommand.");
  const attribution = Object.freeze({
    repository: "https://github.com/Dicklesworthstone/beads_viewer",
    license: "https://github.com/Dicklesworthstone/beads_viewer/blob/main/LICENSE",
    provider: "Jeffrey Emanuel",
  });
  const trackerExecutable = tracker?.executable || bdPath;
  const trackerId = tracker?.id || null;
  const environment = beadsEnvironment({ root: projectRoot, tracker: { ...tracker, path: tracker?.path ?? path.join(projectRoot, '.beads') } });
  const commandLimit = Math.min(Math.max(1, timeoutMs), 5000);
  const outputLimit = Math.min(Math.max(1024, maxOutputBytes), 256 * 1024);
  const reserve = Math.max(0, Number.isFinite(reserveMs) ? reserveMs : 0);
  const sharedSignal = budget?.signal && signal ? AbortSignal.any([budget.signal, signal]) : budget?.signal ?? signal;
  function options(cwd, env) {
    if (sharedSignal?.aborted) return null;
    try { budget?.check(); } catch { return null; }
    const remaining = budget ? budget.remaining() - reserve : commandLimit;
    if (remaining <= 0) return null;
    return { cwd, timeoutMs: Math.max(1, Math.min(commandLimit, remaining)), maxOutputBytes: outputLimit, env, ...(sharedSignal ? { signal: sharedSignal } : {}) };
  }
  async function command(executable, args, cwd = projectRoot, env = environment) {
    const commandOptions = options(cwd, env);
    return commandOptions ? runCommand(executable, args, commandOptions) : { status: "timeout", output: "Dashboard graph budget exhausted." };
  }
  const graphEnvironment = beadsEnvironment({ root: stagingDirectory, tracker: { path: path.join(stagingDirectory, '.beads') } }, environment);
  const exportFile = path.join(stagingDirectory, ".beads", "issues.jsonl");
  async function capability() {
    if (!selected || !termsAcknowledged) return { status: "not_selected", reason: !selected ? "Optional Beads graph is not selected." : "Operator terms acknowledgement is required.", attribution };
    if (tracker?.kind !== "beads" || typeof tracker.id !== 'string' || !tracker.id || typeof trackerExecutable !== 'string' || !trackerExecutable) return { status: "unavailable", reason: "A selected canonical Beads tracker and executable are required.", attribution };
    const trackerVersion = await command(trackerExecutable, ["--version"]);
    if (trackerVersion.status !== "completed") return { status: "unavailable", reason: `Selected tracker version check: ${trackerVersion.status}`, attribution };
    const version = await command(bvPath, ["--version"]);
    if (version.status !== "completed") return { status: "unavailable", reason: `bv version check: ${version.status}`, attribution };
    const robot = await command(bvPath, ["--robot-help"]);
    if (robot.status !== "completed" || !/robot-graph/i.test(robot.output) || !/graph-format/i.test(robot.output) || !/no-hooks/i.test(robot.output)) return { status: "unavailable", reason: "bv robot graph capability is unavailable.", attribution };
    return { status: "available", version: version.output.trim(), trackerVersion: trackerVersion.output.trim(), attribution };
  }
  return Object.freeze({
    attribution,
    capability,
    async refresh() {
      const available = await capability();
      if (available.status !== "available") return { ...available, graph: null };
      try {
        budget?.check();
        if (signal?.aborted) return { status: "unavailable", reason: "Graph request cancelled.", graph: null, attribution };
        await ensureDirectory(path.dirname(exportFile), { recursive: true, mode: 0o700 });
      } catch (error) {
        return { status: "unavailable", reason: `Graph staging directory: ${String(error.message || error)}`, graph: null, attribution };
      }
      const exported = await command(trackerExecutable, ["export", "-o", exportFile]);
      if (exported.status !== "completed") return { status: "unavailable", reason: `Canonical bd export: ${exported.status}`, graph: null, attribution };
      const rendered = await command(bvPath, ["--robot-graph", "--graph-format=json", "--no-hooks"], stagingDirectory, graphEnvironment);
      if (rendered.status !== "completed") return { status: "unavailable", reason: `bv graph: ${rendered.status}`, graph: null, attribution };
      try {
        const payload = JSON.parse(rendered.output);
        if (payload?.format !== "json") throw new Error("Unexpected graph format.");
        const adjacency = payload.adjacency ?? { nodes: [], edges: [] };
        if (!Array.isArray(adjacency.nodes) || (adjacency.edges !== null && !Array.isArray(adjacency.edges))) throw new Error("Invalid graph adjacency.");
        const nodes = adjacency.nodes.map((node) => {
          if (!node || typeof node.id !== "string" || !node.id || node.id.length > 256) throw new Error("Invalid graph node.");
          return { id: node.id, title: typeof node.title === "string" ? node.title.slice(0, 512) : node.id, status: typeof node.status === "string" ? node.status : "unknown", priority: typeof node.priority === "string" || Number.isFinite(node.priority) ? node.priority : "unknown" };
        });
        const ids = new Set(nodes.map((node) => node.id));
        const edges = (adjacency.edges || []).map((edge) => {
          if (!edge || typeof edge.from !== "string" || typeof edge.to !== "string" || !ids.has(edge.from) || !ids.has(edge.to)) throw new Error("Invalid graph edge.");
          return { from: edge.from, to: edge.to, type: typeof edge.type === "string" ? edge.type.slice(0, 128) : "depends on" };
        });
        if (nodes.length > 256 || edges.length > 1024 || Buffer.byteLength(JSON.stringify({ nodes, edges })) > outputLimit) throw new Error("Oversized graph output.");
        return { status: "available", graph: { status: "available", format: "json", adjacency: { nodes, edges }, source: { trackerId, exportFile, dataHash: typeof payload.data_hash === "string" ? payload.data_hash : null } }, attribution };
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
