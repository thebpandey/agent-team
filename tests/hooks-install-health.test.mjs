import assert from "node:assert/strict";
import { execFile, spawn } from "node:child_process";
import { cp, mkdir, mkdtemp, readFile, readdir, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { setTimeout as delay } from "node:timers/promises";
import { promisify } from "node:util";

import { getHealth } from "../hooks/lib/health.mjs";
import { installPackage, uninstallPackage } from "../hooks/lib/install.mjs";
import { appendActivationLog } from "../hooks/lib/telemetry.mjs";

const sourceRoot = path.resolve(import.meta.dirname, "..");
const cli = path.join(sourceRoot, "hooks", "agent-team-cli.mjs");
const run = promisify(execFile);
const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((item) => rm(item, { force: true, recursive: true }))));

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
  assert.equal(result.conflicts.some(({ kind, reason }) => kind === "skill" && reason === "unowned_target"), true);
  await assert.rejects(readFile(path.join(home, ".codex", "hooks.json")), { code: "ENOENT" });
  const uninstalled = await uninstallPackage({ home, host: "codex", scope: "user" });
  assert.equal(uninstalled.conflicts.some(({ kind, reason }) => kind === "skill" && reason === "unowned_target"), true);
  assert.equal(await readFile(path.join(target, "CUSTOM.md"), "utf8"), "unowned\n");
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

test("an update preserves a package previously recorded as pre-existing", async () => {
  // A later release must not convert a content-identical unowned package into replaceable managed state.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-preexisting-skill-update-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-preexisting-skill-source-"));
  temporary.push(home, changedSource);
  await cp(sourceRoot, changedSource, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });
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
  assert.equal(result.conflicts.some(({ kind, reason }) => kind === "skill" && reason === "preexisting_target"), true);
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

test("an update preserves a Claude role previously recorded as pre-existing", async () => {
  // A changed release role cannot overwrite an identical role that existed before installation.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-preexisting-role-update-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-preexisting-role-source-"));
  temporary.push(home, changedSource);
  await cp(sourceRoot, changedSource, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });
  const target = path.join(home, ".claude", "agents", "agent-team-developer.md");
  await mkdir(path.dirname(target), { recursive: true });
  const original = await readFile(path.join(sourceRoot, "assets", "claude-agents", "agent-team-developer.md"), "utf8");
  await writeFile(target, original);
  await installPackage({ sourceRoot, home, host: "claude-code", scope: "user" });
  const changedRole = path.join(changedSource, "assets", "claude-agents", "agent-team-developer.md");
  await writeFile(changedRole, `${original}\nchanged release\n`);

  const result = await installPackage({ sourceRoot: changedSource, home, host: "claude-code", scope: "user" });
  assert.equal(result.conflicts.some(({ kind, reason, target: name }) => kind === "claude_agent" && reason === "preexisting_definition" && name === target), true);
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

test("handler receipts update exact owned handlers while preserving mixed siblings and customization", async () => {
  // Whole-group ownership or substring matching would delete the sibling or overwrite the customized command.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-handler-home-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-handler-source-"));
  temporary.push(home, changedSource);
  await cp(sourceRoot, changedSource, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });
  const args = { home, host: "codex", scope: "user" };
  await installPackage({ sourceRoot, ...args, now: new Date("2026-09-06T12:00:00.000Z") });

  const configPath = path.join(home, ".codex", "hooks.json");
  const config = JSON.parse(await readFile(configPath, "utf8"));
  const group = config.hooks.PreToolUse.find((entry) => agentTeamGroups({ hooks: { PreToolUse: [entry] } }).length);
  group.label = "preserve-this-group-metadata";
  group.hooks.push({ type: "command", command: "unrelated-policy-check", timeout: 19 });
  await writeFile(configPath, `${JSON.stringify(config, null, 2)}\n`);

  const declarationPath = path.join(changedSource, "hooks", "codex-hooks.json");
  const declaration = JSON.parse(await readFile(declarationPath, "utf8"));
  declaration.hooks.PreToolUse[0].hooks[0].statusMessage = "Updated managed policy check";
  await writeFile(declarationPath, `${JSON.stringify(declaration, null, 2)}\n`);
  const updated = await installPackage({ sourceRoot: changedSource, ...args, now: new Date("2026-09-06T12:01:00.000Z") });
  const afterUpdate = JSON.parse(await readFile(configPath, "utf8"));
  const updatedGroup = afterUpdate.hooks.PreToolUse.find((entry) => entry.hooks.some(({ command }) => command === "unrelated-policy-check"));
  assert.equal(updatedGroup.label, "preserve-this-group-metadata");
  assert.equal(updatedGroup.hooks[0].statusMessage, "Updated managed policy check");
  assert.deepEqual(updated.conflicts, []);

  updatedGroup.hooks[0].command += " --custom-user-argument";
  await writeFile(configPath, `${JSON.stringify(afterUpdate, null, 2)}\n`);
  const reinstalled = await installPackage({ sourceRoot: changedSource, ...args, now: new Date("2026-09-06T12:02:00.000Z") });
  const afterReinstall = JSON.parse(await readFile(configPath, "utf8"));
  assert.equal(afterReinstall.hooks.PreToolUse.some((entry) => entry.hooks.some(({ command }) => command?.endsWith("--custom-user-argument"))), true);
  assert.equal(afterReinstall.hooks.PreToolUse.some((entry) => entry.hooks.some(({ command }) => command === "unrelated-policy-check")), true);
  assert.equal(reinstalled.conflicts.some(({ kind, reason }) => kind === "handler" && reason === "customized_handler"), true);

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

test("an update preserves a pre-existing handler when the new declaration differs", async () => {
  // A release update must not turn an unowned exact handler into managed content and overwrite it.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-preexisting-update-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-preexisting-update-source-"));
  temporary.push(home, changedSource);
  await cp(sourceRoot, changedSource, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });
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
  assert.equal(result.conflicts.some(({ reason }) => reason === "preexisting_handler"), true);
  await uninstallPackage({ home, host: "codex", scope: "user" });
  assert.deepEqual(JSON.parse(await readFile(configPath, "utf8")), original);
  await readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md"));
});

test("an update removes an unchanged owned handler retired by the new declaration", async () => {
  // Rebuilding the receipt from current declarations alone would leave the retired hook live and unowned.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-retired-handler-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-retired-source-"));
  temporary.push(home, changedSource);
  await cp(sourceRoot, changedSource, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });
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
  assert.equal(config.hooks.Interrupt, undefined);
  assert.equal(receipt.handlers.some(({ event }) => event === "Interrupt"), false);
  assert.equal(result.conflicts.some(({ event }) => event === "Interrupt"), false);
});

test("an update retains customized retired handler identity and its package", async () => {
  // A customized retired hook must remain receipted as a conflict so uninstall cannot strand it.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-custom-retired-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-custom-retired-source-"));
  temporary.push(home, changedSource);
  await cp(sourceRoot, changedSource, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });
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
  assert.equal(result.conflicts.some(({ event, reason }) => event === "Interrupt" && reason === "customized_handler"), true);
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

test("installer upgrades an unchanged managed package when the manifest adds a file", async () => {
  // This catches clean upgrades being mistaken for user changes when the package file list grows.
  const home = await homeFixture();
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-expanded-source-"));
  temporary.push(changedSource);
  await cp(sourceRoot, changedSource, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });
  await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });

  const addedFile = "references/new-release-file.md";
  await writeFile(path.join(changedSource, addedFile), "new managed file\n");
  const manifestPath = path.join(changedSource, "hooks", "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  manifest.files.push(addedFile);
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  const result = await installPackage({ sourceRoot: changedSource, home, host: "both", scope: "user", now: new Date("2026-09-06T12:01:00.000Z") });

  assert.equal(result.changed, true);
  assert.deepEqual(result.conflicts, []);
  assert.equal(await readFile(path.join(home, ".agents", "skills", "agent-team", addedFile), "utf8"), "new managed file\n");
  assert.equal(await readFile(path.join(home, ".claude", "skills", "agent-team", addedFile), "utf8"), "new managed file\n");
});

test("uninstall after a managed package update removes the installation instead of restoring V1", async () => {
  // Treating an update snapshot as a pre-install backup would restore V1 and then discard its ownership receipt.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-version-uninstall-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-version-source-"));
  temporary.push(home, changedSource);
  await cp(sourceRoot, changedSource, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });
  await installPackage({ sourceRoot, home, host: "codex", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  const addedFile = "references/version-two-marker.md";
  await writeFile(path.join(changedSource, addedFile), "version two\n");
  const manifestPath = path.join(changedSource, "hooks", "manifest.json");
  const manifest = JSON.parse(await readFile(manifestPath, "utf8"));
  manifest.files.push(addedFile);
  await writeFile(manifestPath, `${JSON.stringify(manifest, null, 2)}\n`);

  await installPackage({ sourceRoot: changedSource, home, host: "codex", scope: "user", now: new Date("2026-09-06T12:01:00.000Z") });
  const receipt = JSON.parse(await readFile(path.join(home, ".agent-team-hooks", "install.json"), "utf8"));
  assert.equal(receipt.version, manifest.version);
  assert.match(receipt.transactionId, /^[0-9a-f-]{36}$/);
  assert.equal(receipt.backups.some(({ kind, purpose }) => kind === "skill" && purpose === "update_snapshot"), true);
  assert.equal(receipt.backups.find(({ kind, purpose }) => kind === "skill" && purpose === "update_snapshot").transactionId, receipt.transactionId);
  await uninstallPackage({ home, host: "codex", scope: "user" });
  await assert.rejects(readFile(path.join(home, ".agents", "skills", "agent-team", "SKILL.md")), { code: "ENOENT" });
});

test("uninstall after an update restores only an original backup from an older receipt", async () => {
  // Update snapshots must not hide a pre-install restoration backup created by an earlier installer version.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-original-backup-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-original-backup-source-"));
  temporary.push(home, changedSource);
  await cp(sourceRoot, changedSource, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });
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

test("installer can run from an authoritative Codex source already at its target path", async () => {
  // This test catches an installer that moves its own source before it copies the Claude package.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-source-home-"));
  temporary.push(home);
  const codexTarget = path.join(home, ".agents", "skills", "agent-team");
  await mkdir(path.dirname(codexTarget), { recursive: true });
  await cp(sourceRoot, codexTarget, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });

  const result = await installPackage({ sourceRoot: codexTarget, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });

  assert.equal(result.status, "installed");
  assert.equal(await readFile(path.join(codexTarget, "SKILL.md"), "utf8").then(Boolean), true);
  assert.equal(await readFile(path.join(home, ".claude", "skills", "agent-team", "SKILL.md"), "utf8").then(Boolean), true);

  await uninstallPackage({ home, host: "both", scope: "user" });
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
  await cp(sourceRoot, brokenSource, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });
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

  const child = spawn(process.execPath, [cli, "install", "--source", sourceRoot, "--home", home, "--host", "codex", "--scope", "user"], {
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

  await run(process.execPath, [cli, "install", "--source", sourceRoot, "--home", home, "--host", "codex", "--scope", "user"]);
  await run(process.execPath, [cli, "rollback", "--home", home, "--host", "codex", "--scope", "user"]);
  assert.equal(await readFile(path.join(legacy, "original.txt"), "utf8"), "original ownership\n");
  await assert.rejects(readFile(path.join(home, ".agent-team-hooks", "transaction.json")), { code: "ENOENT" });
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

test("installer updates a previously managed Claude role and backs up its prior version", async () => {
  // This test catches managed role updates being mistaken for user customization.
  const home = await mkdtemp(path.join(os.tmpdir(), "agent-team-role-update-home-"));
  const changedSource = await mkdtemp(path.join(os.tmpdir(), "agent-team-role-source-"));
  temporary.push(home, changedSource);
  await cp(sourceRoot, changedSource, { recursive: true, filter: (source) => !source.includes(`${path.sep}.git${path.sep}`) });
  await installPackage({ sourceRoot, home, host: "both", scope: "user", now: new Date("2026-09-06T12:00:00.000Z") });
  const roleSource = path.join(changedSource, "assets", "claude-agents", "agent-team-developer.md");
  await writeFile(roleSource, `${await readFile(roleSource, "utf8")}\nManaged update marker.\n`);

  const result = await installPackage({ sourceRoot: changedSource, home, host: "both", scope: "user", now: new Date("2026-09-06T12:01:00.000Z") });
  const installed = await readFile(path.join(home, ".claude", "agents", "agent-team-developer.md"), "utf8");

  assert.match(installed, /Managed update marker/);
  assert.equal(result.backups.some(({ kind }) => kind === "claude_agent"), true);
});
