package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rchehd/phpwarden/internal/performance"
)

func performanceCLI(ctx context.Context, arguments []string, cwd string, stdout, stderr io.Writer) int {
	if len(arguments) > 0 && (arguments[0] == "setup" || arguments[0] == "status" || arguments[0] == "teardown") {
		return performanceRuntimeCLI(ctx, arguments, cwd, stdout, stderr)
	}
	settings := performance.Options{}
	format := "text"
	for i, argument := range arguments {
		if argument == "--format=json" || argument == "--format" && i+1 < len(arguments) && arguments[i+1] == "json" {
			format = "json"
		}
	}
	write := func(result performance.Report) int {
		if format == "json" {
			if err := writeJSON(stdout, result); err != nil {
				fmt.Fprintln(stderr, "Could not write the performance report.")
				return 3
			}
		} else {
			writePerformance(stdout, result)
		}
		return result.ExitCode
	}
	invalid := func(message string) int { return write(performance.InputError(message)) }
	seen := map[string]bool{}
	for i := 0; i < len(arguments); i++ {
		name, value, inline := strings.Cut(arguments[i], "=")
		if name == "--help" || name == "-h" {
			fmt.Fprintf(stdout, "Usage: phpwarden performance --reports DIR --producer COMMIT [options]\n       phpwarden performance setup|status|teardown [options]\n       phpwarden performance mcp --reports DIR --producer COMMIT [options]\n\nRead existing complete SPX profiles without running the project, PHP, Docker or MCP.\n\n  --reports DIR         Directory containing metadata/body report pairs\n  --report KEY          Inspect one report (default: validate the directory)\n  --producer COMMIT     Verified SPX producer: %s\n  --project PATH        Local workspace for source mapping\n  --container-root DIR  Corresponding absolute root in captured source paths\n  --limit N             Top functions per report, 1–20 (default 10)\n  --format text|json    Output format (default text)\n\nProducer identity is an explicit assertion. Unknown/sampled/partial captures are rejected.\nSource mapping needs both --project and --container-root. Reading reports installs nothing.\nSee performance setup --help for local runtime installation.\n", performance.Producer)
			return 0
		}
		switch name {
		case "--reports", "--report", "--producer", "--project", "--container-root", "--limit", "--format":
		default:
			return invalid("Unknown performance option; see performance --help.")
		}
		if seen[name] {
			return invalid("Duplicate performance option: " + name)
		}
		seen[name] = true
		if !inline {
			if i+1 >= len(arguments) || strings.HasPrefix(arguments[i+1], "--") {
				return invalid(name + " requires a value.")
			}
			i++
			value = arguments[i]
		}
		if value == "" {
			return invalid(name + " requires a nonempty value.")
		}
		switch name {
		case "--reports":
			settings.Directory = value
		case "--report":
			settings.Key = value
		case "--producer":
			settings.Producer = value
		case "--project":
			settings.Workspace = value
		case "--container-root":
			settings.ContainerRoot = value
		case "--limit":
			n, err := strconv.Atoi(value)
			if err != nil {
				return invalid("--limit must be an integer from 1 to 20.")
			}
			if n < 1 || n > 20 {
				return invalid("--limit must be an integer from 1 to 20.")
			}
			settings.Top = n
		case "--format":
			if value != "text" && value != "json" {
				return invalid("Performance supports text or json output.")
			}
			format = value
		}
	}
	for _, value := range []*string{&settings.Directory, &settings.Workspace} {
		if *value != "" && !filepath.IsAbs(*value) {
			*value = filepath.Join(cwd, *value)
		}
	}
	return write(performance.Run(ctx, settings))
}

func writePerformance(output io.Writer, result performance.Report) {
	status := "complete"
	if !result.Complete {
		status = "incomplete"
	}
	fmt.Fprintf(output, "PHPWarden · performance · %s · %d report(s)\n", status, len(result.Profiles))
	for _, profile := range result.Profiles {
		fmt.Fprintf(output, "\n%s · %s\n  %s · %.3f ms · peak %.2f MiB · %d calls\n", profile.Key, profile.Description, profile.RecordedAt, profile.WallMS, float64(profile.PeakMemory)/(1<<20), profile.Calls)
		if profile.CPUOmittedReason != "" {
			fmt.Fprintln(output, "  "+profile.CPUOmittedReason)
		}
		fmt.Fprintln(output, "\n  Self ms   Total ms   Calls   Function")
		for _, function := range profile.Functions {
			fmt.Fprintf(output, "  %8.3f  %9.3f  %6d   %s\n", function.SelfMS, function.InclusiveMS, function.Calls, function.Name)
			if function.Mapped {
				fmt.Fprintf(output, "                                 %s:%d (definition)\n", function.File, function.Line)
			}
			for _, caller := range function.Callers {
				fmt.Fprintf(output, "                                 from %s × %d\n", caller.Name, caller.Calls)
			}
			if function.OmittedCallers > 0 {
				fmt.Fprintf(output, "                                 %d other caller(s) omitted\n", function.OmittedCallers)
			}
		}
		if profile.OmittedFunctions > 0 {
			fmt.Fprintf(output, "  %d function(s) omitted from this summary.\n", profile.OmittedFunctions)
		}
	}
	for _, diagnostic := range result.Diagnostics {
		fmt.Fprintf(output, "\n  ✗ %s", diagnostic.Code)
		if diagnostic.Report != "" {
			fmt.Fprintf(output, " [%s]", diagnostic.Report)
		}
		fmt.Fprintf(output, ": %s\n", diagnostic.Message)
	}
	for _, note := range result.Notes {
		fmt.Fprintln(output, "\n"+note)
	}
	fmt.Fprintf(output, "\nResult: %s · exit %d\n", status, result.ExitCode)
}
