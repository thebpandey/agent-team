(() => {
  const search = document.querySelector("#task-search");
  const filter = document.querySelector("#task-status");
  const rows = [...document.querySelectorAll("#task-rows tr")];
  const results = document.querySelector("#results");
  let selectedTask = null;
  for (const status of [...new Set(rows.map((row) => row.dataset.status))].sort()) { const option = document.createElement("option"); option.value = status; option.textContent = status; filter?.append(option); }
  function apply() { const query = search?.value.trim().toLowerCase() || ""; const status = filter?.value || ""; let visible = 0; for (const row of rows) { const match = selectedTask ? row.querySelector('th')?.textContent === selectedTask : !query || row.dataset.search.includes(query); const show = match && (!status || row.dataset.status === status); row.hidden = !show; if (show) visible += 1; } if (results) results.textContent = `${visible} task${visible === 1 ? "" : "s"} shown.`; }
  search?.addEventListener("input", () => { selectedTask = null; apply(); });
  filter?.addEventListener("change", apply);
  for (const node of document.querySelectorAll('[data-graph-task]')) {
    const select = () => { selectedTask = node.dataset.graphTask; if (search) search.value = selectedTask; if (filter) filter.value = ''; apply(); document.querySelector('#tasks')?.scrollIntoView({ block: 'start' }); search?.focus({ preventScroll: true }); };
    node.addEventListener('click', select);
    node.addEventListener('keydown', event => { if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); select(); } });
  }
  document.querySelector("#refresh")?.addEventListener("click", () => location.reload()); apply();
})();
