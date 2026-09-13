import assert from "node:assert/strict";
import test from "node:test";

import { classifyOperation } from "../hooks/lib/operation.mjs";

test("git push parsing binds one non-force HEAD refspec to its remote", () => {
  const accepted = classifyOperation({ operation: { kind: "shell", command: "git -C /repo push origin HEAD:main" } });
  assert.deepEqual(accepted, { kind: "integration", repository: "/repo", method: "push",
    push: { valid: true, remote: "origin", targetRef: "refs/heads/main" } });
  for (const command of [
    "git push origin +HEAD:main",
    "git push --force-with-lease=refs/heads/main:abc origin HEAD:main",
    "git push --force-if-includes origin HEAD:main",
    "git push -fv origin HEAD:main",
    "git push origin HEAD:main HEAD:other",
    "git push origin HEAD:main ; git status",
    "echo preflight && git push origin HEAD:main",
  ]) {
    const parsed = classifyOperation({ operation: { kind: "shell", command } });
    assert.equal(parsed.kind, "integration");
    assert.equal(parsed.method, "push");
    assert.equal(parsed.push.valid, false, command);
  }
});

test("git push parsing fails closed on every malformed or alternate global option form", () => {
  for (const command of [
    "GIT_DIR=/tmp/other.git git push origin HEAD:main",
    "env GIT_DIR=/tmp/other.git git push origin HEAD:main",
    "git --git-dir=/repo/.git push origin HEAD:main",
    "git --work-tree /repo push origin HEAD:main",
    "git --namespace tenant push origin HEAD:main",
    "git -c protocol.version=2 push origin HEAD:main",
    "git -C /repo -C /other push origin HEAD:main",
    "git -C push origin HEAD:main",
    "git -C -- push origin HEAD:main",
    "git -- push origin HEAD:main",
    "git -c alias.deploy='push origin HEAD:main' deploy",
  ]) {
    const parsed = classifyOperation({ operation: { kind: "shell", command } });
    assert.equal(parsed.kind, "integration", command);
    assert.equal(parsed.method, "push", command);
    assert.equal(parsed.push.valid, false, command);
  }
  assert.deepEqual(classifyOperation({ operation: { kind: "shell",
    command: 'lean-ctx -c --raw "git -C /repo push origin HEAD:main"' } }), {
    kind: "integration", repository: "/repo", method: "push",
    push: { valid: true, remote: "origin", targetRef: "refs/heads/main" },
  });
});

test("PR integration remains distinct from exact push parsing", () => {
  assert.deepEqual(classifyOperation({ operation: { kind: "shell", command: "gh pr merge 1 --merge" } }), { kind: "integration", method: "pull_request" });
});

test("owner-only native commands require exact non-chained CLI forms", () => {
  const cli = "/opt/agent-team/hooks/agent-team-cli.mjs";
  for (const [nativeCommand, request] of [
    ["legacy-owner-adopt", "/srv/adopt.json"],
    ["gate-evidence", "/srv/gate.json"],
    ["run-reconcile", "/srv/reconcile.json"],
    ["run-scope-extend", "/srv/scope.json"],
  ]) assert.deepEqual(classifyOperation({ operation: { kind: "shell", command:
    `node ${cli} ${nativeCommand} --project /srv/project --request ${request}` } }), {
    kind: "agent_team_native_command", command: nativeCommand, cliPath: cli,
    project: "/srv/project", request, valid: true,
  });
  for (const command of [
    `node ${cli} legacy-owner-adopt --request /srv/adopt.json --project /srv/project`,
    `node ${cli} legacy-owner-adopt --project relative --request /srv/adopt.json`,
    `node ${cli} legacy-owner-adopt --project /srv/project --request /srv/adopt.json ; true`,
    `lean-ctx raw "node ${cli} legacy-owner-adopt --project /srv/project --request /srv/adopt.json"`,
    `node ${cli} run-reconcile --request /srv/reconcile.json --project /srv/project`,
    `node ${cli} run-reconcile --project /srv/project --request /srv/reconcile.json ; true`,
    `node ${cli} run-scope-extend --project relative --request /srv/scope.json`,
    `lean-ctx raw "node ${cli} run-scope-extend --project /srv/project --request /srv/scope.json"`,
    `node ${cli} status --project /srv/project`,
  ]) {
    assert.notEqual(classifyOperation({ operation: { kind: "shell", command } }).kind, "agent_team_native_command", command);
  }
});

test("git push parsing preserves one exact local tag object and target identity", () => {
  for (const command of [
    "git push origin refs/tags/v7.2.0",
    "git push origin refs/tags/v7.2.0:refs/tags/v7.2.0",
  ]) {
    const tag = classifyOperation({ operation: { kind: "shell", command } });
    assert.deepEqual(tag.push, {
      valid: true,
      remote: "origin",
      sourceRef: "refs/tags/v7.2.0",
      targetRef: "refs/tags/v7.2.0",
    }, command);
  }
  for (const command of [
    "git push origin HEAD:refs/tags/v7.2.0",
    "git push origin :refs/tags/v7.2.0",
    "git push --delete origin refs/tags/v7.2.0",
    "git push -d origin refs/tags/v7.2.0",
    "git push --mirror origin refs/tags/v7.2.0",
    "git push origin refs/heads/main:refs/tags/v7.2.0",
    "git push origin refs/tags/v7.2.0:refs/tags/v7.2.1",
    "git push origin refs/tags/.bad",
    "git push origin refs/tags/v/.bad",
    "git push origin refs/tags/v7.2.0.lock",
    "git push origin refs/tags/v7..2",
    "git push origin refs/tags/v7.2.0 refs/tags/v7.2.1",
    "git push origin refs/tags/*:refs/tags/*",
  ]) {
    assert.equal(classifyOperation({ operation: { kind: "shell", command } }).push.valid, false, command);
  }
});
