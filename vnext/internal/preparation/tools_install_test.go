package preparation

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type archiveFetcher struct {
	data  []byte
	calls int
}

func (f *archiveFetcher) fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	f.calls++
	if _, ok := ctx.Deadline(); !ok || limit != archiveLimit {
		return nil, errors.New("unbounded release download")
	}
	return f.data, nil
}

func fixtureArchive(t *testing.T, zipped bool, members []string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if zipped {
		z := zip.NewWriter(&buf)
		for _, member := range members {
			f, err := z.Create(member)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = f.Write([]byte("fixture executable"))
		}
		if err := z.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		gz := gzip.NewWriter(&buf)
		tr := tar.NewWriter(gz)
		for _, member := range members {
			data := []byte("fixture executable")
			if err := tr.WriteHeader(&tar.Header{Name: member, Mode: 0755, Size: int64(len(data))}); err != nil {
				t.Fatal(err)
			}
			_, _ = tr.Write(data)
		}
		if err := tr.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return buf.Bytes()
}

func TestPinnedArchiveInstallsOnlyFixedMemberAndPreservesExistingFiles(t *testing.T) {
	for _, zipped := range []bool{false, true} {
		name := "release.tar.gz"
		if zipped {
			name = "release.zip"
		}
		t.Run(name, func(t *testing.T) {
			data := fixtureArchive(t, zipped, []string{"../escape", "other", "package/tool"})
			archive := pinnedArchive{"https://example.test/" + name, "package/tool", digest(data)}
			root := t.TempDir()
			dest := filepath.Join(root, "tool")
			f := &archiveFetcher{data: data}
			if err := installPinnedArchive(context.Background(), dest, archive, f); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(dest)
			if err != nil || string(got) != "fixture executable" {
				t.Fatalf("binary %q: %v", got, err)
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 1 {
				t.Fatalf("unexpected archive files: %v", entries)
			}
			if err := os.WriteFile(dest, []byte("custom executable"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := installPinnedArchive(context.Background(), dest, archive, f); err == nil {
				t.Fatal("replaced existing file")
			}
			got, _ = os.ReadFile(dest)
			if string(got) != "custom executable" {
				t.Fatal("changed custom executable")
			}
		})
	}
}

func TestPinnedArchiveRejectsCorruptionAndAmbiguousMembers(t *testing.T) {
	for _, zipped := range []bool{false, true} {
		for _, kind := range []string{"checksum", "missing", "duplicate"} {
			t.Run(kind+map[bool]string{true: "/zip", false: "/tar"}[zipped], func(t *testing.T) {
				members := []string{"tool"}
				if kind == "missing" {
					members = []string{"other"}
				}
				if kind == "duplicate" {
					members = append(members, "tool")
				}
				data := fixtureArchive(t, zipped, members)
				url := "https://example.test/release.tar.gz"
				if zipped {
					url = "https://example.test/release.zip"
				}
				archive := pinnedArchive{url, "tool", digest(data)}
				if kind == "checksum" {
					archive.SHA256 = strings.Repeat("0", 64)
				}
				dest := filepath.Join(t.TempDir(), "tool")
				if err := installPinnedArchive(context.Background(), dest, archive, &archiveFetcher{data: data}); err == nil {
					t.Fatal("invalid archive accepted")
				}
				if _, err := os.Lstat(dest); !os.IsNotExist(err) {
					t.Fatalf("created executable: %v", err)
				}
			})
		}
	}
}

func TestPinnedArchiveRejectsExecutableSymlinks(t *testing.T) {
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	h := &zip.FileHeader{Name: "tool"}
	h.SetMode(os.ModeSymlink | 0777)
	f, _ := z.CreateHeader(h)
	_, _ = f.Write([]byte("/outside/tool"))
	_ = z.Close()
	a := pinnedArchive{"https://example.test/tool.zip", "tool", digest(buf.Bytes())}
	if _, err := singleArchiveMember(buf.Bytes(), a); err == nil {
		t.Fatal("symlink executable accepted")
	}
}

func TestReleaseCatalogHasPinnedDigestsAndExactURLAllowlist(t *testing.T) {
	for name, platforms := range toolArchives {
		for platform, a := range platforms {
			sum, err := hex.DecodeString(a.SHA256)
			if err != nil || len(sum) != 32 {
				t.Fatalf("bad pin %s/%s", name, platform)
			}
			if !allowedReleaseURL(a.URL) || allowedReleaseURL(a.URL+"?redirect=1") {
				t.Fatalf("inexact allowlist %s", a.URL)
			}
		}
		f := &archiveFetcher{}
		if err := installReleaseTool(context.Background(), name, filepath.Join(t.TempDir(), name), "plan9", "amd64", f); err == nil || f.calls != 0 {
			t.Fatal("unsupported platform downloaded")
		}
	}
}

type releaseProbeRunner struct {
	fakeRunner
	downloads int
}

func (f *releaseProbeRunner) fetch(context.Context, string, int64) ([]byte, error) {
	f.downloads++
	return nil, errors.New("fixture transport failure")
}

func TestOptionalReleaseInstallsRequireConsentAndReuseExistingTools(t *testing.T) {
	for _, name := range []string{"rg", "ast-grep", "lean-ctx"} {
		t.Run(name, func(t *testing.T) {
			f := &releaseProbeRunner{}
			root := t.TempDir()
			deps, err := prepare(context.Background(), root, []string{name}, true, false, f)
			if err != nil || deps[0].Status != "needs_consent" || f.downloads != 0 {
				t.Fatalf("unapproved install: %v %#v", err, deps)
			}
			entries, _ := os.ReadDir(root)
			if len(entries) != 0 {
				t.Fatal("wrote before consent")
			}
			deps, err = prepare(context.Background(), root, []string{name}, true, true, f)
			if err != nil || deps[0].Status != "failed" {
				t.Fatalf("failure hidden: %v %#v", err, deps)
			}
			_, supported := toolArchives[name][runtime.GOOS+"_"+runtime.GOARCH]
			if supported && f.downloads != 1 {
				t.Fatalf("approved install never attempted download: %#v", deps)
			}
			f.downloads = 0
			f.paths = map[string]string{name: filepath.Join(t.TempDir(), name)}
			f.run = func(context.Context, invocation) (string, error) { return name + " version fixture", nil }
			deps, err = prepare(context.Background(), t.TempDir(), []string{name}, true, true, f)
			if err != nil || !deps[0].Available || f.downloads != 0 {
				t.Fatalf("existing tool not reused: %v %#v", err, deps)
			}
		})
	}
}
