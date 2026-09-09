(() => {
  const search = document.querySelector("#task-search");
  const filter = document.querySelector("#task-status");
  const rows = [...document.querySelectorAll("#task-rows tr")];
  const results = document.querySelector("#results");
  for (const status of [...new Set(rows.map((row) => row.dataset.status))].sort()) { const option = document.createElement("option"); option.value = status; option.textContent = status; filter?.append(option); }
  function apply() { const query = search?.value.trim().toLowerCase() || ""; const status = filter?.value || ""; let visible = 0; for (const row of rows) { const show = (!query || row.dataset.search.includes(query)) && (!status || row.dataset.status === status); row.hidden = !show; if (show) visible += 1; } if (results) results.textContent = `${visible} task${visible === 1 ? "" : "s"} shown.`; }
  search?.addEventListener("input", apply); filter?.addEventListener("change", apply); document.querySelector("#refresh")?.addEventListener("click", () => location.reload()); apply();
})();
