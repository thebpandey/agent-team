# Graphify integration

[Graphify](https://github.com/Graphify-Labs/graphify) is a deterministic whole-repository structural knowledge graph. Tree-sitter AST extraction produces nodes for files, classes, functions and methods, and typed edges (`calls`, `imports`, `method`) between them. It answers structure questions: which modules cluster together (communities), which symbols are hotspots (god-nodes), what a change touches (`affected`), how two symbols connect across modules (`path`), and what surrounds a symbol (`explain`, `query`).

It is prepared by default alongside [Serena](dependencies.md), not instead of it.

## Complementary split with Serena

| Question | Tool |
| --- | --- |
| Exact symbol body, definition, references, owned edit | Serena (language server, precise, current file state) |
| Repository shape, module clusters, cross-module route, blast radius, hotspots | Graphify (whole-graph, approximate, snapshot) |

Serena answers "what is this symbol and who references it" with language-server precision. Graphify answers "what does this repository look like and what does touching this file reach" over the whole tree at once. Do not use Graphify to read a symbol body, and do not walk a repository file by file with Serena to answer a structure question.

## Code-only offline profile

Agent-Team uses `graphify extract . --code-only --no-viz` only. That path is deterministic and offline: no model call, no network, no API key, no visualization output. Every edge it writes is tagged `confidence: "EXTRACTED"`, meaning it came from the AST. Semantic extraction (`extract` without `--code-only`) adds `INFERRED` edges from a model backend; in code-only mode no `INFERRED` edge exists, so an `INFERRED` edge in a graph means a backend ran and the graph is no longer offline evidence.

## Where graphs live

`extract` writes only `./graphify-out/` in the directory it runs in: `graph.json`, `manifest.json`, the analysis record and an AST cache. It creates nothing in the user's home directory. Build the graph inside the worktree that owns the code, never in a shared location. `graphify-out/` is a build artifact and is never committed; adding it to `.gitignore` is a developer task performed only with project authority.

## Lifecycle

1. Build once per worktree at run start: `graphify extract . --code-only --no-viz`.
2. Refresh with `graphify update .` after each integrated revision and before review. It re-extracts only changed code files, needs no key or model, and rewrites `graph.json`. It also regenerates the local `graph.html` and `GRAPH_REPORT.md` inside `graphify-out/`; without a backend the community names stay `Community N` placeholders, which is the intended offline result.
3. Developer worktrees keep their own graph on their own branch. A stale graph is a stale map; refresh before drawing a conclusion from it.

## Who uses what

- **Project orchestrator**, through the delegated verifier: `affected` on the candidate task files and `god-nodes` to find shared entrypoints before dispatch, `path` to check whether two planned tasks meet. This derives disjoint ownership and the parallel task set. The orchestrator does not run these itself.
- **Developers**: `explain "<symbol>"` before editing an unfamiliar symbol, `path "<A>" "<B>"` before changing something that crosses modules. Then read the actual code with Serena.
- **Reviewers and the verifier**: `affected` on the changed files versus the claimed ownership and the stated scope. Reach outside the claim is a finding.

Traversal commands run from the directory holding `graphify-out/`, or with an explicit `--graph <path-to-graph.json>`. `query "<text>"` returns a bounded BFS neighbourhood.

## Optional MCP

`graphify-mcp --graph <path-to-graph.json>` serves a prebuilt graph over stdio (`query_graph`, `get_node`, `get_neighbors`, `shortest_path`). It runs no model. It is not registered automatically; register it only on explicit selection, through the same owned host-registration path as any other MCP server.

## Prohibitions

- Never run `graphify install`, `graphify hook install`, `graphify claude install` or `graphify codex install`. They write host instruction files and PreToolUse hooks outside Agent-Team's ownership.
- No semantic extraction, `cluster-only`, `label`, `--backend` or generated report without an approved key and an explicit decision.
- No `add <url>` and no `--postgres`. Both leave the offline local profile.
- Respect worktree ownership: build and read the graph for the worktree you own.
- A graph is a map, not evidence. Confirm what it suggests with Serena, the source and tests before claiming it.
