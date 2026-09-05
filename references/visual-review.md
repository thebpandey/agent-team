# Pro visual browser review

This feature belongs to Pro only. Do not include it, its agent template, or its browser dependencies in Agent Team Lite.

A visual review checks the actual appearance and use of an app. A “taste test” checks whether its design is clear, consistent, and appropriate for the intended users. Compare the result with the agreed design guide. Personal preference alone is not a defect.

## Assign one review owner

For changes that affect visible UI or user interaction, assign this procedure to the existing independent reviewer. For substantial UI work, the orchestrator may create one dedicated visual tester with a separate, bounded task. It owns the visual findings; the code reviewer handles the remaining code review. Do not repeat the same review or create a panel.

Use `gpt-5.6-terra` with `high` effort in Codex. Use `claude-opus-5` with `high` effort in Claude Code. The model must be able to inspect images. Do not use Luna, Sonnet, or Haiku for this Pro visual judgment role. Follow the selected host's actual model controls and access limits.

Give the reviewer the app URL, source revision, changed screens, main user flow, agreed design guide, and test-data limits. Also supply the task IDs, evidence folder, tracker mode, and assigned CONTEXT.md path. Reviewers report findings; the assigned developer repairs product code.

## Select a supported browser tool

Use the browser controls required by the host. When the host permits a choice, reuse a suitable installed tool. Playwright or Agent Browser can control the browser and capture screenshots. They are automation tools, not substitutes for the AI's visual inspection.

| Tool | What it does | Selection rule |
| --- | --- | --- |
| [Playwright](https://playwright.dev/docs/screenshots) | Opens pages, operates controls, and saves screenshots. | Reuse it when installed and supported by the project and host. |
| [Agent Browser](https://agent-browser.dev/) | Gives an AI agent commands to operate a browser and capture its screens. | Use it when available and supported by the host. |

Use an engine that actually renders the screen and supports screenshots. Do not treat a text-only page snapshot as visual evidence. Do not install a second browser tool when the first meets the need. Offer a missing tool through Pro setup only when permitted; explain its purpose and installation scope before installation. Do not bypass a host tool restriction, a failed connector, or a login boundary with another controller.

Use a development or preview app with approved test data when possible. Do not expose credentials in screenshots or task records. Follow host rules for login. Do not send real messages, make purchases, or change live customer records without the required authorization.

If app access, browser control, or image inspection is unavailable, report “Visual review blocked” with the reason. Preserve completed checks. Do not claim visual acceptance or a successful release gate when the required visual review could not run.

## Review the changed experience

1. Open the actual app at the assigned version. Confirm the page is ready before inspection.
2. Capture and inspect representative desktop and mobile screenshots. Reuse valid screenshots only when the relevant code, state, and environment are unchanged.
3. Check spacing, alignment, fonts, colors, visual hierarchy, consistency, and the approved brand. Check readable text and clipped or overlapping content.
4. Complete the main changed user flow. Check relevant controls, keyboard access, and feedback. Include ordinary load, empty, error, and success states when affected.
5. Check movement only where it changed or could affect use. Respect reduced-motion settings. Avoid exhaustive device lists and obscure edge cases.
6. Record actionable findings in the existing task record. Separate broken behavior, usability or accessibility defects, and unmet design requirements from optional preferences.
7. Send findings to the developer. After repair, inspect the affected screens and flow again. Reuse unaffected evidence. Stop when the agreed criteria pass.

Save a compact report with: task ID, revision, URL/environment, screen sizes, checked flow, screenshot paths, findings, and Pass/Fail/Blocked status. The reviewer must actually inspect the images; creating screenshot files alone does not complete the review. Use a short before/after comparison when it helps explain a repair. Do not add a second task ledger.

After two repair attempts without useful progress, change the approach or escalate through the orchestrator. If the new approach cannot progress, report the blocker. Do not erase findings, reduce acceptance criteria, or enter an open-ended polish loop.

The orchestrator checks coverage and evidence before release. This review supplements [Pro acceptance tests](acceptance-tests.md); screenshots alone do not prove that data was saved correctly. Share the same browser evidence where useful. Read applicable MISTAKES.md lessons before review and report confirmed mistakes through the orchestrator. Preserve live release checks. Reuse the existing release procedure for final verification and recovery.
