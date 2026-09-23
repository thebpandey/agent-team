package install

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFreshInstallUsesDiscoverableEntrypoints(t *testing.T) {
	root := t.TempDir()
	layout, err := ResolveLayout("linux", map[string]string{"HOME": root, "XDG_DATA_HOME": filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	release := legacyReleaseFixture(t, root)
	result, err := Install(context.Background(), layout, release, []Host{Codex, Claude}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range result.Manifest.Files {
		if file.Role == EntrypointRole && file.Path != filepath.Join(layout.SkillRoots[file.Host], "SKILL.md") {
			t.Fatalf("undiscoverable entrypoint: %s", file.Path)
		}
	}
}

func TestHistoricalEntrypointRollbackAndDirectUninstall(t *testing.T) {
	for _, operation := range []string{"rollback", "uninstall"} {
		for _, moved := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "/nested", true: "/moved"}[moved], func(t *testing.T) {
				layout, first, second, current := installedFixture(t)
				updated, err := Update(context.Background(), layout, second, current.Revision)
				if err != nil {
					t.Fatal(err)
				}
				current = stageHistoricalEntrypoints(t, layout)
				top := filepath.Join(layout.SkillRoots[Codex], "SKILL.md")
				if moved {
					if err := os.Rename(nestedEntrypoint(layout, Codex), top); err != nil {
						t.Fatal(err)
					}
				}
				if operation == "rollback" {
					rolled, err := Rollback(context.Background(), layout, first.Version, updated.Manifest.Revision)
					if err != nil {
						t.Fatal(err)
					}
					assertFileDigest(t, top, first.Entrypoints[Codex].SHA256)
					current = rolled.Manifest
				}
				retained, _, err := Uninstall(context.Background(), layout, current.Revision)
				if err != nil || len(retained) != 0 {
					t.Fatalf("uninstall: %v %v", retained, err)
				}
				for _, path := range []string{top, nestedEntrypoint(layout, Codex)} {
					if _, err := os.Lstat(path); !os.IsNotExist(err) {
						t.Fatalf("entrypoint remains: %s %v", path, err)
					}
				}
			})
		}
	}
}

func TestDiscoveryMigrationPreservesUnknownDestinations(t *testing.T) {
	for _, host := range []Host{Codex, Claude} {
		for _, sourcePresent := range []bool{false, true} {
			t.Run(string(host)+map[bool]string{false: "/missing source", true: "/nested source"}[sourcePresent], func(t *testing.T) {
				layout, first := internalFixture(t, "first")
				_, second := internalFixtureAt(t, filepath.Join(filepath.Dir(layout.DataRoot), "second"), "second")
				second.Version, second.Revision = "2.0.0", "abcdef0123456789abcdef0123456789abcdef01"
				if _, err := Install(context.Background(), layout, first, []Host{host}, 0); err != nil {
					t.Fatal(err)
				}
				current := stageHistoricalEntrypoints(t, layout)
				if !sourcePresent {
					if err := os.Remove(nestedEntrypoint(layout, host)); err != nil {
						t.Fatal(err)
					}
				}
				top := filepath.Join(layout.SkillRoots[host], "SKILL.md")
				if err := os.WriteFile(top, []byte("user edited skill"), 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := Update(context.Background(), layout, second, current.Revision); err == nil {
					t.Fatal("unknown destination accepted")
				}
				body, err := os.ReadFile(top)
				if err != nil || string(body) != "user edited skill" {
					t.Fatalf("foreign file changed: %s %v", body, err)
				}
				got, err := NewManifestStore(layout).Read(context.Background())
				if err != nil || !reflect.DeepEqual(got, current) {
					t.Fatal("manifest changed on discovery conflict")
				}
				assertFileDigest(t, layout.BinaryPath, first.Binary.SHA256)
			})
		}
	}
}

func TestDiscoveryMigrationRecoversInterruptedUpdate(t *testing.T) {
	for _, moved := range []bool{false, true} {
		t.Run(map[bool]string{false: "nested", true: "moved"}[moved], func(t *testing.T) {
			layout, _, second, _ := installedFixture(t)
			current := stageHistoricalEntrypoints(t, layout)
			if moved {
				if err := os.Rename(nestedEntrypoint(layout, Codex), filepath.Join(layout.SkillRoots[Codex], "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			}
			stopAfter := 6 // Root replacement and removal of the nested entrypoint.
			if moved {
				stopAfter = 5 // The moved root has been replaced with the new release.
			}
			lifecycleInterruptHook = func(operation string, index int) bool { return operation == "update" && index == stopAfter }
			t.Cleanup(func() { lifecycleInterruptHook = nil })
			if _, err := Update(context.Background(), layout, second, current.Revision); err == nil {
				t.Fatal("interruption not injected")
			}
			lifecycleInterruptHook = nil
			result, err := Update(context.Background(), layout, second, current.Revision)
			if err != nil {
				t.Fatal(err)
			}
			if err := verifyManifestFiles(result.Manifest, nil); err != nil {
				t.Fatal(err)
			}
			assertFileDigest(t, filepath.Join(layout.SkillRoots[Codex], "SKILL.md"), second.Entrypoints[Codex].SHA256)
		})
	}
}

// Model the staging layout written by releases before native discovery was fixed.
func stageHistoricalEntrypoints(t *testing.T, layout Layout) InstallManifest {
	t.Helper()
	manifest, err := NewManifestStore(layout).Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for i, file := range manifest.Files {
		if file.Role != EntrypointRole {
			continue
		}
		nested := filepath.Join(layout.SkillRoots[file.Host], "agent-team-vnext", "SKILL.md")
		if file.Path == nested {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(nested), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(file.Path, nested); err != nil {
			t.Fatal(err)
		}
		manifest.Files[i].Path = nested
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(layout.ManifestPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func TestUpdateMigratesHistoricalAndManuallyMovedEntrypoints(t *testing.T) {
	for _, moved := range []bool{false, true} {
		t.Run(map[bool]string{false: "nested", true: "manually moved"}[moved], func(t *testing.T) {
			root := t.TempDir()
			layout, _ := ResolveLayout("linux", map[string]string{"HOME": root, "XDG_DATA_HOME": filepath.Join(root, "data")})
			release := legacyReleaseFixture(t, root)
			if _, err := Install(context.Background(), layout, release, []Host{Codex, Claude}, 0); err != nil {
				t.Fatal(err)
			}
			manifest := stageHistoricalEntrypoints(t, layout)
			if moved {
				for _, file := range manifest.Files {
					if file.Role == EntrypointRole {
						if err := os.Rename(file.Path, filepath.Join(layout.SkillRoots[file.Host], "SKILL.md")); err != nil {
							t.Fatal(err)
						}
					}
				}
			}
			result, err := Update(context.Background(), layout, release, manifest.Revision)
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range result.Manifest.Files {
				if file.Role != EntrypointRole {
					continue
				}
				if file.Path != filepath.Join(layout.SkillRoots[file.Host], "SKILL.md") {
					t.Fatalf("stale ownership: %s", file.Path)
				}
				assertFileDigest(t, file.Path, file.SHA256)
				if _, err := os.Stat(filepath.Join(layout.SkillRoots[file.Host], "agent-team-vnext", "SKILL.md")); !os.IsNotExist(err) {
					t.Fatal("nested entrypoint retained")
				}
			}
			retained, _, err := Uninstall(context.Background(), layout, result.Manifest.Revision)
			if err != nil || len(retained) != 0 {
				t.Fatalf("uninstall: %v, %v", retained, err)
			}
		})
	}
}
