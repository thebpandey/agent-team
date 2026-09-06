import { access, readFile } from "node:fs/promises";
import path from "node:path";
import { activationCapability, readActivationLogs } from "./telemetry.mjs";

async function present(file) {
  try {
    await access(file);
    return true;
  } catch {
    return false;
  }
}

async function json(file) {
  try {
    return JSON.parse(await readFile(file, "utf8"));
  } catch {
    return {};
  }
}

function registered(config) {
  return Object.values(config.hooks ?? {}).flat().some((group) => group.hooks?.some(({ command = "" }) => command.includes("agent-team-hook.mjs")));
}

/** Report each installation dimension separately. Trust stays unknown without native evidence. */
export async function getHealth({ home }) {
  const logs = await readActivationLogs(path.join(home, ".agent-team-hooks", "logs"));
  const codexConfig = await json(path.join(home, ".codex", "hooks.json"));
  const claudeConfig = await json(path.join(home, ".claude", "settings.json"));
  return {
    status: "completed",
    runtimes: {
      codex: {
        installed: await present(path.join(home, ".agents", "skills", "agent-team", "SKILL.md")),
        registered: registered(codexConfig),
        trusted: "unknown",
        activation: activationCapability("codex"),
        exercised: logs.records.some(({ runtime }) => runtime === "codex"),
      },
      claude: {
        installed: await present(path.join(home, ".claude", "skills", "agent-team", "SKILL.md")),
        registered: registered(claudeConfig),
        trusted: "unknown",
        activation: activationCapability("claude"),
        exercised: logs.records.some(({ runtime }) => runtime === "claude"),
      },
    },
    legacyCodexCopy: await present(path.join(home, ".codex", "skills", "agent-team", "SKILL.md")),
  };
}
