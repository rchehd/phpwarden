package composervalidate

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/phpconstraint"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

const ComposerVersion = "2.10.3"

//go:embed helper.php
var helper []byte

type fileReport struct {
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
	Lock     []string `json:"lock"`
}

type helperReport struct {
	Version         int                   `json:"version"`
	Runtime         string                `json:"runtime"`
	PHPVersion      string                `json:"php_version"`
	ComposerVersion string                `json:"composer_version"`
	Files           map[string]fileReport `json:"files"`
}

// Target is a composer.json to validate; Lock also checks the composer.lock beside it.
type Target struct {
	Path string
	Lock bool
}

// Select validates the root composer.json when a root Composer file changed, plus composer.json files of selected modules.
func Select(selection scope.Selection, cfg config.ComposerValidate) []Target {
	targets := []Target{}
	if len(selection.ProjectFiles) > 0 {
		targets = append(targets, Target{Path: "composer.json", Lock: true})
	}
	for _, name := range selection.Files {
		if path.Base(name) == "composer.json" {
			targets = append(targets, Target{Path: name})
		}
	}
	names := make([]string, len(targets))
	for i, target := range targets {
		names[i] = target.Path
	}
	filtered := scope.Selection{Files: names, LineMap: map[string][]scope.Range{}}.Filter(cfg.Include, cfg.Exclude, func(string) bool { return true })
	return slices.DeleteFunc(targets, func(target Target) bool { return !slices.Contains(filtered.Files, target.Path) })
}

func Run(ctx context.Context, root string, targets []Target, cfg config.ComposerValidate, process runner.Process) checker.Outcome {
	executable, _ := os.Executable()
	bundle := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "composer"))
	return run(ctx, root, targets, cfg, bundle, process)
}

func run(ctx context.Context, root string, targets []Target, cfg config.ComposerValidate, bundle string, process runner.Process) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "composer-validate", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	if !cfg.Enabled || len(targets) == 0 {
		reason := "no_files"
		if !cfg.Enabled {
			reason = "disabled"
		}
		result.Check.Status, result.Check.Reason = "skipped", &reason
		return result
	}
	fail := func(status, code, text string) checker.Outcome {
		result.Check.Status, result.Check.Reason = status, &code
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: new("composer-validate"), Message: text})
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return fail("error", "checker_timeout", "Composer validate did not complete within its execution context")
	}
	if _, err := os.Stat(filepath.Join(bundle, "vendor/autoload.php")); err != nil {
		return fail("unavailable", "dependencies_missing", "Bundled Composer dependencies are unavailable beside the worker binary; rebuild or reinstall the toolset")
	}
	temporary, err := os.MkdirTemp("", "phpwarden-composer-validate-")
	if err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	defer os.RemoveAll(temporary)
	script := filepath.Join(temporary, "composer-validate.php")
	if err := os.WriteFile(script, helper, 0o600); err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	files := []map[string]any{}
	absolute := make([]string, len(targets))
	for i, target := range targets {
		absolute[i] = filepath.Join(root, filepath.FromSlash(target.Path))
		files = append(files, map[string]any{"path": absolute[i], "lock": target.Lock})
	}
	input, _ := json.Marshal(map[string]any{"files": files})
	home := filepath.Join(temporary, "home")
	environment := append(os.Environ(), "COMPOSER_HOME="+home, "COMPOSER_CACHE_DIR="+filepath.Join(home, "cache"), "COMPOSER_DISABLE_NETWORK=1", "COMPOSER_NO_INTERACTION=1")
	execution := process.Run(ctx, runner.Command{Name: "php", Directory: root, Input: input, Env: environment,
		Args: []string{"-d", "auto_prepend_file=", "-d", "auto_append_file=", "-d", "display_errors=stderr", "-d", "log_errors=0", "-d", "memory_limit=512M", script, bundle}})
	if execution.Unavailable {
		return fail("unavailable", "executable_missing", "PHP executable is unavailable in the worker")
	}
	if execution.TimedOut || ctx.Err() != nil {
		return fail("error", "checker_timeout", "Composer validate did not complete within its execution context")
	}
	if execution.Err != nil || execution.ExitCode != 0 {
		return fail("error", "process_failed", fmt.Sprintf("Composer validate helper exited %d: %s", execution.ExitCode, strings.TrimSpace(string(execution.Stderr))))
	}
	parsed, err := decode(execution.Stdout, execution.Stderr, absolute)
	if err != nil {
		return fail("error", "invalid_report", err.Error())
	}
	result.Runtime, result.PHPVersion = &parsed.Runtime, &parsed.PHPVersion
	for i, target := range targets {
		file := parsed.Files[absolute[i]]
		add := func(rule, severity string, messages []string) {
			for _, message := range messages {
				message = strings.ReplaceAll(message, absolute[i], target.Path)
				message = strings.ReplaceAll(message, filepath.Dir(absolute[i])+string(filepath.Separator), "")
				result.Findings = append(result.Findings, report.Finding{Checker: "composer-validate", Rule: rule, Kind: "global", Severity: severity, Message: fmt.Sprintf("Composer file %q: %s", target.Path, message)})
			}
		}
		warning := "warning"
		if cfg.Strict {
			warning = "error"
		}
		add("composer.validate.error", "error", file.Errors)
		add("composer.lock.error", "error", file.Lock)
		add("composer.validate.warning", warning, file.Warnings)
	}
	for _, finding := range result.Findings {
		if cfg.FailOn != "never" && (finding.Severity == "error" || cfg.FailOn == "warning" && finding.Severity == "warning") {
			result.Check.Status = "failed"
		}
	}
	return result
}

func decode(data, stderr []byte, files []string) (helperReport, error) {
	var parsed helperReport
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return parsed, err
	}
	if _, err := decoder.Token(); err != io.EOF || len(stderr) > 0 || parsed.Version != 1 || parsed.ComposerVersion != ComposerVersion {
		return parsed, fmt.Errorf("expected one versioned Composer validate report from the pinned toolset")
	}
	canonical, err := phpconstraint.StableVersion(parsed.PHPVersion)
	if err != nil || canonical != parsed.PHPVersion || canonical[:strings.LastIndex(canonical, ".")] != parsed.Runtime {
		return parsed, fmt.Errorf("invalid Composer validate PHP runtime version")
	}
	if len(parsed.Files) != len(files) {
		return parsed, fmt.Errorf("Composer validate report does not match the selected files")
	}
	for _, name := range files {
		file, exists := parsed.Files[name]
		if !exists || file.Errors == nil || file.Warnings == nil || file.Lock == nil ||
			slices.Contains(file.Errors, "") || slices.Contains(file.Warnings, "") || slices.Contains(file.Lock, "") {
			return parsed, fmt.Errorf("Composer validate report does not match the selected files")
		}
	}
	return parsed, nil
}
