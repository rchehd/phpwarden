package syntax

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
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
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

//go:embed parallel.php
var parallelHelper []byte

func parallelBundle() string {
	executable, _ := os.Executable()
	return filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "parallel-lint"))
}

func runParallel(ctx context.Context, directory string, selected scope.Selection, cfg config.Syntax, bundle string, process runner.Process) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "syntax", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	fail := func(status, code, message string) checker.Outcome {
		result.Check.Status, result.Check.Reason, result.Findings = status, &code, []report.Finding{}
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: new("syntax"), Message: message})
		return result
	}
	if _, err := os.Stat(filepath.Join(bundle, "vendor/autoload.php")); err != nil {
		return fail("unavailable", "dependencies_missing", "The isolated Parallel Lint bundle is unavailable")
	}
	project, err := os.OpenRoot(directory)
	if err != nil {
		return fail("error", "input_read", "Cannot read syntax sources")
	}
	defer project.Close()
	files, names := []string{}, map[string]string{}
	for _, name := range selected.Files {
		if strings.ContainsAny(name, ";\r\n") {
			return fail("error", "unsupported_input", "Parallel Lint does not support file names containing semicolons or line breaks; use the php backend")
		}
		if _, _, err := edit.Read(project, name); err != nil {
			return fail("error", "input_read", "Cannot read a selected syntax source")
		}
		absolute := filepath.Join(directory, filepath.FromSlash(name))
		files = append(files, absolute)
		names[absolute] = name
	}
	temporary, err := os.MkdirTemp("", "phpwarden-lint-")
	if err != nil {
		return fail("error", "helper_prepare", "Cannot prepare parallel syntax helper")
	}
	defer os.RemoveAll(temporary)
	script := filepath.Join(temporary, "helper.php")
	if os.WriteFile(script, parallelHelper, 0600) != nil {
		return fail("error", "helper_prepare", "Cannot write parallel syntax helper")
	}
	input, _ := json.Marshal(map[string]any{"files": files, "jobs": cfg.ParallelJobs})
	environment := []string{"HOME=" + temporary, "TMPDIR=" + temporary, "LANG=C.UTF-8"}
	for _, key := range []string{"PATH", "SYSTEMROOT", "WINDIR"} {
		if value, ok := os.LookupEnv(key); ok {
			environment = append(environment, key+"="+value)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	execution := process.Run(ctx, runner.Command{Name: "php", Args: []string{"-n", "-d", "display_errors=stderr", "-d", "log_errors=0", script, bundle}, Directory: directory, Env: environment, Input: input})
	if execution.Unavailable {
		return fail("unavailable", "executable_missing", "PHP is unavailable in the worker")
	}
	if execution.TimedOut || ctx.Err() != nil {
		return fail("error", "checker_timeout", "Parallel syntax checking exceeded its execution context")
	}
	if execution.Err != nil || execution.ExitCode != 0 || len(execution.Stderr) > 0 {
		return fail("error", "process_failed", "Parallel syntax checking failed or skipped selected files; no partial findings accepted")
	}
	var response struct {
		Version int      `json:"version"`
		Tool    string   `json:"tool_version"`
		PHP     string   `json:"php_version"`
		Checked []string `json:"checked"`
		Errors  []struct {
			File    string `json:"file"`
			Line    int    `json:"line"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	decoder := json.NewDecoder(bytes.NewReader(execution.Stdout))
	decoder.DisallowUnknownFields()
	valid := len(execution.Stdout) <= 32<<20 && decoder.Decode(&response) == nil
	_, tail := decoder.Token()
	version, versionErr := phpconstraint.StableVersion(response.PHP)
	expected := slices.Clone(files)
	slices.Sort(expected)
	slices.Sort(response.Checked)
	if !valid || tail != io.EOF || response.Version != 1 || response.Tool != "1.4.0" || versionErr != nil || version != response.PHP || response.Errors == nil || !slices.Equal(response.Checked, expected) {
		return fail("error", "invalid_report", "Parallel Lint returned an invalid or incomplete report")
	}
	minor := version[:strings.LastIndex(version, ".")]
	result.Runtime, result.PHPVersion = &minor, &version
	seen := map[string]bool{}
	for _, row := range response.Errors {
		name, ok := names[row.File]
		if !ok || seen[row.File] || row.Line < 1 || strings.TrimSpace(row.Message) == "" || strings.Contains(row.Message, "Allowed memory size") || strings.Contains(row.Message, "Maximum execution time") || strings.Contains(row.Message, "Out of memory") {
			return fail("error", "invalid_report", "Parallel Lint returned an invalid syntax finding")
		}
		seen[row.File] = true
		result.Findings = append(result.Findings, report.Finding{Checker: "syntax", Rule: "php.syntax", Kind: "syntax", Severity: "error", Message: strings.ReplaceAll(row.Message, row.File, name), Path: &name, Line: &row.Line})
	}
	if len(result.Findings) > 0 && cfg.FailOn != "never" {
		result.Check.Status = "failed"
	}
	return result
}
