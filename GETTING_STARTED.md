# Agent-Team: a first-time setup guide

Agent-Team helps Codex and Claude Code organize development work across a small team of agents. This guide uses **prompts that you paste into your AI host**. You do not need to run Bash commands yourself.

The Agent-Team repository is private. You need a GitHub account that can read [`thebpandey/agent-team`](https://github.com/thebpandey/agent-team) before you begin.

## 1. Prepare your computer and GitHub account

You need the following before Agent-Team can be installed. Your AI host can check or install local software after you approve its plan, but it cannot complete a browser sign-in, grant GitHub access, buy model access, or approve hooks for you.

| Requirement | Why it is needed | Who installs or approves it? |
| --- | --- | --- |
| Codex or Claude Code | This is the host that runs Agent-Team. | You install and sign in to the host. |
| Git | Downloads the repository and creates safe work folders. | You, or your agent after it shows an install plan and you approve it. |
| GitHub CLI (`gh`) | Signs in to GitHub and verifies access to the private repository. | You, or your agent after approval. |
| GitHub account with repository access | Lets GitHub CLI download Agent-Team. | A repository owner or organization administrator grants this. |
| A web browser | Completes the GitHub CLI sign-in flow. | You complete the sign-in. |
| Node.js 24 | Required only if you want Agent-Team lifecycle hooks. | You, or your agent after approval. |
| Model and project access | Lets the host run agents and read your project. | You or your organization supplies this access. |

Start Codex or Claude Code and paste this prompt:

```text
Check whether Git, GitHub CLI (gh), and Node.js 24 are available on this computer. Do not change anything yet. Report what is missing, the official installation method for this operating system, and every file or setting that installation would change.

I need access to the private GitHub repository https://github.com/thebpandey/agent-team. If gh is missing, ask my permission before installing it. If gh is present but not authenticated, guide me through its browser-based GitHub sign-in. Do not ask me to paste a password, personal access token, or secret into this chat. Use the normal secure credential store when available. Configure Git to use this authenticated GitHub account, then verify that the account can read the Agent-Team repository. Stop and explain any access error.
```

Approve only the installation plan that you understand. During GitHub CLI sign-in, open the link or enter the device code shown by the host and approve it in your browser. The normal GitHub CLI sign-in uses a browser flow and stores its token in the system credential store when one is available. See [GitHub CLI authentication](https://cli.github.com/manual/gh_auth_login).

If the repository check fails, ask the repository owner to give your GitHub account access. Installing `gh` alone does not grant access to a private repository.

## 2. Install Agent-Team from GitHub

Choose one host. A user-wide install is usually easiest because it makes the skill available in all of your projects. A project-only Claude Code install is useful when a team wants to keep the skill inside one repository.

### Codex

Open Codex and paste this prompt:

```text
Install the complete Agent-Team skill from the main branch of https://github.com/thebpandey/agent-team using my authenticated GitHub CLI access. First inspect any existing Agent-Team installation, its source, and local changes. Do not overwrite a customized installation or create a duplicate.

For a user-wide Codex installation, place the complete package at ~/.agents/skills/agent-team. Keep SKILL.md, references/, agents/, assets/, hooks/, README.md, and LICENSE together. Do not copy only SKILL.md. If Node.js 24 is available and I approve, use the bundled managed installer to register Agent-Team lifecycle hooks. Preserve unrelated Codex settings and report exactly what changed. Verify that Codex can discover Agent-Team, then tell me whether I must reload this session.
```

### Claude Code

Open Claude Code and paste this prompt:

```text
Install the complete Agent-Team skill from the main branch of https://github.com/thebpandey/agent-team using my authenticated GitHub CLI access. First inspect any existing Agent-Team installation, its source, and local changes. Do not overwrite a customized installation or create a duplicate.

For a user-wide Claude Code installation, place the complete package at ~/.claude/skills/agent-team. For a project-only installation, use .claude/skills/agent-team in this project instead. Keep SKILL.md, references/, agents/, assets/, hooks/, README.md, and LICENSE together. If Node.js 24 is available and I approve, use the bundled managed installer to register Agent-Team lifecycle hooks and the bundled Claude role definitions. Preserve unrelated Claude settings and customized role definitions. Verify that Claude Code discovers the skill and role definitions, then tell me whether I must reload this session.
```

The managed hook installer copies the authoritative skill to the normal host location, merges only Agent-Team hook groups, and preserves unrelated settings. It can report a conflict instead of overwriting a customized file. That is expected: review the conflict before you decide how to proceed.

## 3. Reload the host and trust lifecycle hooks

Close and reopen Codex or Claude Code, or use its supported refresh action. A new session is the safest choice after a first install or an update.

Then ask the host to show its hook settings:

```text
Open /hooks and show me the Agent-Team lifecycle hooks that were registered. Explain what each group does and whether the host reports it as trusted. Do not modify or approve hooks for me.
```

Hooks run small automatic checks, recovery checkpoints, and reminders. They do not replace agent review. Hook installation and hook trust are separate actions.

If the host asks whether to trust hooks, review the displayed Agent-Team paths and choose the native trust option yourself. If you want to trust every currently registered hook, choose **Trust all** in the host’s own confirmation screen. Do this only after reviewing the listed hooks; an agent must not approve hook access on your behalf. If the host does not show a trust prompt, continue and ask the agent to report hook health.

Finally, confirm that the skill is available:

```text
Show Agent-Team help and confirm the installed version, installation path, hook-registration status, and whether hooks are trusted. Do not start development yet.
```

Use `$agent-team help` in Codex and `/agent-team help` in Claude Code if you prefer to invoke the skill directly. These are host skill instructions, not terminal commands.

## 4. Run setup and choose dependencies

In Codex, paste `Run $agent-team setup.` In Claude Code, paste `Run /agent-team setup.` You can also say, “Set up Agent-Team for this project.”

Setup does four things:

1. Detects the host and checks the project, installed skills, tools, and saved setup record.
2. Shows each missing dependency one at a time, with its source, version, installation location, scope, files/settings changed, and hook or permission effect.
3. Lets you select **Install now**, choose another safe scope, **Skip and remember**, or cancel. It never has permission to install a dependency merely because you ran setup.
4. Opens the complete project settings wizard after the dependency choices, even if you skip everything.

Use this prompt if you want the agent to explain each choice before asking for approval:

```text
Run Agent-Team setup for this project. Check each dependency first. For every missing item, explain what it does, whether Agent-Team can install it automatically, its source and version, its scope, every file or configuration it would change, and whether it needs a separate approval. Ask me one dependency at a time. Do not install anything until I choose the displayed installation option. After dependency setup, continue to the complete settings wizard.
```

### Dependency order and responsibility

Use this order to make decisions. Agent-Team checks selected items individually; it does not silently install this whole list.

| Order | Item | Is it required? | Who handles it? |
| --- | --- | --- | --- |
| 1 | Git, `gh`, GitHub access, Codex or Claude Code | Yes for this GitHub installation path. | Install/sign in before setup. Your agent can propose local installation; you approve it. |
| 2 | Node.js 24 | Required for the optional lifecycle hooks. | Install before hook registration, or skip hooks for now. |
| 3 | Agent-Team package | Yes. | Install from GitHub with the prompts in step 2. |
| 4 | Ponytail, Using-Superpowers, Beads, Impeccable | Recommended, not required. | Setup offers automatic installation for each missing item after your explicit choice. |
| 5 | LeanCTX | Recommended runtime context tool. It does not select the task tracker. | Setup can offer its approved user-scope installation; it requires separate configuration and hook/permission review. |
| 6 | UI/design tools | Optional. | Setup explains them. Install only tools that suit a real UI task. |
| 7 | Task-specific software, such as Python, a package manager, or browser tools | Required only by the work you select. | The agent checks first and asks before installing. |

The four recommended tools in row 4 determine the task tracker. If all four are available and Beads is selected, Agent-Team uses Beads to record work. If any of them is declined, unavailable, or fails to install, Agent-Team continues with the local `.agent-team/TASKS.md` tracker. This is a normal fallback, not a failed setup.

LeanCTX does not affect that tracker choice. If LeanCTX is skipped or cannot be installed, Agent-Team continues with normal source tools unless you make LeanCTX a requirement.

## 5. Configure project settings

Setup automatically opens the settings wizard. It asks one question at a time and saves settings for the current project only. They do not change an active run or give permission to deploy.

| Setting | What it controls | Initial recommendation |
| --- | --- | --- |
| Parallel teams | How many independent development teams can work at once (1-6). | `1` until you have independent, well-defined tasks. |
| Continuous mode | Whether a finished team is replaced with another ready task. | Off for a first run. |
| Auto-deploy | Whether verified completed work deploys automatically in batches. Normal release checks still apply. | Off until you know the release target and process. |
| Deployment batch size | Number of completed top-level tasks in each automatic deployment. | Follow the effective team limit. |
| Role routing | Model and reasoning effort for the orchestrator, developers, reviewers, and visual reviewer. | Keep the host adapter defaults unless you have a concrete reason to change them. |

Review the summary at the end and choose **Save**. You can use **Back** to revise a prior choice or **Cancel without saving** to discard the settings draft.

To change defaults later, use this prompt:

```text
Run Agent-Team settings for this project. Show the current effective values and their sources. Walk me through each setting one at a time, including every role’s model and effort. Keep the current value unless I explicitly select a replacement. Save only after showing me the final summary.
```

Settings affect future starts. For one run, you can override them in the start request without changing saved defaults.

## 6. Start development

Agent-Team starts from a recorded, ready task. A bare start chooses one ready task from Beads or `.agent-team/TASKS.md`. A named start selects one existing task. It does not invent work merely because a task name was typed.

For a new feature, paste a clear request such as:

```text
Use Agent-Team for this feature: <describe the feature, acceptance criteria, constraints, and affected project>. First check for duplicate or unfinished work. Create or reconcile the canonical task, show me the selected tracker and effective run settings, then start one team. Keep continuous mode and auto-deploy off for this first run. Verify the result and report every requirement as implemented, verified, deferred, or blocked. Do not deploy.
```

For work that is already listed, use this prompt:

```text
Start Agent-Team on the existing task named <task name>. Use one team, do not use continuous mode or auto-deploy, and show me the effective run settings before work begins.
```

Useful follow-up requests:

| Goal | What to say |
| --- | --- |
| Check progress | `Show Agent-Team status for this project. Do not interrupt work.` |
| Start several independent tasks | `Start Agent-Team with up to 3 teams for ready independent tasks. Do not enable continuous mode or auto-deploy.` |
| Keep refilling teams | `Start Agent-Team with 2 teams in continuous mode.` |
| Require approval before integration | `Start Agent-Team on <task name> with preview approval required.` |
| Pause or recover work | `Pause Agent-Team` or `Resume Agent-Team`. The host shows a team picker when needed. |

## If something does not work

| Problem | What to do |
| --- | --- |
| Agent-Team is not found | Start a new Codex or Claude Code session. Ask the host to verify the installed path and show Agent-Team help. |
| GitHub download fails | Ask the agent to show `gh` authentication status and repository-access error without exposing secrets. Confirm that your GitHub account can read the private repository. |
| Hooks are installed but untrusted | Open `/hooks`, review the Agent-Team entries, and make the native trust choice yourself. Then reload the host if it asks. |
| Node.js is missing | Install Node.js 24 after reviewing the plan, then repeat only hook registration or health checks. The skill itself can still be present without hooks. |
| A recommended dependency is skipped | Continue. Agent-Team uses `.agent-team/TASKS.md` when the four recommended tracker tools are not all ready. |
| Claude role definitions are missing | Run setup again and ask it to inspect and register the bundled Claude definitions without overwriting customized files. Restart Claude Code if necessary. |
| You move from Codex to Claude Code, or the reverse | Run setup or settings in the new host. Agent-Team preserves run defaults but resets project role routing to the new host’s valid defaults. |

## A safe default for your first run

For a first project, use one team, continuous mode off, auto-deploy off, and the adapter’s default models and effort. Add more parallel teams only when tasks are truly independent. Turn on automatic deployment only after you have confirmed the release target, authority, preview policy, and recovery process.
