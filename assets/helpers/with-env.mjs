import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

export function parseDotenv(source) {
  const values = {};
  for (const raw of source.split(/\r?\n/)) {
    const match = raw.match(/^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(.*?)\s*$/);
    if (!match) continue;
    let value = match[2];
    if ((value.startsWith("\"") && value.endsWith("\"")) || (value.startsWith("'") && value.endsWith("'"))) value = value.slice(1, -1);
    values[match[1]] = value;
  }
  return values;
}

function selfCheck() {
  const literal = "$(printf unsafe);`printf unsafe`";
  const parsed = parseDotenv(`SAFE='${literal}'\nexport SECOND=value\n`);
  if (parsed.SAFE !== literal || parsed.SECOND !== "value") throw new Error("self-check failed");
  process.stdout.write("self-check ok\n");
}

function main(argv) {
  if (argv[0] === "--self-check") return selfCheck();
  const [cwd, command, ...args] = argv;
  if (!cwd || !command) throw new Error("usage: with-env.mjs <cwd> <command> [args...]");
  const envFile = process.env.ENV_FILE ?? ".env";
  const env = { ...process.env, ...parseDotenv(readFileSync(path.resolve(cwd, envFile), "utf8")) };
  const result = spawnSync(command, args, { cwd, env, stdio: "inherit", shell: false });
  if (result.error) throw result.error;
  process.exitCode = result.status ?? 1;
}

if (process.argv[1] && fileURLToPath(import.meta.url) === path.resolve(process.argv[1])) {
  try { main(process.argv.slice(2)); }
  catch (error) { process.stderr.write(`${error.message}\n`); process.exitCode = 2; }
}
