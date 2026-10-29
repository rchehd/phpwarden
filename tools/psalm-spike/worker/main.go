package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"slices"

	"github.com/rchehd/phpwarden/internal/checker/psalm"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/protocol"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
	"github.com/rchehd/phpwarden/internal/textreport"
	"github.com/rchehd/phpwarden/internal/worker"
)

func main() {
	project := flag.String("project", "", "Installed disposable Drupal fixture")
	bundle := flag.String("bundle", "", "Installed isolated Psalm vendor")
	php := flag.String("php", "php", "Native PHP executable")
	format := flag.String("format", "native", "Output: native evidence, json report or text report")
	all := flag.Bool("all", false, "Expand every text finding and flow step")
	flag.Parse()
	if (*format != "native" && *format != "json" && *format != "text") || *all && *format != "text" || flag.NArg() > 0 {
		fmt.Fprintln(os.Stderr, "Use --format native|json|text; --all applies only to text")
		os.Exit(2)
	}
	location, err := config.Resolve(config.LocationOptions{Project: *project})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	effective, err := config.Load(location)
	if err == nil {
		effective, err = discovery.Apply(location, effective)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	ctx := context.Background()
	native := worker.AnalyzePsalm(ctx, location, effective, worker.PsalmOptions{Bundle: *bundle, PHP: *php, MemoryMB: 1536}, runner.Exec{})
	if *format == "native" {
		err = json.NewEncoder(os.Stdout).Encode(native)
	} else {
		selection := scope.Selection{Mode: "full", Files: slices.Clone(native.SourceFiles), LineMap: map[string][]scope.Range{}, ProjectFiles: slices.Clone(native.ContextFiles)}
		if selection.Files == nil {
			selection.Files = []string{}
		}
		if selection.ProjectFiles == nil {
			selection.ProjectFiles = []string{}
		}
		for _, name := range selection.Files {
			selection.LineMap[name] = nil
		}
		outcome := psalm.Normalize(native, selection, psalm.Policy{Required: true, FailOn: "never"})
		result := report.New()
		result.Profile, result.Scope = new(effective.Config.Profile), &selection
		result.Checks, result.Findings, result.Diagnostics = []report.Check{outcome.Check}, outcome.Findings, outcome.Diagnostics
		result.Environment = &report.Environment{PHPRuntime: outcome.Runtime, PHPVersion: outcome.PHPVersion, RequestedRuntime: effective.Config.PHP.Runtime, TargetMin: effective.Config.PHP.Target.Min, TargetMax: effective.Config.PHP.Target.Max, Protocol: protocol.Version}
		if effective.Project != nil {
			result.Environment.FrameworkVersion = effective.Project.Version
		}
		result.Finalize()
		if *format == "text" {
			err = textreport.Write(os.Stdout, result, textreport.Options{All: *all})
		} else {
			err = json.NewEncoder(os.Stdout).Encode(result)
		}
		if err == nil {
			os.Exit(result.ExitCode)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(3)
	}
	if !native.Complete {
		os.Exit(3)
	}
}
