package gitleaks

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

const Version = "8.30.1"
const findingsExit = 23

var ruleID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type finding struct {
	RuleID      string
	Description string
	StartLine   int
	EndLine     int
	StartColumn int
	EndColumn   int
	Match       string
	Secret      string
	File        string
	SymlinkFile string
	Commit      string
	Link        string
	Entropy     float64
	Author      string
	Email       string
	Date        string
	Message     string
	Tags        []string
	Fingerprint string
	Fragment    json.RawMessage
}

func Run(ctx context.Context, root, composerRoot string, selection scope.Selection, cfg config.Gitleaks, process runner.Process) checker.Outcome {
	executable, _ := os.Executable()
	return run(ctx, root, composerRoot, selection, cfg, filepath.Join(filepath.Dir(executable), "gitleaks"), process)
}

func run(ctx context.Context, root, composerRoot string, selection scope.Selection, cfg config.Gitleaks, binary string, process runner.Process) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "gitleaks", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	selected := Select(selection, cfg)
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
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: new("gitleaks"), Message: message})
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	prepared, err := prepare(root, composerRoot, cfg)
	if err != nil {
		return fail("error", "config_invalid", "Gitleaks config, baseline, ignore file or extra input is missing, unreadable or unsafe")
	}
	temporary, err := os.MkdirTemp("", "phpwarden-gitleaks-")
	if err != nil {
		return fail("error", "helper_prepare", "Cannot prepare a private directory for Gitleaks")
	}
	defer os.RemoveAll(temporary)
	environment := []string{"HOME=" + temporary, "XDG_CACHE_HOME=" + temporary, "TMPDIR=" + temporary, "NO_COLOR=1"}
	for _, name := range []string{"PATH", "SYSTEMROOT", "WINDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	invoke := func(args []string) runner.Result {
		return process.Run(ctx, runner.Command{Name: binary, Args: args, Directory: root, Env: environment})
	}
	version := invoke([]string{"version"})
	if version.Unavailable || version.Err != nil && os.IsNotExist(version.Err) {
		return fail("unavailable", "executable_missing", "Gitleaks is unavailable beside the worker; rebuild or reinstall the toolset")
	}
	if version.TimedOut || ctx.Err() != nil {
		return fail("error", "checker_timeout", "Gitleaks did not complete within its execution context")
	}
	if version.Err != nil || version.ExitCode != 0 || len(version.Stderr) > 0 {
		return fail("error", "process_failed", "Gitleaks version probe failed; native output is suppressed to protect secrets")
	}
	if strings.TrimPrefix(strings.TrimSpace(string(version.Stdout)), "v") != Version {
		return fail("unavailable", "version_mismatch", "Gitleaks version does not match the pinned toolset")
	}
	absolute := func(name string) string { return filepath.Join(root, filepath.FromSlash(name)) }
	configFile := filepath.Join(temporary, "default.toml")
	if prepared.Config != "" {
		configFile = absolute(prepared.Config)
	} else if err := os.WriteFile(configFile, []byte("[extend]\nuseDefault = true\n"), 0o600); err != nil {
		return fail("error", "helper_prepare", "Cannot prepare the default Gitleaks configuration")
	}
	ignoreFile := filepath.Join(temporary, "empty.ignore")
	if prepared.Ignore != "" {
		ignoreFile = absolute(prepared.Ignore)
	} else if err := os.WriteFile(ignoreFile, nil, 0o600); err != nil {
		return fail("error", "helper_prepare", "Cannot prepare Gitleaks ignore configuration")
	}
	reportFile := filepath.Join(temporary, "findings.json")
	for _, name := range selected.Files {
		if err := regular(root, name); err != nil {
			return fail("error", "input_read", "A selected Gitleaks input is missing, unreadable or unsafe")
		}
		if err := os.Remove(reportFile); err != nil && !os.IsNotExist(err) {
			return fail("error", "helper_prepare", "Cannot reset the private Gitleaks report")
		}
		args := []string{"dir", "--config=" + configFile, "--gitleaks-ignore-path=" + ignoreFile, "--report-format=json", "--report-path=" + reportFile,
			"--redact=100", "--no-banner", "--no-color", "--log-level=error", "--exit-code=" + strconv.Itoa(findingsExit), "--timeout=" + strconv.Itoa(cfg.TimeoutSeconds),
			"--max-archive-depth=0", "--max-decode-depth=0", "--max-target-megabytes=0"}
		if prepared.Baseline != "" {
			args = append(args, "--baseline-path="+absolute(prepared.Baseline))
		}
		for _, id := range cfg.Rules {
			args = append(args, "--enable-rule="+id)
		}
		execution := invoke(append(args, "--", name))
		if execution.TimedOut || ctx.Err() != nil {
			return fail("error", "checker_timeout", "Gitleaks did not complete within its execution context")
		}
		if execution.Unavailable || execution.Err != nil && os.IsNotExist(execution.Err) {
			return fail("unavailable", "executable_missing", "Gitleaks became unavailable during the scan")
		}
		if execution.ExitCode != 0 && execution.ExitCode != findingsExit || execution.Err != nil && execution.ExitCode != findingsExit || len(execution.Stderr) > 0 || len(execution.Stdout) > 0 {
			return fail("error", "process_failed", "Gitleaks could not complete the scan; native output is suppressed to protect secrets")
		}
		data, err := os.ReadFile(reportFile)
		if err != nil {
			return fail("error", "invalid_report", "Gitleaks did not produce a readable private report")
		}
		rows, ok := decode(data, execution.ExitCode, name)
		if !ok {
			return fail("error", "invalid_report", "Gitleaks returned an invalid, unredacted or out-of-scope report")
		}
		for _, row := range rows {
			line := overlappingLine(selected, name, row.StartLine, row.EndLine)
			if line == 0 {
				continue
			}
			result.Findings = append(result.Findings, report.Finding{Checker: "gitleaks", Rule: row.RuleID, Kind: "code", Severity: "error",
				Message: "Potential secret detected; value and source text are redacted", Path: new(name), Line: new(line), Column: new(max(1, row.StartColumn))})
		}
	}
	if len(result.Findings) > 0 && cfg.FailOn != "never" {
		result.Check.Status = "failed"
	}
	return result
}

func decode(data []byte, exitCode int, name string) ([]finding, bool) {
	var rows []finding
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rows); err != nil {
		return nil, false
	}
	if _, err := decoder.Token(); err != io.EOF || (exitCode == 0) != (len(rows) == 0) {
		return nil, false
	}
	for _, row := range rows {
		if filepath.ToSlash(filepath.Clean(row.File)) != name || !ruleID.MatchString(row.RuleID) || row.SymlinkFile != "" || row.Commit != "" ||
			row.StartLine < 0 || row.EndLine < row.StartLine || row.StartColumn < 0 || row.EndColumn < 0 ||
			row.StartLine == 0 && row.EndLine != 0 || row.Secret != "" && row.Secret != "REDACTED" {
			return nil, false
		}
	}
	return rows, true
}

func overlappingLine(selection scope.Selection, name string, start, end int) int {
	ranges, exists := selection.LineMap[name]
	if !exists {
		return 0
	}
	if ranges == nil {
		return max(1, start)
	}
	if start == 0 && len(ranges) > 0 {
		return ranges[0].Start
	}
	for _, interval := range ranges {
		if interval.Start <= end && interval.End >= start {
			return max(start, interval.Start)
		}
	}
	return 0
}
