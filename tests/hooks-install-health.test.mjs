import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { getHealth } from "../hooks/lib/health.mjs";
import { installPackage, uninstallPackage } from "../hooks/lib/install.mjs";
import { appendActivationLog } from "../hooks/lib/telemetry.mjs";

const sourceRoot = path.resolve(import.meta.dirname, "..");
const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { force: true, recursive: true }))));

async function homeFixture() {
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-home-"));
  temporary.push(home);
  await mkdir(path.join(home, ".codex", "skills", "agent-team"), { recursive: true });
  await mkdir(path.join(home, ".agents", "skills", "agent-team"), { recursive: true });
  await mkdir(path.join(home, ".claude"), { recursive: true });
  await writeFile(path.join(home, ".codex", "skills", "agent-team", "legacy.txt"), "legacy\n");
  await writeFile(path.join(home, ".agents", "skills", "agent-team", "old.txt"), "old\n");
  await writeFile(path.join(home, ".codex", "hooks.json"), JSON.stringify({ hooks: {
    Stop: [{ hooks: [{ type: "command", command: "unrelated-stop" }] }],
  } }));
  await writeFile(path.join(home, ".claude", "settings.json"), JSON.stringify({ model: "fable", hooks: {
    PostToolUse: [{ matcher: "Write", hooks: [{ type: "command", command: "unrelated-write" }] }],
  } }));
  return home;
}

function agentTeamGroups(config) {
  return Object.values(config.hooks ?? {}).flat().filter((group) => group.hooks?.some(({ command = "" }) => command.includes("agent-team-hook.mjs")));
}

test("installer preserves unrelated settings, backs up replaced copies, removes the legacy Codex duplicate, and reruns cleanly", async () => {
  // This test catches whole-config replacement, duplicate registration, and two active Codex copies.
  const home = await homeFixture();
  const first = await installPackage({ sourceRoot, home, now: new Date("2026-09-06T12:00:00.000Z") });
  const codex = JSON.parse(await readFile(path.join(home, ".codex", "hooks.json"), "utf8"));
  const claude = JSON.parse(await readFile(path.join(home, ".claude", "settings.json"), "utf8"));
  const backupCount = (await readdir(path.join(home, ".agent-team-hooks", "backups"), { recursive: true })).length;
  const second = await installPackage({ sourceRoot, home, now: new Date("2026-09-06T12:01:00.000Z") });
  const backupCountAfter = (await readdir(path.join(home, ".agent-team-hooks", "backups"), { recursive: true })).length;

  assert.equal(first.changed, true);
  assert.equal(second.changed, false);
  assert.equal(codex.hooks.Stop[0].hooks[0].command, "unrelated-stop");
  assert.equal(claude.model, "fable");
  assert.equal(claude.hooks.PostToolUse.some((group) => group.hooks[0].command === "unrelated-write"), true);
  assert.ok(agentTeamGroups(codex).length > 0);
  assert.ok(agentTeamGroups(claude).length > 0);
  assert.equal(await readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md"), "utf8").then(Boolean), true);
  assert.equal(await readFile(path.join(home, ".claude", "skills", "agent-team", "hooks", "agent-team-hook.mjs"), "utf8").then(Boolean), true);
  await assert.rejects(readFile(path.join(home, ".codex", "skills", "agent-team", "legacy.txt")), { code: "ENOENT" });
  assert.equal(backupCountAfter, backupCount);
});

test("uninstall removes owned groups, preserves unrelated settings, and restores backed-up skill copies", async () => {
  // This test catches rollback code that deletes unrelated configuration or loses the prior install.
  const home = await homeFixture();
  await installPackage({ sourceRoot, home, now: new Date("2026-09-06T12:00:00.000Z") });
  const result = await uninstallPackage({ home });
  const codex = JSON.parse(await readFile(path.join(home, ".codex", "hooks.json"), "utf8"));
  const claude = JSON.parse(await readFile(path.join(home, ".claude", "settings.json"), "utf8"));

  assert.equal(result.changed, true);
  assert.equal(agentTeamGroups(codex).length, 0);
  assert.equal(agentTeamGroups(claude).length, 0);
  assert.equal(codex.hooks.Stop[0].hooks[0].command, "unrelated-stop");
  assert.equal(claude.model, "fable");
  assert.equal(await readFile(path.join(home, ".agents", "skills", "agent-team", "old.txt"), "utf8"), "old\n");
  assert.equal(await readFile(path.join(home, ".codex", "skills", "agent-team", "legacy.txt"), "utf8"), "legacy\n");
});

test("health separates installed, registered, trusted, supported, and exercised", async () => {
  // This test catches fabricated trust and a Codex activation claim based on file reads.
  const home = await homeFixture();
  await installPackage({ sourceRoot, home, now: new Date("2026-09-06T12:00:00.000Z") });
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
