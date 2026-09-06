function patchSections(source) {
  const sections = [];
  let current;

  for (const line of String(source ?? "").split(/\r?\n/)) {
    const header = line.match(/^\*\*\* (Add|Update|Delete) File: (.+)$/);
    if (header) {
      if (current) sections.push(current);
      current = { action: header[1].toLowerCase(), path: header[2], additions: [] };
      continue;
    }
    if (!current) continue;
    if (line.startsWith("*** Move to: ")) {
      current.previousPath = current.path;
      current.path = line.slice("*** Move to: ".length);
      current.action = "move";
    } else if (line.startsWith("+") && !line.startsWith("+++")) {
      current.additions.push(line.slice(1));
    }
  }
  if (current) sections.push(current);
  return sections;
}

function codexFiles(command) {
  return patchSections(command).map(({ action, path, previousPath, additions }) => ({
    action: action === "update" ? "edit" : action,
    path,
    ...(previousPath ? { previousPath } : {}),
    changedContent: additions.join("\n"),
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
      files: [{ action: "edit", path: input.file_path ?? input.path, changedContent: input.new_string ?? input.content ?? "" }],
    };
  }
  return undefined;
}

/** Normalize host payloads once so every policy sees the same operation shape. */
export function normalizeEvent(runtime, event, payload = {}) {
  if (!new Set(["codex", "claude"]).has(runtime)) throw new Error(`Unsupported runtime: ${runtime}`);
  const tool = String(payload.tool_name ?? payload.toolName ?? "");
  const input = payload.tool_input ?? payload.toolInput ?? {};
  const files = fileOperation(runtime, tool, input);
  let operation = files;

  if (!operation && ["bash", "shell", "unified_exec", "exec_command"].includes(tool.toLowerCase())) {
    operation = { kind: "shell", tool, command: String(input.command ?? input.cmd ?? "") };
  } else if (!operation && tool.toLowerCase() === "skill") {
    operation = { kind: "skill", tool, skill: String(input.skill ?? input.name ?? "") };
  } else if (!operation && tool) {
    operation = { kind: "provider", tool, input };
  } else if (!operation && event === "TaskCompleted") {
    operation = { kind: "completion" };
  } else if (!operation) {
    operation = { kind: "lifecycle" };
  }

  return {
    runtime,
    event,
    cwd: payload.cwd ?? process.cwd(),
    sessionId: String(payload.session_id ?? payload.sessionId ?? "unknown"),
    eventId: String(payload.event_id ?? payload.eventId ?? payload.tool_use_id ?? payload.toolUseId ?? ""),
    operation,
  };
}

export { codexFiles };
