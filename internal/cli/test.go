package cli

import (
	"context"
	"fmt"
	"github.com/rchehd/phpwarden/internal/scope"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	buildinfo "github.com/rchehd/phpwarden"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/launcher"
	"github.com/rchehd/phpwarden/internal/projecttest"
	"github.com/rchehd/phpwarden/internal/runner"
)

const testUsage = `Usage: phpwarden test [PATH...] [options]

  --project DIR           Project root (default: current Git project)
  --config FILE           PHPWarden configuration, relative to caller
  --phpunit-config FILE   Native PHPUnit XML, relative to project root
  --testsuite NAME        Run a configured suite instead of discovered unit paths
  --coverage              Collect line coverage, HTML/XML and method CRAP
  --filter PATTERN        PHPUnit test-name filter
  --output DIR            New artifact directory, relative to caller
  --format text|json      Report format (default: text)
  --help                  Show help

Without paths or --testsuite, discover custom unit-test directories from the profile.
PATHs and tests.* paths are project-relative. Static check scope/exclusions do not apply.
The project's PHPUnit and dev dependencies must already be installed.
Tests run in an isolated copy using the selected PHP worker; Docker is required.
Artifacts: report.json, junit.xml, per-directory JUnit and PHPUnit stdout/stderr.
Default artifacts live in the user cache under phpwarden/tests/run-*.
Coverage thresholds: tests.coverage.min_line_coverage / max_crap (optional).
Exit: 0 passed; 1 failed tests or coverage thresholds; 2 input/setup selection; 3 incomplete run; 130 interrupted.
`

const mutationUsage = `Usage: phpwarden mutation [SOURCE_PATH...] [options]

  --project DIR           Project root
  --config FILE           PHPWarden config, relative to caller
  --infection-config FILE Native Infection JSON/JSON5, relative to project root
  --phpunit-config FILE   Native PHPUnit XML, relative to project root
  --testsuite NAME        Select a native PHPUnit suite
  --filter PATTERN        PHPUnit test-name filter
  --mode full|diff        Source scope (default: full)
  --base REF              Diff base
  --commits N             Last N commits and local edits; implies diff
  --whole-files           Mutate whole selected diff files
  --output DIR            New artifact directory, relative to caller
  --format text|json      Output format
  --help                  Show help

Uses the project's installed PHPUnit and the pinned Infection in the PHP worker.
PHP 8.3/8.4 only. Initial tests and fresh coverage must pass before mutation testing.
Test selection uses tests.*; SOURCE_PATHs select source to mutate, relative to caller.
Runs in a private copy without network access or original project mounts.
Configure mutation.config, threads, mutant_timeout_seconds, timeout_seconds,
mutators, min_msi and min_covered_msi in .phpwarden.yaml. Thresholds are opt-in.
Artifacts: test/coverage reports, infection.json, infection.html and raw logs.
Exit: 0 assessed; 1 initial tests or MSI gate failed; 2 invalid input;
3 incomplete (including no assessed mutations, runtime errors or timeouts); 130 interrupted.
`

func testCLI(ctx context.Context, arguments []string, cwd string, stdout, stderr io.Writer, mutating bool) int {
	format := "text"
	for i, arg := range arguments {
		if arg == "--format=json" || arg == "--format" && i+1 < len(arguments) && arguments[i+1] == "json" {
			format = "json"
		}
	}
	write := func(result projecttest.Report) int {
		if mutating {
			result.Command = "mutation"
		}
		if format == "json" {
			if err := writeJSON(stdout, result); err != nil {
				fmt.Fprintln(stderr, "Could not write test report.")
				return 3
			}
		} else {
			writeTests(stdout, result)
		}
		return result.ExitCode
	}
	invalid := func(message string) int { return write(projecttest.Invalid(message, 2)) }
	scopeArgs := options{}
	options := projecttest.Options{Mutation: mutating}
	sourcePaths := []string{}
	locationOptions := config.LocationOptions{CWD: cwd}
	seen := map[string]bool{}
	positional := false
	for i := 0; i < len(arguments); i++ {
		arg := arguments[i]
		if arg == "--help" || arg == "-h" {
			if mutating {
				fmt.Fprint(stdout, mutationUsage)
			} else {
				fmt.Fprint(stdout, testUsage)
			}
			return 0
		}
		if arg == "--" && !positional {
			positional = true
			continue
		}
		if !strings.HasPrefix(arg, "-") || positional {
			if mutating {
				sourcePaths = append(sourcePaths, arg)
			} else {
				options.Paths = append(options.Paths, arg)
			}
			continue
		}
		name, value, inline := strings.Cut(arg, "=")
		if seen[name] {
			return invalid("Duplicate test option: " + name)
		}
		seen[name] = true
		if mutating && name == "--whole-files" {
			if inline {
				return invalid("--whole-files does not accept a value")
			}
			scopeArgs.wholeFiles = true
			continue
		}
		if !mutating && (name == "--mode" || name == "--base" || name == "--commits" || name == "--infection-config") {
			return invalid("Unknown test option: " + name)
		}
		if name == "--coverage" {
			if inline {
				return invalid("--coverage does not accept a value")
			}
			options.Coverage = true
			continue
		}
		if !inline {
			if i+1 >= len(arguments) || strings.HasPrefix(arguments[i+1], "--") {
				return invalid("Test option requires a value: " + name)
			}
			i++
			value = arguments[i]
		}
		if value == "" || strings.ContainsAny(value, "\x00\r\n") {
			return invalid("Test option requires a nonempty single-line value: " + name)
		}
		switch name {
		case "--infection-config":
			options.MutationConfig = value
		case "--mode":
			if value != "diff" && value != "full" {
				return invalid("mutation mode must be full or diff")
			}
			scopeArgs.mode = value
		case "--base":
			scopeArgs.base = value
		case "--commits":
			count, err := strconv.Atoi(value)
			if err != nil || count < 1 || count > 1000 {
				return invalid("--commits must be between 1 and 1000")
			}
			scopeArgs.commits = count
		case "--project":
			locationOptions.Project = value
		case "--config":
			locationOptions.Config = value
		case "--phpunit-config":
			options.Config = value
		case "--testsuite":
			options.Suite = value
		case "--filter":
			options.Filter = value
		case "--output":
			options.Output = value
		case "--format":
			if value != "text" && value != "json" {
				return invalid("test supports text or json output")
			}
			format = value
		default:
			return invalid("Unknown test option: " + name)
		}
	}
	if len(options.Paths) > 0 && options.Suite != "" {
		return invalid("Test paths and --testsuite are mutually exclusive")
	}
	if buildinfo.Distribution == "container" {
		return invalid("Project tests require the host CLI to create an isolated workspace")
	}
	if options.Output != "" && !filepath.IsAbs(options.Output) {
		options.Output = filepath.Join(cwd, options.Output)
	}
	location, err := config.Resolve(locationOptions)
	if err != nil {
		return invalid(err.Error())
	}
	effective, err := config.Load(location)
	if err != nil {
		return invalid(err.Error())
	}
	effective, err = discovery.Apply(location, effective)
	if err != nil {
		return invalid(err.Error())
	}
	if mutating {
		scopeArgs.command = append([]string{"mutation"}, sourcePaths...)
		cfg := effective.Config
		cfg.Scope.Mode = "full"
		if scopeArgs.commits > 0 || scopeArgs.base != "" || scopeArgs.wholeFiles {
			cfg.Scope.Mode = "diff"
		}
		selectionOptions, err := scopeOptions(scopeArgs, cwd, location, cfg)
		if err != nil {
			return invalid(err.Error())
		}
		options.MutationScopeOptions = selectionOptions
		options.MutationScope, err = scope.Build(ctx, selectionOptions)
		if err != nil {
			return invalid(err.Error())
		}
	}
	ctx, display := startProgress(ctx, format, stdout, stderr)
	defer display.Stop()
	result := launcher.Test(ctx, location, effective, options, runner.Exec{})
	display.Stop()
	return write(result)
}

func writeTests(output io.Writer, result projecttest.Report) {
	label := "project tests"
	if result.Command == "mutation" {
		label = "mutation testing"
	}
	fmt.Fprintf(output, "PHPWarden · %s · %s\n", label, result.Status)
	if result.PHPUnitVersion != "" {
		fmt.Fprintf(output, "PHPUnit %s · %d tests · %d assertions · %d failures · %d errors · %d skipped\n", result.PHPUnitVersion, result.Counts.Tests, result.Counts.Assertions, result.Counts.Failures, result.Counts.Errors, result.Counts.Skipped)
	}
	if result.Coverage != nil {
		coverage := result.Coverage
		fmt.Fprintf(output, "Coverage: %.2f%% lines · %d/%d lines · maximum CRAP %.2f\n", coverage.LinePercent, coverage.CoveredLines, coverage.ExecutableLines, coverage.MaxCRAP)
		for i, method := range coverage.Methods {
			if i == 10 {
				fmt.Fprintf(output, "  … %d more methods in report.json.\n", len(coverage.Methods)-i)
				break
			}
			fmt.Fprintf(output, "  CRAP %.2f · complexity %d · %.2f%% lines · %s::%s\n    %s:%d\n", method.CRAP, method.Complexity, method.LinePercent, method.Class, method.Name, method.File, method.Line)
		}
	}
	if result.Mutation != nil {
		m := result.Mutation
		fmt.Fprintf(output, "\nMutations: %d · MSI %.2f%% · covered MSI %.2f%%\n", m.Stats.Total, m.Stats.MSI, m.Stats.CoveredMSI)
		fmt.Fprintf(output, "  %d killed · %d escaped · %d not covered · %d errors · %d syntax errors · %d timeouts · %d skipped · %d ignored\n", m.Stats.Killed, m.Stats.Escaped, m.Stats.Uncovered, m.Stats.Errors, m.Stats.SyntaxErrors, m.Stats.Timeouts, m.Stats.Skipped, m.Stats.Ignored)
		for i, row := range m.Escaped {
			if i == 10 {
				fmt.Fprintf(output, "  … %d more escaped mutants in report.json.\n", len(m.Escaped)-i)
				break
			}
			fmt.Fprintf(output, "\n  ! %s · %s:%d\n%s\n", row.Mutator.Name, strings.TrimPrefix(row.Mutator.File, "/workspace/"), row.Mutator.Line, row.Diff)
		}
	}
	shown, total := 0, 0
	for _, test := range result.Cases {
		issues := append(append([]projecttest.Issue{}, test.Failures...), test.Errors...)
		if len(issues) == 0 {
			continue
		}
		total++
		if shown == 10 {
			continue
		}
		shown++
		fmt.Fprintf(output, "\n  ✗ %s::%s\n", test.Class, test.Name)
		if test.File != "" {
			fmt.Fprintf(output, "    %s:%d\n", test.File, test.Line)
		}
		for _, issue := range issues {
			message := issue.Message
			if message == "" {
				message = issue.Detail
			}
			if len(message) > 2000 {
				message = message[:2000] + "…"
			}
			fmt.Fprintf(output, "    %s\n", strings.ReplaceAll(message, "\n", "\n    "))
		}
	}
	if total > shown {
		fmt.Fprintf(output, "\n  … %d more failed tests; see report.json and junit.xml.\n", total-shown)
	}
	for _, message := range result.Diagnostics {
		fmt.Fprintf(output, "\n  ✗ %s\n", message)
	}
	if result.Artifacts != "" {
		fmt.Fprintf(output, "\nArtifacts: %s\n", result.Artifacts)
	}
	fmt.Fprintf(output, "Result: %s · exit %d\n", strings.ToUpper(result.Status), result.ExitCode)
}
