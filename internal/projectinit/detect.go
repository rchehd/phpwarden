package projectinit

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/profile"
	"go.yaml.in/yaml/v3"
)

var minorVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

// detectRuntime returns nil when only composer.lock can decide.
func detectRuntime(root string, facts profile.Facts, flag string) (*Choice, error) {
	if flag != "" {
		return &Choice{flag, "--php-runtime"}, nil
	}
	ddev, err := ddevVersion(root)
	if err != nil || ddev != nil {
		return ddev, err
	}
	if facts.PlatformPHP != nil {
		return &Choice{minor(*facts.PlatformPHP), "composer.json config.platform.php"}, nil
	}
	return nil, nil
}

// ddevVersion applies .ddev/config.yaml, then config.*.yaml overrides in name order, as DDEV does.
func ddevVersion(root string) (*Choice, error) {
	overrides, err := filepath.Glob(filepath.Join(root, ".ddev", "config.*.yaml"))
	if err != nil {
		return nil, err
	}
	slices.Sort(overrides)
	var result *Choice
	for _, filename := range append([]string{filepath.Join(root, ".ddev", "config.yaml")}, overrides...) {
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
	stop := cwd
	if top, err := git(ctx, cwd, "rev-parse", "--show-toplevel"); err == nil {
		if stop, err = resolve(top); err != nil {
			return "", err
		}
	}
	for current := cwd; ; current = filepath.Dir(current) {
		if info, err := os.Stat(filepath.Join(current, "composer.json")); err == nil && info.Mode().IsRegular() {
			return current, nil
		}
		if current == stop || current == filepath.Dir(current) {
			break
		}
	}
	return "", fmt.Errorf("no composer.json found from %s up to the Git root; run init inside a Drupal project or pass --project PATH", cwd)
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
