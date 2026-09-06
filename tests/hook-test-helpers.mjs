import { execFileSync } from "node:child_process";
import { mkdir, writeFile } from "node:fs/promises";
import path from "node:path";

export async function policyFixture(root, { now = "2026-09-06T12:00:00.000Z" } = {}) {
  execFileSync("git", ["init", "-q", "-b", "main", root]);
  execFileSync("git", ["config", "user.name", "Hook Test"], { cwd: root });
  execFileSync("git", ["config", "user.email", "hook@example.test"], { cwd: root });
  await writeFile(path.join(root, ".gitignore"), ".agent-team/\n");
  await writeFile(path.join(root, "README.md"), "fixture\n");
  await mkdir(path.join(root, "src"));
  await writeFile(path.join(root, "src", "owned.js"), "export {};\n");
  execFileSync("git", ["add", "."], { cwd: root });
  execFileSync("git", ["commit", "-q", "-m", "fixture"], { cwd: root });
  const remote = `${root}-remote`;
  execFileSync("git", ["init", "-q", "--bare", remote]);
  execFileSync("git", ["remote", "add", "origin", remote], { cwd: root });
  execFileSync("git", ["push", "-q", "-u", "origin", "main"], { cwd: root });
  const feature = `${root}-feature`;
  execFileSync("git", ["worktree", "add", "-q", "-b", "feature", feature], { cwd: root });
  execFileSync("git", ["push", "-q", "-u", "origin", "feature"], { cwd: feature });
  const revision = execFileSync("git", ["rev-parse", "HEAD"], { cwd: feature, encoding: "utf8" }).trim();

  await mkdir(path.join(root, ".agent-team"));
  await writeFile(path.join(root, ".agent-team", "setup.json"), JSON.stringify({
    schemaVersion: 1,
    projectId: "project-1",
    skill: "agent-team",
  }));
  await writeFile(path.join(root, ".agent-team", "TEAMS.md"), `# Agent-Team teams
Project: project-1
Project owner: owner-session
Integration owner: owner-session

| Team ID | Name | Session | Worktree | Branch | Owned paths | Tasks | Status |
| --- | --- | --- | --- | --- | --- | --- | --- |
| TEAM-001 | lifecycle-hooks | developer-session | ${feature} | feature | src/** | AT-001 | in_progress |
`);
  await writeFile(path.join(root, ".agent-team", "TASKS.md"), `# Agent-Team Tasks
| ID | Requirement / acceptance | Owner | Depends on | Status | Revision / evidence | Next action |
| --- | --- | --- | --- | --- | --- | --- |
| AT-001 | Complete fixture work | TEAM-001 | none | in_progress | ${revision} | Verify. |
`);

  const operationMappings = {
    providers: {
      mcp__database__execute: { kind: "database_destructive", sqlField: "query" },
      mcp__filesystem__write: { kind: "file_change", pathField: "path", contentField: "content", action: "edit" },
    },
    shell: [
      { prefix: "node scripts/reset-data.mjs", kind: "database_destructive" },
    ],
  };
  const state = {
    schemaVersion: 1,
    integration: {
      ownerSessionId: "owner-session",
      authorized: true,
      expectedRevision: revision,
      baseRef: "main",
      remoteName: "origin",
      baseRemoteRef: "refs/heads/main",
      baseRevision: revision,
      remoteRef: "refs/heads/feature",
      remoteRevision: revision,
      evidenceAt: now,
      preview: { required: false },
      paused: false,
      hold: false,
      recoveryReconciled: true,
      deltaClean: true,
      updatesRemoteMain: false,
      remoteMainDeploys: false,
    },
    release: {
      ownerSessionId: "owner-session",
      authorized: true,
      expectedRevision: revision,
      evidenceAt: now,
      target: "test-registry",
      process: "npm",
      authorization: {
        source: "user-request",
        target: "test-registry",
        process: "npm",
        scope: "batch-1",
        grantedAt: now,
      },
      runMode: "auto_deploy",
      batchId: "batch-1",
      taskIds: ["AT-001"],
      batch: { id: "batch-1", taskIds: ["AT-001"] },
      artifact: { id: "artifact-1", revision, taskIds: ["AT-001"] },
      integration: { status: "passed", revision, taskIds: ["AT-001"] },
      verification: { status: "passed", revision, taskIds: ["AT-001"] },
      preview: { required: false, status: "not_required", revision },
      delta: { status: "clean", revision, taskIds: ["AT-001"] },
      recovery: { status: "verified", artifactId: "artifact-0", action: "rollback" },
      recoveryReady: true,
      autoDeploy: true,
      hold: false,
    },
    database: {
      ownerSessionId: "owner-session",
      authorized: true,
      environment: "staging",
      productionApproved: false,
      fixtureTarget: false,
      inventoryAt: now,
      recovery: { verifiedAt: now, kind: "backup" },
      dryRun: { capability: "supported", verifiedAt: now },
      allowCascade: true,
    },
    completion: {
      taskId: "AT-001",
      evidenceRevision: revision,
      requirementsReconciled: true,
      review: { status: "passed", revision },
      checks: [{ name: "unit", status: "passed", revision }],
      scope: { deployment: false, cleanup: false },
    },
    operationMappings,
  };
  await writeFile(path.join(root, ".agent-team", "state.json"), JSON.stringify(state, null, 2));
  await writeFile(path.join(root, ".agent-team", "operation-mappings.json"), JSON.stringify({
    schemaVersion: 1,
    operationMappings,
  }, null, 2));
  return { root, feature, remote, revision, state, operationMappings };
}

export async function saveState(fixture, state) {
  await writeFile(path.join(fixture.root, ".agent-team", "state.json"), JSON.stringify(state, null, 2));
}

export function hookEvent(fixture, { sessionId = "developer-session", operation, event = "PreToolUse", cwd = fixture.feature } = {}) {
  return { runtime: "codex", event, cwd, sessionId, eventId: "event-1", operation };
}
