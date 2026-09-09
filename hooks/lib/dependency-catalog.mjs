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
    install: npm("@ast-grep/cli", "0.45.3", "sg"),
    functionalCheck: "positive-negative-structural-pattern",
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
    functionalCheck: "complete-selective-skills",
  },
  {
    id: "ponytail", name: "Ponytail", disposition: "default", version: "356918eba965ee1eac64bd3a7f0dd02108350de5", prerequisites: [],
    install: github("DietrichGebert/ponytail", "356918eba965ee1eac64bd3a7f0dd02108350de5", [
      "skills/ponytail", "skills/ponytail-review", "skills/ponytail-help",
    ]),
    optionalPaths: ["skills/ponytail-audit", "skills/ponytail-debt"],
    functionalCheck: "complete-selective-skills",
  },
  {
    id: "impeccable", name: "Impeccable", disposition: "default", version: "4.1.0", prerequisites: [],
    install: npm("impeccable", "4.1.0", "impeccable"),
    profile: { hooks: false, imagegen: false, interview: false, batchDetection: true },
    functionalCheck: "detector-exit-contract",
  },
  {
    id: "react-best-practices", name: "React Best Practices", disposition: "default", version: "063bee94c3f4df8453406c830b0a7df0f2860278", prerequisites: [],
    install: github("vercel-labs/agent-skills", "063bee94c3f4df8453406c830b0a7df0f2860278", ["skills/react-best-practices"]),
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
    id: "beads-viewer", name: "beads_viewer", disposition: "optional", version: null, prerequisites: ["beads"],
    install: { kind: "permission-gated", source: "https://github.com/Dicklesworthstone/beads_viewer" },
    boundary: "License applicability and permission must be resolved before preparation or distribution.",
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
