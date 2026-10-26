package projectupdates

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rchehd/phpwarden/internal/checker/composeraudit"
	"github.com/rchehd/phpwarden/internal/runner"
)

var packageName = regexp.MustCompile(`^[a-z0-9_.-]+/[a-z0-9_.-]+$`)

type lockedPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Dist    struct {
		Type string `json:"type"`
	} `json:"dist"`
}

type lockFile struct {
	Packages []lockedPackage `json:"packages"`
	Dev      []lockedPackage `json:"packages-dev"`
}

func inventory(composer, lock []byte) (map[string]Package, map[string]bool, error) {
	var root struct {
		Require map[string]string `json:"require"`
		Dev     map[string]string `json:"require-dev"`
	}
	var locked lockFile
	if json.Unmarshal(composer, &root) != nil || json.Unmarshal(lock, &locked) != nil || locked.Packages == nil {
		return nil, nil, fmt.Errorf("invalid Composer manifest or lock package inventory")
	}
	result, local := map[string]Package{}, map[string]bool{}
	for index, group := range [][]lockedPackage{locked.Packages, locked.Dev} {
		for _, row := range group {
			if !packageName.MatchString(row.Name) || row.Version == "" {
				return nil, nil, fmt.Errorf("invalid locked package identity")
			}
			if _, exists := result[row.Name]; exists {
				return nil, nil, fmt.Errorf("duplicate locked package %s", row.Name)
			}
			_, direct := root.Require[row.Name]
			_, devDirect := root.Dev[row.Name]
			result[row.Name] = Package{Name: row.Name, Current: row.Version, Direct: direct || devDirect, Dev: index == 1, Advisories: []Advisory{}, Compatibility: "not_resolved"}
			local[row.Name] = row.Dist.Type == "path"
		}
	}
	return result, local, nil
}

func sanitizedManifest(composer []byte) ([]byte, error) {
	var values map[string]json.RawMessage
	if json.Unmarshal(composer, &values) != nil || values == nil {
		return nil, fmt.Errorf("invalid Composer manifest")
	}
	if raw, exists := values["repositories"]; exists {
		var object map[string]json.RawMessage
		isPath := func(raw json.RawMessage) bool {
			var row struct {
				Type string `json:"type"`
			}
			return json.Unmarshal(raw, &row) == nil && row.Type == "path"
		}
		if json.Unmarshal(raw, &object) == nil && object != nil {
			for name, value := range object {
				if isPath(value) {
					delete(object, name)
				}
			}
			values["repositories"], _ = json.Marshal(object)
		} else {
			var list []json.RawMessage
			if json.Unmarshal(raw, &list) != nil {
				return nil, fmt.Errorf("unsupported Composer repositories shape")
			}
			kept := []json.RawMessage{}
			for _, value := range list {
				if !isPath(value) {
					kept = append(kept, value)
				}
			}
			values["repositories"], _ = json.Marshal(kept)
		}
	}
	return json.Marshal(values)
}

func NativeComposer(bundle string, process runner.Process) ComposerFetcher {
	return func(ctx context.Context, composer, lock []byte) ([]byte, error) {
		_, local, err := inventory(composer, lock)
		if err != nil {
			return nil, err
		}
		composer, err = sanitizedManifest(composer)
		if err != nil {
			return nil, err
		}
		directory, err := os.MkdirTemp("", "phpwarden-outdated-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(directory)
		for name, data := range map[string][]byte{"composer.json": composer, "composer.lock": lock} {
			if err := os.WriteFile(filepath.Join(directory, name), data, 0o600); err != nil {
				return nil, err
			}
		}
		binary := filepath.Join(bundle, "vendor/composer/composer/bin/composer")
		args := []string{"-d", "auto_prepend_file=", "-d", "auto_append_file=", "-d", "memory_limit=512M", binary, "outdated", "--locked", "--ignore-platform-reqs", "--format=json", "--no-interaction", "--no-plugins", "--no-scripts", "--no-ansi", "--working-dir=" + directory}
		for name, isLocal := range local {
			if isLocal {
				args = append(args, "--ignore="+name)
			}
		}
		home := filepath.Join(directory, "home")
		env := append(os.Environ(), "COMPOSER_HOME="+home, "COMPOSER_CACHE_DIR="+filepath.Join(home, "cache"), "COMPOSER_NO_INTERACTION=1", "COMPOSER_DISABLE_NETWORK=0")
		execution := process.Run(ctx, runner.Command{Name: "php", Directory: directory, Args: args, Env: env})
		if execution.ExitCode != 0 || execution.Err != nil || execution.Unavailable || execution.TimedOut || ctx.Err() != nil {
			return nil, fmt.Errorf("Composer outdated could not query all configured repositories (exit %d)", execution.ExitCode)
		}
		diagnostics := len(bytes.TrimSpace(execution.Stderr)) > 0
		audit, err := composeraudit.NativeFetcher(bundle, diagnosticProcess{process, &diagnostics})(ctx, composer, lock, false)
		if err != nil {
			return nil, fmt.Errorf("Composer audit could not query the configured advisory sources")
		}
		if !json.Valid(execution.Stdout) || !json.Valid(audit) {
			return nil, fmt.Errorf("Composer returned invalid JSON")
		}
		return json.Marshal(ComposerData{Outdated: execution.Stdout, Audit: audit, Diagnostics: diagnostics})
	}
}

type diagnosticProcess struct {
	runner.Process
	diagnostics *bool
}

func (process diagnosticProcess) Run(ctx context.Context, command runner.Command) runner.Result {
	result := process.Process.Run(ctx, command)
	if len(bytes.TrimSpace(result.Stderr)) > 0 {
		*process.diagnostics = true
	}
	return result
}

func mapOrEmpty(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil && list != nil && len(list) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	var value map[string]json.RawMessage
	if json.Unmarshal(raw, &value) != nil || value == nil {
		return nil, fmt.Errorf("invalid Composer report map")
	}
	return value, nil
}

func parseComposer(data []byte, packages map[string]Package, local map[string]bool) (bool, error) {
	var value ComposerData
	if json.Unmarshal(data, &value) != nil || len(value.Outdated) == 0 || len(value.Audit) == 0 {
		return false, fmt.Errorf("invalid Composer update envelope")
	}
	var outdated struct {
		Locked []struct {
			Name      string `json:"name"`
			Version   string `json:"version"`
			Latest    string `json:"latest"`
			Abandoned bool   `json:"abandoned"`
		} `json:"locked"`
	}
	if json.Unmarshal(value.Outdated, &outdated) != nil || outdated.Locked == nil {
		return false, fmt.Errorf("Composer outdated omitted locked package rows")
	}
	seen := map[string]bool{}
	partial := value.Diagnostics
	for _, row := range outdated.Locked {
		item, exists := packages[row.Name]
		if !exists || seen[row.Name] || len(strings.Fields(row.Version)) == 0 || strings.TrimPrefix(strings.Fields(row.Version)[0], "v") != strings.TrimPrefix(item.Current, "v") || row.Latest == "" {
			return false, fmt.Errorf("Composer outdated returned an unknown, duplicate or mismatched package")
		}
		seen[row.Name] = true
		if local[row.Name] {
			continue
		}
		if row.Latest == "[none matched]" {
			partial = true
			continue
		}
		item.Latest, item.Abandoned = row.Latest, row.Abandoned
		a, b := strings.TrimPrefix(item.Current, "v"), strings.TrimPrefix(item.Latest, "v")
		item.UpdateType = "unknown"
		if a == b {
			item.UpdateType = "none"
		} else if exactVersion.MatchString(a) && exactVersion.MatchString(b) {
			left, right := strings.Split(a, "."), strings.Split(b, ".")
			switch {
			case left[0] != right[0]:
				item.UpdateType = "major"
			case left[1] != right[1]:
				item.UpdateType = "minor"
			default:
				item.UpdateType = "patch"
			}
		}
		packages[row.Name] = item
	}
	var audit map[string]json.RawMessage
	if json.Unmarshal(value.Audit, &audit) != nil || audit == nil {
		return false, fmt.Errorf("invalid Composer audit report")
	}
	advisories, err := mapOrEmpty(audit["advisories"])
	if err != nil {
		return false, err
	}
	groups := []map[string]json.RawMessage{advisories}
	if raw, exists := audit["ignored-advisories"]; exists {
		ignored, err := mapOrEmpty(raw)
		if err != nil {
			return false, err
		}
		groups = append(groups, ignored)
	}
	for index, group := range groups {
		for name, raw := range group {
			item, exists := packages[name]
			if !exists {
				return false, fmt.Errorf("advisory references unknown locked package")
			}
			var entries []struct {
				ID       string `json:"advisoryId"`
				Title    string `json:"title"`
				Link     string `json:"link"`
				Affected string `json:"affectedVersions"`
				Severity string `json:"severity"`
			}
			if json.Unmarshal(raw, &entries) != nil {
				return false, fmt.Errorf("invalid advisory rows")
			}
			for _, advisory := range entries {
				if advisory.ID == "" || advisory.Title == "" || advisory.Affected == "" {
					return false, fmt.Errorf("incomplete security advisory")
				}
				item.Advisories = append(item.Advisories, Advisory{advisory.ID, advisory.Title, advisory.Link, advisory.Affected, advisory.Severity, index == 1})
			}
			packages[name] = item
		}
	}
	if raw, exists := audit["unreachable-repositories"]; exists && string(bytes.TrimSpace(raw)) != "[]" && string(bytes.TrimSpace(raw)) != "{}" {
		partial = true
	}
	if raw, exists := audit["abandoned"]; exists {
		abandoned, err := mapOrEmpty(raw)
		if err != nil {
			return false, err
		}
		for name := range abandoned {
			item, exists := packages[name]
			if !exists {
				return false, fmt.Errorf("abandoned package is outside lock inventory")
			}
			item.Abandoned = true
			packages[name] = item
		}
	}
	return partial, nil
}
