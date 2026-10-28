package config

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/inputsource"
	"github.com/rchehd/phpwarden/internal/projectpath"
)

const Filename = ".phpwarden.yaml"

type Location struct {
	Root string `json:"project_root"`
	Path string `json:"config_path"`
}

type LocationOptions struct {
	CWD     string
	Project string
	Config  string
}

func Resolve(options LocationOptions) (Location, error) {
	cwd, err := directory(options.CWD)
	if err != nil {
		return Location{}, err
	}
	root := ""
	if options.Project != "" {
		root, err = directory(absolute(cwd, options.Project))
		if err != nil {
			return Location{}, fmt.Errorf("project root: %w", err)
		}
	}
	if options.Config != "" {
		configPath := absolute(cwd, options.Config)
		if root == "" {
			root, err = directory(filepath.Dir(configPath))
			if err != nil {
				return Location{}, err
			}
		}
		return locationAt(root, configPath)
	}
	if root != "" {
		return locationAt(root, filepath.Join(root, Filename))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "-C", cwd, "rev-parse", "--show-toplevel")
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "GIT_") {
			command.Env = append(command.Env, variable)
		}
	}
	output, err := command.Output()
	if err != nil {
		return Location{}, fmt.Errorf("find Git worktree root (or use --project/--config): %w", err)
	}
	gitRoot, err := directory(strings.TrimSuffix(string(output), "\n"))
	if err != nil {
		return Location{}, err
	}
	for current := cwd; ; current = filepath.Dir(current) {
		candidate := filepath.Join(current, Filename)
		if _, err := os.Lstat(candidate); err == nil {
			return locationAt(current, candidate)
		} else if !os.IsNotExist(err) {
			return Location{}, err
		}
		if current == gitRoot || current == filepath.Dir(current) {
			break
		}
	}
	return Location{}, fmt.Errorf("no %s found inside Git worktree; create a project configuration", Filename)
}

// ComposerDir is the absolute directory with composer.json, composer.lock and vendor.
func ComposerDir(root string, cfg Config) string {
	return filepath.Join(root, filepath.FromSlash(cfg.ComposerRoot))
}

func Load(location Location) (Effective, error) {
	data, err := os.ReadFile(location.Path)
	if err != nil {
		return Effective{}, err
	}
	result, err := Parse(data)
	if err != nil {
		return Effective{}, inputsource.WithFile(err, location.Path)
	}
	result.SourceHash = fmt.Sprintf("%x", sha256.Sum256(data))
	for name, paths := range map[string][]string{
		"scope.include":       result.Config.Scope.Include,
		"tools.debug.include": result.Config.Tools.Debug.Include,
	} {
		for i, value := range paths {
			if err := projectpath.Validate(value); err != nil {
				return Effective{}, &inputsource.Error{File: location.Path, Path: fmt.Sprintf("$.%s[%d]", name, i), Message: err.Error(), Err: err}
			}
			if err := projectpath.Within(location.Root, value); err != nil {
				return Effective{}, &inputsource.Error{File: location.Path, Path: fmt.Sprintf("$.%s[%d]", name, i), Message: err.Error(), Err: err}
			}
		}
	}
	return result, nil
}

func directory(name string) (string, error) {
	absolutePath, err := filepath.Abs(name)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", name)
	}
	return resolved, nil
}

func locationAt(root, configPath string) (Location, error) {
	resolved, err := filepath.EvalSymlinks(configPath)
	if err != nil {
		return Location{}, fmt.Errorf("configuration: %w", err)
	}
	if filepath.Dir(resolved) != root {
		return Location{}, fmt.Errorf("configuration must be directly inside project root %q", root)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return Location{}, err
	}
	if !info.Mode().IsRegular() {
		return Location{}, fmt.Errorf("configuration must be a regular file")
	}
	return Location{Root: root, Path: resolved}, nil
}

func absolute(cwd, name string) string {
	if filepath.IsAbs(name) {
		return filepath.Clean(name)
	}
	return filepath.Join(cwd, name)
}

func parseVersion(value string) ([2]uint64, error) {
	var version [2]uint64
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return version, fmt.Errorf("invalid PHP version %q", value)
	}
	for i, part := range parts {
		number, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return version, fmt.Errorf("invalid PHP version %q", value)
		}
		version[i] = number
	}
	return version, nil
}

func validatePatterns(values []string) error {
	for _, value := range values {
		if err := projectpath.Validate(value); err != nil {
			return err
		}
		if _, err := path.Match(value, ""); err != nil {
			return fmt.Errorf("invalid exclusion pattern %q: %w", value, err)
		}
	}
	return nil
}
