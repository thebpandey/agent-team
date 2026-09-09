import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';
import test from 'node:test';
import { policyFixture } from './hook-test-helpers.mjs';
import { resolveProject } from '../hooks/lib/project.mjs';
import { loadCanonicalState } from '../hooks/lib/canonical-state.mjs';

const run = promisify(execFile);

// Explicit qualification only: not part of the portable hooks-* unit suite.
// All mutations are in a new fixture repository, never this project's tracker.
test('selected real Beads supports canonical worktree reads, claims and fresh export', { timeout: 120000 }, async t => {
  const executable = process.env.AGENT_TEAM_BD;
  const output = process.env.AGENT_TEAM_QUALIFICATION_OUTPUT;
  assert.ok(executable && path.isAbsolute(executable), 'Set AGENT_TEAM_BD to the inspected absolute executable.');
  assert.ok(output && path.isAbsolute(output), 'Set AGENT_TEAM_QUALIFICATION_OUTPUT to the private evidence directory.');
  const root = await mkdtemp(path.join(os.tmpdir(), 'agent-team-real-beads-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const fixture = await policyFixture(path.join(root, 'repo'));
  const legacyTracker = await readFile(path.join(fixture.root, '.agent-team/TASKS.md'), 'utf8');
  const beadsDirectory = path.join(fixture.root, '.beads');
  const env = { ...process.env, BEADS_DIR: beadsDirectory, BD_NON_INTERACTIVE: '1',
    PATH: `${path.dirname(executable)}${path.delimiter}${process.env.PATH}` };
  const bd = args => run(executable, args, {
    cwd: fixture.root, env, encoding: 'utf8', timeout: 30000, maxBuffer: 1024 * 1024,
  });
  const version = (await bd(['version'])).stdout.trim();
  await bd(['init', '--prefix', 'atq', '--non-interactive', '--role', 'maintainer', '--skip-agents', '--skip-hooks', '--sandbox']);
  const setupFile = path.join(fixture.root, '.agent-team/setup.json');
  const setup = JSON.parse(await readFile(setupFile, 'utf8'));
  setup.tracker = { kind: 'beads', executable };
  await writeFile(setupFile, JSON.stringify(setup));
  const created = JSON.parse((await bd(['create', '--id', 'atq-101', '--title', 'Native qualification task',
    '--acceptance', 'Preserve one owner and canonical worktree reads.', '--json', '--sandbox'])).stdout);
  assert.equal(created.id, 'atq-101');
  await bd(['update', 'atq-101', '--claim', '--actor', 'TEAM-001', '--json', '--sandbox']);
  await bd(['update', 'atq-101', '--claim', '--actor', 'TEAM-001', '--json', '--sandbox']);
  await assert.rejects(bd(['update', 'atq-101', '--claim', '--actor', 'TEAM-002', '--json', '--sandbox']),
    /claim|assign|owned/i);

  // Exercise the real production reader, not a JSON fixture or fake bd executable.
  const canonicalProject = await resolveProject(fixture.root);
  const linkedProject = await resolveProject(fixture.feature);
  assert.equal(linkedProject.root, fixture.root);
  assert.equal(linkedProject.tracker.id, canonicalProject.tracker.id);
  const canonical = await loadCanonicalState(linkedProject);
  assert.equal(canonical.tracker.status, 'current', JSON.stringify(canonical.tracker));
  assert.equal(canonical.tasks.length, 1);
  assert.equal(canonical.tasks[0].id, 'atq-101');
  assert.equal(canonical.tasks[0].owner, 'TEAM-001');
  assert.equal(canonical.tasks[0].status, 'in_progress');
  const exportPath = path.join(beadsDirectory, 'qualification.jsonl');
  await bd(['export', '-o', exportPath, '--sandbox']);
  const exported = (await readFile(exportPath, 'utf8')).trim().split('\n').map(line => JSON.parse(line));
  assert.ok(exported.some(row => row.id === 'atq-101' && row.assignee === 'TEAM-001'));
  assert.equal(await readFile(path.join(fixture.root, '.agent-team/TASKS.md'), 'utf8'),
    legacyTracker);
  await mkdir(output, { recursive: true, mode: 0o700 });
  await writeFile(path.join(output, 'beads-native.json'), JSON.stringify({
    kind: 'real-beads-qualification', status: 'passed', executable, version,
    backend: 'default embedded Dolt', cases: ['init without generated agent files/hooks',
      'same-owner idempotent claim', 'different-owner claim refused', 'canonical linked-worktree read', 'fresh JSONL export'],
    limitation: 'This tests sequential claims on embedded storage, not external-server multi-writer behavior, native host trust, or bv.',
  }, null, 2) + '\n', { mode: 0o600 });
});
