package projectpath

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var volatileNames = []string{".idea", ".vscode", ".ddev", ".cache", ".phpunit.cache", ".phpunit.result.cache", ".DS_Store"}

// Volatile reports editor, environment and site runtime data that analyzers do not read, so writes there must not invalidate a run.
func Volatile(relative string) bool {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	last := len(parts) - 1
	if slices.Contains(volatileNames, parts[last]) {
		return true
	}
	if last >= 1 && parts[last] == "simpletest" && parts[last-1] == "sites" {
		return true
	}
	return last >= 2 && parts[last-2] == "sites" && (parts[last] == "files" || parts[last] == "private")
}

func Validate(value string) error {
	if value == "" || strings.ContainsAny(value, "\\\x00") || strings.HasPrefix(value, "/") ||
		(len(value) >= 2 && value[1] == ':') {
		return fmt.Errorf("expected a project-relative path, got %q", value)
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return fmt.Errorf("parent traversal is not allowed in %q", value)
		}
	}
	return nil
}

func Within(root, value string) error {
	candidate := filepath.Join(root, filepath.FromSlash(value))
	for {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err == nil {
			relative, err := filepath.Rel(root, resolved)
			if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
				return fmt.Errorf("path %q resolves outside project root", value)
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return fmt.Errorf("path %q: %w", value, err)
		}
		if info, statErr := os.Lstat(candidate); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path %q has an unresolved symlink", value)
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return err
		}
		candidate = parent
	}
}
