package preparation

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Corrupt archives must never produce an executable, even after consent.
func TestBeadsDownloadVerifiesChecksumBeforeWriting(t *testing.T) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	payload := []byte("native binary fixture")
	if err := tw.WriteHeader(&tar.Header{Name: "bd", Mode: 0755, Size: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}
	_, _ = tw.Write(payload)
	_ = tw.Close()
	_ = gz.Close()
	archive := buf.Bytes()
	for _, valid := range []bool{false, true} {
		t.Run(fmt.Sprint(valid), func(t *testing.T) {
			root := t.TempDir()
			dest := filepath.Join(root, "bd")
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Scheme != "https" || req.URL.Host != "github.com" || !strings.HasPrefix(req.URL.Path, "/gastownhall/beads/releases/download/v1.3.0/") {
					t.Fatalf("unexpected source %s", req.URL)
				}
				if _, ok := req.Context().Deadline(); !ok {
					t.Fatal("unbounded download")
				}
				data := archive
				if strings.HasSuffix(req.URL.Path, "checksums.txt") {
					sum := sha256.Sum256(archive)
					if !valid {
						sum[0]++
					}
					data = []byte(fmt.Sprintf("%x  beads_1.3.0_linux_amd64.tar.gz\n", sum))
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
			})}
			err := installBeads(context.Background(), dest, "linux", "amd64", nativeRunner{client: client})
			got, readErr := os.ReadFile(dest)
			if valid {
				if err != nil || readErr != nil || !bytes.Equal(got, payload) {
					t.Fatalf("got %q, %v, %v", got, err, readErr)
				}
			} else if err == nil || !os.IsNotExist(readErr) {
				t.Fatalf("corrupt archive accepted: %v, %v", err, readErr)
			}
		})
	}
}

func TestBeadsUnsupportedPlatformDoesNotDownload(t *testing.T) {
	f := &fakeRunner{}
	if err := installBeads(context.Background(), filepath.Join(t.TempDir(), "bd"), "plan9", "amd64", f); err == nil {
		t.Fatal("unsupported platform accepted")
	}
}

func TestBeadsWindowsArchiveExtractsOnlyExecutable(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("bd.exe")
	_, _ = f.Write([]byte("windows fixture"))
	f, _ = zw.Create("../unexpected")
	_, _ = f.Write([]byte("must not extract"))
	_ = zw.Close()
	archive := buf.Bytes()
	sum := sha256.Sum256(archive)
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		data := archive
		if strings.HasSuffix(req.URL.Path, "checksums.txt") {
			data = []byte(fmt.Sprintf("%x  beads_1.3.0_windows_amd64.zip\n", sum))
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), Header: make(http.Header)}, nil
	})}
	root := t.TempDir()
	dest := filepath.Join(root, "bd.exe")
	if err := installBeads(context.Background(), dest, "windows", "amd64", nativeRunner{client: client}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 1 || entries[0].Name() != "bd.exe" {
		t.Fatalf("unexpected extracted entries %v", entries)
	}
}

func TestDownloadRejectsUnapprovedHostsAndOversizeBodies(t *testing.T) {
	called := false
	n := nativeRunner{client: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		called = true
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("too large")), Header: make(http.Header)}, nil
	})}}
	if _, err := n.fetch(context.Background(), "https://example.com/beads", 100); err == nil || called {
		t.Fatal("unapproved source requested")
	}
	if _, err := n.fetch(context.Background(), beadsReleaseURL+"checksums.txt", 3); err == nil {
		t.Fatal("size bound ignored")
	}
}

func TestExistingPythonSymlinkDoesNotBlockAnotherToolInstall(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, ".agent-team", "dependencies")
	venv := filepath.Join(base, "uv-tools", "serena-agent", "bin")
	if err := os.MkdirAll(venv, 0700); err != nil {
		t.Fatal(err)
	}
	python := filepath.Join(t.TempDir(), "python3")
	if err := os.WriteFile(python, []byte("existing Python"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(python, filepath.Join(venv, "python")); err != nil {
		t.Skip(err)
	}
	if err := ensureDirectory(root, filepath.Join(base, "bin")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(python)
	if string(got) != "existing Python" {
		t.Fatal("changed Python")
	}
}

type fakeRunner struct {
	paths   map[string]string
	run     func(context.Context, invocation) (string, error)
	calls   []invocation
	fetched []string
}

func (f *fakeRunner) lookPath(name string) (string, error) {
	if p, ok := f.paths[name]; ok {
		return p, nil
	}
	return "", os.ErrNotExist
}
func (f *fakeRunner) fetch(_ context.Context, url string, _ int64) ([]byte, error) {
	f.fetched = append(f.fetched, url)
	return nil, errors.New("unexpected download")
}
func (f *fakeRunner) execute(ctx context.Context, c invocation) (string, error) {
	f.calls = append(f.calls, c)
	if f.run != nil {
		return f.run(ctx, c)
	}
	return "", errors.New("unexpected command")
}

// Removing the consent gate would create directories and execute an installer.
func TestInstallWithoutApprovalDoesNotMutate(t *testing.T) {
	root := t.TempDir()
	f := &fakeRunner{paths: map[string]string{"uv": "/tools/uv"}}
	got, err := prepare(context.Background(), root, []string{"serena"}, true, false, f)
	if err != nil || len(got) != 1 || got[0].Status != "needs_consent" || got[0].Available {
		t.Fatalf("got %#v, %v", got, err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("ran commands without approval: %#v", f.calls)
	}
	if len(f.fetched) != 0 {
		t.Fatal("downloaded dependencies before approval")
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("changed project: %v", entries)
	}
}

// An installer failure or missing executable must never become ready.
func TestInstallFailureIsNotAvailable(t *testing.T) {
	for _, installErr := range []error{errors.New("network unavailable"), nil} {
		t.Run(strings.ReplaceAll("error "+errorText(installErr), " ", "_"), func(t *testing.T) {
			f := &fakeRunner{paths: map[string]string{"uv": "/tools/uv"}, run: func(_ context.Context, c invocation) (string, error) { return "", installErr }}
			got, err := prepare(context.Background(), t.TempDir(), []string{"graphify"}, true, true, f)
			if err != nil || got[0].Available || got[0].Status != "failed" || got[0].Error == "" || got[0].Guidance == "" {
				t.Fatalf("got %#v, %v", got, err)
			}
		})
	}
}
func errorText(err error) string {
	if err == nil {
		return "nil"
	}
	return err.Error()
}

// A repeated approved install must probe the existing exact path, not reinstall.
func TestInstallThenReuseProjectExecutable(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, ".agent-team", "dependencies", "bin", executable("graphify"))
	f := &fakeRunner{paths: map[string]string{"uv": "/tools/uv"}}
	f.run = func(ctx context.Context, c invocation) (string, error) {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("command not bounded")
		}
		if c.Path == "/tools/uv" {
			if strings.Join(c.Args, " ") != "tool install --no-config --python 3.13 graphifyy==0.9.65" {
				t.Fatalf("unexpected install: %#v", c)
			}
			if c.Env["UV_TOOL_BIN_DIR"] != filepath.Dir(bin) || !strings.HasPrefix(c.Env["UV_TOOL_DIR"], root+string(os.PathSeparator)) {
				t.Fatalf("unscoped install: %#v", c)
			}
			return "", os.WriteFile(bin, []byte("fixture"), 0700)
		}
		if c.Path != bin || strings.Join(c.Args, " ") != "--version" {
			t.Fatalf("unexpected probe: %#v", c)
		}
		return "graphify 0.9.65", nil
	}
	first, err := prepare(context.Background(), root, []string{"graphify"}, true, true, f)
	if err != nil || first[0].Status != "installed" || !first[0].Available || first[0].Path != bin {
		t.Fatalf("got %#v, %v", first, err)
	}
	f.calls = nil
	again, err := prepare(context.Background(), root, []string{"graphify"}, true, true, f)
	if err != nil || again[0].Status != "available" || len(f.calls) != 1 {
		t.Fatalf("got %#v, %v; calls %#v", again, err, f.calls)
	}
}

// Probe failure must preserve the existing installation and report it unusable.
func TestUnhealthyProjectBeadsIsNotOverwritten(t *testing.T) {
	root := t.TempDir()
	local := filepath.Join(root, ".agent-team", "dependencies", "bin", executable("bd"))
	if err := os.MkdirAll(filepath.Dir(local), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(local, []byte("custom project binary"), 0700); err != nil {
		t.Fatal(err)
	}
	f := &fakeRunner{paths: map[string]string{"bd": "/selected/bd"}, run: func(_ context.Context, c invocation) (string, error) {
		if c.Path != local || strings.Join(c.Args, " ") != "version" {
			t.Fatalf("unexpected command %#v", c)
		}
		return "", errors.New("broken executable")
	}}
	got, err := prepare(context.Background(), root, []string{"beads"}, true, true, f)
	if err != nil || got[0].Status != "failed" || got[0].Available || got[0].Path != local || len(f.calls) != 1 {
		t.Fatalf("got %#v, %v", got, err)
	}
	data, _ := os.ReadFile(local)
	if string(data) != "custom project binary" {
		t.Fatal("overwrote an existing dependency")
	}
}

func TestUnknownSelectionRejectedBeforeAnyCommands(t *testing.T) {
	f := &fakeRunner{paths: map[string]string{"bd": "/selected/bd"}}
	_, err := prepare(context.Background(), t.TempDir(), []string{"beads", "curl | sh"}, true, true, f)
	if err == nil || len(f.calls) != 0 {
		t.Fatalf("err %v, calls %#v", err, f.calls)
	}
}

func TestMissingPrerequisiteAndOptionalToolsKeepFallback(t *testing.T) {
	f := &fakeRunner{}
	got, err := prepare(context.Background(), t.TempDir(), []string{"serena", "rg", "ast-grep", "lean-ctx"}, true, true, f)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range got {
		if d.Available || d.Status != "failed" || d.Fallback == "" || d.Guidance == "" {
			t.Fatalf("misleading dependency: %#v", d)
		}
	}
	if len(f.calls) != 0 {
		t.Fatal("ran missing prerequisite")
	}
}

func TestSymlinkedInstallDirectoryRejected(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(root, ".agent-team")); err != nil {
		t.Skip(err)
	}
	f := &fakeRunner{paths: map[string]string{"uv": "/tools/uv"}}
	got, err := prepare(context.Background(), root, []string{"graphify"}, true, true, f)
	if err != nil || got[0].Status != "failed" || len(f.calls) != 0 {
		t.Fatalf("got %#v, %v", got, err)
	}
	entries, _ := os.ReadDir(elsewhere)
	if len(entries) != 0 {
		t.Fatal("wrote outside project")
	}
}

func TestApprovedPythonInstallScopesRuntimeAndDisablesSharedRegistration(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, ".agent-team", "dependencies")
	f := &fakeRunner{paths: map[string]string{"uv": "/tools/uv"}}
	f.run = func(ctx context.Context, c invocation) (string, error) {
		for key, want := range map[string]string{
			"UV_PYTHON_INSTALL_DIR":      filepath.Join(base, "python"),
			"UV_PYTHON_BIN_DIR":          filepath.Join(base, "python-bin"),
			"UV_PYTHON_DOWNLOADS":        "automatic",
			"UV_PYTHON_INSTALL_BIN":      "false",
			"UV_PYTHON_INSTALL_REGISTRY": "false",
			"UV_NO_MODIFY_PATH":          "1",
		} {
			if c.Env[key] != want {
				t.Fatalf("unscoped Python preparation %s=%q, want %q", key, c.Env[key], want)
			}
		}
		return "", errors.New("fixture stops before real install")
	}
	got, err := prepare(context.Background(), root, []string{"serena"}, true, true, f)
	if err != nil || got[0].Status != "failed" || len(f.calls) != 1 {
		t.Fatalf("got %#v %v", got, err)
	}
	if !strings.Contains(strings.Join(got[0].Prerequisites, " "), "project") {
		t.Fatal("consent inventory omits runtime scope")
	}
}

func TestApprovedPythonToolReusesProjectUV(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, ".agent-team", "dependencies")
	uv := filepath.Join(base, "bin", executable("uv"))
	if err := os.MkdirAll(filepath.Dir(uv), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(uv, []byte("fixture uv"), 0700); err != nil {
		t.Fatal(err)
	}
	f := &fakeRunner{}
	f.run = func(_ context.Context, c invocation) (string, error) {
		if c.Path != uv {
			t.Fatalf("did not reuse scoped uv %#v", c)
		}
		if strings.Join(c.Args, " ") == "--version" {
			return "uv 0.12.17", nil
		}
		return "", errors.New("fixture tool installation stopped")
	}
	got, err := prepare(context.Background(), root, []string{"serena"}, true, true, f)
	if err != nil || got[0].Status != "failed" || len(f.calls) != 2 || len(f.fetched) != 0 {
		t.Fatalf("got %#v %v; calls %#v", got, err, f.calls)
	}
}

func TestMissingUVDownloadsOnlyAfterApproval(t *testing.T) {
	root := t.TempDir()
	f := &fakeRunner{}
	got, err := prepare(context.Background(), root, []string{"serena"}, true, false, f)
	if err != nil || got[0].Status != "needs_consent" || len(f.fetched) != 0 {
		t.Fatalf("download before approval %#v %v", got, err)
	}
	got, err = prepare(context.Background(), root, []string{"serena"}, true, true, f)
	if err != nil || got[0].Status != "failed" || len(f.fetched) != 1 || !strings.Contains(f.fetched[0], "github.com/astral-sh/uv/releases/download/") {
		t.Fatalf("uv not prepared %#v %v fetched=%v", got, err, f.fetched)
	}
	if len(f.calls) != 0 {
		t.Fatal("ran uv after download failed")
	}
}

func TestCommandDiagnosticsKeepFailureReasonAndRedactCredentials(t *testing.T) {
	got := commandDiagnostic("error: authentication abc-secret-value rejected at https://person:password@example.com/package\n"+strings.Repeat("x", 2000), []string{"API_TOKEN=abc-secret-value", "NORMAL_SETTING=authentication"})
	if strings.Contains(got, "abc-secret-value") || strings.Contains(got, "person:password") || !strings.Contains(got, "authentication") || len(got) > 1100 {
		t.Fatalf("unsafe or unusable diagnostic %q", got)
	}
}
