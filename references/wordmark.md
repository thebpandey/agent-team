# AGENT-TEAM wordmark

For user-invoked `help`, `start`, `resume`, `settings`, `setup`, and `status`, print this solid block-letter wordmark once before the command's summary, picker, settings confirmation, or result. Use a fenced plain-text block so spacing stays intact:

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

Place it inside the first [message frame](output.md) for the invocation, before the identity header and body. Later progress messages retain borders but do not repeat the wordmark.

Select by the resolved action, not a substring in a task name, quoted example, or help text. Combined starts show it once; internal continuous refills, each team reply, and each batch do not repeat it. Show it for read-only status, but do not run a shell command, install fonts/tools, write a file, request agent reports, or change task state to display it. During active work, the banner and short status response leave that work running. A help invocation shows the wordmark once; quoted command examples do not trigger additional banners. Do not add banners to individual subagent messages.


## Installed skill version

Immediately below the wordmark's closing code fence, show `Agent-Team v<version>` on its own line, using `metadata.version` from the installed `SKILL.md` that supplied these instructions. Put the creator credit and any GitHub source link on the next line. This applies to every wordmark command, including help, setup, and status. Use the installed version, not the working project's version or a guessed latest version. If metadata is absent, show `Agent-Team — version unknown (unversioned copy)`.

Displaying a version is local and does not check GitHub. Only claim that an installation is current after an explicit update/check request and a successful comparison with the official source. If remote access fails, report that freshness is unverified. Matching version numbers identify a release; local edits can still differ from it.
