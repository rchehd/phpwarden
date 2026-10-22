package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
	"github.com/rchehd/phpwarden/internal/worker"
	"github.com/rchehd/phpwarden/schemas"
)

const usage = `Usage: phpwarden config <validate|show> [options]
       phpwarden scope [PATH...] [options]
       phpwarden worker check [PATH...] [options]

Options:
  --project PATH       Project root
  --config PATH        Configuration file, relative to caller directory
  --format text|json   Output format (default text)
  --help               Show help
  --version            Show version

Scope options:
  --mode diff|full     Override configured mode
  --base REF           Diff base ref
  --whole-files        Include all lines of selected diff files
  --only debug         Explicitly select the debug checker (worker only)

Scope inspection requires explicit scope.include roots until profile discovery is available.
Worker check requires PHP in its execution environment. Docker launcher and init are not available yet.
`

type options struct {
	project    string
	config     string
	format     string
	help       bool
	version    bool
	command    []string
	mode       string
	base       string
	wholeFiles bool
	only       string
}

type response struct {
	Version     int               `json:"version"`
	Command     string            `json:"command"`
	Valid       bool              `json:"valid"`
	ExitCode    int               `json:"exit_code"`
	Location    *config.Location  `json:"location,omitempty"`
	Effective   *config.Effective `json:"effective,omitempty"`
	Scope       *scope.Selection  `json:"scope,omitempty"`
	Diagnostics []diagnostic      `json:"diagnostics"`
}

type diagnostic struct {
	Code    string `json:"code"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

func Run(arguments []string, cwd string, stdout, stderr io.Writer) int {
	args, err := parse(arguments)
	if err != nil {
		return fail(stdout, stderr, args, "usage", err)
	}
	if args.help || len(arguments) == 0 {
		fmt.Fprint(stdout, usage)
		return 0
	}
	if args.version {
		fmt.Fprintln(stdout, "phpwarden dev")
		return 0
	}
	isScope := len(args.command) > 0 && args.command[0] == "scope"
	isWorker := workerCommand(args)
	isConfig := len(args.command) == 2 && args.command[0] == "config" &&
		(args.command[1] == "validate" || args.command[1] == "show")
	if !isScope && !isConfig && !isWorker {
		return fail(stdout, stderr, args, "usage", fmt.Errorf("expected config validate, config show, scope, or worker check; use --help"))
	}
	if !isScope && !isWorker && (args.mode != "" || args.base != "" || args.wholeFiles) {
		return fail(stdout, stderr, args, "usage", fmt.Errorf("scope options require the scope command"))
	}
	if args.only != "" && (!isWorker || args.only != "debug") {
		return fail(stdout, stderr, args, "usage", fmt.Errorf("--only supports debug in worker check; unknown or unavailable checker %q", args.only))
	}
	location, err := config.Resolve(config.LocationOptions{CWD: cwd, Project: args.project, Config: args.config})
	if err != nil {
		return fail(stdout, stderr, args, "config", err)
	}
	effective, err := config.Load(location)
	if err != nil {
		return fail(stdout, stderr, args, "config", err)
	}
	if isScope {
		return showScope(args, cwd, location, effective.Config, stdout, stderr)
	}
	if isWorker {
		workerArgs := args
		workerArgs.command = args.command[1:]
		options, err := scopeOptions(workerArgs, cwd, location, effective.Config)
		if err != nil {
			return fail(stdout, stderr, args, "usage", err)
		}
		result := worker.Check(context.Background(), location, effective, options, args.only == "debug", runner.Exec{})
		return writeReport(stdout, stderr, args.format, result)
	}
	result := response{
		Version: 1, Command: strings.Join(args.command, " "), Valid: true,
		Location: &location, Diagnostics: []diagnostic{},
	}
	if args.command[1] == "show" {
		result.Effective = &effective
	}
	if args.format == "json" {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 3
		}
	} else if args.command[1] == "show" {
		fmt.Fprintf(stdout, "Project: %s\nConfig: %s\n", location.Root, location.Path)
		if err := writeJSON(stdout, effective); err != nil {
			fmt.Fprintln(stderr, err)
			return 3
		}
	} else {
		fmt.Fprintf(stdout, "Valid configuration: %s\n", location.Path)
	}
	return 0
}

func parse(arguments []string) (options, error) {
	result := options{format: "text"}
	seen := map[string]bool{}
	for i := 0; i < len(arguments); i++ {
		argument := arguments[i]
		if argument == "--help" || argument == "-h" {
			result.help = true
			continue
		}
		if argument == "--version" {
			result.version = true
			continue
		}
		if argument == "--whole-files" {
			if result.wholeFiles {
				return result, fmt.Errorf("duplicate option --whole-files")
			}
			result.wholeFiles = true
			continue
		}
		if !strings.HasPrefix(argument, "-") {
			result.command = append(result.command, argument)
			continue
		}
		name, value, inline := strings.Cut(argument, "=")
		if name != "--project" && name != "--config" && name != "--format" && name != "--mode" && name != "--base" && name != "--only" {
			return result, fmt.Errorf("unknown option %q", name)
		}
		if seen[name] {
			return result, fmt.Errorf("duplicate option %s", name)
		}
		seen[name] = true
		if !inline {
			if i+1 >= len(arguments) || strings.HasPrefix(arguments[i+1], "-") {
				return result, fmt.Errorf("%s requires a value", name)
			}
			i++
			value = arguments[i]
		}
		if value == "" {
			return result, fmt.Errorf("%s requires a nonempty value", name)
		}
		switch name {
		case "--only":
			result.only = value
		case "--mode":
			if value != "diff" && value != "full" {
				return result, fmt.Errorf("unsupported mode %q", value)
			}
			result.mode = value
		case "--base":
			result.base = value
		case "--project":
			result.project = value
		case "--config":
			result.config = value
		case "--format":
			if value != "text" && value != "json" {
				return result, fmt.Errorf("unsupported format %q", value)
			}
			result.format = value
		}
	}
	if result.version && (len(result.command) != 0 || len(seen) != 0 || result.help || result.wholeFiles) {
		return result, fmt.Errorf("--version cannot be combined with a command or options")
	}
	return result, nil
}

func showScope(args options, cwd string, location config.Location, cfg config.Config, stdout, stderr io.Writer) int {
	options, err := scopeOptions(args, cwd, location, cfg)
	if err != nil {
		return fail(stdout, stderr, args, "usage", err)
	}
	selection, err := scope.Build(context.Background(), options)
	if err != nil {
		return fail(stdout, stderr, args, "scope", err)
	}
	return writeScope(args, location, selection, stdout, stderr)
}

func scopeOptions(args options, cwd string, location config.Location, cfg config.Config) (scope.Options, error) {
	mode := cfg.Scope.Mode
	if args.mode != "" {
		mode = args.mode
	}
	paths := args.command[1:]
	if len(paths) > 0 {
		if args.mode != "" || args.base != "" || args.wholeFiles {
			return scope.Options{}, fmt.Errorf("explicit paths cannot be combined with mode/base/whole-files")
		}
		mode = "paths"
	}
	if mode != "diff" && (args.base != "" || args.wholeFiles) {
		return scope.Options{}, fmt.Errorf("base and whole-files apply only to diff")
	}
	base, wholeFiles := "", false
	if mode == "diff" {
		base, wholeFiles = cfg.Scope.Base, cfg.Scope.WholeFiles
		if args.base != "" {
			base = args.base
		}
		wholeFiles = wholeFiles || args.wholeFiles
	}
	return scope.Options{
		Root: location.Root, CWD: cwd, Mode: mode, Base: base, WholeFiles: wholeFiles,
		Include: cfg.Scope.Include, Exclude: cfg.Scope.Exclude, Paths: paths,
	}, nil
}

func writeScope(args options, location config.Location, selection scope.Selection, stdout, stderr io.Writer) int {
	if args.format == "json" {
		result := response{Version: 1, Command: "scope", Valid: true, Location: &location, Scope: &selection, Diagnostics: []diagnostic{}}
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 3
		}
	} else {
		fmt.Fprintf(stdout, "Scope: %s (%d files)\n", selection.Mode, len(selection.Files))
		for _, filename := range selection.Files {
			fmt.Fprintf(stdout, "%q: ", filename)
			if selection.LineMap[filename] == nil {
				fmt.Fprintln(stdout, "all lines")
			} else {
				fmt.Fprintln(stdout, selection.LineMap[filename])
			}
		}
	}
	return 0
}

func fail(stdout, stderr io.Writer, args options, stage string, err error) int {
	if workerCommand(args) {
		return writeReport(stdout, stderr, args.format, report.InputError(stage, err))
	}
	if args.format == "json" {
		result := response{
			Version: 1, Command: strings.Join(args.command, " "), Valid: false, ExitCode: 2,
			Diagnostics: []diagnostic{{Code: stage + "_invalid", Stage: stage, Message: err.Error()}},
		}
		if writeErr := writeJSON(stdout, result); writeErr != nil {
			fmt.Fprintln(stderr, writeErr)
			return 3
		}
	} else {
		fmt.Fprintf(stderr, "phpwarden: %v\n", err)
	}
	return 2
}

func workerCommand(args options) bool {
	return len(args.command) >= 2 && args.command[0] == "worker" && args.command[1] == "check"
}

func writeReport(stdout, stderr io.Writer, format string, result report.Report) int {
	encoded, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	var document any
	if err := json.Unmarshal(encoded, &document); err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	if err := schemas.ValidateReport(document); err != nil {
		fmt.Fprintln(stderr, "invalid internal report:", err)
		return 3
	}
	if format == "json" {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 3
		}
	} else {
		label := strings.ToUpper(result.Verdict)
		if !result.Complete && result.ExitCode == 0 {
			label = "PARTIAL"
		}
		fmt.Fprintf(stdout, "%s (%d findings)\n", label, len(result.Findings))
		for _, finding := range result.Findings {
			if finding.Path != nil && finding.Line != nil {
				fmt.Fprintf(stdout, "%q:%d: %s\n", *finding.Path, *finding.Line, finding.Message)
			}
		}
		for _, check := range result.Checks {
			if check.Reason != nil {
				fmt.Fprintf(stdout, "%s: %s (%s)\n", check.ID, check.Status, *check.Reason)
			}
		}
		for _, diagnostic := range result.Diagnostics {
			fmt.Fprintf(stdout, "%s: %s\n", diagnostic.Code, diagnostic.Message)
		}
	}
	return result.ExitCode
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
