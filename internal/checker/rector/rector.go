package rector

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/edit"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

const Version = "2.6.1"
const DrupalVersion = "1.1.3"
const PHPStanVersion = "2.3.1"
const FinderVersion = "1.3.1"

//go:embed helper.php
var helper []byte

//go:embed config.php
var configuration []byte

func Run(ctx context.Context, root, composerRoot, targetPHP string, selected scope.Selection, cfg config.Rector, process runner.Process) checker.Outcome {
	executable, _ := os.Executable()
	bundle := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "rector"))
	return run(ctx, root, composerRoot, targetPHP, selected, cfg, bundle, process)
}

func run(ctx context.Context, root, composerRoot, targetPHP string, selected scope.Selection, cfg config.Rector, bundle string, process runner.Process) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "rector", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	selected = Select(selected, cfg)
	if !cfg.Enabled || len(selected.Files) == 0 {
		reason := "no_files"
		if !cfg.Enabled {
			reason = "disabled"
		}
		result.Check.Status, result.Check.Reason = "skipped", &reason
		return result
	}
	fail := func(status, code, message string) checker.Outcome {
		result.Check.Status, result.Check.Reason, result.Findings = status, &code, []report.Finding{}
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: new("rector"), Message: message})
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return fail("error", "checker_timeout", "Rector execution context expired")
	}
	prepared, err := prepare(root, composerRoot, targetPHP, bundle, selected, cfg)
	if err != nil {
		return fail("error", "config_invalid", err.Error())
	}
	binary := filepath.Join(bundle, "vendor/rector/rector/bin/rector.php")
	for _, filename := range []string{binary, filepath.Join(bundle, "vendor/autoload.php")} {
		if info, err := os.Stat(filename); err != nil || !info.Mode().IsRegular() {
			return fail("unavailable", "dependencies_missing", "The isolated Rector bundle is unavailable; rebuild the pinned toolset")
		}
	}
	temporary, err := os.MkdirTemp("", "phpwarden-rector-")
	if err != nil {
		return fail("error", "helper_prepare", "Cannot prepare the private Rector configuration")
	}
	defer os.RemoveAll(temporary)
	input, _ := json.Marshal(prepared)
	for name, data := range map[string][]byte{"helper.php": helper, "rector.php": configuration, "request.json": input} {
		if err := os.WriteFile(filepath.Join(temporary, name), data, 0o600); err != nil {
			return fail("error", "helper_prepare", "Cannot write the private Rector configuration")
		}
	}
	environment := []string{"HOME=" + temporary, "TMPDIR=" + temporary, "LANG=C.UTF-8", "CI=1"}
	for _, name := range []string{"PATH", "SYSTEMROOT", "WINDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	phpArgs := []string{"-d", "auto_prepend_file=", "-d", "auto_append_file=", "-d", "display_errors=stderr", "-d", "log_errors=0", "-d", fmt.Sprintf("memory_limit=%dM", cfg.MemoryLimitMB)}
	runPHP := func(arguments ...string) runner.Result {
		return process.Run(ctx, runner.Command{Name: "php", Args: append(slices.Clone(phpArgs), arguments...), Directory: config.ComposerDir(root, config.Config{ComposerRoot: composerRoot}), Env: environment})
	}
	metadata := runPHP(filepath.Join(temporary, "helper.php"), bundle)
	if metadata.Unavailable {
		return fail("unavailable", "executable_missing", "PHP is unavailable in the worker")
	}
	if metadata.TimedOut || ctx.Err() != nil {
		return fail("error", "checker_timeout", "Rector dependency inspection timed out")
	}
	var versions struct {
		Version int               `json:"version"`
		Runtime string            `json:"runtime"`
		PHP     string            `json:"php_version"`
		Tools   map[string]string `json:"tools"`
	}
	want := map[string]string{"rector/rector": Version, "palantirnet/drupal-rector": DrupalVersion, "phpstan/phpstan": PHPStanVersion, "webflo/drupal-finder": FinderVersion}
	if metadata.Err != nil || metadata.ExitCode != 0 || len(metadata.Stderr) != 0 || strictJSON(metadata.Stdout, &versions) != nil || versions.Version != 1 || !reflect.DeepEqual(versions.Tools, want) || versions.Runtime == "" || !strings.HasPrefix(versions.PHP, versions.Runtime+".") {
		return fail("error", "dependencies_invalid", "Rector dependencies do not match the pinned isolated bundle")
	}
	result.Runtime, result.PHPVersion = &versions.Runtime, &versions.PHP
	contents := map[string][]byte{}
	project, err := os.OpenRoot(root)
	if err != nil {
		return fail("error", "input_read", "Cannot read Rector inputs")
	}
	defer project.Close()
	for _, name := range selected.Files {
		data, _, err := edit.Read(project, name)
		if err != nil || len(data) > 16<<20 {
			return fail("error", "input_read", "Rector inputs must be readable regular files at most 16 MiB each")
		}
		contents[name] = data
	}
	args := []string{binary, "process", "--config=" + filepath.Join(temporary, "rector.php"), "--dry-run", "--output-format=json", "--no-progress-bar", "--no-ansi", fmt.Sprintf("--memory-limit=%dM", cfg.MemoryLimitMB)}
	if prepared.Autoload != "" {
		args = append(args, "--autoload-file="+prepared.Autoload)
	}
	execution := runPHP(args...)
	if execution.Unavailable {
		return fail("unavailable", "executable_missing", "PHP is unavailable in the worker")
	}
	if execution.TimedOut || ctx.Err() != nil {
		return fail("error", "checker_timeout", "Rector did not complete within its execution context")
	}
	if execution.ExitCode != 0 && execution.ExitCode != 2 || execution.ExitCode == 0 && execution.Err != nil || len(execution.Stderr) > 0 {
		return fail("error", "process_failed", "Rector could not complete its dry-run; no partial suggestions accepted")
	}
	for name, before := range contents {
		after, _, err := edit.Read(project, name)
		if err != nil || !bytes.Equal(before, after) {
			return fail("error", "input_changed", "A Rector input changed during its dry-run")
		}
	}
	findings, err := decode(execution.Stdout, execution.ExitCode, root, selected, contents)
	if err != nil {
		return fail("error", "invalid_report", err.Error())
	}
	result.Findings = findings
	if len(findings) > 0 && cfg.FailOn == "warning" {
		result.Check.Status = "failed"
	}
	return result
}

func strictJSON(data []byte, target any) error {
	if len(data) > 32<<20 {
		return fmt.Errorf("Rector response exceeds 32 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("expected exactly one Rector response")
	}
	return nil
}

func decode(data []byte, exit int, root string, selected scope.Selection, contents map[string][]byte) ([]report.Finding, error) {
	failure := func() ([]report.Finding, error) {
		return nil, fmt.Errorf("Rector returned an invalid, incomplete or out-of-scope dry-run report")
	}
	var response struct {
		Totals *struct {
			Changed *int `json:"changed_files"`
			Errors  *int `json:"errors"`
		} `json:"totals"`
		Changed []string `json:"changed_files"`
		Diffs   []struct {
			File  string   `json:"file"`
			Diff  string   `json:"diff"`
			Rules []string `json:"applied_rectors"`
		} `json:"file_diffs"`
		Errors []json.RawMessage `json:"errors"`
		Unused []string          `json:"unused_skips"`
	}
	if strictJSON(data, &response) != nil || response.Totals == nil || response.Totals.Changed == nil || response.Totals.Errors == nil || *response.Totals.Errors != 0 || len(response.Errors) > 0 || *response.Totals.Changed != len(response.Diffs) || (exit == 0) != (*response.Totals.Changed == 0) {
		return failure()
	}
	visited := map[string]bool{}
	for _, name := range response.Changed {
		if filepath.IsAbs(name) {
			relative, err := filepath.Rel(root, name)
			if err != nil {
				return failure()
			}
			name = filepath.ToSlash(relative)
		}
		if _, ok := contents[name]; !ok || visited[name] {
			return failure()
		}
		visited[name] = true
	}
	result := []report.Finding{}
	seen := map[string]bool{}
	for _, row := range response.Diffs {
		filename := row.File
		if filepath.IsAbs(filename) {
			var err error
			filename, err = filepath.Rel(root, filename)
			if err != nil {
				return failure()
			}
			filename = filepath.ToSlash(filename)
		}
		original, exists := contents[filename]
		if !exists || seen[filename] || !slices.Contains(response.Changed, row.File) || len(row.Rules) == 0 {
			return failure()
		}
		seen[filename] = true
		for _, rule := range row.Rules {
			if !className.MatchString(rule) {
				return failure()
			}
		}
		lines, err := changedLines(row.Diff, original)
		if err != nil || len(lines) == 0 {
			return failure()
		}
		for _, line := range lines {
			if !selected.Contains(filename, line) {
				continue
			}
			result = append(result, report.Finding{Checker: "rector", Rule: "rector.upgrade", Kind: "code", Severity: "warning", Message: "Dry-run upgrade suggestion; applied rules for this file: " + strings.Join(row.Rules, ", "), Path: new(filename), Line: new(line)})
		}
	}
	return result, nil
}
