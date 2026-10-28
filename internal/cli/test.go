package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
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
Exit: 0 passed; 1 failed tests; 2 input/setup selection; 3 incomplete run; 130 interrupted.
`

func testCLI(ctx context.Context, arguments []string, cwd string, stdout, stderr io.Writer) int {
	format := "text"
	for i, arg := range arguments {
		if arg == "--format=json" || arg == "--format" && i+1 < len(arguments) && arguments[i+1] == "json" {
			format = "json"
		}
	}
	write := func(result projecttest.Report) int {
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
	options := projecttest.Options{}
	locationOptions := config.LocationOptions{CWD: cwd}
	seen := map[string]bool{}
	positional := false
	for i := 0; i < len(arguments); i++ {
		arg := arguments[i]
		if arg == "--help" || arg == "-h" {
			fmt.Fprint(stdout, testUsage)
			return 0
		}
		if arg == "--" && !positional {
			positional = true
			continue
		}
		if !strings.HasPrefix(arg, "-") || positional {
			options.Paths = append(options.Paths, arg)
			continue
		}
		name, value, inline := strings.Cut(arg, "=")
		if seen[name] {
			return invalid("Duplicate test option: " + name)
		}
		seen[name] = true
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
	ctx, display := startProgress(ctx, format, stdout, stderr)
	defer display.Stop()
	result := launcher.Test(ctx, location, effective, options, runner.Exec{})
	display.Stop()
	return write(result)
}

func writeTests(output io.Writer, result projecttest.Report) {
	fmt.Fprintf(output, "PHPWarden · project tests · %s\n", result.Status)
	if result.PHPUnitVersion != "" {
		fmt.Fprintf(output, "PHPUnit %s · %d tests · %d assertions · %d failures · %d errors · %d skipped\n", result.PHPUnitVersion, result.Counts.Tests, result.Counts.Assertions, result.Counts.Failures, result.Counts.Errors, result.Counts.Skipped)
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
