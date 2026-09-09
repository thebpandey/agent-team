export const ROLE_DEFINITIONS = Object.freeze([
  { id: "project_orchestrator", name: "Morpheus", purpose: "Project and team orchestration" },
  { id: "complex_developer", name: "Neo", purpose: "Complex development and difficult reasoning" },
  { id: "developer", name: "Trinity", purpose: "Standard development and UI/UX development" },
  { id: "routine_developer", name: "Tank", purpose: "Routine bounded development" },
  { id: "reviewer", name: "Agent Smith", purpose: "Independent code review" },
  { id: "visual_reviewer", name: "The Oracle", purpose: "Independent visual review" },
]);

export const ROUTING_PROFILES = Object.freeze({
  quality: { label: "Quality focused", description: "Use the strongest approved route for each role." },
  balanced: { label: "Balanced", description: "Balance quality, latency, and cost without reducing checks." },
  economical: { label: "Economical", description: "Prefer lower-cost approved routes with the same acceptance gates." },
});
