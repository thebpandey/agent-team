# Project Kickoff v9 Handoff Publication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish Project Kickoff 0.6.0 with an optional, Beads-first Agent-Team 9.0.0 handoff and a truthful separate download.

**Architecture:** Change the default new-handoff instructions to the already present skill-first template. Keep the old v8 template and checker pair for historical projects, but do not route new users through its controller. Verify the new pair with one disposable real-Beads adoption test, then publish one platform-neutral ZIP from Project Kickoff's own repository.

**Tech Stack:** Markdown skill/templates, Python standard-library handoff checker/tests, Beads 1.2.2 canary, Git archive, GitHub Release and GitHub Pages from `main:/`.

**Spec:** `docs/superpowers/specs/2026-09-25-v9-publication-design.md` in `/home/server/dev/skills/agent-team`.

## Global Constraints

- Project Kickoff version `0.6.0`; new handoffs target Agent-Team `9.0.0`, `agentTeam.mode: "skill-first"`, and Beads task IDs.
- Project Kickoff remains optional. Agent-Team standalone setup/start/one-off cannot require this skill or a handoff.
- `TASKS.md` is a one-time import candidate only; no parallel live tracker, `agent-teamctl`, LeanCTX, optional-aid gate, worker dispatch, or Agent-Team runtime receipt in the v9 route.
- Keep the Project Kickoff Pages CSS, DOM section order, layout, typography, colors, and artwork unchanged; update content and existing flow labels only.
- Do not mark native worker execution verified by a checker/Beads canary. Historical v8 records remain explicitly historical.
- Apply Ponytail's smallest complete change; do not add a new package manager, controller, hook, or release service.
- Workers own only named files, are not alone in either repository, and must not revert another agent's edits. Independent non-author review returns revision-bound FIX or CLEAN.

## File map and task boundaries

All paths below are relative to `/home/server/dev/skills/project-kickoff` unless identified otherwise.

| File | Responsibility |
| --- | --- |
| `SKILL.md`, `references/setup.md`, `references/handoff.md` | Current new-project v9 route, with v8 material clearly historical. |
| `assets/templates/AGENT_TEAM_SKILL_FIRST_HANDOFF.json` | Current bounded v9 handoff template. |
| `assets/templates/AGENT_TEAM_HANDOFF.json` | Unchanged historical v8 template. |
| `scripts/check_agent_team_handoff.py` | Recognize the `0.6.0/9.0.0` pair without dropping historical safety checks. |
| `tests/test_agent_team_handoff.py`, `tests/test_package_manifest.py` | Default/version and package contract regression. |
| `tests/test_agent_team_v9_handoff.py` | Disposable cross-repository Beads ID adoption canary. |
| `README.md`, `CHANGELOG.md` | Versioned install/archive instructions and history. |
| `index.html` | Content-only public Pages explanation after release. |

### Task 1: Make the v9 handoff the current new-project route

**Files:** Modify `SKILL.md`, `references/setup.md`, `references/handoff.md`, `assets/templates/AGENT_TEAM_SKILL_FIRST_HANDOFF.json`, `scripts/check_agent_team_handoff.py`, `tests/test_agent_team_handoff.py`, `tests/test_package_manifest.py`, `README.md`, `CHANGELOG.md`. Do not modify the historical v8 template.

**Interfaces:** A new handoff uses `projectKickoff.version: "0.6.0"`, `agentTeam.mode: "skill-first"`, `agentTeam.testedVersion: "9.0.0"`, `tracker.kind: "beads"`, and `plan.tasks` as existing Beads IDs. The checker continues to reject unknown pairs, invalid scope, unsafe paths, missing IDs, and external actions.

- [ ] **Step 1: Change the focused tests first.** Make `test_skill_first_handoff_is_schema_valid_but_not_runtime_qualified` build a `0.6.0/9.0.0` handoff; assert the template matches that pair and the checker still reports `schema-valid-unverified`/`runtimeVerified: false` until broader native execution is observed. Change `test_package_manifest.py` so `HANDOFF_TEMPLATE` points to the v9 template and the v9 template is no longer excluded from package files. Let the tag-comparison test skip only when the exact `v0.6.0` tag does not yet exist, then run it after tagging.

```python
HANDOFF_TEMPLATE = PACKAGE / "assets/templates/AGENT_TEAM_SKILL_FIRST_HANDOFF.json"
UNRELEASED_SOURCE_FILES = set()
# In the tag-only test, use git rev-parse --verify --quiet refs/tags/v0.6.0;
# skip if it is absent before publication, and require equality when it exists.
```

- [ ] **Step 2: Run `python3 -m unittest discover -s tests -p test_agent_team_handoff.py -v` and `python3 -m unittest discover -s tests -p test_package_manifest.py -v`; expect version/default assertions to fail before implementation.**
- [ ] **Step 3: Make the narrow version/default change.** Set `SKILL.md` metadata and v9 template producer version to `0.6.0`; set `CHECKER_VERSION = "0.6.0"`; add `("0.6.0", "9.0.0")` to `SUPPORTED_PAIRS` without deleting historical pairs or changing `RUNTIME_VERIFIED_PAIRS`. Route new handoffs through `AGENT_TEAM_SKILL_FIRST_HANDOFF.json` in `SKILL.md` and `references/setup.md`. Label v8 setup instructions historical; do not call its controller from the v9 route. Keep `references/handoff.md` aligned with Beads ID-only adoption and Markdown one-time import.

```python
CHECKER_VERSION = "0.6.0"
# Add this one tuple inside the existing SUPPORTED_PAIRS literal:
("0.6.0", "9.0.0"),
# Do not change RUNTIME_VERIFIED_PAIRS; this canary does not prove worker dispatch.
```

- [ ] **Step 4: Update `README.md` to version `0.6.0`, add the v9 template to both exact install allowlists and the archive tree, pin clone examples to `v0.6.0`, and move v8 detail out of the first-use path.** Add `## [0.6.0] - 2026-09-25` to `CHANGELOG.md` with the optional v9 handoff and evidence limit. Keep the license and optional project-hook behavior unchanged.
- [ ] **Step 5: Re-run the two focused test files and `git diff --check`; expect PASS except the explicitly skipped pre-release tag assertion.** A non-author reviewer checks that the current path is v9, v8 remains historical, no optional aid is a gate, and the checker keeps its existing safety rejections. Remediate until CLEAN.

### Task 2: Verify real Beads adoption and publish the Project Kickoff package

**Files:** Create `tests/test_agent_team_v9_handoff.py`; no product code outside Task 1. Git tag/release, ZIP, and checksum are outputs. Root orchestrator handles release after independent CLEAN.

**Interfaces:** The test uses `AGENT_TEAM_9_SOURCE=/home/server/dev/skills/agent-team` and a real Beads 1.2.2 executable. It validates a new 0.6.0 handoff, reads every selected ID from the same disposable Beads database, and confirms no task was imported or claimed. This qualifies handoff adoption only, not native worker dispatch.

**Dependency:** Agent-Team Task 3 must have published `v9.0.0` before the Project Kickoff `v0.6.0` release is announced.

- [ ] **Step 1: Write a disposable canary.** Use `tempfile.TemporaryDirectory`, `git init -b main`, `bd init --skip-hooks --skip-agents --non-interactive`, and `bd create --id fixture-a --title A --type task` plus `bd create --id fixture-b --title B --type task`. Fill the v9 template's approved fields with the fixture root, branch revision, those IDs, and empty `externalActions`. Run `scripts/check_agent_team_handoff.py --handoff` on the created `.project-kickoff/AGENT_TEAM_HANDOFF.json`. Read the Agent-Team source instruction from `$AGENT_TEAM_9_SOURCE/v9/agent-team/SKILL.md`; run its `bd show ID --json` read for each selected ID and assert each returned ID matches. Compare the Beads issue-ID set before and after; no v8 controller is invoked.

```python
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

PACKAGE = Path(__file__).resolve().parents[1]
AGENT_TEAM = Path(os.environ.get("AGENT_TEAM_9_SOURCE", ""))
BD = os.environ.get("BD_122_EXECUTABLE") or shutil.which("bd")

@unittest.skipUnless(os.environ.get("AGENT_TEAM_9_SOURCE") and BD,
                     "requires Agent-Team v9 source and Beads")
class AgentTeamV9HandoffTest(unittest.TestCase):
    def test_approved_ids_are_adopted_from_real_beads_without_import(self):
        self.assertIn("1.2.2", subprocess.check_output([BD, "--version"], text=True))
        skill = (AGENT_TEAM / "v9/agent-team/SKILL.md").read_text()
        self.assertIn("bd show ID --json", skill)
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary) / "project"
            root.mkdir()
            def run(*command):
                return subprocess.check_output(command, cwd=root, text=True)
            run("git", "init", "-q", "-b", "main")
            run("git", "config", "user.name", "Fixture")
            run("git", "config", "user.email", "fixture@example.test")
            (root / "README.md").write_text("fixture\n")
            run("git", "add", "README.md")
            run("git", "commit", "-qm", "fixture")
            revision = run("git", "rev-parse", "HEAD").strip()
            run(BD, "init", "--skip-hooks", "--skip-agents", "--non-interactive", "--prefix", "fixture")
            for issue_id in ("fixture-a", "fixture-b"):
                run(BD, "create", "--id", issue_id, "--title", issue_id, "--type", "task")
            def issue_ids():
                return {row["id"] for row in json.loads(run(BD, "list", "--json"))}
            before = issue_ids()
            handoff = json.loads((PACKAGE / "assets/templates/AGENT_TEAM_SKILL_FIRST_HANDOFF.json").read_text())
            handoff["projectKickoff"].update(version="0.6.0", approvalId="APR-001",
                                             approvedRevision=revision)
            handoff["project"].update(id="fixture", root=str(root), branch="main", revision=revision)
            handoff["tracker"] = {"kind": "beads", "executable": str(Path(BD).resolve())}
            handoff["plan"].update(scope="Adopt two tasks", acceptance=["IDs exist"],
                                   verification=["bd show fixture-a --json"], branch="main",
                                   tasks=[{"id": "fixture-a"}, {"id": "fixture-b"}])
            handoff["plan"]["authority"] = {"ownedPaths": ["README.md"], "externalActions": []}
            handoff_dir = root / ".project-kickoff"
            handoff_dir.mkdir()
            handoff_path = handoff_dir / "AGENT_TEAM_HANDOFF.json"
            handoff_path.write_text(json.dumps(handoff))
            result = json.loads(run("python3", str(PACKAGE / "scripts/check_agent_team_handoff.py"),
                                    "--handoff", str(handoff_path)))
            self.assertEqual(result["status"], "passed")
            for issue_id in ("fixture-a", "fixture-b"):
                rows = json.loads(run(BD, "show", issue_id, "--json"))
                self.assertEqual(rows[0]["id"], issue_id)
            self.assertEqual(issue_ids(), before)
```

- [ ] **Step 2: Run the canary with real Beads 1.2.2 and reviewed Agent-Team v9 source.** Use `AGENT_TEAM_9_SOURCE=/home/server/dev/skills/agent-team python3 -m unittest discover -s tests -p test_agent_team_v9_handoff.py -v`; require PASS and record the exact revisions/Beads version. Do not call this a Windows, macOS, Claude, or native worker canary.
- [ ] **Step 3: Integrate CLEAN Task 1 and canary code; run the package and handoff tests, then tag the reviewed Project Kickoff commit `v0.6.0`.** Re-run `test_package_manifest.py` after the tag so its previously skipped tag comparison actually runs and passes. Push the reviewed commit/tag only after this check.
- [ ] **Step 4: Build the single platform-neutral ZIP from the tag's exact package allowlist, create `SHA256SUMS`, verify the archive member list against that allowlist, and download/check the published asset once.** In the Project Kickoff repository, run the commands below after the `v0.6.0` tag exists. Publish both files with `gh release create v0.6.0` in `thebpandey/project-kickoff`. Its notes must say the Beads handoff was tested and native worker dispatch was not part of this canary. Do not include development tests, Pages assets, or hidden project state in the ZIP.

```sh
kickoff_release_dir=$(mktemp -d)
kickoff_archive="$kickoff_release_dir/project-kickoff-0.6.0.zip"
kickoff_files=$(sed -n "s/^kickoff_required_files='\([^']*\)'/\1/p" README.md | head -n 1)
git archive --format=zip --prefix=project-kickoff/ -o "$kickoff_archive" v0.6.0 $kickoff_files
(cd "$kickoff_release_dir" && sha256sum project-kickoff-0.6.0.zip > SHA256SUMS && sha256sum -c SHA256SUMS)
unzip -Z1 "$kickoff_archive"
```

### Task 3: Refresh Project Kickoff public content without changing its design

**Files:** Modify `README.md`, `index.html` only after the release asset exists. If Task 1 already changed README's packaged first-use text, this task changes only the final public links/status language.

**Interfaces:** The root README and `index.html` link to the actual Project Kickoff `v0.6.0` release and the separately published Agent-Team `v9.0.0` page. Neither skill is presented as a prerequisite for the other.

- [ ] **Step 1: Make the first README screen say, in simple words:** “Project Kickoff helps you decide what to build and saves a plan. You can use it alone. If you later choose Agent-Team, it can use your approved Beads task IDs without restarting the interview.” Give the real 0.6.0 download/checksum and one install path for Codex and Claude. Keep the longer reference material below.
- [ ] **Step 2: Change only content in `index.html`:** version/meta/title, current handoff wording, compatibility card, footer links, and any existing flow labels implying v8 controller or optional-tool setup. Keep the CSS `<style>` block, DOM section order, illustration, and layout byte-for-byte unchanged. State that Agent-Team is optional and Beads IDs are adopted only when the user asks.
- [ ] **Step 3: Compare the `<style>...</style>` block with `git show HEAD:index.html` before committing, confirm the artwork hash is unchanged, and verify the new download links exist.** Have a separate reviewer check factual handoff wording, current versions, optionality, and content-only preservation; remediate until CLEAN.
- [ ] **Step 4: Push the content commit and check `https://thebpandey.github.io/project-kickoff/` once for version, download link, and Agent-Team cross-link.** Record the exact release/tag and Pages verification in the handoff report.
