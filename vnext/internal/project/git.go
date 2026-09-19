package project

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

// Discover returns canonical Git identity and non-mutating filesystem
// preflight facts for root. It never changes the worktree or index.
func Discover(ctx context.Context, root string) (Project, error) {
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
		return Project{}, err
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
	info, err := os.Stat(canonicalRoot)
	if err != nil {
		return Project{}, fmt.Errorf("%w: stat project root: %v", core.ErrPath, err)
	}
	mode := info.Mode().Perm()
	return Project{
		Root: canonicalRoot, TopLevel: topLevel, CommonDir: commonDir, Head: head,
		Dirty: strings.TrimSpace(status) != "", Detached: detached,
		Readable: mode&0o444 != 0, Writable: mode&0o222 != 0,
		FreeBytes: freeBytes(ctx, canonicalRoot),
	}, nil
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
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("%w: git %s: %v", core.ErrGit, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output)), nil
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
