import assert from "node:assert/strict";
import { mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import { resolveProject } from "../hooks/lib/project.mjs";
import { loadCanonicalState } from "../hooks/lib/canonical-state.mjs";
import { evaluatePolicy } from "../hooks/lib/policy.mjs";
import { policyFixture, hookEvent } from "./hook-test-helpers.mjs";

const temporary = [];
test.afterEach(async () => Promise.all(temporary.splice(0).map((p) => rm(p, { recursive: true, force: true }))));
async function fixture(tracker) {
  const root = await mkdtemp(path.join(os.tmpdir(), "agent-team-tracker-"));
  temporary.push(root, `${root}-feature`, `${root}-remote`);
  const result = await policyFixture(root);
  await writeFile(path.join(root, ".agent-team/setup.json"), JSON.stringify({ skill: "agent-team", projectId: "project-1", tracker }));
  return result;
}
const beads = async () => ({ stdout: JSON.stringify([{ id: "AT-001", title: "Complete fixture work", assignee: "TEAM-001", status: "in_progress", updated_at: "2026-09-08T10:00:00Z" }]) });

test("explicit selected Beads executable is canonical across linked worktrees; invalid explicit paths never fall back", async () => {
  const value = await fixture({ kind: "beads", executable: "/selected/bin/bd" });
  const main = await resolveProject(value.root);
  const linked = await resolveProject(value.feature);
  assert.deepEqual(main.tracker, linked.tracker);
  assert.equal(linked.tracker.executable, "/selected/bin/bd");
  const current = await loadCanonicalState(linked, { runBeads: async (binary) => { assert.equal(binary, "/selected/bin/bd"); return beads(); } });
  assert.equal(current.tracker.executableSource, "selected");
  for (const executable of ["bd", "", null]) {
    await writeFile(main.paths.setup, JSON.stringify({ skill: "agent-team", tracker: { kind: "beads", executable } }));
    const invalid = await loadCanonicalState(await resolveProject(value.feature));
    assert.equal(invalid.tracker.status, "unavailable");
    assert.equal(invalid.tracker.reason, "invalid_selection");
  }
});

for (const tracker of [{ kind: "markdown", path: "TASKS.md" }, { kind: "markdown", path: ".agent-team/TASKS.md" }, { kind: "beads" }]) {
  test(`selected ${JSON.stringify(tracker)} shares authority and completion gates across worktrees (adapter fixture)`, async () => {
    // Catches hardcoded Markdown selection, lost Beads owner mapping, and worktree-local authority.
    const value = await fixture(tracker);
    if (tracker.path === "TASKS.md") await writeFile(path.join(value.root, "TASKS.md"), await readFile(path.join(value.root, ".agent-team/TASKS.md")));
    const main = await resolveProject(value.root);
    const linked = await resolveProject(value.feature);
    assert.equal(main.paths.tasks, path.join(value.root, tracker.path ?? ".beads"));
    assert.deepEqual(main.tracker, linked.tracker);
    const canonical = await loadCanonicalState(linked, { runBeads: beads });
    assert.equal(canonical.tracker.status, "current");
    assert.match(canonical.tracker.fingerprint, /^[a-f0-9]{64}$/);
    assert.deepEqual(canonical.tasks.map(({ id, owner, status }) => ({ id, owner, status })), [{ id: "AT-001", owner: "TEAM-001", status: "in_progress" }]);
    const event = hookEvent(value, { operation: { kind: "completion", taskId: "AT-001" } });
    assert.equal((await evaluatePolicy(event, linked, { canonical })).allow, true);
    for (const command of ["npm publish", "git push origin feature"]) {
      const release = hookEvent(value, { sessionId: "owner-session", operation: { kind: "shell", command } });
      assert.equal((await evaluatePolicy(release, linked, { canonical, now: new Date("2026-09-06T12:00:00Z") })).allow, true);
    }
    canonical.tasks[0].owner = "TEAM-OTHER";
    assert.equal((await evaluatePolicy(event, linked, { canonical })).allow, false);
  });
}

test("invalid explicit tracker selectors cannot fall back to a legacy passing tracker", async () => {
  const value = await fixture({ kind: "markdown", path: "../TASKS.md" });
  const canonical = await loadCanonicalState(await resolveProject(value.root));
  assert.equal(canonical.tracker.status, "unavailable");
  assert.equal(canonical.tracker.reason, "invalid_selection");
  assert.deepEqual(canonical.tasks, []);
});

for (const [name, runBeads, reason] of [
  ["missing executable", async () => { throw Object.assign(new Error("missing"), { code: "ENOENT" }); }, "missing_executable"],
  ["backend failure", async () => { throw Object.assign(new Error("backend unavailable"), { code: 1 }); }, "backend_failed"],
  ["invalid JSON", async () => ({ stdout: "{bad" }), "invalid_response"],
  ["invalid rows", async () => ({ stdout: '[{"id":"AT-001"}]' }), "invalid_response"],
  ["timeout", async () => { throw Object.assign(new Error("timed out"), { killed: true }); }, "timeout"],
]) {
  test(`Beads ${name} holds completion while repair remains available (subprocess seam)`, async () => {
    const value = await fixture({ kind: "beads" });
    const project = await resolveProject(value.feature);
    const canonical = await loadCanonicalState(project, { runBeads });
    assert.equal(canonical.tracker.status, "unavailable");
    assert.equal(canonical.tracker.reason, reason);
    const completion = await evaluatePolicy(hookEvent(value, { operation: { kind: "completion", taskId: "AT-001" } }), project, { canonical });
    assert.equal(completion.allow, false);
    assert.match(completion.messages.join(" "), /tracker.*unavailable/i);
    const repair = await evaluatePolicy(hookEvent(value, { operation: { kind: "file_change", files: [{ path: "src/owned.js", changedContent: "export {};" }] } }), project, { canonical });
    assert.equal(repair.allow, true);
  });
}

test("only the selected canonical TASKS path is a completion transition and shared owner path", async () => {
  const value = await fixture({ kind: "markdown", path: "TASKS.md" });
  await writeFile(path.join(value.root, "TASKS.md"), await readFile(path.join(value.root, ".agent-team/TASKS.md")));
  const project = await resolveProject(value.feature);
  const ordinary = await evaluatePolicy(hookEvent(value, { operation: { kind: "file_change", files: [{ path: "src/TASKS.md", changedContent: "| OTHER | verified |" }] } }), project);
  assert.equal(ordinary.allow, true);
  const shared = await evaluatePolicy(hookEvent(value, { operation: { kind: "file_change", files: [{ path: path.join(value.root, "TASKS.md"), changedContent: "| AT-001 | TEAM-001 | in_progress |" }] } }), project);
  assert.equal(shared.allow, false);
});

test("tracker fingerprints change after same-length content edits", async () => {
  const value = await fixture({ kind: "markdown", path: ".agent-team/TASKS.md" });
  const project = await resolveProject(value.root);
  const first = await loadCanonicalState(project);
  const source = await readFile(project.paths.tasks, "utf8");
  await writeFile(project.paths.tasks, source.replace("TEAM-001", "TEAM-002"));
  const second = await loadCanonicalState(project);
  assert.notEqual(first.tracker?.fingerprint, second.tracker?.fingerprint);
});

test("empty or malformed Markdown is unavailable, while an explicit empty task table is current", async () => {
  const value = await fixture({ kind: "markdown", path: ".agent-team/TASKS.md" });
  const project = await resolveProject(value.root);
  for (const source of ["", "not a tracker", "| ID | Owner | Status |\n| --- | --- | --- |\n| AT-001 | TEAM-001 | |\n"]) {
    await writeFile(project.paths.tasks, source);
    assert.equal((await loadCanonicalState(project)).tracker.status, "unavailable");
  }
  await writeFile(project.paths.tasks, "| ID | Owner | Status |\n| --- | --- | --- |\n");
  assert.equal((await loadCanonicalState(project)).tracker.status, "current");
});

test("recorded tracker fingerprints invalidate completion evidence after task changes", async () => {
  const value = await fixture({ kind: "markdown", path: ".agent-team/TASKS.md" });
  const project = await resolveProject(value.feature);
  const first = await loadCanonicalState(project);
  value.state.completion.trackerFingerprint = first.tracker.fingerprint;
  await writeFile(project.paths.state, JSON.stringify(value.state));
  await writeFile(project.paths.tasks, (await readFile(project.paths.tasks, "utf8")).replace("Complete fixture work", "Complete changed work"));
  const result = await evaluatePolicy(hookEvent(value, { operation: { kind: "completion", taskId: "AT-001" } }), project);
  assert.equal(result.allow, false);
  assert.match(result.messages.join(" "), /tracker.*stale|tracker.*changed/i);
});

test("Beads bounded read includes closed issues and pins canonical project authority", async () => {
  const value = await fixture({ kind: "beads" });
  let call;
  const canonical = await loadCanonicalState(await resolveProject(value.feature), { runBeads: async (...args) => {
    call = args;
    return { stdout: '[{"id":"AT-002","title":"Closed task","assignee":"TEAM-001","status":"closed"}]' };
  } });
  assert.equal(canonical.tasks[0].status, "closed");
  assert.deepEqual(call.slice(0, 2), ["bd", ["list", "--all", "--limit", "0", "--json", "--readonly"]]);
  assert.equal(call[2].cwd, value.root);
  assert.equal(call[2].env.BEADS_DIR, path.join(value.root, ".beads"));
  assert.ok(call[2].timeout > 0 && call[2].timeout <= 1500);
  assert.ok(call[2].maxBuffer <= 1024 * 1024);
});

test("mapped provider tracker edits cannot bypass completion verification", async () => {
  const value = await fixture({ kind: "markdown", path: "TASKS.md" });
  await writeFile(path.join(value.root, "TASKS.md"), await readFile(path.join(value.root, ".agent-team/TASKS.md")));
  value.state.completion.checks = [{ name: "lint", status: "failed", revision: value.revision }];
  await writeFile(path.join(value.root, ".agent-team/state.json"), JSON.stringify(value.state));
  const result = await evaluatePolicy(hookEvent(value, { cwd: value.root, sessionId: "owner-session", operation: {
    kind: "provider", tool: "mcp__filesystem__write", input: { path: "TASKS.md", content: "| AT-001 | TEAM-001 | verified |" },
  } }), await resolveProject(value.root));
  assert.equal(result.allow, false);
  assert.match(result.messages.join(" "), /check evidence/i);
});

test("selected Beads close and closed-status commands use the completion gate", async () => {
  const value = await fixture({ kind: "beads" });
  const project = await resolveProject(value.feature);
  value.state.completion.checks = [{ name: "lint", status: "failed", revision: value.revision }];
  await writeFile(project.paths.state, JSON.stringify(value.state));
  for (const command of ["bd close AT-001", "bd update AT-001 --status closed", "bd close AT-001 AT-002"]) {
    const result = await evaluatePolicy(hookEvent(value, { operation: { kind: "shell", command } }), project, { runBeads: beads });
    assert.equal(result.allow, false, command);
    assert.match(result.messages.join(" "), /completion|check evidence/i);
  }
});

test("mapped provider final tracker writes preserve shared project-owner enforcement", async () => {
  const value = await fixture({ kind: "markdown", path: ".agent-team/TASKS.md" });
  const project = await resolveProject(value.feature);
  const result = await evaluatePolicy(hookEvent(value, { operation: { kind: "provider", tool: "mcp__filesystem__write", input: {
    path: project.paths.tasks, content: "| AT-001 | TEAM-001 | verified |",
  } } }), project);
  assert.equal(result.allow, false);
  assert.match(result.messages.join(" "), /project owner|shared path/i);
});

test("Markdown transitions compare terminal status per task ID and reject multiple new completions", async () => {
  const value = await fixture({ kind: "markdown", path: ".agent-team/TASKS.md" });
  const project = await resolveProject(value.root);
  for (const [previousContent, changedContent] of [
    ["| AT-001 | TEAM-001 | verified |\n| AT-002 | TEAM-001 | in_progress |", "| AT-001 | TEAM-001 | verified |\n| AT-002 | TEAM-001 | verified |"],
    ["", "| AT-001 | TEAM-001 | verified |\n| AT-002 | TEAM-001 | verified |"],
  ]) {
    const result = await evaluatePolicy(hookEvent(value, { cwd: value.root, sessionId: "owner-session", operation: { kind: "file_change", files: [{ path: project.paths.tasks, previousContent, changedContent }] } }), project);
    assert.equal(result.allow, false);
    assert.match(result.messages.join(" "), /completion|task owner/i);
  }
});

test("invalid separators and blank task IDs cannot create empty current tracker evidence", async () => {
  const value = await fixture({ kind: "markdown", path: ".agent-team/TASKS.md" });
  const project = await resolveProject(value.root);
  for (const source of [
    "| ID | Owner | Status |\n| nonsense | nonsense | nonsense |\n| AT-001 | TEAM-001 | in_progress |\n",
    "| ID | Owner | Status |\n| --- | --- | --- |\n| | TEAM-001 | in_progress |\n",
    "| ID | Owner | Status |\n",
  ]) {
    await writeFile(project.paths.tasks, source);
    assert.equal((await loadCanonicalState(project)).tracker.status, "unavailable");
  }
});

test("Beads normalization preserves explicit priority, dependency IDs and parent without inference", async () => {
  const value = await fixture({ kind: "beads" });
  const project = await resolveProject(value.root);
  const canonical = await loadCanonicalState(project, { runBeads: async () => ({ stdout: JSON.stringify([
    { id: "work-abc.2", title: "Child", status: "open", priority: 1, parent: "work-abc", dependencies: [{ issue_id: "work-abc.2", depends_on_id: "work-xyz", type: "blocks" }] },
    { id: "work-def.3", title: "Unrelated dotted ID", status: "open", priority: 3 },
  ]) }) });
  assert.deepEqual(canonical.tasks[0].dependencies, ["work-xyz"]);
  assert.equal(canonical.tasks[0].parent, "work-abc");
  assert.equal(canonical.tasks[0].priority, 1);
  assert.equal(canonical.tasks[1].parent, undefined);
  assert.equal(canonical.tasks[1].dependencyEvidence, "unavailable");
});
