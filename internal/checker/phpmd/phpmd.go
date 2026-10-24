package phpmd

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

const Version = "3.0.0"

type violation struct {
	BeginLine       int     `json:"beginLine"`
	EndLine         int     `json:"endLine"`
	Package         *string `json:"package"`
	Function        *string `json:"function"`
	Class           *string `json:"class"`
	Method          *string `json:"method"`
	Description     string  `json:"description"`
	Rule            string  `json:"rule"`
	RuleSet         string  `json:"ruleSet"`
	ExternalInfoURL string  `json:"externalInfoUrl"`
	Priority        int     `json:"priority"`
}

type phpmdReport struct {
	Version   string `json:"version"`
	Package   string `json:"package"`
	Timestamp string `json:"timestamp"`
	Files     []struct {
		File         string      `json:"file"`
		RelativePath string      `json:"relativePath"`
		Violations   []violation `json:"violations"`
	} `json:"files"`
	Errors []struct {
		FileName string `json:"fileName"`
		Message  string `json:"message"`
	} `json:"errors"`
}

func Run(ctx context.Context, root, composerRoot string, selection scope.Selection, cfg config.PHPMD, process runner.Process) checker.Outcome {
	executable, _ := os.Executable()
	bundle := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "phpmd"))
	return run(ctx, root, composerRoot, selection, cfg, bundle, process)
}

func run(ctx context.Context, root, composerRoot string, selection scope.Selection, cfg config.PHPMD, bundle string, process runner.Process) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "phpmd", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	if !cfg.Enabled || len(selection.Files) == 0 {
		reason := "no_files"
		if !cfg.Enabled {
			reason = "disabled"
		}
		result.Check.Status, result.Check.Reason = "skipped", &reason
		return result
	}
	fail := func(status, code, text string) checker.Outcome {
		result.Check.Status, result.Check.Reason, result.Findings = status, &code, []report.Finding{}
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: new("phpmd"), Message: text})
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	if ctx.Err() != nil {
		return fail("error", "checker_timeout", "PHPMD did not complete within its execution context")
	}
	prepared, err := prepare(root, composerRoot, selection, cfg)
	if err != nil {
		return fail("error", "config_invalid", err.Error())
	}
	binary := filepath.Join(bundle, "vendor/phpmd/phpmd/bin/phpmd")
	if _, err := os.Stat(binary); err != nil {
		return fail("unavailable", "dependencies_missing", "Bundled PHPMD dependencies are unavailable beside the worker binary; rebuild or reinstall the toolset")
	}
	temporary, err := os.MkdirTemp("", "phpwarden-phpmd-")
	if err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	defer os.RemoveAll(temporary)
	environment := slices.DeleteFunc(os.Environ(), func(variable string) bool {
		return strings.HasPrefix(variable, "GITHUB_ACTIONS=")
	})
	fallback := filepath.Join(temporary, "phpmd.xml")
	if err := os.WriteFile(fallback, profileRuleset(cfg), 0o600); err != nil {
		return fail("error", "helper_prepare", err.Error())
	}
	for _, batch := range prepared.Groups {
		ruleset := fallback
		if batch.Config != "" {
			ruleset = batch.Config
		}
		args := []string{"-d", "auto_prepend_file=", "-d", "auto_append_file=", "-d", "display_errors=stderr", "-d", "log_errors=0",
			"-d", "memory_limit=" + strconv.Itoa(cfg.MemoryLimitMB) + "M", binary, "analyze", "--format=json", "--ruleset=" + ruleset,
			"--threads=1", "--no-progress", "--no-cache", "--no-ansi", "--no-interaction"}
		extensions := []string{}
		for _, name := range batch.Files {
			extension := strings.TrimPrefix(filepath.Ext(name), ".")
			if !slices.Contains(extensions, extension) {
				extensions = append(extensions, extension)
				args = append(args, "--suffixes="+extension)
			}
		}
		args = append(append(args, "--"), batch.Files...)
		execution := process.Run(ctx, runner.Command{Name: "php", Directory: root, Args: args, Env: environment})
		if execution.Unavailable {
			return fail("unavailable", "executable_missing", "PHP executable is unavailable in the worker")
		}
		if execution.TimedOut || ctx.Err() != nil {
			return fail("error", "checker_timeout", "PHPMD did not complete within its execution context")
		}
		if execution.ExitCode != 0 && execution.ExitCode != 2 && execution.ExitCode != 3 || len(execution.Stderr) > 0 {
			return fail("error", "process_failed", fmt.Sprintf("PHPMD exited %d: %s", execution.ExitCode, strings.TrimSpace(string(execution.Stderr))))
		}
		parsed, err := decode(execution.Stdout, execution.ExitCode, root, batch.Files)
		if err != nil {
			return fail("error", "invalid_report", err.Error())
		}
		for _, file := range parsed.Files {
			name := relative(root, file.File)
			for _, row := range file.Violations {
				if selection.Contains(name, row.BeginLine) {
					result.Findings = append(result.Findings, report.Finding{Checker: "phpmd", Rule: row.Rule, Kind: "code", Severity: "warning",
						Message: row.Description, Path: new(name), Line: new(row.BeginLine)})
				}
			}
		}
		for _, row := range parsed.Errors {
			name, message, ok := parseError(root, batch.Files, row.Message)
			if !ok {
				return fail("error", "analysis_failed", "PHPMD could not analyze the selected files: "+strings.SplitN(row.Message, "\n", 2)[0])
			}
			result.Findings = append(result.Findings, report.Finding{Checker: "phpmd", Rule: "phpmd.parse_error", Kind: "global", Severity: "warning",
				Message: fmt.Sprintf("PHP file %q: PHPMD could not parse it: %s", name, message)})
		}
	}
	if cfg.FailOn == "warning" && len(result.Findings) > 0 {
		result.Check.Status = "failed"
	}
	return result
}

// profileRuleset renders the configured built-in rulesets without the excluded rules.
func profileRuleset(cfg config.PHPMD) []byte {
	var buffer bytes.Buffer
	buffer.WriteString(`<?xml version="1.0"?>` + "\n" + `<ruleset name="PHPWarden">` + "\n")
	for _, name := range cfg.Rulesets {
		buffer.WriteString(`  <rule ref="rulesets/` + name + `.xml">` + "\n")
		for _, rule := range cfg.ExcludeRules {
			buffer.WriteString(`    <exclude name="`)
			xml.EscapeText(&buffer, []byte(rule))
			buffer.WriteString(`"/>` + "\n")
		}
		buffer.WriteString("  </rule>\n")
	}
	buffer.WriteString("</ruleset>\n")
	return buffer.Bytes()
}

func relative(root, name string) string {
	if filepath.IsAbs(name) {
		if rel, err := filepath.Rel(root, name); err == nil {
			name = rel
		}
	}
	return filepath.ToSlash(name)
}

// parseError attributes a PDepend parse error such as "Unexpected token: {, line: 2, col: 18, file: /abs/x.php." to a selected file.
func parseError(root string, files []string, text string) (string, string, bool) {
	first := strings.SplitN(text, "\n", 2)[0]
	for _, name := range files {
		absolute := filepath.Join(root, filepath.FromSlash(name))
		marker := ", file: " + absolute + "."
		if index := strings.Index(first, marker); index >= 0 && strings.HasSuffix(first, marker) {
			return name, first[:index], true
		}
	}
	return "", "", false
}

func decode(data []byte, exitCode int, root string, files []string) (phpmdReport, error) {
	var parsed phpmdReport
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parsed); err != nil {
		return parsed, err
	}
	if _, err := decoder.Token(); err != io.EOF || parsed.Version != Version || parsed.Package != "phpmd" || parsed.Files == nil {
		return parsed, fmt.Errorf("expected one versioned PHPMD report from the pinned toolset")
	}
	seen := map[string]bool{}
	violations := 0
	for _, file := range parsed.Files {
		name := relative(root, file.File)
		if !slices.Contains(files, name) || seen[name] || len(file.Violations) == 0 {
			return parsed, fmt.Errorf("PHPMD reported an unexpected file")
		}
		seen[name] = true
		for _, row := range file.Violations {
			if row.BeginLine < 1 || row.EndLine < row.BeginLine || row.Rule == "" || row.Description == "" {
				return parsed, fmt.Errorf("invalid PHPMD finding")
			}
			violations++
		}
	}
	expected := 0
	if len(parsed.Errors) > 0 {
		expected = 3
	} else if violations > 0 {
		expected = 2
	}
	if exitCode != expected {
		return parsed, fmt.Errorf("PHPMD exit code %d does not match its report", exitCode)
	}
	return parsed, nil
}
