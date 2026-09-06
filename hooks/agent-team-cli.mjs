#!/usr/bin/env node
import { execFile } from "node:child_process";
import { mkdir } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import process from "node:process";
import { promisify } from "node:util";
import { auditEffectiveness } from "./lib/telemetry.mjs";
import { buildArtifacts, checkArtifacts } from "./lib/artifacts.mjs";
import { getHealth } from "./lib/health.mjs";
import { installPackage, uninstallPackage } from "./lib/install.mjs";
import { checkPackage } from "./lib/package-validator.mjs";
import { syncOperationMappingInventory } from "./lib/canonical-state.mjs";
import { resolveProject } from "./lib/project.mjs";

const run = promisify(execFile);

function flags(args) {
  const output = { archive: [] };
  for (let index = 0; index < args.length; index += 1) {
    if (!args[index].startsWith("--")) continue;
    const name = args[index].slice(2);
    const value = args[++index];
    if (name === "archive") output.archive.push(value);
    else output[name] = value;
  }
  return output;
}

async function gitRevision(sourceRoot) {
  const { stdout } = await run("git", ["rev-parse", "HEAD"], { cwd: sourceRoot, encoding: "utf8", timeout: 1500 });
  return stdout.trim();
}

/** Provide one structured command surface for health, audit, install, rollback, and validation. */
export async function runCommand(command, options) {
  const sourceRoot = path.resolve(options.source ?? path.join(import.meta.dirname, ".."));
  const home = path.resolve(options.home ?? os.homedir());
  if (command === "health") return getHealth({
    home,
    ...(options.project ? { projectPath: path.resolve(options.project) } : {}),
  });
  if (command === "audit") return auditEffectiveness({
    logDirectory: path.resolve(options.log ?? path.join(home, ".agent-team-hooks", "logs")),
    trackerPath: options.tracker,
    mistakesPath: options.mistakes,
    maxRecords: Number(options.limit ?? 1000),
  });
  if (command === "install") return installPackage({ sourceRoot, home });
  if (command === "migrate-mappings") {
    if (!options.project || !options.session) throw new Error("migrate-mappings requires --project and --session.");
    const project = await resolveProject(path.resolve(options.project));
    return syncOperationMappingInventory(project, options.session);
  }
  if (["uninstall", "rollback"].includes(command)) return uninstallPackage({ home });
  if (command === "check-package") return checkPackage(sourceRoot);
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
  throw new Error(`Unknown command: ${command ?? "missing"}`);
}

if (import.meta.url === `file://${process.argv[1]}`) {
  try {
    const result = await runCommand(process.argv[2], flags(process.argv.slice(3)));
    process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
    if (result.status === "failed") process.exitCode = 1;
  } catch (error) {
    process.stdout.write(`${JSON.stringify({ status: "failed", error: error.message }, null, 2)}\n`);
    process.exitCode = 1;
  }
}
