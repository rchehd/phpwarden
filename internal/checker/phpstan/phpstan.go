package phpstan

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
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

const Version = "2.3.1"
const DrupalVersion = "2.2.2"
const DeprecationVersion = "2.0.5"
const DeadCodeVersion = "1.4.2"
const StrictRulesVersion = "2.1.0"

//go:embed helper.php
var helper []byte

// PHPStan writes agent instructions to stderr when it sees these variables.
var agentVariables = []string{"AUGMENT_AGENT", "AMP_CURRENT_THREAD_ID", "AI_AGENT", "CURSOR_TRACE_ID", "CURSOR_AGENT", "GEMINI_CLI", "CODEX_SANDBOX", "CODEX_THREAD_ID", "OPENCODE_CLIENT", "OPENCODE", "CLAUDECODE", "CLAUDE_CODE", "REPL_ID"}

type metadata struct {
	Version    int               `json:"version"`
	Runtime    string            `json:"runtime"`
	PHPVersion string            `json:"php_version"`
	Tools      map[string]string `json:"tools"`
}

type message struct {
	Message    string `json:"message"`
	Line       *int   `json:"line"`
	Ignorable  bool   `json:"ignorable"`
	Identifier string `json:"identifier"`
	Tip        string `json:"tip"`
}

type fileReport struct {
	Errors   int       `json:"errors"`
	Messages []message `json:"messages"`
}

type nativeReport struct {
	Totals struct {
		Errors     int `json:"errors"`
		FileErrors int `json:"file_errors"`
	} `json:"totals"`
	Files  json.RawMessage `json:"files"`
	Errors []string        `json:"errors"`
}

// analysis is one PHPStan run: the regular checker on selected files, or dead code over whole source roots.
type analysis struct {
	id         string
	paths      []string
	extensions []string
	includes   []string
	identifier string
	severity   string
	level      *int
	exclude    []string
	// PHP files load with the autoloader, before PHPStan builds its container; NEON files are included.
	files map[string][]byte
}

func bundlePath() string {
	executable, _ := os.Executable()
	return filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "phpstan"))
}

// A nonempty dependencies error means installed packages do not match composer.lock.
func Run(ctx context.Context, root, composerRoot string, selection scope.Selection, cfg config.PHPStan, target config.Target, dependencies string, process runner.Process) checker.Outcome {
	return run(ctx, root, composerRoot, selection, cfg, target, dependencies, bundlePath(), process)
}

func run(ctx context.Context, root, composerRoot string, selection scope.Selection, cfg config.PHPStan, target config.Target, dependencies, bundle string, process runner.Process) checker.Outcome {
	return analyse(ctx, root, composerRoot, selection, cfg, target, dependencies, bundle, process, analysis{id: "phpstan", paths: selection.Files, severity: "error"})
}

func analyse(ctx context.Context, root, composerRoot string, selection scope.Selection, cfg config.PHPStan, target config.Target, dependencies, bundle string, process runner.Process, mode analysis) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: mode.id, Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
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
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: new(mode.id), Message: text})
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return fail("error", "checker_timeout", "PHPStan execution context expired")
	}
	if _, err := contextDigest(root); err != nil {
		return fail("error", "context_invalid", err.Error())
	}
	selected, autoload, err := prepare(root, composerRoot, cfg)
	if err != nil {
		return fail("error", "config_invalid", err.Error())
	}
	if cfg.Drupal && autoload == "" {
		return fail("unavailable", "project_dependencies_missing", "Drupal PHPStan analysis requires the project's installed Composer dependencies; run composer install in the project environment")
	}
	if dependencies != "" && autoload != "" {
		return fail("unavailable", "project_dependencies_mismatch", "PHPStan analysis needs installed Composer dependencies that match composer.lock ("+dependencies+"); run composer install in the project environment")
	}
	if _, err := os.Stat(filepath.Join(bundle, "vendor/autoload.php")); err != nil {
		return fail("unavailable", "dependencies_missing", "Bundled PHPStan dependencies are unavailable beside the worker binary; rebuild or reinstall the toolset")
	}
	temporary, err := os.MkdirTemp("", "phpwarden-phpstan-")
	if err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	defer os.RemoveAll(temporary)
	script := filepath.Join(temporary, "prepare.php")
	if err := os.WriteFile(script, helper, 0o600); err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	includes := []string{}
	if cfg.Drupal {
		includes = append(includes, filepath.Join(bundle, "vendor/mglaman/phpstan-drupal/extension.neon"), filepath.Join(bundle, "vendor/mglaman/phpstan-drupal/rules.neon"))
	}
	if cfg.Deprecations {
		includes = append(includes, filepath.Join(bundle, "vendor/phpstan/phpstan-deprecation-rules/rules.neon"))
	}
	if mode.id == "phpstan" && cfg.StrictRules.Enabled {
		includes = append(includes, filepath.Join(bundle, "vendor/phpstan/phpstan-strict-rules/rules.neon"))
	}
	for _, name := range mode.includes {
		includes = append(includes, filepath.Join(bundle, name))
	}
	for _, name := range append(append([]string{selected}, cfg.Includes...), cfg.Baseline) {
		if name != "" {
			includes = append(includes, filepath.Join(root, filepath.FromSlash(name)))
		}
	}
	absolute := func(names []string) []string {
		values := []string{}
		for _, name := range names {
			values = append(values, filepath.Join(root, filepath.FromSlash(name)))
		}
		return values
	}
	preload := []string{}
	if autoload != "" {
		preload = append(preload, filepath.Join(root, filepath.FromSlash(autoload)))
	}
	for _, name := range slices.Sorted(maps.Keys(mode.files)) {
		filename := filepath.Join(temporary, name)
		if err := os.WriteFile(filename, mode.files[name], 0o600); err != nil {
			return fail("error", "helper_prepare", err.Error())
		}
		if strings.HasSuffix(name, ".neon") {
			includes = append(includes, filename)
		} else {
			preload = append(preload, filename)
		}
	}
	if len(preload) > 1 || len(preload) == 1 && autoload == "" {
		quote := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
		wrapper := "<?php\n"
		for _, name := range preload {
			wrapper += "require '" + quote.Replace(name) + "';\n"
		}
		filename := filepath.Join(temporary, "autoload.php")
		if err := os.WriteFile(filename, []byte(wrapper), 0o600); err != nil {
			return fail("error", "helper_prepare", err.Error())
		}
		preload = []string{filename}
	}
	extensions := append([]string{"php"}, mode.extensions...)
	for _, name := range selection.Files {
		ext := strings.TrimPrefix(filepath.Ext(name), ".")
		if !slices.Contains(extensions, ext) {
			extensions = append(extensions, ext)
		}
	}
	versionID := func(version string, maximum bool) int {
		parts := strings.Split(version, ".")
		major, _ := strconv.Atoi(parts[0])
		minor, _ := strconv.Atoi(parts[1])
		patch := 0
		if maximum {
			patch = 99
		}
		return major*10000 + minor*100 + patch
	}
	input, _ := json.Marshal(map[string]any{"includes": includes, "extensions": extensions, "php_version": map[string]int{"min": versionID(target.Min, false), "max": versionID(target.Max, true)},
		"diff": selection.Mode == "diff" && !selection.WholeFiles || mode.id != "phpstan", "bootstrapFiles": absolute(cfg.BootstrapFiles), "scanDirectories": absolute(cfg.ScanDirectories), "stubFiles": absolute(cfg.StubFiles), "exclude": append([]string{}, mode.exclude...),
		"strict_rules": cfg.StrictRules, "strict_enabled": mode.id == "phpstan" && cfg.StrictRules.Enabled})
	phpArgs := []string{"-d", "auto_prepend_file=", "-d", "auto_append_file=", "-d", "display_errors=stderr", "-d", "log_errors=0", "-d", "memory_limit=" + strconv.Itoa(cfg.MemoryLimitMB) + "M"}
	execution := process.Run(ctx, runner.Command{Name: "php", Directory: root, Input: input, Args: append(slices.Clone(phpArgs), script, bundle, temporary)})
	processFailure := func(execution runner.Result) *checker.Outcome {
		var outcome checker.Outcome
		if execution.Unavailable {
			outcome = fail("unavailable", "executable_missing", "PHP executable is unavailable in the worker")
		} else if execution.TimedOut || ctx.Err() != nil {
			outcome = fail("error", "checker_timeout", "PHPStan did not complete within its execution context")
		} else if execution.Err != nil {
			outcome = fail("error", "process_failed", fmt.Sprintf("PHPStan process failed: %v", execution.Err))
		} else {
			return nil
		}
		return &outcome
	}
	if failure := processFailure(execution); failure != nil {
		return *failure
	}
	if execution.ExitCode != 0 {
		return fail("error", "helper_prepare", fmt.Sprintf("PHPStan preparation exited %d: %s", execution.ExitCode, execution.Stderr))
	}
	var meta metadata
	if err := strictJSON(execution.Stdout, &meta); err != nil || len(execution.Stderr) > 0 || meta.Version != 1 || meta.Tools["phpstan/phpstan"] != Version || meta.Tools["mglaman/phpstan-drupal"] != DrupalVersion || meta.Tools["phpstan/phpstan-deprecation-rules"] != DeprecationVersion || meta.Tools["shipmonk/dead-code-detector"] != DeadCodeVersion || meta.Tools["phpstan/phpstan-strict-rules"] != StrictRulesVersion {
		return fail("error", "invalid_report", "Expected PHPStan metadata from the pinned toolset")
	}
	canonical, err := phpconstraint.StableVersion(meta.PHPVersion)
	if err != nil || canonical != meta.PHPVersion || canonical[:strings.LastIndex(canonical, ".")] != meta.Runtime {
		return fail("error", "invalid_report", "Invalid PHPStan PHP runtime version")
	}
	result.Runtime, result.PHPVersion = &meta.Runtime, &meta.PHPVersion
	args := append(slices.Clone(phpArgs), filepath.Join(bundle, "vendor/phpstan/phpstan/phpstan"), "analyse", "--configuration="+filepath.Join(temporary, "phpstan.neon"), "--error-format=json", "--no-progress", "--no-ansi", "--memory-limit="+strconv.Itoa(cfg.MemoryLimitMB)+"M")
	if len(preload) > 0 {
		args = append(args, "--autoload-file="+preload[0])
	}
	if mode.level != nil {
		args = append(args, "--level="+strconv.Itoa(*mode.level))
	} else if cfg.Level != nil {
		args = append(args, "--level="+strconv.Itoa(*cfg.Level))
	} else if selected == "" && len(cfg.Includes) == 0 {
		args = append(args, "--level=3")
	}
	args = append(args, "--")
	args = append(args, absolute(mode.paths)...)
	environment := slices.DeleteFunc(os.Environ(), func(variable string) bool {
		name, _, _ := strings.Cut(variable, "=")
		return slices.Contains(agentVariables, name)
	})
	// phpstan-drupal looks for the Drupal root from the working directory.
	execution = process.Run(ctx, runner.Command{Name: "php", Directory: filepath.Join(root, filepath.FromSlash(composerRoot)), Args: args, Env: environment})
	if execution.ExitCode == 1 && !execution.Unavailable && !execution.TimedOut {
		execution.Err = nil
	}
	if failure := processFailure(execution); failure != nil {
		return *failure
	}
	if execution.ExitCode != 0 && execution.ExitCode != 1 {
		return fail("error", "analysis_failed", fmt.Sprintf("PHPStan exited %d: %s %s", execution.ExitCode, strings.TrimSpace(string(execution.Stderr)), strings.TrimSpace(string(execution.Stdout))))
	}
	rows, globals, err := decode(execution.Stdout, execution.ExitCode, root, mode.paths)
	if err != nil {
		return fail("error", "invalid_report", strings.TrimSpace(err.Error()+": "+string(execution.Stderr)))
	}
	if len(execution.Stderr) > 0 {
		return fail("error", "analysis_failed", strings.TrimSpace(string(execution.Stderr)))
	}
	if len(globals) > 0 {
		return fail("error", "analysis_failed", strings.Join(globals, "; "))
	}
	for name, file := range rows {
		for _, row := range file.Messages {
			// Dead code analyses whole roots, so other PHPStan errors there must not stop it.
			if mode.identifier != "" && !strings.HasPrefix(row.Identifier, mode.identifier) {
				continue
			}
			if row.Line == nil || row.Identifier == "" || row.Identifier == "internalError" || strings.HasPrefix(row.Identifier, "phpstan.") || row.Identifier == "parseError" {
				return fail("error", "analysis_failed", fmt.Sprintf("PHPStan could not complete analysis of %q: %s", name, row.Message))
			}
			if !selection.Contains(name, *row.Line) || !strings.HasPrefix(row.Identifier, mode.identifier) {
				continue
			}
			result.Findings = append(result.Findings, report.Finding{Checker: mode.id, Rule: row.Identifier, Kind: "code", Severity: mode.severity, Message: row.Message, Path: new(name), Line: row.Line})
		}
	}
	if len(result.Findings) > 0 && (cfg.FailOn == "warning" || cfg.FailOn == "error" && mode.severity == "error") {
		result.Check.Status = "failed"
	}
	return result
}

func strictJSON(data []byte, value any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("expected one PHPStan JSON report")
	}
	return nil
}

var traitContext = regexp.MustCompile(`^(.+) \(in context of class [A-Za-z_\\][A-Za-z0-9_\\]*\)$`)

func decode(data []byte, exit int, root string, files []string) (map[string]fileReport, []string, error) {
	var parsed nativeReport
	if err := strictJSON(data, &parsed); err != nil {
		return nil, nil, err
	}
	rows := map[string]fileReport{}
	if string(parsed.Files) != "[]" {
		if err := strictJSON(parsed.Files, &rows); err != nil || rows == nil {
			return nil, nil, fmt.Errorf("invalid PHPStan file reports")
		}
	}
	if parsed.Errors == nil || parsed.Totals.Errors != len(parsed.Errors) {
		return nil, nil, fmt.Errorf("invalid PHPStan global totals")
	}
	result := map[string]fileReport{}
	count := 0
	seen := map[string]bool{}
	for name, file := range rows {
		contextLabel := ""
		if match := traitContext.FindStringSubmatch(name); match != nil {
			contextLabel = strings.TrimPrefix(name, match[1])
			name = match[1]
		}
		if filepath.IsAbs(name) {
			name, _ = filepath.Rel(root, name)
		}
		name = filepath.ToSlash(name)
		if !slices.ContainsFunc(files, func(selected string) bool { return name == selected || strings.HasPrefix(name, selected+"/") }) || file.Messages == nil || file.Errors != len(file.Messages) {
			return nil, nil, fmt.Errorf("PHPStan file or totals do not match selected inputs")
		}

		for _, row := range file.Messages {
			if row.Message == "" || row.Line != nil && *row.Line < 1 {
				return nil, nil, fmt.Errorf("invalid PHPStan finding")
			}
		}
		identity := name + contextLabel
		if seen[identity] {
			return nil, nil, fmt.Errorf("duplicate PHPStan file")
		}
		seen[identity] = true
		previous := result[name]
		previous.Errors += file.Errors
		previous.Messages = append(previous.Messages, file.Messages...)
		result[name] = previous
		count += file.Errors
	}
	if count != parsed.Totals.FileErrors || (exit == 0) != (count+len(parsed.Errors) == 0) {
		return nil, nil, fmt.Errorf("PHPStan totals or exit code do not match findings")
	}
	return result, parsed.Errors, nil
}
