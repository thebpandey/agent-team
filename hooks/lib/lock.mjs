import { mkdir, open, rm } from "node:fs/promises";
import path from "node:path";

const delay = (milliseconds) => new Promise((resolve) => setTimeout(resolve, milliseconds));

/** Serialize short shared writes. Existing owners are never declared orphaned by age alone. */
export async function withDirectoryLock(lockPath, metadata, callback, { timeoutMs = 1000 } = {}) {
  await mkdir(path.dirname(lockPath), { recursive: true, mode: 0o700 });
  const started = Date.now();
  while (true) {
    try {
      await mkdir(lockPath, { mode: 0o700 });
      const owner = await open(path.join(lockPath, "owner.json"), "wx", 0o600);
      await owner.writeFile(`${JSON.stringify(metadata)}\n`);
      await owner.close();
      break;
    } catch (error) {
      if (error.code !== "EEXIST" || Date.now() - started >= timeoutMs) throw error;
      await delay(10);
    }
  }

  try {
    return await callback();
  } finally {
    await rm(lockPath, { force: true, recursive: true });
  }
}
