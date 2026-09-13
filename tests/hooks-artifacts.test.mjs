import assert from "node:assert/strict";
import { execFile } from "node:child_process";
import { createHash } from "node:crypto";
import { chmod, cp, mkdir, mkdtemp, readFile, rm, stat, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { pathToFileURL } from "node:url";
import { promisify } from "node:util";

import { __artifactTest, buildArtifacts, checkArtifacts, fileMapDigest, readZip, verifyReleaseArtifact, writeZip } from "../hooks/lib/artifacts.mjs";

const sourceRoot = path.resolve(import.meta.dirname, "..");
const releaseRevision = "a".repeat(40);
const run = promisify(execFile);
const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { force: true, recursive: true }))));

async function artifacts() {
  const outputDirectory = await mkdtemp(path.join(os.tmpdir(), "agent-team-artifacts-"));
  temporary.push(outputDirectory);
  const built = await buildArtifacts({ sourceRoot, outputDirectory, sourceRevision: releaseRevision });
  const checksums = path.join(outputDirectory, "SHA256SUMS");
  const digest = createHash("sha256").update(await readFile(built.archives[0])).digest("hex");
  await writeFile(checksums, `${digest}  ${path.basename(built.archives[0])}\n`);
  return { outputDirectory, built, checksums };
}

async function releaseArtifact() {
  const outputDirectory = await mkdtemp(path.join(os.tmpdir(), "agent-team-release-"));
  temporary.push(outputDirectory);
  const built = await buildArtifacts({ sourceRoot, outputDirectory, sourceRevision: "a".repeat(40) });
  const archive = built.archives[0];
  const checksums = path.join(outputDirectory, "SHA256SUMS");
  const digest = createHash("sha256").update(await readFile(archive)).digest("hex");
  await writeFile(checksums, `${digest}  ${path.basename(archive)}\n`);
  return { archive, checksums, digest };
}

test("release artifact authority validates one exact checksum record and immutable closed bytes", async () => {
  const { archive, checksums, digest } = await releaseArtifact();
  const authority = await verifyReleaseArtifact({ archive, checksums });
  assert.equal(authority.archiveSha256, digest);
  assert.equal(authority.metadata.sourceRevision, "a".repeat(40));
  assert.equal(authority.packageContentDigest, fileMapDigest(authority.packageFileMap));
  assert.equal(authority.archiveContentDigest, fileMapDigest(authority.archiveFileMap));
  const mutated = authority.bytes;
  mutated.fill(0);
  assert.notEqual(authority.bytes[0], 0);
  assert.equal(Object.isFrozen(authority.packageFileMap), true);

  const alternateChecksums = path.join(path.dirname(checksums), "checksums.txt");
  await writeFile(alternateChecksums, await readFile(checksums));
  await assert.rejects(verifyReleaseArtifact({ archive, checksums: alternateChecksums }), /checksum_manifest_mismatch/);

  for (const contents of ["", `${digest}  wrong.zip\n`, `${digest}  ${path.basename(archive)}\n${digest}  extra.zip\n`, "malformed\n"]) {
    await writeFile(checksums, contents);
    await assert.rejects(verifyReleaseArtifact({ archive, checksums }), /checksum_manifest_mismatch/);
  }
});

test("release artifact authority rejects checksum, duplicate, unsafe, special, mode, and header drift", async (context) => {
  for (const [name, mutate, reason] of [
    ["checksum", async ({ archive }) => writeFile(archive, Buffer.concat([await readFile(archive), Buffer.from("x")])), /archive_checksum_mismatch|archive_header_mismatch/],
    ["duplicate", async ({ archive, checksums }) => { const entries = await readZip(archive); await __artifactTest.writeZipRaw(archive, [...entries, entries[0]]); const digest = createHash("sha256").update(await readFile(archive)).digest("hex"); await writeFile(checksums, `${digest}  ${path.basename(archive)}\n`); }, /duplicate_archive_entry/],
    ["unsafe", async ({ archive, checksums }) => { const entries = await readZip(archive); entries[0].name = "agent-team/../escape"; await __artifactTest.writeZipRaw(archive, entries); const digest = createHash("sha256").update(await readFile(archive)).digest("hex"); await writeFile(checksums, `${digest}  ${path.basename(archive)}\n`); }, /unsafe_archive_entry/],
    ["special", async ({ archive, checksums }) => { const entries = await readZip(archive); entries[0].mode = 0o120777; await __artifactTest.writeZipRaw(archive, entries); const digest = createHash("sha256").update(await readFile(archive)).digest("hex"); await writeFile(checksums, `${digest}  ${path.basename(archive)}\n`); }, /unsafe_archive_entry/],
    ["mode", async ({ archive, checksums }) => { const entries = await readZip(archive); entries[0].mode = 0o100666; await __artifactTest.writeZipRaw(archive, entries); const digest = createHash("sha256").update(await readFile(archive)).digest("hex"); await writeFile(checksums, `${digest}  ${path.basename(archive)}\n`); }, /archive_mode_mismatch/],
    ["header", async ({ archive, checksums }) => { const entries = await readZip(archive); entries[0].raw = { centralSize: entries[0].data.length + 1 }; await __artifactTest.writeZipRaw(archive, entries); const digest = createHash("sha256").update(await readFile(archive)).digest("hex"); await writeFile(checksums, `${digest}  ${path.basename(archive)}\n`); }, /archive_header_mismatch|archive_size_mismatch/],
  ]) await context.test(name, async () => { const fixture = await releaseArtifact(); await mutate(fixture); await assert.rejects(verifyReleaseArtifact(fixture), reason); });
});

test("release artifact authority rejects missing and malformed closed metadata", async (context) => {
  for (const [name, mutate] of [
    ["missing", (entries) => entries.filter((entry) => entry.name !== "agent-team/.agent-team-source.json")],
    ["unknown field", (entries) => entries.map((entry) => entry.name !== "agent-team/.agent-team-source.json" ? entry : {
      ...entry, data: Buffer.from(`${JSON.stringify({ ...JSON.parse(entry.data), unexpected: true }, null, 2)}\n`),
    })],
    ["revision", (entries) => entries.map((entry) => entry.name !== "agent-team/.agent-team-source.json" ? entry : {
      ...entry, data: Buffer.from(`${JSON.stringify({ ...JSON.parse(entry.data), sourceRevision: "moving" }, null, 2)}\n`),
    })],
  ]) await context.test(name, async () => {
    const fixture = await releaseArtifact();
    await writeZip(fixture.archive, mutate(await readZip(fixture.archive)));
    const digest = createHash("sha256").update(await readFile(fixture.archive)).digest("hex");
    await writeFile(fixture.checksums, `${digest}  ${path.basename(fixture.archive)}\n`);
    await assert.rejects(verifyReleaseArtifact(fixture), /archive_manifest_mismatch/);
  });
});

test("archive path is opened without following a symbolic link", async () => {
  const { archive } = await releaseArtifact();
  const link = `${archive}.link`;
  await import("node:fs/promises").then(({ symlink }) => symlink(archive, link));
  temporary.push(link);
  await assert.rejects(__artifactTest.readArchiveOnce(link), /archive_invalid/);
});

test("source and extracted universal CLIs complete every host and scope lifecycle in isolated targets", async (t) => {
  // Importing the source installer, skipping a selector, or losing update/uninstall effects must break this consumer matrix.
  const { built, checksums } = await artifacts();
  const extracted = await mkdtemp(path.join(os.tmpdir(), "agent team extracted "));
  const updated = await mkdtemp(path.join(os.tmpdir(), "agent team updated extracted "));
  temporary.push(extracted, updated);

  assert.equal(built.archives.length, 1);
  assert.equal(path.basename(built.archives[0]), "agent-team-7.2.1.zip");
  await run("unzip", ["-q", built.archives[0], "-d", extracted]);
  const packageRoot = path.join(extracted, "agent-team");
  const updatedRoot = path.join(updated, "agent-team");
  assert.equal(packageRoot.startsWith(sourceRoot), false);
  await readFile(path.join(packageRoot, "hooks", "codex-hooks.json"));
  await readFile(path.join(packageRoot, "hooks", "claude-hooks.json"));
  assert.notEqual((await stat(path.join(packageRoot, "hooks", "agent-team-cli.mjs"))).mode & 0o111, 0);
  await cp(packageRoot, updatedRoot, { recursive: true });
  const addedFile = "references/extracted-consumer-update.md";
  await writeFile(path.join(updatedRoot, addedFile), "updated extracted consumer\n");
  const manifestPath = path.join(updatedRoot, "hooks", "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  manifest.files.push(addedFile);
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
  const updatedOutput = await mkdtemp(path.join(os.tmpdir(), "agent-team-updated-artifact-"));
  temporary.push(updatedOutput);
  const updatedBuilt = await buildArtifacts({ sourceRoot: updatedRoot, outputDirectory: updatedOutput, sourceRevision: "e".repeat(40) });
  const updatedArchive = updatedBuilt.archives[0];
  const updatedChecksums = path.join(updatedOutput, "SHA256SUMS");
  await writeFile(updatedChecksums, `${createHash("sha256").update(await readFile(updatedArchive)).digest("hex")}  ${path.basename(updatedArchive)}\n`);

  for (const [host, scope] of [
    ["codex", "user"],
    ["claude-code", "user"],
    ["both", "user"],
    ["codex", "project"],
    ["claude-code", "project"],
    ["both", "project"],
  ]) {
    for (const [distribution, entryRoot] of [["source", sourceRoot], ["archive", packageRoot]]) {
      t.diagnostic(`${distribution} ${host} ${scope}`);
      const home = await mkdtemp(path.join(os.tmpdir(), "agent team zip home "));
      const projectRoot = await mkdtemp(path.join(os.tmpdir(), "agent team zip project "));
      temporary.push(home, projectRoot);
      await run("git", ["init", "-q", "-b", "main", projectRoot]);
      const initialization = path.join(projectRoot, "initialization-request.json");
      await writeFile(initialization, JSON.stringify({ schemaVersion: 1, actorSessionId: "zip-owner", expectedVersion: 0,
        request: { projectId: "zip-project", operationId: "zip-initialize", source: "standalone", tracker: { kind: "markdown", path: "TASKS.md" },
          plan: { scope: "Verify the installed package", acceptance: ["Helpers run from the installed package"], branch: "main",
            verification: ["node --test"], authority: { ownedPaths: ["src/**"] }, tasks: [{ id: "ZIP-1", title: "Installed helper qualification", status: "ready" }] } } }));
      const configs = {
        user: {
          codex: path.join(home, ".codex", "hooks.json"),
          claude: path.join(home, ".claude", "settings.json"),
        },
        project: {
          codex: path.join(projectRoot, ".codex", "hooks.json"),
          claude: path.join(projectRoot, ".claude", "settings.local.json"),
        },
      };
      const sharedClaude = path.join(projectRoot, ".claude", "settings.json");
      for (const configPath of [...Object.values(configs.user), ...Object.values(configs.project), sharedClaude]) {
        await mkdir(path.dirname(configPath), { recursive: true });
        await writeFile(configPath, `${JSON.stringify({ sentinel: path.basename(configPath) })}\n`);
      }
      const protectedPaths = [...Object.values(configs.user), ...Object.values(configs.project), sharedClaude]
        .filter((configPath) => !Object.entries(configs[scope]).some(([runtime, selectedPath]) =>
          selectedPath === configPath && (host === "both" || runtime === (host === "claude-code" ? "claude" : host))));
      const protectedBytes = new Map(await Promise.all(protectedPaths.map(async (configPath) => [configPath, await readFile(configPath, "utf8")])));
      const selectors = ["--home", home, "--host", host, "--scope", scope, ...(scope === "project" ? ["--project", projectRoot] : [])];
      const releaseSelectors = ["--archive", built.archives[0], "--checksums", checksums];
      const cli = path.join(entryRoot, "hooks", "agent-team-cli.mjs");
      const updatedCli = path.join(updatedRoot, "hooks", "agent-team-cli.mjs");

      const installed = JSON.parse((await run(process.execPath, [cli, "install", ...releaseSelectors, ...selectors])).stdout);
      assert.equal(installed.validation.status, "passed");
      const reinstalled = JSON.parse((await run(process.execPath, [cli, "install", ...releaseSelectors, ...selectors])).stdout);
      assert.equal(reinstalled.changed, false);
      const upgraded = JSON.parse((await run(process.execPath, [updatedCli, "install", "--archive", updatedArchive, "--checksums", updatedChecksums, ...selectors])).stdout);
      assert.equal(upgraded.status, "update_requires_manual_replacement");
      assert.equal(upgraded.changed, false);
      const runtimes = host === "both" ? ["codex", "claude"] : [host === "claude-code" ? "claude" : "codex"];
      for (const runtime of runtimes) {
        const root = scope === "project" ? projectRoot : home;
        const target = runtime === "codex"
          ? path.join(root, ".agents", "skills", "agent-team")
          : path.join(root, ".claude", "skills", "agent-team");
        await assert.rejects(readFile(path.join(target, addedFile)), { code: "ENOENT" });
        const configured = JSON.parse(await readFile(configs[scope][runtime], "utf8"));
        assert.equal(configured.sentinel, path.basename(configs[scope][runtime]));
        assert.ok(Object.values(configured.hooks).flat().some((group) => group.hooks.some(({ command = "" }) => command.includes("agent-team-hook.mjs"))));
        const installedCli = path.join(target, "hooks", "agent-team-cli.mjs");
        const setupPath = path.join(projectRoot, ".agent-team", "setup.json");
        const beforeShell = await readFile(setupPath).catch((error) => error.code === "ENOENT" ? undefined : Promise.reject(error));
        const shell = JSON.parse((await run(process.execPath, [installedCli, "project-initialize", "--project", projectRoot, "--request", initialization])).stdout);
        assert.deepEqual({ status: shell.status, ready: shell.ready, reason: shell.reason },
          { status: "validated", ready: false, reason: "native_identity_required" });
        assert.deepEqual(await readFile(setupPath).catch((error) => error.code === "ENOENT" ? undefined : Promise.reject(error)), beforeShell);
        const { runCommand: runInstalledCommand } = await import(`${pathToFileURL(installedCli).href}?consumer=${Math.random()}`);
        const selectedHost = runtime === "claude" ? "claude-code" : "codex";
        const initialized = await runInstalledCommand("project-initialize", { project: projectRoot, request: initialization }, {
          nativeIdentity: { host: selectedHost, sessionId: "zip-owner", observed: true, cwd: projectRoot },
        });
        assert.ok(["applied", "duplicate"].includes(initialized.status), JSON.stringify(initialized));
        assert.equal(initialized.canonicalReady, true);
        assert.equal(initialized.ready, false);
        const recoveryRequest = path.join(projectRoot, `owner-recovery-${runtime}.json`);
        await writeFile(recoveryRequest, JSON.stringify({ schemaVersion: 1, request: { operationId: `recover-${runtime}`, projectId: "zip-project",
          expectedOwnerSessionId: "zip-owner", expectedOwnershipEpoch: 1, expectedSetupVersion: 1, expectedStateVersion: 0,
          expectedStateFingerprint: "a".repeat(64), expectedTeamsFingerprint: "b".repeat(64), expectedSetupFingerprint: "c".repeat(64),
          expectedOwnerHistoryFingerprint: "d".repeat(64), reason: "prior_owner_unavailable" } }));
        let recoveryCallbacks = 0;
        const forgedRecovery = await runInstalledCommand("project-owner-recover", { project: projectRoot, request: recoveryRequest }, {
          nativeOwnerRecovery: { identity: { host: selectedHost, sessionId: "forged", invocationId: "inv-forged", projectRoot, worktreeRoot: projectRoot },
            projectIdentity: { root: projectRoot }, capability: Object.freeze({}),
            inspectSession: async () => { recoveryCallbacks += 1; return "stopped"; },
            confirm: async () => { recoveryCallbacks += 1; return { approved: true, approvalId: "forged" }; } },
        });
        assert.deepEqual(forgedRecovery, { status: "validated", ready: false, reason: "native_owner_recovery_required" });
        assert.equal(recoveryCallbacks, 0);
        const overview = JSON.parse((await run(process.execPath, [installedCli, "settings", "--project", projectRoot, "--host", selectedHost, "--scope", "project"])).stdout);
        assert.ok(overview.roles.length > 0);
        const readiness = JSON.parse((await run(process.execPath, [installedCli, "readiness", "--project", projectRoot, "--host", selectedHost, "--scope", scope])).stdout);
        assert.equal(readiness.projectInitialization.required, false);
        assert.equal(readiness.readyForDispatch, false, "extracted files do not prove native dependencies ready");
        const status = JSON.parse((await run(process.execPath, [installedCli, "status", "--project", projectRoot])).stdout);
        assert.deepEqual(status.tasks.map(({ id }) => id), ["ZIP-1"]);
      }
      for (const [configPath, bytes] of protectedBytes) assert.equal(await readFile(configPath, "utf8"), bytes);
      assert.match(await readFile(path.join(projectRoot, "TASKS.md"), "utf8"), /ZIP-1/);

      const uninstalled = JSON.parse((await run(process.execPath, [updatedCli, "uninstall", ...selectors])).stdout);
      assert.equal(uninstalled.status, "uninstalled");
      for (const runtime of runtimes) {
        const root = scope === "project" ? projectRoot : home;
        const target = runtime === "codex"
          ? path.join(root, ".agents", "skills", "agent-team", "SKILL.md")
          : path.join(root, ".claude", "skills", "agent-team", "SKILL.md");
        await assert.rejects(readFile(target), { code: "ENOENT" });
        const configured = JSON.parse(await readFile(configs[scope][runtime], "utf8"));
        assert.equal(configured.sentinel, path.basename(configs[scope][runtime]));
        assert.equal(Object.values(configured.hooks ?? {}).flat().some((group) => group.hooks.some(({ command = "" }) => command.includes("agent-team-hook.mjs"))), false);
      }
      for (const [configPath, bytes] of protectedBytes) assert.equal(await readFile(configPath, "utf8"), bytes);
    }
  }
});

test("artifact validation reports source-only checks as not applicable", async () => {
  // This test catches invented archive success when no archive was supplied.
  const result = await checkArtifacts({ sourceRoot, archives: [], expectedRevision: releaseRevision });
  assert.equal(result.status, "not_applicable");
});

test("archive permissions and validation are independent of checkout write permissions", async () => {
  const fixture = await mkdtemp(path.join(os.tmpdir(), "agent-team-artifact-modes-"));
  temporary.push(fixture);
  await mkdir(path.join(fixture, "hooks"));
  const manifest = JSON.parse(await readFile(path.join(sourceRoot, "hooks", "manifest.json"), "utf8"));
  manifest.files = ["note.txt", "tool.mjs"];
  await writeFile(path.join(fixture, "hooks", "manifest.json"), JSON.stringify(manifest));
  await writeFile(path.join(fixture, "note.txt"), "fixture text\n");
  await writeFile(path.join(fixture, "tool.mjs"), "#!/usr/bin/env node\n");
  const builds = [];
  for (const [label, noteMode, toolMode] of [["standard", 0o644, 0o755], ["group-writable", 0o664, 0o775]]) {
    await chmod(path.join(fixture, "note.txt"), noteMode);
    await chmod(path.join(fixture, "tool.mjs"), toolMode);
    const outputDirectory = path.join(fixture, label);
    await mkdir(outputDirectory);
    const built = await buildArtifacts({ sourceRoot: fixture, outputDirectory, sourceRevision: "mode-fixture" });
    builds.push(built.archives[0]);
    assert.equal((await stat(path.join(fixture, "note.txt"))).mode & 0o777, noteMode);
    assert.equal((await stat(path.join(fixture, "tool.mjs"))).mode & 0o777, toolMode);
  }
  assert.deepEqual(await readFile(builds[0]), await readFile(builds[1]));
  for (const archive of builds) {
    const modes = new Map((await readZip(archive)).map(({ name, mode }) => [name, mode]));
    assert.equal(modes.get("agent-team/note.txt"), 0o100644);
    assert.equal(modes.get("agent-team/tool.mjs"), 0o100755);
    assert.equal(modes.get(manifest.artifacts.metadata), 0o100644);
    const result = await checkArtifacts({ sourceRoot: fixture, archives: [archive], expectedRevision: "mode-fixture" });
    assert.equal(result.status, "passed", result.errors.join("\n"));
  }
});

test("the reproducible universal archive matches the manifest and source", async () => {
  // This test catches platform payload drift and nondeterministic archive output.
  const first = await artifacts();
  const second = await artifacts();
  const result = await checkArtifacts({ sourceRoot, archives: first.built.archives, expectedRevision: releaseRevision });
  const firstBytes = await Promise.all(first.built.archives.map((file) => readZip(file).then((entries) => entries.map(({ name, data }) => [name, data.toString("hex")]))));
  const secondBytes = await Promise.all(second.built.archives.map((file) => readZip(file).then((entries) => entries.map(({ name, data }) => [name, data.toString("hex")]))));
  const firstArchives = await Promise.all(first.built.archives.map((file) => readFile(file)));
  const secondArchives = await Promise.all(second.built.archives.map((file) => readFile(file)));

  assert.equal(result.status, "passed", result.errors.join("\n"));
  assert.deepEqual(firstBytes, secondBytes);
  assert.deepEqual(firstArchives, secondArchives);
});

test("archives identify the canonical repository and pinned release source", async () => {
  const value = await artifacts();
  for (const archive of value.built.archives) {
    const entries = await readZip(archive);
    const source = JSON.parse(entries.find(({ name }) => name === "agent-team/.agent-team-source.json").data.toString("utf8"));
    assert.deepEqual(source.hosts, ["codex", "claude-code"]);
    assert.equal(source.repository, "https://github.com/thebpandey/agent-team");
    assert.deepEqual(Object.keys(source).sort(), ["hosts", "name", "packageContentDigest", "packageFileMap", "releaseTag", "releaseUrl", "repository", "sourceRevision", "updateUrl", "version"]);
    assert.equal(source.name, "agent-team");
    assert.equal(source.version, "7.2.1");
    assert.equal(source.releaseTag, "v7.2.1");
    assert.equal(source.releaseUrl, "https://github.com/thebpandey/agent-team/releases/tag/v7.2.1");
    assert.equal(source.updateUrl, "https://github.com/thebpandey/agent-team/releases/latest");
    assert.equal(source.sourceRevision, releaseRevision);
    assert.match(source.sourceRevision, /^[a-f0-9]{40}$/);
    assert.equal(Object.keys(source.packageFileMap).length, 96);
    assert.equal(entries.length, 97);
  }
});

test("artifact validation rejects stale, omitted, unexpected, duplicate, absolute, and path-escaping entries", async (context) => {
  // This table protects every archive boundary named by the package contract.
  const cases = [
    ["stale", (entries) => entries.map((entry) => entry.name === "agent-team/SKILL.md" ? { ...entry, data: Buffer.from("stale") } : entry)],
    ["omitted", (entries) => entries.filter((entry) => entry.name !== "agent-team/SKILL.md")],
    ["unexpected", (entries) => [...entries, { name: "agent-team/unexpected.txt", data: Buffer.from("unexpected") }]],
    ["duplicate", (entries) => [...entries, { ...entries[0] }]],
    ["entrypoint mode", (entries) => entries.map((entry) => entry.name === "agent-team/hooks/agent-team-cli.mjs" ? { ...entry, mode: 0o100644 } : entry)],
    ["absolute", (entries) => [...entries, { name: "/absolute.txt", data: Buffer.from("unsafe") }]],
    ["path escape", (entries) => [...entries, { name: "../escape.txt", data: Buffer.from("unsafe") }]],
  ];

  for (const [name, mutate] of cases) {
    await context.test(name, async () => {
      const value = await artifacts();
      const archive = value.built.archives[0];
      await writeZip(archive, mutate(await readZip(archive)));
      const result = await checkArtifacts({ sourceRoot, archives: value.built.archives, expectedRevision: releaseRevision });
      assert.equal(result.status, "failed");
    });
  }
});
