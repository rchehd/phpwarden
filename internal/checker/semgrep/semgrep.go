package semgrep

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/edit"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
	"go.yaml.in/yaml/v3"
)

const Version = "1.182.0"
const maxSourceBytes = 16 << 20

var versionOutput = regexp.MustCompile(`^semgrep-core version: 1\.182\.0 \([^\r\n]+\)\n?$`)

type position struct {
	Line   int `json:"line"`
	Col    int `json:"col"`
	Offset int `json:"offset"`
}

type match struct {
	Rule  string   `json:"check_id"`
	Path  string   `json:"path"`
	Start position `json:"start"`
	End   position `json:"end"`
	Extra struct {
		Engine     string          `json:"engine_kind"`
		Ignored    *bool           `json:"is_ignored"`
		Metavars   json.RawMessage `json:"metavars"`
		Message    json.RawMessage `json:"message"`
		Metadata   json.RawMessage `json:"metadata"`
		Severity   json.RawMessage `json:"severity"`
		Fix        json.RawMessage `json:"fix"`
		Trace      json.RawMessage `json:"dataflow_trace"`
		SCA        json.RawMessage `json:"sca_match"`
		Validation json.RawMessage `json:"validation_state"`
		Historical json.RawMessage `json:"historical_info"`
		Extra      json.RawMessage `json:"extra_extra"`
	} `json:"extra"`
}

type coreReport struct {
	Version string            `json:"version"`
	Results []match           `json:"results"`
	Errors  []json.RawMessage `json:"errors"`
	Paths   struct {
		Scanned []string          `json:"scanned"`
		Skipped []json.RawMessage `json:"skipped"`
	} `json:"paths"`
	SkippedRules  []json.RawMessage `json:"skipped_rules"`
	RulesByEngine [][]string        `json:"rules_by_engine"`
	Engine        string            `json:"engine_requested"`
	Interfile     []string          `json:"interfile_languages_used"`
	Time          json.RawMessage   `json:"time"`
	Explanations  json.RawMessage   `json:"explanations"`
	Symbols       json.RawMessage   `json:"symbol_analysis"`
	Subprojects   json.RawMessage   `json:"subprojects"`
	MCP           json.RawMessage   `json:"mcp_scan_results"`
	Profiling     json.RawMessage   `json:"profiling_results"`
}

func Run(ctx context.Context, root, composerRoot string, selected scope.Selection, cfg config.Semgrep, process runner.Process) checker.Outcome {
	executable, _ := os.Executable()
	binary := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "semgrep", "vendor", "bin", "semgrep-core"))
	return run(ctx, root, composerRoot, selected, cfg, binary, process)
}

func run(ctx context.Context, root, composerRoot string, selected scope.Selection, cfg config.Semgrep, binary string, process runner.Process) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "semgrep", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	selected = Select(selected, cfg)
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
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: new("semgrep"), Message: message})
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	prepared, err := prepare(root, composerRoot, cfg)
	if err != nil {
		return fail("error", "config_invalid", err.Error())
	}
	if len(prepared.Rules) == 0 {
		reason := "no_rules"
		result.Check.Status, result.Check.Reason = "skipped", &reason
		return result
	}
	if _, err := os.Stat(binary); err != nil {
		return fail("unavailable", "dependencies_missing", "Pinned Semgrep CE engine is unavailable; prepare the tool bundle or rebuild the image")
	}
	temporary, err := os.MkdirTemp("", "phpwarden-semgrep-")
	if err != nil {
		return fail("error", "helper_prepare", "Cannot prepare a private directory for Semgrep")
	}
	defer os.RemoveAll(temporary)
	environment := []string{"HOME=" + temporary, "TMPDIR=" + temporary, "XDG_CACHE_HOME=" + temporary, "LANG=C.UTF-8"}
	for _, name := range []string{"PATH", "SYSTEMROOT", "WINDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	invoke := func(args []string) runner.Result {
		return process.Run(ctx, runner.Command{Name: binary, Args: args, Directory: temporary, Env: environment})
	}
	version := invoke([]string{"-version"})
	if version.Unavailable {
		return fail("unavailable", "executable_missing", "Semgrep CE engine cannot be executed in this worker")
	}
	if version.TimedOut || ctx.Err() != nil {
		return fail("error", "checker_timeout", "Semgrep did not complete within its execution context")
	}
	if version.Err != nil || version.ExitCode != 0 || len(version.Stderr) != 0 || !versionOutput.Match(version.Stdout) {
		return fail("unavailable", "version_mismatch", "Semgrep CE engine does not match the pinned version or platform")
	}
	values := []map[string]any{}
	byID := map[string]rule{}
	for _, rule := range prepared.Rules {
		values = append(values, rule.Value)
		byID[rule.ID] = rule
	}
	rules, err := yaml.Marshal(map[string]any{"rules": values})
	if err != nil {
		return fail("error", "config_invalid", "Cannot encode local Semgrep rules")
	}
	rulesFile := filepath.Join(temporary, "rules.yaml")
	if err := os.WriteFile(rulesFile, rules, 0o600); err != nil {
		return fail("error", "helper_prepare", "Cannot prepare local Semgrep rules")
	}
	project, err := os.OpenRoot(root)
	if err != nil {
		return fail("error", "input_read", "Cannot open Semgrep project inputs")
	}
	defer project.Close()
	input := filepath.Join(temporary, "target.php")
	for _, name := range selected.Files {
		data, _, err := edit.Read(project, name)
		if err != nil || len(data) > maxSourceBytes {
			return fail("error", "input_read", "Semgrep inputs must be readable regular files without symlinks, at most 16 MiB each")
		}
		if err := os.WriteFile(input, data, 0o600); err != nil {
			return fail("error", "helper_prepare", "Cannot copy a Semgrep source input")
		}
		args := []string{"-rules", rulesFile, "-lang", "php", "-j", "1", "-json_time", "-json_nodots", "-strict", "-no_filter_irrelevant_rules", "-timeout", strconv.Itoa(cfg.RuleTimeoutSeconds), "-timeout_threshold", "1", "-max_memory", strconv.Itoa(cfg.MemoryLimitMB), "-max_match_per_file", "10000", input}
		execution := invoke(args)
		if execution.TimedOut || ctx.Err() != nil {
			return fail("error", "checker_timeout", "Semgrep did not complete within its execution context")
		}
		if execution.Unavailable {
			return fail("unavailable", "executable_missing", "Semgrep CE engine became unavailable during the scan")
		}
		if execution.Err != nil || execution.ExitCode != 0 || len(execution.Stderr) != 0 {
			return fail("error", "process_failed", "Semgrep CE could not complete the scan; native output is suppressed")
		}
		parsed, err := decode(execution.Stdout, input, data, prepared.Rules)
		if err != nil {
			return fail("error", "invalid_report", err.Error())
		}
		occurrences := map[string]int{}
		for _, row := range parsed.Results {
			rule := byID[row.Rule]
			contentHash := edit.Hash(data[row.Start.Offset:row.End.Offset])
			identity := row.Rule + "\x00" + name + "\x00" + contentHash
			ordinal := occurrences[identity]
			occurrences[identity]++
			fingerprint := edit.Hash([]byte(identity + "\x00" + strconv.Itoa(ordinal)))
			endLine := row.End.Line
			if row.End.Col == 1 && endLine > row.Start.Line {
				endLine--
			}
			line := overlappingLine(selected, name, row.Start.Line, endLine)
			if line == 0 || prepared.Baseline[fingerprint] {
				continue
			}
			severity := map[string]string{"ERROR": "error", "WARNING": "warning", "INFO": "info"}[rule.Severity]
			column := row.Start.Col
			if line != row.Start.Line {
				column = 1
			}
			result.Findings = append(result.Findings, report.Finding{Checker: "semgrep", Rule: row.Rule, Kind: "code", Severity: severity, Message: rule.Message, Path: new(name), Line: new(line), Column: new(column), Fingerprint: new(fingerprint)})
			if cfg.FailOn != "never" && (severity == "error" || cfg.FailOn == "warning" && severity == "warning") {
				result.Check.Status = "failed"
			}
		}
	}
	return result
}

func decode(data []byte, input string, source []byte, rules []rule) (coreReport, error) {
	var result coreReport
	failure := func() (coreReport, error) {
		return coreReport{}, fmt.Errorf("Semgrep returned an invalid, incomplete, non-CE or out-of-scope report")
	}
	if len(data) > 32<<20 {
		return failure()
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return failure()
	}
	if _, err := decoder.Token(); err != io.EOF || result.Version != Version || result.Results == nil || result.Errors == nil || len(result.Errors) != 0 || len(result.SkippedRules) != 0 || len(result.Paths.Skipped) != 0 || !slices.Equal(result.Paths.Scanned, []string{input}) || result.Engine != "OSS" || len(result.Interfile) != 0 {
		return failure()
	}
	var timing struct {
		FixpointTimeouts []json.RawMessage `json:"fixpoint_timeouts"`
	}
	if len(result.Time) == 0 || bytes.Equal(result.Time, []byte("null")) || json.Unmarshal(result.Time, &timing) != nil || len(timing.FixpointTimeouts) != 0 {
		return failure()
	}
	known := map[string]bool{}
	for _, rule := range rules {
		known[rule.ID] = true
	}
	seen := map[string]bool{}
	for _, pair := range result.RulesByEngine {
		if len(pair) != 2 || !known[pair[0]] || seen[pair[0]] || pair[1] != "OSS" {
			return failure()
		}
		seen[pair[0]] = true
	}
	if len(seen) != len(known) {
		return failure()
	}
	positionValid := func(p position) bool {
		if p.Offset < 0 || p.Offset > len(source) || p.Line < 1 || p.Col < 1 {
			return false
		}
		prefix := source[:p.Offset]
		return p.Line == bytes.Count(prefix, []byte{'\n'})+1 && p.Col == p.Offset-bytes.LastIndexByte(prefix, '\n')
	}
	for _, row := range result.Results {
		if !known[row.Rule] || row.Path != input || row.Extra.Engine != "OSS" || row.Extra.Ignored == nil || *row.Extra.Ignored || !positionValid(row.Start) || !positionValid(row.End) || row.End.Offset <= row.Start.Offset {
			return failure()
		}
	}
	slices.SortStableFunc(result.Results, func(a, b match) int {
		if a.Start.Offset != b.Start.Offset {
			return a.Start.Offset - b.Start.Offset
		}
		if a.End.Offset != b.End.Offset {
			return a.End.Offset - b.End.Offset
		}
		return strings.Compare(a.Rule, b.Rule)
	})
	return result, nil
}

func overlappingLine(selected scope.Selection, name string, start, end int) int {
	ranges, ok := selected.LineMap[name]
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
