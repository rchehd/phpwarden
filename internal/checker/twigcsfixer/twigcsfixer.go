package twigcsfixer

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/edit"
	"github.com/rchehd/phpwarden/internal/phpconstraint"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

const Version = "4.1.1"
const TwigVersion = "3.30.0"

//go:embed helper.php
var helper []byte

type violation struct {
	Level   int    `json:"level"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
	Line    *int   `json:"line"`
	Column  *int   `json:"column"`
}

type helperReport struct {
	Version    int                    `json:"version"`
	Runtime    string                 `json:"runtime"`
	PHPVersion string                 `json:"php_version"`
	Tools      map[string]string      `json:"tools"`
	Files      map[string][]violation `json:"files"`
	Edits      []edit.Edit            `json:"edits,omitempty"`
}

func Run(ctx context.Context, root, composerRoot string, selection scope.Selection, cfg config.TwigCSFixer, profileID string, process runner.Process) checker.Outcome {
	executable, _ := os.Executable()
	bundle := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "twig-cs-fixer"))
	return run(ctx, root, composerRoot, selection, cfg, profileID, bundle, process)
}

func run(ctx context.Context, root, composerRoot string, selection scope.Selection, cfg config.TwigCSFixer, profileID, bundle string, process runner.Process) checker.Outcome {
	return runMode(ctx, root, composerRoot, selection, cfg, profileID, bundle, process, false)
}

func Fix(ctx context.Context, root, composerRoot string, selection scope.Selection, cfg config.TwigCSFixer, profileID string, process runner.Process) checker.Outcome {
	executable, _ := os.Executable()
	bundle := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "twig-cs-fixer"))
	return runMode(ctx, root, composerRoot, selection, cfg, profileID, bundle, process, true)
}

func runMode(ctx context.Context, root, composerRoot string, selection scope.Selection, cfg config.TwigCSFixer, profileID, bundle string, process runner.Process, fixing bool) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "twig-cs-fixer", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	if !cfg.Enabled || len(selection.Files) == 0 {
		reason := "no_files"
		if !cfg.Enabled {
			reason = "disabled"
		}
		result.Check.Status, result.Check.Reason = "skipped", &reason
		return result
	}
	fail := func(status, code, text string) checker.Outcome {
		result.Check.Status, result.Check.Reason = status, &code
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: new("twig-cs-fixer"), Message: text})
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return fail("error", "checker_timeout", "Twig CS Fixer did not complete within its execution context")
	}
	selected, _, err := prepare(root, composerRoot, cfg)
	if err != nil {
		return fail("error", "config_invalid", err.Error())
	}
	if _, err := os.Stat(filepath.Join(bundle, "vendor/autoload.php")); err != nil {
		return fail("unavailable", "dependencies_missing", "Bundled Twig CS Fixer dependencies are unavailable beside the worker binary; rebuild or reinstall the toolset")
	}
	temporary, err := os.MkdirTemp("", "phpwarden-twig-cs-fixer-")
	if err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	defer os.RemoveAll(temporary)
	script := filepath.Join(temporary, "twig-cs-fixer.php")
	if err := os.WriteFile(script, helper, 0o600); err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	files := make([]string, len(selection.Files))
	for i, name := range selection.Files {
		files[i] = filepath.Join(root, filepath.FromSlash(name))
	}
	var configPath *string
	if selected != "" {
		configPath = new(filepath.Join(root, filepath.FromSlash(selected)))
	}
	autoload, err := projectAutoload(root, composerRoot, cfg.Autoload)
	if err != nil {
		return fail("error", "config_invalid", err.Error())
	}
	if autoload != "" {
		autoload = filepath.Join(root, filepath.FromSlash(autoload))
	}
	defaults, _ := profile.Lookup(profileID)
	input, _ := json.Marshal(map[string]any{"config": configPath, "autoload": autoload, "removed_rules": defaults.TwigCSFixerRemovedRules, "files": files, "fix": fixing})
	execution := process.Run(ctx, runner.Command{Name: "php", Directory: root, Input: input, Args: []string{"-d", "auto_prepend_file=", "-d", "auto_append_file=", "-d", "display_errors=stderr", "-d", "log_errors=0", "-d", "memory_limit=256M", script, bundle}})
	if execution.Unavailable {
		return fail("unavailable", "executable_missing", "PHP executable is unavailable in the worker")
	}
	if execution.TimedOut || ctx.Err() != nil {
		return fail("error", "checker_timeout", "Twig CS Fixer did not complete within its execution context")
	}
	if execution.Err != nil || execution.ExitCode != 0 {
		return fail("error", "process_failed", fmt.Sprintf("Twig CS Fixer helper exited %d: %s", execution.ExitCode, strings.TrimSpace(string(execution.Stderr))))
	}
	parsed, err := decode(execution.Stdout, execution.Stderr, files)
	if err != nil {
		return fail("error", "invalid_report", err.Error())
	}
	if !fixing && len(parsed.Edits) != 0 || fixing && parsed.Edits == nil {
		return fail("error", "invalid_report", "Twig CS Fixer output does not match the requested operation")
	}
	for _, change := range parsed.Edits {
		name := change.Path
		if filepath.IsAbs(name) {
			name, err = filepath.Rel(root, name)
		}
		if err != nil || !slices.Contains(selection.Files, filepath.ToSlash(name)) {
			return fail("error", "invalid_report", "Twig CS Fixer returned an unexpected fix path")
		}
		change.Path = filepath.ToSlash(name)
		result.Edits = append(result.Edits, change)
	}
	result.Runtime, result.PHPVersion = &parsed.Runtime, &parsed.PHPVersion
	for i, name := range selection.Files {
		for _, row := range parsed.Files[files[i]] {
			if row.Level == 3 {
				if !strings.HasPrefix(row.Message, "Unable to tokenize file:") && !strings.HasPrefix(row.Message, "File is invalid:") {
					return fail("error", "analysis_failed", fmt.Sprintf("Twig CS Fixer could not analyze %q: %s", name, row.Message))
				}
				result.Findings = append(result.Findings, report.Finding{Checker: "twig-cs-fixer", Rule: "File.Invalid", Kind: "global", Severity: "error", Message: fmt.Sprintf("Twig template %q: %s", name, row.Message)})
				continue
			}
			if row.Rule == "" {
				result.Findings = append(result.Findings, report.Finding{Checker: "twig-cs-fixer", Rule: "Twig.Deprecation", Kind: "global", Severity: "info", Message: fmt.Sprintf("Twig template %q: %s", name, row.Message)})
				continue
			}
			if !selection.Contains(name, *row.Line) {
				continue
			}
			severity := map[int]string{0: "info", 1: "warning", 2: "error"}[row.Level]
			result.Findings = append(result.Findings, report.Finding{Checker: "twig-cs-fixer", Rule: row.Rule, Kind: "code", Severity: severity, Message: row.Message, Path: new(name), Line: row.Line, Column: row.Column})
		}
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
	if _, err := decoder.Token(); err != io.EOF || len(stderr) > 0 || parsed.Version != 1 || len(parsed.Tools) != 2 ||
		parsed.Tools["vincentlanglet/twig-cs-fixer"] != Version || parsed.Tools["twig/twig"] != TwigVersion {
		return parsed, fmt.Errorf("expected one versioned Twig CS Fixer report from the pinned toolset")
	}
	canonical, err := phpconstraint.StableVersion(parsed.PHPVersion)
	if err != nil || canonical != parsed.PHPVersion || canonical[:strings.LastIndex(canonical, ".")] != parsed.Runtime {
		return parsed, fmt.Errorf("invalid Twig CS Fixer PHP runtime version")
	}
	if len(parsed.Files) != len(files) {
		return parsed, fmt.Errorf("Twig CS Fixer report does not match the selected files")
	}
	for _, name := range files {
		rows, exists := parsed.Files[name]
		if !exists || rows == nil {
			return parsed, fmt.Errorf("Twig CS Fixer report does not match the selected files")
		}
		for _, row := range rows {
			fatal := row.Level == 3 && row.Rule == ""
			deprecation := row.Level == 0 && row.Rule == "" && row.Line == nil
			valid := row.Level >= 0 && row.Level <= 2 && row.Rule != "" && row.Line != nil && *row.Line >= 1 && (row.Column == nil || *row.Column >= 1)
			if row.Message == "" || !fatal && !deprecation && !valid {
				return parsed, fmt.Errorf("invalid Twig CS Fixer finding")
			}
		}
	}
	return parsed, nil
}
