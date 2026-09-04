# Agent-Team

A Codex development skill that coordinates GPT-6 Astra, Terra, Luna, and Sol to deliver tracked, verified software changes. Give it a task; it chooses the smallest useful team, preserves task state, and reconciles the result against every requirement.

## Install

Install this repository as the `agent-team` skill using your host's supported skill installer. The repository root contains `SKILL.md` and its supporting references.

For Codex CLI on macOS/Linux, a manual user-wide installation is:

```bash
mkdir -p ~/.agents/skills
git clone https://github.com/thebpandey/agent-team.git ~/.agents/skills/agent-team
```

If that destination already exists, inspect and update your existing installation instead of cloning over it. Use the current host documentation for other installation locations or operating systems. ChatGPT-managed skill installation is separate from installing into your own laptop/server.

Select GPT-6 Astra with high reasoning in a host that exposes the required models and agent controls. A skill cannot switch the parent model or grant access to unavailable models.

Then invoke:

```text
$agent-team setup
```

Or start work directly:

```text
$agent-team Add the requested feature, verify it, and reconcile every requirement.
```

In ChatGPT, select Agent-Team from the available skills or use the host-supported mention. A newly installed skill may require a refresh or later turn before the host exposes it.

## First-run setup

Agent-Team checks installed skills, project packages, runtimes, and Beads before implementation. It shows missing items and a concrete installation plan, then offers one choice:

| Profile | What it installs |
| --- | --- |
| Required + project needs (recommended) | Missing core dependencies and only the optional tools needed by this project |
| All compatible free dependencies | Eligible referenced skills/reference packs and compatible application packages, with exclusions and overlaps shown first |
| Choose individually or defer | Your selected items, or no installation |

It uses your chosen scope, skips existing usable installations, verifies results, remembers the choice locally, and resumes the original task. It does not reinstall or upgrade everything each time. Required dependencies that remain unavailable block affected implementation; optional ones do not.

The full profile does not mean installing every component in every registry. Registries are catalogs, some packages require a compatible app stack, and paid products need existing entitled access. Application packages are deferred when no compatible project exists. Installation cannot bypass OS permissions, host restrictions, hook trust prompts, or missing credentials.

Setup details and official sources: [dependency setup](references/setup.md).

## Required dependencies

- [Ponytail](https://github.com/DietrichGebert/ponytail): simple, complete implementation.
- [Using-Superpowers](https://github.com/obra/superpowers): relevant development procedures.
- [Beads](https://github.com/gastownhall/beads): authoritative task and failure tracking.
- [Impeccable](https://github.com/pbakaus/impeccable): required whenever the task includes UI/UX.

Every teammate receives the applicable skill requirements. Dependencies are installed through their supported distributions, not copied into this repository. The setup workflow handles missing dependencies before enforcing implementation blockers.

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

| Role | Default model | Effort |
| --- | --- | --- |
| Orchestrator and trivial direct work | GPT-6 Astra | High |
| Standard development / independent review | GPT-5.6 Terra | Medium or high |
| Complex developer teammate | GPT-5.6 Sol | High; higher when justified |
| Narrow routine tasks | GPT-5.6 Luna | Low or medium |

Astra handles trivial changes itself when delegation would add overhead. Substantive work normally gets a developer and one independent reviewer. Parallel developers get separate implementation streams and worktrees; review and testing reuse stable checkouts when appropriate. No judge panels or reviewers of reviewers.

Beads holds requirements, ownership, dependencies, progress, evidence, and meaningful failure history. Local `CONTEXT.md` checkpoints hold short resumption notes; they do not duplicate the task graph. Retries need new evidence or a changed approach. Verification focuses on changed behavior, common failures, and required project gates.

## Deployment and cleanup

Agent-Team uses the user's applicable standing deployment and safe-rollback authorization without asking again for every task. Installing the skill is not deployment authorization. Unknown targets, unavailable permissions, and irreversible recovery need the missing decision before proceeding.

Astra verifies the integrated revision before release, checks live behavior afterward, records every successful deployment or recovery in Beads, and removes eligible completed task worktrees only after confirmed deployment. Main, unrelated work, unintegrated changes, and needed evidence are preserved. Prefer platform-native rollback; an instruction file cannot keep monitoring after its runtime stops.

## Repository layout

- `SKILL.md`: compact entrypoint.
- `agents/openai.yaml`: skill display metadata.
- `references/`: conditional setup, dependency, team, state, UI, and release instructions.
- `legacy/claude-v3/`: preserved previous Claude workflow, inactive and not part of the current installation instructions.
- `CHANGELOG.md`: release history.

The former `astra-dev-harness` personal skill is renamed `agent-team`; avoid keeping two active copies. The public repository is the distribution source. Updates to an installed personal copy are explicit, not automatic two-way synchronization.

## Validation and license

This is an instruction-based skill. Structure, internal links, and workflow consistency are checked; model routing, third-party installers, and production recovery still depend on the actual host/project and must be verified there. No universal cross-platform installation guarantee is made.

[MIT License](LICENSE). Third-party dependencies retain their own licenses.
