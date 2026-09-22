package preparation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSerenaYAMLSyntaxAndTypesAreRequired(t *testing.T) {
	for _, data := range []string{"project_name: [\nlanguages: [\n", "project_name: []\nlanguages: [go]\n", "project_name: fixture\nlanguages: nope\n", "project_name: fixture\nlanguages: []\n", "project_name: fixture\nlanguages: [null]\n", "project_name: fixture\nlanguages: [go]\n---\nother: document\n"} {
		if configured("serena", []byte(data)) {
			t.Fatalf("malformed configuration accepted: %q", data)
		}
	}
	valid := "project_name: fixture\nlanguage_servers: [go]\ncustom:\n  nested: [one, two]\ninitial_prompt: |\n  Keep this custom text.\n"
	if !configured("serena", []byte(valid)) {
		t.Fatal("valid customized YAML rejected")
	}
}

func TestEmptyProjectPreparationIsDeferred(t *testing.T) {
	root := t.TempDir()
	f := initRunner(t, root, func(c invocation) (string, error) {
		if c.Path == "/tools/git" {
			return "", errors.New("no HEAD")
		}
		t.Fatalf("unexpected initializer %#v", c)
		return "", nil
	})
	got, err := initialize(context.Background(), root, []string{"serena", "graphify"}, true, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range got {
		if d.Status != "deferred" || !d.Deferred || d.Prepared || !d.Available || d.Error != "" {
			t.Fatalf("empty project treated as failure %#v", d)
		}
	}
	preview, err := initialize(context.Background(), root, []string{"serena", "graphify"}, false, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range preview {
		if !d.Deferred {
			t.Fatalf("preview blocks project planning %#v", d)
		}
	}
}

func TestCommittedDocumentOnlyGraphPreparationIsDeferred(t *testing.T) {
	for _, files := range [][]string{nil, {"README.md", "notes.txt"}} {
		for _, approved := range []bool{false, true} {
			root := t.TempDir()
			for _, name := range files {
				writeFixture(t, root, name, "Project plans\n")
			}
			f := initRunner(t, root, func(c invocation) (string, error) {
				if c.Path == "/tools/git" {
					return strings.Repeat("a", 40), nil
				}
				t.Fatalf("document-only project ran extractor: %#v", c)
				return "", nil
			})
			got, err := initialize(context.Background(), root, []string{"graphify"}, approved, f)
			if err != nil || !got[0].Deferred || got[0].Prepared || !got[0].Available || got[0].Error != "" {
				t.Fatalf("document-only project not deferred: %#v %v", got, err)
			}
		}
	}
}

func TestGraphSourceAndUnknownFileErrorsRemainFailures(t *testing.T) {
	for _, name := range []string{"main.go", "apm.yml", "script", "plugin.unknown"} {
		root := t.TempDir()
		writeFixture(t, root, name, "source fixture\n")
		extracts := 0
		f := initRunner(t, root, func(c invocation) (string, error) {
			if c.Path == "/tools/git" {
				return strings.Repeat("a", 40), nil
			}
			extracts++
			return "", errors.New("real extractor failure")
		})
		got, err := initialize(context.Background(), root, []string{"graphify"}, true, f)
		if err != nil || got[0].Deferred || got[0].Status != "failed" || extracts != 1 || !strings.Contains(got[0].Error, "real extractor failure") {
			t.Fatalf("%s extraction failure hidden: %#v %v", name, got, err)
		}
	}
}

func TestInterruptedOwnedSerenaCreationCanRetry(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "main.go", "package fixture\n")
	calls := 0
	f := initRunner(t, root, func(c invocation) (string, error) {
		calls++
		if calls == 1 {
			writeFixture(t, root, ".serena/partial.txt", "owned partial output")
			return "", errors.New("interrupted")
		}
		if _, err := os.Stat(filepath.Join(root, ".serena", "partial.txt")); !os.IsNotExist(err) {
			t.Fatal("partial state not isolated before retry")
		}
		writeFixture(t, root, ".serena/project.yml", "project_name: fixture\nlanguages: [go]\n")
		return "", nil
	})
	first, err := initialize(context.Background(), root, []string{"serena"}, true, f)
	if err != nil || first[0].Status != "failed" {
		t.Fatalf("first %#v %v", first, err)
	}
	second, err := initialize(context.Background(), root, []string{"serena"}, true, f)
	if err != nil || !second[0].Prepared || calls != 2 {
		t.Fatalf("retry %#v %v calls%d", second, err, calls)
	}
}

func TestChangedPartialSerenaOutputIsPreserved(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "main.go", "package fixture\n")
	calls := 0
	f := initRunner(t, root, func(c invocation) (string, error) {
		calls++
		writeFixture(t, root, ".serena/partial.txt", "owned partial")
		return "", errors.New("interrupted")
	})
	_, _ = initialize(context.Background(), root, []string{"serena"}, true, f)
	writeFixture(t, root, ".serena/partial.txt", "customized after failure")
	got, err := initialize(context.Background(), root, []string{"serena"}, true, f)
	if err != nil || got[0].Prepared || got[0].Status != "needs_consent" || calls != 1 {
		t.Fatalf("changed partial overwritten %#v %v", got, err)
	}
}

func TestApprovedBrokenGlobalCLIUsesPinnedProjectCopy(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, ".agent-team", "dependencies", "bin", executable("graphify"))
	f := &fakeRunner{paths: map[string]string{"graphify": "/global/graphify", "uv": "/tools/uv"}}
	f.run = func(ctx context.Context, c invocation) (string, error) {
		if c.Path == "/global/graphify" {
			return "", errors.New("incompatible installed version")
		}
		if c.Path == "/tools/uv" {
			return "", os.WriteFile(local, []byte("fixture"), 0700)
		}
		if c.Path == local {
			return "graphify 0.9.65", nil
		}
		t.Fatalf("unexpected command %#v", c)
		return "", nil
	}
	preview, err := prepare(context.Background(), root, []string{"graphify"}, true, false, f)
	if err != nil || preview[0].Status != "needs_consent" {
		t.Fatalf("repair preview %#v %v", preview, err)
	}
	got, err := prepare(context.Background(), root, []string{"graphify"}, true, true, f)
	if err != nil || !got[0].Available || got[0].Path != local || got[0].RepairSourcePath != "/global/graphify" {
		t.Fatalf("repair %#v %v", got, err)
	}
	for _, c := range f.calls {
		if c.Path == "/global/graphify" && strings.Join(c.Args, " ") != "--version" {
			t.Fatalf("mutated global CLI %#v", c)
		}
	}
}

func TestGraphFingerprintRecordsSymlinkWithoutFollowingExternalTarget(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.go")
	if err := os.WriteFile(outside, []byte("outside bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "alias.go")); err != nil {
		t.Skip(err)
	}
	f := &fakeRunner{run: func(context.Context, invocation) (string, error) { return "alias.go\x00", nil }}
	first, err := graphSourceDigest(context.Background(), root, "git", f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outside, []byte("changed outside bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := graphSourceDigest(context.Background(), root, "git", f)
	if err != nil || first != second {
		t.Fatalf("followed external target %q %q %v", first, second, err)
	}
	if err := os.Remove(filepath.Join(root, "alias.go")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("different.go", filepath.Join(root, "alias.go")); err != nil {
		t.Fatal(err)
	}
	third, err := graphSourceDigest(context.Background(), root, "git", f)
	if err != nil || third == first {
		t.Fatalf("link change not detected %v", err)
	}
}

func TestGraphFingerprintIncludesGitlinkAndCheckedOutSubmoduleSources(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "module/.git", "gitdir: ../.git/modules/module\n")
	writeFixture(t, root, "module/source.go", "package before\n")
	oid := strings.Repeat("a", 40)
	f := &fakeRunner{run: func(_ context.Context, c invocation) (string, error) {
		if c.Dir == filepath.Join(root, "module") {
			return "source.go\x00", nil
		}
		if len(c.Args) > 1 && c.Args[1] == "--stage" {
			return "160000 " + oid + " 0\tmodule\x00", nil
		}
		return "module\x00", nil
	}}
	first, err := graphSourceDigest(context.Background(), root, "git", f)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, root, "module/source.go", "package changed\n")
	second, err := graphSourceDigest(context.Background(), root, "git", f)
	if err != nil || first == second {
		t.Fatalf("submodule edit missed %v", err)
	}
	oid = strings.Repeat("b", 40)
	third, err := graphSourceDigest(context.Background(), root, "git", f)
	if err != nil || third == second {
		t.Fatalf("gitlink edit missed %v", err)
	}
}

func TestUnsupportedGlobalCommandAllowsApprovedScopedRepair(t *testing.T) {
	for _, unsupported := range []bool{false, true} {
		t.Run(map[bool]string{true: "unsupported", false: "project_failure"}[unsupported], func(t *testing.T) {
			root := t.TempDir()
			writeFixture(t, root, "main.go", "package fixture\n")
			local := filepath.Join(root, ".agent-team", "dependencies", "bin", executable("serena"))
			installs := 0
			f := &fakeRunner{paths: map[string]string{"serena": "/global/serena", "uv": "/tools/uv"}}
			f.run = func(_ context.Context, c invocation) (string, error) {
				if strings.Join(c.Args, " ") == "--version" {
					return "Serena 1.0.0", nil
				}
				if c.Path == "/tools/uv" {
					installs++
					return "", os.WriteFile(local, []byte("scoped fixture"), 0700)
				}
				if unsupported {
					return "", errors.New("No such option: --language")
				}
				return "", errors.New("permission denied writing configuration")
			}
			first, err := initialize(context.Background(), root, []string{"serena"}, true, f)
			if err != nil || first[0].Status != "failed" {
				t.Fatalf("first %#v %v", first, err)
			}
			got, err := prepare(context.Background(), root, []string{"serena"}, true, true, f)
			if err != nil {
				t.Fatal(err)
			}
			if unsupported {
				if installs != 1 || got[0].Path != local || got[0].RepairSourcePath != "/global/serena" {
					t.Fatalf("unsupported retry reused global %#v installs%d", got, installs)
				}
			} else if installs != 0 || got[0].Path != "/global/serena" {
				t.Fatalf("project error triggered reinstall %#v", got)
			}
		})
	}
}
