# Command interpretation scenarios

Use a fresh agent with the current SKILL.md and linked references. Ask it to choose its next actions from the supplied state without executing a project, changing files, or deploying. These checks test instruction interpretation, not actual host concurrency, Git integration, or provider behavior. Model/host behavior must also be verified in the target environment before claiming it works there.

Assume no settings unless stated, safe independent top-level tasks, and established release authority/target unless the scenario says otherwise. Evaluate the behavior, not exact prose.

| ID | Input and state | Required decision |
| --- | --- | --- |
| R1 | `start`; five ready tasks | Admit one; no refill; integrate then ask before deployment. |
| R2 | `start 3`; five ready tasks | Admit three; no fourth replacement; report their integration and ask before deployment. |
| R3 | `start 3`; only two safe independent tasks | Admit two, report reduced count, freeze that set. Do not invent a third task. |
| R4 | `start 3 continuous`; A integrated, B awaits preview, C develops | Admit one replacement; B still occupies its slot. Do not wait for deployment. |
| R5 | `start 6`; two existing standalone unfinished teams, no active scheduling run | At most four new teams if host capacity permits; keep existing ownership. `start 7`, `start 0`, and `start 2.5` make no changes. |
| R6 | `start 3 continuous auto-deploy`; eight tasks | Up to three occupied teams, refill after integration; serial batches of 3, 3, and final 2. Count tasks, not commits/subtasks. |
| R7 | `start 3 auto-deploy 1` | Fixed admitted set of three; each successfully integrated task is a separate batch. |
| R8 | `auto-deploy 3`; active run, two eligible tasks, all remaining work blocked | Enable for this run, release final two under gates, report blocked state; start no teams, change no defaults. |
| R9 | Standalone `auto-deploy`; active run | Batch size one, not the saved/current team limit; preserve team count and continuous mode. |
| R10 | `start`; saved auto-deploy=true, batch=2 | Display wordmark; notify saved preference and ask keep/disable before claims. No response means no start/deployment. |
| R11 | Saved team limit=2, continuous=true, auto-deploy=true, batch=5; `start 3 no-continuous auto-deploy` | Effective count=3, continuous=false, auto-deploy=true, batch=3; no inherited-setting prompt, saved values unchanged. |
| R12 | Same settings; `start billing no-auto-deploy` | One named feature only, no continuous expansion or deployment; defaults unchanged. |
| R13 | `settings` during an active run | Display wordmark and defaults; write only explicit saved choices under project ownership, preserve receipt; active run unchanged. |
| R14 | `pause all` between tasks, continuous run has an undeployed remainder | Hold admission and release even with zero teams. No final batch. Bare resume offers run/All; project resume restores choices without repeating confirmation. |
| R15 | Named resume while project-wide pause remains | Resume only named team; no project refill/integration/release until project hold is cleared. |
| R16 | Deployment reports failure; large ready batch and deadline pressure | Hold automatic release, inspect provider state, recover under existing authority; no unchanged retry. Continue only unaffected development. |
| R17 | Provider succeeded before interruption but tracker write failed | Inspect actual evidence, repair tracking/live verification as needed; no duplicate deployment or double counting. |
| R18 | Batch size three; main now contains four undeployed tasks | Deploy exact boundary for oldest three after delta checks; never silently deploy four. If provider only deploys latest, coordinate a supported boundary or report limitation. |
| R19 | `status all` while slots are free and batch threshold is met | Wordmark and recorded report only; no refill, release, probes, or writes. Show slot/batch counts separately from task percentage. |
| R20 | `help` while running | List all supported commands and examples; no setup, settings write, banner triggered by quoted examples, or interruption. |
| R21 | `auto-agent start 3 continuous auto-deploy` | Same resolved behavior as Agent-Team start; wordmark once, no extra banner for refill/batches, no shell command. |
| R22 | `start 3 continuous with-preview auto-deploy`; one unapproved task | Separate version-specific approvals for every admitted/replacement task; unapproved task retains slot and cannot enter batch. |
| R23 | `start 3 no-auto-deploy`; remote-main push triggers production | Honor existing gated integration process or report conflict before push; do not deploy or change pipeline configuration silently. |
| R24 | Restart with changed saved settings and a prior deployment-failure hold | Restore recorded run choices and hold; changed defaults or resume cannot clear it. |
| R25 | `auto-deploy off`; no active run | Report already off; create no run/settings/task record. |
| R26 | `start 3`; another scheduling run is active | Identify existing run; no second scheduler or silent setting change. |
| R27 | Deployment failed; cause unresolved; deadline pressure to retry or keep all teams busy | Hold releases and inspect evidence; continue only work whose independence is established. No unchanged retry. |
| R28 | Individual team handoff and a project-wide status report | Each message has exact top/bottom borders and actual identity; project report uses Project Orchestrator. Wordmark only once for status, none in team handoff; native tool/widget payloads unchanged. |
| R29 | Continuous run stopped as Blocked with no eligible team; user invokes bare resume after access is restored | Offer the identified Blocked run and All; wait for selection, recheck ownership/blockers, then continue recorded run choices. |

Baseline at `a08e173`: fresh-agent inspection reported “unsupported syntax” for `start 3 continuous auto-deploy`, “No automatic slot refill” for R4, no batch/remainder rule for R8, and no saved-settings schema or help/settings actions. Re-run against the revised references and retain actual results with the maintenance handoff; this table alone is not a passing test.
