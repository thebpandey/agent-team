const github = (repository, revision, paths = []) => ({
  kind: "git-skill",
  repository: `https://github.com/${repository}.git`,
  source: `https://github.com/${repository}`,
  revision,
  paths,
});

const npm = (packageName, version, command) => ({
  kind: "npm",
  package: packageName,
  version,
  command,
  source: `https://www.npmjs.com/package/${packageName.replace("/", "%2f")}`,
});

const skillCompatibility = (selectedPaths, blobs) => ({
  kind: "required-files",
  entrypoint: "SKILL.md",
  allowUnrelatedRegularFiles: true,
  selectedPaths: selectedPaths.map((selectedPath) => ({ selectedPath, requiredFiles: [{
    path: "SKILL.md", digest: { algorithm: "git-blob-sha1", value: blobs[selectedPath] },
  }] })),
});

export const DEPENDENCY_CATALOG = Object.freeze([
  {
    id: "uv", name: "uv", disposition: "prerequisite", version: "0.12.10", prerequisites: [],
    install: { kind: "github-release", repository: "astral-sh/uv", tag: "0.12.10", source: "https://github.com/astral-sh/uv/releases/tag/0.12.10", checksums: "sha256.sum" },
    functionalCheck: "command",
  },
  {
    id: "serena", name: "Serena", disposition: "mandatory", version: "1.7.0", prerequisites: ["uv"],
    install: { kind: "uv-tool", package: "serena-agent", version: "1.7.0", python: "3.13", source: "https://pypi.org/project/serena-agent/1.7.0/" },
    profile: { backend: "language-server", editing: false, memory: false, activeProjectIsolation: true },
    boundary: "Host MCP registration, trust, reload, and fresh-worker discovery remain separate approval and verification steps.",
    functionalCheck: "symbol-operation",
  },
  {
    id: "playwright-cli", name: "Microsoft Playwright CLI", disposition: "mandatory", version: "0.1.19", prerequisites: [],
    install: npm("@playwright/cli", "0.1.19", "playwright-cli"),
    preparation: ["inspect-help", "install-skills", "install-browser"],
    boundary: "Browser download, host skill reload, and fresh-worker discovery must succeed before browser capability is ready.",
    functionalCheck: "browser-interaction",
  },
  {
    id: "ast-grep", name: "ast-grep CLI", disposition: "default", version: "0.45.3", prerequisites: [],
    install: npm("@ast-grep/cli", "0.45.3", "ast-grep"),
    functionalCheck: "positive-negative-structural-pattern",
  },
  {
    id: "graphify", name: "Graphify", disposition: "default", version: "0.9.57", prerequisites: ["uv"],
    install: { kind: "uv-tool", package: "graphifyy", version: "0.9.57", python: "3.12", source: "https://pypi.org/project/graphifyy/0.9.57/" },
    profile: { codeOnly: true, viz: false, llmBackend: false, hostInstall: false, hooks: false, mcp: "optional-stdio" },
    boundary: "Build graphs per worktree with extract --code-only --no-viz; graphify install/hook/claude/codex installers, semantic extraction and MCP registration remain separate explicit decisions.",
    functionalCheck: "code-graph-traversal",
  },
  {
    id: "lean-ctx", name: "LeanCTX", disposition: "default", version: "3.10.1", prerequisites: [],
    install: {
      kind: "github-release",
      source: "https://github.com/yvgude/lean-ctx/releases/download/v3.10.1",
      checksums: "https://github.com/yvgude/lean-ctx/releases/download/v3.10.1/SHA256SUMS",
      skill: {
        source: "https://raw.githubusercontent.com/yvgude/lean-ctx/4a76710a6c792229f170a66fdda1f4a0a64f47ee/rust/src/templates/SKILL.md",
        gitBlob: "258398981da1eb779677dc999bd11ac6206cf606",
      },
    },
    profile: { wrap: false, proxy: false, knowledge: false, coordination: false, modelSteering: false, exactRecovery: true },
    compatibility: skillCompatibility(["skills/lean-ctx"], { "skills/lean-ctx": "258398981da1eb779677dc999bd11ac6206cf606" }),
    functionalCheck: "narrow-read-recovery",
  },
  {
    id: "superpowers", name: "Superpowers", disposition: "default", version: "b36e0829c6d0140e93cfef2ca599b1b07d4a7797", prerequisites: [],
    install: github("obra/superpowers", "b36e0829c6d0140e93cfef2ca599b1b07d4a7797", [
      "skills/using-superpowers", "skills/test-driven-development", "skills/systematic-debugging",
      "skills/requesting-code-review", "skills/receiving-code-review", "skills/verification-before-completion",
      "skills/brainstorming", "skills/writing-plans", "skills/subagent-driven-development",
      "skills/executing-plans", "skills/dispatching-parallel-agents", "skills/using-git-worktrees",
      "skills/finishing-a-development-branch", "skills/writing-skills",
    ]),
    compatibility: skillCompatibility([
      "skills/using-superpowers", "skills/test-driven-development", "skills/systematic-debugging",
      "skills/requesting-code-review", "skills/receiving-code-review", "skills/verification-before-completion",
      "skills/brainstorming", "skills/writing-plans", "skills/subagent-driven-development", "skills/executing-plans",
      "skills/dispatching-parallel-agents", "skills/using-git-worktrees", "skills/finishing-a-development-branch", "skills/writing-skills",
    ], {
      "skills/using-superpowers": "7ab2eb678f649c8befb94512359ee0b6e9a9a9a4",
      "skills/test-driven-development": "4320d8879a639d0536c2239bb80c9d7257ad8947",
      "skills/systematic-debugging": "095d194ac041502905f15b01d22d294fb94db8b2",
      "skills/requesting-code-review": "fa4f2f9965591d6d8a7ef4ad47e6ba4d5ac344d0",
      "skills/receiving-code-review": "950da7b74bf6dbed6b8726d12ddadd65a9f5fda7",
      "skills/verification-before-completion": "7d45333cc4a49c57a80df6c1fe2fa777a207afbc",
      "skills/brainstorming": "b56a3b5ed6ea0d6216501e0e401ecb00a1b4675f",
      "skills/writing-plans": "f74605bfa9af7a3fb7e4ad7f17750a86a9b0d728",
      "skills/subagent-driven-development": "aac35b91c07af15eca580a205eb0c894b86e87c8",
      "skills/executing-plans": "b51d97d2cc4c2ecc5eabecf27e499aa8c933efac",
      "skills/dispatching-parallel-agents": "3fa091b348bbe0a18eaa0e17e9092c97f7005cbe",
      "skills/using-git-worktrees": "1381dacb3516b94382dd6ec736ede21ee8ede2b3",
      "skills/finishing-a-development-branch": "fa8aecaf8813f1b19d19996f7957d1b0121f2d9c",
      "skills/writing-skills": "f33f39f523c8a3ae2ec0eaf365a524571df8ff3d",
    }),
    functionalCheck: "complete-selective-skills",
  },
  {
    id: "ponytail", name: "Ponytail", disposition: "default", version: "356918eba965ee1eac64bd3a7f0dd02108350de5", prerequisites: [],
    install: github("DietrichGebert/ponytail", "356918eba965ee1eac64bd3a7f0dd02108350de5", [
      "skills/ponytail", "skills/ponytail-review", "skills/ponytail-help",
    ]),
    compatibility: skillCompatibility(["skills/ponytail", "skills/ponytail-review", "skills/ponytail-help"], {
      "skills/ponytail": "02c0712c86277d49d18a77da3a2b825657bf02d1",
      "skills/ponytail-review": "e137a855bd87119a4517895a1000a59b0999e1b8",
      "skills/ponytail-help": "ba145c0ebb7c7e5682bb2f36047af3bf2030d470",
    }),
    optionalPaths: ["skills/ponytail-audit", "skills/ponytail-debt"],
    functionalCheck: "complete-selective-skills",
  },
  {
    id: "impeccable", name: "Impeccable", disposition: "default", version: "4.1.0", prerequisites: [],
    install: npm("impeccable", "4.1.0", "impeccable"),
    guidance: {
      repository: "https://github.com/pbakaus/impeccable.git",
      source: "https://github.com/pbakaus/impeccable/tree/2c33196c51ac52e47691384e61d89f1218d8d21d",
      revision: "2c33196c51ac52e47691384e61d89f1218d8d21d",
      selectedPaths: { codex: ".agents/skills/impeccable", "claude-code": ".agent/skills/impeccable" },
      gitBlobs: {
        codex: "ac845e87f80eb9cba9389b1c0a907c54d1c4d3dd",
        "claude-code": "e2d1ee0347f92fda0f36533cb251773325f3413a",
      },
    },
    profile: { hooks: false, imagegen: false, interview: false, batchDetection: true },
    functionalCheck: "detector-exit-contract",
  },
  {
    id: "react-best-practices", name: "React Best Practices", disposition: "default", version: "063bee94c3f4df8453406c830b0a7df0f2860278", prerequisites: [],
    install: github("vercel-labs/agent-skills", "063bee94c3f4df8453406c830b0a7df0f2860278", ["skills/react-best-practices"]),
    compatibility: skillCompatibility(["skills/react-best-practices"], {
      "skills/react-best-practices": "237988de4a66dd8a71d30a2c24ebe1a86b58d04e",
    }),
    functionalCheck: "complete-selective-skills",
  },
  {
    id: "beads", name: "Beads", disposition: "tracker", version: "1.2.2", prerequisites: [],
    install: npm("@beads/bd", "1.2.2", "bd"),
    revision: "6c124203e771433a3550c348771a5b5e27fd3c21",
    functionalCheck: "atomic-tracker-write",
  },
  {
    id: "context7", name: "Context7", disposition: "optional", version: "4.0.6", prerequisites: [],
    install: npm("@upstash/context7-mcp", "4.0.6", "context7-mcp"),
    boundary: "Online service; authentication, rate limits, and query privacy remain external boundaries.",
    functionalCheck: "focused-documentation-query",
  },
  {
    id: "project-kickoff", name: "Project Kickoff", disposition: "optional", version: "89e6228611b1dd726c6aa0363e7206d0c72fa16d", prerequisites: [],
    install: {
      ...github("thebpandey/project-kickoff", "89e6228611b1dd726c6aa0363e7206d0c72fa16d", ["."]),
      includePaths: ["SKILL.md", "README.md", "CHANGELOG.md", "LICENSE", "agents", "assets", "references", "scripts"],
    },
    boundary: "Independent optional planning workflow; never required by Agent-Team.",
    functionalCheck: "skill-discovery",
  },
  {
    id: "beads-viewer", name: "beads_viewer", disposition: "optional", version: "0.24.1", prerequisites: ["beads"],
    install: { kind: "github-release", command: "bv", source: "https://github.com/Dicklesworthstone/beads_viewer/releases/download/v0.24.1", checksums: "per-asset .sha256" },
    boundary: "Optional Beads graph provider; preserve the complete upstream license and operator terms acknowledgement.",
    functionalCheck: "fresh-export-graph",
  },
  ...[
    ["vercel-agent-browser", "Vercel Agent Browser"], ["rtk", "RTK"], ["beads-rust", "beads_rust"],
    ["backlog-md", "Backlog.md"], ["gsd", "GSD Core"], ["ralph", "Ralph"], ["caveman-runtime", "Caveman runtime"],
    ["matt-tdd", "Matt Pocock TDD"], ["matt-diagnosing-bugs", "Matt diagnosing-bugs"],
    ["matt-code-review", "Matt code-review"], ["ast-grep-companion", "ast-grep companion skill"],
    ["ponytail-gain", "Ponytail gain"],
  ].map(([id, name]) => ({ id, name, disposition: "excluded", prerequisites: [] })),
]);

export const CATALOG_BY_ID = new Map(DEPENDENCY_CATALOG.map((entry) => [entry.id, entry]));
