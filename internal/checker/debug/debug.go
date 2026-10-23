package debug

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

//go:embed helper.php
var helper []byte

type helperReport struct {
	Version    int    `json:"version"`
	Runtime    string `json:"runtime"`
	PHPVersion string `json:"php_version,omitempty"`
	Findings   []struct {
		Path     string `json:"path"`
		Line     int    `json:"line"`
		Function string `json:"function"`
	} `json:"findings"`
}

var phpMinor = regexp.MustCompile(`^[1-9][0-9]*\.[0-9]+$`)

func Run(ctx context.Context, root string, selection scope.Selection, cfg config.Debug, process runner.Process) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "debug", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	if !cfg.Enabled || len(selection.Files) == 0 {
		reason := "no_files"
		if !cfg.Enabled {
			reason = "disabled"
		}
		result.Check.Status, result.Check.Reason = "skipped", &reason
		return result
	}
	fail := func(status, code, message string) checker.Outcome {
		result.Check.Status, result.Check.Reason = status, &code
		checker := "debug"
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Message: message, Checker: &checker})
		return result
	}
	temporary, err := os.MkdirTemp("", "phpwarden-debug-")
	if err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	defer os.RemoveAll(temporary)
	script := filepath.Join(temporary, "debug.php")
	if err := os.WriteFile(script, helper, 0o600); err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	input, err := json.Marshal(map[string]any{"files": selection.Files, "functions": cfg.Functions})
	if err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	execution := process.Run(ctx, runner.Command{Name: "php", Args: []string{"-d", "auto_prepend_file=", "-d", "auto_append_file=", "-d", "display_errors=stderr", script}, Directory: root, Input: input})
	if execution.Unavailable {
		return fail("unavailable", "executable_missing", "PHP executable is unavailable in the worker")
	}
	if execution.TimedOut {
		return fail("error", "checker_timeout", "PHP debug checker timed out")
	}
	if execution.Err != nil || execution.ExitCode != 0 {
		return fail("error", "process_failed", fmt.Sprintf("PHP helper exited %d: %s", execution.ExitCode, strings.TrimSpace(string(execution.Stderr))))
	}
	decoder := json.NewDecoder(bytes.NewReader(execution.Stdout))
	decoder.DisallowUnknownFields()
	var parsed helperReport
	if err := decoder.Decode(&parsed); err != nil {
		return fail("error", "invalid_report", err.Error())
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF || parsed.Version != 1 || !phpMinor.MatchString(parsed.Runtime) || parsed.Findings == nil {
		return fail("error", "invalid_report", "Expected one versioned PHP helper report with runtime and findings array")
	}
	allowed := make(map[string]bool)
	for _, name := range cfg.Functions {
		allowed[strings.ToLower(name)] = true
	}
	for _, row := range parsed.Findings {
		if _, exists := selection.LineMap[row.Path]; !exists || row.Line < 1 || !allowed[row.Function] {
			return fail("error", "invalid_report", "Helper finding does not match selected files or configured functions")
		}
	}
	result.Runtime = &parsed.Runtime
	if parsed.PHPVersion != "" {
		if !strings.HasPrefix(parsed.PHPVersion, parsed.Runtime+".") {
			return fail("error", "invalid_report", "PHP patch version does not match reported minor")
		}
		result.PHPVersion = &parsed.PHPVersion
	}
	for _, row := range parsed.Findings {
		if selection.Contains(row.Path, row.Line) {
			result.Findings = append(result.Findings, report.Finding{Checker: "debug", Rule: "debug.call", Kind: "code", Severity: "error", Message: "Debug function " + row.Function + "() is called", Path: &row.Path, Line: &row.Line})
		}
	}
	if len(result.Findings) > 0 && cfg.FailOn != "never" {
		result.Check.Status = "failed"
	}
	return result
}
