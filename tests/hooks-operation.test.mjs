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

test("PR integration remains distinct from exact push parsing", () => {
  assert.deepEqual(classifyOperation({ operation: { kind: "shell", command: "gh pr merge 1 --merge" } }), { kind: "integration", method: "pull_request" });
});

test("git push parsing accepts only an exact valid tag target", () => {
  const tag = classifyOperation({ operation: { kind: "shell", command: "git push origin HEAD:refs/tags/v7.1.0" } });
  assert.deepEqual(tag.push, { valid: true, remote: "origin", targetRef: "refs/tags/v7.1.0" });
  for (const command of ["git push origin HEAD:refs/tags/.bad", "git push origin HEAD:refs/tags/v7..1", "git push origin HEAD:refs/tags/v7.1 HEAD:refs/tags/v7.2"]) {
    assert.equal(classifyOperation({ operation: { kind: "shell", command } }).push.valid, false, command);
  }
});
