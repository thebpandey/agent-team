import { execFile, spawn } from "node:child_process";
import { constants } from "node:fs";
import { createHash, randomUUID } from "node:crypto";
import { access, chmod, cp, link, lstat, mkdir, mkdtemp, open, opendir, readFile, realpath, rename, rm, rmdir, unlink, writeFile } from "node:fs/promises";
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
    failed: items.filter(({ status }) => ["failed", "cannot_use", "manual_action", "required_unavailable"].includes(status)).length,
  });
  const tools = receipts.filter(({ id }) => TOOL_IDS.has(id));
  const skills = receipts.filter(({ id }) => !TOOL_IDS.has(id));
  const failed = receipts.filter(({ status }) => ["failed", "cannot_use", "manual_action", "required_unavailable"].includes(status));
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
  const details = (result) => Object.fromEntries(["path", "paths", "components", "lifecycleOwnership"]
    .filter((key) => result?.[key] !== undefined).map((key) => [key, structuredClone(result[key])]));
  let ownership = details(probe);
  if (["customized", "manual_action"].includes(probe.status)) {
    return {
      id: dependency.id, version: probe.version ?? null, detected: true, installed: "preserved", ...ownership,
      functional: "not_run", availableToWorker: "not_run", status: probe.status === "manual_action" ? "manual_action" : "cannot_use",
      boundary: dependencyBoundary(dependency, probe, "Existing customization is not owned by Agent-Team."), observedAt,
    };
  }
  const compatible = probe.status === "passed" && (dependency.version === null || probe.version === dependency.version);
  if (["graphify", "ast-grep"].includes(dependency.id) && probe.status === "passed" && !compatible
    && ownership.lifecycleOwnership === "unowned") {
    return {
      id: dependency.id, version: probe.version ?? null, detected: true, installed: "preserved", ...ownership,
      functional: "not_run", availableToWorker: "not_run", status: "manual_action", observedAt,
      boundary: `The canonical unowned ${dependency.id} executable reports ${probe.version ?? "an unknown version"}; expected ${dependency.version}. It was preserved without installer mutation.`,
    };
  }
  if (dependency.executable && !compatible && (probe.status === "passed" || dependency.id === "beads-viewer")) {
    return {
      id: dependency.id, version: probe.version ?? null, detected: true, installed: "preserved", ...ownership,
      functional: "not_run", availableToWorker: "not_run", status: "cannot_use", observedAt,
      boundary: `The selected executable ${dependency.executable} reports ${probe.version ?? "an unknown version"}; expected ${dependency.version}. It was preserved and no shadow tracker was installed.`,
    };
  }
  let installed = probe.installed ?? "reused";
  let detected = probe.status === "passed";
  if (!compatible) {
    const installation = await run({ dependency, phase: "install" });
    if (installation.status !== "passed") {
      const installationOwnership = details(installation);
      if (["manual_action", "customized"].includes(installation.status)) {
        return {
          id: dependency.id, version: installation.version ?? probe.version ?? null, detected,
          installed: installation.installed ?? "preserved", ...installationOwnership,
          functional: "not_run", availableToWorker: "not_run",
          status: installation.status === "manual_action" ? "manual_action" : "cannot_use", observedAt,
          boundary: dependencyBoundary(dependency, installation, "A destination appeared during installation and was preserved."),
        };
      }
      return {
        id: dependency.id, version: installation.version ?? probe.version ?? null, detected,
        installed: "not_installed", functional: "not_run", availableToWorker: "not_run", status: "failed",
        boundary: dependencyBoundary(dependency, installation, "Installation failed."), observedAt,
      };
    }
    installed = installation.installed ?? "installed";
    const createdOwnership = details(installation);
    const installedProbe = await run({ dependency, phase: "probe" });
    ownership = details(installedProbe);
    if (ownership.components && createdOwnership.components) {
      ownership.components = ownership.components.map((component) => {
        const created = createdOwnership.components.find((candidate) => candidate.path === component.path
          && (!candidate.compatibility?.identity || sameExecutableIdentity(candidate.compatibility.identity, component.compatibility?.identity)));
        return created?.lifecycleOwnership === "managed" ? { ...component, installed: "installed", lifecycleOwnership: "managed" } : component;
      });
      ownership.lifecycleOwnership = ownership.components.every(({ lifecycleOwnership }) => lifecycleOwnership === "managed") ? "managed" : "unowned";
    }
    detected = installedProbe.status === "passed";
    if (!detected || (dependency.version !== null && installedProbe.version !== dependency.version)) {
      return {
        id: dependency.id, version: installedProbe.version ?? null, detected, installed, ...ownership,
        functional: "not_run", availableToWorker: "not_run", status: "failed", observedAt,
        boundary: `Installed ${dependency.id}, but expected ${dependency.version} and detected ${installedProbe.version ?? "no executable"}.`,
      };
    }
  }
  if (["playwright-cli", "lean-ctx", "impeccable"].includes(dependency.id)) {
    const companion = await run({ dependency, phase: "companion" });
    const companionOwnership = details(companion);
    if (companionOwnership.components) {
      ownership.components = [...(ownership.components ?? []), ...companionOwnership.components];
      ownership.paths = [...new Set([...(ownership.paths ?? []), ...(companionOwnership.paths ?? [])])];
      ownership.path ??= companionOwnership.path;
      ownership.lifecycleOwnership = ownership.components.every(({ lifecycleOwnership }) => lifecycleOwnership === "managed") ? "managed" : "unowned";
    }
    if (companion.status !== "passed") {
      return {
        id: dependency.id, version: dependency.version, detected, installed, ...ownership,
        functional: "not_run", availableToWorker: "not_run",
        status: companion.status === "manual_action" ? "manual_action" : companion.status === "customized" ? "cannot_use" : "failed", observedAt,
        boundary: dependencyBoundary(dependency, companion, "The selected-scope companion prerequisites were not prepared."),
      };
    }
  }
  const functional = await run({ dependency, phase: "functional", check: dependency.functionalCheck });
  if (functional.status !== "passed") {
    return {
      id: dependency.id, version: dependency.version, detected, installed, ...ownership, functional: "failed",
      availableToWorker: "not_run", status: "failed",
      boundary: dependencyBoundary(dependency, functional, "Functional verification failed."), observedAt,
    };
  }
  const worker = await run({ dependency, phase: "worker", check: "fresh-worker-discovery" });
  if (worker.status !== "passed") {
    return {
      id: dependency.id, version: dependency.version, detected, installed, ...ownership, functional: "passed",
      availableToWorker: worker.status === "unverified" ? "unknown" : "failed", status: "failed",
      boundary: dependencyBoundary(dependency, worker, "A fresh worker could not discover the capability."), observedAt,
    };
  }
  return {
    id: dependency.id, version: dependency.version, detected, installed, ...ownership, functional: "passed",
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
      const planRequired = [...new Set(setup.plan?.requiredCapabilities ?? [])]
        .filter((id) => CATALOG_BY_ID.has(id) && CATALOG_BY_ID.get(id).disposition !== "excluded");
      const defaults = [...new Set([...(selections.defaults ?? DEPENDENCY_CATALOG.filter(({ disposition }) => disposition === "default").map(({ id }) => id)), ...planRequired])]
        .filter((id) => !declinedSet.has(id) || planRequired.includes(id));
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
        const unavailablePrerequisites = dependency.prerequisites.filter((id) => receipts.find((receipt) => receipt.id === id)?.status !== "ready");
        if ((dependency.disposition === "mandatory" || planRequired.includes(dependency.id)) && declinedSet.has(dependency.id)) {
          receipts.push({
            id: dependency.id, version: dependency.version, detected: false, installed: "not_installed",
            functional: "not_run", availableToWorker: "failed", status: "required_unavailable",
            boundary: `Mandatory dependency ${dependency.id} was explicitly declined; readiness remains unavailable until it is approved and verified.`,
            observedAt: new Date().toISOString(),
          });
        } else if (unavailablePrerequisites.length) {
          receipts.push({
            id: dependency.id, version: dependency.version, detected: false, installed: "not_installed",
            functional: "not_run", availableToWorker: "unknown", status: "required_unavailable",
            boundary: `Declared prerequisite(s) unavailable: ${unavailablePrerequisites.join(", ")}.`, observedAt: new Date().toISOString(),
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

function contained(root, candidate) {
  const relative = path.relative(root, candidate);
  return relative === "" || relative !== ".." && !relative.startsWith(`..${path.sep}`) && !path.isAbsolute(relative);
}

async function noLinkedParents(root, target) {
  const relative = path.relative(root, target);
  if (!contained(root, target) || relative === "") throw new Error(`Executable escapes selected tool root: ${target}.`);
  for (const candidate of [root, ...path.dirname(relative).split(path.sep).filter((part) => part !== ".")
    .reduce((items, part) => [...items, path.join(items.at(-1) ?? root, part)], [])]) {
    const stat = await lstat(candidate);
    if (!stat.isDirectory() || stat.isSymbolicLink()) throw new Error(`Executable parent is not a stable selected directory: ${candidate}.`);
  }
}

async function stableExecutableIdentity({ executable, requested = executable, canonical = executable, paths, budget,
  allowExecutableLink = false, packageRoot, packageName, packageVersion }) {
  await noLinkedParents(paths.toolRoot, executable);
  const lexical = await lstat(executable);
  if ((!allowExecutableLink && lexical.isSymbolicLink()) || (!lexical.isFile() && !lexical.isSymbolicLink())) {
    throw new Error(`Executable is not a stable regular selected file: ${executable}.`);
  }
  const [rootReal, resolved] = await Promise.all([realpath(paths.toolRoot), realpath(executable)]);
  if (!contained(rootReal, resolved)) throw new Error(`Executable realpath escapes selected tool root: ${executable}.`);
  let packageReal = null;
  if (packageRoot) {
    packageReal = await realpath(packageRoot);
    if (!contained(rootReal, packageReal) || !contained(packageReal, resolved)) throw new Error("Executable escapes the verified package root.");
    const metadata = JSON.parse((await companionBytes(path.join(packageReal, "package.json"), budget, 64 * 1024)).toString("utf8"));
    if (metadata.name !== packageName || metadata.version !== packageVersion) throw new Error("Executable package identity does not match the pinned package.");
  }
  const handle = await open(resolved, constants.O_RDONLY | constants.O_NOFOLLOW | constants.O_NONBLOCK);
  let stat;
  let bytes;
  try {
    stat = await handle.stat();
    if (!stat.isFile() || stat.size > 128 * 1024 * 1024) throw new Error(`Executable byte/type limit exceeded: ${executable}.`);
    const chunks = [];
    for await (const chunk of handle.createReadStream({ autoClose: false, start: 0, end: 128 * 1024 * 1024, signal: budget?.signal })) {
      budget?.check();
      chunks.push(chunk);
    }
    bytes = Buffer.concat(chunks);
    const after = await handle.stat();
    if (after.dev !== stat.dev || after.ino !== stat.ino || after.size !== stat.size || bytes.length !== stat.size) {
      throw new Error(`Executable identity changed while it was read: ${executable}.`);
    }
  } finally { await handle.close(); }
  const [targetStat, resolvedAgain] = await Promise.all([lstat(resolved), realpath(executable)]);
  if (resolvedAgain !== resolved || targetStat.dev !== stat.dev || targetStat.ino !== stat.ino || !targetStat.isFile()) {
    throw new Error(`Executable identity changed while it was inspected: ${executable}.`);
  }
  return { requested, canonical, realpath: resolved, ...(packageReal ? { packageRoot: packageReal, packageName, packageVersion } : {}),
    dev: stat.dev, ino: stat.ino, mode: stat.mode, size: stat.size, sha256: createHash("sha256").update(bytes).digest("hex") };
}

async function astGrepExecutable(dependency, paths, budget) {
  const canonical = path.join(paths.toolRoot, "bin", process.platform === "win32" ? "ast-grep.cmd" : "ast-grep");
  const requested = dependency.executable ?? canonical;
  if (requested !== canonical && (!path.isAbsolute(requested) || path.basename(requested, path.extname(requested)) !== "sg" || path.dirname(requested) !== path.dirname(canonical))) {
    throw new Error(`Rejected ast-grep alias outside the selected managed package: ${requested}.`);
  }
  const packageRoot = path.join(paths.toolRoot, process.platform === "win32" ? "node_modules" : "lib/node_modules", "@ast-grep", "cli");
  const [requestedIdentity, canonicalIdentity] = await Promise.all([
    stableExecutableIdentity({ executable: requested, requested, canonical, paths, budget, allowExecutableLink: true,
      packageRoot, packageName: "@ast-grep/cli", packageVersion: "0.45.3" }),
    stableExecutableIdentity({ executable: canonical, requested, canonical, paths, budget, allowExecutableLink: true,
      packageRoot, packageName: "@ast-grep/cli", packageVersion: "0.45.3" }),
  ]);
  if (requestedIdentity.dev !== canonicalIdentity.dev || requestedIdentity.ino !== canonicalIdentity.ino
    || requestedIdentity.sha256 !== canonicalIdentity.sha256) throw new Error("Rejected ast-grep alias that does not identify the canonical entrypoint.");
  return { executable: canonical, identity: requestedIdentity };
}

function executableComponent(executable, identity, ownership = "unowned", installed = "reused_unowned") {
  return { id: "executable", path: executable, realpath: identity.realpath, lifecycleOwnership: ownership, installed,
    compatibility: { status: "exact", entrypoint: null, requiredFiles: [], unrelatedRegularFilesPreserved: false,
      identity: structuredClone(identity) } };
}

async function manualExecutableResult(executable, identity, evidence) {
  let observed = identity;
  if (!observed) {
    try { observed = { requested: executable, canonical: executable, realpath: await realpath(executable) }; }
    catch { observed = { requested: executable, canonical: executable }; }
  }
  return { status: "manual_action", installed: "preserved", lifecycleOwnership: "unowned", path: executable,
    paths: [executable], components: [{ id: "executable", path: executable, ...(observed.realpath ? { realpath: observed.realpath } : {}),
      lifecycleOwnership: "unowned", installed: "preserved", compatibility: { status: "incompatible", entrypoint: null,
        requiredFiles: [], unrelatedRegularFilesPreserved: false, identity: structuredClone(observed) } }], evidence };
}

function sameExecutableIdentity(left, right) {
  return left && right && ["requested", "canonical", "realpath", "packageRoot", "packageName", "packageVersion", "dev", "ino", "mode", "size", "sha256"]
    .every((key) => left[key] === right[key]);
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

function skillCompatibilityForGuidance(dependency, host) {
  const selectedPath = dependency.guidance?.selectedPath;
  const value = dependency.guidance?.gitBlobs?.[host];
  if (!selectedPath || !value) return undefined;
  return { kind: "required-files", entrypoint: "SKILL.md", allowUnrelatedRegularFiles: true, selectedPaths: [{
    selectedPath, requiredFiles: [{ path: "SKILL.md", digest: { algorithm: "git-blob-sha1", value } }],
  }] };
}

async function manualSkillResult(dependency, paths, selectedPath, destination, evidence, compatibilityStatus = "incompatible") {
  let resolved = null;
  try { resolved = await realpath(destination); } catch { /* Preserve the selected lexical path even when it cannot resolve safely. */ }
  const component = { id: dependency.id === "impeccable" ? "guidance" : "skill", path: destination,
    ...(resolved ? { realpath: resolved } : {}), lifecycleOwnership: "unowned", installed: "preserved",
    compatibility: { status: compatibilityStatus, entrypoint: dependency.compatibility?.entrypoint ?? null,
      requiredFiles: [], unrelatedRegularFilesPreserved: true } };
  return { status: "manual_action", installed: "preserved", lifecycleOwnership: "unowned", path: destination,
    paths: [destination], components: [component], evidence };
}

async function inspectSkillDestinations(dependency, paths, { budget, bounded = false } = {}) {
  const present = [];
  const absent = [];
  for (const selectedPath of dependency.install.paths) {
    budget?.check();
    const destination = skillDestination(dependency, paths, selectedPath);
    try { await lstat(destination); }
    catch (error) {
      if (error.code !== "ENOENT") throw error;
      absent.push({ selectedPath, destination });
      continue;
    }
    present.push({ selectedPath, destination });
  }
  if (!present.length) return { status: "not_found" };
  if (absent.length) {
    const evidence = `Preserved mixed partial skill destinations ${present.map(({ destination }) => destination).join(", ")}; missing ${absent.map(({ destination }) => destination).join(", ")}.`;
    const selected = [...present, ...absent].sort((left, right) => dependency.install.paths.indexOf(left.selectedPath) - dependency.install.paths.indexOf(right.selectedPath));
    const classified = await Promise.all(selected.map(({ selectedPath, destination }) => manualSkillResult(
      dependency, paths, selectedPath, destination, evidence, "partial")));
    return { status: "manual_action", installed: "preserved", lifecycleOwnership: "unowned",
      path: classified[0].path, paths: classified.map(({ path: selectedPath }) => selectedPath),
      components: classified.flatMap(({ components }) => components), evidence };
  }
  const components = [];
  const problems = [];
  const preserve = async (selectedPath, destination, evidence, status) => {
    const result = await manualSkillResult(dependency, paths, selectedPath, destination, evidence, status);
    components.push(...result.components);
    problems.push(result.evidence);
  };
  destinationLoop: for (const { selectedPath, destination } of present) {
    budget?.check();
    const requirement = dependency.compatibility?.selectedPaths?.find((entry) => entry.selectedPath === selectedPath);
    let allFiles;
    try {
      allFiles = await skillContents(destination, budget);
      const entrypoints = Object.keys(allFiles).filter((file) => path.posix.basename(file) === (dependency.compatibility?.entrypoint ?? "SKILL.md"));
      if (entrypoints.length !== 1 || entrypoints[0] !== (dependency.compatibility?.entrypoint ?? "SKILL.md")) {
        await preserve(selectedPath, destination,
          `Preserved ambiguous skill entrypoint(s): ${entrypoints.join(", ") || "none"}.`, "ambiguous_entrypoint");
        continue destinationLoop;
      }
    } catch (error) {
      if (error.code === "EVENT_DEADLINE") throw error;
      await preserve(selectedPath, destination, error.message);
      continue;
    }
    const metadataPath = path.join(destination, ".agent-team-source.json");
    let metadata;
    let metadataPresent = false;
    try {
      metadata = JSON.parse((await companionBytes(metadataPath, budget, 64 * 1024)).toString("utf8"));
      metadataPresent = true;
      if (metadata.source !== dependency.install.source || metadata.revision !== dependency.version || metadata.selectedPath !== selectedPath) {
        await preserve(selectedPath, destination, `Preserved existing skill path with different provenance: ${destination}`);
        continue destinationLoop;
      }
    } catch (error) {
      if (error.code === "EVENT_DEADLINE") throw error;
      budget?.check();
      if (error.code !== "ENOENT") {
        await preserve(selectedPath, destination, `Preserved malformed or non-regular skill provenance: ${destination}. ${error.message}`);
        continue destinationLoop;
      }
    }
    if (!metadataPresent && !requirement) {
      await preserve(selectedPath, destination, `Preserved existing skill path without a pinned compatibility declaration: ${destination}.`, "undeclared");
      continue;
    }
    const requiredFiles = [];
    for (const required of requirement?.requiredFiles ?? []) {
      const file = path.join(destination, required.path);
      try {
        const bytes = await companionBytes(file, budget);
        const algorithm = required.digest.algorithm;
        const actual = algorithm === "git-blob-sha1"
          ? createHash("sha1").update(`blob ${bytes.length}\0`).update(bytes).digest("hex")
          : createHash("sha256").update(bytes).digest("hex");
        if (actual !== required.digest.value) {
          await preserve(selectedPath, destination, `Preserved incompatible required skill file: ${file}.`);
          continue destinationLoop;
        }
        requiredFiles.push({ path: required.path, digest: structuredClone(required.digest), size: bytes.length });
      } catch (error) {
        if (error.code === "EVENT_DEADLINE") throw error;
        await preserve(selectedPath, destination, `Preserved missing or invalid required skill file: ${file}. ${error.message}`);
        continue destinationLoop;
      }
    }
    const ownership = metadataPresent ? "managed" : "unowned";
    components.push({
      id: dependency.id === "impeccable" ? "guidance" : "skill", path: destination, realpath: await realpath(destination), lifecycleOwnership: ownership,
      installed: metadataPresent ? "reused" : "reused_unowned",
      compatibility: { status: requirement ? "exact" : "not_applicable", entrypoint: requirement ? dependency.compatibility.entrypoint : null,
        requiredFiles, unrelatedRegularFilesPreserved: requirement ? Object.keys(allFiles).some((file) => !requiredFiles.some(({ path: requiredPath }) => requiredPath === file)) : false },
    });
  }
  if (problems.length) {
    return { status: "manual_action", installed: "preserved", lifecycleOwnership: "unowned", path: components[0].path,
      paths: components.map(({ path: selected }) => selected), components, evidence: problems.join(" ") };
  }
  const lifecycleOwnership = components.every(({ lifecycleOwnership }) => lifecycleOwnership === "managed") ? "managed" : "unowned";
  return { status: "passed", version: dependency.version, path: components[0].path, paths: components.map(({ path: selected }) => selected), components,
    lifecycleOwnership, installed: lifecycleOwnership === "managed" ? "reused" : "reused_unowned",
    evidence: lifecycleOwnership === "managed" ? "All selected managed skill paths matched." : "All selected unowned skill paths matched pinned compatibility." };
}

async function validateGitSkillStage(dependency, selectedPath, root, budget) {
  const reserved = path.join(root, ".agent-team-source.json");
  try {
    await lstat(reserved);
    throw new Error(`Pinned skill source contains reserved provenance: ${reserved}`);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
  }
  const files = await skillContents(root, budget);
  if (Object.keys(files).some((file) => path.posix.basename(file) === ".agent-team-source.json")) {
    throw new Error("Pinned skill source contains nested reserved provenance.");
  }
  const entrypoint = dependency.compatibility?.entrypoint ?? "SKILL.md";
  const entrypoints = Object.keys(files).filter((file) => path.posix.basename(file) === entrypoint);
  if (entrypoints.length !== 1 || entrypoints[0] !== entrypoint) {
    throw new Error(`Pinned skill source has ambiguous entrypoint(s): ${entrypoints.join(", ") || "none"}.`);
  }
  const requirement = dependency.compatibility?.selectedPaths?.find((entry) => entry.selectedPath === selectedPath);
  for (const required of requirement?.requiredFiles ?? []) {
    const bytes = await companionBytes(path.join(root, required.path), budget);
    if (!["git-blob-sha1", "sha256"].includes(required.digest.algorithm)) {
      throw new Error(`Pinned skill source declares unsupported digest: ${required.digest.algorithm}.`);
    }
    const actual = required.digest.algorithm === "git-blob-sha1"
      ? createHash("sha1").update(`blob ${bytes.length}\0`).update(bytes).digest("hex")
      : createHash("sha256").update(bytes).digest("hex");
    if (actual !== required.digest.value) throw new Error(`Pinned skill source has incompatible required file: ${required.path}.`);
  }
  return files;
}

async function unchangedGitSkillPublication(destination, expected) {
  try {
    const stat = await lstat(destination);
    if (!stat.isDirectory() || stat.dev !== expected.identity.dev || stat.ino !== expected.identity.ino) return false;
    const files = await skillContents(destination);
    const metadata = await companionBytes(path.join(destination, ".agent-team-source.json"), undefined, 64 * 1024);
    return JSON.stringify(files) === JSON.stringify(expected.files) && metadata.equals(expected.metadata);
  } catch { return false; }
}

async function installGitSkills(dependency, paths, budget) {
  if (!dependency.install.paths.length) return { status: "failed", evidence: "No selective skill paths are approved for this optional source." };
  const preflight = await inspectSkillDestinations(dependency, paths, { budget, bounded: true });
  if (preflight.status === "passed") return preflight;
  if (preflight.status !== "not_found") return preflight;
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
  const afterFetch = await inspectSkillDestinations(dependency, paths, { budget, bounded: true });
  if (afterFetch.status !== "not_found") return afterFetch;
  await mkdir(paths.skillRoot, { recursive: true, mode: 0o700 });
  const stageRoot = path.join(paths.skillRoot, `.agent-team-stage-${randomUUID()}`);
  const staged = [];
  try {
    for (const selectedPath of dependency.install.paths) {
      const stage = path.join(stageRoot, path.basename(selectedPath === "." ? dependency.id : selectedPath));
      await mkdir(stage, { recursive: true, mode: 0o700 });
      if (selectedPath === ".") {
        for (const includedPath of dependency.install.includePaths ?? []) {
          await cp(path.join(sourceRoot, includedPath), path.join(stage, includedPath), { recursive: true, errorOnExist: true, force: false });
        }
      } else {
        for await (const entry of await opendir(path.join(sourceRoot, selectedPath))) {
          await cp(path.join(sourceRoot, selectedPath, entry.name), path.join(stage, entry.name), { recursive: true, errorOnExist: true, force: false });
        }
      }
      const files = await validateGitSkillStage(dependency, selectedPath, stage, budget);
      const metadata = Buffer.from(`${JSON.stringify({ source: dependency.install.source, revision: dependency.version, selectedPath }, null, 2)}\n`);
      await writeFile(path.join(stage, ".agent-team-source.json"), metadata, { mode: 0o600, flag: "wx" });
      const stat = await lstat(stage);
      staged.push({ selectedPath, stage, destination: skillDestination(dependency, paths, selectedPath),
        expected: { files, metadata, identity: { dev: stat.dev, ino: stat.ino } } });
    }
  } catch (error) {
    await rm(stageRoot, { recursive: true, force: true });
    if (error.code === "EVENT_DEADLINE") throw error;
    return { status: "failed", evidence: `Pinned skill source prevalidation failed before publication: ${error.message}` };
  }
  let beforePublish;
  try { beforePublish = await inspectSkillDestinations(dependency, paths, { budget, bounded: true }); }
  catch (error) {
    await rm(stageRoot, { recursive: true, force: true });
    throw error;
  }
  if (beforePublish.status !== "not_found") {
    await rm(stageRoot, { recursive: true, force: true });
    return beforePublish;
  }
  const published = [];
  try {
    for (const item of staged) {
      await rename(item.stage, item.destination);
      published.push(item);
    }
  } catch (error) {
    for (const item of published.reverse()) {
      if (await unchangedGitSkillPublication(item.destination, item.expected)) await rm(item.destination, { recursive: true, force: true });
    }
    await rm(stageRoot, { recursive: true, force: true });
    if (["EEXIST", "ENOTEMPTY"].includes(error.code)) return inspectSkillDestinations(dependency, paths, { budget, bounded: true });
    if (error.code === "EVENT_DEADLINE") throw error;
    return { status: "failed", evidence: `Pinned skill publication failed: ${error.message}` };
  }
  await rm(stageRoot, { recursive: true, force: true });
  const installed = await inspectSkillDestinations(dependency, paths, { budget, bounded: true });
  if (installed.status !== "passed") return installed;
  const createdSet = new Set(published.map(({ destination }) => destination));
  const components = installed.components.map((component) => createdSet.has(component.path)
    ? { ...component, installed: "installed", lifecycleOwnership: "managed" } : component);
  return { ...installed, installed: components.every(({ installed: disposition }) => disposition === "installed") ? "installed" : installed.installed,
    lifecycleOwnership: components.every(({ lifecycleOwnership }) => lifecycleOwnership === "managed") ? "managed" : "unowned", components };
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
    if (!["not_found", "passed"].includes(preflight.status)) return { ...preflight, status: "customized" };
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
    const identity = await stableExecutableIdentity({ executable: destination, requested: destination, canonical: destination, paths, budget });
    return { status: "passed", version: dependency.version, path: destination, paths: [destination], lifecycleOwnership: "managed",
      components: [executableComponent(destination, identity, "managed", "installed")],
      evidence: `Verified pinned ${asset} SHA256 ${digest}; no npm lifecycle or initializer executed.` };
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
  const component = async (ownership, installed, bytes) => ({ id: "skill", path: destination, realpath: await realpath(destination),
    lifecycleOwnership: ownership, installed, compatibility: { status: "exact", entrypoint: "SKILL.md",
      requiredFiles: [{ path: "SKILL.md", digest: { algorithm: "git-blob-sha1", value: provenance.gitBlob }, size: bytes.length }],
      unrelatedRegularFilesPreserved: Object.keys(await skillContents(destination, budget)).some((file) => file !== "SKILL.md") } });
  try {
    try {
      const files = await skillContents(destination, budget);
      const entrypoints = Object.keys(files).filter((file) => path.posix.basename(file) === "SKILL.md");
      if (entrypoints.length !== 1 || entrypoints[0] !== "SKILL.md") {
        return manualSkillResult(dependency, paths, "skills/lean-ctx", destination,
          `Preserved ambiguous skill entrypoint(s): ${entrypoints.join(", ") || "none"}.`, "ambiguous_entrypoint");
      }
      const bytes = await companionBytes(path.join(destination, "SKILL.md"), budget, 64 * 1024);
      const blob = createHash("sha1").update(`blob ${bytes.length}\0`).update(bytes).digest("hex");
      if (blob !== provenance.gitBlob) return manualSkillResult(dependency, paths, "skills/lean-ctx", destination,
        `Preserved customized LeanCTX skill: ${destination}`);
      let ownership = "unowned";
      let installed = "reused_unowned";
      try {
        const metadata = JSON.parse(await companionBytes(path.join(destination, ".agent-team-source.json"), budget, 64 * 1024));
        if (!Object.entries(provenance).every(([key, value]) => metadata[key] === value)) {
          return manualSkillResult(dependency, paths, "skills/lean-ctx", destination,
            `Preserved mismatched LeanCTX skill provenance: ${destination}`);
        }
        ownership = "managed";
        installed = "reused";
      } catch (error) {
        if (error.code === "EVENT_DEADLINE") throw error;
        if (error.code !== "ENOENT") return manualSkillResult(dependency, paths, "skills/lean-ctx", destination,
          `Preserved malformed or non-regular LeanCTX skill provenance: ${destination}`);
      }
      const record = await component(ownership, installed, bytes);
      return { status: "passed", installed, lifecycleOwnership: ownership, path: destination, paths: [destination], components: [record],
        evidence: ownership === "managed" ? "Reused complete pinned LeanCTX skill." : "Reused exact unowned LeanCTX skill in place." };
    } catch (error) {
      if (error.code === "EVENT_DEADLINE") throw error;
      try { await lstat(destination); return manualSkillResult(dependency, paths, "skills/lean-ctx", destination,
        `Preserved existing LeanCTX skill: ${destination}. ${error.message}`); }
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
    const record = await component("managed", "installed", bytes);
    return { status: "passed", installed: "installed", lifecycleOwnership: "managed", path: destination, paths: [destination], components: [record],
      evidence: "Prepared complete pinned LeanCTX host skill; Agent-Team narrowed profile governs its use." };
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
const GRAPHIFY_FIXTURE_CALLBACK = {
  from: "dispatch()",
  to: "handler()",
  relation: "indirect_call",
  confidence: "INFERRED",
  confidenceScore: 0.85,
  origin: "ast",
};

async function graphifyFunctional(executable, paths, budget) {
  const verification = path.join(paths.toolRoot, "verification");
  budget?.check();
  await mkdir(verification, { recursive: true, mode: 0o700 });
  const root = await mkdtemp(path.join(verification, "graphify-"));
  const env = { ...process.env };
  for (const key of GRAPHIFY_BACKEND_KEYS) delete env[key];
  const options = { cwd: root, env, budget };
  try {
    await writeFile(path.join(root, "app.py"), "def load_config():\n    return {}\n\n\ndef start_server():\n    cfg = load_config()\n    return cfg\n\n\ndef handler(value):\n    return value\n\n\ndef dispatch(values):\n    return list(map(handler, values))\n", { mode: 0o600, signal: budget?.signal });
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
    const graphItems = [...(graph.nodes ?? []), ...(graph.links ?? [])];
    const nonAst = graphItems.find(({ _origin }) => _origin !== "ast");
    if (nonAst) {
      return {
        status: "failed",
        evidence: `Graphify code-only extraction emitted ${nonAst._origin ?? "missing"} provenance; expected AST-origin graph content only.`,
      };
    }
    const labels = [...GRAPHIFY_FIXTURE_CHAIN, GRAPHIFY_FIXTURE_CALLBACK.from, GRAPHIFY_FIXTURE_CALLBACK.to];
    const ids = new Map((graph.nodes ?? []).filter(({ label }) => labels.includes(label)).map(({ label, id }) => [label, id]));
    const missingLabel = GRAPHIFY_FIXTURE_CHAIN.find((label) => !ids.has(label));
    if (missingLabel) return { status: "failed", evidence: `The extracted graph did not contain the known fixture symbol ${missingLabel}.` };
    const missingCallbackLabel = [GRAPHIFY_FIXTURE_CALLBACK.from, GRAPHIFY_FIXTURE_CALLBACK.to].find((label) => !ids.has(label));
    if (missingCallbackLabel) return { status: "failed", evidence: `The extracted graph did not contain the known callback fixture symbol ${missingCallbackLabel}.` };
    const missingEdge = GRAPHIFY_FIXTURE_EDGES.find(([from, to, relation]) => !(graph.links ?? [])
      .some((link) => link.source === ids.get(from) && link.target === ids.get(to) && link.relation === relation));
    if (missingEdge) {
      return { status: "failed", evidence: `The extracted graph did not contain the known fixture edge ${missingEdge[0]} --${missingEdge[2]}--> ${missingEdge[1]}.` };
    }
    const callback = (graph.links ?? []).find((link) => link.source === ids.get(GRAPHIFY_FIXTURE_CALLBACK.from) &&
      link.target === ids.get(GRAPHIFY_FIXTURE_CALLBACK.to) && link.relation === GRAPHIFY_FIXTURE_CALLBACK.relation);
    if (!callback || callback.confidence !== GRAPHIFY_FIXTURE_CALLBACK.confidence || callback.confidence_score !== GRAPHIFY_FIXTURE_CALLBACK.confidenceScore || callback._origin !== GRAPHIFY_FIXTURE_CALLBACK.origin) {
      return { status: "failed", evidence: `The extracted graph did not contain the known callback fixture edge ${GRAPHIFY_FIXTURE_CALLBACK.from} --${GRAPHIFY_FIXTURE_CALLBACK.relation}--> ${GRAPHIFY_FIXTURE_CALLBACK.to} with AST-origin INFERRED confidence 0.85.` };
    }
    const traced = await command(executable, ["path", "Pool", "load_config"], options);
    const offsets = GRAPHIFY_FIXTURE_CHAIN.map((label) => traced.stdout?.indexOf(label) ?? -1);
    if (traced.status !== "passed" || offsets.some((offset, index) => offset < 0 || (index > 0 && offset <= offsets[index - 1]))) {
      return { status: "failed", evidence: evidence(traced) ?? `Graphify path did not report the chain ${GRAPHIFY_FIXTURE_CHAIN.join(" -> ")}.` };
    }
    const explained = await command(executable, ["explain", "start_server"], options);
    return explained.status === "passed" && ["start_server()", "load_config()"].every((label) => explained.stdout?.includes(label))
      ? { status: "passed", evidence: "Graphify code-only extraction built an AST-origin graph with deterministic inferred callback resolution and resolved path Pool -> load_config and explain start_server in an isolated fixture." }
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
  const boundIdentities = new WeakMap();
  return async ({ dependency, phase, check, budget = eventBudget }) => {
    budget?.check();
    const execute = (file, args, options = {}) => command(file, args, { ...options, budget });
    let executable = binaryPath(dependency, paths.toolRoot);
    let identity = null;
    try {
      const canonicalGraphify = binaryPath({ ...dependency, executable: undefined }, paths.toolRoot);
      if (dependency.id === "graphify" && dependency.executable && dependency.executable !== canonicalGraphify) {
        return manualExecutableResult(canonicalGraphify, null, `Graphify must use the selected canonical path ${canonicalGraphify}.`);
      }
      if (phase !== "install" && dependency.id === "ast-grep") ({ executable, identity } = await astGrepExecutable(dependency, paths, budget));
      if (phase !== "install" && dependency.id === "graphify") identity = await stableExecutableIdentity({ executable, requested: executable,
        canonical: canonicalGraphify, paths, budget });
    } catch (error) {
      if (error.code === "EVENT_DEADLINE") throw error;
      if (error.code === "ENOENT" && phase === "probe") identity = null;
      else return manualExecutableResult(executable, identity, error.message);
    }
    if (identity) {
      const prior = boundIdentities.get(dependency);
      if (prior && !sameExecutableIdentity(prior, identity)) {
        return manualExecutableResult(executable, identity, `Selected ${dependency.id} executable identity changed after qualification.`);
      }
      if (phase !== "probe" && !prior) boundIdentities.set(dependency, identity);
    }
    if (phase === "probe") {
      if (dependency.id === "playwright-cli") {
        const companion = await inspectSkillDestinations(playwrightSkill(dependency), paths, { budget, bounded: true });
        if (companion.status === "customized") return companion;
      }
      if (dependency.install?.kind === "git-skill") {
        if (!dependency.install.paths.length) return { status: "not_found" };
        return inspectSkillDestinations(dependency, paths, { budget, bounded: true });
      }
      const result = await execute(executable, ["--version"], dependency.id === "playwright-cli"
        ? { env: { ...process.env, NO_UPDATE_NOTIFIER: "1" } } : {});
      if (result.status !== "passed") return result;
      if (["graphify", "ast-grep"].includes(dependency.id) && !identity) {
        return manualExecutableResult(executable, identity, `Selected ${dependency.id} executable could not be bound to a stable identity.`);
      }
      if (["graphify", "ast-grep"].includes(dependency.id)) boundIdentities.set(dependency, identity);
      if (["graphify", "ast-grep", "impeccable", "lean-ctx"].includes(dependency.id)) {
        const executableIdentity = identity ?? await stableExecutableIdentity({ executable, requested: executable, canonical: executable, paths, budget });
        if (["impeccable", "lean-ctx"].includes(dependency.id)) boundIdentities.set(dependency, executableIdentity);
        const component = executableComponent(executable, executableIdentity);
        return { ...result, version: parsedVersion(`${result.stdout}\n${result.stderr}`, dependency.id), installed: "reused_unowned",
          lifecycleOwnership: "unowned", path: executable, paths: [executable], components: [component] };
      }
      return { ...result, version: parsedVersion(`${result.stdout}\n${result.stderr}`, dependency.id) };
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
      if (dependency.install?.kind === "git-skill") return installGitSkills(dependency, paths, budget);
      if (dependency.id === "uv" && dependency.install?.kind === "github-release") return installUvRelease(dependency, paths);
      if (dependency.id === "lean-ctx" && dependency.install?.kind === "github-release") return installLeanCtxRelease(dependency, paths, budget);
      if (dependency.id === "beads-viewer" && dependency.install?.kind === "github-release") return installBeadsViewerRelease(dependency, paths, budget);
      return { status: "failed", evidence: `Pinned ${dependency.install?.kind ?? "unknown"} preparation requires its verified installer adapter.` };
    }
    if (phase === "companion" && dependency.id === "lean-ctx") return prepareLeanCtxSkill(dependency, paths, budget);
    if (phase === "companion" && dependency.id === "impeccable") {
      const guidance = {
        ...dependency,
        install: { kind: "git-skill", source: dependency.install.source, revision: dependency.version,
          paths: [dependency.guidance.selectedPath] },
        compatibility: skillCompatibilityForGuidance(dependency, host),
      };
      const existing = await inspectSkillDestinations(guidance, paths, { budget, bounded: true });
      if (["passed", "manual_action", "customized"].includes(existing.status)) return existing;
      const result = await execute(executable,
        ["install", "-y", `--providers=${host === "codex" ? "codex" : "claude"}`, `--scope=${scope}`, "--no-hooks"],
        { cwd: paths.projectRoot, env: { ...process.env, IMPECCABLE_HOME: path.join(paths.toolRoot, "impeccable-home") } });
      if (result.status !== "passed") return result;
      const installedGuidance = await inspectSkillDestinations(guidance, paths, { budget, bounded: true });
      if (installedGuidance.status !== "passed") return installedGuidance;
      return installedGuidance;
    }
    if (phase === "companion" && dependency.id === "playwright-cli") {
      const companion = await preparePlaywrightSkill(dependency, paths, budget);
      if (companion.status !== "passed") return companion;
      const step = buildPreparationPlan({ dependencyId: dependency.id, host, scope, paths, executable }).browserInstall;
      const browser = await execute(step.file, step.args, { cwd: step.cwd, env: { ...process.env, ...step.env } });
      return browser.status === "passed" ? companion : browser;
    }
    if (phase === "functional") {
      if (check === "command") return execute(executable, ["--version"]);
      if (functionalAdapters[dependency.id]) return functionalAdapters[dependency.id]({ dependency, executable, executableIdentity: identity, host, scope, paths, command: execute });
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
      if (workerDiscovery) return workerDiscovery({ dependency, executable, executableIdentity: identity, host, scope, paths });
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
