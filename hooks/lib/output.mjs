function message(decision) {
  return [...new Set(decision.messages.filter(Boolean))].join("\n");
}

/** Convert one shared decision to the selected host's documented hook response. */
export function adaptOutput(runtime, event, decision) {
  const text = message(decision);
  if (runtime === "claude" && event === "TaskCompleted") return {};
  if (runtime === "claude" && ["PostToolUse", "PostToolBatch"].includes(event)) return text ? { additionalContext: text } : {};
  if (runtime === "claude" && ["UserPromptExpansion", "PreCompact"].includes(event) && !decision.allow) {
    return { decision: "block", reason: text || "Agent-Team policy denied this operation." };
  }
  if (runtime === "codex" && ["PreCompact", "PostCompact"].includes(event)) {
    if (!decision.allow) return { continue: false, stopReason: text || "Agent-Team policy denied this operation." };
    return text ? { systemMessage: text } : {};
  }
  const hookSpecificOutput = { hookEventName: event };
  if (!decision.allow) {
    hookSpecificOutput.permissionDecision = "deny";
    hookSpecificOutput.permissionDecisionReason = text || "Agent-Team policy denied this operation.";
  } else if (text) {
    hookSpecificOutput.additionalContext = text;
  }
  return { hookSpecificOutput };
}

export function adaptTransport(runtime, event, decision) {
  const text = message(decision) || "Agent-Team policy denied this operation.";
  if (runtime === "claude" && event === "TaskCompleted" && !decision.allow) {
    return { exitCode: 2, stdout: "", stderr: `${text}\n` };
  }
  return { exitCode: 0, stdout: `${JSON.stringify(adaptOutput(runtime, event, decision))}\n`, stderr: "" };
}
