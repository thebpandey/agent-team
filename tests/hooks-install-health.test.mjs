import assert from "node:assert/strict";
import { execFile, spawn } from "node:child_process";
import { createHash } from "node:crypto";
import { chmod, lstat, mkdir, mkdtemp, readFile, readdir, rename, rm, symlink, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { setTimeout as delay } from "node:timers/promises";
import { promisify } from "node:util";

import { getHealth, resolveHookEvidenceRoot } from "../hooks/lib/health.mjs";
import { __installTest, installPackage as installReleasePackage, uninstallPackage } from "../hooks/lib/install.mjs";
import { buildArtifacts, fileMapDigest } from "../hooks/lib/artifacts.mjs";
import { appendActivationLog } from "../hooks/lib/telemetry.mjs";
import { copyTrackedSource } from "./hook-test-helpers.mjs";

const sourceRoot = path.resolve(import.meta.dirname, "..");
const cli = path.join(sourceRoot, "hooks", "agent-team-cli.mjs");
const run = promisify(execFile);
const temporary = [];
let sharedArtifact;
let sharedArtifactDirectory;
test.afterEach(async () => {
  await Promise.all(temporary.splice(0).map((item) => rm(item, { force: true, recursive: true })));
});
test.after(async () => { if (sharedArtifactDirectory) await rm(sharedArtifactDirectory, { force: true, recursive: true }); });

async function homeFixture() {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-home-"));
  temporary.push(home);
  await mkdir(path.join(home, ".codex", "skills", "agent-team"), { recursive: true });
  await mkdir(path.join(home, ".claude"), { recursive: true });
  await writeFile(path.join(home, ".codex", "skills", "agent-team", "legacy.txt"), "legacy\n");
  await writeFile(path.join(home, ".codex", "hooks.json"), JSON.stringify({ hooks: {
    Stop: [{ hooks: [{ type: "command", command: "unrelated-stop" }] }],
    SessionStart: [{ hooks: [{ type: "command", command: "lean-ctx hook codex-session-start" }] }],
  } }));
  await writeFile(path.join(home, ".claude", "settings.json"), JSON.stringify({ model: "fable", hooks: {
    PostToolUse: [{ matcher: "Write", hooks: [{ type: "command", command: "unrelated-write" }] }],
    PreToolUse: [{ matcher: "Read", hooks: [{ type: "command", command: "lean-ctx hook redirect" }] }],
  } }));
  return home;
}

function agentTeamGroups(config) {
  return Object.values(config.hooks ?? {}).flat().filter((group) => group.hooks?.some(({ command = "" }) => command.includes("agent-team-hook.mjs")));
}

function fileGuard(contents) {
  return { kind: "file", digest: createHash("sha256").update(contents).digest("hex") };
}

async function archiveFixture(root = sourceRoot, revision = "b".repeat(40), persistent = false) {
  const outputDirectory = await mkdtemp(path.join(os.tmpdir(), "agent-team-install-artifact-"));
  if (persistent) sharedArtifactDirectory = outputDirectory;
  else temporary.push(outputDirectory);
  const { archives: [archive] } = await buildArtifacts({ sourceRoot: root, outputDirectory, sourceRevision: revision });
  const checksums = path.join(outputDirectory, "SHA256SUMS");
  await writeFile(checksums, `${createHash("sha256").update(await readFile(archive)).digest("hex")}  ${path.basename(archive)}\n`);
  return { archive, checksums };
}

async function installPackage(input) {
  if (input.archive) return installReleasePackage(input);
  const { sourceRoot: root, ...options } = input;
  const artifact = root === sourceRoot ? sharedArtifact ??= archiveFixture(root, "b".repeat(40), true) : archiveFixture(root);
  return installReleasePackage({ ...await artifact, ...options });
}

test("artifact install writes schema 4 provenance and remains verifiable after temporary inputs disappear", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-sealed-install-"));
  temporary.push(home);
  const artifact = await archiveFixture();
  const result = await installPackage({ ...artifact, home, host: "both", scope: "user" });
  assert.equal(result.status, "installed");
  const receiptPath = path.join(home, ".agent-team-hooks", "install.json");
  const before = await readFile(receiptPath);
  const receipt = JSON.parse(before);
  assert.equal(receipt.schemaVersion, 4);
  assert.equal(Object.hasOwn(receipt, "sourceRoot"), false);
  assert.equal(receipt.artifact.sourceRevision, "b".repeat(40));
  assert.deepEqual(Object.keys(receipt.installedFileMaps).sort(), ["claude", "codex"]);
  const expectedNames = Object.keys(receipt.artifact.archiveFileMap).sort();
  assert.equal(expectedNames.includes(".agent-team-source.json"), true);
  for (const runtime of ["codex", "claude"]) {
    const target = path.join(home, runtime === "codex" ? ".agents" : ".claude", "skills", "agent-team");
    const installed = receipt.installedFileMaps[runtime];
    const targetReceipt = receipt.targets.find((entry) => entry.runtime === runtime);
    assert.deepEqual(installed.files, receipt.artifact.archiveFileMap);
    assert.deepEqual([...targetReceipt.files].sort(), expectedNames);
    const metadataBytes = await readFile(path.join(target, ".agent-team-source.json"));
    const metadataStat = await lstat(path.join(target, ".agent-team-source.json"));
    assert.deepEqual({
      sha256: createHash("sha256").update(metadataBytes).digest("hex"),
      mode: metadataStat.mode & 0o777,
      size: metadataStat.size,
    }, receipt.artifact.archiveFileMap[".agent-team-source.json"]);
    const metadata = JSON.parse(metadataBytes);
    assert.deepEqual(metadata.packageFileMap, receipt.artifact.packageFileMap);
    assert.equal(Object.hasOwn(metadata.packageFileMap, ".agent-team-source.json"), false);
  }
  await rm(path.dirname(artifact.archive), { recursive: true });
  const health = await getHealth({ home });
  assert.equal(health.runtimes.codex.artifact.status, "current");
  const again = await installPackage({ ...await archiveFixture(), home, host: "both", scope: "user" });
  assert.equal(again.changed, false);
  assert.deepEqual(await readFile(receiptPath), before);
  await uninstallPackage({ home, host: "both", scope: "user" });
  await assert.rejects(readFile(path.join(home, ".agents", "skills", "agent-team", ".agent-team-source.json")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(home, ".claude", "skills", "agent-team", ".agent-team-source.json")), { code: "ENOENT" });
});

test("present-target preflight rejects root and nested links and special entries before both-host mutation", async (context) => {
  const cases = [
    ["root symlink", async (target, outside) => {
      await rename(target, outside);
      await symlink(outside, target, "dir");
    }],
    ["nested symlink", async (target, outside) => {
      await mkdir(outside);
      await symlink(outside, path.join(target, "operator-link"), "dir");
    }],
    ["fifo", async (target) => {
      await run("mkfifo", [path.join(target, "operator-fifo")]);
    }],
  ];
  for (const [label, addUnsafeEntry] of cases) await context.test(label, async () => {
    const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-unsafe-present-target-"));
    temporary.push(home);
    const artifact = await archiveFixture();
    await installPackage({ ...artifact, home, host: "codex", scope: "user" });
    const target = path.join(home, ".agents", "skills", "agent-team");
    const outside = path.join(home, "operator-owned-target");
    const receiptPath = path.join(home, ".agent-team-hooks", "install.json");
    const configPath = path.join(home, ".codex", "hooks.json");
    const priorReceipt = await readFile(receiptPath);
    const priorConfig = await readFile(configPath);
    const priorBackups = await readdir(path.join(home, ".agent-team-hooks", "backups"), { recursive: true }).catch((error) => error.code === "ENOENT" ? [] : Promise.reject(error));
    await addUnsafeEntry(target, outside);

    const result = await installPackage({ ...artifact, home, host: "both", scope: "user" });

    assert.equal(result.status, "update_requires_manual_replacement");
    assert.equal(result.changed, false);
    assert.equal(result.conflicts.some(({ runtime, target: conflictTarget, reason }) => runtime === "codex"
      && conflictTarget === target && reason === "differing_present_target"), true);
    assert.deepEqual(await readFile(receiptPath), priorReceipt);
    assert.deepEqual(await readFile(configPath), priorConfig);
    assert.deepEqual(await readdir(path.join(home, ".agent-team-hooks", "backups"), { recursive: true }).catch((error) => error.code === "ENOENT" ? [] : Promise.reject(error)), priorBackups);
    await assert.rejects(readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
    await assert.rejects(readFile(path.join(home, ".claude", "agents", "agent-team-developer.md")), { code: "ENOENT" });
    if (label === "root symlink") {
      assert.equal((await lstat(target)).isSymbolicLink(), true);
      assert.equal(await readFile(path.join(target, "SKILL.md"), "utf8").then(Boolean), true);
    } else {
      const unsafe = path.join(target, label === "fifo" ? "operator-fifo" : "operator-link");
      assert.equal(label === "fifo" ? (await lstat(unsafe)).isFIFO() : (await lstat(unsafe)).isSymbolicLink(), true);
    }
  });
});

test("artifact replacement after verification cannot change sealed install bytes", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-opened-artifact-"));
  temporary.push(home);
  const artifact = await archiveFixture();
  const expected = await readFile(path.join(sourceRoot, "SKILL.md"));
  const result = await __installTest.installPackage({ ...artifact, home, host: "codex", scope: "user" }, {
    afterArchiveVerified: async () => {
      await writeFile(artifact.archive, "replaced archive");
      await writeFile(artifact.checksums, "replaced checksums\n");
    },
  });
  assert.equal(result.status, "installed");
  assert.deepEqual(await readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md")), expected);
});

test("both-host artifact install rolls back the first swap and prior receipt on injected failure", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-both-rollback-"));
  temporary.push(home);
  const artifact = await archiveFixture();
  await assert.rejects(__installTest.installPackage({ ...artifact, home, host: "both", scope: "user" }, { failAfterFirstSwap: true }), /injected_after_first_swap/);
  await assert.rejects(readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(home, ".agent-team-hooks", "install.json")), { code: "ENOENT" });
});

test("staged artifact drift between host swaps aborts and rolls both hosts back", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-staged-drift-"));
  temporary.push(home);
  const artifact = await archiveFixture();
  await assert.rejects(__installTest.installPackage({ ...artifact, home, host: "both", scope: "user" }, {
    beforeTargetSwap: async ({ index }) => {
      if (index !== 1) return;
      const sealed = (await readdir(path.join(home, ".agent-team-hooks"))).find((name) => name.startsWith(".agent-team-sealed-"));
      await writeFile(path.join(home, ".agent-team-hooks", sealed, "agent-team", "SKILL.md"), "tampered staging\n");
    },
  }), /staged_file_map_mismatch/);
  await assert.rejects(readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
});

test("both-host publication preserves a concurrently appeared second target and rolls back the first", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-target-race-"));
  temporary.push(home);
  const artifact = await archiveFixture();
  const raced = path.join(home, ".claude", "skills", "agent-team");
  let identity;
  await assert.rejects(__installTest.installPackage({ ...artifact, home, host: "both", scope: "user" }, {
    beforeTargetSwap: async ({ index }) => {
      if (index !== 1) return;
      await mkdir(raced, { recursive: true });
      const metadata = await import("node:fs/promises").then(({ lstat }) => lstat(raced));
      identity = { dev: metadata.dev, ino: metadata.ino };
    },
  }), { code: "EEXIST" });
  const after = await import("node:fs/promises").then(({ lstat }) => lstat(raced));
  assert.deepEqual({ dev: after.dev, ino: after.ino }, identity);
  assert.deepEqual(await readdir(raced), []);
  await assert.rejects(readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
});

test("a differing present owned target requires manual replacement before any mutation", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-owned-update-manual-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-owned-update-source-"));
  temporary.push(home, changedSource);
  await copyTrackedSource(sourceRoot, changedSource);
  const target = path.join(home, ".agents", "skills", "agent-team");
  const receiptPath = path.join(home, ".agent-team-hooks", "install.json");
  const configPath = path.join(home, ".codex", "hooks.json");
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  const priorReceipt = await readFile(receiptPath);
  const priorConfig = await readFile(configPath);
  const priorSkill = await readFile(path.join(target, "SKILL.md"));
  const priorIdentity = await lstat(target);
  const backupRoot = path.join(home, ".agent-team-hooks", "backups");
  const priorBackups = await readdir(backupRoot, { recursive: true }).catch((error) => error.code === "ENOENT" ? [] : Promise.reject(error));
  await writeFile(path.join(changedSource, "README.md"), "changed release bytes\n");
  const artifact = await archiveFixture(changedSource);
  let mutationHookCalls = 0;

  const result = await __installTest.installPackage({ ...artifact, home, host: "both", scope: "user" }, {
    beforeTargetSwap: async () => { mutationHookCalls += 1; },
    afterTargetPrecheck: async () => { mutationHookCalls += 1; },
  });

  assert.equal(result.status, "update_requires_manual_replacement");
  assert.equal(result.changed, false);
  assert.equal(mutationHookCalls, 0);
  const afterIdentity = await lstat(target);
  assert.deepEqual({ dev: afterIdentity.dev, ino: afterIdentity.ino }, { dev: priorIdentity.dev, ino: priorIdentity.ino });
  assert.deepEqual(await readFile(path.join(target, "SKILL.md")), priorSkill);
  assert.deepEqual(await readFile(configPath), priorConfig);
  assert.deepEqual(await readFile(receiptPath), priorReceipt);
  assert.deepEqual(await readdir(backupRoot, { recursive: true }).catch((error) => error.code === "ENOENT" ? [] : Promise.reject(error)), priorBackups);
  await assert.rejects(readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(home, ".claude", "agents", "agent-team-developer.md")), { code: "ENOENT" });
});

test("a differing schema 3 target requires manual replacement without fabricating an update", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-schema3-update-manual-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-schema3-update-source-"));
  temporary.push(home, changedSource);
  await copyTrackedSource(sourceRoot, changedSource);
  const target = path.join(home, ".agents", "skills", "agent-team");
  const receiptPath = path.join(home, ".agent-team-hooks", "install.json");
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  const receipt = JSON.parse(await readFile(receiptPath));
  await writeFile(receiptPath, `${JSON.stringify({ ...receipt, schemaVersion: 3, artifact: undefined, installedFileMaps: undefined }, null, 2)}\n`);
  const priorReceipt = await readFile(receiptPath);
  const configPath = path.join(home, ".codex", "hooks.json");
  const priorConfig = await readFile(configPath);
  const priorIdentity = await lstat(target);
  const priorSkill = await readFile(path.join(target, "SKILL.md"));
  await writeFile(path.join(changedSource, "README.md"), "changed schema3 release bytes\n");

  const result = await installPackage({ sourceRoot: changedSource, home, host: "codex", scope: "user" });

  assert.equal(result.status, "update_requires_manual_replacement");
  assert.equal(result.changed, false);
  const afterIdentity = await lstat(target);
  assert.deepEqual({ dev: afterIdentity.dev, ino: afterIdentity.ino }, { dev: priorIdentity.dev, ino: priorIdentity.ino });
  assert.deepEqual(await readFile(path.join(target, "SKILL.md")), priorSkill);
  assert.deepEqual(await readFile(configPath), priorConfig);
  assert.deepEqual(await readFile(receiptPath), priorReceipt);
});

test("pre-receipt failure restores packages roles configs and prior receipt", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-pre-receipt-"));
  temporary.push(home);
  const artifact = await archiveFixture();
  await assert.rejects(__installTest.installPackage({ ...artifact, home, host: "both", scope: "user" }, { failBeforeReceipt: true }), /injected_before_receipt/);
  for (const file of [
    path.join(home, ".agents", "skills", "agent-team", "SKILL.md"),
    path.join(home, ".claude", "skills", "agent-team", "SKILL.md"),
    path.join(home, ".codex", "hooks.json"),
    path.join(home, ".claude", "settings.json"),
    path.join(home, ".agent-team-hooks", "install.json"),
  ]) await assert.rejects(readFile(file), { code: "ENOENT" });
});

test("health reports artifact mode drift and labels schema 3 provenance unverified legacy", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-artifact-health-"));
  temporary.push(home);
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  const target = path.join(home, ".agents", "skills", "agent-team");
  await chmod(path.join(target, "SKILL.md"), 0o600);
  assert.equal((await getHealth({ home })).runtimes.codex.artifact.status, "drifted");
  const receiptPath = path.join(home, ".agent-team-hooks", "install.json");
  const receipt = JSON.parse(await readFile(receiptPath));
  await writeFile(receiptPath, `${JSON.stringify({ schemaVersion: 3, targets: receipt.targets }, null, 2)}\n`);
  assert.equal((await getHealth({ home })).runtimes.codex.artifact.status, "unverified_legacy");
});

test("schema 4 health rejects forged version target and installed-map receipt chains", async (context) => {
  const cases = [
    ["version", (receipt) => { receipt.version = "0.0.0"; }],
    ["target", (receipt, target) => { receipt.installedFileMaps.codex.target = `${target}-forged`; }],
    ["map", (receipt) => { receipt.installedFileMaps.codex.files = {}; }],
    ["semver", (receipt) => {
      const artifact = receipt.artifact;
      receipt.version = artifact.version = "01.2.3";
      artifact.releaseTag = `v${artifact.version}`;
      artifact.archiveName = `agent-team-${artifact.version}.zip`;
      artifact.releaseUrl = `${artifact.repository}/releases/tag/${artifact.releaseTag}`;
      artifact.archiveUrl = `${artifact.repository}/releases/download/${artifact.releaseTag}/${artifact.archiveName}`;
      artifact.checksumUrl = `${artifact.repository}/releases/download/${artifact.releaseTag}/${artifact.checksumFileName}`;
    }],
    ["checksum digest", (receipt) => { receipt.artifact.checksumFileSha256 = "not-a-sha256"; }],
    ["archive metadata", (receipt) => {
      receipt.artifact.archiveFileMap[".agent-team-source.json"] = { sha256: "d".repeat(64), mode: 0o644, size: 1 };
      receipt.artifact.archiveContentDigest = fileMapDigest(receipt.artifact.archiveFileMap);
    }],
    ["empty maps", (receipt) => {
      const artifact = receipt.artifact;
      artifact.packageFileMap = {};
      artifact.packageContentDigest = fileMapDigest({});
      const metadata = { name: artifact.name, version: artifact.version, hosts: ["codex", "claude-code"],
        repository: artifact.repository, releaseTag: artifact.releaseTag, releaseUrl: artifact.releaseUrl,
        updateUrl: artifact.updateUrl, sourceRevision: artifact.sourceRevision, packageFileMap: {},
        packageContentDigest: artifact.packageContentDigest };
      const bytes = Buffer.from(`${JSON.stringify(metadata, null, 2)}\n`);
      artifact.archiveFileMap = { ".agent-team-source.json": {
        sha256: createHash("sha256").update(bytes).digest("hex"), mode: 0o644, size: bytes.length,
      } };
      artifact.archiveContentDigest = fileMapDigest(artifact.archiveFileMap);
      receipt.installedFileMaps.codex.files = {};
      receipt.installedFileMaps.codex.digest = fileMapDigest({});
      receipt.targets.find(({ runtime }) => runtime === "codex").files = [];
    }],
    ["joint identity", (receipt, target) => {
      const artifact = receipt.artifact;
      artifact.name = "agent-team-shadow";
      artifact.repository = "https://example.test/agent-team-shadow";
      artifact.releaseUrl = `${artifact.repository}/releases/tag/${artifact.releaseTag}`;
      artifact.updateUrl = `${artifact.repository}/releases/latest`;
      artifact.archiveName = `agent-team-shadow-${artifact.version}.zip`;
      artifact.archiveUrl = `${artifact.repository}/releases/download/${artifact.releaseTag}/${artifact.archiveName}`;
      artifact.checksumFileName = "SHADOWSUMS";
      artifact.checksumUrl = `${artifact.repository}/releases/download/${artifact.releaseTag}/${artifact.checksumFileName}`;
      const targetReceipt = receipt.targets.find((entry) => entry.path === target);
      targetReceipt.digest = "c".repeat(64);
      targetReceipt.mode = "shadow";
      targetReceipt.files.reverse();
    }],
  ];
  for (const [name, forge] of cases) await context.test(name, async () => {
    const home = await mkdtemp(path.join(os.tmpdir(), `agent-team-artifact-chain-${name}-`));
    temporary.push(home);
    await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
    const target = path.join(home, ".agents", "skills", "agent-team");
    const receiptPath = path.join(home, ".agent-team-hooks", "install.json");
    const receipt = JSON.parse(await readFile(receiptPath, "utf8"));
    forge(receipt, target);
    await writeFile(receiptPath, `${JSON.stringify(receipt, null, 2)}\n`);
    assert.equal((await getHealth({ home })).runtimes.codex.artifact.status, "drifted");
    assert.equal(await resolveHookEvidenceRoot(target, "codex"), null);
  });
});

test("post-install file-map hook receives only a label and cannot replace authoritative output", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-post-map-label-"));
  temporary.push(home);
  const artifact = await archiveFixture();
  const calls = [];
  const result = await __installTest.installPackage({ ...artifact, home, host: "codex", scope: "user" }, {
    afterInstalledFileMap: async (label) => {
      calls.push(label);
      return {};
    },
  });
  assert.equal(result.status, "installed");
  assert.deepEqual(calls, [{ runtime: "codex" }]);
  assert.equal((await getHealth({ home })).runtimes.codex.artifact.status, "current");
});

test("post-install callback mutation preserves same-inode operator bytes and durable recovery truth", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-post-map-mutation-"));
  temporary.push(home);
  const configPath = path.join(home, ".codex", "hooks.json");
  await mkdir(path.dirname(configPath), { recursive: true });
  await writeFile(configPath, '{"sentinel":"preimage"}\n');
  const configPreimage = await readFile(configPath);
  const artifact = await archiveFixture();
  const codexTarget = path.join(home, ".agents", "skills", "agent-team");
  await assert.rejects(__installTest.installPackage({ ...artifact, home, host: "both", scope: "user" }, {
    afterInstalledFileMap: async ({ runtime }) => {
      if (runtime === "codex") await writeFile(path.join(codexTarget, "SKILL.md"), "callback mutation\n");
    },
  }), /installed_file_map_mismatch/);
  assert.equal(await readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md"), "utf8"), "callback mutation\n");
  await assert.rejects(readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(home, ".agent-team-hooks", "install.json")), { code: "ENOENT" });
  assert.deepEqual(await readFile(configPath), configPreimage);
  const journal = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "transaction.json"), "utf8"));
  assert.equal(journal.status, "recovery_conflicts");
  assert.equal(journal.recoveryConflicts.some(({ target }) => target === codexTarget), true);
  assert.ok((await readdir(path.join(home, ".agent-team-hooks", "backups"))).length > 0);
});

test("artifact installer denies semantic downgrade without changing the current receipt", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-downgrade-"));
  const newer = await mkdtemp(path.join(os.tmpdir(), "agent-team-newer-source-"));
  temporary.push(home, newer);
  await copyTrackedSource(sourceRoot, newer);
  const manifestPath = path.join(newer, "hooks", "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath));
  manifest.version = "7.2.0";
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);
  await installPackage({ sourceRoot: newer, home, host: "codex", scope: "user" });
  const receiptPath = path.join(home, ".agent-team-hooks", "install.json");
  const before = await readFile(receiptPath);
  const denied = await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  assert.equal(denied.status, "downgrade_denied");
  assert.deepEqual(await readFile(receiptPath), before);
});

test("target resolution rejects missing or ambiguous metadata and never guesses both", async () => {
  // Accepting no host or an ambiguous metadata value would silently configure an unauthorized host.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-target-home-"));
  temporary.push(home);
  await assert.rejects(
    installPackage({ sourceRoot, home, scope: "user" }),
    /host is required/i,
  );
  await assert.rejects(
    installPackage({ sourceRoot, home, trustedHost: "both", scope: "user" }),
    /ambiguous|explicit/i,
  );
  await assert.rejects(
    installPackage({ sourceRoot, home, host: "codex", scope: "user", projectRoot: path.join(home, "ignored-project") }),
    /projectRoot.*user scope|ineffective/i,
  );
  await assert.rejects(installReleasePackage({ sourceRoot, home, host: "codex", scope: "user" }), /Unsupported install authority/);
});

test("installer preserves an unreceipted custom skill directory and does not register its hooks", async () => {
  // Replacing an existing target merely because it has no receipt would overwrite an unowned resource.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-unowned-skill-"));
  temporary.push(home);
  const target = path.join(home, ".agents", "skills", "agent-team");
  await mkdir(target, { recursive: true });
  await writeFile(path.join(target, "CUSTOM.md"), "unowned\n");

  const result = await installPackage({ sourceRoot, home, host: "codex", scope: "user" });

  assert.equal(await readFile(path.join(target, "CUSTOM.md"), "utf8"), "unowned\n");
  assert.equal(result.status, "update_requires_manual_replacement");
  assert.equal(result.conflicts.some(({ kind, reason }) => kind === "skill" && reason === "differing_present_target"), true);
  await assert.rejects(readFile(path.join(home, ".codex", "hooks.json")), { code: "ENOENT" });
  const uninstalled = await uninstallPackage({ home, host: "codex", scope: "user" });
  assert.equal(uninstalled.changed, false);
  assert.deepEqual(uninstalled.conflicts, []);
  assert.equal(await readFile(path.join(target, "CUSTOM.md"), "utf8"), "unowned\n");
});

test("package install and uninstall never claim or alter reused unowned dependency paths", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-unowned-dependencies-"));
  temporary.push(home);
  const skill = path.join(home, ".agents", "skills", "lean-ctx", "SKILL.md");
  const graphify = path.join(home, ".agent-team", "tools", "bin", "graphify");
  await mkdir(path.dirname(skill), { recursive: true });
  await mkdir(path.dirname(graphify), { recursive: true });
  await writeFile(skill, "exact unowned skill\n");
  await writeFile(graphify, "exact unowned executable\n", { mode: 0o755 });
  const before = { skill: await readFile(skill), graphify: await readFile(graphify) };

  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  const receipt = await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8");
  assert.ok(!receipt.includes(skill));
  assert.ok(!receipt.includes(graphify));
  await uninstallPackage({ home, host: "codex", scope: "user" });

  assert.deepEqual(await readFile(skill), before.skill);
  assert.deepEqual(await readFile(graphify), before.graphify);
});

test("installer uses but never claims an identical unreceipted skill directory", async () => {
  // Content equality is not ownership authority; uninstall must retain a package that predates its receipt.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-identical-skill-"));
  temporary.push(home);
  const target = path.join(home, ".agents", "skills", "agent-team");
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  await rm(path.join(home, ".agent-team-hooks"), { recursive: true });
  await rm(path.join(home, ".codex", "hooks.json"));

  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  const receipt = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8"));
  assert.equal(receipt.targets.find(({ runtime }) => runtime === "codex").preexisting, true);

  await uninstallPackage({ home, host: "codex", scope: "user" });
  await readFile(path.join(target, "SKILL.md"));
});

test("a changed release requires manual replacement for a package recorded as pre-existing", async () => {
  // A later release must not convert a content-identical unowned package into replaceable managed state.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-preexisting-skill-update-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-preexisting-skill-source-"));
  temporary.push(home, changedSource);
  await copyTrackedSource(sourceRoot, changedSource);
  const target = path.join(home, ".agents", "skills", "agent-team");
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  await rm(path.join(home, ".agent-team-hooks"), { recursive: true });
  await rm(path.join(home, ".codex", "hooks.json"));
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  const addedFile = "references/preexisting-update-marker.md";
  await writeFile(path.join(changedSource, addedFile), "new release\n");
  const manifestPath = path.join(changedSource, "hooks", "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  manifest.files.push(addedFile);
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  const result = await installPackage({ sourceRoot: changedSource, home, host: "codex", scope: "user" });
  assert.equal(result.status, "update_requires_manual_replacement");
  assert.equal(result.conflicts.some(({ kind, reason }) => kind === "skill" && reason === "differing_present_target"), true);
  await assert.rejects(readFile(path.join(target, addedFile)), { code: "ENOENT" });
  await uninstallPackage({ home, host: "codex", scope: "user" });
  await readFile(path.join(target, "SKILL.md"));
});

test("installer never claims an identical pre-existing Claude role", async () => {
  // An identical role file that predates installation must survive uninstall.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-identical-role-"));
  temporary.push(home);
  const source = path.join(sourceRoot, "assets", "claude-agents", "agent-team-developer.md");
  const target = path.join(home, ".claude", "agents", "agent-team-developer.md");
  await mkdir(path.dirname(target), { recursive: true });
  const original = await readFile(source, "utf8");
  await writeFile(target, original);

  await installPackage({ sourceRoot, home, host: "claude-code", scope: "user" });
  const receipt = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8"));
  assert.equal(receipt.claudeAgents.find(({ path: name }) => name === target).preexisting, true);

  await uninstallPackage({ home, host: "claude-code", scope: "user" });
  assert.equal(await readFile(target, "utf8"), original);
  await assert.rejects(readFile(path.join(home, ".claude", "agents", "agent-team-reviewer.md")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
  const remaining = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8"));
  assert.deepEqual(remaining.claudeAgents.map(({ path: name }) => name), [target]);
});

test("a changed release leaves a pre-existing Claude role untouched pending manual replacement", async () => {
  // A changed release role cannot overwrite an identical role that existed before installation.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-preexisting-role-update-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-preexisting-role-source-"));
  temporary.push(home, changedSource);
  await copyTrackedSource(sourceRoot, changedSource);
  const target = path.join(home, ".claude", "agents", "agent-team-developer.md");
  await mkdir(path.dirname(target), { recursive: true });
  const original = await readFile(path.join(sourceRoot, "assets", "claude-agents", "agent-team-developer.md"), "utf8");
  await writeFile(target, original);
  await installPackage({ sourceRoot, home, host: "claude-code", scope: "user" });
  const changedRole = path.join(changedSource, "assets", "claude-agents", "agent-team-developer.md");
  await writeFile(changedRole, `${original}\nchanged release\n`);

  const result = await installPackage({ sourceRoot: changedSource, home, host: "claude-code", scope: "user" });
  assert.equal(result.status, "update_requires_manual_replacement");
  assert.equal(result.conflicts.some(({ kind, reason }) => kind === "skill" && reason === "differing_present_target"), true);
  assert.equal(await readFile(target, "utf8"), original);
});

test("source installs configure only the selected host and scope, including paths with spaces", async (context) => {
  // A wrong target path, unquoted project path, or implicit second-host install must break this matrix.
  const cases = [
    ["codex", "user"],
    ["claude-code", "user"],
    ["both", "user"],
    ["codex", "project"],
    ["claude-code", "project"],
    ["both", "project"],
  ];
  for (const [host, scope] of cases) {
    await context.test(`${host} ${scope}`, async () => {
      const home = await mkdtemp(path.join(os.tmpdir(), "agent team selected home "));
      const projectRoot = await mkdtemp(path.join(os.tmpdir(), "agent team selected project "));
      temporary.push(home, projectRoot);
      const paths = {
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
      for (const pair of Object.values(paths)) {
        for (const configPath of Object.values(pair)) {
          await mkdir(path.dirname(configPath), { recursive: true });
          await writeFile(configPath, '{"sentinel":"unchanged"}\n');
        }
      }
      await writeFile(sharedClaude, '{"shared":"untouched"}\n');
      const before = new Map(await Promise.all(
        [...Object.values(paths.user), ...Object.values(paths.project), sharedClaude]
          .map(async (configPath) => [configPath, await readFile(configPath, "utf8")]),
      ));

      const result = await installPackage({ sourceRoot, home, host, scope, ...(scope === "project" ? { projectRoot } : {}) });
      const selectedRuntimes = host === "both" ? ["codex", "claude"] : [host === "claude-code" ? "claude" : host];
      assert.deepEqual(result.selection, { host, scope, ...(scope === "project" ? { projectRoot } : {}) });
      assert.deepEqual(result.configured.map(({ runtime, trust }) => [runtime, trust]), selectedRuntimes.map((runtime) => [runtime, "required"]));
      for (const [candidateScope, pair] of Object.entries(paths)) {
        for (const [runtime, configPath] of Object.entries(pair)) {
          if (candidateScope === scope && selectedRuntimes.includes(runtime)) {
            const config = JSON.parse(await readFile(configPath, "utf8"));
            assert.ok(agentTeamGroups(config).length > 0);
            const commands = agentTeamGroups(config).flatMap((group) => group.hooks.map(({ command }) => command));
            if (scope === "project" && runtime === "codex") assert.ok(commands.every((command) => command.includes("git rev-parse --show-toplevel")));
            if (scope === "project" && runtime === "claude") assert.ok(commands.every((command) => command.includes("${CLAUDE_PROJECT_DIR}")));
          } else {
            assert.equal(await readFile(configPath, "utf8"), before.get(configPath));
          }
        }
      }
      assert.equal(await readFile(sharedClaude, "utf8"), before.get(sharedClaude));
    });
  }
});

test("a changed handler release requires manual package replacement and preserves customization", async () => {
  // Whole-group ownership or substring matching would delete the sibling or overwrite the customized command.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-handler-home-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-handler-source-"));
  temporary.push(home, changedSource);
  await copyTrackedSource(sourceRoot, changedSource);
  const args = { home, host: "codex", scope: "user" };
  await installPackage({ sourceRoot, ...args, now: new Date("2026-09-06T12:00:00.000Z") });

  const configPath = path.join(home, ".codex", "hooks.json");
  const config = JSON.parse(await readFile(configPath, "utf8"));
  const group = config.hooks.PreToolUse.find((entry) => agentTeamGroups({ hooks: { PreToolUse: [entry] } }).length);
  group.label = "preserve-this-group-metadata";
  group.hooks.push({ type: "command", command: "unrelated-policy-check", timeout: 19 });
  await writeFile(configPath, `${JSON.stringify(config, null, 2)}\n`);
  const beforeUpdate = await readFile(configPath);
  const receiptPath = path.join(home, ".agent-team-hooks", "install.json");
  const receiptBeforeUpdate = await readFile(receiptPath);

  const declarationPath = path.join(changedSource, "hooks", "codex-hooks.json");
  const declaration = JSON.parse(await readFile(declarationPath, "utf8"));
  declaration.hooks.PreToolUse[0].hooks[0].statusMessage = "Updated managed policy check";
  await writeFile(declarationPath, `${JSON.stringify(declaration, null, 2)}\n`);
  const updated = await installPackage({ sourceRoot: changedSource, ...args, now: new Date("2026-09-06T12:01:00.000Z") });
  const afterUpdate = JSON.parse(await readFile(configPath, "utf8"));
  const updatedGroup = afterUpdate.hooks.PreToolUse.find((entry) => entry.hooks.some(({ command }) => command === "unrelated-policy-check"));
  assert.equal(updatedGroup.label, "preserve-this-group-metadata");
  assert.notEqual(updatedGroup.hooks[0].statusMessage, "Updated managed policy check");
  assert.equal(updated.status, "update_requires_manual_replacement");
  assert.deepEqual(await readFile(configPath), beforeUpdate);
  assert.deepEqual(await readFile(receiptPath), receiptBeforeUpdate);

  updatedGroup.hooks[0].command += " --custom-user-argument";
  await writeFile(configPath, `${JSON.stringify(afterUpdate, null, 2)}\n`);
  const reinstalled = await installPackage({ sourceRoot: changedSource, ...args, now: new Date("2026-09-06T12:02:00.000Z") });
  const afterReinstall = JSON.parse(await readFile(configPath, "utf8"));
  assert.equal(afterReinstall.hooks.PreToolUse.some((entry) => entry.hooks.some(({ command }) => command?.endsWith("--custom-user-argument"))), true);
  assert.equal(afterReinstall.hooks.PreToolUse.some((entry) => entry.hooks.some(({ command }) => command === "unrelated-policy-check")), true);
  assert.equal(reinstalled.status, "update_requires_manual_replacement");

  await uninstallPackage(args);
  const afterUninstall = JSON.parse(await readFile(configPath, "utf8"));
  assert.equal(afterUninstall.hooks.PreToolUse.some((entry) => entry.hooks.some(({ command }) => command?.endsWith("--custom-user-argument"))), true);
  assert.equal(afterUninstall.hooks.PreToolUse.some((entry) => entry.hooks.some(({ command }) => command === "unrelated-policy-check")), true);
  await readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md"));
  const receipt = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8"));
  assert.ok(receipt.handlers.every(({ event, handlerId, digest }) => event && handlerId && digest));
});

test("selective uninstall keeps the other host byte-for-byte and retains its receipt", async () => {
  // Deleting the shared receipt or rewriting the unselected host would make the second uninstall impossible.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-selective-uninstall-"));
  temporary.push(home);
  await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  const claudeConfig = path.join(home, ".claude", "settings.json");
  const beforeClaude = await readFile(claudeConfig, "utf8");

  await uninstallPackage({ home, host: "codex", scope: "user" });
  assert.equal(await readFile(claudeConfig, "utf8"), beforeClaude);
  await readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md"));
  await assert.rejects(readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
  const receipt = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8"));
  assert.deepEqual([...new Set(receipt.handlers.map(({ runtime }) => runtime))], ["claude"]);

  const second = await uninstallPackage({ home, host: "claude-code", scope: "user" });
  assert.equal(second.status, "uninstalled");
  await assert.rejects(readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
});

test("separate selected-host installs merge ownership instead of orphaning the first host", async () => {
  // Replacing the receipt on the second host install would make a later combined rollback leak the first package.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-merged-receipt-"));
  temporary.push(home);
  await installPackage({ sourceRoot, home, host: "codex", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  const codexBefore = await readFile(path.join(home, ".codex", "hooks.json"), "utf8");
  await installPackage({ sourceRoot, home, host: "claude-code", scope: "user", now: new Date("2026-09-06T12:01:00.000Z") });

  assert.equal(await readFile(path.join(home, ".codex", "hooks.json"), "utf8"), codexBefore);
  const receipt = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8"));
  assert.deepEqual([...new Set(receipt.handlers.map(({ runtime }) => runtime))].sort(), ["claude", "codex"]);
  await uninstallPackage({ home, host: "both", scope: "user" });
  await assert.rejects(readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
});

test("an old whole-group receipt migrates exact unchanged handlers to individual ownership", async () => {
  // Treating legacy managed handlers as pre-existing would strand them after a safe receipt upgrade.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-old-receipt-"));
  temporary.push(home);
  const args = { sourceRoot, home, host: "codex", scope: "user" };
  await installPackage({ ...args, now: new Date("2026-09-06T12:00:00.000Z") });
  const receiptPath = path.join(home, ".agent-team-hooks", "install.json");
  const oldReceipt = JSON.parse(await readFile(receiptPath, "utf8"));
  oldReceipt.schemaVersion = 2;
  delete oldReceipt.handlers;
  await writeFile(receiptPath, `${JSON.stringify(oldReceipt, null, 2)}\n`);

  await installPackage({ ...args, now: new Date("2026-09-06T12:01:00.000Z") });
  const migrated = JSON.parse(await readFile(receiptPath, "utf8"));
  assert.ok(migrated.handlers.length > 0);
  assert.equal(migrated.handlers.some(({ preexisting }) => preexisting), false);
  await uninstallPackage({ home, host: "codex", scope: "user" });
  const config = JSON.parse(await readFile(path.join(home, ".codex", "hooks.json"), "utf8"));
  assert.equal(agentTeamGroups(config).length, 0);
});

test("a receipt-free customized handler is preserved as ambiguous without adding a duplicate", async () => {
  // Substring ownership would overwrite it; blindly appending would run two handlers for one event.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-ambiguous-handler-"));
  temporary.push(home);
  const configPath = path.join(home, ".codex", "hooks.json");
  await mkdir(path.dirname(configPath), { recursive: true });
  const command = 'node "$HOME/.agents/skills/agent-team/hooks/agent-team-hook.mjs" --runtime codex --event PreToolUse --custom-user-argument';
  await writeFile(configPath, `${JSON.stringify({ hooks: { PreToolUse: [{ matcher: "Bash", hooks: [
    { type: "command", command, timeout: 31 },
    { type: "command", command: "unrelated-check" },
  ] }] } }, null, 2)}\n`);

  const result = await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  const config = JSON.parse(await readFile(configPath, "utf8"));
  const commands = config.hooks.PreToolUse.flatMap((group) => group.hooks.map((handler) => handler.command));
  assert.equal(commands.filter((value) => value?.includes("--runtime codex --event PreToolUse")).length, 1);
  assert.equal(commands.includes(command), true);
  assert.equal(commands.includes("unrelated-check"), true);
  assert.equal(result.conflicts.some(({ kind, reason }) => kind === "handler" && reason === "ambiguous_handler"), true);
  const uninstall = await uninstallPackage({ home, host: "codex", scope: "user" });
  assert.equal(uninstall.conflicts.some(({ kind, reason }) => kind === "handler" && reason === "ambiguous_handler"), true);
  await readFile(path.join(home, ".agents", "skills", "agent-team", "hooks", "agent-team-hook.mjs"));

  const resolved = JSON.parse(await readFile(configPath, "utf8"));
  resolved.hooks.PreToolUse[0].hooks = resolved.hooks.PreToolUse[0].hooks.filter(({ command: value }) => value !== command);
  await writeFile(configPath, `${JSON.stringify(resolved, null, 2)}\n`);
  assert.equal((await uninstallPackage({ home, host: "codex", scope: "user" })).status, "uninstalled");
  await assert.rejects(readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
});

test("uninstall keeps the package needed by a customized owned handler", async () => {
  // Removing an unchanged package while preserving its customized command would leave a broken live hook.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-custom-handler-package-"));
  temporary.push(home);
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  const configPath = path.join(home, ".codex", "hooks.json");
  const config = JSON.parse(await readFile(configPath, "utf8"));
  const group = config.hooks.PreToolUse.find((entry) => entry.hooks.some(({ command }) => command?.includes("agent-team-hook.mjs")));
  group.hooks[0].command += " --custom-user-argument";
  await writeFile(configPath, `${JSON.stringify(config, null, 2)}\n`);

  const result = await uninstallPackage({ home, host: "codex", scope: "user" });
  assert.equal(result.conflicts.some(({ kind, reason }) => kind === "handler" && reason === "customized_handler"), true);
  await readFile(path.join(home, ".agents", "skills", "agent-team", "hooks", "agent-team-hook.mjs"));
});

test("uninstall preserves a receipt-marked pre-existing exact handler and its package", async () => {
  // Reusing an identical handler must not convert it into deletable ownership or strand its command.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-preexisting-handler-"));
  temporary.push(home);
  const configPath = path.join(home, ".codex", "hooks.json");
  await mkdir(path.dirname(configPath), { recursive: true });
  const declaration = JSON.parse(await readFile(path.join(sourceRoot, "hooks", "codex-hooks.json"), "utf8"));
  await writeFile(configPath, `${JSON.stringify(declaration, null, 2)}\n`);
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });

  const result = await uninstallPackage({ home, host: "codex", scope: "user" });
  assert.equal(result.conflicts.some(({ reason }) => reason === "preexisting_handler"), true);
  assert.ok(agentTeamGroups(JSON.parse(await readFile(configPath, "utf8"))).length > 0);
  await readFile(path.join(home, ".agents", "skills", "agent-team", "hooks", "agent-team-hook.mjs"));
});

test("a changed release preserves a pre-existing handler pending manual package replacement", async () => {
  // A release update must not turn an unowned exact handler into managed content and overwrite it.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-preexisting-update-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-preexisting-update-source-"));
  temporary.push(home, changedSource);
  await copyTrackedSource(sourceRoot, changedSource);
  const configPath = path.join(home, ".codex", "hooks.json");
  await mkdir(path.dirname(configPath), { recursive: true });
  const original = JSON.parse(await readFile(path.join(sourceRoot, "hooks", "codex-hooks.json"), "utf8"));
  await writeFile(configPath, `${JSON.stringify(original, null, 2)}\n`);
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });

  const declarationPath = path.join(changedSource, "hooks", "codex-hooks.json");
  const changed = JSON.parse(await readFile(declarationPath, "utf8"));
  changed.hooks.PreToolUse[0].hooks[0].statusMessage = "new release text";
  await writeFile(declarationPath, `${JSON.stringify(changed, null, 2)}\n`);
  const result = await installPackage({ sourceRoot: changedSource, home, host: "codex", scope: "user" });

  const after = JSON.parse(await readFile(configPath, "utf8"));
  assert.notEqual(after.hooks.PreToolUse[0].hooks[0].statusMessage, "new release text");
  assert.equal(result.status, "update_requires_manual_replacement");
  assert.equal(result.conflicts.some(({ reason }) => reason === "differing_present_target"), true);
  await uninstallPackage({ home, host: "codex", scope: "user" });
  assert.deepEqual(JSON.parse(await readFile(configPath, "utf8")), original);
  await readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md"));
});

test("a changed release cannot retire an owned handler before manual package replacement", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-retired-handler-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-retired-source-"));
  temporary.push(home, changedSource);
  await copyTrackedSource(sourceRoot, changedSource);
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  const declarationPath = path.join(changedSource, "hooks", "codex-hooks.json");
  const changed = JSON.parse(await readFile(declarationPath, "utf8"));
  delete changed.hooks.Interrupt;
  await writeFile(declarationPath, `${JSON.stringify(changed, null, 2)}\n`);
  const manifestPath = path.join(changedSource, "hooks", "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  manifest.requiredEvents.codex = manifest.requiredEvents.codex.filter((event) => event !== "Interrupt");
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  const result = await installPackage({ sourceRoot: changedSource, home, host: "codex", scope: "user" });
  const config = JSON.parse(await readFile(path.join(home, ".codex", "hooks.json"), "utf8"));
  const receipt = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8"));
  assert.equal(result.status, "update_requires_manual_replacement");
  assert.notEqual(config.hooks.Interrupt, undefined);
  assert.equal(receipt.handlers.some(({ event }) => event === "Interrupt"), true);
});

test("a changed release preserves customized retired-handler identity pending manual replacement", async () => {
  // A customized retired hook must remain receipted as a conflict so uninstall cannot strand it.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-custom-retired-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-custom-retired-source-"));
  temporary.push(home, changedSource);
  await copyTrackedSource(sourceRoot, changedSource);
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  const configPath = path.join(home, ".codex", "hooks.json");
  const config = JSON.parse(await readFile(configPath, "utf8"));
  config.hooks.Interrupt[0].hooks[0].command += " --custom";
  await writeFile(configPath, `${JSON.stringify(config, null, 2)}\n`);
  const declarationPath = path.join(changedSource, "hooks", "codex-hooks.json");
  const changed = JSON.parse(await readFile(declarationPath, "utf8"));
  delete changed.hooks.Interrupt;
  await writeFile(declarationPath, `${JSON.stringify(changed, null, 2)}\n`);
  const manifestPath = path.join(changedSource, "hooks", "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  manifest.requiredEvents.codex = manifest.requiredEvents.codex.filter((event) => event !== "Interrupt");
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  const result = await installPackage({ sourceRoot: changedSource, home, host: "codex", scope: "user" });
  const receipt = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8"));
  assert.equal(result.status, "update_requires_manual_replacement");
  assert.equal(receipt.handlers.some(({ event }) => event === "Interrupt"), true);
  await uninstallPackage({ home, host: "codex", scope: "user" });
  assert.match(await readFile(configPath, "utf8"), /--event Interrupt --custom/);
  await readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md"));
});

test("a conflict in one host does not retain stale receipt ownership for the other host", async () => {
  // One customized Codex handler must not prevent a clean Claude uninstall or leave stale Claude receipt entries.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-one-host-conflict-"));
  temporary.push(home);
  await installPackage({ sourceRoot, home, host: "both", scope: "user" });
  const codexPath = path.join(home, ".codex", "hooks.json");
  const codex = JSON.parse(await readFile(codexPath, "utf8"));
  codex.hooks.PreToolUse.find((group) => group.hooks.some(({ command }) => command?.includes("agent-team-hook.mjs"))).hooks[0].command += " --custom";
  await writeFile(codexPath, `${JSON.stringify(codex, null, 2)}\n`);

  const result = await uninstallPackage({ home, host: "both", scope: "user" });
  assert.equal(result.status, "uninstalled_with_conflicts");
  await readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md"));
  await assert.rejects(readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
  const receipt = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8"));
  assert.deepEqual([...new Set(receipt.handlers.map(({ runtime }) => runtime))], ["codex"]);
  assert.deepEqual([...new Set(receipt.targets.map(({ runtime }) => runtime))], ["codex"]);
  const remainingCodex = JSON.parse(await readFile(codexPath, "utf8"));
  assert.equal(agentTeamGroups(remainingCodex).length, 1);
});

test("partial Claude uninstall retains receipts for every role it leaves in place", async () => {
  // Blocking role removal because one handler is customized must not discard ownership of the retained roles.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-retained-claude-roles-"));
  temporary.push(home);
  await installPackage({ sourceRoot, home, host: "claude-code", scope: "user" });
  const receiptPath = path.join(home, ".agent-team-hooks", "install.json");
  const before = JSON.parse(await readFile(receiptPath, "utf8"));
  const configPath = path.join(home, ".claude", "settings.json");
  const config = JSON.parse(await readFile(configPath, "utf8"));
  Object.values(config.hooks).flat().find((group) => group.hooks?.[0])
    .hooks[0].command += " --user-custom-option";
  await writeFile(configPath, `${JSON.stringify(config, null, 2)}\n`);

  const result = await uninstallPackage({ home, host: "claude-code", scope: "user" });
  const after = JSON.parse(await readFile(receiptPath, "utf8"));
  assert.equal(result.status, "uninstalled_with_conflicts");
  for (const role of before.claudeAgents) {
    await readFile(role.path);
    assert.equal(after.claudeAgents.some(({ path: name }) => name === role.path), true);
  }
});

test("restoring one customized hook lets retry finish after safe siblings were removed", async () => {
  // Retaining receipts for successfully removed same-runtime siblings creates permanent missing-handler conflicts.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-retry-uninstall-"));
  temporary.push(home);
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  const configPath = path.join(home, ".codex", "hooks.json");
  const config = JSON.parse(await readFile(configPath, "utf8"));
  const [event, groups] = Object.entries(config.hooks).find(([, rows]) => rows.some((group) => group.hooks?.[0]));
  const handler = groups.find((group) => group.hooks?.[0]).hooks[0];
  const original = structuredClone(handler);
  handler.command += " --user-custom-option";
  await writeFile(configPath, `${JSON.stringify(config, null, 2)}\n`);
  assert.equal((await uninstallPackage({ home, host: "codex", scope: "user" })).status, "uninstalled_with_conflicts");

  const remaining = JSON.parse(await readFile(configPath, "utf8"));
  remaining.hooks[event].find((group) => group.hooks?.some(({ command }) => command?.endsWith("--user-custom-option"))).hooks[0] = original;
  await writeFile(configPath, `${JSON.stringify(remaining, null, 2)}\n`);
  assert.equal((await uninstallPackage({ home, host: "codex", scope: "user" })).status, "uninstalled");
});

test("installer preserves unrelated settings, removes the legacy Codex duplicate, and reruns cleanly", async () => {
  // This test catches whole-config replacement, duplicate registration, and two active Codex copies.
  const home = await homeFixture();
  const first = await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  const codex = JSON.parse(await readFile(path.join(home, ".codex", "hooks.json"), "utf8"));
  const claude = JSON.parse(await readFile(path.join(home, ".claude", "settings.json"), "utf8"));
  const backupCount = (await readdir(path.join(home, ".agent-team-hooks", "backups"), { recursive: true })).length;
  const second = await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:01:00.000Z") });
  const backupCountAfter = (await readdir(path.join(home, ".agent-team-hooks", "backups"), { recursive: true })).length;

  assert.equal(first.changed, true);
  assert.equal(second.changed, false);
  assert.equal(codex.hooks.Stop[0].hooks[0].command, "unrelated-stop");
  assert.equal(codex.hooks.SessionStart.some((group) => group.hooks[0].command === "lean-ctx hook codex-session-start"), true);
  assert.equal(claude.model, "fable");
  assert.equal(claude.hooks.PostToolUse.some((group) => group.hooks[0].command === "unrelated-write"), true);
  assert.equal(claude.hooks.PreToolUse.some((group) => group.hooks[0].command === "lean-ctx hook redirect"), true);
  assert.ok(agentTeamGroups(codex).length > 0);
  assert.ok(agentTeamGroups(claude).length > 0);
  assert.equal(await readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md"), "utf8").then(Boolean), true);
  assert.equal(await readFile(path.join(home, ".claude", "skills", "agent-team", "hooks", "agent-team-hook.mjs"), "utf8").then(Boolean), true);
  await assert.rejects(readFile(path.join(home, ".codex", "skills", "agent-team", "legacy.txt")), { code: "ENOENT" });
  assert.equal(backupCountAfter, backupCount);
});

test("installer requires manual replacement when a release manifest adds a file", async () => {
  const home = await homeFixture();
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-expanded-source-"));
  temporary.push(changedSource);
  await copyTrackedSource(sourceRoot, changedSource);
  await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });

  const addedFile = "references/new-release-file.md";
  await writeFile(path.join(changedSource, addedFile), "new managed file\n");
  const manifestPath = path.join(changedSource, "hooks", "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  manifest.files.push(addedFile);
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  const result = await installPackage({ sourceRoot: changedSource, home, host: "both", scope: "user", now: new Date("2026-09-06T12:01:00.000Z") });

  assert.equal(result.status, "update_requires_manual_replacement");
  assert.equal(result.changed, false);
  await assert.rejects(readFile(path.join(home, ".agents", "skills", "agent-team", addedFile)), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(home, ".claude", "skills", "agent-team", addedFile)), { code: "ENOENT" });
});

test("uninstall after a denied automatic update removes the unchanged installed release", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-version-uninstall-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-version-source-"));
  temporary.push(home, changedSource);
  await copyTrackedSource(sourceRoot, changedSource);
  await installPackage({ sourceRoot, home, host: "codex", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  const addedFile = "references/version-two-marker.md";
  await writeFile(path.join(changedSource, addedFile), "version two\n");
  const manifestPath = path.join(changedSource, "hooks", "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  manifest.files.push(addedFile);
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  const denied = await installPackage({ sourceRoot: changedSource, home, host: "codex", scope: "user", now: new Date("2026-09-06T12:01:00.000Z") });
  const receipt = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8"));
  assert.equal(denied.status, "update_requires_manual_replacement");
  assert.equal(receipt.version, manifest.version);
  assert.match(receipt.transactionId, /^[0-9a-f-]{36}$/);
  assert.equal(receipt.backups.some(({ kind, purpose }) => kind === "skill" && purpose === "update_snapshot"), false);
  await uninstallPackage({ home, host: "codex", scope: "user" });
  await assert.rejects(readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
});

test("uninstall after an update restores only an original backup from an older receipt", async () => {
  // Update snapshots must not hide a pre-install restoration backup created by an earlier installer version.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-original-backup-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-original-backup-source-"));
  temporary.push(home, changedSource);
  await copyTrackedSource(sourceRoot, changedSource);
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  const target = path.join(home, ".agents", "skills", "agent-team");
  const receiptPath = path.join(home, ".agent-team-hooks", "install.json");
  const originalBackup = path.join(home, ".agent-team-hooks", "backups", "legacy-receipt", "original-skill");
  await mkdir(originalBackup, { recursive: true });
  await writeFile(path.join(originalBackup, "ORIGINAL.md"), "original resource\n");
  const oldReceipt = JSON.parse(await readFile(receiptPath, "utf8"));
  oldReceipt.backups.unshift({ kind: "skill", target, backup: originalBackup });
  await writeFile(receiptPath, `${JSON.stringify(oldReceipt, null, 2)}\n`);
  const addedFile = "references/second-version.md";
  await writeFile(path.join(changedSource, addedFile), "second version\n");
  const manifestPath = path.join(changedSource, "hooks", "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  manifest.files.push(addedFile);
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  await installPackage({ sourceRoot: changedSource, home, host: "codex", scope: "user" });
  await uninstallPackage({ home, host: "codex", scope: "user" });
  assert.equal(await readFile(path.join(target, "ORIGINAL.md"), "utf8"), "original resource\n");
  await assert.rejects(readFile(path.join(target, "SKILL.md")), { code: "ENOENT" });
});

test("adding a host from an installed CLI preserves copied ownership for the first host", async () => {
  // Reclassifying sourceRoot=target as protected source would orphan the first copied package on combined uninstall.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-installed-source-ownership-"));
  temporary.push(home);
  await installPackage({ sourceRoot, home, host: "codex", scope: "user" });
  const installedRoot = path.join(home, ".agents", "skills", "agent-team");
  await installPackage({ sourceRoot: installedRoot, home, host: "both", scope: "user" });
  const receipt = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8"));
  assert.equal(receipt.targets.find(({ runtime }) => runtime === "codex").mode, "copied");

  await uninstallPackage({ home, host: "both", scope: "user" });
  await assert.rejects(readFile(path.join(installedRoot, "SKILL.md")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
});

test("uninstall removes owned groups and copies while preserving unrelated settings and the legacy copy", async () => {
  // This test catches rollback code that deletes unrelated configuration or retains its owned package.
  const home = await homeFixture();
  await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  const result = await uninstallPackage({ home, host: "both", scope: "user" });
  const codex = JSON.parse(await readFile(path.join(home, ".codex", "hooks.json"), "utf8"));
  const claude = JSON.parse(await readFile(path.join(home, ".claude", "settings.json"), "utf8"));

  assert.equal(result.changed, true);
  assert.equal(agentTeamGroups(codex).length, 0);
  assert.equal(agentTeamGroups(claude).length, 0);
  assert.equal(codex.hooks.Stop[0].hooks[0].command, "unrelated-stop");
  assert.equal(claude.model, "fable");
  await assert.rejects(readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
  assert.equal(await readFile(path.join(home, ".codex", "skills", "agent-team", "legacy.txt"), "utf8"), "legacy\n");
});

test("health separates installed, registered, trusted, supported, and exercised", async () => {
  // This test catches fabricated trust and a Codex activation claim based on file reads.
  const home = await homeFixture();
  await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  let health = await getHealth({ home });
  assert.equal(health.runtimes.codex.installed, true);
  assert.equal(health.runtimes.codex.registered, true);
  assert.equal(health.runtimes.codex.trusted, "unknown");
  assert.equal(health.runtimes.codex.activation.status, "unsupported");
  assert.equal(health.runtimes.claude.exercised, false);

  await appendActivationLog(path.join(home, ".agent-team-hooks", "logs"), {
    runtime: "claude", skill: "agent-team", sessionId: "s", eventKind: "PreToolUse", correlationId: "health-1",
  });
  health = await getHealth({ home });
  assert.equal(health.runtimes.claude.activation.status, "supported");
  assert.equal(health.runtimes.claude.exercised, true);
});

test("installer does not infer update authority from a Codex source already at its target path", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-source-home-"));
  temporary.push(home);
  const codexTarget = path.join(home, ".agents", "skills", "agent-team");
  await mkdir(path.dirname(codexTarget), { recursive: true });
  await copyTrackedSource(sourceRoot, codexTarget);

  const result = await installPackage({ sourceRoot: codexTarget, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });

  assert.equal(result.status, "update_requires_manual_replacement");
  assert.equal(await readFile(path.join(codexTarget, "SKILL.md"), "utf8").then(Boolean), true);
  await assert.rejects(readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
  await assert.rejects(readFile(path.join(home, ".agent-team-hooks", "install.json")), { code: "ENOENT" });
  assert.equal(await readFile(path.join(codexTarget, "SKILL.md"), "utf8").then(Boolean), true);
});

test("uninstall preserves a copied package that the user changed after installation", async () => {
  // This test catches recursive removal based only on a receipt target path.
  const home = await homeFixture();
  await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  const target = path.join(home, ".agents", "skills", "agent-team");
  await writeFile(path.join(target, "LOCAL-NOTE.md"), "keep this user change\n");

  const result = await uninstallPackage({ home, host: "both", scope: "user" });

  assert.equal(await readFile(path.join(target, "LOCAL-NOTE.md"), "utf8"), "keep this user change\n");
  assert.equal(result.conflicts.some(({ target: name }) => name === target), true);
});

test("installer rolls back its package and config mutations after a later failure", async () => {
  // This test catches a multi-file install that leaves partial state after invalid input.
  const home = await homeFixture();
  const brokenSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-broken-source-"));
  temporary.push(brokenSource);
  await copyTrackedSource(sourceRoot, brokenSource);
  await writeFile(path.join(brokenSource, "hooks", "claude-hooks.json"), "{bad json\n");
  const originalCodex = await readFile(path.join(home, ".codex", "hooks.json"), "utf8");

  await assert.rejects(installPackage({ sourceRoot: brokenSource, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") }));

  await assert.rejects(readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
  assert.equal(await readFile(path.join(home, ".codex", "hooks.json"), "utf8"), originalCodex);
  await assert.rejects(readFile(path.join(home, ".agent-team-hooks", "install.json")), { code: "ENOENT" });
});

test("a subsequent CLI invocation recovers ownership after abrupt process termination", async () => {
  // An in-memory undo stack cannot recover the legacy resource after the installer process is killed.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-interrupted-install-"));
  temporary.push(home);
  const legacy = path.join(home, ".codex", "skills", "agent-team");
  await mkdir(legacy, { recursive: true });
  await writeFile(path.join(legacy, "original.txt"), "original ownership\n");
  const configPath = path.join(home, ".codex", "hooks.json");
  await mkdir(path.dirname(configPath), { recursive: true });
  await writeFile(configPath, JSON.stringify({ padding: "x".repeat(16 * 1024 * 1024), hooks: {} }));
  const artifact = await archiveFixture();
  const installArgs = [cli, "install", "--archive", artifact.archive, "--checksums", artifact.checksums, "--home", home, "--host", "codex", "--scope", "user"];

  const child = spawn(process.execPath, installArgs, {
    stdio: "ignore",
  });
  let interruptedBackup;
  for (let attempt = 0; attempt < 5000 && !interruptedBackup; attempt += 1) {
    let journal;
    try {
      journal = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "transaction.json"), "utf8"));
    } catch {}
    const action = journal?.undo?.find(({ kind, to }) => kind === "move" && to === legacy);
    if (action) {
      try {
        await readdir(action.from);
        interruptedBackup = action.from;
      } catch {}
    }
    if (child.exitCode !== null) break;
    await delay(1);
  }
  assert.ok(interruptedBackup, "installer exited before a moved resource was durably journaled");
  child.kill("SIGKILL");
  await new Promise((resolve) => child.once("exit", resolve));
  await assert.rejects(readFile(path.join(legacy, "original.txt")), { code: "ENOENT" });

  const recovered = await Promise.all([
    run(process.execPath, installArgs),
    run(process.execPath, installArgs),
  ]);
  const recoveredResults = recovered.map(({ stdout }) => JSON.parse(stdout));
  assert.equal(recoveredResults.filter(({ recovery }) => recovery?.action === "rolled_back").length, 1);
  assert.equal(recoveredResults.filter(({ changed }) => changed).length, 1);
  await run(process.execPath, [cli, "rollback", "--home", home, "--host", "codex", "--scope", "user"]);
  assert.equal(await readFile(path.join(legacy, "original.txt"), "utf8"), "original ownership\n");
  await assert.rejects(readFile(path.join(home, ".agent-team-hooks", "transaction.json")), { code: "ENOENT" });
});

test("recovery preserves a user replacement created after abrupt termination", async () => {
  // Unconditional undo would delete the replacement before restoring the interrupted transaction's backup.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-interrupted-user-edit-"));
  temporary.push(home);
  const legacy = path.join(home, ".codex", "skills", "agent-team");
  await mkdir(legacy, { recursive: true });
  await writeFile(path.join(legacy, "original.txt"), "original ownership\n");
  const configPath = path.join(home, ".codex", "hooks.json");
  await mkdir(path.dirname(configPath), { recursive: true });
  await writeFile(configPath, JSON.stringify({ padding: "x".repeat(16 * 1024 * 1024), hooks: {} }));
  const artifact = await archiveFixture();
  const installArgs = [cli, "install", "--archive", artifact.archive, "--checksums", artifact.checksums, "--home", home, "--host", "codex", "--scope", "user"];
  const child = spawn(process.execPath, installArgs, {
    stdio: "ignore",
  });
  let interruptedBackup;
  for (let attempt = 0; attempt < 5000 && !interruptedBackup; attempt += 1) {
    let journal;
    try {
      journal = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "transaction.json"), "utf8"));
    } catch {}
    const action = journal?.undo?.find(({ kind, to }) => kind === "move" && to === legacy);
    if (action) {
      try {
        await readdir(action.from);
        interruptedBackup = action.from;
      } catch {}
    }
    if (child.exitCode !== null) break;
    await delay(1);
  }
  assert.ok(interruptedBackup, "installer exited before a moved resource was durably journaled");
  child.kill("SIGKILL");
  await new Promise((resolve) => child.once("exit", resolve));
  await mkdir(legacy, { recursive: true });
  await writeFile(path.join(legacy, "USER.md"), "created after crash\n");

  await assert.rejects(
    run(process.execPath, installArgs),
    (error) => /recovery conflict/i.test(error.stdout),
  );
  assert.equal(await readFile(path.join(legacy, "USER.md"), "utf8"), "created after crash\n");
  assert.equal(await readFile(path.join(interruptedBackup, "original.txt"), "utf8"), "original ownership\n");
});

test("restore-file recovery accepts an intact preimage at either idempotent crash point", async (t) => {
  // The same intact original is observable before the forward write and after a completed undo.
  for (const recoveryPoint of ["before config write", "after restore before journal cleanup"]) {
    await t.test(recoveryPoint, async () => {
      const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-restore-preimage-"));
      temporary.push(home);
      const stateRoot = path.join(home, ".agent-team-hooks");
      const configPath = path.join(home, ".codex", "hooks.json");
      const backup = path.join(stateRoot, "backups", recoveryPoint.startsWith("before") ? "before" : "after", "config", "hooks.json");
      const original = `${JSON.stringify({ sentinel: recoveryPoint, hooks: {} }, null, 2)}\n`;
      const postimage = `${JSON.stringify({ sentinel: "installer postimage", hooks: {} }, null, 2)}\n`;
      await mkdir(path.dirname(configPath), { recursive: true });
      await mkdir(path.dirname(backup), { recursive: true });
      await writeFile(configPath, original);
      await writeFile(backup, original);
      await writeFile(path.join(stateRoot, "transaction.json"), `${JSON.stringify({
        schemaVersion: 1,
        transactionId: `restore-preimage-${recoveryPoint.replaceAll(" ", "-")}`,
        operation: "install",
        status: "active",
        undo: [{
          kind: "restore_file",
          target: configPath,
          backup,
          preimageGuard: fileGuard(original),
          targetGuard: fileGuard(postimage),
        }],
      }, null, 2)}\n`);

      const result = await installPackage({ sourceRoot, home, host: "codex", scope: "user" });

      assert.equal(result.recovery?.action, "rolled_back");
      assert.equal(JSON.parse(await readFile(configPath, "utf8")).sentinel, recoveryPoint);
      await assert.rejects(readFile(path.join(stateRoot, "transaction.json")), { code: "ENOENT" });
    });
  }
});

test("a dead stale-lock recovery claimant is reclaimed without displacing a new owner", async () => {
  // Killing the claimant after its exclusive claim is written must not strand the install lock.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-dead-recovery-claim-"));
  temporary.push(home);
  const stateRoot = path.join(home, ".agent-team-hooks");
  const lockPath = path.join(stateRoot, "install.lock");
  const child = spawn(process.execPath, ["-e", `
    const fs = require("node:fs");
    const path = require("node:path");
    const lockPath = path.join(process.argv[1], ".agent-team-hooks", "install.lock");
    fs.mkdirSync(lockPath, { recursive: true });
    fs.writeFileSync(path.join(lockPath, "owner.json"), JSON.stringify({ pid: process.pid, operation: "install", acquiredAt: "2026-09-06T12:00:00.000Z", lockToken: "stale-owner" }) + "\\n");
    fs.writeFileSync(path.join(lockPath, "recovery.json"), JSON.stringify({ pid: process.pid, operation: "install", acquiredAt: "2026-09-06T12:00:01.000Z", lockToken: "dead-claimant" }) + "\\n");
    process.kill(process.pid, "SIGKILL");
  `, home], { stdio: "ignore" });
  const signal = await new Promise((resolve) => child.once("exit", (_code, exitSignal) => resolve(exitSignal)));
  assert.equal(signal, "SIGKILL");

  const results = await Promise.all([
    installPackage({ sourceRoot, home, host: "codex", scope: "user", now: new Date("2026-09-06T12:01:00.000Z") }),
    installPackage({ sourceRoot, home, host: "codex", scope: "user", now: new Date("2026-09-06T12:01:01.000Z") }),
  ]);

  assert.equal(results.filter(({ changed }) => changed).length, 1);
  await assert.rejects(readFile(lockPath), { code: "ENOENT" });
  const staleLocks = (await readdir(stateRoot)).filter((name) => name.startsWith("install.lock.stale-"));
  assert.equal(staleLocks.length, 1);
  const fencedLock = path.join(stateRoot, staleLocks[0]);
  assert.equal(JSON.parse(await readFile(path.join(fencedLock, "owner.json"), "utf8")).lockToken, "stale-owner");
  assert.equal(JSON.parse(await readFile(path.join(fencedLock, "recovery.json"), "utf8")).lockToken, "dead-claimant");
});

test("recovery rejects an out-of-scope journal without applying it", async () => {
  // A writable journal is recovery evidence, not authority to mutate an arbitrary path.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-invalid-journal-home-"));
  const outside = await mkdtemp(path.join(os.tmpdir(), "agent-team-invalid-journal-outside-"));
  temporary.push(home, outside);
  const protectedFile = path.join(outside, "keep.txt");
  await writeFile(protectedFile, "keep\n");
  const stateRoot = path.join(home, ".agent-team-hooks");
  await mkdir(stateRoot, { recursive: true });
  await writeFile(path.join(stateRoot, "transaction.json"), `${JSON.stringify({
    schemaVersion: 1,
    transactionId: "not-authority",
    operation: "install",
    status: "active",
    undo: [{ kind: "remove_path", path: protectedFile, recursive: true, internal: true }],
  }, null, 2)}\n`);

  await assert.rejects(
    installPackage({ sourceRoot, home, host: "codex", scope: "user" }),
    /journal is invalid|out-of-scope/i,
  );
  assert.equal(await readFile(protectedFile, "utf8"), "keep\n");
});

test("parallel installer calls serialize one transaction and keep one registration set", async () => {
  // This test catches concurrent read-merge-write operations without a user-level lock.
  const home = await homeFixture();
  const results = await Promise.all([
    installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") }),
    installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:01.000Z") }),
  ]);
  const codex = JSON.parse(await readFile(path.join(home, ".codex", "hooks.json"), "utf8"));
  const claude = JSON.parse(await readFile(path.join(home, ".claude", "settings.json"), "utf8"));

  assert.equal(results.filter(({ changed }) => changed).length, 1);
  assert.equal(agentTeamGroups(codex).length, Object.keys(JSON.parse(await readFile(path.join(sourceRoot, "hooks", "codex-hooks.json"), "utf8")).hooks).length);
  assert.equal(agentTeamGroups(claude).length, Object.keys(JSON.parse(await readFile(path.join(sourceRoot, "hooks", "claude-hooks.json"), "utf8")).hooks).length);
});

test("parallel uninstall calls serialize and preserve later unrelated config entries", async () => {
  // This test checks that install and uninstall use the same user-level transaction lock.
  const home = await homeFixture();
  await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  const configPath = path.join(home, ".codex", "hooks.json");
  const config = JSON.parse(await readFile(configPath, "utf8"));
  config.hooks.SessionEnd = [{ hooks: [{ type: "command", command: "unrelated-session-end" }] }];
  await writeFile(configPath, JSON.stringify(config));

  const results = await Promise.all([uninstallPackage({ home, host: "both", scope: "user" }), uninstallPackage({ home, host: "both", scope: "user" })]);
  const stored = JSON.parse(await readFile(configPath, "utf8"));

  assert.equal(results.some(({ status }) => status === "uninstalled"), true);
  assert.equal(results.some(({ status }) => status === "not_installed"), true);
  assert.equal(stored.hooks.SessionEnd[0].hooks[0].command, "unrelated-session-end");
});

test("uninstall rolls back earlier config changes when a later config is malformed", async () => {
  // This test checks rollback of an incomplete uninstall transaction.
  const home = await homeFixture();
  await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  const codexPath = path.join(home, ".codex", "hooks.json");
  const before = await readFile(codexPath, "utf8");
  await writeFile(path.join(home, ".claude", "settings.json"), "{bad json\n");

  await assert.rejects(uninstallPackage({ home, host: "both", scope: "user" }));

  assert.equal(await readFile(codexPath, "utf8"), before);
  assert.equal(await readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md"), "utf8").then(Boolean), true);
  assert.equal(await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8").then(Boolean), true);
});

test("installer manages current Claude roles, leaves unchanged roles, and reports custom conflicts", async () => {
  // This test catches an install that copies role sources only inside the skill directory.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-role-home-"));
  temporary.push(home);
  const agents = path.join(home, ".claude", "agents");
  await mkdir(agents, { recursive: true });
  const custom = path.join(agents, "agent-team-reviewer.md");
  await writeFile(custom, "custom reviewer\n");

  const first = await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  const developer = path.join(agents, "agent-team-developer.md");
  const original = await readFile(developer, "utf8");
  const second = await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:01:00.000Z") });

  assert.match(original, /name: agent-team-developer/);
  assert.equal(await readFile(custom, "utf8"), "custom reviewer\n");
  assert.equal(first.conflicts.some(({ target }) => target === custom), true);
  assert.equal(second.changed, false);
  assert.equal(await readFile(developer, "utf8"), original);
});

test("installer leaves a managed Claude role unchanged when package replacement is required", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-role-update-home-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-role-source-"));
  temporary.push(home, changedSource);
  await copyTrackedSource(sourceRoot, changedSource);
  await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  const roleSource = path.join(changedSource, "assets", "claude-agents", "agent-team-developer.md");
  const installedRole = path.join(home, ".claude", "agents", "agent-team-developer.md");
  const before = await readFile(installedRole, "utf8");
  await writeFile(roleSource, `${await readFile(roleSource, "utf8")}\nManaged update marker.\n`);

  const result = await installPackage({ sourceRoot: changedSource, home, host: "both", scope: "user", now: new Date("2026-09-06T12:01:00.000Z") });
  const installed = await readFile(installedRole, "utf8");

  assert.equal(result.status, "update_requires_manual_replacement");
  assert.equal(installed, before);
  assert.equal(result.backups.some(({ kind }) => kind === "claude_agent"), false);
});

test("mixed-host update requires manual replacement without changing either host", async () => {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-mixed-unavailable-home-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-mixed-unavailable-source-"));
  temporary.push(home, changedSource);

  // Seed an identical, unowned Claude package, then install both hosts so only its package is pre-existing.
  await installPackage({ sourceRoot, home, host: "claude-code", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  await rm(path.join(home, ".agent-team-hooks"), { force: true, recursive: true });
  await rm(path.join(home, ".claude", "settings.json"), { force: true });
  await rm(path.join(home, ".claude", "agents"), { force: true, recursive: true });
  await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:01:00.000Z") });
  const receiptPath = path.join(home, ".agent-team-hooks", "install.json");
  const before = JSON.parse(await readFile(receiptPath, "utf8"));
  const claudeConfigPath = path.join(home, ".claude", "settings.json");
  const claudeConfig = await readFile(claudeConfigPath, "utf8");

  await copyTrackedSource(sourceRoot, changedSource);
  const addedFile = "references/mixed-host-update.md";
  await writeFile(path.join(changedSource, addedFile), "changed package\n");
  const manifestPath = path.join(changedSource, "hooks", "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  manifest.files.push(addedFile);
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  const result = await installPackage({ sourceRoot: changedSource, home, host: "both", scope: "user", now: new Date("2026-09-06T12:02:00.000Z") });
  const after = JSON.parse(await readFile(receiptPath, "utf8"));

  assert.equal(result.status, "update_requires_manual_replacement");
  assert.equal(result.conflicts.every(({ reason }) => reason === "differing_present_target"), true);
  await assert.rejects(readFile(path.join(home, ".agents", "skills", "agent-team", addedFile)), { code: "ENOENT" });
  assert.equal(await readFile(claudeConfigPath, "utf8"), claudeConfig);
  assert.deepEqual(after.handlers.filter(({ runtime }) => runtime === "claude"), before.handlers.filter(({ runtime }) => runtime === "claude"));
  assert.deepEqual(after.claudeAgents, before.claudeAgents);
  assert.deepEqual(after.targets.filter(({ runtime }) => runtime === "claude"), before.targets.filter(({ runtime }) => runtime === "claude"));
});
