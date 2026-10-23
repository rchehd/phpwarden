package scope

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/rchehd/phpwarden/internal/projectpath"
)

type Options struct {
	Root       string
	CWD        string
	Mode       string
	Base       string
	Include    []string
	Exclude    []string
	Paths      []string
	WholeFiles bool
	// Project-level files outside the source roots, such as composer.json, reported when changed or requested.
	ProjectFiles []string
}

type Selection struct {
	Mode         string             `json:"mode"`
	BaseRef      *string            `json:"base_ref"`
	MergeBase    *string            `json:"merge_base"`
	WholeFiles   bool               `json:"whole_files"`
	Files        []string           `json:"files"`
	LineMap      map[string][]Range `json:"line_map"`
	ProjectFiles []string           `json:"project_files"`
}

func (selection Selection) Contains(filename string, line int) bool {
	ranges, found := selection.LineMap[filename]
	if !found || line < 1 {
		return false
	}
	if ranges == nil {
		return true
	}
	for _, interval := range ranges {
		if line >= interval.Start && line <= interval.End {
			return true
		}
	}
	return false
}

func (selection Selection) Filter(includes, excludes []string, applicable func(string) bool) Selection {
	result := selection
	result.Files, result.LineMap = []string{}, map[string][]Range{}
	roots := make([]string, len(includes))
	for i, name := range includes {
		roots[i] = path.Clean(name)
	}
	for _, filename := range selection.Files {
		if len(roots) > 0 && !inRoots(filename, roots) || excluded(filename, excludes) || !applicable(filename) {
			continue
		}
		result.Files = append(result.Files, filename)
		result.LineMap[filename] = selection.LineMap[filename]
	}
	return result
}

func Build(ctx context.Context, options Options) (Selection, error) {
	result := Selection{Mode: options.Mode, WholeFiles: options.WholeFiles, Files: []string{}, LineMap: map[string][]Range{}, ProjectFiles: []string{}}
	if result.Mode == "" {
		result.Mode = "diff"
	}
	if result.Mode != "diff" && result.Mode != "full" && result.Mode != "paths" {
		return result, fmt.Errorf("unknown scope mode %q", result.Mode)
	}
	if result.Mode != "diff" && (options.Base != "" || options.WholeFiles) {
		return result, fmt.Errorf("base and whole-files apply only to diff")
	}
	if (result.Mode == "paths") != (len(options.Paths) > 0) {
		return result, fmt.Errorf("explicit paths require paths mode")
	}
	root, err := filepath.Abs(options.Root)
	if err != nil {
		return result, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return result, err
	}
	if options.CWD == "" {
		options.CWD = root
	} else {
		options.CWD, err = filepath.Abs(options.CWD)
		if err != nil {
			return result, err
		}
		options.CWD, err = filepath.EvalSymlinks(options.CWD)
		if err != nil {
			return result, err
		}
	}
	repo, err := openRepository(ctx, root)
	if err != nil {
		return result, err
	}
	if len(options.Include) == 0 {
		return result, fmt.Errorf("source roots are unresolved; set scope.include or configure a discoverable Drupal layout")
	}
	roots := make([]string, 0, len(options.Include))
	for _, include := range options.Include {
		if err := projectpath.Validate(include); err != nil {
			return result, err
		}
		if err := projectpath.Within(root, include); err != nil {
			return result, err
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(include)))
		if err != nil {
			return result, fmt.Errorf("source root %q: %w", include, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return result, fmt.Errorf("source root %q must not be a symlink", include)
		}
		roots = append(roots, path.Clean(include))
	}
	for _, pattern := range options.Exclude {
		if err := projectpath.Validate(pattern); err != nil {
			return result, err
		}
		if _, err := path.Match(pattern, ""); err != nil {
			return result, fmt.Errorf("invalid exclusion %q: %w", pattern, err)
		}
	}
	requested, err := explicitPaths(root, options.CWD, options.Paths)
	if err != nil {
		return result, err
	}
	files, err := repo.inventory()
	if err != nil {
		return result, err
	}
	var changed map[string]bool
	if result.Mode == "diff" {
		base := options.Base
		if base == "" {
			output, err := repo.git("symbolic-ref", "--quiet", "refs/remotes/origin/HEAD")
			if err != nil {
				return result, fmt.Errorf("base ref is unavailable; pass --base or fetch origin/HEAD: %w", err)
			}
			base = strings.TrimSpace(string(output))
		}
		output, err := repo.git("rev-parse", "--verify", "--end-of-options", base+"^{commit}")
		if err != nil {
			return result, fmt.Errorf("invalid base ref %q; fetch it or pass --base: %w", base, err)
		}
		output, err = repo.git("merge-base", strings.TrimSpace(string(output)), "HEAD")
		if err != nil {
			return result, fmt.Errorf("merge base unavailable; fetch the required history: %w", err)
		}
		mergeBase := strings.TrimSpace(string(output))
		result.BaseRef, result.MergeBase = &base, &mergeBase
		changed, err = repo.changed(mergeBase)
		if err != nil {
			return result, err
		}
	}
	selectedCounts := make(map[string]int)
	for filename := range files {
		if !inRoots(filename, roots) || excluded(filename, options.Exclude) || len(requested) > 0 && !inRoots(filename, requested) {
			continue
		}
		if result.Mode == "diff" {
			if _, exists := changed[filename]; !exists {
				continue
			}
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(filename)))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return result, err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if err := projectpath.Within(root, filename); err != nil {
			return result, err
		}
		var ranges []Range
		if result.Mode == "diff" && !options.WholeFiles && !changed[filename] {
			ranges, err = repo.changedLines(*result.MergeBase, filename)
			if err != nil {
				return result, err
			}
		}
		result.Files = append(result.Files, filename)
		result.LineMap[filename] = ranges
		for _, request := range requested {
			if inRoots(filename, []string{request}) {
				selectedCounts[request]++
			}
		}
	}
	for _, filename := range options.ProjectFiles {
		if _, exists := files[filename]; !exists || len(requested) > 0 && !inRoots(filename, requested) {
			continue
		}
		if _, exists := changed[filename]; result.Mode == "diff" && !exists {
			continue
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(filename)))
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		result.ProjectFiles = append(result.ProjectFiles, filename)
		for _, request := range requested {
			if inRoots(filename, []string{request}) {
				selectedCounts[request]++
			}
		}
	}
	for _, request := range requested {
		if selectedCounts[request] == 0 {
			return result, fmt.Errorf("path %q selects no eligible files (check roots, excludes, and Git ignores)", request)
		}
	}
	sort.Strings(result.Files)
	sort.Strings(result.ProjectFiles)
	return result, nil
}

func excluded(filename string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchGlob(pattern, filename) {
			return true
		}
	}
	return false
}

func explicitPaths(root, cwd string, values []string) ([]string, error) {
	requested := make([]string, 0, len(values))
	for _, value := range values {
		filename := value
		if !filepath.IsAbs(filename) {
			filename = filepath.Join(cwd, filename)
		}
		relative, err := filepath.Rel(root, filename)
		if err != nil {
			return nil, err
		}
		name := filepath.ToSlash(relative)
		if err := projectpath.Validate(name); err != nil {
			return nil, err
		}
		if err := projectpath.Within(root, name); err != nil {
			return nil, err
		}
		info, err := os.Lstat(filename)
		if err != nil {
			return nil, fmt.Errorf("explicit path %q: %w", value, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("explicit path %q must not be a symlink", value)
		}
		requested = append(requested, name)
	}
	return requested, nil
}
