package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/launcher"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
)

func prepareProject(ctx context.Context, args options, cwd string, stdout, stderr io.Writer) int {
	if len(args.command) != 1 || args.format == "compact" || args.fix || args.all || args.noCache || args.noInteraction || args.preCommit || args.agents != "" || args.hooks != "" || args.only != "" || args.mode != "" || args.base != "" || args.wholeFiles || args.commits != 0 || args.phpRuntime != "" || args.phpTarget != "" || args.composerRoot != "" || args.security || args.direct || args.noDev || args.offline || args.runtimeVersion != "" {
		return fail(stdout, stderr, args, "usage", fmt.Errorf("prepare accepts --project, --config, --color and text/JSON output; set runtime and toolset in .phpwarden.yaml"))
	}
	ctx, display := startProgress(ctx, args.format, stdout, stderr)
	defer display.Stop()
	if err := ctx.Err(); err != nil {
		display.Stop()
		code, diagnosticCode := 3, "prepare_timeout"
		if ctx.Err() == context.Canceled {
			code, diagnosticCode = 130, "prepare_cancelled"
		}
		return writePreparation(stdout, stderr, args.format, launcher.Preparation{Version: 1, Command: "prepare", ExitCode: code, Diagnostics: []report.Diagnostic{{Code: diagnosticCode, Stage: "prepare", Message: err.Error()}}})
	}
	location, err := config.Resolve(config.LocationOptions{CWD: cwd, Project: args.project, Config: args.config})
	if err != nil {
		display.Stop()
		return fail(stdout, stderr, args, "config", err)
	}
	effective, err := config.Load(location)
	if err != nil {
		display.Stop()
		return fail(stdout, stderr, args, "config", err)
	}
	display.Stage("Detecting project compatibility")
	effective, err = discovery.Apply(location, effective)
	if err != nil {
		display.Stop()
		return fail(stdout, stderr, args, "discovery", err)
	}
	result := launcher.Prepare(ctx, location, effective, runner.Exec{})
	display.Stop()
	return writePreparation(stdout, stderr, args.format, result)
}

func writePreparation(stdout, stderr io.Writer, format string, result launcher.Preparation) int {
	if format == "json" {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 3
		}
	} else if result.Ready {
		fmt.Fprintf(stdout, "PHPWarden · worker image ready · PHP %s\nImage: %s\nPlatform: %s\nToolset: %s\nLocal image: %s\n", result.Selection.PHPVersion, result.Selection.Image, result.Selection.Platform, result.Selection.ID, result.ImageID)
	} else {
		fmt.Fprintf(stdout, "PHPWarden · image preparation failed · exit %d\n", result.ExitCode)
		for _, diagnostic := range result.Diagnostics {
			fmt.Fprintf(stdout, "  %s\n", diagnostic.Message)
		}
	}
	return result.ExitCode
}
