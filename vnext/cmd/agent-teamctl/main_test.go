package main

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"

	releasepkg "github.com/thebpandey/agent-team/vnext/internal/release"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

func TestLocalReleaseRequiresPackagedProvenance(t *testing.T) {
	root, executable := packagedDirectory(t)
	rel, err := localReleaseFrom(root, executable, "1.0.0")
	if err != nil || rel.Version != "1.0.0" || rel.Revision != testRevision {
		t.Fatal(rel, err)
	}
	for name, mutate := range map[string]func(string){
		"tampered source": func(root string) { writeTestFile(t, filepath.Join(root, "WORKER-CONTRACT"), "tampered") },
		"missing manifest": func(root string) {
			if err := os.Remove(filepath.Join(root, "RELEASE.json")); err != nil {
				t.Fatal(err)
			}
		},
		"missing checksums": func(root string) {
			if err := os.Remove(filepath.Join(root, "SHA256SUMS")); err != nil {
				t.Fatal(err)
			}
		},
		"extra path": func(root string) { writeTestFile(t, filepath.Join(root, "extra"), "unexpected") },
		"symlink source": func(root string) {
			path := filepath.Join(root, "WORKER-CONTRACT")
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "VERSION"), path); err != nil {
				t.Fatal(err)
			}
		},
		"wrong revision": func(root string) {
			replaceTestBytes(t, filepath.Join(root, "RELEASE.json"), []byte(testRevision), []byte("not-a-release-revision------------------"))
		},
	} {
		t.Run(name, func(t *testing.T) {
			copyRoot, executable := packagedDirectory(t)
			mutate(copyRoot)
			if _, err := localReleaseFrom(copyRoot, executable, "1.0.0"); err == nil {
				t.Fatal("invalid package accepted")
			}
		})
	}
	if _, err := localReleaseFrom(root, executable, "2.0.0"); err == nil {
		t.Fatal("wrong requested version accepted")
	}
}

func packagedDirectory(t *testing.T) (string, string) {
	t.Helper()
	source := t.TempDir()
	writeTestFile(t, filepath.Join(source, "agent-teamctl"), "binary")
	writeTestFile(t, filepath.Join(source, "WORKER-CONTRACT"), "contract")
	writeTestFile(t, filepath.Join(source, "codex", "SKILL.md"), "codex")
	writeTestFile(t, filepath.Join(source, "claude", "SKILL.md"), "claude")
	writeTestFile(t, filepath.Join(source, "VERSION"), "1.0.0\n")
	output := t.TempDir()
	if err := releasepkg.BuildReleasePackageFrom(source, output, "1.0.0", testRevision); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.OpenReader(filepath.Join(output, "agent-teamctl-1.0.0.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	for _, member := range archive.File {
		reader, err := member.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(reader)
		_ = reader.Close()
		if err != nil {
			t.Fatal(err)
		}
		writeTestFile(t, filepath.Join(output, filepath.FromSlash(member.Name)), string(body))
	}
	return output, filepath.Join(output, "agent-teamctl")
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func replaceTestBytes(t *testing.T, path string, old, replacement []byte) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index+len(old) <= len(body); index++ {
		match := true
		for offset := range old {
			if body[index+offset] != old[offset] {
				match = false
				break
			}
		}
		if match {
			body = append(append(append([]byte(nil), body[:index]...), replacement...), body[index+len(old):]...)
			if err := os.WriteFile(path, body, 0o644); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("test bytes not found")
}
