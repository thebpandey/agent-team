import { mkdir, open, rm } from "node:fs/promises";
import path from "node:path";

const delay = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds));

/** Serialize short shared writes. Existing owners are never declared orphaned by age alone. */
export async function withDirectoryLock(lockPath, metadata, callback, { timeoutMs = 1000, budget } = {}) {
  const check = () => {
    if (budget?.signal.aborted || budget?.remaining() === 0) throw Object.assign(new Error("Agent-Team event deadline exceeded while waiting for a lock."), { code: "EVENT_DEADLINE" });
  };
  check();
  await mkdir(path.dirname(lockPath), { recursive: true, mode: 0o700 });
  const started = Date.now();
  while (true) {
    check();
    let acquired = false;
    try {
      await mkdir(lockPath, { mode: 0o700 });
      acquired = true;
      check();
      const owner = await open(path.join(lockPath, "owner.json"), "wx", 0o600);
      try { check(); await owner.writeFile(`${JSON.stringify(metadata)}\n`); } finally { await owner.close(); }
      break;
    } catch (error) {
      if (acquired) await rm(lockPath, { force: true, recursive: true });
      if (error.code !== "EEXIST" || Date.now() - started >= timeoutMs) throw error;
      await delay(10);
    }
  }

  try {
    check();
    return await callback();
  } finally {
    await rm(lockPath, { force: true, recursive: true });
  }
}
