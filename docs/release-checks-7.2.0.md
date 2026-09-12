# Agent-Team 7.2.0 qualification method

This document defines the source-candidate qualification method. It intentionally records no candidate commit, archive digest, publication state, workflow run, asset result, evidence path, or machine-specific temporary directory. Final mutable facts belong only in the ignored, closed qualification receipt.

## Preconditions

- Start from the clean, serially integrated, independently reviewed REL-001 through REL-009 lineage.
- Verify every accepted task head and repair is an ancestor, its review is bound to that exact head, and its reviewed path set matches Git.
- Keep the qualification worktree clean. Route any product finding to the task that owns the affected files; do not repair product code in this qualification task.
- Treat portable skips separately from real-dependency qualification. A skipped, unavailable, cancelled, or timed-out check is never a pass.

## Candidate checks

Run the portable suite serially and retain pass, fail, cancelled, skipped, and todo counts separately:

```sh
node --test --test-concurrency=1 tests/hooks-*.test.mjs
node hooks/agent-team-cli.mjs check-package
git diff --check
```

Run the real dependency cases with real execution enabled and explicit absolute executable paths where required:

```sh
AGENT_TEAM_REAL_DEPS=1 \
AGENT_TEAM_REAL_LEAN_CTX=/absolute/path/to/lean-ctx \
AGENT_TEAM_REAL_BD=/absolute/path/to/bd \
AGENT_TEAM_REAL_SERENA=1 \
node --test --test-concurrency=1 tests/hooks-dependencies-real.test.mjs
```

Every required real case must pass without a skip. An environment-inapplicable optional case must be named and justified; it cannot be folded into the pass count.

## Exact-revision deterministic artifacts

Capture the clean committed candidate revision, create two fresh build directories, build independently, verify each archive independently, and make byte equality a hard gate:

```sh
revision="$(git rev-parse HEAD)"
build_one="$(mktemp -d)"
build_two="$(mktemp -d)"
node hooks/agent-team-cli.mjs build-artifacts --revision "$revision" --output "$build_one"
node hooks/agent-team-cli.mjs build-artifacts --revision "$revision" --output "$build_two"
node hooks/agent-team-cli.mjs check-artifacts --revision "$revision" --archive "$build_one/agent-team-7.2.0.zip"
node hooks/agent-team-cli.mjs check-artifacts --revision "$revision" --archive "$build_two/agent-team-7.2.0.zip"
cmp -s "$build_one/agent-team-7.2.0.zip" "$build_two/agent-team-7.2.0.zip"
sha256sum "$build_one/agent-team-7.2.0.zip" "$build_two/agent-team-7.2.0.zip"
```

Both builds must report `built`; both checks must report `passed`; `cmp -s` must exit zero; and both independently observed SHA-256 values must match. The archive must contain exactly 97 entries: the 96 manifest-listed files plus generated `.agent-team-source.json`.

The embedded metadata remains the closed ten-key, non-self-referential package-source object. Artifact verification separately derives the complete archive map from sealed bytes and validates normalized paths, types, modes, sizes, hashes, package/archive digests, repository, release identity, and exact source revision. Embedded package metadata is not publication or installation-receipt authority.

## Interpretation and handoff

- Package success requires structured `status: "passed"`; process exit alone is insufficient.
- Portable qualification requires zero failures and cancellations while retaining the actual skip count.
- Real dependency qualification requires every required named case to be freshly passed or supported by approved unchanged-path ancestry evidence.
- Final evidence uses the closed ignored `final-artifacts.json` schema, recomputes the archive-map digest, binds the exact clean HEAD, and validates the two equal build hashes and independent artifact checks.
- A review may bind the immediate pre-document ancestor only when this file is the sole final tracked change and contains no mutable final evidence.
- Publication, tags, releases, Pages checks, official downloads, and live host installation are separate authorized tasks.
