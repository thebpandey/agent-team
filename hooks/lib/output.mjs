function message(decision) {
  return decision.messages.filter(Boolean).join("\n");
}

/** Convert one shared decision to the selected host's documented hook response. */
export function adaptOutput(runtime, event, decision) {
  const text = message(decision);
  const hookSpecificOutput = { hookEventName: event };
  if (!decision.allow) {
    hookSpecificOutput.permissionDecision = "deny";
    hookSpecificOutput.permissionDecisionReason = text || "Agent-Team policy denied this operation.";
  } else if (text) {
    hookSpecificOutput.additionalContext = text;
  }
  return { hookSpecificOutput };
}
