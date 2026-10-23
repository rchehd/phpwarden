package gitleaks

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/internal/scope"
)

type plan struct {
	Config   string
	Baseline string
	Ignore   string
	Inputs   map[string]string
}

func DiscoverConfig(root, composerRoot string) (string, error) {
	prepared, err := prepare(root, composerRoot, config.Gitleaks{DiscoverConfig: true})
	return prepared.Config, err
}

func Select(selection scope.Selection, cfg config.Gitleaks) scope.Selection {
	return selection.Filter(cfg.Include, cfg.Exclude, func(string) bool { return true })
}

func InputRequired(cfg config.Config, selection scope.Selection, only []string) bool {
	return checker.Enabled("gitleaks", cfg.Tools.Gitleaks.Enabled, only) && len(Select(selection, cfg.Tools.Gitleaks).Files) > 0
}

func ContextHash(root, composerRoot string, cfg config.Gitleaks) string {
	prepared, err := prepare(root, composerRoot, cfg)
	encoded, _ := json.Marshal(struct {
		Inputs map[string]string
		Failed bool
	}{prepared.Inputs, err != nil})
	return fmt.Sprintf("%x", sha256.Sum256(encoded))
}

func regular(root, name string) error {
	if err := projectpath.Validate(name); err != nil {
		return err
	}
	current := root
	for _, part := range strings.Split(name, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Gitleaks inputs must not traverse symlinks")
		}
	}
	info, err := os.Stat(current)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("Gitleaks input must be a regular file")
	}
	return nil
}

func prepare(root, composerRoot string, cfg config.Gitleaks) (plan, error) {
	result := plan{Config: cfg.Config, Baseline: cfg.Baseline, Ignore: cfg.IgnoreFile, Inputs: map[string]string{}}
	read := func(name string) error {
		result.Inputs[name] = "invalid"
		if err := regular(root, name); err != nil {
			if os.IsNotExist(err) {
				result.Inputs[name] = "missing"
			}
			return err
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return err
		}
		result.Inputs[name] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	}
	for _, input := range []struct {
		Name   string
		Target *string
	}{{".gitleaks.toml", &result.Config}, {".gitleaksignore", &result.Ignore}} {
		if *input.Target != "" || !cfg.DiscoverConfig {
			continue
		}
		for _, directory := range slices.Compact([]string{".", composerRoot}) {
			name := path.Join(directory, input.Name)
			if err := read(name); err == nil {
				*input.Target = name
				break
			} else if !os.IsNotExist(err) {
				return result, err
			}
		}
	}
	for _, name := range append(slices.Clone(cfg.ExtraInputs), result.Config, result.Baseline, result.Ignore) {
		if name != "" {
			if err := read(name); err != nil {
				return result, err
			}
		}
	}
	return result, nil
}
