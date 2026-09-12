#!/usr/bin/env node
import { execFile } from "node:child_process";
import { mkdir, realpath } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { auditEffectiveness } from "./lib/telemetry.mjs";
import { buildArtifacts, checkArtifacts } from "./lib/artifacts.mjs";
import { getHealth } from "./lib/health.mjs";
import { initializeProject, validateInitializationEnvelope } from "./lib/initialization.mjs";
import { readOwnerRecoveryEnvelope, validateOwnerRecoveryEnvelope } from "./lib/owner-recovery.mjs";
import { installPackage, rollbackPackage, uninstallPackage } from "./lib/install.mjs";
import { checkInstalledPackage, checkPackage } from "./lib/package-validator.mjs";
import { runSetupCommand, setupCommandFlags } from "./lib/setup-cli.mjs";
import { readRequestEnvelope, runWorkflowCommand, workflowCommandFlags } from "./lib/workflow-cli.mjs";

const run = promisify(execFile);

const commandFlags = {
  health: new Set(["home", "project", "scope"]),
  audit: new Set(["home", "log", "tracker", "mistakes", "limit"]),
  install: new Set(["source", "home", "host", "scope", "project"]),
  uninstall: new Set(["home", "host", "scope", "project"]),
  rollback: new Set(["home", "host", "scope", "project"]),
  "check-package": new Set(["source"]),
  "check-installed-package": new Set(["source"]),
  "check-artifacts": new Set(["source", "archive", "revision"]),
  "build-artifacts": new Set(["source", "output", "revision"]),
  "project-initialize": new Set(["project", "request"]),
  "project-owner-recover": new Set(["project", "request"]),
  ...workflowCommandFlags,
  ...setupCommandFlags,
};

function flags(command, args) {
  if (!commandFlags[command]) throw new Error(`Unknown command: ${command ?? "missing"}`);
  const output = { archive: [] };
  for (let index = 0; index < args.length; index += 1) {
    if (!args[index].startsWith("--")) throw new Error(`Unexpected positional argument: ${args[index]}`);
    const name = args[index].slice(2);
    if (!commandFlags[command]?.has(name)) throw new Error(`Unsupported flag for ${command ?? "missing"}: --${name}`);
    const value = args[++index];
    if (value === undefined || value.startsWith("--")) throw new Error(`Missing value for --${name}.`);
    if (name === "archive") output.archive.push(value);
    else output[name] = value;
  }
  return output;
}

async function gitRevision(sourceRoot) {
  const { stdout } = await run("git", ["rev-parse", "HEAD"], { cwd: sourceRoot, encoding: "utf8", timeout: 1500 });
  return stdout.trim();
}

/** Provide one structured command surface for package and project workflows. */
export async function runCommand(command, options, context = {}) {
  const sourceRoot = path.resolve(options.source ?? path.join(import.meta.dirname, ".."));
  const home = path.resolve(options.home ?? os.homedir());
  if (command === "health") return getHealth({
    home,
    ...(options.project ? { projectPath: path.resolve(options.project) } : {}),
    ...(options.scope ? { scope: options.scope } : {}),
  });
  if (command === "audit") return auditEffectiveness({
    logDirectory: path.resolve(options.log ?? path.join(home, ".agent-team-hooks", "logs")),
    trackerPath: options.tracker,
    mistakesPath: options.mistakes,
    maxRecords: Number(options.limit ?? 1000),
  });
  if (command === "install") {
    if (options.scope === "user" && options.project) throw new Error("--project is ineffective with --scope user.");
    return installPackage({ sourceRoot, home, host: options.host, scope: options.scope, projectRoot: options.project && path.resolve(options.project) });
  }
  if (["uninstall", "rollback"].includes(command)) {
    if (options.scope === "user" && options.project) throw new Error("--project is ineffective with --scope user.");
    const uninstall = command === "rollback" ? rollbackPackage : uninstallPackage;
    return uninstall({ home, host: options.host, scope: options.scope, projectRoot: options.project && path.resolve(options.project) });
  }
  if (command === "check-package") return checkPackage(sourceRoot);
  if (command === "check-installed-package") return checkInstalledPackage(sourceRoot);
  if (command === "check-artifacts") return checkArtifacts({
    sourceRoot,
    archives: options.archive.map((file) => path.resolve(file)),
    expectedRevision: options.archive.length ? options.revision ?? await gitRevision(sourceRoot) : options.revision,
  });
  if (command === "build-artifacts") {
    const outputDirectory = path.resolve(options.output ?? path.join(sourceRoot, ".superpowers", "artifacts"));
    await mkdir(outputDirectory, { recursive: true });
    return buildArtifacts({ sourceRoot, outputDirectory, sourceRevision: options.revision ?? await gitRevision(sourceRoot) });
  }
  if (command === "project-initialize") {
    if (typeof options.project !== "string" || !options.project.trim()) throw new Error("--project is required.");
    const envelope = await readRequestEnvelope(options.request);
    const invalid = validateInitializationEnvelope(envelope);
    if (invalid) return { status: "conflict", ready: false, reason: invalid, canonicalReady: false };
    const result = await initializeProject(path.resolve(options.project), envelope.request, {
      actorSessionId: envelope.actorSessionId, nativeIdentity: context.nativeIdentity, expectedVersion: envelope.expectedVersion,
    });
    return { ...result, canonicalReady: result.ready, ready: false,
      ...(["applied", "duplicate"].includes(result.status) ? { nextAction: "Prepare selected dependencies and run readiness for the actual host and capability scope. Native trust and discovery remain separate." } : {}) };
  }
  if (command === "project-owner-recover") {
    if (typeof options.project !== "string" || !options.project.trim()) throw new Error("--project is required.");
    const envelope = await readOwnerRecoveryEnvelope(options.request);
    const invalid = validateOwnerRecoveryEnvelope(envelope);
    if (invalid) return { status: "conflict", ready: false, reason: invalid };
    // No current host bootstrap can mint the opaque capability. Refuse before resolving or reading the project.
    return { status: "validated", ready: false, reason: "native_owner_recovery_required" };
  }
  if (workflowCommandFlags[command]) return runWorkflowCommand(command, options, context);
  if (setupCommandFlags[command]) return runSetupCommand(command, options, context);
  throw new Error(`Unknown command: ${command ?? "missing"}`);
}

const invokedPath = process.argv[1] && await realpath(process.argv[1]).catch(() => undefined);
if (invokedPath && invokedPath === await realpath(fileURLToPath(import.meta.url))) {
  const command = process.argv[2];
  const streaming = command === "dashboard-start";
  try {
    const result = await runCommand(command, flags(command, process.argv.slice(3)), {
      onStarted(value) { process.stdout.write(`${JSON.stringify(value)}\n`); },
    });
    process.stdout.write(`${JSON.stringify(result, null, streaming ? 0 : 2)}\n`);
    if (result.status === "failed") process.exitCode = 1;
  } catch (error) {
    process.stdout.write(`${JSON.stringify({ status: "failed", error: error.message }, null, streaming ? 0 : 2)}\n`);
    process.exitCode = 1;
  }
}
