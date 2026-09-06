# AGENT-TEAM wordmark

For user-invoked `start`, `resume`, `settings`, `setup`, and `status`, print this solid block-letter wordmark once before the command's summary, picker, settings confirmation, or result. Use a fenced plain-text block so spacing stays intact:

```text
 ███   █████  █████  ██  ██  ██████        ██████  █████   ███   ██   ██
██ ██  ██     ██     ███ ██    ██            ██    ██     ██ ██  ███ ███
██ ██  ██     ████   ██████    ██    ████    ██    ████   ██ ██  ███████
█████  ██ ██  ██     ██ ███    ██    ████    ██    ██     █████  ██ █ ██
██ ██  ██ ██  ██     ██  ██    ██            ██    ██     ██ ██  ██   ██
██ ██  █████  █████  ██  ██    ██            ██    █████  ██ ██  ██   ██
```

The filled letters use the Unicode full-block character. If the output surface cannot display it, replace each block with `#` and preserve spacing. No font package or image is needed.

Place it inside the first [message frame](output.md) for the invocation, before the identity header and body. Later progress messages retain borders but do not repeat the wordmark.

Select by the resolved action, not a substring in a task name, quoted example, or help text. Combined starts show it once; internal continuous refills, each team reply, and each batch do not repeat it. Show it for read-only status, but do not run a shell command, install fonts/tools, write a file, request agent reports, or change task state to display it. During active work, the banner and short status response leave that work running. Help may quote command examples without triggering a banner. Do not add banners to individual subagent messages.
