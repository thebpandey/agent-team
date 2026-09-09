import assert from 'node:assert/strict';
import { execFile } from 'node:child_process';
import { constants } from 'node:fs';
import test from 'node:test';
import { mkdtemp, open, readFile, rm, writeFile } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import { promisify } from 'node:util';

const load = () => import('../hooks/lib/usage.mjs');

test('all-role usage counts cached input as a subset, deduplicates receipts and includes repairs', async () => {
  const { summarizeUsage } = await load();
  const records = [
    { id: 'a', agentId: 'root', role: 'orchestrator', inputTokens: 100, cachedInputTokens: 80, outputTokens: 20 },
    { id: 'b', agentId: 'dev', role: 'developer', inputTokens: 200, cachedInputTokens: 100, outputTokens: 30, repairAttempts: 1 },
    { id: 'c', agentId: 'review', role: 'reviewer', inputTokens: 50, cachedInputTokens: 0, outputTokens: 10 },
  ];
  const result = summarizeUsage([...records, records[1]], { expectedAgentIds: ['root', 'dev', 'review'] });
  assert.equal(result.status, 'observed');
  assert.equal(result.inputTokens, 350);
  assert.equal(result.cachedInputTokens, 180);
  assert.equal(result.outputTokens, 60);
  assert.equal(result.totalTokens, 410);
  assert.equal(result.repairAttempts, 1);
  assert.equal(result.duplicateRecords, 1);
  assert.deepEqual(result.byRole.developer, { records: 1, knownTokens: 230 });
});

test('missing worker usage and absent cached figures remain explicitly partial or unknown', async () => {
  const { summarizeUsage } = await load();
  const result = summarizeUsage([
    { id: 'a', agentId: 'dev', role: 'developer', inputTokens: 10, outputTokens: 2 },
    { id: 'b', agentId: 'review', role: 'reviewer', outputTokens: 3 },
  ], { expectedAgentIds: ['root', 'dev', 'review'] });
  assert.equal(result.status, 'partial');
  assert.equal(result.totalTokens, null);
  assert.equal(result.cachedInputTokens, null);
  assert.equal(result.knownTokens, 15);
  assert.deepEqual(result.missingAgents, ['root']);
  const empty = summarizeUsage([]);
  assert.equal(empty.status, 'unknown');
  assert.equal(empty.totalTokens, null);
  assert.equal(empty.cost, null);
});

test('estimates and malformed/conflicting receipts cannot become observed totals', async () => {
  const { summarizeUsage } = await load();
  const result = summarizeUsage([
    { id: 'a', agentId: 'dev', inputTokens: 10, outputTokens: 2, source: 'estimate' },
    { id: 'b', agentId: 'review', inputTokens: -1, outputTokens: 4 },
    { id: 'c', agentId: 'root', inputTokens: 10, cachedInputTokens: 11, outputTokens: 2 },
  ]);
  assert.equal(result.status, 'partial');
  assert.equal(result.totalTokens, null);
  assert.equal(result.estimatedRecords, 1);
  assert.equal(result.invalidRecords, 2);
  const conflicting = summarizeUsage([
    { id: 'same', agentId: 'dev', inputTokens: 10, outputTokens: 2 },
    { id: 'same', agentId: 'dev', inputTokens: 20, outputTokens: 2 },
  ]);
  assert.equal(conflicting.status, 'partial');
  assert.equal(conflicting.conflictingRecords, 1);
  assert.equal(conflicting.totalTokens, null);
});

test('soft budget suggests strategy changes while explicit hard budget requests a safe checkpoint', async () => {
  const { assessBudget } = await load();
  const usage = { status: 'observed', totalTokens: 100, knownTokens: 100 };
  assert.deepEqual(assessBudget(usage), { action: 'continue', reason: 'no_budget', waiveChecks: false });
  assert.deepEqual(assessBudget(usage, { softTokens: 90 }), { action: 'adjust_strategy', reason: 'soft_budget', waiveChecks: false });
  assert.deepEqual(assessBudget(usage, { hardTokens: 100 }), { action: 'checkpoint', reason: 'hard_budget', waiveChecks: false });
  assert.equal(assessBudget({ status: 'partial', totalTokens: null, knownTokens: 20 }, { hardTokens: 100 }).reason, 'usage_incomplete');
  assert.throws(() => assessBudget(usage, { hardTokens: -1 }), /positive/);
  assert.throws(() => assessBudget(usage, { softTokens: '90' }), /positive/);
});

test('an unknown roster cannot imply all-agent coverage and overflowing totals stay unknown', async () => {
  const { summarizeUsage } = await load();
  const unknownRoster = summarizeUsage([{ id: 'a', agentId: 'dev', inputTokens: 10, outputTokens: 2 }]);
  assert.equal(unknownRoster.status, 'partial');
  assert.equal(unknownRoster.totalTokens, null);
  assert.equal(unknownRoster.coverage.rosterKnown, false);
  const overflow = summarizeUsage([
    { id: 'a', agentId: 'dev', inputTokens: Number.MAX_SAFE_INTEGER, outputTokens: 2 },
    { id: 'b', agentId: 'dev', inputTokens: 10, outputTokens: 3 },
  ], { expectedAgentIds: ['dev'] });
  assert.equal(overflow.totalTokens, null);
  assert.equal(overflow.knownTokens, null);
  assert.equal(overflow.inputTokens, null);
  assert.equal(overflow.byRole.unknown.knownTokens, null);
  assert.equal(overflow.status, 'partial');
});

test('usage report reads existing receipts without writes, copying raw content, or inferring missing data', async t => {
  const { readUsageReport } = await load();
  const root = await mkdtemp(path.join(os.tmpdir(), 'agent-team-usage-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const statePath = path.join(root, 'state.json');
  const state = JSON.stringify({ schemaVersion: 1, unrelated: 'preserved', usage: {
    expectedAgentIds: ['root', 'dev'],
    records: [
      { id: 'a', agentId: 'root', role: 'orchestrator', inputTokens: 10, outputTokens: 2, prompt: 'PRIVATE PROMPT' },
      { id: 'b', agentId: 'dev', role: 'developer', inputTokens: 20, outputTokens: 4, raw: 'PRIVATE TOOL OUTPUT' },
    ],
  } });
  await writeFile(statePath, state);
  const project = { active: true, paths: { state: statePath } };
  const result = await readUsageReport(project, { budgetPolicy: { softTokens: 30 } });
  assert.equal(result.usage.totalTokens, 36);
  assert.equal(result.budget.action, 'adjust_strategy');
  assert.doesNotMatch(JSON.stringify(result), /PRIVATE|unrelated/);
  assert.equal(await readFile(statePath, 'utf8'), state);
  const missing = await readUsageReport({ active: true, paths: { state: path.join(root, 'missing.json') } });
  assert.equal(missing.usage.status, 'unknown');
  assert.equal(missing.source.status, 'missing');
  await writeFile(statePath, '{invalid');
  const invalid = await readUsageReport(project);
  assert.equal(invalid.source.status, 'invalid');
  assert.equal(invalid.usage.totalTokens, null);
});

test('usage report rejects non-regular inputs without waiting for a FIFO writer', { skip: process.platform === 'win32', timeout: 2000 }, async t => {
  const { readUsageReport } = await load();
  const root = await mkdtemp(path.join(os.tmpdir(), 'agent-team-usage-input-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const fifo = path.join(root, 'receipts.pipe');
  await promisify(execFile)('mkfifo', [fifo]);
  // Let the negative baseline settle too; never strand a libuv open on timeout.
  const release = setTimeout(async () => {
    const writer = await open(fifo, constants.O_RDWR | constants.O_NONBLOCK);
    await writer.close();
  }, 500);
  t.after(() => clearTimeout(release));
  const started = performance.now();
  const result = await readUsageReport(undefined, { receiptPath: fifo });
  assert.ok(performance.now() - started < 400, 'non-regular input waited for a writer');
  assert.equal(result.source.status, 'invalid');
  assert.equal(result.usage.totalTokens, null);
});
