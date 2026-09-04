# Agent-Team

A development skill for **Codex and Claude Code** that chooses a capable model for each role and delivers tracked, verified software changes. Give it a task; it chooses the smallest useful team, preserves task state, and reconciles the result against every requirement.

## Install

Install this repository as the `agent-team` skill using your host's supported skill installer. The repository root contains `SKILL.md` and its supporting references.

For Codex CLI on macOS/Linux, a manual user-wide installation is:

```bash
mkdir -p ~/.agents/skills
git clone https://github.com/thebpandey/agent-team.git ~/.agents/skills/agent-team
```

If that destination already exists, inspect and update your existing installation instead of cloning over it. Use the current host documentation for other installation locations or operating systems. ChatGPT-managed skill installation is separate from installing into your own laptop/server.

Select GPT-6 Astra with high reasoning in Codex, then invoke:

```text
$agent-team setup
$agent-team Add the requested feature, verify it, and reconcile every requirement.
```

For **Claude Code** on macOS/Linux, install the same repository in Claude's user skill directory:

```bash
mkdir -p ~/.claude/skills
git clone https://github.com/thebpandey/agent-team.git ~/.claude/skills/agent-team
claude --model claude-fable-5-1 --effort high
```

Then invoke:

```text
/agent-team setup
/agent-team Add the requested feature, verify it, and reconcile every requirement.
```

For project scope, use `.claude/skills/agent-team` instead. Setup also provisions the bundled native agent definitions in `.claude/agents/`, or the selected user scope, without overwriting existing definitions. Verify that Claude discovers them; a refresh or new session may be needed. Installation paths and invocation follow [Claude's skill documentation](https://code.claude.com/docs/en/skills).

Fable 5.1 requires Claude Code v2.1.255+ and account/provider access. Opus 5 with high effort is the disclosed orchestration fallback if Fable is unavailable or its usage-credit choice is declined. A skill cannot switch its parent model, grant model access, or accept purchases. See [Claude model configuration](https://code.claude.com/docs/en/model-config).

In ChatGPT, select Agent-Team from available skills or use its supported mention. ChatGPT installation does not install it on a laptop/server. Keep only one active installation per host/scope; inspect an existing destination before updating it.

Both platforms use the same root skill, task state, and dependency catalog. Only the matching platform adapter is loaded. Native subagent definitions are included for Claude; Codex uses its exposed agent controls. No experimental Claude agent-team feature is required.

## First-run setup

Agent-Team checks installed skills, project packages, runtimes, and Beads before implementation. It shows missing items and a concrete installation plan, then offers one choice:

| Profile | What it installs |
| --- | --- |
| Recommended dependencies + project needs | Offer all four recommended dependencies and install accepted items plus relevant optional tools |
| All compatible free dependencies | Eligible referenced skills/reference packs and compatible application packages, with exclusions and overlaps shown first |
| Choose individually or defer | Your selected items, or no installation |

It uses your chosen scope, skips existing usable installations, verifies results, remembers the choice locally, and resumes the original task. It does not reinstall or upgrade everything each time. If any of Beads, Ponytail, Using-Superpowers, or Impeccable is declined, unavailable, or fails installation, work continues with a local `.agent-team/TASKS.md` tracker and the remaining available skills. Declines are remembered without repeated prompts.

The full profile does not mean installing every component in every registry. Registries are catalogs, some packages require a compatible app stack, and paid products need existing entitled access. Application packages are deferred when no compatible project exists. Installation cannot bypass OS permissions, host restrictions, hook trust prompts, or missing credentials.

Setup details and official sources: [dependency setup](references/setup.md).

## Recommended dependencies, with a built-in fallback

- [Ponytail](https://github.com/DietrichGebert/ponytail): simple, complete implementation.
- [Using-Superpowers](https://github.com/obra/superpowers): relevant development procedures.
- [Beads](https://github.com/gastownhall/beads): preferred task and failure tracking when enabled.
- [Impeccable](https://github.com/pbakaus/impeccable): preferred for UI/UX; built-in design guidance is available without it.

Every teammate receives the selected tracker mode and available skills. Dependencies are installed through their supported distributions, not copied into this repository. Choosing not to install any or all of these four does not block development.

## Optional design resources

| Resource | Use when |
| --- | --- |
| UI UX Pro Max | Broader design-system exploration needs searchable references |
| UI Skills | A targeted design-engineering procedure is useful |
| shadcn/ui | A compatible app needs reusable interface components |
| Magic UI / React Bits | Selected expressive components serve the brief |
| Motion | Purposeful animations need more than CSS |
| Taste Skill | Landing pages, portfolios, or substantial redesigns need more art direction |
| Awesome DESIGN.md | A relevant design-system example helps establish direction |
| Bklit UI | A compatible dashboard needs charts and data visualization |
| img2threejs | A real 3D task needs procedural reconstruction from imagery |

See [UI workflow](references/ui.md) and [optional routing](references/ui-optional.md) for sources and limitations. The GPT-specific Taste variant is excluded from automatic selection. Motion+ and Bklit Studio are not covered by their free core/component licenses. React Bits includes additional Commons Clause restrictions.

## Team and workflow

| Role | Codex | Claude Code |
| --- | --- | --- |
| Orchestrator; trivial direct work | GPT-6 Astra, high | Fable 5.1, high |
| Standard developer | GPT-5.6 Terra, medium/high | Sonnet 5, medium/high |
| Independent reviewer | GPT-5.6 Terra, medium/high | Sonnet 5, high |
| Complex developer | GPT-5.6 Sol, high; xhigh when warranted | Opus 5, high; xhigh when warranted |
| Routine developer and defined checks | GPT-5.6 Luna, low/medium | Sonnet 5, medium |
| Optional simple text rewrite/paraphrase | Usually direct or Luna | Haiku 4.5, no effort override |

All Luna-equivalent work goes to **Sonnet** in Claude Code. Haiku is limited to menial text transformations, never coding, investigation, testing, review, planning, or release decisions. Tiny rewrites may stay with the orchestrator to avoid dispatch overhead.

Claude selections were checked on 2026-09-04 against [Anthropic's model overview](https://platform.claude.com/docs/en/models/overview). This is a recommended role mapping, not a claim of benchmark equivalence. Exact IDs, supported effort, and availability handling are in the [Codex adapter](references/platform-codex.md) and [Claude adapter](references/platform-claude.md).

The orchestrator handles trivial changes itself when delegation would add overhead. Substantive work normally gets a developer and one independent reviewer. Parallel developers get separate implementation streams and worktrees; review and testing reuse stable checkouts when appropriate. No judge panels or reviewers of reviewers.

The selected tracker, Beads or the canonical local TASKS.md, holds requirements, ownership, dependencies, progress, evidence, and meaningful failure history. In local mode the orchestrator is the only writer; teammates send updates to the orchestrator to avoid conflicting edits. Local `CONTEXT.md` checkpoints hold short resumption notes; they do not duplicate the task graph. Retries need new evidence or a changed approach. Verification focuses on changed behavior, common failures, and required project gates.

## Deployment and cleanup

Agent-Team uses the user's applicable standing deployment and safe-rollback authorization without asking again for every task. Installing the skill is not deployment authorization. Unknown targets, unavailable permissions, and irreversible recovery need the missing decision before proceeding.

The orchestrator verifies the integrated revision before release, checks live behavior afterward, records every successful deployment or recovery in the selected tracker, and removes eligible completed task worktrees only after confirmed deployment. Main, unrelated work, unintegrated changes, and needed evidence are preserved. Prefer platform-native rollback; an instruction file cannot keep monitoring after its runtime stops.

## Repository layout

- `SKILL.md`: shared entrypoint; selects the adapter for the actual host.
- `agents/openai.yaml`: OpenAI display metadata; ignored by Claude.
- `assets/claude-agents/`: installable Sonnet developer/reviewer, Opus complex developer, and restricted Haiku text assistant definitions.
- `references/`: platform adapters and conditional setup, dependency, team, state, UI, and release instructions.
- `legacy/claude-v3/`: preserved previous Claude workflow, inactive and not part of the current installation instructions.
- `CHANGELOG.md`: release history.

The former `astra-dev-harness` personal skill is renamed `agent-team`; avoid keeping two active copies. The public repository is the distribution source. Updates to an installed personal copy are explicit, not automatic two-way synchronization.

## Validation and license

This is an instruction-based skill. Structure, native agent frontmatter, internal links, and workflow consistency are checked; model routing, third-party installers, and production recovery still depend on the actual host/project and must be verified there. No universal cross-platform installation guarantee is made.

[MIT License](LICENSE). Third-party dependencies retain their own licenses.
