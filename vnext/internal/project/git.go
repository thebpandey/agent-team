package project

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

const maxGitOutputBytes = 1 << 20

// Discover returns canonical Git identity and non-mutating filesystem
// preflight facts for root. It never changes the worktree or index.
func Discover(ctx context.Context, root string) (Project, error) {
	// Project preflight is bounded even when the caller supplies no deadline.
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	canonicalRoot, err := canonicalDirectory(root)
	if err != nil {
		return Project{}, err
	}
	topLevel, err := git(ctx, canonicalRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return Project{}, err
	}
	topLevel, err = canonicalDirectory(topLevel)
	if err != nil {
		return Project{}, fmt.Errorf("%w: canonical top level: %v", core.ErrGit, err)
	}
	commonDir, err := git(ctx, canonicalRoot, "rev-parse", "--git-common-dir")
	if err != nil {
		return Project{}, err
	}
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(canonicalRoot, commonDir)
	}
	commonDir, err = canonicalDirectory(commonDir)
	if err != nil {
		return Project{}, fmt.Errorf("%w: canonical common directory: %v", core.ErrGit, err)
	}
	head, err := git(ctx, canonicalRoot, "rev-parse", "HEAD")
	if err != nil {
		// Setup is useful before a new project's first commit. Accept only an
		// actual unborn branch; detached or corrupt repositories still reject.
		branch, branchErr := git(ctx, canonicalRoot, "symbolic-ref", "-q", "HEAD")
		if branchErr != nil {
			return Project{}, err
		}
		probeErr := exec.CommandContext(ctx, "git", "-C", canonicalRoot, "show-ref", "--verify", "--quiet", branch).Run()
		if exit, ok := probeErr.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			return Project{}, err
		}
		head = ""
	}
	status, err := gitAllowEmpty(ctx, canonicalRoot, "status", "--porcelain=v1", "--untracked-files=all")
	if err != nil {
		return Project{}, err
	}
	branchCmd := exec.CommandContext(ctx, "git", "-C", canonicalRoot, "symbolic-ref", "-q", "--short", "HEAD")
	_, branchErr := branchCmd.Output()
	detached := branchErr != nil
	if detached {
		if exit, ok := branchErr.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
			return Project{}, fmt.Errorf("%w: determine branch: %v", core.ErrGit, branchErr)
		}
	}
	readable, writable := access(canonicalRoot)
	return Project{
		Root: canonicalRoot, TopLevel: topLevel, CommonDir: commonDir, Head: head,
		Dirty: strings.TrimSpace(status) != "", Detached: detached,
		Readable: readable, Writable: writable,
		FreeBytes: freeBytes(ctx, canonicalRoot),
	}, nil
}

func access(root string) (readable, writable bool) {
	if directory, err := os.Open(root); err == nil {
		_, readErr := directory.ReadDir(1)
		_ = directory.Close()
		readable = readErr == nil || readErr == io.EOF
	}
	// Discover is strictly read-only. A portable actual write probe requires a
	// mutation, so report writability conservatively until Initialize is allowed
	// to perform its owned preflight.
	return readable, writable
}

func git(ctx context.Context, root string, args ...string) (string, error) {
	value, err := gitAllowEmpty(ctx, root, args...)
	if err != nil {
		return "", err
	}
	if value == "" {
		return "", fmt.Errorf("%w: git %s returned an empty value", core.ErrGit, strings.Join(args, " "))
	}
	return value, nil
}

func gitAllowEmpty(ctx context.Context, root string, args ...string) (string, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	var output boundedOutput
	command.Stdout = &output
	command.Stderr = io.Discard
	err := command.Run()
	if output.overflow {
		return "", fmt.Errorf("%w: git %s output exceeds %d bytes", core.ErrLimit, strings.Join(args, " "), maxGitOutputBytes)
	}
	if err != nil {
		return "", fmt.Errorf("%w: git %s: %v", core.ErrGit, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(output.String()), nil
}

type boundedOutput struct {
	bytes.Buffer
	overflow bool
}

func (output *boundedOutput) Write(value []byte) (int, error) {
	if output.Len()+len(value) > maxGitOutputBytes {
		remaining := maxGitOutputBytes - output.Len()
		if remaining > 0 {
			_, _ = output.Buffer.Write(value[:remaining])
		}
		output.overflow = true
		return len(value), nil
	}
	return output.Buffer.Write(value)
}

func freeBytes(ctx context.Context, root string) int64 {
	if output, err := exec.CommandContext(ctx, "df", "-Pk", root).Output(); err == nil {
		lines := strings.Split(strings.TrimSpace(string(output)), "\n")
		if len(lines) >= 2 {
			fields := strings.Fields(lines[len(lines)-1])
			if len(fields) >= 4 {
				if available, err := strconv.ParseInt(fields[3], 10, 64); err == nil && available >= 0 {
					return available * 1024
				}
			}
		}
	}
	// fsutil is present on standard Windows installations. Its output has a
	// stable "Total # of free bytes" line, but failure remains explicit as -1.
	if output, err := exec.CommandContext(ctx, "fsutil", "volume", "diskfree", root).Output(); err == nil {
		for _, line := range strings.Split(string(output), "\n") {
			if !strings.Contains(strings.ToLower(line), "free bytes") {
				continue
			}
			fields := strings.FieldsFunc(line, func(r rune) bool { return r < '0' || r > '9' })
			for _, field := range fields {
				if value, err := strconv.ParseInt(field, 10, 64); err == nil {
					return value
				}
			}
		}
	}
	return -1
}
