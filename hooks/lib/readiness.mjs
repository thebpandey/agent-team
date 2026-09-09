const missingDefinitions = {
  scope: "What exact work is in scope for this run?",
  acceptance_conditions: "Which observable outcomes must pass before this work is accepted?",
  actionable_tasks: "Which task is actionable now, and what blocks the others?",
  integration_branch: "Which branch is the canonical integration target?",
  verification_commands: "Which commands verify the requested behavior?",
  authorization_boundaries: "Which files and external actions are authorized for this work?",
  tracker_available: "When will the selected tracker be available again?",
};

const BASELINE_CAPABILITIES = Object.freeze(["serena", "playwright-cli"]);
const TERMINAL_TASK_STATUSES = new Set(["closed", "complete", "completed", "done", "integrated"]);

function gap(id, detail) {
  return { id, question: missingDefinitions[id], ...(detail ? { detail } : {}) };
}

function readyTask(tasks) {
  if (!Array.isArray(tasks)) return undefined;
  const ids = tasks.map(({ id }) => id);
  if (ids.some((id) => typeof id !== "string" || !id) || new Set(ids).size !== ids.length) return undefined;
  const byId = new Map(tasks.map((task) => [task.id, task]));
  return tasks.find((task) => task.status === "ready" && (task.dependencies === undefined || (
    Array.isArray(task.dependencies) && task.dependencies.every((id) => {
      const predecessor = typeof id === "string" ? byId.get(id) : undefined;
      return predecessor && TERMINAL_TASK_STATUSES.has(String(predecessor.status).toLowerCase());
    })
  )));
}

function requiredCapabilities(plan) {
  return [...new Set([...BASELINE_CAPABILITIES, ...(Array.isArray(plan.requiredCapabilities) ? plan.requiredCapabilities : [])])];
}

function requirements(plan, tracker, capabilities) {
  const missing = [];
  if (typeof plan.scope !== "string" || !plan.scope.trim()) missing.push(gap("scope"));
  if (!Array.isArray(plan.acceptance) || plan.acceptance.length === 0) missing.push(gap("acceptance_conditions"));
  if (!Array.isArray(plan.tasks) || !readyTask(plan.tasks)) missing.push(gap("actionable_tasks"));
  if (typeof plan.branch !== "string" || !plan.branch.trim()) missing.push(gap("integration_branch"));
  if (!Array.isArray(plan.verification) || plan.verification.length === 0) missing.push(gap("verification_commands"));
  if (!plan.authority || typeof plan.authority !== "object" || Object.keys(plan.authority).length === 0) missing.push(gap("authorization_boundaries"));
  if (!tracker || tracker.status !== "current") missing.push(gap("tracker_available", tracker?.reason ?? tracker?.status));
  for (const id of requiredCapabilities(plan)) {
    const receipt = capabilities?.[id];
    if (receipt?.functional !== "passed" || receipt?.availableToWorker !== "passed") {
      missing.push({ id: `capability:${id}`, question: `How will the required ${id} capability become functional for this task?` });
    }
  }
  return missing;
}

function result(path, planning, plan, tracker, capabilities, projectInitialization = { required: false }) {
  const missing = requirements(plan, tracker, capabilities);
  const eligibleTask = readyTask(plan.tasks ?? null) ?? null;
  return {
    path,
    planning,
    eligible: missing.length === 0 && Boolean(eligibleTask),
    readyForDispatch: missing.length === 0 && Boolean(eligibleTask) && !projectInitialization.required,
    eligibleTask,
    scope: plan.scope,
    acceptance: plan.acceptance ?? [],
    tasks: plan.tasks ?? [],
    branch: plan.branch,
    tracker,
    verification: plan.verification ?? [],
    authority: plan.authority ?? {},
    requiredCapabilities: requiredCapabilities(plan),
    projectInitialization,
    missing,
  };
}

export function assessReadiness({ kickoff, existing, request, project = {}, capabilities = {} } = {}) {
  if (kickoff?.status === "approved" && kickoff.handoff) {
    return result("kickoff", "reused", kickoff.handoff, kickoff.handoff.tracker, capabilities);
  }
  if (existing?.plan || existing?.tracker) {
    const plan = existing.plan ?? {};
    return result("existing", "adopted", plan, existing.tracker ?? plan.tracker, capabilities);
  }
  const tracker = project.tracker ?? { kind: "markdown", path: ".agent-team/TASKS.md", status: "current" };
  const plan = {
    scope: request?.summary,
    acceptance: request?.acceptance ?? [],
    tasks: request?.summary ? [{
      id: "AT-001",
      title: request.summary,
      status: "ready",
      dependencies: [],
      acceptance: request.acceptance ?? [],
    }] : [],
    branch: project.currentBranch,
    verification: project.verification ?? [],
    authority: project.authority ?? {},
    requiredCapabilities: project.requiredCapabilities ?? [],
  };
  return result("standalone", "created", plan, tracker, capabilities, {
    required: true,
    projectId: project.projectId ?? null,
    projectOwner: project.projectOwner ?? null,
    teamsPath: ".agent-team/TEAMS.md",
    tracker,
    teams: [{
      id: "TEAM-001",
      name: "Morpheus 01",
      owner: project.projectOwner ?? null,
      taskIds: plan.tasks.map(({ id }) => id),
      status: "planned",
    }],
  });
}
