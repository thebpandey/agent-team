# First-run dependency setup

Run this workflow on first invocation, explicit `setup` or `install dependencies`, environment changes, or newly missing dependencies. A skill is an instruction package, not an automatic package-manager dependency resolver. Perform installation using tools actually exposed by the user's host. Do not claim every dependency is installed simply because this file lists it.

## 1. Inventory before asking

Inspect the host/OS, project stack, available package manager and lockfile, installed skills/plugins, Beads (`bd`), and relevant runtime tools. Check the actual installed skill frontmatter names and paths, including host-managed personal skills and plugin-provided skills. Distinguish installed, usable, missing, incompatible, paid, and available only through a manual host action. A plugin folder alone is not proof the skill is available to teammates.

Check the lightweight local setup receipt if present. Keep `.agent-team/setup.json` local and out of product commits; it records setup choices and availability only, not a second task ledger. Respect an existing file's structure; do not overwrite unrelated user data. Save no credentials. Useful fields: schema version, host/project identity, active tracker and canonical path, chosen profile and scope, dependency source/version/path/status, deferred reason, explicit installation authorization scope, and last verified time. The receipt cannot grant permissions or prove current availability on another host.

## 2. Offer one setup choice

If installation was already authorized for this same concrete scope, proceed without asking again. Otherwise present a concise inventory and the exact proposed changes, then ask one question with these choices:

1. **Recommended dependencies + project needs (Recommended):** offer missing Beads, Ponytail, Using-Superpowers, and Impeccable, identifying Impeccable as useful for UI work; install the accepted items and relevant optional tools.
2. **All compatible free dependencies:** make all eligible referenced skills/reference packs available and install the free runtime packages compatible with this project's stack. List exclusions and any overlapping libraries before the user chooses. Do not add unused packages to an unrelated project or invent a project just to install them.
3. **Choose individually or continue without:** let the user accept any subset or decline all four. Continue immediately using available enabled skills and local-file tracking; none of these four is a hard prerequisite.

The displayed plan must identify names, trusted sources, scope (project/user), proposed versions or source revisions, commands or supported host actions, and meaningful effects such as hooks or package changes. Show only missing/changed items prominently. Package the choice so selecting it authorizes that displayed plan; ask separate questions only for genuinely new scope or enforced approvals. Prefer project scope for application packages and project-specific configuration. Respect an established user-wide skill installation preference. If no project exists, install selected skills/reference assets only and defer application packages until a compatible target exists.

A request to use this skill is not permission to buy subscriptions, change system permissions, overwrite user installations, or trust hooks. Do not ask about those hypothetical changes unless actually needed. Offer entitled paid tools only when existing access is available; never include purchases in the free profile.

## 3. Resolve and install selected items

Use installed-version help and current upstream installation instructions from the sources below. Inspect installer behavior and choose a released version or reviewed revision; record what was resolved. Do not freeze guessed CLI commands in the harness, pipe an uninspected remote script into a shell, force overwrite directories, or install from similarly named unofficial packages. Use the project's package manager and preserve its lockfile conventions. Serially install mutations affecting the same package manifest or host skill store.

| Item | Kind and source | Installation rule |
| --- | --- | --- |
| Ponytail | Recommended skill/plugin: https://github.com/DietrichGebert/ponytail | Resolve `ponytail`; inspect plugin hooks and use the supported host skill/plugin installer |
| Using-Superpowers | Recommended skill in https://github.com/obra/superpowers | Install the supported distribution and referenced companion resources needed by this skill; resolve `using-superpowers`; do not pretend one copied file provides the framework |
| Beads | Recommended tracker: https://github.com/gastownhall/beads | Install the supported `bd` release for the OS; initialize project tracking only after checking existing configuration and concurrent-write support |
| Impeccable | Recommended UI skill/tool: https://github.com/pbakaus/impeccable | Install the correct provider build and required engine/resources; respect hook trust prompts |
| UI UX Pro Max | Optional skill/tool: https://github.com/nextlevelbuilder/ui-ux-pro-max-skill | Use supported provider setup; include its data/search scripts and required Python runtime when authorized |
| UI Skills | Optional registry: https://github.com/ibelick/ui-skills | Select relevant first-party skills such as baseline UI; this registry is not one skill and the full profile does not mean install every external registry entry |
| shadcn/ui | Optional components: https://ui.shadcn.com/docs/installation | Configure only compatible projects; preserve existing components/configuration and add selected components, not the entire catalog |
| Magic UI | Optional skill/components: https://github.com/magicuidesign/magicui | Resolve its published `magic-ui` skill if available; install only selected free components compatible with the project |
| Motion | Optional library: https://motion.dev/docs | Install the free library for the actual stack; skip Motion+ AI Kit/premium products without existing entitled access |
| React Bits | Optional component registry: https://github.com/DavidHDev/react-bits | Add selected components after confirming the MIT + Commons Clause terms fit the intended use; do not treat it as unrestricted MIT |
| Taste Skill | Optional skill: https://github.com/Leonxlnx/taste-skill | Resolve `design-taste-frontend`; use a reviewed revision; exclude the GPT-specific `gpt-taste` variant from automatic selection |
| img2threejs | Optional specialized skill/tool: https://github.com/img2threejs/img2threejs | Include supporting scripts/resources; defer Three.js/runtime additions to an actual 3D task |
| Awesome DESIGN.md | Optional reference collection: https://github.com/VoltAgent/awesome-design-md | Fetch/cache relevant examples, or the reference collection under the full profile; do not overwrite the project's canonical DESIGN.md |
| Bklit UI | Optional skill/components: https://bklit.com/docs/skills | Resolve `bklit-ui`, configure compatible shadcn registry and selected charts; exclude proprietary Studio source |

Git, Node, Python, package managers, browser tooling, and model access are prerequisites only when selected workflows require them. Detect existing tools before adding any. Follow supported OS/host installation procedures; report privileged or manual steps precisely. Do not reconfigure the machine globally simply to satisfy a catalog.

In ChatGPT-managed environments, use the supported personal-skill/plugin management workflow instead of writing to arbitrary skill directories. In Codex or Claude Code, use the selected platform adapter's supported locations and installer. For Claude Code, also provision the bundled native role definitions as described in its adapter, preserving existing definitions and verifying discovery before dispatch. A remote ChatGPT installation does not install software on the user's laptop/server. If the host cannot expose a newly installed skill until a later turn, report that state and give the supported refresh/resume instruction; do not bypass discovery or claim it ran.

## 4. Verify and resume

After installation, verify each selected skill resolves by name and supporting files exist; verify relevant CLIs run their version/help checks and packages resolve in the intended project. Recheck selected skill visibility before spawning teammates and pass the actual availability and fallback mode to every teammate. Use a targeted smoke check when a dependency's behavior or native binary requires it. Reuse unchanged evidence; do not launch the full application test suite solely because documentation skills were added.

Record installed/verified, failed, deferred, incompatible, and excluded items accurately in the setup receipt. Link meaningful setup failures and the receipt from the active task tracker. The receipt records setup choices only; TASKS.md, not the receipt, manages work in fallback mode. Avoid repeated install loops. Preserve successful installs when another fails. If any of Beads, Ponytail, Using-Superpowers, or Impeccable is declined or remains unusable, record the decision, activate local-file tracking, and continue the original task. Keep successful available skills enabled unless declined. Optional installation failure never blocks an otherwise valid task. Only a concrete inability to perform the actual task, such as missing source access or a required runtime, may block that operation.

End setup with a short status and resume the original task automatically using verified dependencies or the documented fallback. A setup-only request ends after the status. Later runs recheck presence and changed environment fingerprints, retain the user's profile and applicable authorization, and ask only for newly proposed scope, never repeatedly for remembered declines. Switching to Beads after local tracking has begun requires explicit user selection and the reconciliation procedure in the state reference. Do not upgrade dependencies on every invocation.
