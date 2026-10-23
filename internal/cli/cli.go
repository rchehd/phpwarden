package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/checker/composeraudit"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/launcher"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/runrequest"
	"github.com/rchehd/phpwarden/internal/scope"
	"github.com/rchehd/phpwarden/internal/textreport"
	"github.com/rchehd/phpwarden/internal/toolset"
	"github.com/rchehd/phpwarden/internal/worker"
	"github.com/rchehd/phpwarden/schemas"
)

const usage = `Usage: phpwarden config <validate|show> [options]
       phpwarden scope [PATH...] [options]
       phpwarden resolve [options]
       phpwarden check [PATH...] [options]
       phpwarden worker check [PATH...] [options]

Options:
  --project PATH       Project root
  --config PATH        Configuration file, relative to caller directory
  --format FORMAT      text (grouped, default), compact (one path:line:col per finding; check only), or json
  --color MODE         auto (default; terminals without NO_COLOR), always, or never
  --help               Show help
  --version            Show version

Scope options:
  --mode diff|full     Override configured mode
  --base REF           Diff base ref
  --whole-files        Include all lines of selected diff files
  --only ID[,ID...]    Select composer-audit, composer-validate, debug, drupal-config, phpcs, phpstan, syntax, twig-cs-fixer, whitespace, yaml; enable selected checkers

Drupal source roots are discovered from Composer/layout, or overridden by scope.include.
Check uses the local development Docker image (Linux amd64, PHP runtime/target 8.4).
Worker check requires PHP in its execution environment. Published images and init are not available yet.
`

type options struct {
	project    string
	config     string
	format     string
	color      string
	help       bool
	version    bool
	command    []string
	mode       string
	base       string
	wholeFiles bool
	only       string
}

type response struct {
	Version     int                `json:"version"`
	Command     string             `json:"command"`
	Valid       bool               `json:"valid"`
	ExitCode    int                `json:"exit_code"`
	Location    *config.Location   `json:"location,omitempty"`
	Effective   *config.Effective  `json:"effective,omitempty"`
	Scope       *scope.Selection   `json:"scope,omitempty"`
	Selection   *toolset.Selection `json:"selection,omitempty"`
	Diagnostics []diagnostic       `json:"diagnostics"`
}

type diagnostic struct {
	Code    string `json:"code"`
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

func Run(arguments []string, cwd string, stdout, stderr io.Writer) int {
	return RunContext(context.Background(), arguments, cwd, os.Stdin, stdout, stderr)
}

func RunContext(ctx context.Context, arguments []string, cwd string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(arguments) >= 2 && arguments[0] == "worker" && arguments[1] == "audit" {
		return workerAudit(ctx, arguments[2:], cwd, stdout, stderr)
	}
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
	if len(args.command) == 2 && args.command[0] == "worker" && args.command[1] == "execute" {
		if len(arguments) != 2 {
			return writeReport(stdout, stderr, "json", "never", report.InputError("request", fmt.Errorf("worker execute accepts only its stdin request")))
		}
		decoder := json.NewDecoder(io.LimitReader(stdin, 32<<20))
		decoder.DisallowUnknownFields()
		var request runrequest.Request
		if err := decoder.Decode(&request); err != nil {
			return writeReport(stdout, stderr, "json", "never", report.InputError("request", err))
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return writeReport(stdout, stderr, "json", "never", report.InputError("request", fmt.Errorf("expected one run request")))
		}
		return writeReport(stdout, stderr, "json", "never", worker.Execute(ctx, cwd, request, runner.Exec{}))
	}
	isScope := len(args.command) > 0 && args.command[0] == "scope"
	isWorker := workerCommand(args)
	isCheck := len(args.command) > 0 && args.command[0] == "check"
	isResolve := len(args.command) == 1 && args.command[0] == "resolve"
	isConfig := len(args.command) == 2 && args.command[0] == "config" &&
		(args.command[1] == "validate" || args.command[1] == "show")
	if args.format == "compact" && !isWorker && !isCheck {
		return fail(stdout, stderr, args, "usage", fmt.Errorf("--format compact applies only to check and worker check"))
	}
	if !isScope && !isConfig && !isWorker && !isCheck && !isResolve {
		return fail(stdout, stderr, args, "usage", fmt.Errorf("expected config validate/show, resolve, scope, check, or worker check; use --help"))
	}
	if !isScope && !isWorker && !isCheck && (args.mode != "" || args.base != "" || args.wholeFiles) {
		return fail(stdout, stderr, args, "usage", fmt.Errorf("scope options require the scope command"))
	}
	only, err := checker.ParseOnly(args.only)
	if err != nil {
		return fail(stdout, stderr, args, "usage", err)
	}
	if args.only != "" && !isWorker && !isCheck {
		return fail(stdout, stderr, args, "usage", fmt.Errorf("--only requires check or worker check"))
	}
	location, err := config.Resolve(config.LocationOptions{CWD: cwd, Project: args.project, Config: args.config})
	if err != nil {
		return fail(stdout, stderr, args, "config", err)
	}
	effective, err := config.Load(location)
	if err != nil {
		return fail(stdout, stderr, args, "config", err)
	}
	if !isConfig || args.command[1] == "show" {
		effective, err = discovery.Apply(location, effective)
		if err != nil {
			return fail(stdout, stderr, args, "discovery", err)
		}
	}
	if isScope {
		return showScope(args, cwd, location, effective.Config, stdout, stderr)
	}
	if isWorker || isCheck {
		workerArgs := args
		if isWorker {
			workerArgs.command = args.command[1:]
		}
		options, err := scopeOptions(workerArgs, cwd, location, effective.Config)
		if err != nil {
			return fail(stdout, stderr, args, "usage", err)
		}
		var result report.Report
		if isCheck {
			result = launcher.Check(ctx, location, effective, options, only, runner.Exec{})
		} else {
			result = worker.Check(ctx, location, effective, options, only, runner.Exec{})
		}
		return writeReport(stdout, stderr, args.format, args.color, result)
	}
	result := response{
		Version: 1, Command: strings.Join(args.command, " "), Valid: true,
		Location: &location, Diagnostics: []diagnostic{},
	}
	if isConfig && args.command[1] == "show" {
		result.Effective = &effective
	}
	if isResolve {
		selection, err := toolset.Resolve(effective)
		if err != nil {
			return fail(stdout, stderr, args, "compatibility", err)
		}
		result.Selection = &selection
	}
	if args.format == "json" {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 3
		}
	} else if isResolve {
		label := "Toolset"
		if result.Selection.Development {
			label = "Development toolset"
		}
		fmt.Fprintf(stdout, "%s: %s\nManifest: %s\nEntry: %s\nImage: %s\nPlatform: %s\n", label,
			result.Selection.ID, result.Selection.Release, result.Selection.Entry, result.Selection.Image, result.Selection.Platform)
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
	result := options{format: "text", color: "auto"}
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
		if name != "--project" && name != "--config" && name != "--format" && name != "--mode" && name != "--base" && name != "--only" && name != "--color" {
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
			if value != "text" && value != "json" && value != "compact" {
				return result, fmt.Errorf("unsupported format %q", value)
			}
			result.format = value
		case "--color":
			if value != "auto" && value != "always" && value != "never" {
				return result, fmt.Errorf("unsupported color mode %q", value)
			}
			result.color = value
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

// workerAudit prints Composer's audit report for the composer.json and composer.lock in cwd; the launcher runs it in a networked container.
func workerAudit(ctx context.Context, arguments []string, cwd string, stdout, stderr io.Writer) int {
	if len(arguments) > 1 || len(arguments) == 1 && arguments[0] != "--no-dev" {
		fmt.Fprintln(stderr, "worker audit accepts only --no-dev")
		return 2
	}
	files := [2][]byte{}
	for i, name := range []string{"composer.json", "composer.lock"} {
		data, err := os.ReadFile(filepath.Join(cwd, name))
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 2
		}
		files[i] = data
	}
	output, err := composeraudit.NativeFetcher(composeraudit.Bundle(), runner.Exec{})(ctx, files[0], files[1], len(arguments) == 1)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	fmt.Fprintln(stdout, string(output))
	return 0
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
	defaults, _ := profile.Lookup(cfg.Profile)
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
		Include: cfg.Scope.Include, Exclude: cfg.Scope.Exclude, Paths: paths, ProjectFiles: defaults.ProjectFiles,
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
	if workerCommand(args) || len(args.command) > 0 && args.command[0] == "check" {
		return writeReport(stdout, stderr, args.format, args.color, report.InputError(stage, err))
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

func writeReport(stdout, stderr io.Writer, format, color string, result report.Report) int {
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
	} else if err := textreport.Write(stdout, result, textreport.Options{Compact: format == "compact", Color: useColor(color, stdout)}); err != nil {
		fmt.Fprintln(stderr, err)
		return 3
	}
	return result.ExitCode
}

func useColor(mode string, stdout io.Writer) bool {
	if mode != "auto" {
		return mode == "always"
	}
	file, ok := stdout.(*os.File)
	if !ok || os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func writeJSON(writer io.Writer, value any) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
