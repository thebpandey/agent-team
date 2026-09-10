import { execFile, spawn } from "node:child_process";
import { constants } from "node:fs";
import { createHash, randomUUID } from "node:crypto";
import { access, chmod, cp, link, lstat, mkdir, mkdtemp, open, opendir, readFile, realpath, rm, rmdir, unlink, writeFile } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";
import { CATALOG_BY_ID, DEPENDENCY_CATALOG } from "./dependency-catalog.mjs";
import { mutateSetup } from "./settings.mjs";
import { beadsEnvironment } from "./tracker.mjs";
import { createEventBudget } from "./budget.mjs";
import { createBeadsGraphCommandAdapter } from "./dashboard.mjs";

const exec = promisify(execFile);
const HOSTS = new Set(["codex", "claude-code"]);

function includePrerequisites(ids) {
  const selected = new Set(ids);
  let changed = true;
  while (changed) {
    changed = false;
    for (const id of [...selected]) {
      for (const prerequisite of CATALOG_BY_ID.get(id)?.prerequisites ?? []) {
        if (!selected.has(prerequisite)) {
          selected.add(prerequisite);
          changed = true;
        }
      }
    }
  }
  return selected;
}

export function resolveCatalogSelection({ tracker, defaults, optionals = [] } = {}) {
  const defaultIds = defaults === undefined
    ? DEPENDENCY_CATALOG.filter(({ disposition }) => disposition === "default").map(({ id }) => id)
    : defaults;
  const requested = [
    ...DEPENDENCY_CATALOG.filter(({ disposition }) => disposition === "mandatory").map(({ id }) => id),
    ...defaultIds,
    ...(tracker?.kind === "beads" ? ["beads"] : []),
    ...optionals,
  ];
  for (const id of requested) {
    const entry = CATALOG_BY_ID.get(id);
    if (!entry || !["mandatory", "default", "tracker", "optional", "prerequisite"].includes(entry.disposition)) {
      throw new Error(`Dependency is not selectable: ${id}.`);
    }
  }
  const ids = includePrerequisites(requested);
  return {
    selected: DEPENDENCY_CATALOG.filter(({ id }) => ids.has(id)),
    optional: DEPENDENCY_CATALOG.filter(({ disposition, id }) => disposition === "optional" && !ids.has(id)),
    excluded: DEPENDENCY_CATALOG.filter(({ disposition }) => disposition === "excluded"),
  };
}

const TOOL_IDS = new Set(["uv", "serena", "playwright-cli", "ast-grep", "graphify", "lean-ctx", "beads", "beads-viewer", "context7"]);

export function inspectDependencies({ setup = {}, host }) {
  if (!HOSTS.has(host)) throw new Error(`Unknown dependency host: ${host ?? "missing"}.`);
  const profile = setup.dependencies?.hosts?.[host] ?? { selected: [], receipts: [] };
  const receipts = profile.receipts ?? [];
  const count = (items) => ({
    ready: items.filter(({ status }) => status === "ready").length,
    failed: items.filter(({ status }) => ["failed", "cannot_use", "required_unavailable"].includes(status)).length,
  });
  const tools = receipts.filter(({ id }) => TOOL_IDS.has(id));
  const skills = receipts.filter(({ id }) => !TOOL_IDS.has(id));
  const failed = receipts.filter(({ status }) => ["failed", "cannot_use", "required_unavailable"].includes(status));
  return {
    host,
    scope: profile.scope ?? "unknown",
    selected: [...(profile.selected ?? [])],
    receipts: structuredClone(receipts),
    groups: [
      { id: "runtimes_tools", ...count(tools) },
      { id: "skills", ...count(skills) },
      { id: "project_readiness", ready: failed.length ? 0 : (receipts.length ? 1 : 0), failed: failed.length ? 1 : 0 },
    ],
    unresolved: failed.map(({ id, boundary }) => ({ id, boundary: boundary ?? "No functional evidence." })),
  };
}

export async function inspectDependenciesFile({ setupPath, host }) {
  return inspectDependencies({ setup: JSON.parse(await readFile(setupPath, "utf8")), host });
}

export function buildPreparationPlan({ dependencyId, host, scope, paths, executable }) {
  if (!HOSTS.has(host)) throw new Error(`Unknown dependency host: ${host ?? "missing"}.`);
  if (!["user", "project"].includes(scope)) throw new Error(`Unknown dependency scope: ${scope ?? "missing"}.`);
  const dependency = CATALOG_BY_ID.get(dependencyId);
  if (!dependency || dependency.disposition === "excluded") throw new Error(`Dependency is not preparable: ${dependencyId}.`);
  const target = executable ?? binaryPath(dependency, paths.toolRoot);
  const install = [];
  if (dependency.install?.kind === "npm") {
    install.push({ file: "npm", args: ["install", "--global", "--prefix", paths.toolRoot, "--no-audit", "--no-fund", `${dependency.install.package}@${dependency.version}`] });
  } else if (dependency.install?.kind === "uv-tool") {
    install.push({
      file: binaryPath(CATALOG_BY_ID.get("uv"), paths.toolRoot),
      args: ["tool", "install", "--python", dependency.install.python, `${dependency.install.package}==${dependency.version}`],
    });
  } else if (dependency.install?.kind === "git-skill") {
    install.push({ file: "git", args: ["fetch", "--depth", "1", dependency.install.repository, dependency.install.revision] });
  } else if (dependency.install?.kind === "github-release") {
    install.push({ file: "download-verified-release", args: [dependency.install.source, dependency.install.checksums] });
  }
  // The upstream skill installer initializes cwd and overwrites existing skills.
  // Prepare the bundled companion ourselves, including on compatible CLI reuse.
  const browserInstall = dependency.id === "playwright-cli" ? {
    file: target, args: ["install-browser", "chromium"], cwd: paths.toolRoot,
    env: { PLAYWRIGHT_BROWSERS_PATH: path.join(paths.toolRoot, "playwright-browsers"), NO_UPDATE_NOTIFIER: "1" },
  } : undefined;
  if (dependency.id === "impeccable") {
    install.push({
      file: target,
      args: ["install", "-y", `--providers=${host === "codex" ? "codex" : "claude"}`, `--scope=${scope}`, "--no-hooks"],
      cwd: paths.projectRoot,
      env: { IMPECCABLE_HOME: path.join(paths.toolRoot, "impeccable-home") },
    });
  }
  const functionalSteps = {
    "browser-interaction": ["open isolated local fixture", "click fixture control", "verify changed page state", "close owned session"],
    "symbol-operation": ["activate isolated fixture project", "locate a known symbol through Serena"],
    "positive-negative-structural-pattern": ["match positive structural fixture", "reject negative structural fixture"],
    "code-graph-traversal": ["extract an isolated code fixture with --code-only --no-viz", "trace a known call path and explain a known symbol"],
    "detector-exit-contract": ["verify clean exit 0", "verify findings exit 2", "verify execution failure exit 1"],
  };
  const registration = dependency.id === "serena" ? {
    kind: "mcp-server",
    name: "serena",
    command: target,
    args: ["start-mcp-server", "--context", host, "--project", paths.projectRoot],
    scope,
    ownership: "agent-team-entry-only",
    status: "host-approval-required",
    boundary: "Merge only the owned Serena entry, complete host trust/reload, then verify it from a fresh worker.",
  } : undefined;
  return {
    id: dependency.id,
    version: dependency.version,
    source: dependency.install?.source,
    profile: structuredClone(dependency.profile ?? {}),
    install,
    ...(browserInstall ? { browserInstall } : {}),
    functional: { id: dependency.functionalCheck, steps: functionalSteps[dependency.functionalCheck] ?? ["run declared functional operation"] },
    ...(registration ? { registration } : {}),
  };
}

function evidence(result) {
  return result?.evidence ?? result?.stderr ?? result?.stdout ?? result?.reason;
}

function dependencyBoundary(dependency, result, fallback) {
  return [...new Set([evidence(result), dependency.boundary, fallback].filter(Boolean))].join(" ");
}

async function prepareOne(dependency, runner, budget) {
  const observedAt = new Date().toISOString();
  const run = async (request) => {
    try {
      budget?.check();
      const outcome = await runner({ ...request, budget });
      budget?.check();
      return outcome && typeof outcome.status === "string"
        ? outcome
        : { status: "failed", evidence: `Dependency runner returned a malformed ${request.phase} result.` };
    } catch (error) {
      if (error.code === "EVENT_DEADLINE") throw error;
      budget?.check();
      return { status: "failed", evidence: error.message };
    }
  };
  const probe = await run({ dependency, phase: "probe" });
  if (probe.status === "customized") {
    return {
      id: dependency.id, version: probe.version ?? null, detected: true, installed: "preserved",
      functional: "not_run", availableToWorker: "not_run", status: "cannot_use",
      boundary: dependencyBoundary(dependency, probe, "Existing customization is not owned by Agent-Team."), observedAt,
    };
  }
  const compatible = probe.status === "passed" && (dependency.version === null || probe.version === dependency.version);
  if (dependency.executable && !compatible && (probe.status === "passed" || dependency.id === "beads-viewer")) {
    return {
      id: dependency.id, version: probe.version ?? null, detected: true, installed: "preserved",
      functional: "not_run", availableToWorker: "not_run", status: "cannot_use", observedAt,
      boundary: `The selected executable ${dependency.executable} reports ${probe.version ?? "an unknown version"}; expected ${dependency.version}. It was preserved and no shadow tracker was installed.`,
    };
  }
  let installed = "reused";
  let detected = probe.status === "passed";
  if (!compatible) {
    const installation = await run({ dependency, phase: "install" });
    if (installation.status !== "passed") {
      return {
        id: dependency.id, version: installation.version ?? probe.version ?? null, detected,
        installed: "not_installed", functional: "not_run", availableToWorker: "not_run", status: "failed",
        boundary: dependencyBoundary(dependency, installation, "Installation failed."), observedAt,
      };
    }
    installed = "installed";
    const installedProbe = await run({ dependency, phase: "probe" });
    detected = installedProbe.status === "passed";
    if (!detected || (dependency.version !== null && installedProbe.version !== dependency.version)) {
      return {
        id: dependency.id, version: installedProbe.version ?? null, detected, installed,
        functional: "not_run", availableToWorker: "not_run", status: "failed", observedAt,
        boundary: `Installed ${dependency.id}, but expected ${dependency.version} and detected ${installedProbe.version ?? "no executable"}.`,
      };
    }
  }
  if (["playwright-cli", "lean-ctx"].includes(dependency.id)) {
    const companion = await run({ dependency, phase: "companion" });
    if (companion.status !== "passed") {
      return {
        id: dependency.id, version: dependency.version, detected, installed,
        functional: "not_run", availableToWorker: "not_run",
        status: companion.status === "customized" ? "cannot_use" : "failed", observedAt,
        boundary: dependencyBoundary(dependency, companion, "The selected-scope companion prerequisites were not prepared."),
      };
    }
  }
  const functional = await run({ dependency, phase: "functional", check: dependency.functionalCheck });
  if (functional.status !== "passed") {
    return {
      id: dependency.id, version: dependency.version, detected, installed, functional: "failed",
      availableToWorker: "not_run", status: "failed",
      boundary: dependencyBoundary(dependency, functional, "Functional verification failed."), observedAt,
    };
  }
  const worker = await run({ dependency, phase: "worker", check: "fresh-worker-discovery" });
  if (worker.status !== "passed") {
    return {
      id: dependency.id, version: dependency.version, detected, installed, functional: "passed",
      availableToWorker: worker.status === "unverified" ? "unknown" : "failed", status: "failed",
      boundary: dependencyBoundary(dependency, worker, "A fresh worker could not discover the capability."), observedAt,
    };
  }
  return {
    id: dependency.id, version: dependency.version, detected, installed, functional: "passed",
    availableToWorker: "passed", status: "ready", observedAt,
    evidence: { probe: evidence(probe) ?? null, functional: evidence(functional) ?? null, worker: evidence(worker) ?? null },
  };
}

export async function prepareDependencies({ setupPath, expectedVersion, writer, operationId, loadRegistry, host, scope, selections = {}, paths, runner, budget }) {
  if (!HOSTS.has(host)) throw new Error(`Unknown dependency host: ${host ?? "missing"}.`);
  if (!["user", "project"].includes(scope)) throw new Error(`Unknown dependency scope: ${scope ?? "missing"}.`);
  const execute = runner ?? createDependencyRunner({ host, scope, paths, budget });
  return mutateSetup({
    setupPath, expectedVersion, writer, operationId, loadRegistry, budget,
    operation: { kind: "dependencies", host, scope, selections },
    mutate: async (setup) => {
      const previousProfile = setup.dependencies?.hosts?.[host] ?? {};
      let declined = [...new Set(selections.declined === undefined ? (previousProfile.declined ?? []) : selections.declined)];
      if (selections.declined === undefined) {
        const explicitlySelected = new Set([...(selections.defaults ?? []), ...(selections.optionals ?? [])]);
        declined = declined.filter((id) => !explicitlySelected.has(id));
      }
      for (const id of declined) {
        const entry = CATALOG_BY_ID.get(id);
        if (!entry || !["mandatory", "default", "optional"].includes(entry.disposition)) {
          throw new Error(`Dependency cannot be declined: ${id}.`);
        }
      }
      const declinedSet = new Set(declined);
      const defaults = (selections.defaults ?? DEPENDENCY_CATALOG.filter(({ disposition }) => disposition === "default").map(({ id }) => id))
        .filter((id) => !declinedSet.has(id));
      const optionals = (selections.optionals ?? []).filter((id) => !declinedSet.has(id));
      const selection = resolveCatalogSelection({ tracker: setup.tracker, defaults, optionals });
      if (selection.selected.some(({ id }) => id === "beads-viewer") && (setup.tracker?.kind !== "beads"
        || setup.dashboard?.graph?.enabled !== true || setup.dashboard.graph.termsAcknowledged !== true)) {
        throw new Error("bv preparation requires the selected Beads tracker and enabled graph with terms acknowledged.");
      }
      const dependencies = selection.selected.map((dependency) => dependency.id === "beads" && setup.tracker?.executable
        ? { ...dependency, executable: setup.tracker.executable }
        : dependency.id === "beads-viewer" ? { ...dependency,
          ...(setup.dashboard?.graph?.executable ? { executable: setup.dashboard.graph.executable } : {}),
          trackerExecutable: setup.tracker?.executable ?? binaryPath(CATALOG_BY_ID.get("beads"), paths.toolRoot),
        }
        : dependency);
      const receipts = [];
      for (const dependency of dependencies) {
        budget?.check();
        if (dependency.disposition === "mandatory" && declinedSet.has(dependency.id)) {
          receipts.push({
            id: dependency.id, version: dependency.version, detected: false, installed: "not_installed",
            functional: "not_run", availableToWorker: "failed", status: "required_unavailable",
            boundary: `Mandatory dependency ${dependency.id} was explicitly declined; readiness remains unavailable until it is approved and verified.`,
            observedAt: new Date().toISOString(),
          });
        } else {
          const receipt = await prepareOne(dependency, execute, budget);
          if (dependency.id === "beads-viewer") {
            receipt.executable = binaryPath(dependency, paths.toolRoot);
            receipt.termsAcknowledged = true;
            if (receipt.functional === "passed") setup.dashboard.graph.executable = receipt.executable;
          }
          receipts.push(receipt);
        }
      }
      budget?.check();
      const next = setup;
      next.dependencies ??= {};
      next.dependencies.hosts ??= {};
      next.dependencies.hosts[host] = {
        ...previousProfile,
        scope,
        selected: dependencies.map(({ id }) => id),
        declined,
        receipts,
        preparedAt: new Date().toISOString(),
      };
      return { setup: next, result: { preparationStatus: receipts.every(({ status }) => status === "ready") ? "ready" : "incomplete", receipts } };
    },
  }).then((result) => result.status === "applied"
    ? { ...result, status: result.preparationStatus }
    : { ...result, receipts: result.receipts ?? [] });
}

function binaryPath(dependency, toolRoot) {
  if (dependency.executable) return dependency.executable;
  if (dependency.id === "beads-viewer") return path.join(toolRoot, `bv-${dependency.version}`, process.platform === "win32" ? "bv.exe" : "bv");
  // Keep old npm shims/custom binaries untouched; register this exact scoped release path.
  if (dependency.id === "lean-ctx" && dependency.install?.kind === "github-release") {
    return path.join(toolRoot, `lean-ctx-${dependency.version}`, process.platform === "win32" ? "lean-ctx.exe" : "lean-ctx");
  }
  const command = dependency.install?.command ?? dependency.id;
  return path.join(toolRoot, "bin", process.platform === "win32" ? `${command}.cmd` : command);
}

async function command(file, args, options = {}) {
  const { budget, timeout = 120_000, ...executionOptions } = options;
  try {
    budget?.check();
    const result = await exec(file, args, {
      encoding: "utf8", maxBuffer: 1024 * 1024, ...executionOptions,
      timeout: budget?.timeout(timeout) ?? timeout, signal: budget?.signal ?? executionOptions.signal,
    });
    budget?.check();
    return { status: "passed", code: 0, stdout: result.stdout, stderr: result.stderr };
  } catch (error) {
    if (error.code === "EVENT_DEADLINE") throw error;
    budget?.check();
    return { status: error.code === "ENOENT" ? "not_found" : "failed", code: typeof error.code === "number" ? error.code : null, stdout: error.stdout, stderr: error.stderr, evidence: error.message };
  }
}

async function exists(file) {
  try { await access(file); return true; } catch { return false; }
}

function parsedVersion(output, dependencyId) {
  // Serena's release CLI appends the current repository's commit, not a prerelease tag.
  if (dependencyId === "serena") {
    const release = String(output ?? "").trim().match(/^Serena (\d+\.\d+\.\d+)(?:-[0-9a-f]{8}(?:-dirty)?)?$/);
    if (release) return release[1];
  }
  return String(output ?? "").match(/\d+\.\d+\.\d+(?:[-+][\w.-]+)?/)?.[0] ?? null;
}

function skillDestination(dependency, paths, selectedPath) {
  return path.join(paths.skillRoot, selectedPath === "." ? dependency.id : path.basename(selectedPath));
}

async function inspectSkillDestinations(dependency, paths, { budget, bounded = false } = {}) {
  let absent = false;
  for (const selectedPath of dependency.install.paths) {
    budget?.check();
    const destination = skillDestination(dependency, paths, selectedPath);
    if (!await exists(destination)) {
      absent = true;
      continue;
    }
    const metadataPath = path.join(destination, ".agent-team-source.json");
    try {
      const metadata = JSON.parse(bounded ? await companionBytes(metadataPath, budget, 64 * 1024) : await readFile(metadataPath, "utf8"));
      if (metadata.source !== dependency.install.source || metadata.revision !== dependency.version || metadata.selectedPath !== selectedPath) {
        return { status: "customized", evidence: `Preserved existing skill path with different provenance: ${destination}` };
      }
    } catch (error) {
      if (error.code === "EVENT_DEADLINE") throw error;
      budget?.check();
      return { status: "customized", evidence: `Preserved existing skill path without matching Agent-Team provenance: ${destination}. ${error.message}` };
    }
  }
  return absent
    ? { status: "not_found" }
    : { status: "passed", version: dependency.version, evidence: "All selected managed skill paths matched." };
}

async function installGitSkills(dependency, paths) {
  if (!dependency.install.paths.length) return { status: "failed", evidence: "No selective skill paths are approved for this optional source." };
  const preflight = await inspectSkillDestinations(dependency, paths);
  if (preflight.status === "customized") return preflight;
  const sourceRoot = path.join(paths.toolRoot, "sources", `${dependency.id}-${dependency.version.slice(0, 12)}`);
  await mkdir(path.dirname(sourceRoot), { recursive: true, mode: 0o700 });
  if (!await exists(path.join(sourceRoot, ".git"))) {
    const cloned = await command("git", ["clone", "--filter=blob:none", "--no-checkout", dependency.install.repository, sourceRoot]);
    if (cloned.status !== "passed") return cloned;
  }
  const fetched = await command("git", ["-C", sourceRoot, "fetch", "--depth", "1", "origin", dependency.install.revision]);
  if (fetched.status !== "passed") return fetched;
  const checkedOut = await command("git", ["-C", sourceRoot, "checkout", "--detach", "FETCH_HEAD"]);
  if (checkedOut.status !== "passed") return checkedOut;
  await mkdir(paths.skillRoot, { recursive: true, mode: 0o700 });
  for (const selectedPath of dependency.install.paths) {
    const destination = skillDestination(dependency, paths, selectedPath);
    if (await exists(destination)) continue;
    if (selectedPath === ".") {
      await mkdir(destination, { recursive: true, mode: 0o700 });
      for (const includedPath of dependency.install.includePaths ?? []) {
        await cp(path.join(sourceRoot, includedPath), path.join(destination, includedPath), { recursive: true, errorOnExist: true, force: false });
      }
    } else {
      await cp(path.join(sourceRoot, selectedPath), destination, { recursive: true, errorOnExist: true, force: false });
    }
    await writeFile(path.join(destination, ".agent-team-source.json"), `${JSON.stringify({
      source: dependency.install.source,
      revision: dependency.version,
      selectedPath,
    }, null, 2)}\n`, { mode: 0o600 });
  }
  return { status: "passed", version: dependency.version, evidence: `Prepared ${dependency.install.paths.length} complete skill path(s).` };
}

function playwrightSkill(dependency) {
  return { ...dependency, install: { ...dependency.install, paths: ["skills/playwright-cli"] } };
}

// The pinned companion is ~100 KB. Bound even customized trees before reading/copying.
async function companionBytes(file, budget, maxBytes = 1024 * 1024) {
  budget?.check();
  const handle = await open(file, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
  try {
    budget?.check();
    const stat = await handle.stat();
    if (!stat.isFile() || stat.size > maxBytes) throw new Error(`Companion file byte/type limit exceeded: ${file}`);
    const chunks = [];
    let size = 0;
    const stream = handle.createReadStream({ autoClose: false, start: 0, end: maxBytes, signal: budget?.signal });
    for await (const chunk of stream) {
      budget?.check();
      size += chunk.length;
      if (size > maxBytes) throw new Error(`Companion file byte limit exceeded: ${file}`);
      chunks.push(chunk);
    }
    budget?.check();
    return Buffer.concat(chunks);
  } finally { await handle.close(); }
}

// Reject links/special files and compare every bundled file, not just SKILL.md.
async function skillContents(root, budget, prefix = "", bounds = { entries: 0, bytes: 0 }, depth = 0) {
  budget?.check();
  if (depth > 12) throw new Error("Companion directory depth limit exceeded.");
  if (!(await lstat(root)).isDirectory()) throw new Error(`Not a regular skill directory: ${root}`);
  const files = {};
  for await (const entry of await opendir(root)) {
    budget?.check();
    if (++bounds.entries > 128) throw new Error("Companion filesystem entry limit exceeded.");
    const name = entry.name;
    if (!prefix && name === ".agent-team-source.json") continue;
    const file = path.join(root, name);
    const relative = path.posix.join(prefix, name);
    const stat = await lstat(file);
    if (stat.isDirectory()) Object.assign(files, await skillContents(file, budget, relative, bounds, depth + 1));
    else if (stat.isFile()) {
      if (bounds.bytes + stat.size > 8 * 1024 * 1024) throw new Error("Companion total byte limit exceeded.");
      const bytes = await companionBytes(file, budget);
      bounds.bytes += bytes.length;
      if (bounds.bytes > 8 * 1024 * 1024) throw new Error("Companion total byte limit exceeded.");
      files[relative] = createHash("sha256").update(bytes).digest("hex");
    }
    else throw new Error(`Preserved non-regular skill path: ${file}`);
  }
  return Object.fromEntries(Object.entries(files).sort(([a], [b]) => a.localeCompare(b)));
}

async function preparePlaywrightSkill(dependency, paths, budget) {
  const companion = playwrightSkill(dependency);
  const destination = skillDestination(companion, paths, companion.install.paths[0]);
  try {
    budget?.check();
    // A symlink (including a dangling one) is not an owned destination.
    try {
      if (!(await lstat(destination)).isDirectory()) return { status: "customized", evidence: `Preserved non-directory companion: ${destination}` };
    } catch (error) { if (error.code !== "ENOENT") throw error; }
    const preflight = await inspectSkillDestinations(companion, paths, { budget, bounded: true });
    if (preflight.status === "customized") return preflight;
    const packageRoot = path.join(paths.toolRoot, process.platform === "win32" ? "node_modules" : "lib/node_modules", "@playwright/cli");
    const source = path.join(packageRoot, "skills/playwright-cli");
    const relative = path.relative(await realpath(paths.toolRoot), await realpath(source));
    if (relative === ".." || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative)) {
      return { status: "failed", evidence: "Pinned Playwright companion source escapes the managed tool root." };
    }
    const metadata = JSON.parse(await companionBytes(path.join(packageRoot, "package.json"), budget, 64 * 1024));
    if (metadata.name !== dependency.install.package || metadata.version !== dependency.version) {
      return { status: "failed", evidence: `Expected pinned ${dependency.install.package}@${dependency.version} companion package.` };
    }
    const sourceFiles = await skillContents(source, budget);
    const complete = await verifySkillFiles(companion, { ...paths, skillRoot: path.dirname(source) }, { ignoreFencedExamples: true, budget });
    if (complete.status !== "passed") return complete;
    if (preflight.status === "passed") {
      try {
        if (JSON.stringify(sourceFiles) !== JSON.stringify(await skillContents(destination, budget))) {
          return { status: "customized", evidence: `Preserved edited or incomplete managed companion: ${destination}` };
        }
      } catch (error) {
        if (error.code === "EVENT_DEADLINE") throw error;
        budget?.check();
        return { status: "customized", evidence: error.message };
      }
    } else {
      budget?.check();
      await mkdir(paths.skillRoot, { recursive: true, mode: 0o700 });
      budget?.check();
      await mkdir(destination, { mode: 0o700 });
      for (const [relative, hash] of Object.entries(sourceFiles)) {
        const bytes = await companionBytes(path.join(source, relative), budget);
        if (createHash("sha256").update(bytes).digest("hex") !== hash) throw new Error("Companion source changed during preparation.");
        budget?.check();
        const target = path.join(destination, relative);
        await mkdir(path.dirname(target), { recursive: true, mode: 0o700 });
        budget?.check();
        await writeFile(target, bytes, { mode: 0o600, flag: "wx", signal: budget?.signal });
      }
      budget?.check();
      await writeFile(path.join(destination, ".agent-team-source.json"), `${JSON.stringify({
        source: companion.install.source, revision: companion.version, selectedPath: companion.install.paths[0],
      }, null, 2)}\n`, { mode: 0o600, flag: "wx", signal: budget?.signal });
    }
    budget?.check();
    return { status: "passed", evidence: "Complete pinned Playwright companion is present in the selected skill scope." };
  } catch (error) {
    if (error.code === "EVENT_DEADLINE") throw error;
    budget?.check();
    return { status: "failed", evidence: error.message };
  }
}

function uvAsset() {
  const architecture = process.arch === "x64" ? "x86_64" : process.arch === "arm64" ? "aarch64" : null;
  const platform = process.platform === "linux" ? "unknown-linux-gnu" : process.platform === "darwin" ? "apple-darwin" : null;
  return architecture && platform ? `uv-${architecture}-${platform}.tar.gz` : null;
}

async function download(url, file) {
  const response = await fetch(url, { redirect: "follow" });
  if (!response.ok) throw new Error(`Download failed (${response.status}) for ${url}`);
  await writeFile(file, Buffer.from(await response.arrayBuffer()), { mode: 0o600 });
}

async function installUvRelease(dependency, paths) {
  const asset = uvAsset();
  if (!asset) return { status: "failed", evidence: `No verified uv release asset is configured for ${process.platform}/${process.arch}.` };
  const stage = path.join(paths.toolRoot, "downloads", `uv-${dependency.version}`);
  const archive = path.join(stage, asset);
  const checksumFile = `${archive}.sha256`;
  const base = `https://github.com/astral-sh/uv/releases/download/${dependency.version}`;
  try {
    await mkdir(stage, { recursive: true, mode: 0o700 });
    await download(`${base}/${asset}`, archive);
    await download(`${base}/${asset}.sha256`, checksumFile);
    const expected = (await readFile(checksumFile, "utf8")).trim().split(/\s+/)[0];
    const actual = createHash("sha256").update(await readFile(archive)).digest("hex");
    if (!expected || expected !== actual) return { status: "failed", evidence: `Checksum mismatch for ${asset}.` };
    const extracted = await command("tar", ["-xzf", archive, "-C", stage]);
    if (extracted.status !== "passed") return extracted;
    const source = path.join(stage, asset.replace(/\.tar\.gz$/, ""), "uv");
    const destination = path.join(paths.toolRoot, "bin", "uv");
    await mkdir(path.dirname(destination), { recursive: true, mode: 0o700 });
    await cp(source, destination, { errorOnExist: true, force: false });
    await chmod(destination, 0o755);
    return { status: "passed", version: dependency.version, evidence: `Verified ${asset} with its pinned release checksum.` };
  } catch (error) {
    return { status: "failed", evidence: error.message };
  }
}

function leanCtxAsset() {
  const architecture = { x64: "x86_64", arm64: "aarch64" }[process.arch];
  if (!architecture) return null;
  if (process.platform === "darwin") return `lean-ctx-${architecture}-apple-darwin.tar.gz`;
  if (process.platform === "win32") return process.arch === "x64" ? "lean-ctx-x86_64-pc-windows-msvc.zip" : null;
  if (process.platform !== "linux") return null;
  const [major, minor] = (process.report?.getReport().header.glibcVersionRuntime ?? "0.0").split(".").map(Number);
  const libc = major > 2 || major === 2 && minor >= 35 ? "gnu" : "musl";
  return `lean-ctx-${architecture}-unknown-linux-${libc}.tar.gz`;
}

async function releaseBytes(url, limit, budget) {
  budget?.check();
  const response = await fetch(url, { signal: budget?.signal ?? AbortSignal.timeout(120_000), redirect: "follow" });
  if (!response.ok) throw new Error(`Download failed (${response.status}) for ${url}`);
  const chunks = [];
  let size = 0;
  for await (const chunk of response.body) {
    budget?.check();
    size += chunk.length;
    if (size > limit) throw new Error(`Release byte limit exceeded for ${url}`);
    chunks.push(chunk);
  }
  return Buffer.concat(chunks);
}

async function installLeanCtxRelease(dependency, paths, budget) {
  const asset = leanCtxAsset();
  if (!asset) return { status: "failed", evidence: `No verified LeanCTX asset for ${process.platform}/${process.arch}.` };
  const destination = binaryPath(dependency, paths.toolRoot);
  const directory = path.dirname(destination);
  let stage;
  try {
    try {
      await lstat(directory);
      return { status: "customized", evidence: `Preserved existing LeanCTX release directory: ${directory}` };
    } catch (error) { if (error.code !== "ENOENT") throw error; }
    await mkdir(paths.toolRoot, { recursive: true });
    stage = await mkdtemp(path.join(paths.toolRoot, ".lean-ctx-download-"));
    const archive = await releaseBytes(`${dependency.install.source}/${asset}`, 128 * 1024 * 1024, budget);
    const checksums = (await releaseBytes(dependency.install.checksums, 64 * 1024, budget)).toString("utf8");
    const matching = checksums.split(/\r?\n/).map((line) => line.match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/))
      .filter((match) => match?.[2] === asset);
    const digest = createHash("sha256").update(archive).digest("hex");
    if (matching.length !== 1 || matching[0][1].toLowerCase() !== digest) throw new Error(`LeanCTX checksum mismatch for ${asset}.`);
    const archivePath = path.join(stage, asset);
    await writeFile(archivePath, archive, { mode: 0o600, flag: "wx" });
    const name = path.basename(destination);
    const extracted = await command("tar", ["-xf", archivePath, "-C", stage, name], { budget });
    if (extracted.status !== "passed") return extracted;
    const source = path.join(stage, name);
    await companionBytes(source, budget, 128 * 1024 * 1024); // Reject symlinks/special entries before publication.
    await chmod(source, 0o755);
    budget?.check();
    await mkdir(directory, { mode: 0o700 }); // Exclusive: preserve a directory created during download.
    await link(source, destination); // Atomic and never follows or replaces a destination link.
    return { status: "passed", version: dependency.version, evidence: `Verified pinned ${asset} SHA256 ${digest}; no npm lifecycle or initializer executed.` };
  } catch (error) {
    if (error.code === "EVENT_DEADLINE") throw error;
    return { status: "failed", evidence: error.message };
  } finally { if (stage) await rm(stage, { recursive: true, force: true }); }
}

async function installBeadsViewerRelease(dependency, paths, budget) {
  const platform = { linux: "linux", darwin: "darwin" }[process.platform];
  const architecture = { x64: "amd64", arm64: "arm64" }[process.arch];
  if (!platform || !architecture) {
    return { status: "failed", evidence: `No verified bv asset for ${process.platform}/${process.arch}.` };
  }
  const asset = `bv_${dependency.version}_${platform}_${architecture}.tar.gz`;
  const directory = path.dirname(binaryPath(dependency, paths.toolRoot));
  let stage;
  let directoryCreated = false;
  const published = [];
  try {
    try {
      await lstat(directory);
      return { status: "customized", evidence: `Preserved existing bv release directory: ${directory}` };
    } catch (error) { if (error.code !== "ENOENT") throw error; }
    await mkdir(paths.toolRoot, { recursive: true, mode: 0o700 });
    stage = await mkdtemp(path.join(paths.toolRoot, ".bv-download-"));
    const url = `${dependency.install.source}/${asset}`;
    const archive = await releaseBytes(url, 128 * 1024 * 1024, budget);
    const checksums = (await releaseBytes(`${url}.sha256`, 64 * 1024, budget)).toString("utf8");
    const matching = checksums.split(/\r?\n/).map((line) => line.match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/)).filter((match) => match?.[2] === asset);
    const digest = createHash("sha256").update(archive).digest("hex");
    if (matching.length !== 1 || matching[0][1].toLowerCase() !== digest) throw new Error(`bv checksum mismatch for ${asset}.`);
    const archivePath = path.join(stage, asset);
    await writeFile(archivePath, archive, { flag: "wx", mode: 0o600 });
    const name = path.basename(binaryPath(dependency, paths.toolRoot));
    const extracted = await command("tar", ["-xf", archivePath, "-C", stage, name, "LICENSE"], { budget });
    if (extracted.status !== "passed") return extracted;
    await companionBytes(path.join(stage, name), budget, 128 * 1024 * 1024);
    await companionBytes(path.join(stage, "LICENSE"), budget, 64 * 1024);
    await chmod(path.join(stage, name), 0o755);
    budget?.check();
    await mkdir(directory, { mode: 0o700 });
    directoryCreated = true;
    for (const member of ["LICENSE", name]) {
      await link(path.join(stage, member), path.join(directory, member));
      published.push(member);
    }
    return { status: "passed", version: dependency.version, evidence: `Verified pinned ${asset} SHA256 ${digest}; complete LICENSE retained. No source build, updater or initializer executed.` };
  } catch (error) {
    if (directoryCreated) {
      for (const member of published) {
        try {
          const [source, target] = await Promise.all([lstat(path.join(stage, member)), lstat(path.join(directory, member))]);
          if (source.dev === target.dev && source.ino === target.ino) await unlink(path.join(directory, member));
        } catch { /* Preserve files no longer identifiable as this invocation's links. */ }
      }
      await rmdir(directory).catch(() => {}); // Remove only an empty directory we created, never unrelated contents.
    }
    if (error.code === "EVENT_DEADLINE") throw error;
    return { status: "failed", evidence: error.message };
  } finally { if (stage) await rm(stage, { recursive: true, force: true }); }
}

async function prepareLeanCtxSkill(dependency, paths, budget) {
  const destination = path.join(paths.skillRoot, "lean-ctx");
  const provenance = { source: dependency.install.skill.source, revision: dependency.version, gitBlob: dependency.install.skill.gitBlob };
  try {
    try {
      const files = await skillContents(destination, budget);
      const metadata = JSON.parse(await companionBytes(path.join(destination, ".agent-team-source.json"), budget, 64 * 1024));
      const bytes = await companionBytes(path.join(destination, "SKILL.md"), budget, 64 * 1024);
      const blob = createHash("sha1").update(`blob ${bytes.length}\0`).update(bytes).digest("hex");
      return Object.keys(files).length === 1 && files["SKILL.md"] && blob === provenance.gitBlob &&
        Object.entries(provenance).every(([key, value]) => metadata[key] === value)
        ? { status: "passed", evidence: "Reused complete pinned LeanCTX skill." }
        : { status: "customized", evidence: `Preserved customized LeanCTX skill: ${destination}` };
    } catch (error) {
      if (error.code === "EVENT_DEADLINE") throw error;
      try { await lstat(destination); return { status: "customized", evidence: `Preserved existing LeanCTX skill: ${destination}` }; }
      catch (missing) { if (missing.code !== "ENOENT") throw missing; }
    }
    const bytes = await releaseBytes(provenance.source, 64 * 1024, budget);
    const blob = createHash("sha1").update(`blob ${bytes.length}\0`).update(bytes).digest("hex");
    if (blob !== provenance.gitBlob) throw new Error("Pinned LeanCTX skill blob mismatch.");
    budget?.check();
    await mkdir(paths.skillRoot, { recursive: true });
    await mkdir(destination, { mode: 0o700 });
    budget?.check();
    await writeFile(path.join(destination, "SKILL.md"), bytes, { mode: 0o600, flag: "wx", signal: budget?.signal });
    budget?.check();
    await writeFile(path.join(destination, ".agent-team-source.json"), `${JSON.stringify(provenance)}\n`, { mode: 0o600, flag: "wx", signal: budget?.signal });
    return { status: "passed", evidence: "Prepared complete pinned LeanCTX host skill; Agent-Team narrowed profile governs its use." };
  } catch (error) {
    if (error.code === "EVENT_DEADLINE") throw error;
    return { status: "failed", evidence: error.message };
  }
}

async function verifySkillFiles(dependency, paths, { ignoreFencedExamples = false, budget } = {}) {
  for (const selectedPath of dependency.install.paths) {
    const root = skillDestination(dependency, paths, selectedPath);
    const skill = path.join(root, "SKILL.md");
    if (!await exists(skill)) return { status: "failed", evidence: `Missing complete skill entry: ${skill}` };
    for (const includedPath of dependency.install.includePaths ?? []) {
      if (!await exists(path.join(root, includedPath))) return { status: "failed", evidence: `Missing approved package path: ${includedPath}` };
    }
    const text = ignoreFencedExamples ? (await companionBytes(skill, budget)).toString("utf8") : await readFile(skill, "utf8");
    const source = ignoreFencedExamples ? text.replace(/^```[^\n]*\n[\s\S]*?^```[^\n]*$/gm, "") : text;
    for (const match of source.matchAll(/\]\(([^)#]+)(?:#[^)]+)?\)/g)) {
      budget?.check();
      const reference = match[1];
      if (/^[a-z]+:/i.test(reference) || reference.startsWith("/")) continue;
      if (!await exists(path.resolve(root, reference))) return { status: "failed", evidence: `Missing referenced skill file: ${reference}` };
    }
  }
  return { status: "passed", evidence: "Selected skill entries and their local references are present." };
}

async function astGrepFunctional(executable, paths) {
  const root = path.join(paths.toolRoot, "verification", `ast-grep-${randomUUID()}`);
  const fixture = path.join(root, "fixture.js");
  await mkdir(root, { recursive: true, mode: 0o700 });
  await writeFile(fixture, "const total = left + right;\n", { mode: 0o600 });
  const positive = await command(executable, ["--pattern", "$A + $B", "--lang", "js", fixture]);
  if (positive.status !== "passed" || !positive.stdout?.includes("left + right")) return { status: "failed", evidence: "Positive structural fixture did not match." };
  const negative = await command(executable, ["--pattern", "console.log($A)", "--lang", "js", fixture]);
  return negative.code === 1
    ? { status: "passed", evidence: "Positive and negative structural fixtures passed." }
    : { status: "failed", evidence: "Negative structural fixture unexpectedly matched or failed to execute." };
}

// Code-only extraction stays deterministic and offline only while no backend key reaches the CLI.
const GRAPHIFY_BACKEND_KEYS = [
  "ANTHROPIC_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENAI_API_KEY",
  "MOONSHOT_API_KEY", "DEEPSEEK_API_KEY", "OLLAMA_BASE_URL",
];

// Exact labels the pinned extractor emits for the fixture; methods and functions carry their parentheses.
const GRAPHIFY_FIXTURE_CHAIN = ["Pool", ".connect()", "start_server()", "load_config()"];
const GRAPHIFY_FIXTURE_EDGES = [["Pool", ".connect()", "method"], [".connect()", "start_server()", "calls"], ["start_server()", "load_config()", "calls"]];

async function graphifyFunctional(executable, paths, budget) {
  const verification = path.join(paths.toolRoot, "verification");
  budget?.check();
  await mkdir(verification, { recursive: true, mode: 0o700 });
  const root = await mkdtemp(path.join(verification, "graphify-"));
  const env = { ...process.env };
  for (const key of GRAPHIFY_BACKEND_KEYS) delete env[key];
  const options = { cwd: root, env, budget };
  try {
    await writeFile(path.join(root, "app.py"), "def load_config():\n    return {}\n\n\ndef start_server():\n    cfg = load_config()\n    return cfg\n", { mode: 0o600, signal: budget?.signal });
    await writeFile(path.join(root, "db.py"), "from app import start_server\n\n\nclass Pool:\n    def connect(self):\n        return start_server()\n", { mode: 0o600, signal: budget?.signal });
    const extracted = await command(executable, ["extract", ".", "--code-only", "--no-viz"], options);
    if (extracted.status !== "passed") return { status: "failed", evidence: evidence(extracted) ?? "Graphify code-only extraction failed." };
    let graph;
    try {
      graph = JSON.parse(await readFile(path.join(root, "graphify-out", "graph.json"), { encoding: "utf8", signal: budget?.signal }));
    } catch (error) {
      if (error.code === "EVENT_DEADLINE") throw error;
      return { status: "failed", evidence: `Graphify did not write a parseable graphify-out/graph.json: ${error.message}` };
    }
    budget?.check();
    // Code-only extraction is pure AST work, so any inferred edge means a semantic backend ran.
    const inferred = (graph.links ?? []).find(({ confidence }) => confidence !== "EXTRACTED");
    if (inferred) {
      return { status: "failed", evidence: `Graphify emitted an edge with ${inferred.confidence ?? "missing"} confidence; code-only extraction must produce EXTRACTED edges only.` };
    }
    const ids = new Map((graph.nodes ?? []).filter(({ label }) => GRAPHIFY_FIXTURE_CHAIN.includes(label)).map(({ label, id }) => [label, id]));
    const missingLabel = GRAPHIFY_FIXTURE_CHAIN.find((label) => !ids.has(label));
    if (missingLabel) return { status: "failed", evidence: `The extracted graph did not contain the known fixture symbol ${missingLabel}.` };
    const missingEdge = GRAPHIFY_FIXTURE_EDGES.find(([from, to, relation]) => !(graph.links ?? [])
      .some((link) => link.source === ids.get(from) && link.target === ids.get(to) && link.relation === relation));
    if (missingEdge) {
      return { status: "failed", evidence: `The extracted graph did not contain the known fixture edge ${missingEdge[0]} --${missingEdge[2]}--> ${missingEdge[1]}.` };
    }
    const traced = await command(executable, ["path", "Pool", "load_config"], options);
    const offsets = GRAPHIFY_FIXTURE_CHAIN.map((label) => traced.stdout?.indexOf(label) ?? -1);
    if (traced.status !== "passed" || offsets.some((offset, index) => offset < 0 || (index > 0 && offset <= offsets[index - 1]))) {
      return { status: "failed", evidence: evidence(traced) ?? `Graphify path did not report the chain ${GRAPHIFY_FIXTURE_CHAIN.join(" -> ")}.` };
    }
    const explained = await command(executable, ["explain", "start_server"], options);
    return explained.status === "passed" && ["start_server()", "load_config()"].every((label) => explained.stdout?.includes(label))
      ? { status: "passed", evidence: "Graphify code-only extraction built an EXTRACTED-only graph and resolved path Pool -> load_config and explain start_server in an isolated fixture." }
      : { status: "failed", evidence: evidence(explained) ?? "Graphify explain did not report the known fixture symbol and its call edge." };
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}

async function leanCtxFunctional(executable, paths) {
  const root = path.join(paths.toolRoot, "verification", `lean-ctx-${randomUUID()}`);
  const fixture = path.join(root, "fixture.txt");
  await mkdir(root, { recursive: true, mode: 0o700 });
  await writeFile(fixture, "readinessLeanCtxMarker: exact source remains recoverable.\n", { mode: 0o600 });
  const env = { ...process.env };
  delete env.LEAN_CTX_DISABLED;
  delete env.LEAN_CTX_RAW;
  env.XDG_CONFIG_HOME = path.join(root, "xdg-config");
  env.XDG_DATA_HOME = path.join(root, "xdg-data");
  env.XDG_STATE_HOME = path.join(root, "xdg-state");
  env.XDG_CACHE_HOME = path.join(root, "xdg-cache");
  env.XDG_RUNTIME_DIR = path.join(root, "xdg-runtime");
  // Explicit upstream pins take precedence over XDG and legacy layout detection.
  for (const kind of ["CONFIG", "DATA", "STATE", "CACHE"]) {
    env[`LEAN_CTX_${kind}_DIR`] = path.join(env[`XDG_${kind}_HOME`], "lean-ctx");
  }
  try {
    const read = await command(executable, ["read", fixture], { cwd: root, env });
    return read.status === "passed" && read.stdout?.includes("readinessLeanCtxMarker")
      ? { status: "passed", evidence: "LeanCTX narrow read recovered readinessLeanCtxMarker from an isolated source." }
      : { status: "failed", evidence: evidence(read) ?? "LeanCTX narrow read did not return the known source marker." };
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}

async function impeccableFunctional(executable, paths) {
  const root = path.join(paths.toolRoot, "verification", `impeccable-${randomUUID()}`);
  const clean = path.join(root, "clean.css");
  const finding = path.join(root, "finding.css");
  const missing = path.join(root, "missing.css");
  await mkdir(root, { recursive: true, mode: 0o700 });
  await writeFile(clean, "body { color: #111; background: #fff; }\n", { mode: 0o600 });
  await writeFile(finding, ".control { transition: all 300ms cubic-bezier(.68,-.55,.27,1.55); }\n", { mode: 0o600 });
  const args = ["detect", "--no-config", "--no-advisory"];
  const options = { cwd: root, env: { ...process.env, IMPECCABLE_HOME: path.join(paths.toolRoot, "impeccable-home") } };
  try {
    const cleanResult = await command(executable, [...args, clean], options);
    const findingResult = await command(executable, [...args, finding], options);
    const failureResult = await command(executable, [...args, missing], options);
    return cleanResult.code === 0 && findingResult.code === 2 && failureResult.code === 1
      ? { status: "passed", evidence: "Impeccable detector produced the documented clean 0, findings 2, and operational failure 1 exits." }
      : {
          status: "failed",
          evidence: `Impeccable detector exits were clean=${cleanResult.code}, findings=${findingResult.code}, failure=${failureResult.code}; expected 0, 2, 1.`,
        };
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}

async function beadsFunctional(executable, paths) {
  const root = path.join(paths.toolRoot, "verification", `beads-${randomUUID()}`);
  const project = { root, tracker: { kind: "beads", path: path.join(root, ".beads"), executable } };
  const options = {
    cwd: root,
    env: { ...beadsEnvironment(project), BD_NON_INTERACTIVE: "1", BEADS_ACTOR: "agent-team-readiness" },
  };
  await mkdir(root, { recursive: true, mode: 0o700 });
  try {
    const git = await command("git", ["init", "--quiet"], options);
    if (git.status !== "passed") return git;
    const initialized = await command(executable, ["init", "--non-interactive", "--skip-agents", "--skip-hooks", "--prefix", "ATV"], options);
    if (initialized.status !== "passed") return initialized;
    const created = await Promise.all([
      command(executable, ["create", "--title", "writer-a", "--type", "task", "--priority", "2", "--json"], options),
      command(executable, ["create", "--title", "writer-b", "--type", "task", "--priority", "2", "--json"], options),
    ]);
    if (created.some(({ status }) => status !== "passed")) {
      return { status: "failed", evidence: `Beads concurrent writes did not both complete: ${created.map(({ code }) => code).join(", ")}.` };
    }
    const exported = await command(executable, ["export"], options);
    return exported.status === "passed" && exported.stdout?.includes("writer-a") && exported.stdout?.includes("writer-b")
      ? { status: "passed", evidence: "Beads completed two concurrent isolated writes and exported both records." }
      : { status: "failed", evidence: evidence(exported) ?? "Beads export did not contain both concurrent writes." };
  } finally {
    await rm(root, { recursive: true, force: true });
  }
}

async function beadsViewerFunctional(executable, trackerExecutable, paths, budget) {
  const root = path.join(paths.toolRoot, "verification", `bv-${randomUUID()}`);
  const tracker = { kind: "beads", id: "bv-readiness", path: path.join(root, ".beads"), executable: trackerExecutable };
  const options = { cwd: root, budget, env: { ...beadsEnvironment({ root, tracker }), BD_NON_INTERACTIVE: "1", BEADS_ACTOR: "agent-team-readiness" } };
  await mkdir(root, { recursive: true, mode: 0o700 });
  try {
    for (const [file, args] of [
      ["git", ["init", "--quiet"]],
      [trackerExecutable, ["init", "--non-interactive", "--skip-agents", "--skip-hooks", "--prefix", "ATV"]],
    ]) {
      const result = await command(file, args, options);
      if (result.status !== "passed") return result;
    }
    const ids = [];
    for (const title of ["graph-prerequisite", "graph-dependent"]) {
      const created = await command(trackerExecutable, ["create", "--title", title, "--type", "task", "--priority", "2", "--json"], options);
      if (created.status !== "passed") return created;
      const id = JSON.parse(created.stdout).id;
      if (typeof id !== "string" || !id) throw new Error("Beads did not return a graph fixture ID.");
      ids.push(id);
    }
    const linked = await command(trackerExecutable, ["dep", "add", ids[1], ids[0]], options);
    if (linked.status !== "passed") return linked;
    const result = await createBeadsGraphCommandAdapter({ projectRoot: root, tracker, bvPath: executable, selected: true, termsAcknowledged: true, budget }).refresh();
    const adjacency = result.graph?.adjacency;
    if (result.status !== "available" || !ids.every((id) => adjacency.nodes.some((node) => node.id === id))
      || !adjacency.edges.some((edge) => edge.from === ids[1] && edge.to === ids[0])) {
      return { status: "failed", evidence: result.reason ?? "Fresh exported graph did not preserve both tasks and their dependency." };
    }
    return { status: "passed", evidence: "Selected Beads exported two isolated tasks and their dependency; bv returned the matching attributed JSON graph with hooks disabled." };
  } finally { await rm(root, { recursive: true, force: true }); }
}

async function serenaFunctional(executable, paths) {
  const root = path.join(paths.toolRoot, "verification", `serena-${randomUUID()}`);
  const fixture = path.join(root, "fixture.js");
  await mkdir(root, { recursive: true, mode: 0o700 });
  await writeFile(fixture, "export function readinessFixtureSymbol() { return true; }\n", { mode: 0o600 });

  const environment = { ...process.env, SERENA_HOME: path.join(root, "serena-home"), NO_COLOR: "1", PWD: root };
  for (const key of ["GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"]) delete environment[key];
  const child = spawn(executable, ["start-mcp-server", "--context", "ide", "--project", root], {
    cwd: root,
    env: environment,
    stdio: ["pipe", "pipe", "pipe"],
  });
  const pending = new Map();
  let buffer = "";
  let stderr = "";
  let processError = null;
  const rejectPending = (error) => {
    for (const waiter of pending.values()) waiter.reject(error);
    pending.clear();
  };
  child.on("error", (error) => {
    processError = error;
    rejectPending(error);
  });
  child.on("exit", (code, signal) => {
    if (pending.size) rejectPending(new Error(`Serena exited before completing MCP verification (${code ?? signal ?? "unknown"}).`));
  });
  child.stdin.on("error", () => {});
  child.stderr.setEncoding("utf8");
  child.stderr.on("data", (chunk) => { stderr = `${stderr}${chunk}`.slice(-8192); });
  child.stdout.setEncoding("utf8");
  child.stdout.on("data", (chunk) => {
    buffer += chunk;
    for (;;) {
      const newline = buffer.indexOf("\n");
      if (newline < 0) break;
      const line = buffer.slice(0, newline).trim();
      buffer = buffer.slice(newline + 1);
      if (!line) continue;
      try {
        const message = JSON.parse(line);
        const waiter = pending.get(message.id);
        if (!waiter) continue;
        pending.delete(message.id);
        if (message.error) waiter.reject(new Error(message.error.message ?? "MCP request failed."));
        else waiter.resolve(message.result);
      } catch {
        // Serena may log to stdout during startup; only JSON-RPC response lines matter here.
      }
    }
  });

  const request = (id, method, params = {}) => new Promise((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(id);
      reject(new Error(`Timed out waiting for Serena ${method}.`));
    }, 30_000);
    pending.set(id, {
      resolve: (value) => { clearTimeout(timer); resolve(value); },
      reject: (error) => { clearTimeout(timer); reject(error); },
    });
    child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", id, method, params })}\n`, (error) => {
      if (error && pending.has(id)) {
        pending.delete(id);
        clearTimeout(timer);
        reject(error);
      }
    });
  });

  try {
    const initialized = await request(1, "initialize", {
      protocolVersion: "2025-06-18",
      capabilities: {},
      clientInfo: { name: "agent-team-readiness", version: "1" },
    });
    if (!initialized?.capabilities) throw new Error("Serena did not complete MCP initialization.");
    child.stdin.write(`${JSON.stringify({ jsonrpc: "2.0", method: "notifications/initialized", params: {} })}\n`);
    const listed = await request(2, "tools/list");
    if (!listed?.tools?.some(({ name }) => name === "find_symbol")) throw new Error("Serena did not expose find_symbol.");
    const called = await request(3, "tools/call", {
      name: "find_symbol",
      arguments: { name_path_pattern: "readinessFixtureSymbol", relative_path: "fixture.js", include_body: false },
    });
    const content = called?.content?.map(({ text }) => text ?? "").join("\n") ?? "";
    if (!content.includes("readinessFixtureSymbol")) throw new Error("Serena find_symbol returned no matching symbol.");
    return { status: "passed", evidence: "Serena find_symbol located readinessFixtureSymbol in an isolated project." };
  } catch (error) {
    const detail = processError?.message ?? error.message;
    return { status: "failed", evidence: `${detail}${stderr ? ` Serena stderr: ${stderr.trim()}` : ""}` };
  } finally {
    child.stdin.end();
    const waitForExit = (milliseconds) => new Promise((resolve) => {
      if (child.exitCode !== null || child.signalCode !== null) return resolve(true);
      const finished = () => { clearTimeout(timer); resolve(true); };
      const timer = setTimeout(() => { child.off("exit", finished); resolve(false); }, milliseconds);
      child.once("exit", finished);
    });
    if (child.exitCode === null && child.signalCode === null) child.kill("SIGTERM");
    if (!await waitForExit(2_000) && child.exitCode === null && child.signalCode === null) {
      child.kill("SIGKILL");
      await waitForExit(2_000);
    }
    await rm(root, { recursive: true, force: true });
  }
}

async function playwrightFunctional(executable, paths, budget) {
  const eventBudget = budget ?? createEventBudget(120_000);
  eventBudget.check();
  const remaining = eventBudget.remaining();
  const reserve = Math.min(1000, Math.max(250, Math.floor(remaining / 3)));
  if (remaining < reserve + 100) {
    if (!budget) eventBudget.close();
    return { status: "failed", evidence: "Insufficient event time for a Playwright action and owned-session cleanup reserve; no session was opened." };
  }
  const actionClock = createEventBudget(remaining - reserve);
  const actionBudget = {
    ...actionClock,
    signal: AbortSignal.any([eventBudget.signal, actionClock.signal]),
    check() { eventBudget.check(); actionClock.check(); },
  };
  const session = `agent-team-${randomUUID()}`;
  const url = "data:text/html,<button%20id='activate'%20onclick=\"document.body.dataset.ready='yes'\">Activate</button>";
  const options = { cwd: paths.projectRoot, env: { ...process.env, PLAYWRIGHT_BROWSERS_PATH: path.join(paths.toolRoot, "playwright-browsers"), NO_UPDATE_NOTIFIER: "1" } };
  const execute = (args) => command(executable, [`-s=${session}`, ...args], { ...options, budget: actionBudget });
  let outcome;
  let actionError;
  let cleanup;
  try {
    // Even an interrupted open may have created a persistent session daemon.
    outcome = await execute(["open", url]);
    if (outcome.status === "passed") outcome = await execute(["click", "#activate"]);
    if (outcome.status === "passed") {
      const checked = await execute(["eval", "document.body.dataset.ready"]);
      outcome = checked.status === "passed" && checked.stdout?.includes("yes")
        ? { status: "passed", evidence: "Launched an isolated browser, clicked the fixture, and observed its state change." }
        : { status: "failed", evidence: "Browser interaction did not produce the expected page state." };
    }
  } catch (error) {
    actionError = error;
  } finally {
    actionClock.close();
    // Cancellation stops actions, but the reserved time remains available only
    // for closing this exact owned session, never for global browser cleanup.
    const cleanupClock = createEventBudget(Math.min(reserve, eventBudget.remaining()));
    try {
      const closed = await command(executable, [`-s=${session}`, "close"], { ...options, budget: cleanupClock });
      cleanup = {
        status: closed.status === "passed" ? "close_command_passed" : "unverified", session,
        evidence: closed.status === "passed" ? "Exact-session close command succeeded." : evidence(closed),
      };
    } catch (error) {
      cleanup = { status: "unverified", session, evidence: error.message };
    } finally {
      cleanupClock.close();
      if (!budget) eventBudget.close();
    }
  }
  try { eventBudget.check(); } catch (error) { actionError ??= error; }
  const cleanupMessage = `Owned Playwright session ${session} cleanup: ${cleanup.status}. ${cleanup.evidence ?? "Session termination was not verified."}`;
  if (actionError) {
    actionError.cleanup = cleanup;
    actionError.message = `${actionError.message} ${cleanupMessage}`;
    throw actionError;
  }
  return { ...outcome, status: cleanup.status === "unverified" ? "failed" : outcome.status, cleanup, evidence: `${evidence(outcome) ?? ""} ${cleanupMessage}`.trim() };
}

/** Execute pinned installers in candidate-managed paths. Callers may replace only external probes in tests. */
export function createDependencyRunner({ host, scope, paths, functionalAdapters = {}, workerDiscovery, budget: eventBudget }) {
  if (!paths?.toolRoot || !paths?.skillRoot || !paths?.projectRoot) throw new Error("Dependency preparation paths are required.");
  return async ({ dependency, phase, check, budget = eventBudget }) => {
    budget?.check();
    const execute = (file, args, options = {}) => command(file, args, { ...options, budget });
    const executable = binaryPath(dependency, paths.toolRoot);
    if (phase === "probe") {
      if (dependency.id === "playwright-cli") {
        const companion = await inspectSkillDestinations(playwrightSkill(dependency), paths, { budget, bounded: true });
        if (companion.status === "customized") return companion;
      }
      if (dependency.install?.kind === "git-skill") {
        if (!dependency.install.paths.length) return { status: "not_found" };
        return inspectSkillDestinations(dependency, paths);
      }
      const result = await execute(executable, ["--version"], dependency.id === "playwright-cli"
        ? { env: { ...process.env, NO_UPDATE_NOTIFIER: "1" } } : {});
      return result.status === "passed" ? { ...result, version: parsedVersion(`${result.stdout}\n${result.stderr}`, dependency.id) } : result;
    }
    if (phase === "install") {
      await mkdir(paths.toolRoot, { recursive: true, mode: 0o700 });
      if (dependency.install?.kind === "npm") {
        for (const step of buildPreparationPlan({ dependencyId: dependency.id, host, scope, paths, executable: dependency.executable }).install) {
          const result = await execute(step.file, step.args, { cwd: step.cwd, env: step.env ? { ...process.env, ...step.env } : process.env });
          if (result.status !== "passed") return result;
        }
        return { status: "passed", version: dependency.version, evidence: `Installed pinned ${dependency.install.package}@${dependency.version}.` };
      }
      if (dependency.install?.kind === "uv-tool") {
        const uv = binaryPath(CATALOG_BY_ID.get("uv"), paths.toolRoot);
        return execute(uv, ["tool", "install", "--python", dependency.install.python, `${dependency.install.package}==${dependency.version}`], {
          env: { ...process.env, UV_TOOL_DIR: path.join(paths.toolRoot, "uv-tools"), UV_TOOL_BIN_DIR: path.join(paths.toolRoot, "bin") },
        });
      }
      if (dependency.install?.kind === "git-skill") return installGitSkills(dependency, paths);
      if (dependency.id === "uv" && dependency.install?.kind === "github-release") return installUvRelease(dependency, paths);
      if (dependency.id === "lean-ctx" && dependency.install?.kind === "github-release") return installLeanCtxRelease(dependency, paths, budget);
      if (dependency.id === "beads-viewer" && dependency.install?.kind === "github-release") return installBeadsViewerRelease(dependency, paths, budget);
      return { status: "failed", evidence: `Pinned ${dependency.install?.kind ?? "unknown"} preparation requires its verified installer adapter.` };
    }
    if (phase === "companion" && dependency.id === "lean-ctx") return prepareLeanCtxSkill(dependency, paths, budget);
    if (phase === "companion" && dependency.id === "playwright-cli") {
      const companion = await preparePlaywrightSkill(dependency, paths, budget);
      if (companion.status !== "passed") return companion;
      const step = buildPreparationPlan({ dependencyId: dependency.id, host, scope, paths, executable }).browserInstall;
      const browser = await execute(step.file, step.args, { cwd: step.cwd, env: { ...process.env, ...step.env } });
      return browser.status === "passed" ? companion : browser;
    }
    if (phase === "functional") {
      if (check === "command") return execute(executable, ["--version"]);
      if (functionalAdapters[dependency.id]) return functionalAdapters[dependency.id]({ dependency, executable, host, scope, paths, command: execute });
      if (check === "complete-selective-skills") return verifySkillFiles(dependency, paths);
      if (check === "skill-discovery") return verifySkillFiles(dependency, paths);
      if (check === "symbol-operation") return serenaFunctional(executable, paths);
      if (check === "positive-negative-structural-pattern") return astGrepFunctional(executable, paths);
      if (check === "code-graph-traversal") return graphifyFunctional(executable, paths, budget);
      if (check === "browser-interaction") return playwrightFunctional(executable, paths, budget);
      if (check === "narrow-read-recovery") return leanCtxFunctional(executable, paths);
      if (check === "detector-exit-contract") return impeccableFunctional(executable, paths);
      if (check === "atomic-tracker-write") return beadsFunctional(executable, paths);
      if (check === "fresh-export-graph") return beadsViewerFunctional(executable, dependency.trackerExecutable ?? binaryPath(CATALOG_BY_ID.get("beads"), paths.toolRoot), paths, budget);
      return { status: "failed", evidence: `Functional adapter '${check}' did not run.` };
    }
    if (phase === "worker") {
      if (workerDiscovery) return workerDiscovery({ dependency, executable, host, scope, paths });
      return { status: "unverified", evidence: `Fresh ${host} worker discovery was not exercised for ${scope} scope.` };
    }
    return { status: "failed", evidence: `Unknown preparation phase: ${phase}.` };
  };
}

export function selectInstructions({ role, task = {} }) {
  const result = [];
  if (["developer", "complex_developer", "routine_developer"].includes(role)) {
    result.push("superpowers:test-driven-development", "superpowers:systematic-debugging", "ponytail");
  }
  if (["reviewer", "visual_reviewer"].includes(role)) result.push("superpowers:verification-before-completion");
  if (task.kind === "ui" || task.kind === "react" || role === "visual_reviewer") result.push("impeccable");
  if (task.kind === "react") result.push("react-best-practices");
  if (["ui", "react", "browser"].includes(task.kind) || role === "visual_reviewer") result.push("playwright-cli");
  if (task.planning === "unresolved") result.push("superpowers:brainstorming", "superpowers:writing-plans");
  // Structural graph reading only helps where code spans modules; text work and routine single-file edits skip it.
  if (task.kind !== "text" && ["developer", "complex_developer", "reviewer", "project_orchestrator"].includes(role)) result.push("graphify");
  return result;
}
