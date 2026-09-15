import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, rm, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import test from "node:test";

import { readLanePacket, validateLaneBrief, validateLaneHandover } from "../hooks/lib/lane-schema.mjs";
import { ROLE_DEFINITIONS } from "../hooks/lib/dependency-profiles.mjs";

const root = path.resolve(import.meta.dirname, "..");
const temporary = [];
const files = [
  "references/LANES.md",
  "assets/templates/lanes/BRIEF.md",
  "assets/templates/lanes/NEXT-TASK.md",
  "assets/templates/lanes/HANDOVER.md",
];
const read = (relative) => readFile(path.join(root, relative), "utf8");
const replace = (source, values) => Object.entries(values)
  .reduce((result, [key, value]) => result.replaceAll(`{{${key}}}`, String(value)), source);
test.afterEach(async () => Promise.all(temporary.splice(0).map((entry) => rm(entry, { recursive: true, force: true }))));

test("lane protocol is one retrievable reference with the exact native route phases and safety boundaries", async () => {
  const source = await read("references/LANES.md");
  for (const route of ["lane-create", "lane-next", "lane-rotate", "lane-close"]) assert.match(source, new RegExp(`\\b${route}\\b`));
  for (const phase of ["prepare", "dispatch", "result", "bind"]) assert.match(source, new RegExp(`\\b${phase}\\b`));
  for (const { id } of ROLE_DEFINITIONS) assert.match(source, new RegExp(`\\b${id}\\b`));
  for (const contract of [
    /packet preparation is not dispatch proof/i,
    /unknown liveness never frees/i,
    /rotation.*never proves.*stopped/i,
    /tracker remains the only task authority/i,
    /one retained worker.*one.*worktree/i,
    /verifier.*before.*independent review/i,
    /no.*daemon.*scheduled.*after.*host turn/is,
    /cannot change.*parent model/i,
    /phase-specific preconditions/i,
    /dispatch.*lane fingerprint.*revision.*packet hash/is,
    /result.*lane fingerprint.*packet hash/is,
    /bind.*lane fingerprint.*revision.*handover hash/is,
    /writerRelease.*null.*worker.*already null/is,
    /explicit_release.*clears the worker/is,
    /reserved reviewer.*sourceAssignment.*no tracker claim/is,
    /failed review.*settles.*attempt.*retry/is,
    /passed review.*author history/is,
    /canonicalRecordMaxBytes=16777216/,
  ]) assert.match(source, contract);
  assert.doesNotMatch(source, /All native routes[^.]*tracker fingerprints/i);
  const headings = [...source.matchAll(/^##\s+(.+)$/gm)].map((match) => match[1]);
  for (const heading of ["Route contract", "Files and bounded context", "Verification and integration", "Capacity and supervision", "Host continuity and recovery"]) {
    assert.ok(headings.includes(heading), `missing retrievable section: ${heading}`);
  }
});

test("filled uppercase lane templates pass the production brief packet and handover validators", async () => {
  for (const relative of files) {
    const basename = path.basename(relative, ".md");
    assert.equal(basename, basename.toUpperCase());
  }
  const revision = "c".repeat(40);
  const briefSha256 = "b".repeat(64);
  const brief = replace(await read("assets/templates/lanes/BRIEF.md"), {
    LANE_ID: "build-a", ROLE: "developer", MODEL: "gpt-6-astra", EFFORT: "high", QUEUE: "AT-001, AT-002",
    WRITABLE_PATHS: "src/lanes/**", REQUIRED_SKILLS: `test-driven-development/SKILL.md ${"f".repeat(64)}`, GOLD_EXAMPLE: "none",
    INSTRUCTION_PATH: "AGENTS.md", INSTRUCTION_SHA256: "a".repeat(64), CONTEXT_REVISION: revision,
    EVIDENCE_DESTINATION: ".agent-team/lanes/build-a/evidence/", HANDOFF_PATH: ".agent-team/lanes/build-a/HANDOVER-001.md",
  });
  assert.equal(validateLaneBrief(brief, { maxWords: 6000 }), undefined);

  const expected = { assignmentId: "assignment-at-001-1", taskId: "AT-001", attempt: 1, revision,
    trackerFingerprint: "d".repeat(64), laneFingerprint: "e".repeat(64), briefSha256 };
  const packet = replace(await read("assets/templates/lanes/NEXT-TASK.md"), {
    ASSIGNMENT_ID: expected.assignmentId, TASK_ID: expected.taskId, ATTEMPT: expected.attempt, REVISION: revision,
    TRACKER_FINGERPRINT: expected.trackerFingerprint, LANE_FINGERPRINT: expected.laneFingerprint,
    BRIEF_SHA256: briefSha256, ACCEPTANCE: "Return the documented accepted behavior.",
    DELTA: "Implement only the assigned task against the shared brief.", DECISIONS_OR_NONE: "None.",
    REVISION_POINTERS: "Tracker AT-001; evidence .agent-team/lanes/build-a/evidence/AT-001.json",
  });
  assert.equal(readLanePacket(packet, { expected, maxWords: 400 }).taskId, "AT-001");
  assert.match(await read("assets/templates/lanes/NEXT-TASK.md"), /\{\{DECISIONS_OR_NONE\}\}/);
  assert.doesNotMatch(await read("assets/templates/lanes/NEXT-TASK.md"), /DECISION_ID|DECISION_KIND|SUPERSEDES_DECISION_ID/);

  const handover = replace(await read("assets/templates/lanes/HANDOVER.md"), {
    VOICE: "Continue directly from the accepted task boundary.", GOTCHAS: "Do not reinterpret UNKNOWN as stopped.",
    OPEN_THREADS: "AT-002 remains next in the immutable queue.", DECISIONS: "DEC-AT-001 remains active.", EXACT_REVISION: revision,
  });
  assert.equal(validateLaneHandover(handover, { expected: { laneId: "build-a", sequence: 1, revision }, maxWords: 500 }), undefined);
});

test("instantiated next-task packets retain a project-root-relative protocol pointer", async () => {
  const project = await mkdtemp(path.join(os.tmpdir(), "agent-team-packet-doc-"));
  temporary.push(project);
  const packetPath = path.join(project, ".agent-team/lanes/build-a/packets/AT-001-1.md");
  await mkdir(path.dirname(packetPath), { recursive: true });
  await mkdir(path.join(project, "references"));
  await writeFile(packetPath, await read("assets/templates/lanes/NEXT-TASK.md"));
  await writeFile(path.join(project, "references/LANES.md"), await read("references/LANES.md"));

  const packet = await readFile(packetPath, "utf8");
  const protocolPath = packet.match(/^Protocol:\s+`([^`]+)`/m)?.[1];
  assert.equal(protocolPath, "references/LANES.md");
  assert.match(await readFile(path.resolve(project, protocolPath), "utf8"), /^# Lane protocol/m);
});

test("new lane documents use resolvable local links and retain the 7.3.0 release boundary", async () => {
  const manifest = JSON.parse(await read("hooks/manifest.json"));
  assert.equal(manifest.version, "7.3.0");
  for (const relative of files) {
    const source = await read(relative);
    assert.doesNotMatch(source, /\u2014/);
    for (const match of source.matchAll(/\[[^\]]+\]\(([^)]+)\)/g)) {
      const target = match[1].split("#")[0];
      if (!target || /^[a-z]+:/i.test(target)) continue;
      await readFile(path.resolve(root, path.dirname(relative), target));
    }
  }
});
