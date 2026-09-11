# Lint Budget Test Stabilization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the delayed-log lint deadline test deterministic under concurrent release-suite load without changing production deadlines or error classification.

**Architecture:** `runLintChecks` gains one optional dependency-injection seam whose default is the existing promisified `execFile` runner. Only the delayed-log test injects an immediate ordinary lint rejection, so it enters the intended log-write phase before the shared event budget expires; all production and other test calls retain the real runner.

**Tech Stack:** Node.js ESM, `node:test`, `child_process.execFile`, Agent-Team event budgets.

**Spec:** `docs/superpowers/specs/2026-09-10-graphify-provenance-release-design.md`

## Global Constraints

- Keep Agent-Team at 7.1.1 and Graphify pinned to `graphifyy==0.9.57`.
- Do not increase timeouts, add retries, serialize the suite, broaden assertions, or change production deadline/error behavior.
- Modify only `hooks/lib/lint.mjs` and `tests/hooks-lint.test.mjs` for the stabilization.
- Preserve the existing production call in `hooks/lib/policy.mjs` without source changes.
- Require 16/16 passes under the recorded contention probe and one fresh complete hook-suite pass.

---

### Task 1: Inject the lint command runner only in the delayed-log test

**Files:**
- Modify: `hooks/lib/lint.mjs`
- Modify: `tests/hooks-lint.test.mjs`
- Test: `tests/hooks-budget.test.mjs`

**Interfaces:**
- Consumes: the current `runLintChecks(root, changedFiles, options)` and its promisified `execFile` adapter `run`.
- Produces: optional `options.runCommand`, defaulting reference-identically to `run`, with the same promise result and rejection fields.

- [ ] **Step 1: Preserve the recorded failing contention evidence**

Use the final-review evidence at candidate `7b237ba`: the named test passed 10/10 serially but failed 5/16 and 4/16 in two bounded 16-way runs with `timeout` instead of the intended `failed` result.

- [ ] **Step 2: Add the minimal runner seam**

Change only the options signature and invocation identifier:

```js
export async function runLintChecks(root, changedFiles, {
  timeoutMs = 3000,
  maxOutputBytes = 4096,
  executableName = "eslint",
  budget,
  filesystem = {},
  progress,
  runCommand = run,
} = {}) {
```

```js
const { stdout, stderr } = await runCommand(binary, targets, {
  cwd,
  timeout: budget?.timeout(timeoutMs) ?? timeoutMs,
  maxBuffer: 1024 * 1024,
  encoding: "utf8",
  ...(budget ? { signal: budget.signal } : {}),
});
```

Do not rename or export `run`, add another abstraction, or alter the catch path.

- [ ] **Step 3: Inject a deterministic ordinary lint failure in only the named test**

Keep the executable fixture and existing assertions. Add this option to the delayed-log test call:

```js
runCommand: async () => {
  throw Object.assign(new Error("lint failed"), {
    code: 1,
    stdout: "index.js:1:1 broken",
    stderr: "",
  });
},
```

- [ ] **Step 4: Run focused and contention checks**

```bash
node --test --test-name-pattern='delayed log-directory creation cannot start a log write after event deadline' tests/hooks-lint.test.mjs
node --test tests/hooks-lint.test.mjs
node --test tests/hooks-budget.test.mjs
seq 1 16 | xargs -P16 -I RUN_ID sh -c '
  out=$(node --test --test-name-pattern="delayed log-directory creation cannot start a log write after event deadline" tests/hooks-lint.test.mjs 2>&1)
  code=$?
  summary=$(printf "%s\n" "$out" | rg "ℹ (pass|fail) " | tr "\n" ",")
  printf "parallel-run=%s exit=%s %s\n" RUN_ID "$code" "$summary"
'
```

Require every exit to be zero and 16/16 total passes.

- [ ] **Step 5: Run complete release-source gates**

```bash
node --test tests/hooks-dependencies.test.mjs
AGENT_TEAM_REAL_DEPS=1 node --test --test-name-pattern='real isolated Graphify package builds and traverses an offline code graph' tests/hooks-dependencies-real.test.mjs
node --test tests/hooks-docs.test.mjs tests/hooks-artifacts.test.mjs
node --test tests/hooks-*.test.mjs
node hooks/agent-team-cli.mjs check-package
python3 /home/server/.codex/skills/.system/skill-creator/scripts/quick_validate.py .
git diff --check
```

- [ ] **Step 6: Commit the stabilization**

```bash
git add hooks/lib/lint.mjs tests/hooks-lint.test.mjs
git commit -m "test: stabilize lint deadline coverage"
```
