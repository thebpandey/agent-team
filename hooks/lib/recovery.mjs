import { readFile, readdir } from "node:fs/promises";
import path from "node:path";

/** Report recovery evidence freshness. This advisory never resumes or edits work. */
export async function inspectRecovery(project, { now = new Date(), staleAfterMs = 15 * 60_000 } = {}) {
  if (!project.active) return { status: "unavailable", reason: project.reason };
  let names;
  try {
    names = await readdir(project.paths.checkpoints);
  } catch (error) {
    if (error.code === "ENOENT") return { status: "unavailable", reason: "checkpoint_missing" };
    return { status: "unavailable", reason: "checkpoint_unreadable" };
  }

  const records = [];
  for (const name of names.filter((entry) => entry.endsWith(".json")).slice(0, 100)) {
    try {
      const record = JSON.parse(await readFile(path.join(project.paths.checkpoints, name), "utf8"));
      const timestamp = Date.parse(record.updatedAt);
      if (Number.isFinite(timestamp)) records.push({ ...record, timestamp });
    } catch {
      // A malformed record is unavailable evidence, not proof that work is current.
    }
  }
  records.sort((left, right) => right.timestamp - left.timestamp);
  const latest = records[0];
  if (!latest) return { status: "unavailable", reason: "checkpoint_invalid" };
  return {
    status: now.getTime() - latest.timestamp <= staleAfterMs ? "current" : "stale",
    updatedAt: latest.updatedAt,
    sessionId: latest.sessionId,
    nextAction: latest.nextAction,
  };
}
