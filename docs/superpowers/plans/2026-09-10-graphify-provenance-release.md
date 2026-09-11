# Graphify Provenance and Paired Skill Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Correct Agent-Team's Graphify provenance gate and publish verified Project Kickoff 0.4.1 and Agent-Team 7.1.1 releases.

**Architecture:** Project Kickoff remains a declarative producer of `plan.requiredCapabilities`; Agent-Team remains the sole owner of Graphify preparation and readiness. Agent-Team's functional probe will distinguish AST versus semantic provenance with `_origin`, while `confidence` remains a resolution-strength field that may legitimately be `INFERRED` for deterministic AST relationships.

**Tech Stack:** Node.js ESM and `node:test`, Python fixture source, Graphify CLI 0.9.57, Git worktrees, GitHub CLI, GitHub Actions, Project Kickoff Python `unittest` package checks.

**Spec:** `docs/superpowers/specs/2026-09-10-graphify-provenance-release-design.md`

## Global Constraints

- Agent-Team releases as 7.1.1; Project Kickoff releases as 0.4.1.
- Keep Graphify pinned to `graphifyy==0.9.57`; version 0.9.58 is unrelated to this defect.
- Project Kickoff must not install, initialize, execute, register, or evaluate Graphify.
- Agent-Team must run `graphify extract . --code-only --no-viz` in a fresh isolated worktree graph with backend environment variables scrubbed.
- `_origin: "ast"` is accepted; `_origin: "semantic"` or missing provenance fails the offline functional gate.
- `confidence: "INFERRED"` is valid when `_origin` is `ast` and remains a structural lead, not behavioral proof.
- Do not weaken deadlines or add blind retries to make the full Agent-Team suite green.
- Do not force-push, reuse a tag, duplicate an ambiguous release operation, or publish an unverified revision.
- Publish and verify Project Kickoff 0.4.1 before Agent-Team 7.1.1 links to it.
- Agent-Team's existing tag workflow owns its GitHub release; Project Kickoff's release is created explicitly with GitHub CLI.

---

### Task 1: Agent-Team Graphify provenance gate

**Files:**
- Modify: `hooks/lib/dependencies.mjs:767`
- Modify: `tests/hooks-dependencies.test.mjs:307`
- Test: `tests/hooks-dependencies-real.test.mjs:146`

**Interfaces:**
- Consumes: private `graphifyFunctional(executable, paths, budget)` and the pinned Graphify 0.9.57 graph schema.
- Produces: `{status: "passed" | "failed", evidence: string}` with AST-provenance validation and a known callback-edge compatibility assertion.

- [ ] **Step 1: Add failing fake-Graphify tests for valid AST inference and invalid semantic provenance**

Change the fake edge helper to emit explicit confidence and provenance:

```js
const edge = (source, target, relation, {
  confidence = "EXTRACTED",
  confidenceScore = 1.0,
  origin = "ast",
  context,
} = {}) => ({
  source,
  target,
  relation,
  confidence,
  confidence_score: confidenceScore,
  _origin: origin,
  ...(context ? { context } : {}),
});
```

Give every fake node `_origin: "ast"`. Add `dispatch()` and `handler()` nodes and this edge to the complete fixture:

```js
edge("app_dispatch", "app_handler", "indirect_call", {
  confidence: "INFERRED",
  confidenceScore: 0.85,
  origin: variant === "semantic-origin" ? "semantic" : "ast",
  context: "argument",
})
```

Assert that `complete` succeeds through `extract`, `path`, and `explain`; `semantic-origin` fails after `extract` with a provenance-specific message; `missing-origin` fails closed; and `missing-edge` retains its current failure.

- [ ] **Step 2: Run the fake dependency test and confirm the new success case fails under 7.1.0**

Run:

```bash
node --test --test-name-pattern='default Graphify gate extracts an offline code graph and traverses it' tests/hooks-dependencies.test.mjs
```

Expected before implementation: failure because 7.1.0 rejects the AST-origin `INFERRED` callback edge.

- [ ] **Step 3: Add the real callback compatibility fixture**

Add this Python to the isolated `app.py` fixture in `graphifyFunctional`:

```python
def handler(value):
    return value


def dispatch(values):
    return list(map(handler, values))
```

Add a separate descriptor without changing the existing ordered path chain:

```js
const GRAPHIFY_FIXTURE_CALLBACK = {
  from: "dispatch()",
  to: "handler()",
  relation: "indirect_call",
  confidence: "INFERRED",
  confidenceScore: 0.85,
  origin: "ast",
};
```

- [ ] **Step 4: Replace confidence-only rejection with fail-closed provenance validation**

Validate all ordinary nodes and links:

```js
const graphItems = [...(graph.nodes ?? []), ...(graph.links ?? [])];
const nonAst = graphItems.find(({ _origin }) => _origin !== "ast");
if (nonAst) {
  return {
    status: "failed",
    evidence: `Graphify code-only extraction emitted ${nonAst._origin ?? "missing"} provenance; expected AST-origin graph content only.`,
  };
}
```

After label lookup, require `dispatch() --indirect_call--> handler()` with confidence `INFERRED`, score `0.85`, origin `ast`, and a distinct diagnostic for missing callback labels. Update pass evidence to say the graph is AST-origin and includes deterministic inferred callback resolution.

- [ ] **Step 5: Run focused and real pinned-dependency tests**

Run:

```bash
node --test tests/hooks-dependencies.test.mjs
AGENT_TEAM_REAL_DEPS=1 node --test --test-name-pattern='real isolated Graphify package builds and traverses an offline code graph' tests/hooks-dependencies-real.test.mjs
git diff --check
```

Expected: all selected tests pass; the real test observes the AST-origin `INFERRED` callback edge from Graphify 0.9.57.

- [ ] **Step 6: Commit the core repair**

```bash
git add hooks/lib/dependencies.mjs tests/hooks-dependencies.test.mjs tests/hooks-dependencies-real.test.mjs
git commit -m "fix: validate Graphify extraction provenance"
```

---

### Task 2: Agent-Team 7.1.1 documentation and package surfaces

**Files:**
- Modify: `references/graphify.md:18`
- Modify: `SKILL.md:4`
- Modify: `hooks/manifest.json:4`
- Modify: `CHANGELOG.md:1`
- Modify: `README.md:5`
- Modify: `README.md:258`
- Modify: `README.md:267`
- Modify: `index.html:44`
- Modify: `tests/hooks-artifacts.test.mjs:31`
- Modify: `tests/hooks-artifacts.test.mjs:198`

**Interfaces:**
- Consumes: Task 1's `_origin`-based behavior and published Project Kickoff tag `v0.4.1`.
- Produces: coherent 7.1.1 package metadata, documentation, archive expectations, and a valid Project Kickoff 0.4.1 install link.

- [ ] **Step 1: Write failing version and documentation assertions**

Update artifact expectations to require:

```js
assert.equal(path.basename(result.archive), "agent-team-7.1.1.zip");
assert.equal(result.source.releaseTag, "v7.1.1");
```

Add or update documentation assertions so Graphify guidance contains both `_origin` provenance and AST-derived `INFERRED` confidence semantics.

- [ ] **Step 2: Run documentation and artifact tests to prove current 7.1.0 text fails**

```bash
node --test tests/hooks-docs.test.mjs tests/hooks-artifacts.test.mjs
```

Expected before edits: failures on the new 7.1.1 and provenance assertions.

- [ ] **Step 3: Update Graphify guidance**

Document these exact rules in `references/graphify.md`:

- `_origin` distinguishes AST from semantic extraction.
- `confidence` describes resolution strength.
- AST-origin `INFERRED` relationships are valid offline structural leads.
- Semantic or missing provenance is invalid for Agent-Team's offline readiness evidence.
- A fresh worktree graph must not adopt a graph that may contain a prior semantic layer.

- [ ] **Step 4: Apply the 7.1.1 version and release metadata**

Set `SKILL.md` and `hooks/manifest.json` to `7.1.1`. Add a dated changelog entry. Update README current-version and artifact examples, the site version badge, and artifact tests. Preserve the named 7.1.0 field guide, artwork, and guide tests as historical content.

After Project Kickoff Task 4 confirms the live release, change Agent-Team's optional Project Kickoff installation link from its stale historical tag to `v0.4.1`; do not present Project Kickoff as mandatory.

- [ ] **Step 5: Run documentation, artifact, and package checks**

```bash
node --test tests/hooks-docs.test.mjs tests/hooks-artifacts.test.mjs
node hooks/agent-team-cli.mjs check-package
python3 /home/server/.codex/skills/.system/skill-creator/scripts/quick_validate.py .
git diff --check
```

Expected: all commands exit zero and package version surfaces agree on 7.1.1.

- [ ] **Step 6: Commit documentation and release metadata**

```bash
git add references/graphify.md SKILL.md hooks/manifest.json CHANGELOG.md README.md index.html tests/hooks-artifacts.test.mjs tests/hooks-docs.test.mjs
git commit -m "docs: prepare Agent-Team 7.1.1"
```

---

### Task 3: Agent-Team deadline-sensitive suite stabilization

**Files:**
- Test: `tests/hooks-install-health.test.mjs:729`
- Test: `tests/hooks-playwright-companion.test.mjs:201`
- Modify only after diagnosis: the narrow implementation or test file proven responsible for a reproducible race.

**Interfaces:**
- Consumes: exact Agent-Team candidate revision after Tasks 1 and 2.
- Produces: a full suite that passes for a causal reason, or evidence that the candidate is blocked without weakening deadlines.

- [ ] **Step 1: Reproduce each prior failure in isolation**

```bash
node --test --test-name-pattern='a subsequent CLI invocation recovers ownership after abrupt process termination' tests/hooks-install-health.test.mjs
node --test --test-name-pattern='shared browser deadline kills the fake child and never commits a late receipt' tests/hooks-playwright-companion.test.mjs
```

Expected: record the exact outcome of each; a pass is not yet proof that the concurrent-suite race is absent.

- [ ] **Step 2: Run the complete concurrent suite once on the exact candidate**

```bash
node --test tests/hooks-*.test.mjs
```

Expected: zero failures. If either named failure appears, stop release work and capture its original output, process state, and timing.

- [ ] **Step 3: Diagnose a reproduced race and hold this release plan**

For a reproduced failure, use the systematic-debugging procedure: establish one concrete reproduction, inspect the responsible timeout/process ownership path, and form competing hypotheses when needed. Record the responsible implementation and test paths, the failing command, and the causal evidence. Hold Tasks 5 and 6 and write a separate narrow stabilization plan from that evidence. Do not increase a timeout or add a retry merely to unblock this release.

- [ ] **Step 4: Close the diagnostic gate only with evidence**

If the complete suite passes without reproducing either issue, create no stabilization commit and retain the exact successful log as release evidence. If a separate stabilization plan becomes necessary, resume this plan only after that repair receives independent review and both named isolated tests plus the complete suite pass on the new exact base revision.

---

### Task 4: Verify and publish Project Kickoff 0.4.1

**Files:**
- Modify: `/home/server/dev/skills/project-kickoff/tests/test_agent_team_handoff.py`
- Verify source: `/home/server/dev/skills/project-kickoff`
- Create ignored artifact: `dist/project-kickoff-0.4.1.zip`
- Create ignored checksum: `dist/v0.4.1/SHA256SUMS`

**Interfaces:**
- Consumes: Project Kickoff commit `7a522aab466709571b6ee084cb1a86afcdf87f3a` and the final compatible Agent-Team 7.1.1 source.
- Produces: `origin/main`, annotated tag `v0.4.1`, GitHub release `Project Kickoff v0.4.1`, verified ZIP and checksum assets, and verified Pages version.

- [ ] **Step 1: Update only the real cross-version integration assertion**

Rename `test_real_agent_team_710_adopts_the_handoff` to `test_real_agent_team_711_adopts_the_710_handoff`. Require the supplied real Agent-Team root to report 7.1.1, while continuing to assert that the Project Kickoff template and checker use `agentTeam.testedVersion: "7.1.0"`. Do not change the checker, handoff template, or compatibility documentation: Agent-Team 7.1.1 is a compatible patch, and Project Kickoff removes its producer-only compatibility metadata before initialization.

Run the focused test against the final Agent-Team integration worktree:

```bash
PYTHONDONTWRITEBYTECODE=1 AGENT_TEAM_ROOT=/home/server/dev/skills/agent-team/.worktrees/integrate-graphify-provenance-7.1.1 python3 -m unittest tests.test_agent_team_handoff -v
```

Expected before the test-only adjustment: one failure because the test requires the Agent-Team package version to equal 7.1.0. Expected afterward: every focused test passes and the emitted handoff compatibility baseline remains 7.1.0.

Commit only the test adjustment:

```bash
git add tests/test_agent_team_handoff.py
git commit -m "test: qualify Agent-Team 7.1.1 compatibility"
```

- [ ] **Step 2: Verify the exact Project Kickoff candidate**

```bash
cd /home/server/dev/skills/project-kickoff
git merge-base --is-ancestor 7a522aab466709571b6ee084cb1a86afcdf87f3a HEAD
test "$(git status --porcelain)" = ""
git diff --check
python3 /home/server/.codex/skills/.system/skill-creator/scripts/quick_validate.py .
PYTHONDONTWRITEBYTECODE=1 AGENT_TEAM_ROOT=/home/server/dev/skills/agent-team python3 -m unittest discover -s tests -v
node scripts/check-guide.mjs project-kickoff-guide-v0.3.1.html
```

Expected: the candidate descends from the reviewed 0.4.1 source, the tree is clean, 54 tests pass, Skill validation passes, and guide checks report 6/6.

- [ ] **Step 3: Build the exact 34-file archive**

Use the 34-path README allowlist and exact revision:

```bash
release_version=0.4.1
release_revision="$(git rev-parse HEAD)"
mkdir -p "dist/v$release_version"
git archive --format=zip --prefix=project-kickoff/ --output="dist/project-kickoff-$release_version.zip" "$release_revision" $kickoff_required_files
(cd dist && sha256sum "project-kickoff-$release_version.zip" > "v$release_version/SHA256SUMS" && sha256sum -c "v$release_version/SHA256SUMS")
```

Define `kickoff_required_files` from the first README installation allowlist and assert the second list is byte-identical before invoking `git archive`.

- [ ] **Step 4: Verify archive contents and bytes**

Create a disposable extraction directory. Assert exactly 34 regular files, no symlinks, every path equals the allowlist, and every extracted byte equals `git show "$release_revision:$path"`. Run Skill Creator validation on the extracted package and `unzip -t` on the ZIP. Remove only the disposable extraction directory.

- [ ] **Step 5: Recheck remote identity, then push main and tag**

```bash
git fetch origin
test "$(git rev-parse origin/main)" = "a199929f7803a8595b4de840a55fb35e8720caf3"
test "$(git rev-parse HEAD)" = "$release_revision"
git push origin main
test "$(git ls-remote origin refs/heads/main | awk '{print $1}')" = "$release_revision"
git tag -a "v$release_version" -m "Project Kickoff $release_version" "$release_revision"
git push origin "v$release_version"
```

If remote main changed, stop publication, incorporate it, and repeat affected verification. Never replace the expected hash without reviewing the new commits.

- [ ] **Step 6: Create and verify the GitHub release**

```bash
gh release create v0.4.1 dist/project-kickoff-0.4.1.zip dist/v0.4.1/SHA256SUMS --repo thebpandey/project-kickoff --verify-tag --title "Project Kickoff v0.4.1" --generate-notes
```

Confirm the release is published, targets `v0.4.1`, and contains exactly the ZIP and checksum assets. Download both assets into a new disposable directory, run `sha256sum -c SHA256SUMS`, compare the ZIP digest to the local artifact, and verify the live Pages version badge reports 0.4.1.

---

### Task 5: Integrate and verify Agent-Team 7.1.1

**Files:**
- Integrate Task 1 and Task 2 branches into one Agent-Team integration worktree.
- Create ignored release artifacts through `hooks/agent-team-cli.mjs build-artifacts`.

**Interfaces:**
- Consumes: independently reviewed Task 1 and Task 2 commits and verified Project Kickoff 0.4.1 release.
- Produces: one exact Agent-Team 7.1.1 release candidate with complete local evidence.

- [ ] **Step 1: Merge reviewed branches serially in the integration worktree**

Merge the core branch first, then the documentation/version branch. Resolve only genuine integration conflicts; any behavioral change returns to its owning developer and requires renewed review.

- [ ] **Step 2: Run targeted and complete gates on the combined revision**

```bash
node --test tests/hooks-dependencies.test.mjs
AGENT_TEAM_REAL_DEPS=1 node --test --test-name-pattern='real isolated Graphify package builds and traverses an offline code graph' tests/hooks-dependencies-real.test.mjs
node --test tests/hooks-docs.test.mjs tests/hooks-artifacts.test.mjs
node --test tests/hooks-*.test.mjs
node hooks/agent-team-cli.mjs check-package
python3 /home/server/.codex/skills/.system/skill-creator/scripts/quick_validate.py .
git diff --check
```

Expected: every required command exits zero. Any deadline-sensitive failure returns to Task 3 and blocks release.

- [ ] **Step 3: Build and validate Agent-Team release artifacts**

```bash
release_revision="$(git rev-parse HEAD)"
artifact_dir="$(mktemp -d /tmp/agent-team-7.1.1-artifacts.XXXXXX)"
node hooks/agent-team-cli.mjs build-artifacts --revision "$release_revision" --output "$artifact_dir"
node hooks/agent-team-cli.mjs check-artifacts --revision "$release_revision" --archive "$artifact_dir/agent-team-7.1.1.zip"
sha256sum "$artifact_dir/agent-team-7.1.1.zip"
```

Retain the artifact directory and digest as release evidence until the remote release is verified.

- [ ] **Step 4: Obtain independent combined-revision acceptance**

The delegated verifier reviews the exact integration revision for requirements and code quality, confirms Project Kickoff remains declarative-only, confirms the README link resolves to the published 0.4.1 tag, and reruns the affected gates. Findings return to the owning writer; only an accepted revision can advance.

---

### Task 6: Publish and verify Agent-Team 7.1.1

**Files:**
- Publish verified Agent-Team source and tag; no additional source edits during this task.

**Interfaces:**
- Consumes: accepted Agent-Team 7.1.1 integration revision and validated artifact.
- Produces: `origin/main`, annotated tag `v7.1.1`, successful tag workflow, published GitHub release and verified assets.

- [ ] **Step 1: Recheck main, remote base, and release identity**

```bash
git fetch origin
test "$(git status --porcelain)" = ""
test "$(git rev-parse --abbrev-ref HEAD)" = main
test "$(git rev-parse origin/main)" = "be79af6487059779af41f60c05b68b7ed10171ea"
test "$(git rev-parse HEAD)" = "$release_revision"
```

The expected remote hash is valid only while no one else has advanced `origin/main`. A change requires incorporation and affected re-verification.

- [ ] **Step 2: Push the verified main revision**

```bash
git push origin main
test "$(git ls-remote origin refs/heads/main | awk '{print $1}')" = "$release_revision"
```

- [ ] **Step 3: Tag once and let the existing workflow publish**

```bash
git tag -a v7.1.1 -m "Agent-Team 7.1.1" "$release_revision"
git push origin v7.1.1
```

Do not manually create a duplicate GitHub release. Record the tag-triggered workflow identity and wait for its terminal result.

- [ ] **Step 4: Verify GitHub checks, release, and downloaded assets**

Require the exact-revision Check package workflow and the tag release workflow to succeed. Confirm the published release targets `v7.1.1`, is not a draft or prerelease, and contains the expected archive and checksum assets. Download them into a new disposable directory, validate the checksum, run `check-artifacts` against the downloaded archive, and verify README and Pages current-version surfaces.

- [ ] **Step 5: Install and requalify the released skills**

Atomically update the user-scoped Agent-Team installation from the verified 7.1.1 release while preserving the prior 7.1.0 installation as a timestamped backup. Confirm the existing Project Kickoff installation matches released 0.4.1. Run an authorized Agent-Team dependency preparation with a fresh worker in a fresh worktree and require:

- Graphify functional evidence accepts the known AST-origin `INFERRED` callback edge.
- Semantic or missing provenance fixtures remain rejected.
- Graphify fresh-worker discovery passes honestly.
- No host Graphify installer, hook, semantic backend, or MCP registration runs.

- [ ] **Step 6: Record release evidence and clean task-owned resources**

Update each canonical tracker with exact source revisions, tags, workflow IDs, release URLs, asset digests, live verification, and installation/requalification evidence. Remove only stopped, clean, integrated task-owned worktrees and temporary extraction directories after preserving evidence; retain unrelated existing worktrees and user files.
