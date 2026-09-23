package preparation

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

// Initialize prepares selected project state, without installing executables or
// registering host integrations. Prepared means project configuration (Beads or
// Serena) or an owned graph at HEAD (Graphify), not a live MCP/backend health gate.
// An empty selection uses DefaultNames and never implicitly selects Beads.
func Initialize(ctx context.Context, root string, names []string, approved bool) ([]Dependency, error) {
	return initialize(ctx, root, names, approved, nativeRunner{})
}

func initialize(ctx context.Context, root string, names []string, approved bool, run runner) ([]Dependency, error) {
	if len(names) == 0 {
		names = DefaultNames()
	}
	deps, err := prepare(ctx, root, names, false, false, run)
	if err != nil {
		return deps, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	for i := range deps {
		if err := ctx.Err(); err != nil {
			return deps, err
		}
		d := &deps[i]
		if !d.Available {
			continue
		}
		var release func() error
		if approved && (d.Name == "beads" || d.Name == "serena" || d.Name == "graphify") {
			guard, err := acquirePreparationLock(ctx, root, "initialize-"+d.Name)
			if err != nil {
				preparationFailure(d, err)
				continue
			}
			release = guard.Release
		}
		switch d.Name {
		case "beads", "serena":
			initializeConfig(ctx, root, d, approved, run)
		case "graphify":
			initializeGraph(ctx, root, d, approved, run)
		default:
			d.Prepared = true
			d.Guidance = "CLI requires no project initialization. " + d.Guidance
		}
		if release != nil {
			if err := release(); err != nil {
				preparationFailure(d, err)
			}
		}
	}
	return deps, nil
}

func initializeConfig(ctx context.Context, root string, d *Dependency, approved bool, run runner) {
	dir := ".beads"
	artifact := "metadata.json"
	if d.Name == "serena" {
		dir = ".serena"
		artifact = "project.yml"
	}
	path := filepath.Join(root, dir, artifact)
	limit := int64(1 << 20)
	if d.Name == "serena" {
		limit = 64 << 10
	}
	data, readErr := readProjectFile(root, path, limit)
	if readErr == nil && configured(d.Name, data) {
		if d.Name == "beads" {
			if err := probeBeadsBackend(ctx, root, d.Path, run); err != nil {
				recordUnsupportedFailure(root, d, approved, err)
				preparationFailure(d, err)
				return
			}
		}
		d.Prepared = true
		d.Guidance = "Existing project configuration reused at " + path + "; runtime/backend qualification remains separate."
		return
	}
	if _, err := os.Lstat(filepath.Join(root, dir)); err == nil || !os.IsNotExist(err) {
		retried := false
		if d.Name == "serena" && approved {
			cleanup, ok, retryErr := recoverSerenaPartial(root)
			if retryErr != nil {
				preparationFailure(d, retryErr)
				return
			}
			if ok {
				defer cleanup()
				retried = true
			}
		}
		if !retried {
			needsReview(d, "Existing "+dir+" is not recognized as a complete project configuration. Review or move it explicitly before retrying; setup preserves it.")
			return
		}
	}
	var languages []string
	if d.Name == "serena" {
		var err error
		languages, err = serenaLanguages(root)
		if errors.Is(err, errNoSource) {
			deferPreparation(d, "Project planning can continue. Add source files, then rerun setup to prepare Serena.")
			return
		}
		if err != nil {
			preparationFailure(d, err)
			return
		}
	}
	if !approved {
		needsReview(d, "Approve project initialization with setup --install "+d.Name+" --approve.")
		return
	}
	call := invocation{Path: d.Path, Dir: root, Env: map[string]string{}}
	if d.Name == "beads" {
		call.Args = []string{"init", "--skip-hooks", "--skip-agents", "--non-interactive", "--init-if-missing"}
		call.Env["BEADS_DIR"] = filepath.Join(root, ".beads")
	} else {
		home := filepath.Join(root, ".agent-team", "dependencies", "serena-home")
		if err := ensureDirectory(root, home); err != nil {
			preparationFailure(d, err)
			return
		}
		call.Args = []string{"project", "create", root, "--name", filepath.Base(root)}
		for _, language := range languages {
			call.Args = append(call.Args, "--language", language)
		}
		call.Env["SERENA_HOME"] = home
	}
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if d.Name == "serena" {
		defer func() {
			if !d.Prepared {
				_ = recordSerenaPartial(root)
			}
		}()
	}
	if _, err := run.execute(callCtx, call); err != nil {
		recordUnsupportedFailure(root, d, approved, err)
		preparationFailure(d, err)
		return
	}
	data, err := readProjectFile(root, path, limit)
	if err != nil {
		preparationFailure(d, fmt.Errorf("initializer did not produce readable project configuration: %w", err))
		return
	}
	if !configured(d.Name, data) {
		preparationFailure(d, errors.New("initializer did not produce recognized project configuration"))
		return
	}
	if d.Name == "beads" {
		if err := probeBeadsBackend(ctx, root, d.Path, run); err != nil {
			recordUnsupportedFailure(root, d, approved, err)
			preparationFailure(d, err)
			return
		}
	}
	d.Prepared = true
	d.Status = "prepared"
	d.Guidance = "Project configuration created at " + path + "; runtime/backend qualification remains separate."
}

func probeBeadsBackend(ctx context.Context, root, path string, run runner) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	output, err := run.execute(ctx, invocation{Path: path, Args: []string{"--readonly", "status", "--json", "--no-activity"}, Dir: root, Env: map[string]string{"BEADS_DIR": filepath.Join(root, ".beads")}, OutputLimit: 64 << 10})
	if err != nil {
		return fmt.Errorf("Beads backend probe failed: %w", err)
	}
	var status struct {
		Summary *struct {
			TotalIssues *int64 `json:"total_issues"`
		} `json:"summary"`
	}
	if json.Unmarshal([]byte(output), &status) != nil || status.Summary == nil || status.Summary.TotalIssues == nil || *status.Summary.TotalIssues < 0 {
		return errors.New("Beads backend probe returned no valid issue summary")
	}
	return nil
}

func configured(name string, data []byte) bool {
	if name == "serena" {
		if len(data) > 64<<10 {
			return false
		}
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		var config map[string]any
		if decoder.Decode(&config) != nil {
			return false
		}
		var trailing any
		if decoder.Decode(&trailing) != io.EOF {
			return false
		}
		project, ok := config["project_name"].(string)
		if !ok || strings.TrimSpace(project) == "" {
			return false
		}
		value, ok := config["language_servers"]
		if !ok {
			value = config["languages"]
		}
		languages, ok := value.([]any)
		if !ok || len(languages) == 0 {
			return false
		}
		for _, value := range languages {
			language, ok := value.(string)
			if !ok || strings.TrimSpace(language) == "" {
				return false
			}
		}
		if value, ok := config["read_only"]; ok {
			if _, ok := value.(bool); !ok {
				return false
			}
		}
		return true
	}
	var metadata map[string]json.RawMessage
	if json.Unmarshal(data, &metadata) != nil {
		return false
	}
	for _, key := range []string{"database", "backend", "dolt_database"} {
		var value string
		if json.Unmarshal(metadata[key], &value) == nil && value != "" {
			return true
		}
	}
	return false
}

type graphReceipt struct {
	Schema       int    `json:"schema"`
	Root         string `json:"root"`
	Revision     string `json:"revision"`
	Digest       string `json:"digest"`
	TreeDigest   string `json:"tree_digest"`
	SourceDigest string `json:"source_digest"`
	Path         string `json:"path"`
	Version      string `json:"version"`
}

func initializeGraph(ctx context.Context, root string, d *Dependency, approved bool, run runner) {
	graphDir := filepath.Join(root, "graphify-out")
	graphPath := filepath.Join(graphDir, "graph.json")
	receiptPath := filepath.Join(root, ".agent-team", "dependencies", "prepared", "graphify.json")
	var receipt graphReceipt
	receiptBytes, receiptErr := readProjectFile(root, receiptPath, 16<<10)
	if receiptErr == nil {
		receiptErr = json.Unmarshal(receiptBytes, &receipt)
	}
	graphBytes, graphErr := readProjectFile(root, graphPath, 64<<20)
	treeDigest, treeErr := graphTreeDigest(graphDir)
	owned := receiptErr == nil && receipt.Schema == 2 && receipt.Root == root && graphErr == nil && receipt.Digest == digest(graphBytes) && validGraph(graphBytes) && treeErr == nil && receipt.TreeDigest == treeDigest
	_, dirErr := os.Lstat(graphDir)
	if !os.IsNotExist(dirErr) && !owned {
		needsReview(d, "Existing graphify-out has no matching Agent-Team code-only receipt. Review or move it explicitly before retrying; setup preserves it.")
		return
	}
	git, err := run.lookPath("git")
	if err != nil {
		preparationFailure(d, errors.New("Git is required to bind Graphify preparation to HEAD"))
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	head, err := run.execute(probeCtx, invocation{Path: git, Args: []string{"rev-parse", "--verify", "HEAD"}, Dir: root})
	cancel()
	head = strings.TrimSpace(head)
	decoded, decodeErr := hex.DecodeString(head)
	if err != nil || decodeErr != nil || (len(decoded) != 20 && len(decoded) != 32) {
		deferPreparation(d, "Project planning can continue. Create a first Git commit, then rerun setup to prepare Graphify.")
		return
	}
	sourceDigest, potentialCode, err := graphSourceInventory(ctx, root, git, run)
	if err != nil {
		preparationFailure(d, err)
		return
	}
	if !potentialCode {
		deferPreparation(d, "Project planning can continue. Add source code, then rerun setup to prepare Graphify.")
		return
	}
	if owned && receipt.Revision == head && receipt.Path == d.Path && receipt.Version == d.Version && receipt.SourceDigest == sourceDigest {
		d.Prepared = true
		d.Guidance = "Existing code-only graph reused at Git HEAD " + head + " with matching source files."
		return
	}
	if !approved {
		needsReview(d, "Graphify preparation is stale for the current sources, HEAD or executable; approve a fresh code-only extraction.")
		return
	}
	if err := ensureDirectory(root, filepath.Dir(receiptPath)); err != nil {
		preparationFailure(d, err)
		return
	}
	stage, err := os.MkdirTemp(filepath.Dir(receiptPath), ".graphify-stage-")
	if err != nil {
		preparationFailure(d, err)
		return
	}
	defer os.RemoveAll(stage)
	// Extract into a private ordinary directory. Explicitly bind GRAPHIFY_OUT:
	// upstream honors this environment variable and follows output symlinks.
	callCtx, stop := context.WithTimeout(ctx, 10*time.Minute)
	defer stop()
	_, err = run.execute(callCtx, invocation{Path: d.Path, Args: []string{"extract", ".", "--code-only", "--no-viz"}, Dir: root, Env: map[string]string{"GRAPHIFY_QUERY_LOG_DISABLE": "1", "GRAPHIFY_OUT": stage}})
	if err != nil {
		recordUnsupportedFailure(root, d, approved, err)
		preparationFailure(d, err)
		return
	}
	graphBytes, err = readProjectFile(root, filepath.Join(stage, "graph.json"), 64<<20)
	if err != nil || !validGraph(graphBytes) {
		preparationFailure(d, errors.New("extractor did not produce a valid graph.json"))
		return
	}
	newTree, err := graphTreeDigest(stage)
	if err != nil {
		preparationFailure(d, err)
		return
	}
	// Do not certify a graph if HEAD moved while extraction was running.
	checkCtx, checkCancel := context.WithTimeout(ctx, 10*time.Second)
	after, err := run.execute(checkCtx, invocation{Path: git, Args: []string{"rev-parse", "--verify", "HEAD"}, Dir: root})
	checkCancel()
	if err != nil || strings.TrimSpace(after) != head {
		preparationFailure(d, errors.New("Git HEAD changed during extraction; rerun preparation"))
		return
	}
	afterSource, err := graphSourceDigest(ctx, root, git, run)
	if err != nil || afterSource != sourceDigest {
		preparationFailure(d, errors.New("source files changed during extraction; rerun preparation"))
		return
	}
	receipt = graphReceipt{Schema: 2, Root: root, Revision: head, Digest: digest(graphBytes), TreeDigest: newTree, SourceDigest: sourceDigest, Path: d.Path, Version: d.Version}
	data, _ := json.MarshalIndent(receipt, "", "  ")
	expectedTree := ""
	if owned {
		expectedTree = treeDigest
	}
	if err := publishGraph(stage, graphDir, expectedTree, receiptPath, receiptBytes, data); err != nil {
		preparationFailure(d, err)
		return
	}
	d.Prepared = true
	d.Status = "prepared"
	d.Guidance = "Offline code-only graph prepared at Git HEAD " + head + "; no host integration was registered."
}

func deferPreparation(d *Dependency, guidance string) {
	d.Status = "deferred"
	d.Deferred = true
	d.Prepared = false
	d.Error = ""
	d.Guidance = guidance
}

func validGraph(data []byte) bool {
	var graph map[string]json.RawMessage
	if json.Unmarshal(data, &graph) != nil {
		return false
	}
	var nodes []json.RawMessage
	if raw, ok := graph["nodes"]; !ok || json.Unmarshal(raw, &nodes) != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false
	}
	for _, key := range []string{"links", "edges"} {
		if raw, ok := graph[key]; ok {
			var edges []json.RawMessage
			return json.Unmarshal(raw, &edges) == nil && !bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
		}
	}
	return false
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func needsReview(d *Dependency, reason string) {
	d.Status = "needs_consent"
	d.Prepared = false
	d.Guidance = reason + " " + d.Fallback
}
func preparationFailure(d *Dependency, err error) {
	d.Status = "failed"
	d.Prepared = false
	d.Error = "Project preparation failed: " + err.Error()
	d.Guidance = "Resolve the project preparation failure, then rerun setup --install " + d.Name + " --approve. " + d.Fallback
}

// Refuse special files and redirected ancestor directories before reading state.
func readProjectFile(root, path string, limit int64) ([]byte, error) {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return nil, errors.New("project state is outside project root")
	}
	current := root
	parts := strings.Split(rel, string(os.PathSeparator))
	for i, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("project state path is a symlink: %s", current)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return nil, errors.New("project state parent is not a directory")
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return nil, errors.New("project state is not a regular file")
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readBounded(f, limit)
}

func writeReceipt(path string, data []byte) error {
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return errors.New("preparation receipt is not a regular file")
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".graphify-receipt-")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	_, writeErr := f.Write(data)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		return errors.Join(writeErr, closeErr)
	}
	return os.Rename(name, path)
}
