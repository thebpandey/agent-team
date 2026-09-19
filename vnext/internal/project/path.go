package project

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/thebpandey/agent-team/vnext/internal/core"
)

// Contain returns a canonical candidate path only when it is below root after
// resolving every existing symlink (including a Windows reparse-point target).
// The final candidate may not exist; its closest existing parent is resolved.
func Contain(root, candidate string) (string, error) {
	canonicalRoot, err := canonicalDirectory(root)
	if err != nil {
		return "", err
	}
	if candidate == "" || portableAbsolute(candidate) && !filepath.IsAbs(candidate) {
		return "", fmt.Errorf("%w: invalid candidate path %q", core.ErrPath, candidate)
	}
	abs, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("%w: absolute candidate: %v", core.ErrPath, err)
	}
	abs = filepath.Clean(abs)
	if err := validateCandidateSegments(canonicalRoot, abs); err != nil {
		return "", err
	}
	canonicalCandidate, err := resolveCandidate(abs)
	if err != nil {
		return "", err
	}
	if !contained(canonicalRoot, canonicalCandidate) {
		return "", fmt.Errorf("%w: %q escapes %q", core.ErrPath, candidate, root)
	}
	return canonicalCandidate, nil
}

func validateCandidateSegments(root, candidate string) error {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil // containment supplies the authoritative outside-root error.
	}
	current := root
	for _, segment := range strings.Split(filepath.ToSlash(relative), "/") {
		if segment == "." || segment == "" {
			continue
		}
		if err := ValidateSegment(segment); err != nil {
			return err
		}
		if entries, err := os.ReadDir(current); err == nil {
			for _, entry := range entries {
				if strings.EqualFold(entry.Name(), segment) && entry.Name() != segment {
					return fmt.Errorf("%w: case-folding alias %q", core.ErrPath, candidate)
				}
			}
		}
		current = filepath.Join(current, segment)
	}
	return nil
}

// ValidateSegment enforces a filename segment acceptable across supported
// platforms, including Windows device names and unsafe trailing characters.
func ValidateSegment(segment string) error {
	if segment == "" || segment == "." || segment == ".." || !utf8.ValidString(segment) {
		return fmt.Errorf("%w: invalid path segment %q", core.ErrPath, segment)
	}
	// The standard library intentionally has no Unicode normalization package.
	// Reject non-ASCII authority segments rather than accepting NFC/NFD aliases
	// that cannot be compared safely without a platform-dependent dependency.
	if strings.IndexFunc(segment, func(r rune) bool { return r > 0x7f }) >= 0 {
		return fmt.Errorf("%w: Unicode-normalization-ambiguous segment %q", core.ErrPath, segment)
	}
	if strings.ContainsAny(segment, `/\\<>:"|?*`) || strings.IndexFunc(segment, func(r rune) bool { return r < 0x20 }) >= 0 {
		return fmt.Errorf("%w: invalid path segment %q", core.ErrPath, segment)
	}
	if strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, " ") {
		return fmt.Errorf("%w: unsafe trailing path segment %q", core.ErrPath, segment)
	}
	base := strings.ToUpper(strings.TrimRight(strings.SplitN(segment, ".", 2)[0], " ."))
	switch base {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9", "LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return fmt.Errorf("%w: reserved Windows path segment %q", core.ErrPath, segment)
	}
	return nil
}

func canonicalDirectory(root string) (string, error) {
	if root == "" || portableAbsolute(root) && !filepath.IsAbs(root) {
		return "", fmt.Errorf("%w: invalid root %q", core.ErrPath, root)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("%w: absolute root: %v", core.ErrPath, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("%w: resolve root %q: %v", core.ErrPath, root, err)
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("not a directory")
		}
		return "", fmt.Errorf("%w: root %q: %v", core.ErrPath, root, err)
	}
	return filepath.Clean(resolved), nil
}

func resolveCandidate(candidate string) (string, error) {
	missing := make([]string, 0)
	current := candidate
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if !info.IsDir() && len(missing) > 0 {
				return "", fmt.Errorf("%w: non-directory path component %q", core.ErrPath, current)
			}
			resolved, err := filepath.EvalSymlinks(current)
			if err != nil {
				return "", fmt.Errorf("%w: resolve candidate %q: %v", core.ErrPath, current, err)
			}
			for index := len(missing) - 1; index >= 0; index-- {
				resolved = filepath.Join(resolved, missing[index])
			}
			return filepath.Clean(resolved), nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: inspect candidate %q: %v", core.ErrPath, current, err)
		}
		parent, base := filepath.Dir(current), filepath.Base(current)
		if parent == current {
			return "", fmt.Errorf("%w: no existing candidate parent %q", core.ErrPath, candidate)
		}
		if entries, readErr := os.ReadDir(parent); readErr == nil {
			for _, entry := range entries {
				if entry.Name() != base && strings.EqualFold(entry.Name(), base) {
					return "", fmt.Errorf("%w: case-folding alias %q", core.ErrPath, candidate)
				}
			}
		}
		missing = append(missing, base)
		current = parent
	}
}

func contained(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return false
	}
	return true
}

// portableAbsolute catches Windows absolute/UNC paths even on a Unix host.
func portableAbsolute(value string) bool {
	if strings.HasPrefix(value, `\\`) || strings.HasPrefix(value, `//`) {
		return true
	}
	return len(value) >= 3 && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) && value[1] == ':' && (value[2] == '\\' || value[2] == '/')
}

func relativeSegments(value string) ([]string, error) {
	if value == "" || filepath.IsAbs(value) || portableAbsolute(value) {
		return nil, fmt.Errorf("%w: artifact path %q must be relative", core.ErrPath, value)
	}
	parts := strings.Split(strings.ReplaceAll(value, `\`, "/"), "/")
	for _, part := range parts {
		if err := ValidateSegment(part); err != nil {
			return nil, err
		}
	}
	return parts, nil
}
