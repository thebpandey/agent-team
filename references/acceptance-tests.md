# Pro end-to-end acceptance tests

An acceptance test checks that a user requirement produces its expected result. An end-to-end test follows a complete task through the connected parts of the app. This Pro procedure is not part of Agent Team Lite.

## Select the changed user flows

Map the important changed requirements to concrete user actions and observable results before implementation. Start with a small set, usually one to three main flows. Add flows only when a required behavior remains uncovered. Do not create an exhaustive test plan for obscure cases.

For an app, test from the visible control through the real service to the saved result. For a command-line tool or library, use its public entry point and inspect its real output. Check persistence only when the feature saves state. Mark a flow not applicable only with a concrete reason; an unavailable environment is blocked, not not-applicable.

Reuse the project's test tools and existing tests. Use supported browser controls for browser tasks. Follow the host's tool, login, and permission rules. Offer a missing tool through Pro setup only when needed and authorized. Do not install a separate framework merely to follow this procedure.

## Assign one owner

The developer implements and runs focused checks. The existing independent reviewer owns acceptance findings alongside the code review. A dedicated testing agent is optional for substantial work with a distinct test scope. Share evidence with the visual reviewer; do not repeat browser sessions or create a second review panel without need.

Use the platform's standard review tier for acceptance decisions: Terra high in Codex, Opus high in Claude Code. Routine agents may run predefined checks under that owner's contract. They must not replace independent judgment for the result. Requirement coverage and the integrated revision are verified before release by the host's delegated verifier route (GPT-5.6-Sol at medium effort; Opus 5 fallback in Claude Code); the orchestrator dispatches that check, reads its verdict and evidence, and records the outcome. It does not run the verification itself.

## Define each test briefly

Record these fields within the existing task or its linked test artifact:

| Field | Required detail |
| --- | --- |
| Requirement and test ID | Link the test to the original request. |
| Preconditions | Identify the app version, environment, account role, and required starting state. |
| Test data | Identify approved inputs and records owned by the test. |
| Actions | List the few steps needed to complete the user task. |
| Expected result | State the visible result and saved state, where applicable. |
| Evidence | Link the test output, screenshots, and relevant service-result checks. |
| Outcome | Pass, Fail, Blocked, or Not applicable with a reason. |

The active tracker remains the only task-status authority. Test files and reports contain evidence, not a parallel task ledger.

## Exercise and verify the result

1. Use a preview or test environment and approved test accounts where possible. Identify any external side effects before execution.
2. Perform the real user action. Confirm that the app's visible response matches the requirement.
3. When the action saves state, read the result through an authorized service or approved data access where available. Reload or reopen the app and confirm that the expected result remains.
4. Check the relevant business rule, such as a required field, correct account ownership, or a calculated value. Check ordinary affected failure paths only.
5. Record what ran, what passed, and what could not be checked. Link evidence to the actual revision, environment, account role, and test data.
6. Remove only test-owned records and resources through an approved safe cleanup action. Do not delete customer data or shared fixtures. Record cleanup that could not complete.

A success message alone does not prove that data was saved. If direct data access is unavailable, use a fresh authorized read after reload where possible. State the evidence limit. Do not bypass access controls to obtain stronger proof.

Use the real connected components for claims of end-to-end verification. Tests with simulated responses are useful, but must be labeled as simulated or partial. Do not count them as proof that the real integration works. A failed test, inaccessible service, or unrun check is never a pass.

Do not send real messages, charge a card, or alter live customer records without the required authorization. Use approved provider test modes when they fit the requirement. Preserve existing project gates.

## Repair and close

Send actionable failures to the developer through the active tracker. Recheck the failure and affected flow after repair. Reuse results only when relevant code, dependencies, configuration, and environment are unchanged. Follow the shared bounded failure procedure after two attempts without useful progress.

The delegated verifier checks the integrated revision before release; the orchestrator dispatches that check and acts on its verdict. A previous worktree pass may need a focused rerun after integration. Existing release instructions still require live checks and recording of the deployment. Final reconciliation links each requirement to its acceptance evidence and any blocked or deferred work.

Example: complete a lesson, verify the saved completion, reload the dashboard, and confirm that the same learner's progress remains correct. This checks a business outcome beyond visual appearance.
