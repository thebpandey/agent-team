import { readFile } from "node:fs/promises";

async function text(file) {
  try {
    return await readFile(file, "utf8");
  } catch (error) {
    if (error.code === "ENOENT") return "";
    throw error;
  }
}

function label(source, name) {
  return source.match(new RegExp(`^${name}:\\s*(.+)$`, "im"))?.[1].trim();
}

function tables(source) {
  const lines = source.split(/\r?\n/);
  const output = [];
  for (let index = 0; index < lines.length - 1; index += 1) {
    if (!/^\s*\|/.test(lines[index]) || !/^\s*\|(?:\s*:?-+:?\s*\|)+\s*$/.test(lines[index + 1])) continue;
    const headers = cells(lines[index]).map((entry) => entry.toLowerCase());
    const rows = [];
    index += 2;
    while (index < lines.length && /^\s*\|/.test(lines[index])) {
      const values = cells(lines[index]);
      rows.push(Object.fromEntries(headers.map((header, position) => [header, values[position] ?? ""])));
      index += 1;
    }
    output.push(rows);
  }
  return output;
}

function cells(line) {
  return line.trim().replace(/^\||\|$/g, "").split("|").map((entry) => entry.trim());
}

/** Read identities and task status from their canonical records; state.json only carries gate evidence. */
export async function loadCanonicalState(project) {
  const [teamsText, tasksText, stateText] = await Promise.all([
    text(project.paths.teams),
    text(project.paths.tasks),
    text(project.paths.state),
  ]);
  let state = {};
  if (stateText) state = JSON.parse(stateText);
  return {
    state,
    registry: {
      projectId: label(teamsText, "Project"),
      projectOwner: label(teamsText, "Project owner"),
      integrationOwner: label(teamsText, "Integration owner"),
      teams: tables(teamsText).flat().filter((row) => row["team id"]),
    },
    tasks: tables(tasksText).flat().filter((row) => row.id),
  };
}

export function identityFor(registry, sessionId) {
  if (sessionId === registry.projectOwner) return { role: "project_owner", sessionId };
  const team = registry.teams.find((entry) => entry.session.split(/\s*,\s*/).includes(sessionId));
  return team ? { role: "team", sessionId, team } : { role: "unknown", sessionId };
}
