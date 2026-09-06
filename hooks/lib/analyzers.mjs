import path from "node:path";

function finding(id, file, message, details) {
  return { id, severity: "warning", path: file.path, message, ...(details ? { details } : {}) };
}

function testData(file) {
  if (!/(^|\/)(test|tests|spec|specs)(\/|\.)/i.test(file.path)) return [];
  const source = file.changedContent ?? "";
  if (/(^|\/)(fixtures?|disposable)(\/|\.)/i.test(file.path) || /\b(disposable|test-owned fixture)\b/i.test(source)) return [];
  const destructive = /\b(delete\s+from|drop\s+table|truncate\s+table)\b/i.test(source)
    || /\b(?:db|database|client|prisma|supabase|adminClient)\s*\.\s*(?:delete|remove)\s*\(/i.test(source);
  const privileged = /\b(service_role|adminClient|superuser|rootClient)\b/i.test(source);
  const output = [];
  if (destructive) output.push(finding("test_data_destructive", file, "Test code can delete shared or live data. Use test-owned records or a disposable target."));
  if (privileged) output.push(finding("test_privileged_client", file, "Test code uses a privileged data client. Confirm isolation and the minimum required access."));
  if (destructive && !/\b(assert|expect|where|owner(?:Id|_id)|test[_-]?run[_-]?id)\b/i.test(source)) {
    output.push(finding("test_cleanup_assertion", file, "Destructive test cleanup has no visible ownership or target assertion."));
  }
  return output;
}

function migration(file, context) {
  if (!/(^|\/)migrations?\/.*\.sql$/i.test(file.path)) return [];
  const source = file.changedContent ?? "";
  const settings = context.migration ?? {};
  const output = [];
  if (!settings.databaseType) output.push(finding("migration_database_unknown", file, "The database type is unknown; dialect checks are unavailable."));
  if (/\b(drop\s+(?:column|table)|create\s+(?:table|index))\b/i.test(source) && !/\bif\s+(?:not\s+)?exists\b/i.test(source)) {
    output.push(finding("migration_guard_missing", file, "The migration changes schema without an existence guard."));
  }
  if (!settings.environment || settings.environment === "unknown") output.push(finding("migration_environment_unknown", file, "The migration target environment is unknown."));
  if (!settings.disposableRun) output.push(finding("migration_disposable_unverified", file, "No disposable-database migration run is recorded."));
  if (settings.catalogStatus !== "passed") {
    output.push(finding("migration_catalog_unavailable", file, "Database catalog verification is unavailable; this is not a pass."));
  }
  const statements = source.replace(/--.*$/gm, "");
  if (/\binvariant\s*:/i.test(source) && !/\b(check|unique|foreign\s+key|references)\b/i.test(statements)) {
    output.push(finding("migration_invariant_unenforced", file, "A stated invariant has no visible database constraint."));
  }
  return output;
}

function recurringCost(file, context) {
  const added = file.changedContent ?? "";
  const removed = file.previousContent ?? "";
  const schedule = /\b(cron|schedule|every[_ -]?(?:minute|hour|day)|rate\s*\()/i;
  if (!schedule.test(added) || added.trim() === removed.trim()) return [];
  const estimatedCost = context.pricing?.estimatedCost ?? "unknown";
  return [finding("recurring_cost_change", file, "Changed content contains a recurring schedule. Confirm whether its frequency changed and check operating cost.", { estimatedCost })];
}

/** Run bounded changed-content heuristics and deduplicate one batch of advice. */
export function analyzeChangedFiles(files, context = {}) {
  const seen = new Set();
  const output = [];
  for (const file of files) {
    for (const item of [...testData(file), ...migration(file, context), ...recurringCost(file, context)]) {
      const key = `${item.id}:${path.normalize(item.path)}`;
      if (!seen.has(key)) {
        seen.add(key);
        output.push(item);
      }
    }
  }
  return output;
}
