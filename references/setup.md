# Set up tools and skills

Run this procedure on first use or when the user requests setup. Also check it when the environment changes or a selected tool is missing. A skill contains instructions. It cannot install software without tools supplied by the host. The host is the app that runs the agent, such as Codex or Claude Code.

An explicit setup action displays the [wordmark](wordmark.md) once, with the installed skill version, then the creator credit and GitHub source below it. Preserve `run_defaults` in an existing receipt. Explicit `setup` includes the project settings step below. The standalone [settings](settings.md) action remains available without installing tools or starting work; dependency setup must not silently enable continuous mode or auto-deploy.

## 1. Check what is available

Check the operating system, project software, installed skills, and required tools. Check Beads with its `bd` command. Find each installed skill's name and required files. A folder alone does not prove that the skill works.

Resolve the canonical main checkout from the project record and Git metadata before setup. Read its `.agent-team/setup.json` if it exists; do not create a separate receipt or tracker in each feature worktree. Only the project owner updates the shared receipt or changes tracking mode. Team leads reuse the recorded choices and route new setup needs to that owner. This local file records setup choices, not task progress. Keep it out of app releases and project commits. Preserve unrelated data. Do not store passwords or access tokens.

Record the host, project, task file location, installation scope, tool sources, versions, and status. Also record declined items and the scope of approved installation. A saved record does not grant new permission. Check actual access when the host changes.

## 2. Explain the choices

Use ASD-STE100 principles for all tool descriptions and setup messages. Use short, active sentences. Give one instruction per sentence. Explain technical terms at first use. Preserve official names, file paths, and commands. Use [the tool guide](dependencies.md) for the explanations.

Show what each item does, why the project needs it, and its current status. Use simple status labels: Ready, Missing, Cannot use, Paid access needed, or Manual step needed.

Present missing Beads, Ponytail, Using-Superpowers, and Impeccable. Also show all optional design tools from the guide. Explain which optional items fit the project. Do not hide the other choices because they are not needed now.

If the user already approved the same installation plan, continue. Otherwise, show the proposed changes and ask one question with these choices:

1. **Recommended tools for this project (Recommended).** Install the accepted recommended tools and the optional tools needed by this project.
2. **All free tools that work here.** Install accepted free skills and reference files. Add app packages only to a project that needs them.
3. **Choose tools or skip installation.** Install only the selected items. If the user declines the four recommended tools, continue with a local task file.

Before the user chooses, show each source, version, installation location, and required command or host action. Explain any automatic scripts or changes to project files. A hook is a script that runs automatically after a specified event. Do not approve hook access for the user.

State whether an item is for this project or all projects for the user. Identify duplicate tools, paid features, and items that cannot work here. The selected choice approves only the displayed plan. Ask again only for a new scope or a required approval.

Keep app packages in the project. Respect an existing preference for user-wide skills. If no project exists, install selected skills and reference files only. Defer app packages until a suitable project exists.

Use of this skill does not approve purchases, new system permissions, or replacement of user installations. Offer paid features only when the user already has the required access.

## 3. Install the selected items

Use current installation instructions from each official source. Inspect each installer before use. Select a released version or an inspected source revision. Record that choice. Do not guess commands or run uninspected downloaded scripts.

Use the project's package manager. Preserve the lockfile, which records package versions. Install changes to the same package list or skill store one at a time.

| Item and official source | Installation instructions |
| --- | --- |
| [Ponytail](https://github.com/DietrichGebert/ponytail) | Find the `ponytail` skill. Inspect its hooks. Use the host's supported installer. |
| [Using-Superpowers](https://github.com/obra/superpowers) | Install the supported package and required companion files. Find `using-superpowers`. One copied instruction file is not the complete tool. |
| [Beads](https://github.com/gastownhall/beads) | Install `bd` for the operating system. Check existing project settings before setup. Confirm safe access before multiple agents write task updates. |
| [Impeccable](https://github.com/pbakaus/impeccable) | Install the version for the selected host. Include its required files. Respect approval requests for hooks. |
| [UI UX Pro Max](https://github.com/nextlevelbuilder/ui-ux-pro-max-skill) | Use the setup for the selected host. Include its data and search programs. Install Python if needed and approved. |
| [UI Skills](https://github.com/ibelick/ui-skills) | Select the instructions needed by the task. The collection contains separate skills. Do not install every linked external skill. |
| [shadcn/ui](https://ui.shadcn.com/docs/installation) | Check that the app supports it. Preserve existing settings and components. Add only selected components. |
| [Magic UI](https://github.com/magicuidesign/magicui) | Find the published `magic-ui` skill if available. Add only selected free components that work with the project. |
| [Motion](https://motion.dev/docs) | Install the free package for the project's software. Exclude paid Motion+ features unless the user already has access. |
| [React Bits](https://github.com/DavidHDev/react-bits) | Check the MIT + Commons Clause conditions. Add selected components only when the intended use meets those conditions. |
| [Taste Skill](https://github.com/Leonxlnx/taste-skill) | Find `design-taste-frontend`. Use an inspected revision. Do not automatically select the separate `gpt-taste` variant. |
| [img2threejs](https://github.com/img2threejs/img2threejs) | Include its required programs and files. Add Three.js only for an actual 3D task. Three.js displays 3D scenes in a browser. |
| [Awesome DESIGN.md](https://github.com/VoltAgent/awesome-design-md) | Save relevant examples, or the reference collection under the full setup choice. Preserve the project's approved `DESIGN.md`. |
| [Bklit UI](https://bklit.com/docs/skills) | Find `bklit-ui`. Set up its component source for a compatible shadcn app. Add selected charts. Exclude private Studio code. |

Install Git, Node.js, Python, package managers, and browser tools only when the selected work requires them. Explain any step that needs administrator access. Do not change global machine settings only to complete the list.

Use the host's supported installation method. Follow the selected platform adapter. In Claude Code, also update managed agent definitions as that adapter specifies. Preserve user changes. Check that the host can find those definitions before you assign work.

For the built-in Agent-Team hook package, follow the [lifecycle hook guide](hooks.md). The managed installer puts Codex at `~/.agents/skills/agent-team`, puts Claude Code at `~/.claude/skills/agent-team`, and merges only Agent-Team hook groups. It creates backups before it replaces a skill or changes a host configuration. It backs up and removes the old `~/.codex/skills/agent-team` duplicate. Do not run this user-wide install without explicit authority. Do not claim native hook trust; report the required `/hooks` action when the host requires it.

An installation in ChatGPT does not install software on the user's computer. If a new skill is not yet visible, give the supported refresh instruction. Do not claim that an unavailable skill ran.

## Pro browser review tools

For Pro UI review, first check the host's available browser controls. If a tool is missing, explain the choices in [visual browser review](visual-review.md). Offer one suitable tool only through the supported installation process. Do not add browser packages solely to complete a catalog. Include the Pro visual tester definition only in Pro installations. Agent Team Lite must not contain these dependency choices or this review feature.

## 4. Check the result and continue

Check that each selected skill is available by name. Check that its required files exist. Run version or help checks for installed tools. Check that app packages are available in the intended project.

Tell each teammate which tools are available and which task record to use. Run a small functional check if a selected tool needs one. Do not run all app tests only because a documentation skill was added.

Record each result: Ready, Failed, Deferred, Cannot use, or Excluded. Link meaningful setup failures from the active task record. Preserve successful installations when another installation fails. Do not repeat an unchanged failed installation.

If any recommended tool is declined or unusable, select local-file mode and continue the original task. Use `.agent-team/TASKS.md` in the main checkout. Keep the other available skills enabled. Optional tool failures do not stop feasible work. Report a blocker only when the actual task cannot proceed.

For explicit `setup`, complete the project settings step below before the final status, even when tools are already ready or installation is skipped. Automatic dependency checks during start, resume, or recovery do not open the settings flow.

After setup, give a short status and continue the task. A setup-only request ends with that status. Reuse saved choices on later runs. Do not repeatedly ask about declined tools. Do not upgrade tools on every run. A later move to Beads needs the user's selection and the safe transfer procedure in [state and recovery](state.md).


## 5. Set project defaults

On every explicit `setup`, automatically show the effective project defaults from [settings](settings.md): parallel teams, continuous mode, auto-deploy, and deployment batch size. Show whether each value is saved or built-in. Offer to keep these values or change them within setup; the user does not need to invoke `settings` separately. Apply clear settings choices already supplied with the setup request without asking again.

Use the existing settings procedure to validate and save only the user's chosen changes in the canonical project's `run_defaults`. Keep or skip leaves existing defaults unchanged. An unanswered offer does not authorize a change. Preserve dependency results and all other receipt fields. Settings affect future starts only; setup does not start teams, change an active run, or deploy. Show the wordmark only once for the whole setup invocation.

If no project is available, explain that defaults are project-only and defer this step until the user identifies a project. Do not create user-wide defaults. Include the saved, unchanged, or deferred settings result in the final setup status.
