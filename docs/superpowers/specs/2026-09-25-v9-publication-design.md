# Agent-Team v9 and Project Kickoff publication design

**Status:** Approved in conversation on 2026-09-25. This document defines the publication work, not new Agent-Team runtime behavior.

## Goal and boundaries

Publish Agent-Team 9.0.0 and Project Kickoff 0.6.0 as separate, downloadable skills. Agent-Team remains useful without Project Kickoff. The existing [skill-first design](2026-09-24-agent-team-skill-first-design.md) remains the runtime contract; this design replaces only its platform-neutral packaging and public-release plan.

Keep both GitHub Pages sites' existing visual design. Change only text, links, accessible diagram descriptions, and labels inside existing flowcharts. Do not change CSS, layout, typography, colors, or artwork. Write the READMEs and page copy in simple, task-oriented language.

## Agent-Team release

Use one shared `v9/agent-team/` source tree and version 9.0.0. Make the repository-root `SKILL.md` point to v9, with no v8 controller instructions in its active entrypoint. Keep v8 source and release records as historical material, not part of the v9 package.

Publish three self-contained archives, each with the shared skill tree, the matching installer, and the v9 install/cutover guides:

| Asset | Installer | Platform check |
| --- | --- | --- |
| `agent-team-skill-9.0.0-linux-any.zip` | `install.sh` | Linux installer and package tests |
| `agent-team-skill-9.0.0-windows-any.zip` | `install.ps1` | Windows PowerShell installer and package tests |
| `agent-team-skill-9.0.0-macos-any.zip` | `install.sh` | macOS installer and package tests |

`any` is the truthful architecture label: the archive contains no CPU-specific executable. Each archive has its own SHA-256 sidecar and an exact file allowlist. Packaging must exclude controller binaries, hooks, project state, hidden files, and the other platform's installer. All three archives must copy the same reviewed five-file skill tree from one release commit. Each platform's test/package lane runs independently; Linux publication does not wait for Windows or macOS checks. The GitHub Release is created from the reviewed tag with the first passing platform asset, and later passing assets attach to that same tag. Failed platform checks do not gain a download link or a verified claim.

The release notes distinguish installer/package evidence from native Codex or Claude worker evidence. The Linux Codex worker/reviewer canary is observed. Claude native worker dispatch and Windows/macOS native-host behavior remain unverified unless new canaries actually pass. An installer test must not be described as host acceptance.

## Project Kickoff release

Publish version 0.6.0 from its separate repository. For new handoffs, its default route targets Agent-Team 9.0.0 and Beads. The handoff carries selected Beads task IDs; Agent-Team validates them from Beads only after the user explicitly asks to adopt the handoff. `TASKS.md` can be imported once with approval, never used as a parallel live tracker. Project Kickoff does not invoke Agent-Team workers, require Agent-Team installation, or require optional aids.

Include the existing v9 handoff template in the package, update its package allowlist and compatibility/version checks, and remove v8 directions from the active user path. Retain any v8 guidance only as clearly labeled historical material. One platform-neutral Project Kickoff ZIP plus checksum is sufficient because it has no platform-specific installer or executable. Before claiming compatibility, run one disposable real-Beads cross-repository adoption canary against the reviewed Agent-Team v9 source. The two releases remain independently installable.

## Public explanation and order

The first screen of each README and Pages site should answer: what the skill does, whether the other skill is needed, which download to choose, how to install it, and what to type first. Agent-Team's existing inline SVG workflow keeps its geometry and style; its labels explain Beads selection, separate worktrees, independent review/repair, integration, and continuing or pausing. Existing text-free artwork stays unchanged. Project Kickoff's existing flowchart gets content corrections only.

Update guides that ship inside each archive before tagging. Review changed source and package boundaries independently before publication. Verify each claimed package by downloading its uploaded asset, checking its checksum and file allowlist, and installing it into disposable host homes. Run the one cross-repository handoff canary before Project Kickoff publication. Tag and release the reviewed commits, then update the repository-root READMEs and Pages copy/links so they describe only assets and behavior that are public. Confirm the published download links and both live Pages sites once; avoid repeating equivalent verification loops.

## Failure and recovery

A failed platform lane leaves that asset unpublished; other passing platforms can ship. A failed Project Kickoff handoff canary leaves 0.6.0 unpublished and does not block standalone Agent-Team v9. If a release asset or Pages link fails verification, correct that exact asset or link without changing the already reviewed skill source. Do not silently replace an asset under the same name with different bytes; record checksums and revisions in release notes.
