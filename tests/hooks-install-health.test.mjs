import assert from "node:assert/strict";
import { cp, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
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

test("installer can run from an authoritative Codex source already at its target path", async () => {
  // This test catches an installer that moves its own source before it copies the Claude package.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-source-home-"));
  temporary.push(home);
  const codexTarget = path.join(home, ".agents", "skills", "agent-team");
  await mkdir(path.dirname(codexTarget), { recursive: true });
  await cp(sourceRoot, codexTarget, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });

  const result = await installPackage({ sourceRoot: codexTarget, home, now: new Date("2026-09-06T12:00:00.000Z") });

  assert.equal(result.status, "installed");
  assert.equal(await readFile(path.join(codexTarget, "SKILL.md"), "utf8").then(Boolean), true);
  assert.equal(await readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md"), "utf8").then(Boolean), true);

  await uninstallPackage({ home });
  assert.equal(await readFile(path.join(codexTarget, "SKILL.md"), "utf8").then(Boolean), true);
});

test("uninstall preserves a copied package that the user changed after installation", async () => {
  // This test catches recursive removal based only on a receipt target path.
  const home = await homeFixture();
  await installPackage({ sourceRoot, home, now: new Date("2026-09-06T12:00:00.000Z") });
  const target = path.join(home, ".agents", "skills", "agent-team");
  await writeFile(path.join(target, "LOCAL-NOTE.md"), "keep this user change\n");

  const result = await uninstallPackage({ home });

  assert.equal(await readFile(path.join(target, "LOCAL-NOTE.md"), "utf8"), "keep this user change\n");
  assert.equal(result.conflicts.some(({ target: name }) => name === target), true);
});

test("installer rolls back its package and config mutations after a later failure", async () => {
  // This test catches a multi-file install that leaves partial state after invalid input.
  const home = await homeFixture();
  const brokenSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-broken-source-"));
  temporary.push(brokenSource);
  await cp(sourceRoot, brokenSource, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });
  await writeFile(path.join(brokenSource, "hooks", "claude-hooks.json"), "{bad json\n");
  const originalCodex = await readFile(path.join(home, ".codex", "hooks.json"), "utf8");

  await assert.rejects(installPackage({ sourceRoot: brokenSource, home, now: new Date("2026-09-06T12:00:00.000Z") }));

  assert.equal(await readFile(path.join(home, ".agents", "skills", "agent-team", "old.txt"), "utf8"), "old\n");
  assert.equal(await readFile(path.join(home, ".codex", "hooks.json"), "utf8"), originalCodex);
  await assert.rejects(readFile(path.join(home, ".agent-team-hooks", "install.json")), { code: "ENOENT" });
});

test("parallel installer calls serialize one transaction and keep one registration set", async () => {
  // This test catches concurrent read-merge-write operations without a user-level lock.
  const home = await homeFixture();
  const results = await Promise.all([
    installPackage({ sourceRoot, home, now: new Date("2026-09-06T12:00:00.000Z") }),
    installPackage({ sourceRoot, home, now: new Date("2026-09-06T12:00:01.000Z") }),
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
  await installPackage({ sourceRoot, home, now: new Date("2026-09-06T12:00:00.000Z") });
  const configPath = path.join(home, ".codex", "hooks.json");
  const config = JSON.parse(await readFile(configPath, "utf8"));
  config.hooks.SessionEnd = [{ hooks: [{ type: "command", command: "unrelated-session-end" }] }];
  await writeFile(configPath, JSON.stringify(config));

  const results = await Promise.all([uninstallPackage({ home }), uninstallPackage({ home })]);
  const stored = JSON.parse(await readFile(configPath, "utf8"));

  assert.equal(results.some(({ status }) => status === "uninstalled"), true);
  assert.equal(results.some(({ status }) => status === "not_installed"), true);
  assert.equal(stored.hooks.SessionEnd[0].hooks[0].command, "unrelated-session-end");
});

test("uninstall rolls back earlier config changes when a later config is malformed", async () => {
  // This test checks rollback of an incomplete uninstall transaction.
  const home = await homeFixture();
  await installPackage({ sourceRoot, home, now: new Date("2026-09-06T12:00:00.000Z") });
  const codexPath = path.join(home, ".codex", "hooks.json");
  const before = await readFile(codexPath, "utf8");
  await writeFile(path.join(home, ".claude", "settings.json"), "{bad json\n");

  await assert.rejects(uninstallPackage({ home }));

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

  const first = await installPackage({ sourceRoot, home, now: new Date("2026-09-06T12:00:00.000Z") });
  const developer = path.join(agents, "agent-team-developer.md");
  const original = await readFile(developer, "utf8");
  const second = await installPackage({ sourceRoot, home, now: new Date("2026-09-06T12:01:00.000Z") });

  assert.match(original, /name: agent-team-developer/);
  assert.equal(await readFile(custom, "utf8"), "custom reviewer\n");
  assert.equal(first.conflicts.some(({ target }) => target === custom), true);
  assert.equal(second.changed, false);
  assert.equal(await readFile(developer, "utf8"), original);
});

test("installer updates a previously managed Claude role and backs up its prior version", async () => {
  // This test catches managed role updates being mistaken for user customization.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-role-update-home-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-role-source-"));
  temporary.push(home, changedSource);
  await cp(sourceRoot, changedSource, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });
  await installPackage({ sourceRoot, home, now: new Date("2026-09-06T12:00:00.000Z") });
  const roleSource = path.join(changedSource, "assets", "claude-agents", "agent-team-developer.md");
  await writeFile(roleSource, `${await readFile(roleSource, "utf8")}\nManaged update marker.\n`);

  const result = await installPackage({ sourceRoot: changedSource, home, now: new Date("2026-09-06T12:01:00.000Z") });
  const installed = await readFile(path.join(home, ".claude", "agents", "agent-team-developer.md"), "utf8");

  assert.match(installed, /Managed update marker/);
  assert.equal(result.backups.some(({ kind }) => kind === "claude_agent"), true);
});
