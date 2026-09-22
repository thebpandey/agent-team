package preparation

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func initRunner(t *testing.T, root string, mutate func(invocation) (string, error)) *fakeRunner {
	t.Helper()
	f := &fakeRunner{paths: map[string]string{"bd": "/tools/bd", "serena": "/tools/serena", "graphify": "/tools/graphify", "git": "/tools/git"}}
	f.run = func(ctx context.Context, c invocation) (string, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("unbounded initialization command")
		}
		if c.Dir != root {
			t.Fatalf("wrong project scope: %#v", c)
		}
		if c.Path == "/tools/git" && len(c.Args) > 0 && c.Args[0] == "ls-files" {
			var names []string
			err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				rel, _ := filepath.Rel(root, path)
				if entry.IsDir() {
					if entry.Name() == ".agent-team" || entry.Name() == "graphify-out" || entry.Name() == ".serena" || entry.Name() == ".beads" {
						return filepath.SkipDir
					}
					return nil
				}
				names = append(names, filepath.ToSlash(rel))
				return nil
			})
			if err != nil {
				return "", err
			}
			if len(names) == 0 {
				return "", nil
			}
			return strings.Join(names, "\x00") + "\x00", nil
		}
		if len(c.Args) == 1 && (c.Args[0] == "--version" || c.Args[0] == "version") {
			return "fixture 1.0.0", nil
		}
		return mutate(c)
	}
	return f
}

func TestInitializeWithoutConsentDoesNotWriteOrRunMutation(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "main.go", "package fixture\n")
	f := initRunner(t, root, func(c invocation) (string, error) {
		if c.Path == "/tools/git" {
			return strings.Repeat("a", 40), nil
		}
		t.Fatalf("unexpected mutation: %#v", c)
		return "", nil
	})
	got, err := initialize(context.Background(), root, []string{"beads", "serena", "graphify"}, false, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range got {
		if d.Prepared || !d.Available || d.Status != "needs_consent" {
			t.Fatalf("misleading preparation %#v", d)
		}
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 || entries[0].Name() != "main.go" {
		t.Fatal("created files without consent")
	}
}

func TestBeadsInitializationUsesExactSelectedPathAndSafeFlags(t *testing.T) {
	root := t.TempDir()
	calls := 0
	f := initRunner(t, root, func(c invocation) (string, error) {
		if strings.Join(c.Args, " ") == "--readonly status --json --no-activity" {
			return `{"summary":{"total_issues":0}}`, nil
		}
		calls++
		if c.Path != "/tools/bd" || strings.Join(c.Args, " ") != "init --skip-hooks --skip-agents --non-interactive --init-if-missing" || c.Env["BEADS_DIR"] != filepath.Join(root, ".beads") {
			t.Fatalf("unsafe init: %#v", c)
		}
		writeFixture(t, root, ".beads/metadata.json", `{"database":"dolt","backend":"dolt"}`)
		return "initialized", nil
	})
	got, err := initialize(context.Background(), root, []string{"beads"}, true, f)
	if err != nil || !got[0].Prepared || calls != 1 {
		t.Fatalf("got %#v, %v", got, err)
	}
	got, err = initialize(context.Background(), root, []string{"beads"}, false, f)
	if err != nil || !got[0].Prepared || calls != 1 {
		t.Fatalf("not reused: %#v %v", got, err)
	}
}

func TestSerenaInitializationConfinesHomeAndReusesConfig(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "main.go", "package main\n")
	calls := 0
	f := initRunner(t, root, func(c invocation) (string, error) {
		calls++
		if c.Path != "/tools/serena" || strings.Join(c.Args[:2], " ") != "project create" || c.Args[2] != root || c.Env["SERENA_HOME"] != filepath.Join(root, ".agent-team", "dependencies", "serena-home") {
			t.Fatalf("unscoped Serena init %#v", c)
		}
		writeFixture(t, root, ".serena/project.yml", "project_name: fixture\nlanguage_servers:\n- go\n")
		return "", nil
	})
	got, err := initialize(context.Background(), root, []string{"serena"}, true, f)
	if err != nil || !got[0].Prepared {
		t.Fatalf("got %#v %v", got, err)
	}
	got, err = initialize(context.Background(), root, []string{"serena"}, true, f)
	if err != nil || !got[0].Prepared || calls != 1 {
		t.Fatalf("not reused %#v %v", got, err)
	}
}

func TestSerenaMixedLanguagesAreExplicitAndDoNotScanManagedTools(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "api.py", "pass\n")
	writeFixture(t, root, "web/app.ts", "export {}\n")
	writeFixture(t, root, "node_modules/helper/index.rs", "fn main(){}")
	f := initRunner(t, root, func(c invocation) (string, error) {
		joined := strings.Join(c.Args, " ")
		if !strings.Contains(joined, "--language python --language typescript") || strings.Contains(joined, "--language rust") {
			t.Fatalf("interactive or unrelated language selection: %#v", c.Args)
		}
		writeFixture(t, root, ".serena/project.yml", "project_name: project\nlanguage_servers: [python, typescript]\n")
		return "", nil
	})
	got, err := initialize(context.Background(), root, []string{"serena"}, true, f)
	if err != nil || !got[0].Prepared {
		t.Fatalf("got %#v %v", got, err)
	}
}

func TestExistingBeadsMetadataWithoutBackendIsNotPrepared(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, ".beads/metadata.json", `{"database":"dolt"}`)
	f := initRunner(t, root, func(c invocation) (string, error) {
		if c.Path != "/tools/bd" || strings.Join(c.Args, " ") != "--readonly status --json --no-activity" || c.Env["BEADS_DIR"] != filepath.Join(root, ".beads") {
			t.Fatalf("unsafe health probe %#v", c)
		}
		return "", errors.New("database unavailable")
	})
	got, err := initialize(context.Background(), root, []string{"beads"}, false, f)
	if err != nil || got[0].Prepared || !got[0].Available || got[0].Status != "failed" {
		t.Fatalf("unhealthy backend ready %#v %v", got, err)
	}
}

func TestGraphifyPreparationReusesSameRevisionAndRefreshesChangedHEAD(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "main.go", "package fixture\n")
	revision := strings.Repeat("a", 40)
	extracts := 0
	f := initRunner(t, root, func(c invocation) (string, error) {
		if c.Path == "/tools/git" {
			if strings.Join(c.Args, " ") != "rev-parse --verify HEAD" {
				t.Fatalf("unexpected git %#v", c)
			}
			return revision, nil
		}
		if c.Path != "/tools/graphify" || strings.Join(c.Args, " ") != "extract . --code-only --no-viz" {
			t.Fatalf("unsafe extraction %#v", c)
		}
		extracts++
		writeGraphFixture(t, root, c, "graph.json", `{"nodes":[],"links":[]}`)
		return "", nil
	})
	got, err := initialize(context.Background(), root, []string{"graphify"}, true, f)
	if err != nil || !got[0].Prepared || extracts != 1 {
		t.Fatalf("got %#v %v", got, err)
	}
	got, err = initialize(context.Background(), root, []string{"graphify"}, true, f)
	if err != nil || !got[0].Prepared || extracts != 1 {
		t.Fatalf("not reused %#v %v", got, err)
	}
	revision = strings.Repeat("b", 40)
	got, err = initialize(context.Background(), root, []string{"graphify"}, false, f)
	if err != nil || got[0].Prepared || got[0].Status != "needs_consent" || extracts != 1 {
		t.Fatalf("stale graph accepted %#v %v", got, err)
	}
	got, err = initialize(context.Background(), root, []string{"graphify"}, true, f)
	if err != nil || !got[0].Prepared || extracts != 2 {
		t.Fatalf("not refreshed %#v %v", got, err)
	}
}

func TestUnknownExistingProjectStatePreserved(t *testing.T) {
	for _, name := range []string{"serena", "graphify"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := ".serena"
			if name == "graphify" {
				dir = "graphify-out"
			}
			writeFixture(t, root, dir+"/custom.txt", "preserve me")
			f := initRunner(t, root, func(c invocation) (string, error) {
				if c.Path == "/tools/git" {
					return strings.Repeat("a", 40), nil
				}
				t.Fatalf("overwrote unknown state %#v", c)
				return "", nil
			})
			got, err := initialize(context.Background(), root, []string{name}, true, f)
			if err != nil || got[0].Prepared || got[0].Status != "needs_consent" {
				t.Fatalf("got %#v %v", got, err)
			}
			content, _ := os.ReadFile(filepath.Join(root, dir, "custom.txt"))
			if string(content) != "preserve me" {
				t.Fatal("custom state changed")
			}
		})
	}
}

func TestInitializationFailureOrMissingEvidenceIsNotPrepared(t *testing.T) {
	for _, failure := range []error{nil, errors.New("backend unavailable")} {
		t.Run(errorText(failure), func(t *testing.T) {
			root := t.TempDir()
			f := initRunner(t, root, func(c invocation) (string, error) { return "", failure })
			got, err := initialize(context.Background(), root, []string{"beads"}, true, f)
			if err != nil || got[0].Prepared || got[0].Status != "failed" || !got[0].Available {
				t.Fatalf("got %#v %v", got, err)
			}
		})
	}
}

func TestInvalidSerenaConfigIsNotReportedPrepared(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, ".serena/project.yml", "project_name:\nlanguage_servers:\n- go\n")
	f := initRunner(t, root, func(c invocation) (string, error) { t.Fatalf("unexpected command %#v", c); return "", nil })
	got, err := initialize(context.Background(), root, []string{"serena"}, true, f)
	if err != nil || got[0].Prepared || got[0].Status != "needs_consent" {
		t.Fatalf("invalid config reused %#v %v", got, err)
	}
}

func TestModifiedOwnedGraphIsPreserved(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "main.go", "package fixture\n")
	extracts := 0
	f := initRunner(t, root, func(c invocation) (string, error) {
		if c.Path == "/tools/git" {
			return strings.Repeat("a", 40), nil
		}
		extracts++
		writeGraphFixture(t, root, c, "graph.json", `{"nodes":[],"edges":[]}`)
		return "", nil
	})
	got, err := initialize(context.Background(), root, []string{"graphify"}, true, f)
	if err != nil || !got[0].Prepared {
		t.Fatalf("got %#v %v", got, err)
	}
	custom := `{"nodes":[{"id":"custom"}],"edges":[]}`
	writeFixture(t, root, "graphify-out/graph.json", custom)
	got, err = initialize(context.Background(), root, []string{"graphify"}, true, f)
	data, _ := os.ReadFile(filepath.Join(root, "graphify-out", "graph.json"))
	if err != nil || got[0].Prepared || extracts != 1 || string(data) != custom {
		t.Fatalf("modified graph not preserved %#v %v", got, err)
	}
}

func TestGraphHEADChangeDuringExtractionDoesNotCertifyGraph(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "main.go", "package fixture\n")
	revision := strings.Repeat("a", 40)
	f := initRunner(t, root, func(c invocation) (string, error) {
		if c.Path == "/tools/git" {
			return revision, nil
		}
		writeGraphFixture(t, root, c, "graph.json", `{"nodes":[],"links":[]}`)
		revision = strings.Repeat("b", 40)
		return "", nil
	})
	got, err := initialize(context.Background(), root, []string{"graphify"}, true, f)
	if err != nil || got[0].Prepared || got[0].Status != "failed" {
		t.Fatalf("moving HEAD certified %#v %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".agent-team", "dependencies", "prepared", "graphify.json")); !os.IsNotExist(err) {
		t.Fatal("wrote stale receipt")
	}
}

func TestSymlinkedProjectConfigurationIsNotReused(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFixture(t, outside, "project.yml", "project_name: elsewhere\nlanguages: [go]\n")
	if err := os.Symlink(outside, filepath.Join(root, ".serena")); err != nil {
		t.Skip(err)
	}
	f := initRunner(t, root, func(c invocation) (string, error) { t.Fatalf("unexpected command %#v", c); return "", nil })
	got, err := initialize(context.Background(), root, []string{"serena"}, true, f)
	if err != nil || got[0].Prepared || got[0].Status != "needs_consent" {
		t.Fatalf("redirected state reused %#v %v", got, err)
	}
}

func TestGraphifyOutputEnvironmentIsBoundToProject(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "main.go", "package fixture\n")
	t.Setenv("GRAPHIFY_OUT", t.TempDir())
	f := initRunner(t, root, func(c invocation) (string, error) {
		if c.Path == "/tools/git" {
			return strings.Repeat("a", 40), nil
		}
		if !strings.HasPrefix(c.Env["GRAPHIFY_OUT"], filepath.Join(root, ".agent-team", "dependencies", "prepared")+string(os.PathSeparator)) {
			t.Fatalf("Graphify inherits untrusted output path: %#v", c.Env)
		}
		writeGraphFixture(t, root, c, "graph.json", `{"nodes":[],"links":[]}`)
		return "", nil
	})
	got, err := initialize(context.Background(), root, []string{"graphify"}, true, f)
	if err != nil || !got[0].Prepared {
		t.Fatalf("got %#v %v", got, err)
	}
}

func TestGraphifyChangedOrRedirectedSidecarsPreventRefresh(t *testing.T) {
	for _, sidecar := range []string{"manifest.json", ".graphify_analysis.json", ".graphify_root", ".graphify_build_config.json", "cache/ast.json"} {
		for _, redirect := range []bool{false, true} {
			t.Run(sidecar+"/"+map[bool]string{false: "modified", true: "symlink"}[redirect], func(t *testing.T) {
				root := t.TempDir()
				writeFixture(t, root, "main.go", "package fixture\n")
				revision := strings.Repeat("a", 40)
				extracts := 0
				f := initRunner(t, root, func(c invocation) (string, error) {
					if c.Path == "/tools/git" {
						return revision, nil
					}
					extracts++
					writeGraphFixture(t, root, c, "graph.json", `{"nodes":[],"links":[]}`)
					writeGraphFixture(t, root, c, sidecar, "owned artifact")
					return "", nil
				})
				got, err := initialize(context.Background(), root, []string{"graphify"}, true, f)
				if err != nil || !got[0].Prepared {
					t.Fatalf("got %#v %v", got, err)
				}
				path := filepath.Join(root, "graphify-out", filepath.FromSlash(sidecar))
				outside := filepath.Join(t.TempDir(), "keep")
				if err := os.WriteFile(outside, []byte("external bytes"), 0600); err != nil {
					t.Fatal(err)
				}
				if redirect {
					if err := os.Remove(path); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(outside, path); err != nil {
						t.Skip(err)
					}
				} else {
					writeFixture(t, root, "graphify-out/"+sidecar, "custom bytes")
				}
				revision = strings.Repeat("b", 40)
				got, err = initialize(context.Background(), root, []string{"graphify"}, true, f)
				if err != nil || got[0].Prepared || extracts != 1 || got[0].Status != "needs_consent" {
					t.Fatalf("unsafe refresh %#v %v extracts=%d", got, err, extracts)
				}
				data, _ := os.ReadFile(outside)
				if string(data) != "external bytes" {
					t.Fatal("external file changed")
				}
			})
		}
	}
}

func writeGraphFixture(t *testing.T, root string, c invocation, rel, content string) {
	t.Helper()
	out := c.Env["GRAPHIFY_OUT"]
	if out == "" {
		t.Fatal("extraction did not bind GRAPHIFY_OUT")
	}
	within, err := filepath.Rel(root, out)
	if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(os.PathSeparator)) {
		t.Fatalf("unscoped graph output %s", out)
	}
	writeFixture(t, root, filepath.Join(within, rel), content)
}

func TestFailedGraphExtractionPreservesPublishedGraphAndCanRetry(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "main.go", "package fixture\n")
	revision := strings.Repeat("a", 40)
	fail := false
	f := initRunner(t, root, func(c invocation) (string, error) {
		if c.Path == "/tools/git" {
			return revision, nil
		}
		writeGraphFixture(t, root, c, "graph.json", `{"nodes":[],"links":[]}`)
		if fail {
			writeGraphFixture(t, root, c, "manifest.json", "partial")
			return "", errors.New("extract failed")
		}
		return "", nil
	})
	got, err := initialize(context.Background(), root, []string{"graphify"}, true, f)
	if err != nil || !got[0].Prepared {
		t.Fatalf("got %#v %v", got, err)
	}
	oldReceipt, _ := os.ReadFile(filepath.Join(root, ".agent-team", "dependencies", "prepared", "graphify.json"))
	revision = strings.Repeat("b", 40)
	fail = true
	got, err = initialize(context.Background(), root, []string{"graphify"}, true, f)
	if err != nil || got[0].Prepared || got[0].Status != "failed" {
		t.Fatalf("failed extraction accepted %#v %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "graphify-out", "manifest.json")); !os.IsNotExist(err) {
		t.Fatal("partial extraction replaced live graph")
	}
	receipt, _ := os.ReadFile(filepath.Join(root, ".agent-team", "dependencies", "prepared", "graphify.json"))
	if string(receipt) != string(oldReceipt) {
		t.Fatal("failed extraction changed receipt")
	}
	fail = false
	got, err = initialize(context.Background(), root, []string{"graphify"}, true, f)
	if err != nil || !got[0].Prepared {
		t.Fatalf("retry did not recover %#v %v", got, err)
	}
}

func TestGraphSourceEditsInvalidateWithoutHEADChange(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "main.go", "package first\n")
	extracts := 0
	f := initRunner(t, root, func(c invocation) (string, error) {
		if c.Path == "/tools/git" {
			return strings.Repeat("a", 40), nil
		}
		extracts++
		writeGraphFixture(t, root, c, "graph.json", `{"nodes":[],"links":[]}`)
		return "", nil
	})
	got, err := initialize(context.Background(), root, []string{"graphify"}, true, f)
	if err != nil || !got[0].Prepared {
		t.Fatalf("got %#v %v", got, err)
	}
	writeFixture(t, root, "main.go", "package changed\n")
	got, err = initialize(context.Background(), root, []string{"graphify"}, false, f)
	if err != nil || got[0].Prepared || got[0].Status != "needs_consent" || extracts != 1 {
		t.Fatalf("dirty graph reused %#v %v", got, err)
	}
	got, err = initialize(context.Background(), root, []string{"graphify"}, true, f)
	if err != nil || !got[0].Prepared || extracts != 2 {
		t.Fatalf("dirty graph not refreshed %#v %v", got, err)
	}
	writeFixture(t, root, ".agent-team/generated-note", "ignore generated state")
	got, err = initialize(context.Background(), root, []string{"graphify"}, false, f)
	if err != nil || !got[0].Prepared {
		t.Fatalf("generated file invalidates graph %#v %v", got, err)
	}
	writeFixture(t, root, "new-untracked.go", "package added\n")
	got, err = initialize(context.Background(), root, []string{"graphify"}, false, f)
	if err != nil || got[0].Prepared {
		t.Fatalf("new source not detected %#v %v", got, err)
	}
}

func writeFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
