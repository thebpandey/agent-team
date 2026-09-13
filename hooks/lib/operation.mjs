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

function unwrapLeanCtx(tokens) {
  const index = tokens.findIndex((token) => ["lean-ctx", "lean-ctx.exe"].includes(executable(token)));
  if (index === -1 || !["-c", "exec", "raw"].includes(tokens[index + 1])) return tokens;
  const wrapped = tokens.slice(index + 2);
  if (["-c", "exec"].includes(tokens[index + 1]) && wrapped[0] === "--raw") wrapped.shift();
  if (!wrapped.length) return tokens;
  return [...tokens.slice(0, index), ...tokenizeShell(wrapped.join(" "))];
}

function gitOperation(tokens) {
  const index = tokens.findIndex((token) => executable(token) === "git");
  if (index === -1) return undefined;
  let cursor = index + 1;
  let repository;
  let parserFailed = index !== 0;
  let seenRepository = false;
  while (cursor < tokens.length && tokens[cursor].startsWith("-")) {
    if (tokens[cursor] === "-C") {
      const candidate = tokens[cursor + 1];
      if (seenRepository || !candidate || candidate === "push" || candidate.startsWith("-") || [";", "|", "&"].includes(candidate)) parserFailed = true;
      else {
        repository = candidate;
        seenRepository = true;
        cursor += 1;
      }
    } else parserFailed = true;
    cursor += 1;
  }
  let commandIndex = cursor;
  let command = tokens[commandIndex];
  if (parserFailed) {
    const pushIndex = tokens.findIndex((token, position) => position > index && token === "push");
    if (pushIndex !== -1) {
      commandIndex = pushIndex;
      command = "push";
    } else if (tokens.slice(index + 1).some((token) => /(?:^|[=\s])push(?:\s|$)/.test(token))) command = "push";
  }
  return { command, repository, commandIndex, parserFailed };
}

function pushOperation(tokens, git) {
  const arguments_ = tokens.slice(git.commandIndex + 1);
  const control = tokens.some((token) => [";", "|", "&"].includes(token));
  const force = arguments_.some((token) => /^--force(?:$|=|-)/.test(token)
    || (/^-[^-]/.test(token) && token.slice(1).includes("f")));
  const options = arguments_.some((token) => token.startsWith("-"));
  const positional = arguments_.filter((token) => !token.startsWith("-"));
  const [remote, refspec, ...extra] = positional;
  const branch = typeof refspec === "string" && refspec.match(/^HEAD:((?:refs\/heads\/|(?!refs\/))[\w./-]+)$/);
  const tag = typeof refspec === "string" && refspec.match(/^(refs\/tags\/([\w./-]+))(?::(refs\/tags\/([\w./-]+)))?$/);
  const validTagName = (name) => /^[A-Za-z0-9][\w./-]*$/.test(name) && !/[./]$|\.\.|\/\//.test(name)
    && name.split("/").every((component) => !component.startsWith(".") && !component.toLowerCase().endsWith(".lock"));
  const validTag = tag && validTagName(tag[2]) && (!tag[3] || validTagName(tag[4]) && tag[1] === tag[3]);
  const targetRef = validTag ? tag[1] : branch ? `refs/heads/${branch[1].replace(/^refs\/heads\//, "")}` : undefined;
  return { valid: !git.parserFailed && !control && !force && !options && typeof remote === "string" && Boolean(targetRef) && extra.length === 0,
    ...(typeof remote === "string" ? { remote } : {}), ...(validTag ? { sourceRef: tag[1] } : {}), ...(targetRef ? { targetRef } : {}) };
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
export function classifyOperation(event, mappings = {}, { tracker } = {}) {
  if (event.operation.kind === "completion") return { ...event.operation };
  if (event.operation.kind === "file_change") {
    const trackerFiles = event.operation.files.filter((file) => (tracker
      ? tracker.kind === "markdown" && path.resolve(event.cwd, file.path) === tracker.path
      : path.basename(file.path) === "TASKS.md"));
    const terminalIds = (source) => new Set(String(source ?? "").split(/\r?\n/)
      .filter((line) => /\|\s*(verified|deployed)\s*\|/i.test(line))
      .map((line) => line.match(/^\s*\|\s*([^|]*)\|/)?.[1].trim() ?? ""));
    const taskIds = [...new Set(trackerFiles.flatMap((file) => {
      const previous = terminalIds(file.previousContent);
      return [...terminalIds(file.changedContent)].filter((id) => !previous.has(id));
    }))];
    if (taskIds.length) return { kind: "completion", taskId: taskIds[0], taskIds, files: event.operation.files,
      parserFailed: taskIds.length !== 1 || !taskIds[0] };
    return event.operation;
  }
  if (event.operation.kind === "provider") {
    const mapping = mappings.providers?.[event.operation.tool];
    if (!mapping) return { kind: "unknown_provider" };
    if (mapping.kind === "file_change") {
      const file = event.operation.input?.[mapping.pathField];
      const operation = {
        kind: "file_change",
        files: typeof file === "string" ? [{
          action: mapping.action ?? "edit",
          path: file,
          changedContent: mapping.contentField ? String(event.operation.input?.[mapping.contentField] ?? "") : "",
        }] : [],
        parserFailed: typeof file !== "string",
      };
      return operation.parserFailed ? operation : classifyOperation({ ...event, operation }, mappings, { tracker });
    }
    const sql = mapping.sqlField ? event.operation.input?.[mapping.sqlField] : "";
    return {
      kind: mapping.kind,
      sql,
      cascade: /\bcascade\b/i.test(sql ?? ""),
      parserFailed: mapping.sqlField && typeof sql !== "string",
      ...(mapping.process ? { process: mapping.process } : {}),
    };
  }
  if (event.operation.kind !== "shell") return event.operation;

  const command = event.operation.command;
  const tokens = unwrapLeanCtx(tokenizeShell(command));
  const bd = tokens.findIndex((token) => ["bd", "bd.exe"].includes(executable(token)));
  if (bd !== -1 && (!tracker || tracker.kind === "beads")) {
    const action = tokens.findIndex((token, index) => index > bd && ["close", "update"].includes(token));
    const closes = tokens[action] === "close" || (tokens[action] === "update"
      && tokens.some((token, index) => token === "--status=closed" || (["--status", "-s"].includes(token) && tokens[index + 1] === "closed")));
    if (closes) {
      const ids = [];
      let parserFailed = action !== bd + 1;
      for (let index = action + 1; index < tokens.length; index += 1) {
        const token = tokens[index];
        if (["--reason", "-r", "--status", "-s"].includes(token)) { index += 1; continue; }
        if (/^--(reason|status)=/.test(token) || ["--json", "--force", "-f"].includes(token)) continue;
        if (token.startsWith("-") || [";", "&", "|"].includes(token)) parserFailed = true;
        else ids.push(token);
      }
      return { kind: "completion", taskId: ids[0], parserFailed: parserFailed || ids.length !== 1 };
    }
  }
  const git = gitOperation(tokens);
  if (git?.command === "push") return { kind: "integration", repository: git.repository, method: "push", push: pushOperation(tokens, git) };
  const gh = tokens.findIndex((token) => executable(token) === "gh");
  if (gh !== -1 && tokens[gh + 1] === "pr" && ["create", "merge"].includes(tokens[gh + 2])) return { kind: "integration", method: "pull_request" };
  if (gh !== -1 && tokens[gh + 1] === "release" && tokens[gh + 2] === "create") return { kind: "release", process: "gh-release" };
  const packageManager = tokens.find((token) => ["npm", "pnpm"].includes(executable(token)));
  if (packageManager && tokens.includes("publish")) return { kind: "release", process: executable(packageManager) };
  if (tokens.some((token) => executable(token) === "vercel") && tokens.includes("--prod")) return { kind: "release", process: "vercel" };

  const database = sqlClient(tokens);
  if (database && destructiveSql(database.sql)) {
    return { kind: "database_destructive", sql: database.sql, cascade: /\bcascade\b/i.test(database.sql), parserFailed: !database.sql };
  }
  const mapped = mappings.shell?.find(({ prefix }) => command.trim().startsWith(prefix));
  if (mapped) return { kind: mapped.kind, cascade: false, ...(mapped.process ? { process: mapped.process } : {}) };
  const writeCommand = executable(tokens[0]);
  if (writeCommand === "rm") {
    return {
      kind: "file_change",
      files: tokens.slice(1).filter((token) => !token.startsWith("-")).map((file) => ({ action: "delete", path: file, changedContent: "" })),
    };
  }
  if (writeCommand === "mv") {
    const arguments_ = tokens.slice(1);
    const operands = [];
    let optionsEnded = false;
    let parserFailed = false;
    for (const argument of arguments_) {
      if ([";", "|", "&"].includes(argument)) {
        parserFailed = true;
        break;
      }
      if (!optionsEnded && argument === "--") {
        optionsEnded = true;
        continue;
      }
      if (!optionsEnded && argument.startsWith("-")) {
        const safeLong = new Set(["--force", "--interactive", "--no-clobber", "--verbose", "--no-target-directory", "--strip-trailing-slashes"]);
        if (!safeLong.has(argument) && !/^-([finvT]+)$/.test(argument)) parserFailed = true;
        continue;
      }
      operands.push(argument);
    }
    if (parserFailed || operands.length < 2) return { kind: "file_change", files: [], parserFailed: true };
    const destination = operands.at(-1);
    return {
      kind: "file_change",
      files: operands.slice(0, -1).map((source) => ({ action: "move", previousPath: source, path: destination, changedContent: "" })),
    };
  }
  if (writeCommand === "cp") {
    const targets = tokens.slice(1).filter((token) => !token.startsWith("-"));
    const destination = targets.at(-1);
    return destination ? { kind: "file_change", files: [{ action: writeCommand === "mv" ? "move" : "add_or_edit", path: destination, changedContent: "" }] } : { kind: "ordinary" };
  }
  if (/\b(drop|truncate|delete|destroy)[-_ ]?(?:data|database|table|records?)\b/i.test(command)) return { kind: "database_blind_spot" };
  return { kind: "ordinary" };
}
