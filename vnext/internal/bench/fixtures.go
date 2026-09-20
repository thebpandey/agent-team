package bench

type Fixture struct {
	Name, Kind, InputPath, ExpectedFallback string
	Complexity                              int
}

func Fixtures() []Fixture {
	rows := []struct{ name, kind string }{
		{"discovery-small", "discovery"}, {"discovery-large", "discovery"}, {"discovery-generated", "discovery"},
		{"test-output-short", "test-output"}, {"test-output-verbose", "test-output"}, {"test-output-failure", "test-output"},
		{"semantic-definition", "semantic"}, {"semantic-references", "semantic"}, {"semantic-callers", "semantic"}, {"semantic-symbol-search", "semantic"},
		{"refactor-imports", "refactor"}, {"refactor-api", "refactor"}, {"refactor-migration", "refactor"},
		{"blast-radius-small", "blast-radius"}, {"blast-radius-large", "blast-radius"}, {"parallel-boundary", "parallel"},
		{"ui-layout", "ui"}, {"ui-responsive", "ui"}, {"ui-accessibility", "ui"}, {"ui-interaction", "ui"},
		{"recovery-malformed", "recovery"}, {"recovery-stale", "recovery"}, {"recovery-conflict", "recovery"}, {"recovery-capacity", "recovery"},
	}
	out := make([]Fixture, 0, len(rows))
	for i, row := range rows {
		out = append(out, Fixture{Name: row.name, Kind: row.kind, InputPath: "generated:" + row.name, ExpectedFallback: "native", Complexity: i + 1})
	}
	return out
}
