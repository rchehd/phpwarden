package debug

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
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/profile"
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

// Select keeps PHP, JavaScript (not minified) and Twig files.
func Select(selection scope.Selection, cfg config.Debug, profileID string) scope.Selection {
	defaults, _ := profile.Lookup(profileID)
	return selection.Filter(cfg.Include, cfg.Exclude, func(filename string) bool {
		extension := strings.ToLower(path.Ext(filename))
		return slices.Contains(defaults.PHPExtensions, extension) || slices.Contains(defaults.TwigExtensions, extension) ||
			slices.Contains(defaults.JSExtensions, extension) && !strings.HasSuffix(strings.ToLower(filename), ".min"+extension)
	})
}

func language(filename string) string {
	switch strings.ToLower(path.Ext(filename)) {
	case ".js":
		return "javascript"
	case ".twig":
		return "twig"
	}
	return "php"
}

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
		result.Check.Status, result.Check.Reason, result.Findings = status, &code, []report.Finding{}
		checker := "debug"
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Message: message, Checker: &checker})
		return result
	}
	var phpFiles []string
	for _, filename := range selection.Files {
		if language(filename) == "php" {
			phpFiles = append(phpFiles, filename)
			continue
		}
		source, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(filename)))
		if err != nil {
			return fail("error", "read_failed", fmt.Sprintf("read %s: %v", filename, err))
		}
		for _, finding := range sourceFindings(filename, source, cfg) {
			if selection.Contains(filename, *finding.Line) {
				result.Findings = append(result.Findings, finding)
			}
		}
	}
	if len(phpFiles) > 0 {
		outcome, ok := runPHP(ctx, root, selection, phpFiles, cfg, process, fail)
		if !ok {
			return outcome
		}
		result.Findings = append(result.Findings, outcome.Findings...)
		result.Runtime, result.PHPVersion = outcome.Runtime, outcome.PHPVersion
	}
	if len(result.Findings) > 0 && cfg.FailOn != "never" {
		result.Check.Status = "failed"
	}
	return result
}

func sourceFindings(filename string, source []byte, cfg config.Debug) []report.Finding {
	var result []report.Finding
	add := func(line int, rule, message string) {
		result = append(result, report.Finding{Checker: "debug", Rule: rule, Kind: "code", Severity: "error", Message: message, Path: &filename, Line: &line})
	}
	if language(filename) == "twig" {
		for _, call := range calls(maskTwig(source), cfg.TwigFunctions) {
			add(call.Line, "debug.twig_call", "Debug function "+call.Name+"() is called in a Twig template")
		}
		return result
	}
	masked := maskJS(source)
	for _, call := range calls(masked, cfg.JSFunctions) {
		add(call.Line, "debug.js_call", "Debug call "+call.Name+"() is left in JavaScript")
	}
	for _, line := range debuggers(masked) {
		add(line, "debug.debugger", "debugger statement is left in JavaScript")
	}
	return result
}

// runPHP returns the helper findings, or the failed outcome with ok false.
func runPHP(ctx context.Context, root string, selection scope.Selection, files []string, cfg config.Debug, process runner.Process,
	fail func(status, code, message string) checker.Outcome) (checker.Outcome, bool) {
	result := checker.Outcome{}
	temporary, err := os.MkdirTemp("", "phpwarden-debug-")
	if err != nil {
		return fail("error", "helper_prepare", err.Error()), false
	}
	defer os.RemoveAll(temporary)
	script := filepath.Join(temporary, "debug.php")
	if err := os.WriteFile(script, helper, 0o600); err != nil {
		return fail("error", "helper_prepare", err.Error()), false
	}
	input, err := json.Marshal(map[string]any{"files": files, "functions": cfg.Functions})
	if err != nil {
		return fail("error", "helper_prepare", err.Error()), false
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	execution := process.Run(ctx, runner.Command{Name: "php", Args: []string{"-d", "auto_prepend_file=", "-d", "auto_append_file=", "-d", "display_errors=stderr", script}, Directory: root, Input: input})
	if execution.Unavailable {
		return fail("unavailable", "executable_missing", "PHP executable is unavailable in the worker"), false
	}
	if execution.TimedOut {
		return fail("error", "checker_timeout", "PHP debug checker timed out"), false
	}
	if execution.Err != nil || execution.ExitCode != 0 {
		return fail("error", "process_failed", fmt.Sprintf("PHP helper exited %d: %s", execution.ExitCode, strings.TrimSpace(string(execution.Stderr)))), false
	}
	decoder := json.NewDecoder(bytes.NewReader(execution.Stdout))
	decoder.DisallowUnknownFields()
	var parsed helperReport
	if err := decoder.Decode(&parsed); err != nil {
		return fail("error", "invalid_report", err.Error()), false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF || parsed.Version != 1 || !phpMinor.MatchString(parsed.Runtime) || parsed.Findings == nil {
		return fail("error", "invalid_report", "Expected one versioned PHP helper report with runtime and findings array"), false
	}
	allowed := make(map[string]bool)
	for _, name := range cfg.Functions {
		allowed[strings.ToLower(name)] = true
	}
	for _, row := range parsed.Findings {
		if _, exists := selection.LineMap[row.Path]; !exists || row.Line < 1 || !allowed[row.Function] {
			return fail("error", "invalid_report", "Helper finding does not match selected files or configured functions"), false
		}
	}
	result.Runtime = &parsed.Runtime
	if parsed.PHPVersion != "" {
		if !strings.HasPrefix(parsed.PHPVersion, parsed.Runtime+".") {
			return fail("error", "invalid_report", "PHP patch version does not match reported minor"), false
		}
		result.PHPVersion = &parsed.PHPVersion
	}
	for _, row := range parsed.Findings {
		if selection.Contains(row.Path, row.Line) {
			result.Findings = append(result.Findings, report.Finding{Checker: "debug", Rule: "debug.call", Kind: "code", Severity: "error", Message: "Debug function " + row.Function + "() is called", Path: &row.Path, Line: &row.Line})
		}
	}
	return result, true
}
