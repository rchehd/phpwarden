package projectinit

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"go.yaml.in/yaml/v3"
)

var minorVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// detectRuntime returns nil when only composer.lock can decide.
func detectRuntime(root, composerRoot string, facts profile.Facts, flag string) (*Choice, error) {
	if flag != "" {
		return &Choice{flag, "--php-runtime"}, nil
	}
	for _, directory := range slices.Compact([]string{".", composerRoot}) {
		ddev, err := ddevVersion(root, directory)
		if err != nil || ddev != nil {
			return ddev, err
		}
	}
	if facts.PlatformPHP != nil {
		return &Choice{minor(*facts.PlatformPHP), path.Join(composerRoot, "composer.json") + " config.platform.php"}, nil
	}
	return nil, nil
}

// ddevVersion applies .ddev/config.yaml, then config.*.yaml overrides in name order, as DDEV does.
func ddevVersion(root, directory string) (*Choice, error) {
	overrides, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(directory), ".ddev", "config.*.yaml"))
	if err != nil {
		return nil, err
	}
	slices.Sort(overrides)
	var result *Choice
	for _, filename := range append([]string{filepath.Join(root, filepath.FromSlash(directory), ".ddev", "config.yaml")}, overrides...) {
		name := filepath.ToSlash(strings.TrimPrefix(filename, root+string(filepath.Separator)))
		data, err := os.ReadFile(filename)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		var document struct {
			PHPVersion yaml.Node `yaml:"php_version"`
		}
		if err := yaml.Unmarshal(data, &document); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if document.PHPVersion.Kind == 0 {
			continue
		}
		if document.PHPVersion.Kind != yaml.ScalarNode || !minorVersion.MatchString(document.PHPVersion.Value) {
			return nil, fmt.Errorf("%s: php_version must be MAJOR.MINOR, such as \"8.3\"", name)
		}
		result = &Choice{document.PHPVersion.Value, name + " php_version"}
	}
	return result, nil
}

func projectRoot(ctx context.Context, options Options) (string, error) {
	cwd, err := resolve(options.CWD)
	if err != nil {
		return "", err
	}
	if options.Project != "" {
		project := options.Project
		if !filepath.IsAbs(project) {
			project = filepath.Join(cwd, project)
		}
		root, err := resolve(project)
		if err != nil {
			return "", fmt.Errorf("project root: %w", err)
		}
		return root, nil
	}
	// The configuration lives at the Git root, so the whole repository is in scope.
	if top, err := git(ctx, cwd, "rev-parse", "--show-toplevel"); err == nil {
		return resolve(top)
	}
	return cwd, nil
}

// findComposerRoot picks the Drupal Composer project: the root itself, the only one below it, or the one containing the working directory.
func findComposerRoot(root string, options Options) (string, error) {
	if options.ComposerRoot != "" {
		name := path.Clean(options.ComposerRoot)
		if err := projectpath.Validate(name); err != nil {
			return "", fmt.Errorf("--composer-root: %w", err)
		}
		return name, nil
	}
	var candidates []string
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name, _ := filepath.Rel(root, current)
		name = filepath.ToSlash(name)
		if entry.IsDir() && name != "." && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" || entry.Name() == "node_modules" || strings.Count(name, "/") >= 3) {
			return fs.SkipDir
		}
		if !entry.IsDir() && entry.Name() == "composer.lock" && entry.Type().IsRegular() && locksDrupal(current) {
			candidates = append(candidates, path.Dir(name))
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if len(candidates) == 0 || slices.Contains(candidates, ".") {
		return ".", nil
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	cwd, _ := resolve(options.CWD)
	for _, candidate := range candidates {
		if relative, err := filepath.Rel(filepath.Join(root, filepath.FromSlash(candidate)), cwd); err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("several Drupal Composer projects found (%s); pass --composer-root DIR or run init inside one of them", strings.Join(candidates, ", "))
}

func locksDrupal(filename string) bool {
	data, err := os.ReadFile(filename)
	if err != nil {
		return false
	}
	var lock struct {
		Packages []struct {
			Name string `json:"name"`
		} `json:"packages"`
	}
	return json.Unmarshal(data, &lock) == nil && slices.ContainsFunc(lock.Packages, func(pkg struct {
		Name string `json:"name"`
	}) bool {
		return pkg.Name == "drupal/core"
	})
}

func resolve(name string) (string, error) {
	absolute, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(resolved); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", name)
	}
	return resolved, nil
}

func git(ctx context.Context, directory string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", directory}, args...)...)
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "GIT_") {
			command.Env = append(command.Env, variable)
		}
	}
	output, err := command.Output()
	return strings.TrimSpace(string(output)), err
}

func minor(version string) string {
	parts := strings.SplitN(version, ".", 3)
	return parts[0] + "." + parts[1]
}
