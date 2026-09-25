# Agent-Team v9 Publication Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Publish Agent-Team 9.0.0 as independently downloadable Linux, Windows, and macOS skill bundles, with truthful public instructions.

**Architecture:** Keep `v9/agent-team/` as the one skill source. A small CI-only Python packager copies an exact allowlist into one OS-specific ZIP and writes its SHA-256 sidecar. One manually selected GitHub Actions platform run produces one asset without waiting for the other platforms; the reviewed tag and GitHub Release collect passing assets.

**Tech Stack:** Markdown skill, POSIX shell, PowerShell, Python standard library for release packaging/tests only, GitHub Actions, GitHub Pages from `main:/`.

**Spec:** `docs/superpowers/specs/2026-09-25-v9-publication-design.md`

## Global Constraints

- Version: Agent-Team `9.0.0`; archive suffixes: `linux-any`, `windows-any`, `macos-any`. `any` means no CPU-specific executable is shipped.
- No v8 controller, hook, LeanCTX integration, project state, or other platform's installer in a v9 archive.
- Preserve existing `index.html` CSS, DOM section order, layout, typography, colors, and artwork; change content, links, and inline-SVG labels only.
- Keep Project Kickoff optional. Beads is Agent-Team's only live tracker; Git holds revisions.
- Apply Ponytail's smallest complete change; add no release-time or runtime dependency when the standard library and existing GitHub Actions suffice.
- Do not claim native Claude, Windows, or macOS worker acceptance from installer CI. The recorded Linux Codex worker/reviewer canary has narrower scope.
- Existing user changes to `.beads/interactions.jsonl` and `.gitignore` are outside this plan; do not stage, overwrite, or clean them.
- Implementers own only the files named in their task; they are not alone in the repository. Reviewer must be a different agent and return revision-bound FIX or CLEAN.

## File map and task boundaries

| File | Responsibility |
| --- | --- |
| `SKILL.md` | Repository-root discovery route to the current v9 skill, never v8 runtime authority. |
| `v9/tests/test_root_entrypoint.py` | Regression check for that route and its no-controller boundary. |
| `v9/tests/package_release.py` | CI-only, exact-file, platform-specific ZIP and SHA-256 sidecar builder. |
| `v9/tests/test_package_release.py` | Three-platform archive member/checksum regression. |
| `.github/workflows/v9-release.yml` | Independently dispatched Linux, Windows, or macOS test/package/artifact run. |
| `v9/README.md` | Guide shipped inside each platform bundle. |
| `CHANGELOG.md`, `docs/releases/9.0.0-readiness.md` | Version history and exact evidence/limits. |
| `README.md`, `index.html` | Public first-use instructions and content-only Pages flowchart update. |

### Task 1: Make repository-root discovery route to v9 (`atv-uns.13`)

**Files:** Modify `SKILL.md`; create `v9/tests/test_root_entrypoint.py`.

**Interfaces:** Root `SKILL.md` is a repository entrypoint; the installed skill remains `v9/agent-team/SKILL.md` and resolves its own relative references. No new command or runtime API.

- [ ] **Step 1: Add a failing source-contract check.** In `v9/tests/test_root_entrypoint.py`, assert that root frontmatter contains `version: "9.0.0"`, root text directs readers to `v9/agent-team/SKILL.md`, root text does not contain `agent-teamctl` or `LeanCTX`, and the packaged skill frontmatter also contains `version: "9.0.0"`.

```python
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]

class RootEntrypointTest(unittest.TestCase):
    def test_root_routes_to_packaged_v9_without_v8_runtime(self):
        root = (ROOT / "SKILL.md").read_text()
        packaged = (ROOT / "v9/agent-team/SKILL.md").read_text()
        self.assertIn('version: "9.0.0"', root)
        self.assertIn("v9/agent-team/SKILL.md", root)
        self.assertNotIn("agent-teamctl", root)
        self.assertNotIn("LeanCTX", root)
        self.assertIn('version: "9.0.0"', packaged)
```

- [ ] **Step 2: Confirm it fails on the current v8 root.** Run `python3 -m unittest discover -s v9/tests -p test_root_entrypoint.py -v`; expect the version assertion to fail.
- [ ] **Step 3: Replace root `SKILL.md` with a short v9 pointer, not a copy of the full skill.** Keep valid `name`, `description`, and `metadata.version` frontmatter. Explain that release bundles install `v9/agent-team/`, instruct readers of the repository checkout to read that file and its relative references, and label `vnext/` and old root references as historical. Do not include active v8 commands.

```markdown
---
name: agent-team
metadata:
  version: "9.0.0"
description: Coordinate bounded Beads work in an active Codex or Claude session with independent review.
---

# Agent-Team v9 repository entrypoint

The installable skill is [v9/agent-team/SKILL.md](v9/agent-team/SKILL.md).
Read that file and resolve its relative references from `v9/agent-team/`.
Install the platform bundle from the v9 release; do not install the repository root as the skill.
Older `vnext/` and root `references/` material documents historical releases only.
```

- [ ] **Step 4: Re-run the focused test and `git diff --check`; expect PASS and no whitespace errors.** Commit only these two files. Have an independent reviewer check root discovery versus packaged v9 behavior and return CLEAN before integration.

### Task 2: Build independently checked platform bundles (`atv-uns.14`)

**Files:** Create `v9/tests/package_release.py`, `v9/tests/test_package_release.py`; modify `.github/workflows/v9-release.yml`, `v9/README.md`, `CHANGELOG.md`.

**Interfaces:** `python v9/tests/package_release.py linux release-candidate/linux` writes `agent-team-skill-9.0.0-linux-any.zip` and `agent-team-skill-9.0.0-linux-any.zip.sha256`; substituting `windows` or `macos` changes those two filenames accordingly. It takes no project or host-home path and has no runtime role after packaging.

- [ ] **Step 1: Test exact members for each platform.** Add one table-driven `unittest` that invokes the packager into a temporary directory for `linux`, `windows`, and `macos`; checks the eight archive files below; checks that only the matching installer is present; and recomputes the sidecar hash.

```python
import hashlib
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from zipfile import ZipFile

ROOT = Path(__file__).resolve().parents[2]
COMMON = {
    "agent-team/SKILL.md", "agent-team/assets/dashboard.html",
    "agent-team/references/HOSTS.md", "agent-team/references/STATE.md",
    "agent-team/references/WORKER_RULES.md", "README.md", "CUTOVER.md",
}
INSTALLER = {"linux": "install.sh", "windows": "install.ps1", "macos": "install.sh"}

class PackageReleaseTest(unittest.TestCase):
    def test_exact_platform_archives(self):
        for platform, installer in INSTALLER.items():
            with self.subTest(platform=platform), tempfile.TemporaryDirectory() as output:
                subprocess.run([sys.executable, str(ROOT / "v9/tests/package_release.py"),
                                platform, output], check=True)
                archive = Path(output) / f"agent-team-skill-9.0.0-{platform}-any.zip"
                with ZipFile(archive) as bundle:
                    self.assertEqual(set(bundle.namelist()), COMMON | {installer})
                    self.assertIsNone(bundle.testzip())
                    for name in bundle.namelist():
                        self.assertEqual(bundle.read(name), (ROOT / "v9" / name).read_bytes())
                digest, name = Path(str(archive) + ".sha256").read_text().split()
                self.assertEqual(name, archive.name)
                self.assertEqual(digest, hashlib.sha256(archive.read_bytes()).hexdigest())
```

- [ ] **Step 2: Run `python3 -m unittest discover -s v9/tests -p test_package_release.py -v`; expect failure because the packager is absent.**
- [ ] **Step 3: Implement the minimum standard-library packager.** Use `argparse`, `Path`, `zipfile.ZipFile`, and `hashlib.sha256`. Reject any source symlink and any file under `v9/agent-team/` outside `COMMON`. Sort member names before writing. Reopen the ZIP and verify its member set and CRC. Write the archive filename with `.sha256` appended as one `hash  filename` line. Do not include the packager or tests in the archive.

```python
import argparse
from hashlib import sha256
from pathlib import Path
from zipfile import ZIP_DEFLATED, ZipFile

ROOT = Path(__file__).resolve().parents[1]
SKILL_FILES = {
    "agent-team/SKILL.md", "agent-team/assets/dashboard.html",
    "agent-team/references/HOSTS.md", "agent-team/references/STATE.md",
    "agent-team/references/WORKER_RULES.md",
}
INSTALLER = {"linux": "install.sh", "windows": "install.ps1", "macos": "install.sh"}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("platform", choices=sorted(INSTALLER))
    parser.add_argument("output_dir", type=Path)
    args = parser.parse_args()
    tree = ROOT / "agent-team"
    entries = list(tree.rglob("*"))
    if any(path.is_symlink() for path in entries):
        raise SystemExit("skill tree contains a symlink")
    actual = {path.relative_to(ROOT).as_posix() for path in entries if path.is_file()}
    if actual != SKILL_FILES:
        raise SystemExit("skill tree differs from exact package allowlist")
    names = sorted(SKILL_FILES | {"README.md", "CUTOVER.md", INSTALLER[args.platform]})
    for name in names:
        source = ROOT / name
        if not source.is_file() or source.is_symlink():
            raise SystemExit(f"unsafe or missing package file: {name}")
    args.output_dir.mkdir(parents=True, exist_ok=True)
    archive = args.output_dir / f"agent-team-skill-9.0.0-{args.platform}-any.zip"
    with ZipFile(archive, "w", ZIP_DEFLATED) as bundle:
        for name in names:
            bundle.write(ROOT / name, arcname=name)
    with ZipFile(archive) as bundle:
        if set(bundle.namelist()) != set(names) or bundle.testzip() is not None:
            raise SystemExit("archive member or CRC mismatch")
    Path(str(archive) + ".sha256").write_text(
        f"{sha256(archive.read_bytes()).hexdigest()}  {archive.name}\n")

if __name__ == "__main__":
    main()
```

- [ ] **Step 4: Replace the single universal candidate job in `v9-release.yml` with three `workflow_dispatch`-selected jobs.** Input `platform` is a required choice: `linux`, `windows`, or `macos`. Use job conditions `inputs.platform == 'linux'`, `inputs.platform == 'windows'`, and `inputs.platform == 'macos'`; use matching runners `ubuntu-latest`, `windows-latest`, and `macos-latest`. No job `needs` another platform. Keep `contents: read`, run the matching installer test and packager, and upload only its ZIP and `.sha256` as `v9-linux-${{ github.sha }}`, `v9-windows-${{ github.sha }}`, or `v9-macos-${{ github.sha }}`. Keep the pinned Beads fixture evidence in the readiness ledger rather than rerunning it as a release gate.

```yaml
on:
  workflow_dispatch:
    inputs:
      platform:
        description: Platform to package
        required: true
        type: choice
        options: [linux, windows, macos]
permissions:
  contents: read
```

- [ ] **Step 5: Update the bundled `v9/README.md` to choose the exact archive and sidecar, verify the hash, extract, and run its one installer.** Add `## [9.0.0] - 2026-09-25` to `CHANGELOG.md` describing the skill-first replacement and its evidence limits. Run the packager test, Unix installer test on Linux, `git diff --check`, and the matching CI runs later on their actual OSes. Have a non-author reviewer inspect the archive allowlist, workflow independence, version names, and installer commands; remediate until CLEAN.

### Task 3: Publish reviewed bytes and record actual evidence (`atv-uns.10`)

**Files:** Modify `docs/releases/9.0.0-readiness.md`; Git tag/release and their assets are the external output. Root orchestrator performs publication after CLEAN, not the implementer.

**Interfaces:** The three CI artifacts have the same version/tag and the same five-file skill source, but distinct platform installers and SHA sidecars. The GitHub release tag is `v9.0.0` at the reviewed main-branch revision.

- [ ] **Step 1: Integrate CLEAN Tasks 1–2, check `git status --short`, and push the reviewed revision to `origin/main`.** Stage only plan-owned files; preserve `.beads/interactions.jsonl` and `.gitignore`.
- [ ] **Step 2: Dispatch one `v9-release.yml` run per platform with `gh workflow run v9-release.yml --repo thebpandey/agent-team --ref main -f platform=linux` (then `windows`, then `macos`).** Runs are separate, so publish Linux after its own PASS even if the other two are pending. Record each run URL, revision, status, and artifact ID. A failed lane has no asset.
- [ ] **Step 3: Download each passing artifact and verify its `.sha256`, ZIP members, and disposable installation on that platform.** On Linux/macOS use `sha256sum -c` or `shasum -a 256 -c` plus `unzip -Z1`; on Windows use `Get-FileHash`, `Expand-Archive`, and `v9/tests/install_windows.ps1`. Require the ZIP's five skill files to hash-match the reviewed source. Check one platform's downloaded asset once; do not repeat equivalent old v8 suites.
- [ ] **Step 4: Create the annotated `v9.0.0` tag at the reviewed commit, publish the first passing platform asset and checksum with `gh release create`, then attach later passing assets using `gh release upload` to the same tag.** Release notes must say exactly which native-host canaries passed and which remain unverified. Never replace bytes under an existing asset name; a correction gets a new version or an explicitly documented distinct filename.
- [ ] **Step 5: Update `docs/releases/9.0.0-readiness.md` with the exact commit, CI run URLs, downloaded asset hashes, package/member/install results, and native-host limits.** Commit and push that evidence record separately after publication.

### Task 4: Update public words and existing diagram, then verify Pages

**Files:** Modify `README.md`, `index.html` only. Root orchestrator owns final copy integration; a Luna worker may make content-only edits and a separate reviewer checks factual links.

**Interfaces:** `README.md` and the GitHub Pages root `index.html` link to the actual `v9.0.0` release asset(s) and optional Project Kickoff release. The site's existing `<style>` block and `assets/site/agent-team-orchestration.webp` are unchanged.

- [ ] **Step 1: Replace the candidate and v8-primary wording in `README.md`.** Lead with one sentence about an active-session Beads/Git skill, link the actual per-platform downloads/checksums, then show `install.sh codex|claude|both` or `install.ps1 -TargetHost codex|claude|both` and first `status`/`start`. Say Project Kickoff is optional and the dashboard is a local snapshot. Keep v8 under a short Historical releases section.
- [ ] **Step 2: Edit only content nodes/attributes in `index.html`:** title/meta/versions, hero text, release links, optional-Kickoff callout, install cards, footer, SVG `<title>`/`<desc>`/text labels. Keep the six SVG positions and arrow geometry. Suggested flow labels: `Beads`, `Choose`, `Build`, `Review`, `Integrate`, `Continue`; the sublabels explain setup approval, ready work, separate worktrees, review/repair, accepted changes, and pause/resume. Do not change CSS, DOM section order, artwork, or layout.
- [ ] **Step 3: Before commit, compare the current `<style>...</style>` byte-for-byte with `git show HEAD:index.html` and confirm the artwork hash is unchanged.** Parse the page read-only with `python3 -c 'from html.parser import HTMLParser; from pathlib import Path; HTMLParser().feed(Path("index.html").read_text())'`. Check all new release URLs against actual uploaded assets.
- [ ] **Step 4: Have an independent content reviewer verify simple wording, optionality, no host-overclaim, correct links, and content-only design preservation.** Integrate CLEAN, push `main`, then request both the GitHub README and `https://thebpandey.github.io/agent-team/` once and confirm version, links, and SVG accessible description. Close the Beads release tasks only with exact evidence.
