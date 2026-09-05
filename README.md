# Agent-Team

Codex and Claude Code spin up a team of agents to get the job done faster and more efficiently. Agents work in a review-remediate loop to ensure clean, efficient code.

Agent-Team is a development skill for **Codex and Claude Code**. A skill is a set of instructions for an AI agent. The host is the app that runs the agent.

Give Agent-Team a task. It selects a suitable team, records progress, checks the result, and reports which requirements it met. The lead agent is called the **orchestrator**. It assigns work and controls the final checks.

Tool descriptions use short sentences and consistent terms based on [ASD-STE100 principles](https://www.asd-ste100.org/STE_faq.html). Official tool names and commands keep their original form.

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

Both platforms use the same main instructions and tool list. Agent-Team loads the instructions for the current host. Claude agent definitions are included. Codex uses its available agent controls.

## Start, status, pause, resume, and approve

These are instructions understood by Agent-Team, not new commands added to the host's terminal. Codex uses `$agent-team`; Claude Code uses `/agent-team`. Clear plain-language requests such as “agent team start” also work.

| Command in Codex | What happens |
| --- | --- |
| `$agent-team start` | Select the next ready, unassigned task from Beads or the configured local task file. Start one team, assign its team number and readable name, and preserve its task ID. |
| `$agent-team start with-preview` | Select the next ready task and require your preview approval before integration. |
| `$agent-team start email-preferences with-preview` | Start the named feature, provide a local preview, and wait for your approval before integration. |
| `$agent-team status` | Report the current team's progress, or the project overview from a project session. Do not interrupt development. |
| `$agent-team status email-preferences` | Report one team's tasks, progress, blockers, preview, and release state. A team ID such as TEAM-002 also works. |
| `$agent-team status all` | Report all teams and unassigned work in the current project. |
| `$agent-team pause` | Safely pause all teams in the current project, including when issued from a feature session. `pause all` is equivalent. |
| `$agent-team pause email-preferences` | Save progress and safely pause that team. Preserve its unfinished files. |
| `$agent-team resume` | Recover all unfinished teams in this project, including paused teams and incomplete integration, deployment, or cleanup. |
| `$agent-team resume email-preferences` | Recover only the named team. |
| `$agent-team approve email-preferences` | Approve the submitted preview version for integration. Existing integration and deployment checks still apply. |

`start` creates new work; it does not resume other teams. Each start creates at most one feature team. It does not drain the entire task list. If no task is ready, Agent-Team explains why and creates no team. `resume` retains team identities, checks for surviving agents, and continues from the actual unfinished stage. It does not restart completed work or bypass approval.

### Named teams and work folders

A team has a stable number such as `TEAM-001` and a readable name such as `lesson-progress`. Session replacement does not change its team number. The project keeps one directory of team identities and one authoritative task tracker. Each task has one implementation owner. Independent team sessions depend on host support; otherwise, one orchestrator can manage named developer groups without claiming independent sessions.

The main checkout is used for project initialization, planning, and shared records. All later feature changes use separate worktrees. The project orchestrator combines branches one at a time in a separate integration worktree, checks the combined result, then updates main through the established merge process. It does not merge just because a feature team reports success.

### Status without interruption

Status reads recorded progress. It does not ask agents for fresh reports, run tests, change tasks, or pause work. It shows the source and age of the information. Active work continues where the host supports it; the orchestrator may briefly use a turn to answer.

| Team | Total | Completed | In progress | Not started | Blocked | Remaining | Complete |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| TEAM-001 / lesson-progress | 10 | 6 | 2 | 1 | 1 | 4 | 60% |
| TEAM-002 / email-preferences | 8 | 2 | 3 | 3 | 0 | 6 | 25% |
| Project total | 18 | 8 | 5 | 4 | 1 | 10 | 44% |

This is an example. Completion is completed tasks divided by included tasks. Remaining includes active and blocked tasks. Count actionable tasks once; exclude summary groups, cancelled tasks, and approved deferrals. Show unassigned and unknown work when present. Zero tasks means N/A. This measures tasks, not time or effort. Approval and production status are reported separately.

### Preview before integration

`with-preview` creates a required approval gate. The team uses the project's existing development or preview command and its own available port. It checks the page and gives you the address, review version, and feature summary. The source remains stable while you review it.

Passing tests does not replace your approval. If the preview fails, integration stays blocked. Feedback produces a new review version. Material changes after approval require renewed approval. Pause and resume preserve this requirement. Approval permits integration of that version, not an unchecked production deployment.

A development preview is not a production deployment. `localhost` refers to the machine running the server. Viewing a server preview from another device needs an approved private address or tunnel. The harness does not expose ports publicly without authorization.

### Recovery and required cleanup

Unqualified `pause` pauses the current project's teams; a name or ID limits it to one team. The lead stops new assignments and integrations, saves checkpoints, and reports each team's result. Operations that cannot safely stop are reported as still stopping or unknown. The harness does not claim a complete pause until the affected writers and release activity have stopped safely. It preserves worktrees, preview approval, and safely running previews with frozen source. Other projects are unaffected.

Each agent saves short checkpoints during meaningful progress and before a requested pause. Resume reads those notes, current files, task evidence, active processes, approval gates, and release records. It checks whether earlier agents are still writing before replacing them. If deployment already succeeded, it records the result and continues remaining verification or cleanup instead of deploying again.

After successful production verification, cleanup is required. Stop task-owned previews and processes, preserve needed evidence, and remove all eligible feature and child worktrees, including their disposable build files and local dependency folders. Reuse at most one integration worktree while it is needed; remove it when idle and eligible. Preserve main, unfinished work, user files, and shared resources. Record retained worktrees and failed cleanup so resume can finish it. Report recovered disk space only when measured.

A skill cannot guarantee a final checkpoint after a crash or recover code from a lost temporary workspace. It also cannot keep servers or agents running after the host stops unless the host provides that capability.

See the [action rules](references/actions.md), [team coordination](references/projects.md), [recovery procedure](references/recovery.md), [status rules](references/status.md), and [preview gate](references/preview.md).

## How Agent-Team works

The first flowchart shows named-team development, read-only status, and preview approval. Remediate means to repair a problem found during review. A lead can handle small feature work in its assigned feature worktree. Larger tasks use developers and one focused review.

[![Setup and development flowchart, including the review and repair loop](assets/diagrams/setup-development.png)](assets/diagrams/setup-development.svg)

[Open the full-size flowchart](assets/diagrams/setup-development.svg) · [Mermaid source](assets/diagrams/setup-development.mmd)

Agent-Team uses Beads or one local `TASKS.md` file. If any recommended tool is missing or declined, it uses the local file. Each agent saves short resume notes in its assigned `CONTEXT.md` file. The task record holds progress and failure history.

After two attempts without useful progress, the lead agent changes the approach or assigns a more capable developer. It does not repeat the same failed attempt. If the new approach also fails, it asks one focused question or reports the blocker.

The second diagram shows serial integration, release, recovery, and required cleanup. **Deployment** means publication of a checked app version to the intended destination. **Rollback** means restoration of an earlier working version.

[![Release and recovery flowchart, including authorization and live checks](assets/diagrams/release-recovery.png)](assets/diagrams/release-recovery.svg)

[Open the full-size flowchart](assets/diagrams/release-recovery.svg) · [Mermaid source](assets/diagrams/release-recovery.mmd)

Installation does not grant deployment permission. Agent-Team reuses an existing approval for the same target. It restores an earlier version only when the action is authorized and safe. It does not automatically reverse destructive data changes. If recovery fails, it stops further releases and reports the incident.

After every successful release or recovery, Agent-Team updates the active task record. A recovery does not mean that the requested feature is complete. Before work-folder removal, it preserves required evidence and checks that the work is deployed and verified. It keeps the main folder, unrelated work, and unfinished work.

## Pro quality and project memory

This repository currently includes the full package that forms the basis for Pro. Agent Team Lite is planned separately. The features below are excluded from Lite.

| Feature | What it does |
| --- | --- |
| [Visual browser review](references/visual-review.md) | A reviewer opens the app and inspects desktop and mobile screenshots. It checks the changed screens against the agreed design. |
| [End-to-end acceptance tests](references/acceptance-tests.md) | Checks that a main user task works through the connected app, including the saved result when needed. |
| [Explanations beside code](references/code-explanations.md) | Adds short, plain-English explanations of a feature's purpose and important rules. Updates a feature guide when several files or setup steps need explanation. |
| [Shared mistake lessons](references/mistakes-memory.md) | Keeps confirmed agent mistakes, corrections, and prevention steps in one project `MISTAKES.md` file. |

These checks use the existing review and repair loop. A separate visual tester is optional for substantial screen changes. Browser review uses supported host tools or a suitable available tool such as Playwright or Agent Browser. Capturing screenshots without inspecting them does not complete visual review. Simulated tests do not prove that the real app integration works. Unavailable checks are reported as blocked.

For each feature, the developer adds or updates the nearby explanation and reports its location in the task record. The existing reviewer checks that it matches the code. Small changes do not require a separate feature guide or documentation review pass.

All teammates read relevant mistake lessons before work and retries. Only the lead agent updates the shared file. Repeated mistakes update an existing lesson. Old advice is corrected or marked as superseded. This file supplies project context; it does not train the model.

| Record | Purpose |
| --- | --- |
| Beads or `.agent-team/TASKS.md` | Requirements, ownership, task status, attempts, and verification evidence. |
| Each agent's `CONTEXT.md` | Short resume notes and the next action. |
| Main project `MISTAKES.md` | Confirmed mistakes and actions that help prevent repeats. |

## First-run setup

A dependency is a tool or skill that helps Agent-Team do a task. Agent-Team checks what is installed before it offers changes. It explains each tool in simple terms.

| Choice | What happens |
| --- | --- |
| Recommended tools for this project | Install the accepted recommended tools and useful optional tools. |
| All free tools that work here | Install accepted free skills and reference files. Add app packages only where needed. |
| Choose tools or skip installation | Install selected items, or continue without new tools. |

Before installation, you see the source, version, location, and proposed changes. Agent-Team remembers declined tools. It does not ask about them again unless you change the choice. It checks each selected installation before use.

A package is software that you can install. Some packages work only with specific app software. A collection can contain many separate components. Agent-Team adds only the needed parts. Paid features require existing access. Installation cannot bypass permissions or required approvals.

See the [setup procedure](references/setup.md) for exact installation rules.

## Recommended tools

These tools are recommended, not mandatory. You can decline any of them and continue with the local task file.

| Tool | What it does | When Agent-Team uses it |
| --- | --- | --- |
| [Ponytail](https://github.com/DietrichGebert/ponytail) | Helps the agents write simple code that meets the task requirements. | All agents use it when available and enabled. |
| [Using-Superpowers](https://github.com/obra/superpowers) | Gives the agents procedures to plan, build, find faults, and check their work. | All agents use the procedures that apply to their tasks. |
| [Beads](https://github.com/gastownhall/beads) | Stores tasks, task owners, progress, and records of failures. Shows which tasks must finish before other tasks can start. | Use it when Beads is selected and all four recommended tools are ready. Keep an existing local-file choice until changed. |
| [Impeccable](https://github.com/pbakaus/impeccable) | Helps the agents design and check clear, consistent app screens. | Use it for screen design, layout, and user controls. |


Every teammate uses the available skills selected for its task. Missing skills are not reported as used.

## Optional design tools

UI means the screens and controls that a person uses. UX means how easy the product is to understand and use. A component is a reusable screen part, such as a button.

React is software for building app screens.

Agent-Team offers all the options below during setup. It explains which ones fit the project.

| Tool | What it does | Use it when |
| --- | --- | --- |
| [UI UX Pro Max](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill) | Gives the agents searchable examples of colors, fonts, and screen layouts. | A new design needs more reference examples. |
| [UI Skills](https://github.com/ibelick/ui-skills) | Provides a collection of separate design instructions. | One of those instructions helps with the current task. |
| [shadcn/ui](https://ui.shadcn.com/docs/installation) | Provides screen parts, such as buttons, forms, and menus. | The app needs reusable controls and supports this tool. |
| [Magic UI](https://github.com/magicuidesign/magicui) | Provides ready-made visual effects and screen parts with movement. | An effect helps explain or improve part of the page. |
| [Motion](https://motion.dev/docs) | Adds controlled movement to screen parts. | Simple built-in page styles cannot provide the required effect. |
| [React Bits](https://github.com/DavidHDev/react-bits) | Provides visual effects for text, backgrounds, and user controls in React apps. | The app needs a specific effect and meets the license conditions. |
| [Taste Skill](https://github.com/Leonxlnx/taste-skill) | Gives design instructions for a distinct visual style. | A landing page, portfolio, or major redesign needs more design direction. |
| [img2threejs](https://github.com/img2threejs/img2threejs) | Helps build a 3D scene from reference images. | Users need to view or interact with a 3D object. |
| [Awesome DESIGN.md](https://github.com/VoltAgent/awesome-design-md) | Provides written examples of colors, fonts, spacing, and layouts. | The agents need a useful design reference. This is a reference collection, not a program. |
| [Bklit UI](https://bklit.com/docs/skills) | Provides charts for app screens. | A dashboard needs to show measurements, totals, or changes over time. |


These tools are optional. Agent-Team does not install whole component collections or add unused packages. Motion+ features and Bklit Studio are separate products. React Bits has additional Commons Clause license conditions. Agent-Team does not automatically select the separate GPT-specific Taste variant.

See the [tool guide](references/dependencies.md) for tools needed by specific tasks, such as Git, Node.js, and Python. See the [UI procedure](references/ui.md) and [optional design guide](references/ui-optional.md) for selection rules.

## Team and workflow

| Role | Codex | Claude Code |
| --- | --- | --- |
| Project/team orchestrator; small work in feature worktree | GPT-6 Astra, high | Fable 5.1, high |
| Standard developer | GPT-5.6 Terra, medium/high | Opus 5, high |
| Independent reviewer | GPT-5.6 Terra, medium/high | Opus 5, high |
| Pro visual and acceptance judgment | GPT-5.6 Terra, high | Opus 5, high |
| Complex developer | GPT-5.6 Sol, high; xhigh when warranted | Opus 5, xhigh |
| Routine developer and defined checks | GPT-5.6 Luna, low/medium | Sonnet 5, high |
| Optional simple text rewrite/paraphrase | Usually direct or Luna | Haiku 4.5, no effort override |

Claude routing uses **Opus xhigh for Sol-level work**, **Opus high for Terra-level development and review**, and **Sonnet high only for Luna-level work**. “Extra effort” means the native `xhigh` setting. If a required tier is unavailable, the harness reports it instead of silently downgrading. Haiku is limited to menial text transformations, never coding, investigation, testing, review, planning, or release decisions. Tiny rewrites may stay with the orchestrator to avoid dispatch overhead.

Claude selections were checked on 2026-09-04 against [Anthropic's model overview](https://platform.claude.com/docs/en/models/overview). This is a recommended role mapping, not a claim of benchmark equivalence. Exact IDs, supported effort, and availability handling are in the [Codex adapter](references/platform-codex.md) and [Claude adapter](references/platform-claude.md).

The lead handles small feature changes in a feature worktree when a teammate would add unnecessary work. Larger changes normally use a developer and one independent reviewer. A worktree is a separate project folder managed by Git. Developers use separate worktrees for independent changes. Reviewers can reuse a stable folder. Agent-Team does not create judge panels.

Beads or the local `TASKS.md` file holds the task record. In local-file mode, only the lead agent writes task updates. Teammates send their results to the lead agent. Each `CONTEXT.md` file holds short resume notes, not a second task list. Checks cover changed behavior, common failures, and project requirements.

## Deployment and cleanup

The release diagram above shows the main decisions. Agent-Team checks the combined changes before release and checks the live app afterward. It records results before it removes completed work folders. It reports the result for every user requirement.

A skill does not run after its host stops. Use the deployment service's own health checks and recovery features when available.

## Repository layout

- `SKILL.md`: shared entrypoint; selects the adapter for the actual host.
- `agents/openai.yaml`: OpenAI display metadata; ignored by Claude.
- `assets/claude-agents/`: installable Opus developer/reviewer and complex developer, Sonnet routine developer, restricted Haiku text assistant, and Pro visual tester definitions.
- `references/`: platform adapters and conditional setup, dependency, team, state, UI, release, named-team, recovery, status, and preview-approval instructions.
- `legacy/claude-v3/`: preserved previous Claude workflow, inactive and not part of the current installation instructions.
- `CHANGELOG.md`: release history.

The former `astra-dev-harness` personal skill is renamed `agent-team`; avoid keeping two active copies. The public repository is the distribution source. Updates to an installed personal copy are explicit, not automatic two-way synchronization.

## Validation and license

This is an instruction-based skill. Structure, native agent frontmatter, internal links, and workflow consistency are checked; model routing, third-party installers, and production recovery still depend on the actual host/project and must be verified there. No universal cross-platform installation guarantee is made.

[MIT License](LICENSE). Third-party dependencies retain their own licenses.
