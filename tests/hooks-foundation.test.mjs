import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtemp, mkdir, readFile, readdir, realpath, rm, symlink, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { normalizeEvent } from "../hooks/lib/event.mjs";
import { resolveProject } from "../hooks/lib/project.mjs";
import { inspectRecovery } from "../hooks/lib/recovery.mjs";
import { writeCheckpoint } from "../hooks/lib/checkpoint.mjs";
import { adaptOutput } from "../hooks/lib/output.mjs";

const temporary = [];

test.afterEach(async () => {
  await Promise.all(temporary.splice(0).map((directory) => rm(directory, { force: true, recursive: true })));
});

async function temporaryDirectory() {
  const directory = await mkdtemp(path.join(os.tmpdir(), "agent-team-hooks-"));
  temporary.push(directory);
  return directory;
}

async function projectFixture() {
  const root = await temporaryDirectory();
  execFileSync("git", ["init", "-q", root]);
  await mkdir(path.join(root, ".agent-team"));
  await writeFile(path.join(root, ".agent-team", "setup.json"), JSON.stringify({
    schemaVersion: 1,
    projectId: "project-1",
    skill: "agent-team",
  }));
  return root;
}

test("Codex apply_patch normalization keeps every add, edit, delete, and move", () => {
  // This test catches a parser that inspects only the first patch section.
  const event = normalizeEvent("codex", "PreToolUse", {
    cwd: "/repo/nested",
    session_id: "codex-session",
    tool_name: "apply_patch",
    tool_input: {
      command: `*** Begin Patch
*** Add File: added.txt
+alpha
*** Update File: edited.txt
@@
-old
+new
*** Delete File: deleted.txt
*** Update File: before.txt
*** Move to: after.txt
@@
-before
+after
*** End Patch`,
    },
  });

  assert.deepEqual(event.operation.files, [
    { action: "add", path: "added.txt", changedContent: "alpha" },
    { action: "edit", path: "edited.txt", changedContent: "new", previousContent: "old" },
    { action: "delete", path: "deleted.txt", changedContent: "" },
    { action: "move", path: "after.txt", previousPath: "before.txt", changedContent: "after", previousContent: "before" },
  ]);
  assert.equal(event.operation.kind, "file_change");
  assert.equal(event.cwd, "/repo/nested");
});

test("Claude Write and Edit payloads normalize to the shared file model", () => {
  // This test catches an adapter that handles only one Claude file tool.
  const write = normalizeEvent("claude", "PreToolUse", {
    cwd: "/repo",
    session_id: "claude-session",
    tool_name: "Write",
    tool_input: { file_path: "new.txt", content: "new file" },
  });
  const edit = normalizeEvent("claude", "PreToolUse", {
    cwd: "/repo",
    session_id: "claude-session",
    tool_name: "Edit",
    tool_input: { file_path: "old.txt", old_string: "before", new_string: "after" },
  });

  assert.deepEqual(write.operation.files, [{ action: "add_or_edit", path: "new.txt", changedContent: "new file" }]);
  assert.deepEqual(edit.operation.files, [{ action: "edit", path: "old.txt", changedContent: "after", previousContent: "before" }]);
});

test("project resolution finds canonical Agent-Team state from nested and symlink paths", async () => {
  // This test catches a resolver that assumes the current directory is the repository root.
  const root = await projectFixture();
  const nested = path.join(root, "one", "two");
  const links = await temporaryDirectory();
  const linked = path.join(links, "linked-project");
  await mkdir(nested, { recursive: true });
  await symlink(root, linked, "dir");

  const nestedProject = await resolveProject(nested);
  const linkedProject = await resolveProject(path.join(linked, "one"));
  const canonicalRoot = await realpath(root);

  assert.equal(nestedProject.active, true);
  assert.equal(nestedProject.root, canonicalRoot);
  assert.equal(linkedProject.root, canonicalRoot);
  assert.equal(nestedProject.projectId, "project-1");
  assert.equal(nestedProject.paths.tasks, path.join(canonicalRoot, ".agent-team", "TASKS.md"));
});

test("recovery labels checkpoint evidence as current, stale, or unavailable without mutation", async () => {
  // This test catches freshness guesses and advisory reads that change project state.
  const root = await projectFixture();
  const checkpointDirectory = path.join(root, ".agent-team", "checkpoints");
  await mkdir(checkpointDirectory);
  const checkpoint = path.join(checkpointDirectory, "session.json");
  const project = await resolveProject(root);

  await writeFile(checkpoint, JSON.stringify({ updatedAt: "2026-09-06T12:00:00.000Z", nextAction: "Run tests." }));
  const before = await readdir(path.join(root, ".agent-team"), { recursive: true });
  const current = await inspectRecovery(project, { now: new Date("2026-09-06T12:04:00.000Z"), staleAfterMs: 300_000 });
  const stale = await inspectRecovery(project, { now: new Date("2026-09-06T12:06:00.001Z"), staleAfterMs: 300_000 });
  await rm(checkpoint);
  const unavailable = await inspectRecovery(project, { now: new Date("2026-09-06T12:07:00.000Z"), staleAfterMs: 300_000 });
  const after = await readdir(path.join(root, ".agent-team"), { recursive: true });

  assert.equal(current.status, "current");
  assert.equal(stale.status, "stale");
  assert.equal(unavailable.status, "unavailable");
  assert.deepEqual(after, before.filter((entry) => entry !== "checkpoints/session.json"));
});

test("checkpoint writes are atomic, idempotent, and preserve authored notes", async () => {
  // This test catches duplicate event writes, partial files, and lost recovery notes.
  const root = await projectFixture();
  const project = await resolveProject(root);
  const original = {
    eventId: "compact-1",
    sessionId: "session-1",
    eventKind: "PreCompact",
    projectId: "project-1",
    teamId: "TEAM-001",
    nextAction: "Run the focused test.",
    decisionNotes: "Use the standard library.",
  };

  const first = await writeCheckpoint(project, original, { now: new Date("2026-09-06T12:00:00.000Z") });
  const repeated = await writeCheckpoint(project, {
    ...original,
    nextAction: "",
    decisionNotes: "",
  }, { now: new Date("2026-09-06T12:01:00.000Z") });
  const stored = JSON.parse(await readFile(first.path, "utf8"));
  const directoryEntries = await readdir(path.dirname(first.path));

  assert.equal(first.created, true);
  assert.equal(repeated.created, false);
  assert.equal(stored.updatedAt, "2026-09-06T12:00:00.000Z");
  assert.equal(stored.nextAction, "Run the focused test.");
  assert.equal(stored.decisionNotes, "Use the standard library.");
  assert.deepEqual(directoryEntries, ["session-1.json"]);
});

test("checkpoint allowlists fields and redacts common secret values", async () => {
  // This test catches credentials, raw SQL, and native payload data leaking into recovery files.
  const root = await projectFixture();
  const project = await resolveProject(root);
  const result = await writeCheckpoint(project, {
    eventId: "compact-secret",
    sessionId: "secret-session",
    eventKind: "PreCompact",
    projectId: "project-1",
    nextAction: "Use token sk-test-secret to continue.",
    evidence: { status: "pending", rawSql: "DELETE FROM customer_records", credential: "private-value" },
    prompt: "customer private content",
  });
  const stored = await readFile(result.path, "utf8");

  assert.equal(stored.includes("sk-test-secret"), false);
  assert.equal(stored.includes("DELETE FROM"), false);
  assert.equal(stored.includes("private-value"), false);
  assert.equal(stored.includes("customer private content"), false);
  assert.match(stored, /\[REDACTED\]/);
});

test("runtime output adapters emit native advisory and deny contracts without ask", () => {
  // This test catches an adapter that emits the forbidden Codex ask decision.
  const denied = {
    mode: "enforce",
    allow: false,
    messages: ["Record current integration evidence before push."],
    context: {},
    capabilities: {},
    mutations: [],
  };
  const advisory = { ...denied, mode: "advisory", allow: true };
  const codex = adaptOutput("codex", "PreToolUse", denied);
  const claude = adaptOutput("claude", "PreToolUse", denied);
  const codexAdvice = adaptOutput("codex", "SessionStart", advisory);

  assert.equal(codex.hookSpecificOutput.permissionDecision, "deny");
  assert.equal(claude.hookSpecificOutput.permissionDecision, "deny");
  assert.equal(JSON.stringify(codex).includes('"ask"'), false);
  assert.match(codexAdvice.hookSpecificOutput.additionalContext, /integration evidence/);
});
