package deptrac

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

const Version = "4.4.0"
const PHPStanVersion = "2.3.1"

//go:embed helper.php
var helper []byte

type finding struct {
	File      string  `json:"file"`
	Line      int     `json:"line"`
	Kind      string  `json:"kind"`
	Depender  string  `json:"depender"`
	Dependent string  `json:"dependent"`
	From      string  `json:"from_layer"`
	To        *string `json:"to_layer"`
}

type response struct {
	Version  int               `json:"version"`
	Runtime  string            `json:"runtime"`
	PHP      string            `json:"php_version"`
	Tools    map[string]string `json:"tools"`
	Files    []string          `json:"files"`
	Counts   map[string]int    `json:"counts"`
	Errors   *int              `json:"errors"`
	Warnings []string          `json:"warnings"`
	Findings []finding         `json:"findings"`
}

func Run(ctx context.Context, root, composerRoot string, selected scope.Selection, cfg config.Deptrac, process runner.Process) checker.Outcome {
	executable, _ := os.Executable()
	bundle := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "deptrac"))
	return run(ctx, root, composerRoot, selected, cfg, bundle, process)
}

func run(ctx context.Context, root, composerRoot string, selected scope.Selection, cfg config.Deptrac, bundle string, process runner.Process) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "deptrac", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	skipped := func(reason string) checker.Outcome {
		result.Check.Status, result.Check.Reason = "skipped", &reason
		return result
	}
	fail := func(status, code, message string) checker.Outcome {
		result.Check.Status, result.Check.Reason, result.Findings = status, &code, []report.Finding{}
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: new("deptrac"), Message: message})
		return result
	}
	if !cfg.Enabled {
		return skipped("disabled")
	}
	selected = Select(selected, cfg)
	if len(selected.Files) == 0 {
		return skipped("no_files")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return fail("error", "checker_timeout", "Deptrac execution context expired")
	}
	filename, err := prepare(root, composerRoot, cfg)
	if err != nil {
		return fail("error", "config_invalid", err.Error())
	}
	if filename == "" {
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "architecture_config_missing", Stage: "checker", Checker: new("deptrac"), Message: "Architecture rules are not configured. Install the architecture skill with 'phpwarden init --agents all --no-interaction', then invoke phpwarden-architecture in your agent, or create a native Deptrac YAML config."})
		return skipped("no_config")
	}
	if info, err := os.Stat(filepath.Join(bundle, "vendor/autoload.php")); err != nil || !info.Mode().IsRegular() {
		return fail("unavailable", "dependencies_missing", "The isolated Deptrac bundle is unavailable; rebuild the pinned toolset")
	}
	temporary, err := os.MkdirTemp("", "phpwarden-deptrac-")
	if err != nil {
		return fail("error", "helper_prepare", "Cannot prepare the private Deptrac helper")
	}
	defer os.RemoveAll(temporary)
	script := filepath.Join(temporary, "helper.php")
	if err := os.WriteFile(script, helper, 0o600); err != nil {
		return fail("error", "helper_prepare", "Cannot write the private Deptrac helper")
	}
	environment := []string{"HOME=" + temporary, "TMPDIR=" + temporary, "LANG=C.UTF-8"}
	for _, name := range []string{"PATH", "SYSTEMROOT", "WINDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	input, _ := json.Marshal(map[string]string{"config": filename, "project_root": root})
	execution := process.Run(ctx, runner.Command{Name: "php", Args: []string{"-d", "auto_prepend_file=", "-d", "auto_append_file=", "-d", "display_errors=stderr", "-d", "log_errors=0", "-d", fmt.Sprintf("memory_limit=%dM", cfg.MemoryLimitMB), script, bundle}, Directory: root, Env: environment, Input: input})
	if execution.Unavailable {
		return fail("unavailable", "executable_missing", "PHP is unavailable in the worker")
	}
	if execution.TimedOut || ctx.Err() != nil {
		return fail("error", "checker_timeout", "Deptrac did not complete within its execution context")
	}
	if execution.ExitCode != 0 || execution.Err != nil || len(execution.Stderr) > 0 {
		return fail("error", "process_failed", "Deptrac could not complete its architecture analysis; no partial findings accepted")
	}
	parsed, err := decode(execution.Stdout, root)
	if err != nil {
		return fail("error", "invalid_report", err.Error())
	}
	result.Runtime, result.PHPVersion = &parsed.Runtime, &parsed.PHP
	if !slices.ContainsFunc(selected.Files, func(name string) bool { return slices.Contains(parsed.Files, name) }) {
		return skipped("outside_graph")
	}
	for _, row := range parsed.Findings {
		if !selected.Contains(row.File, row.Line) {
			continue
		}
		severity, message := "error", fmt.Sprintf("%s must not depend on %s (%s on %s)", row.Depender, row.Dependent, row.From, value(row.To))
		switch row.Kind {
		case "uncovered":
			if !cfg.ReportUncovered && !cfg.FailOnUncovered {
				continue
			}
			severity, message = "warning", fmt.Sprintf("%s has an uncovered dependency on %s (%s)", row.Depender, row.Dependent, row.From)
			if cfg.FailOnUncovered {
				severity = "error"
			}
		case "skipped":
			if !cfg.ReportSkipped {
				continue
			}
			severity, message = "info", fmt.Sprintf("Accepted dependency: %s on %s (%s on %s)", row.Depender, row.Dependent, row.From, value(row.To))
		}
		result.Findings = append(result.Findings, report.Finding{Checker: "deptrac", Rule: "deptrac." + row.Kind, Kind: "code", Severity: severity, Message: message, Path: new(row.File), Line: new(row.Line)})
	}
	for _, warning := range parsed.Warnings {
		result.Findings = append(result.Findings, report.Finding{Checker: "deptrac", Rule: "deptrac.configuration_warning", Kind: "global", Severity: "warning", Message: "Deptrac configuration: " + warning})
	}
	for _, item := range result.Findings {
		if cfg.FailOn != "never" && (item.Severity == "error" || cfg.FailOn == "warning" && item.Severity == "warning") {
			result.Check.Status = "failed"
		}
	}
	return result
}

func value(text *string) string {
	if text == nil {
		return ""
	}
	return *text
}

func decode(data []byte, directory string) (response, error) {
	failure := func() (response, error) {
		return response{}, fmt.Errorf("Deptrac returned an invalid, incomplete or out-of-project report")
	}
	var result response
	if len(data) > 32<<20 {
		return failure()
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil {
		return failure()
	}
	want := map[string]string{"deptrac/deptrac": Version, "phpstan/phpstan": PHPStanVersion}
	if _, err := decoder.Token(); err != io.EOF || result.Version != 1 || result.Errors == nil || *result.Errors != 0 || result.Files == nil || result.Findings == nil || result.Warnings == nil || !reflect.DeepEqual(want, result.Tools) || result.Runtime == "" || !strings.HasPrefix(result.PHP, result.Runtime+".") || len(result.Counts) != 4 {
		return failure()
	}
	counts := map[string]int{"violation": 0, "uncovered": 0, "skipped": 0, "allowed": result.Counts["allowed"]}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return failure()
	}
	defer root.Close()
	lines := map[string]int{}
	previous := ""
	for _, name := range result.Files {
		if name <= previous || path.Clean(name) != name || path.Ext(name) != ".php" {
			return failure()
		}
		data, _, err := edit.Read(root, name)
		if err != nil {
			return failure()
		}
		lines[name], previous = bytes.Count(data, []byte{'\n'})+1, name
	}
	for _, row := range result.Findings {
		if row.Kind != "violation" && row.Kind != "uncovered" && row.Kind != "skipped" || row.Line < 1 || row.Line > lines[row.File] || strings.TrimSpace(row.Depender) == "" || strings.TrimSpace(row.Dependent) == "" || strings.TrimSpace(row.From) == "" || row.Kind == "uncovered" && row.To != nil || row.Kind != "uncovered" && (row.To == nil || strings.TrimSpace(*row.To) == "") {
			return failure()
		}
		counts[row.Kind]++
	}
	if counts["allowed"] < 0 || !reflect.DeepEqual(result.Counts, counts) {
		return failure()
	}
	for _, warning := range result.Warnings {
		if strings.TrimSpace(warning) == "" {
			return failure()
		}
	}
	return result, nil
}
