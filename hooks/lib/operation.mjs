import path from "node:path";

export function tokenizeShell(command) {
  const tokens = [];
  let token = "";
  let quote;
  let escaped = false;
  for (const character of String(command ?? "")) {
    if (escaped) {
      token += character;
      escaped = false;
    } else if (character === "\\" && quote !== "'") {
      escaped = true;
    } else if (quote) {
      if (character === quote) quote = undefined;
      else token += character;
    } else if (character === "'" || character === '"') {
      quote = character;
    } else if (/\s/.test(character)) {
      if (token) tokens.push(token);
      token = "";
    } else if (";|&".includes(character)) {
      if (token) tokens.push(token);
      tokens.push(character);
      token = "";
    } else {
      token += character;
    }
  }
  if (token) tokens.push(token);
  return tokens;
}

function executable(token) {
  return path.basename(token ?? "").toLowerCase();
}

function gitOperation(tokens) {
  const index = tokens.findIndex((token) => executable(token) === "git");
  if (index === -1) return undefined;
  let cursor = index + 1;
  let repository;
  while (cursor < tokens.length && tokens[cursor].startsWith("-")) {
    if (tokens[cursor] === "-C") repository = tokens[++cursor];
    cursor += 1;
  }
  return { command: tokens[cursor], repository };
}

function sqlClient(tokens) {
  const clients = new Set(["psql", "mysql", "mysql.exe", "sqlite3", "sqlcmd"]);
  const index = tokens.findIndex((token) => clients.has(executable(token)));
  if (index === -1) return undefined;
  const commandFlag = tokens.findIndex((token, position) => position > index && ["-c", "-e", "--command", "--execute"].includes(token));
  const sql = commandFlag === -1 ? tokens.at(-1) : tokens[commandFlag + 1];
  return { sql: sql ?? "", client: executable(tokens[index]) };
}

function destructiveSql(sql) {
  return /\b(drop|truncate)\b|\bdelete\s+from\b|\balter\s+table\b[^;]*\bdrop\b/i.test(sql ?? "");
}

/** Recognize only explicit critical operation forms; unmatched paths remain documented blind spots. */
export function classifyOperation(event, mappings = {}) {
  if (event.operation.kind === "completion") return { kind: "completion", taskId: event.operation.taskId };
  if (event.operation.kind === "file_change") {
    const transition = event.operation.files.find((file) => path.basename(file.path) === "TASKS.md"
      && /\|\s*(verified|deployed)\s*\|/i.test(file.changedContent ?? "")
      && !/\|\s*(verified|deployed)\s*\|/i.test(file.previousContent ?? ""));
    if (transition) return { kind: "completion", taskId: transition.changedContent.match(/^\s*\|\s*([^|]+)\|/)?.[1].trim() };
    return event.operation;
  }
  if (event.operation.kind === "provider") {
    const mapping = mappings.providers?.[event.operation.tool];
    if (!mapping) return { kind: "unknown_provider" };
    if (mapping.kind === "file_change") {
      const file = event.operation.input?.[mapping.pathField];
      return {
        kind: "file_change",
        files: typeof file === "string" ? [{
          action: mapping.action ?? "edit",
          path: file,
          changedContent: mapping.contentField ? String(event.operation.input?.[mapping.contentField] ?? "") : "",
        }] : [],
        parserFailed: typeof file !== "string",
      };
    }
    const sql = mapping.sqlField ? event.operation.input?.[mapping.sqlField] : "";
    return { kind: mapping.kind, sql, cascade: /\bcascade\b/i.test(sql ?? ""), parserFailed: mapping.sqlField && typeof sql !== "string" };
  }
  if (event.operation.kind !== "shell") return event.operation;

  const command = event.operation.command;
  const tokens = tokenizeShell(command);
  const git = gitOperation(tokens);
  if (git?.command === "push") return { kind: "integration", repository: git.repository };
  const gh = tokens.findIndex((token) => executable(token) === "gh");
  if (gh !== -1 && tokens[gh + 1] === "pr" && ["create", "merge"].includes(tokens[gh + 2])) return { kind: "integration" };
  if (gh !== -1 && tokens[gh + 1] === "release" && tokens[gh + 2] === "create") return { kind: "release" };
  if (tokens.some((token) => ["npm", "pnpm"].includes(executable(token))) && tokens.includes("publish")) return { kind: "release" };
  if (tokens.some((token) => executable(token) === "vercel") && tokens.includes("--prod")) return { kind: "release" };

  const database = sqlClient(tokens);
  if (database && destructiveSql(database.sql)) {
    return { kind: "database_destructive", sql: database.sql, cascade: /\bcascade\b/i.test(database.sql), parserFailed: !database.sql };
  }
  const mapped = mappings.shell?.find(({ prefix }) => command.trim().startsWith(prefix));
  if (mapped) return { kind: mapped.kind, cascade: false };
  const writeCommand = executable(tokens[0]);
  if (writeCommand === "rm") {
    return {
      kind: "file_change",
      files: tokens.slice(1).filter((token) => !token.startsWith("-")).map((file) => ({ action: "delete", path: file, changedContent: "" })),
    };
  }
  if (["mv", "cp"].includes(writeCommand)) {
    const targets = tokens.slice(1).filter((token) => !token.startsWith("-"));
    const destination = targets.at(-1);
    return destination ? { kind: "file_change", files: [{ action: writeCommand === "mv" ? "move" : "add_or_edit", path: destination, changedContent: "" }] } : { kind: "ordinary" };
  }
  if (/\b(drop|truncate|delete|destroy)[-_ ]?(?:data|database|table|records?)\b/i.test(command)) return { kind: "database_blind_spot" };
  return { kind: "ordinary" };
}
