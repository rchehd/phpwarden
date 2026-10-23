package phpcs

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
	"strconv"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/phpconstraint"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

const Version = "4.0.4"
const CoderVersion = "9.0.1"
const CompatibilityVersion = "10.0.0-alpha2"

//go:embed helper.php
var helper []byte

type totals struct {
	Errors   int `json:"errors"`
	Warnings int `json:"warnings"`
	Fixable  int `json:"fixable"`
}
type message struct {
	Message  string `json:"message"`
	Source   string `json:"source"`
	Severity int    `json:"severity"`
	Fixable  bool   `json:"fixable"`
	Type     string `json:"type"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
}
type fileReport struct {
	Errors   int       `json:"errors"`
	Warnings int       `json:"warnings"`
	Messages []message `json:"messages"`
}
type helperReport struct {
	Version              int    `json:"version"`
	Runtime              string `json:"runtime"`
	PHPVersion           string `json:"php_version"`
	PHPCSVersion         string `json:"phpcs_version"`
	CoderVersion         string `json:"coder_version"`
	CompatibilityVersion string `json:"compatibility_version"`
	ExitCode             int    `json:"exit_code"`
	Report               struct {
		Totals totals                `json:"totals"`
		Files  map[string]fileReport `json:"files"`
	} `json:"report"`
}

func Run(ctx context.Context, root string, selection scope.Selection, cfg config.PHPCS, target config.Target, process runner.Process) checker.Outcome {
	executable, _ := os.Executable()
	bundle := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "phpcs"))
	return run(ctx, root, selection, cfg, target, bundle, process)
}

func run(ctx context.Context, root string, selection scope.Selection, cfg config.PHPCS, target config.Target, bundle string, process runner.Process) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "phpcs", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
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
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: new("phpcs"), Message: text})
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return fail("error", "checker_timeout", "PHPCS did not complete within its execution context")
	}
	prepared, err := prepare(root, selection, cfg)
	if err != nil {
		return fail("error", "config_invalid", err.Error())
	}
	if _, err := os.Stat(filepath.Join(bundle, "vendor/autoload.php")); err != nil {
		return fail("unavailable", "dependencies_missing", "Bundled PHPCS dependencies are unavailable beside the worker binary; rebuild or reinstall the toolset")
	}
	temporary, err := os.MkdirTemp("", "phpwarden-phpcs-")
	if err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	defer os.RemoveAll(temporary)
	script := filepath.Join(temporary, "phpcs.php")
	if err := os.WriteFile(script, helper, 0o600); err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	paths := []string{"drupal/coder/coder_sniffer", "slevomat/coding-standard", "sirbrillig/phpcs-variable-analysis", "phpcsstandards/phpcsutils", "phpcompatibility/php-compatibility"}
	for i, name := range paths {
		paths[i] = filepath.Join(bundle, "vendor", name)
	}
	for _, name := range cfg.StandardPaths {
		paths = append(paths, filepath.Join(root, filepath.FromSlash(name)))
	}
	for _, batch := range prepared.Groups {
		standards := slices.Clone(cfg.Standards)
		if batch.Config != "" {
			standards = []string{filepath.Join(root, filepath.FromSlash(batch.Config))}
		}
		if cfg.Compatibility.Enabled {
			standards = append(standards, "PHPCompatibility")
		}
		args := []string{"-q", "-s", "--report=PHPWardenJSON", "--no-colors", "--no-cache", "--parallel=1", "--basepath=" + root,
			"--error-severity=" + strconv.Itoa(cfg.ErrorSeverity), "--warning-severity=" + strconv.Itoa(cfg.WarningSeverity),
			"--runtime-set", "installed_paths", strings.Join(paths, ","), "--runtime-set", "ignore_errors_on_exit", "0",
			"--runtime-set", "ignore_warnings_on_exit", "0", "--runtime-set", "ignore_non_auto_fixable_on_exit", "0"}
		extensions := []string{}
		for _, name := range batch.Files {
			extension := strings.TrimPrefix(filepath.Ext(name), ".")
			if !slices.Contains(extensions, extension) {
				extensions = append(extensions, extension)
			}
		}
		args = append(args, "--extensions="+strings.Join(extensions, ","))
		version := cfg.Compatibility.TestVersion
		if version == "" {
			version = target.Min + "-" + target.Max
		}
		args = append(args, "--runtime-set", "testVersion", version)
		if len(cfg.Sniffs) > 0 {
			args = append(args, "--sniffs="+strings.Join(cfg.Sniffs, ","))
		}
		if len(cfg.ExcludeSniffs) > 0 {
			args = append(args, "--exclude="+strings.Join(cfg.ExcludeSniffs, ","))
		}
		files := make([]string, len(batch.Files))
		for i, name := range batch.Files {
			files[i] = filepath.Join(root, filepath.FromSlash(name))
		}
		input, _ := json.Marshal(map[string]any{"standards": standards, "args": args, "files": files})
		execution := process.Run(ctx, runner.Command{Name: "php", Directory: root, Input: input, Args: []string{"-d", "auto_prepend_file=", "-d", "auto_append_file=", "-d", "display_errors=stderr", "-d", "log_errors=0", "-d", "memory_limit=256M", script, bundle}})
		if execution.Unavailable {
			return fail("unavailable", "executable_missing", "PHP executable is unavailable in the worker")
		}
		if execution.TimedOut || ctx.Err() != nil {
			return fail("error", "checker_timeout", "PHPCS did not complete within its execution context")
		}
		if execution.Err != nil || execution.ExitCode != 0 {
			return fail("error", "process_failed", fmt.Sprintf("PHPCS helper exited %d: %s", execution.ExitCode, strings.TrimSpace(string(execution.Stderr))))
		}
		parsed, err := decode(execution.Stdout, execution.Stderr, root, batch.Files)
		if err != nil {
			return fail("error", "invalid_report", err.Error())
		}
		if result.PHPVersion != nil && *result.PHPVersion != parsed.PHPVersion {
			return fail("error", "invalid_report", "PHPCS runtime changed between ruleset groups")
		}
		result.Runtime, result.PHPVersion = &parsed.Runtime, &parsed.PHPVersion
		for name, file := range parsed.Report.Files {
			if filepath.IsAbs(name) {
				name, _ = filepath.Rel(root, name)
			}
			name = filepath.ToSlash(name)
			for _, row := range file.Messages {
				if strings.HasPrefix(row.Source, "Internal.") && !slices.Contains([]string{"Internal.NoCodeFound", "Internal.DetectLineEndings", "Internal.LineEndings.Mixed"}, row.Source) {
					return fail("error", "analysis_failed", "PHPCS could not analyze "+strconv.Quote(name)+": "+row.Message)
				}
				if !selection.Contains(name, row.Line) {
					continue
				}
				severity := "error"
				if row.Type == "WARNING" {
					severity = "warning"
				}
				result.Findings = append(result.Findings, report.Finding{Checker: "phpcs", Rule: row.Source, Kind: "code", Severity: severity, Message: row.Message, Path: new(name), Line: new(row.Line), Column: new(row.Column)})
			}
		}
	}
	for _, finding := range result.Findings {
		if cfg.FailOn != "never" && (finding.Severity == "error" || cfg.FailOn == "warning") {
			result.Check.Status = "failed"
		}
	}
	return result
}

func decode(data, stderr []byte, root string, files []string) (helperReport, error) {
	var parsed helperReport
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return parsed, err
	}
	if _, err := decoder.Token(); err != io.EOF || len(stderr) > 0 || parsed.Version != 1 || parsed.PHPCSVersion != Version || parsed.CoderVersion != CoderVersion || parsed.CompatibilityVersion != CompatibilityVersion || parsed.ExitCode < 0 || parsed.ExitCode > 3 || len(parsed.Report.Files) == 0 {
		return parsed, fmt.Errorf("expected one versioned PHPCS report from the pinned toolset")
	}
	canonical, err := phpconstraint.StableVersion(parsed.PHPVersion)
	if err != nil || canonical != parsed.PHPVersion || canonical[:strings.LastIndex(canonical, ".")] != parsed.Runtime {
		return parsed, fmt.Errorf("invalid PHPCS PHP runtime version")
	}
	count := totals{}
	seen := map[string]bool{}
	for name, file := range parsed.Report.Files {
		if filepath.IsAbs(name) {
			name, err = filepath.Rel(root, name)
			if err != nil {
				return parsed, err
			}
		}
		name = filepath.ToSlash(name)
		if !slices.Contains(files, name) || seen[name] || file.Messages == nil {
			return parsed, fmt.Errorf("PHPCS reported an unexpected file")
		}
		seen[name] = true
		errors, warnings := 0, 0
		for _, row := range file.Messages {
			if row.Line < 1 || row.Column < 1 || row.Message == "" || row.Source == "" || row.Severity < 0 || row.Type != "ERROR" && row.Type != "WARNING" {
				return parsed, fmt.Errorf("invalid PHPCS finding")
			}
			if row.Type == "ERROR" {
				errors++
			} else {
				warnings++
			}
			if row.Fixable {
				count.Fixable++
			}
		}
		if file.Errors != errors || file.Warnings != warnings {
			return parsed, fmt.Errorf("PHPCS file totals do not match findings")
		}
		count.Errors += errors
		count.Warnings += warnings
	}
	if count != parsed.Report.Totals || (parsed.ExitCode == 0) != (count.Errors+count.Warnings == 0) {
		return parsed, fmt.Errorf("PHPCS totals or exit code do not match findings")
	}
	return parsed, nil
}
