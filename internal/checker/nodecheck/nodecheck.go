package nodecheck

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
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

//go:embed helper.cjs
var helper []byte

type message struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Kind     string `json:"kind"`
	Text     string `json:"text"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	EndLine  int    `json:"end_line"`
}

type fileResult struct {
	Path     string    `json:"path"`
	Ignored  bool      `json:"ignored"`
	Messages []message `json:"messages"`
}

type helperReport struct {
	Version     int          `json:"version"`
	Tool        string       `json:"tool"`
	ToolVersion string       `json:"tool_version"`
	Files       []fileResult `json:"files"`
	Error       string       `json:"error"`
	Code        string       `json:"code"`
}

func Run(ctx context.Context, root string, selection scope.Selection, cfg config.Config, id string, process runner.Process) checker.Outcome {
	executable, _ := os.Executable()
	bundle := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "node"))
	return run(ctx, root, selection, cfg, id, bundle, process)
}

func run(ctx context.Context, root string, selection scope.Selection, cfg config.Config, id, bundle string, process runner.Process) checker.Outcome {
	settings := Settings(cfg, id)
	result := checker.Outcome{Check: report.Check{ID: id, Status: "passed", Required: settings.Required, FailOn: settings.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	selected := Select(selection, settings, id)
	skip := func(reason string) checker.Outcome {
		result.Check.Status, result.Check.Reason = "skipped", &reason
		return result
	}
	if !settings.Enabled {
		return skip("disabled")
	}
	if len(selected.Files) == 0 {
		return skip("no_files")
	}
	fail := func(status, code, text string) checker.Outcome {
		result.Check.Status, result.Check.Reason, result.Findings = status, &code, []report.Finding{}
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: new(id), Message: text})
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(settings.TimeoutSeconds)*time.Second)
	defer cancel()
	var prepared []group
	var err error
	analysisFiles := selected.Files
	if id == "jscpd" {
		all, err := inventory(ctx, root, cfg)
		if err != nil {
			return fail("error", "input_read", err.Error())
		}
		analysisFiles = Select(all, settings, id).Files
		selectedConfig := settings.Config
		if selectedConfig == "" && settings.DiscoverConfig {
			selectedConfig, err = findConfig(root, ".", cfg.ComposerRoot, id)
			if err != nil {
				return fail("error", "config_invalid", err.Error())
			}
		}
		if selectedConfig != "" {
			if err := regular(root, selectedConfig); err != nil {
				return fail("error", "config_invalid", err.Error())
			}
		}
		prepared = []group{{Config: selectedConfig, Files: analysisFiles}}
	} else {
		prepared, err = groups(root, cfg.ComposerRoot, id, selected, settings)
		if err != nil {
			return fail("error", "config_invalid", err.Error())
		}
	}
	active := []group{}
	for _, batch := range prepared {
		if batch.Config == "" && id != "jscpd" {
			result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "no_config", Stage: "checker", Checker: new(id), Message: fmt.Sprintf("No %s config for %d selected file(s), starting with %q", id, len(batch.Files), batch.Files[0])})
			continue
		}
		active = append(active, batch)
	}
	if len(active) == 0 {
		return skip("no_config")
	}
	if id != "jscpd" {
		analysisFiles = []string{}
		for _, batch := range active {
			analysisFiles = append(analysisFiles, batch.Files...)
		}
	}
	temporary, err := os.MkdirTemp("", "phpwarden-node-")
	if err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	defer os.RemoveAll(temporary)
	filename := filepath.Join(temporary, "helper.cjs")
	if err := os.WriteFile(filename, helper, 0o600); err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	request, err := json.Marshal(struct {
		ID           string       `json:"id"`
		Root         string       `json:"root"`
		ComposerRoot string       `json:"composer_root"`
		Bundle       string       `json:"bundle"`
		Groups       []group      `json:"groups"`
		Duplicates   config.JSCPD `json:"duplicates"`
	}{id, root, cfg.ComposerRoot, bundle, active, cfg.Tools.JSCPD})
	if err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	execution := process.Run(ctx, runner.Command{Name: "node", Args: []string{filename}, Directory: root, Input: request, Env: cleanEnvironment(temporary)})
	if execution.Unavailable {
		return fail("unavailable", "executable_missing", "Node.js is unavailable beside the worker; rebuild or reinstall the toolset")
	}
	if execution.TimedOut || ctx.Err() != nil {
		return fail("error", "checker_timeout", id+" did not complete within its execution context")
	}
	if execution.Err != nil || execution.ExitCode != 0 || len(execution.Stderr) > 0 {
		return fail("error", "process_failed", fmt.Sprintf("%s exited %d: %s", id, execution.ExitCode, strings.TrimSpace(string(execution.Stderr))))
	}
	parsed, err := decode(execution.Stdout, id, root, analysisFiles)
	if err != nil {
		return fail("error", "invalid_report", err.Error())
	}
	if parsed.Error != "" {
		status := "error"
		if parsed.Code == "dependencies_missing" || parsed.Code == "version_mismatch" {
			status = "unavailable"
		}
		return fail(status, parsed.Code, parsed.Error)
	}
	for _, file := range parsed.Files {
		name := relative(root, file.Path)
		for _, row := range file.Messages {
			line := row.Line
			if id == "jscpd" {
				line = overlappingLine(selected, name, row.Line, row.EndLine)
			} else if row.Kind != "syntax" && !selected.Contains(name, line) {
				continue
			}
			if line == 0 {
				continue
			}
			result.Findings = append(result.Findings, report.Finding{Checker: id, Rule: row.Rule, Kind: row.Kind, Severity: row.Severity,
				Message: row.Text, Path: new(name), Line: new(line), Column: new(row.Column)})
			if settings.FailOn != "never" && (row.Severity == "error" || settings.FailOn == "warning" && row.Severity == "warning") {
				result.Check.Status = "failed"
			}
		}
	}
	return result
}

func overlappingLine(selection scope.Selection, name string, start, end int) int {
	ranges, ok := selection.LineMap[name]
	if !ok {
		return 0
	}
	if ranges == nil {
		return start
	}
	for _, interval := range ranges {
		if interval.Start <= end && interval.End >= start {
			return max(start, interval.Start)
		}
	}
	return 0
}

func relative(root, name string) string {
	if filepath.IsAbs(name) {
		if rel, err := filepath.Rel(root, name); err == nil {
			name = rel
		}
	}
	return filepath.ToSlash(name)
}

func decode(data []byte, id, root string, files []string) (helperReport, error) {
	var parsed helperReport
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return parsed, err
	}
	if _, err := decoder.Token(); err != io.EOF || parsed.Version != 1 || parsed.Tool != id || parsed.Files == nil {
		return parsed, fmt.Errorf("expected one versioned %s report", id)
	}
	if parsed.Error != "" {
		if len(parsed.Files) != 0 || !slices.Contains([]string{"dependencies_missing", "version_mismatch", "config_invalid", "analysis_failed"}, parsed.Code) {
			return parsed, fmt.Errorf("invalid %s failure report", id)
		}
		return parsed, nil
	}
	if parsed.Code != "" || parsed.ToolVersion == "" || id == "jscpd" && parsed.ToolVersion != "4.3.0" {
		return parsed, fmt.Errorf("invalid %s tool identity", id)
	}
	seen := map[string]bool{}
	for _, file := range parsed.Files {
		name := relative(root, file.Path)
		if !slices.Contains(files, name) || seen[name] || file.Messages == nil || file.Ignored && len(file.Messages) > 0 {
			return parsed, fmt.Errorf("%s reported an unexpected file", id)
		}
		seen[name] = true
		for _, row := range file.Messages {
			if row.Rule == "" || row.Text == "" || row.Line < 1 || row.Column < 1 || row.EndLine < row.Line ||
				!slices.Contains([]string{"error", "warning"}, row.Severity) ||
				(row.Kind != "code" && row.Kind != "syntax") || id == "jscpd" && (row.Kind != "code" || row.Severity != "warning") {
				return parsed, fmt.Errorf("invalid %s finding", id)
			}
		}
	}
	if len(seen) != len(files) {
		return parsed, fmt.Errorf("%s omitted selected input files", id)
	}
	return parsed, nil
}

func cleanEnvironment(temporary string) []string {
	env := []string{"HOME=" + temporary, "XDG_CACHE_HOME=" + temporary, "TMPDIR=" + temporary, "NO_COLOR=1"}
	for _, name := range []string{"PATH", "SYSTEMROOT", "WINDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}
	return env
}
