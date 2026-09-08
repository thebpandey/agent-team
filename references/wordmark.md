# AGENT-TEAM wordmark

On first setup or requested help, this optional wordmark may precede the compact summary when the terminal is wide enough. Ordinary start, resume, settings, status, refills and progress updates omit it. Use a fenced plain-text block so spacing stays intact:

```text
 ███   █████  █████  ██  ██  ██████        ██████  █████   ███   ██   ██
██ ██  ██     ██     ███ ██    ██            ██    ██     ██ ██  ███ ███
██ ██  ██     ████   ██████    ██    ████    ██    ████   ██ ██  ███████
█████  ██ ██  ██     ██ ███    ██    ████    ██    ██     █████  ██ █ ██
██ ██  ██ ██  ██     ██  ██    ██            ██    ██     ██ ██  ██   ██
██ ██  █████  █████  ██  ██    ██            ██    █████  ██ ██  ██   ██
```

Immediately below the installed skill version line, show `Created by thebpandey.` as ordinary text, before the identity header. For resolved `help` or `setup` actions (including combined commands), use this single line instead, with the GitHub source beside the credit:

Created by [thebpandey](https://github.com/thebpandey) · [GitHub source](https://github.com/thebpandey/agent-team)

Show the attribution once with the wordmark. In a plain-text host, use `Created by thebpandey · GitHub source: https://github.com/thebpandey/agent-team` for help and setup. These are static links; displaying them requires no network request.

The filled letters use the Unicode full-block character. If the output surface cannot display it, replace each block with `#` and preserve spacing. No font package or image is needed.

Follow [compact output](output.md); do not add a message frame or border. Later progress messages omit the wordmark.

Select by the resolved help/first-setup action, not a substring in a task name or quoted example. Do not run a shell command, install fonts/tools, write a file, request agent reports or change task state just for branding. On narrow or incompatible surfaces, use the plain product name/version and creator/source credit. Do not add banners to individual subagent messages.


## Installed skill version

Immediately below an emitted wordmark, show `Agent-Team v<version>` from metadata.version in the installed SKILL.md, followed by creator/source credit. A plain version request returns the version without the banner. Use the installed version, not a guessed latest version. If metadata is absent, show `Agent-Team — version unknown (unversioned copy)`.

Displaying a version is local and does not check GitHub. Only claim that an installation is current after an explicit update/check request and a successful comparison with the official source. If remote access fails, report that freshness is unverified. Matching version numbers identify a release; local edits can still differ from it.
