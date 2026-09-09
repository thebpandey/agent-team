import { constants } from 'node:fs';
import { open } from 'node:fs/promises';

const tokenFields = ['inputTokens', 'cachedInputTokens', 'outputTokens'];
const countFields = ['repairAttempts', 'compactions', 'agentTimeMs', 'compactionTimeMs', 'checkpointTimeMs', 'startupTimeMs'];
const fields = ['id', 'agentId', 'role', 'source', ...tokenFields, ...countFields];
const nonnegative = value => Number.isSafeInteger(value) && value >= 0;
const safeSum = values => {
  let total = 0;
  for (const value of values) {
    if (value === null || !nonnegative(total + value)) return null;
    total += value;
  }
  return total;
};

/** Summarize per-request usage receipts, not cumulative session counters.
 * Input includes cached input; adding the cached subset again overstates cost.
 * No price or absent worker usage is inferred from a model name or task count.
 */
export function summarizeUsage(records, { expectedAgentIds = [] } = {}) {
  if (!Array.isArray(records) || !Array.isArray(expectedAgentIds) || expectedAgentIds.some(id => typeof id !== 'string' || !id)) throw new TypeError('Usage records and expected agent IDs must be arrays of valid entries.');
  const receipts = new Map();
  const conflicts = new Set();
  let invalidRecords = 0;
  let duplicateRecords = 0;
  let conflictingRecords = 0;
  for (const input of records) {
    if (!input || typeof input !== 'object' || typeof input.id !== 'string' || !input.id || typeof input.agentId !== 'string' || !input.agentId) {
      invalidRecords += 1;
      continue;
    }
    const receipt = Object.fromEntries(fields.filter(field => input[field] !== undefined).map(field => [field, input[field]]));
    if ([...tokenFields, ...countFields].some(field => receipt[field] !== undefined && !nonnegative(receipt[field])) ||
      (receipt.cachedInputTokens !== undefined && (receipt.inputTokens === undefined || receipt.cachedInputTokens > receipt.inputTokens)) ||
      (receipt.source !== undefined && !['observed', 'estimate'].includes(receipt.source))) {
      invalidRecords += 1;
      continue;
    }
    const serialized = JSON.stringify(receipt);
    if (receipts.has(receipt.id)) {
      if (receipts.get(receipt.id).serialized === serialized) duplicateRecords += 1;
      else {
        conflictingRecords += 1;
        conflicts.add(receipt.id);
      }
    } else receipts.set(receipt.id, { receipt, serialized });
  }

  const observed = [];
  let estimatedRecords = 0;
  for (const { receipt } of receipts.values()) {
    if (conflicts.has(receipt.id)) continue;
    if (receipt.source === 'estimate') estimatedRecords += 1;
    else observed.push(receipt);
  }
  const agents = new Set(observed.map(receipt => receipt.agentId));
  const missingAgents = [...new Set(expectedAgentIds)].filter(id => !agents.has(id));
  const sum = field => safeSum(observed.map(receipt => receipt[field] ?? 0));
  const completeField = field => observed.length > 0 && observed.every(receipt => receipt[field] !== undefined);
  const knownTokens = safeSum([sum('inputTokens'), sum('outputTokens')]);
  const rosterKnown = expectedAgentIds.length > 0;
  const complete = rosterKnown && knownTokens !== null && observed.length > 0 && !invalidRecords && !conflictingRecords && !estimatedRecords && !missingAgents.length &&
    completeField('inputTokens') && completeField('outputTokens');
  const byRole = Object.create(null);
  for (const receipt of observed) {
    const role = typeof receipt.role === 'string' && receipt.role ? receipt.role : 'unknown';
    const row = byRole[role] ??= { records: 0, knownTokens: 0 };
    row.records += 1;
    row.knownTokens = safeSum([row.knownTokens, receipt.inputTokens ?? 0, receipt.outputTokens ?? 0]);
  }
  return {
    status: complete ? 'observed' : records.length ? 'partial' : 'unknown',
    records: observed.length,
    invalidRecords, duplicateRecords, conflictingRecords, estimatedRecords, missingAgents,
    coverage: { rosterKnown, expectedAgents: new Set(expectedAgentIds).size, observedAgents: agents.size },
    inputTokens: completeField('inputTokens') ? sum('inputTokens') : null,
    cachedInputTokens: completeField('cachedInputTokens') ? sum('cachedInputTokens') : null,
    outputTokens: completeField('outputTokens') ? sum('outputTokens') : null,
    totalTokens: complete ? knownTokens : null,
    knownTokens,
    byRole,
    ...Object.fromEntries(countFields.map(field => [field, observed.some(receipt => receipt[field] !== undefined) ? sum(field) : null])),
    incompleteCounters: countFields.filter(field => !completeField(field)),
    cost: null,
    limitation: 'Known tokens and counters are observed subtotals when coverage is incomplete. Agent time is summed work, not elapsed wall time. Price, acceptance quality and savings require separate matched evidence.',
  };
}

/** Read an explicit receipt envelope or existing operational state; never initialize it.
 * The caller supplies actual native request receipts. Absent host counters remain unknown.
 * Return only the allowlisted aggregate, never prompts, raw tool output or other state.
 */
export async function readUsageReport(project, { receiptPath, budgetPolicy = {} } = {}) {
  const sourcePath = receiptPath ?? (project?.active ? project.paths?.state : undefined);
  let source = { status: sourcePath ? 'current' : 'missing', kind: receiptPath ? 'receipt_file' : 'operational_state' };
  let records = [];
  let expectedAgentIds = [];
  if (sourcePath) {
    let handle;
    try {
      // A receipt is a bounded regular file, never a pipe waiting for a writer.
      // Validate the opened handle so replacement between lookup and open cannot
      // turn a status-only read into an unbounded FIFO wait.
      handle = await open(sourcePath, constants.O_RDONLY | constants.O_NONBLOCK);
      const metadata = await handle.stat();
      if (!metadata.isFile() || metadata.size > 4 * 1024 * 1024) throw new Error('Invalid receipt file.');
      const buffer = Buffer.alloc(4 * 1024 * 1024 + 1);
      let length = 0;
      while (length < buffer.length) {
        const result = await handle.read(buffer, length, buffer.length - length, null);
        if (!result.bytesRead) break;
        length += result.bytesRead;
      }
      if (length === buffer.length) throw new Error('Receipt input exceeds 4 MiB.');
      const parsed = JSON.parse(buffer.subarray(0, length).toString('utf8'));
      const envelope = receiptPath ? parsed : parsed?.usage;
      if (envelope === undefined) source.status = 'missing';
      else {
        if (!envelope || !Array.isArray(envelope.records) || !Array.isArray(envelope.expectedAgentIds) ||
          envelope.expectedAgentIds.some(id => typeof id !== 'string' || !id)) throw new Error('Invalid receipt envelope.');
        records = envelope.records;
        expectedAgentIds = envelope.expectedAgentIds;
      }
    } catch (error) {
      source.status = error.code === 'ENOENT' ? 'missing' : 'invalid';
    } finally {
      await handle?.close();
    }
  }
  const usage = summarizeUsage(records, { expectedAgentIds });
  return { status: 'completed', source, usage, budget: assessBudget(usage, budgetPolicy) };
}

/** A budget changes strategy or requests a safe checkpoint; it never waives checks. */
export function assessBudget(usage, policy = {}) {
  for (const field of ['softTokens', 'hardTokens']) {
    if (policy[field] !== undefined && (!Number.isSafeInteger(policy[field]) || policy[field] <= 0)) {
      throw new TypeError(`${field} must be a positive integer.`);
    }
  }
  const result = (action, reason) => ({ action, reason, waiveChecks: false });
  if (policy.softTokens === undefined && policy.hardTokens === undefined) return result('continue', 'no_budget');
  const tokens = nonnegative(usage?.totalTokens) ? usage.totalTokens : nonnegative(usage?.knownTokens) ? usage.knownTokens : 0;
  if (policy.hardTokens !== undefined && tokens >= policy.hardTokens) return result('checkpoint', 'hard_budget');
  if (policy.softTokens !== undefined && tokens >= policy.softTokens) return result('adjust_strategy', 'soft_budget');
  return result('continue', usage?.status === 'observed' ? 'within_budget' : 'usage_incomplete');
}
