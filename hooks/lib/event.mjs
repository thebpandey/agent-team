function patchSections(source) {
  const sections = [];
  let current;

  for (const line of String(source ?? "").split(/\r?\n/)) {
    const header = line.match(/^\*\*\* (Add|Update|Delete) File: (.+)$/);
    if (header) {
      if (current) sections.push(current);
      current = { action: header[1].toLowerCase(), path: header[2], additions: [], deletions: [] };
      continue;
    }
    if (!current) continue;
    if (line.startsWith("*** Move to: ")) {
      current.previousPath = current.path;
      current.path = line.slice("*** Move to: ".length);
      current.action = "move";
    } else if (line.startsWith("+") && !line.startsWith("+++")) {
      current.additions.push(line.slice(1));
    } else if (line.startsWith("-") && !line.startsWith("---")) {
      current.deletions.push(line.slice(1));
    }
  }
  if (current) sections.push(current);
  return sections;
}

function codexFiles(command) {
  return patchSections(command).map(({ action, path, previousPath, additions, deletions }) => ({
    action: action === "update" ? "edit" : action,
    path,
    ...(previousPath ? { previousPath } : {}),
    changedContent: additions.join("\n"),
    ...(deletions.length ? { previousContent: deletions.join("\n") } : {}),
  }));
}

function fileOperation(runtime, tool, input) {
  if (runtime === "codex" && tool.toLowerCase() === "apply_patch") {
    return { kind: "file_change", tool, files: codexFiles(input.command) };
  }

  const name = tool.toLowerCase();
  if (name === "write") {
    return {
      kind: "file_change",
      tool,
      files: [{ action: "add_or_edit", path: input.file_path ?? input.path, changedContent: input.content ?? "" }],
    };
  }
  if (name === "edit") {
    return {
      kind: "file_change",
      tool,
      files: [{
        action: "edit",
        path: input.file_path ?? input.path,
        changedContent: input.new_string ?? input.content ?? "",
        previousContent: input.old_string ?? "",
      }],
    };
  }
  return undefined;
}

function batchFileOperation(runtime, event, payload) {
  if (runtime !== "claude" || event !== "PostToolBatch" || !Array.isArray(payload.tool_calls)) return undefined;
  const files = payload.tool_calls.flatMap((call) => {
    const operation = fileOperation(runtime, String(call.tool_name ?? ""), call.tool_input ?? {});
    return operation?.files ?? [];
  });
  return files.length ? { kind: "file_change", tool: "PostToolBatch", files } : { kind: "lifecycle" };
}

/** Normalize host payloads once so every policy sees the same operation shape. */
export function normalizeEvent(runtime, event, payload = {}) {
  if (!new Set(["codex", "claude"]).has(runtime)) throw new Error(`Unsupported runtime: ${runtime}`);
  const tool = String(payload.tool_name ?? payload.toolName ?? "");
  const input = payload.tool_input ?? payload.toolInput ?? {};
  const files = fileOperation(runtime, tool, input);
  let operation = files ?? batchFileOperation(runtime, event, payload);

  if (!operation && event === "UserPromptExpansion") {
    operation = { kind: "skill", skill: String(payload.skill_name ?? payload.skillName ?? payload.command_name ?? "") };
  } else if (!operation && ["bash", "shell", "unified_exec", "exec_command"].includes(tool.toLowerCase())) {
    operation = { kind: "shell", tool, command: String(input.command ?? input.cmd ?? "") };
  } else if (!operation && tool.toLowerCase() === "skill") {
    operation = { kind: "skill", tool, skill: String(input.skill ?? input.name ?? "") };
  } else if (!operation && tool) {
    operation = { kind: "provider", tool, input };
  } else if (!operation && event === "TaskCompleted") {
    operation = {
      kind: "completion",
      ...(payload.task_id ?? payload.taskId ? { taskId: String(payload.task_id ?? payload.taskId) } : {}),
      ...(payload.status ?? payload.outcome ? { outcome: String(payload.status ?? payload.outcome).toLowerCase() } : {}),
    };
  } else if (!operation) {
    operation = { kind: "lifecycle" };
  }

  const batchId = event === "PostToolBatch" && Array.isArray(payload.tool_calls)
    ? `batch:${payload.tool_calls.map((call) => call.tool_use_id ?? call.toolUseId).filter(Boolean).slice(0, 20).join(",")}`
    : "";
  return {
    runtime,
    event,
    cwd: payload.cwd ?? process.cwd(),
    sessionId: String(payload.session_id ?? payload.sessionId ?? "unknown"),
    eventId: String(payload.event_id ?? payload.eventId ?? payload.tool_use_id ?? payload.toolUseId ?? batchId),
    operation,
  };
}

export { codexFiles };
