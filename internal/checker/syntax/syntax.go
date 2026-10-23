package syntax

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
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

func Run(ctx context.Context, root string, selection scope.Selection, cfg config.Syntax, process runner.Process) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "syntax", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn},
		Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	if !cfg.Enabled || len(selection.Files) == 0 {
		reason := "no_files"
		if !cfg.Enabled {
			reason = "disabled"
		}
		result.Check.Status, result.Check.Reason = "skipped", &reason
		return result
	}
	if cfg.Backend == "parallel-lint" {
		return runParallel(ctx, root, selection, cfg, parallelBundle(), process)
	}
	fail := func(status, code, message string) checker.Outcome {
		result.Check.Status, result.Check.Reason = status, &code
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Message: message, Checker: new("syntax")})
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	probe := process.Run(ctx, runner.Command{Name: "php", Args: []string{"-n", "-r", "echo PHP_VERSION;"}, Directory: root})
	if probe.Unavailable {
		return fail("unavailable", "executable_missing", "PHP executable is unavailable in the worker")
	}
	if probe.TimedOut || ctx.Err() != nil {
		return fail("error", "checker_timeout", "PHP syntax checker did not complete within its execution context")
	}
	if probe.Err != nil || probe.ExitCode != 0 {
		return fail("error", "process_failed", "Cannot determine the PHP syntax checker runtime")
	}
	version := string(probe.Stdout)
	canonical, err := phpconstraint.StableVersion(version)
	if err != nil || canonical != version || len(probe.Stderr) != 0 {
		return fail("error", "invalid_report", "Expected a canonical stable PHP runtime version")
	}
	minor := version[:strings.LastIndex(version, ".")]
	result.Runtime, result.PHPVersion = &minor, &version
	for _, filename := range selection.Files {
		absolute := filepath.Join(root, filepath.FromSlash(filename))
		execution := process.Run(ctx, runner.Command{Name: "php", Directory: root, Args: []string{
			"-n", "-d", "display_errors=stderr", "-d", "log_errors=0", "-d", "html_errors=0", "-d", "error_reporting=69", "-l", "-f", absolute,
		}})
		if execution.Unavailable {
			return fail("unavailable", "executable_missing", "PHP executable is unavailable in the worker")
		}
		if execution.TimedOut || ctx.Err() != nil {
			return fail("error", "checker_timeout", "PHP syntax checker did not complete within its execution context")
		}
		if execution.ExitCode == 0 && execution.Err == nil {
			if string(execution.Stdout) != "No syntax errors detected in "+absolute+"\n" || len(execution.Stderr) != 0 {
				return fail("error", "invalid_report", fmt.Sprintf("Unexpected PHP lint success response for %q", filename))
			}
			continue
		}
		if execution.ExitCode != 255 || string(execution.Stdout) != "Errors parsing "+absolute+"\n" {
			return fail("error", "process_failed", fmt.Sprintf("PHP lint did not complete for %q (exit %d)", filename, execution.ExitCode))
		}
		pattern := regexp.MustCompile(`(?s)^(?:PHP )?(?:Parse error|Fatal error):\s*(.+) in ` + regexp.QuoteMeta(absolute) + ` on line ([1-9][0-9]*)\n?$`)
		match := pattern.FindStringSubmatch(string(execution.Stderr))
		if match == nil {
			return fail("error", "invalid_report", fmt.Sprintf("Cannot interpret PHP lint failure for %q", filename))
		}
		for _, prefix := range []string{"Allowed memory size", "Maximum execution time", "Out of memory"} {
			if strings.HasPrefix(match[1], prefix) {
				return fail("error", "process_failed", fmt.Sprintf("PHP lint exhausted resources while checking %q", filename))
			}
		}
		line, err := strconv.Atoi(match[2])
		if err != nil {
			return fail("error", "invalid_report", fmt.Sprintf("Invalid PHP lint error position for %q", filename))
		}
		message := strings.ReplaceAll(match[1], absolute, filename)
		result.Findings = append(result.Findings, report.Finding{Checker: "syntax", Rule: "php.syntax", Kind: "syntax", Severity: "error",
			Message: message, Path: new(filename), Line: &line})
	}
	if len(result.Findings) > 0 && cfg.FailOn != "never" {
		result.Check.Status = "failed"
	}
	return result
}
