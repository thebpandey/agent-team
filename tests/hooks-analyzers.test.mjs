import assert from "node:assert/strict";
import test from "node:test";

import { analyzeChangedFiles } from "../hooks/lib/analyzers.mjs";

test("test-data advice flags shared destructive use, privileged clients, and weak cleanup assertions", () => {
  // This test catches destructive shared-data cleanup that receives no advisory.
  const findings = analyzeChangedFiles([
    {
      action: "edit",
      path: "tests/customer-live.test.js",
      changedContent: "const adminClient = service_role(); await adminClient.delete('customer');",
    },
  ], { migration: { catalogStatus: "unavailable" } });
  const ids = findings.map(({ id }) => id);

  assert.ok(ids.includes("test_data_destructive"));
  assert.ok(ids.includes("test_privileged_client"));
  assert.ok(ids.includes("test_cleanup_assertion"));
});

test("fixture cleanup and Map.delete do not create destructive-data warnings", () => {
  // This test catches the realistic Map.delete and disposable-fixture false positives.
  const findings = analyzeChangedFiles([
    { action: "edit", path: "tests/fixtures/cache.test.js", changedContent: "fixtureRecords.delete(id); cache.delete(key); // disposable" },
  ]);
  assert.deepEqual(findings, []);
});

test("migration advice covers guards, environment, disposable execution, catalog checks, invariants, and dedupe", () => {
  // This test catches an unavailable migration check being reported as a pass.
  const migration = {
    action: "edit",
    path: "db/migrations/20260906_accounts.sql",
    changedContent: "-- invariant: email is unique\nALTER TABLE accounts DROP COLUMN legacy;",
  };
  const findings = analyzeChangedFiles([migration, migration], {
    migration: { databaseType: "postgres", environment: "unknown", disposableRun: false, catalogStatus: "unavailable" },
  });
  const ids = findings.map(({ id }) => id);

  assert.equal(ids.length, new Set(ids).size);
  assert.ok(ids.includes("migration_guard_missing"));
  assert.ok(ids.includes("migration_environment_unknown"));
  assert.ok(ids.includes("migration_disposable_unverified"));
  assert.ok(ids.includes("migration_catalog_unavailable"));
  assert.ok(ids.includes("migration_invariant_unenforced"));
});

test("new or materially changed schedules warn with unknown pricing while unrelated edits do not", () => {
  // This test catches cost claims based on an existing unchanged schedule line.
  const added = analyzeChangedFiles([{ action: "edit", path: "jobs.yml", previousContent: "cron: daily", changedContent: "cron: every_minute" }]);
  const unrelated = analyzeChangedFiles([{ action: "edit", path: "jobs.yml", previousContent: "name: old", changedContent: "name: clearer" }]);

  assert.equal(added.some(({ id }) => id === "recurring_cost_change"), true);
  assert.equal(added.find(({ id }) => id === "recurring_cost_change").details.estimatedCost, "unknown");
  assert.equal(unrelated.some(({ id }) => id === "recurring_cost_change"), false);
});
