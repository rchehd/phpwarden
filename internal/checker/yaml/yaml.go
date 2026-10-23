package yamlcheck

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/phpconstraint"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

const Version = "7.4.21"

//go:embed helper.php
var helper []byte

type helperReport struct {
	Version     int    `json:"version"`
	Runtime     string `json:"runtime"`
	PHPVersion  string `json:"php_version"`
	YAMLVersion string `json:"yaml_version"`
	Findings    []struct {
		Path    string `json:"path"`
		Line    *int   `json:"line"`
		Message string `json:"message"`
	} `json:"findings"`
}

func Run(ctx context.Context, root string, selection scope.Selection, cfg config.YAML, process runner.Process) checker.Outcome {
	executable, err := os.Executable()
	autoload := ""
	if err == nil {
		autoload = filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "yaml", "vendor", "autoload.php"))
	}
	return run(ctx, root, selection, cfg, autoload, process)
}

func run(ctx context.Context, root string, selection scope.Selection, cfg config.YAML, autoload string, process runner.Process) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "yaml", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn},
		Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
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
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Message: message, Checker: new("yaml")})
		return result
	}
	if info, err := os.Stat(autoload); err != nil || !info.Mode().IsRegular() {
		return fail("unavailable", "dependencies_missing", "Bundled Symfony YAML dependencies are unavailable beside the worker binary; rebuild or reinstall the toolset")
	}
	temporary, err := os.MkdirTemp("", "phpwarden-yaml-")
	if err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	defer os.RemoveAll(temporary)
	script := filepath.Join(temporary, "yaml.php")
	if err := os.WriteFile(script, helper, 0o600); err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	input, err := json.Marshal(map[string]any{"files": selection.Files})
	if err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	execution := process.Run(ctx, runner.Command{Name: "php", Directory: root, Input: input,
		Args: []string{"-n", "-d", "display_errors=stderr", "-d", "log_errors=0", "-d", "memory_limit=256M", script, autoload}})
	if execution.Unavailable {
		return fail("unavailable", "executable_missing", "PHP executable is unavailable in the worker")
	}
	if execution.TimedOut || ctx.Err() != nil {
		return fail("error", "checker_timeout", "YAML checker did not complete within its execution context")
	}
	if execution.Err != nil || execution.ExitCode != 0 {
		return fail("error", "process_failed", fmt.Sprintf("YAML helper exited %d: %s", execution.ExitCode, strings.TrimSpace(string(execution.Stderr))))
	}
	decoder := json.NewDecoder(bytes.NewReader(execution.Stdout))
	decoder.DisallowUnknownFields()
	var parsed helperReport
	if err := decoder.Decode(&parsed); err != nil {
		return fail("error", "invalid_report", err.Error())
	}
	if _, err := decoder.Token(); err != io.EOF || parsed.Version != 1 || parsed.Findings == nil || len(execution.Stderr) != 0 {
		return fail("error", "invalid_report", "Expected one versioned YAML helper report")
	}
	canonical, err := phpconstraint.StableVersion(parsed.PHPVersion)
	if err != nil || canonical != parsed.PHPVersion || canonical[:strings.LastIndex(canonical, ".")] != parsed.Runtime || parsed.YAMLVersion != Version {
		return fail("error", "invalid_report", "YAML helper runtime or Symfony version does not match the toolset")
	}
	for _, row := range parsed.Findings {
		if _, exists := selection.LineMap[row.Path]; !exists || row.Line != nil && *row.Line < 1 || row.Message == "" {
			return fail("error", "invalid_report", "YAML finding does not match selected inputs")
		}
	}
	result.Runtime, result.PHPVersion = &parsed.Runtime, &parsed.PHPVersion
	for _, row := range parsed.Findings {
		finding := report.Finding{Checker: "yaml", Rule: "yaml.syntax", Kind: "syntax", Severity: "error", Message: row.Message, Path: &row.Path, Line: row.Line}
		if row.Line == nil {
			finding.Kind, finding.Path = "global", nil
			finding.Message = fmt.Sprintf("YAML file %q: %s", row.Path, row.Message)
		}
		result.Findings = append(result.Findings, finding)
	}
	if len(result.Findings) > 0 && cfg.FailOn != "never" {
		result.Check.Status = "failed"
	}
	return result
}
