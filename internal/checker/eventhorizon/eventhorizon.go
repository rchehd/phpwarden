package eventhorizon

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
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

const Version = "0.1.0"
const Revision = "ae11df1cd8e4e2b843c6c53bac60026ef344935f"
const YAMLVersion = "6.0.3"

//go:embed helper.py
var helper []byte

var Rules = []string{
	"caching.bigpipe_opportunity", "caching.block_cacheability_leak", "caching.cache_bin_opportunity",
	"caching.dependency_chain_break", "caching.early_rendering", "caching.metadata_bubbling_lost",
	"caching.missed_drupal_static", "caching.missing_cache_metadata", "caching.missing_cache_tags",
	"caching.missing_entity_tag", "caching.missing_url_context", "caching.missing_user_context",
	"caching.render_rebuilt_every_request", "caching.stale_cache_tag", "caching.uncached_database_query", "caching.user_context_overuse",
	"config.broken_reference", "config.circular_dependency", "config.consolidation_opportunity", "config.missing_description",
	"config.orphaned_entity", "config.overly_complex", "config.unused_field",
}

type finding struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Rule     string `json:"rule"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type response struct {
	Version     int       `json:"version"`
	ToolVersion string    `json:"tool_version"`
	Revision    string    `json:"revision"`
	YAML        string    `json:"pyyaml"`
	Files       []string  `json:"files"`
	ConfigDirs  []string  `json:"config_dirs"`
	Findings    []finding `json:"findings"`
}

func Run(ctx context.Context, root string, selected scope.Selection, cfg config.EventHorizon, process runner.Process) checker.Outcome {
	executable, _ := os.Executable()
	bundle := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "eventhorizon", "vendor"))
	return run(ctx, root, selected, cfg, bundle, process)
}

func run(ctx context.Context, root string, selected scope.Selection, cfg config.EventHorizon, bundle string, process runner.Process) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "eventhorizon", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
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
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: new("eventhorizon"), Message: message})
		return result
	}
	for _, id := range cfg.ExcludeRules {
		if !slices.Contains(Rules, id) {
			return fail("error", "config_invalid", fmt.Sprintf("Unknown EventHorizon rule %q", id))
		}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	prepared, err := prepare(root, selected, cfg)
	if err != nil {
		return fail("error", "input_read", err.Error())
	}
	if _, err := os.Stat(filepath.Join(bundle, "source.json")); err != nil {
		return fail("unavailable", "dependencies_missing", "Bundled EventHorizon sources are unavailable; prepare the pinned bundle or rebuild the toolset")
	}
	temporary, err := os.MkdirTemp("", "phpwarden-eventhorizon-")
	if err != nil {
		return fail("error", "helper_prepare", "Cannot prepare EventHorizon private inputs")
	}
	defer os.RemoveAll(temporary)
	inputRoot := filepath.Join(temporary, "project")
	for name, data := range prepared.contents {
		target := filepath.Join(inputRoot, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fail("error", "helper_prepare", "Cannot prepare EventHorizon private inputs")
		}
		if err := os.WriteFile(target, data, 0o600); err != nil {
			return fail("error", "helper_prepare", "Cannot copy EventHorizon inputs")
		}
	}
	script := filepath.Join(temporary, "helper.py")
	if err := os.WriteFile(script, helper, 0o600); err != nil {
		return fail("error", "helper_prepare", "Cannot prepare the EventHorizon helper")
	}
	input, _ := json.Marshal(map[string]any{"tool_version": Version, "revision": Revision, "pyyaml": YAMLVersion, "files": prepared.Files, "config_dirs": prepared.ConfigDirs})
	environment := []string{"HOME=" + temporary, "TMPDIR=" + temporary, "LANG=C.UTF-8"}
	for _, name := range []string{"PATH", "SYSTEMROOT", "WINDIR"} {
		if value, ok := os.LookupEnv(name); ok {
			environment = append(environment, name+"="+value)
		}
	}
	execution := process.Run(ctx, runner.Command{Name: "python3", Args: []string{"-I", "-S", "-B", script, bundle, inputRoot}, Input: input, Directory: temporary, Env: environment})
	if execution.Unavailable {
		return fail("unavailable", "executable_missing", "Python 3 is unavailable in the worker")
	}
	if execution.TimedOut || ctx.Err() != nil {
		return fail("error", "checker_timeout", "EventHorizon did not complete within its execution context")
	}
	if execution.Err != nil || execution.ExitCode != 0 || len(execution.Stderr) > 0 {
		return fail("error", "process_failed", "EventHorizon could not complete its analysis; no partial findings accepted")
	}
	parsed, err := decode(execution.Stdout, prepared)
	if err != nil {
		return fail("error", "invalid_report", err.Error())
	}
	for _, row := range parsed.Findings {
		if slices.Contains(cfg.ExcludeRules, row.Rule) {
			continue
		}
		severity := row.Severity
		if severity == "error" {
			severity = "warning"
		}
		item := report.Finding{Checker: "eventhorizon", Rule: row.Rule, Kind: "code", Severity: severity, Message: row.Message}
		if row.Line == 0 {
			item.Kind, item.Message = "global", fmt.Sprintf("Config sync %q: %s", row.File, row.Message)
		} else {
			if !selected.Contains(row.File, row.Line) {
				continue
			}
			item.Path, item.Line = new(row.File), new(row.Line)
		}
		result.Findings = append(result.Findings, item)
		if cfg.FailOn == "warning" && severity == "warning" {
			result.Check.Status = "failed"
		}
	}
	return result
}

func decode(data []byte, prepared plan) (response, error) {
	var result response
	failure := func() (response, error) {
		return response{}, fmt.Errorf("EventHorizon returned an invalid, incomplete or out-of-scope report")
	}
	if len(data) > 32<<20 {
		return failure()
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return failure()
	}
	if _, err := decoder.Token(); err != io.EOF || result.Version != 1 || result.ToolVersion != Version || result.Revision != Revision || result.YAML != YAMLVersion || result.Findings == nil ||
		!reflect.DeepEqual(result.Files, prepared.Files) || !reflect.DeepEqual(result.ConfigDirs, prepared.ConfigDirs) {
		return failure()
	}
	for _, row := range result.Findings {
		if !slices.Contains(Rules, row.Rule) || !slices.Contains([]string{"error", "warning", "info"}, row.Severity) || strings.TrimSpace(row.Message) == "" || row.Line < 0 {
			return failure()
		}
		if strings.HasPrefix(row.Rule, "config.") {
			if row.Line != 0 || !slices.Contains(prepared.ConfigDirs, row.File) {
				return failure()
			}
		} else if !slices.Contains(prepared.Files, row.File) || row.Line < 1 || row.Line > bytes.Count(prepared.contents[row.File], []byte{'\n'})+1 || path.Clean(row.File) != row.File {
			return failure()
		}
	}
	return result, nil
}
