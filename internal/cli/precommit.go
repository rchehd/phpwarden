package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/githook"
	"github.com/rchehd/phpwarden/internal/launcher"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func preCommit(ctx context.Context, args options, cwd string, stdout, stderr io.Writer) int {
	failure := func(err error) int {
		fmt.Fprintf(stderr, "PHPWarden pre-commit: %v\n", err)
		if ctx.Err() == context.Canceled {
			return 130
		}
		if ctx.Err() != nil {
			return 3
		}
		return 2
	}
	if args.format != "text" || args.mode != "" || args.base != "" || args.commits != 0 || args.wholeFiles || args.fix || args.preCommit || args.phpRuntime != "" || args.phpTarget != "" || args.composerRoot != "" || args.config != "" || args.only != "" || args.noCache {
		return failure(fmt.Errorf("hook pre-commit accepts only --project, --color and --all; configure checks in .phpwarden.yaml"))
	}
	directory := cwd
	if args.project != "" {
		directory = args.project
		if !filepath.IsAbs(directory) {
			directory = filepath.Join(cwd, directory)
		}
	}
	snapshot, err := githook.Read(ctx, directory)
	if err != nil {
		return failure(err)
	}
	if len(snapshot.Files) == 0 {
		fmt.Fprintln(stdout, "PHPWarden: no staged files to check.")
		return 0
	}
	location, err := config.Resolve(config.LocationOptions{CWD: snapshot.Root, Project: snapshot.Root})
	if err != nil {
		return failure(err)
	}
	effective, err := config.Load(location)
	if err != nil {
		return failure(err)
	}
	effective, err = discovery.Apply(location, effective)
	if err != nil {
		return failure(err)
	}
	options, err := scopeOptions(options{command: []string{"check"}, mode: "full"}, snapshot.Root, location, effective.Config)
	if err != nil {
		return failure(err)
	}
	candidates := (scope.Selection{Files: snapshot.Files}).Filter(options.Include, options.Exclude, func(string) bool { return true }).Files
	for _, name := range snapshot.Files {
		if slices.Contains(options.ProjectFiles, name) && !slices.Contains(candidates, name) {
			candidates = append(candidates, name)
		}
	}
	if err := snapshot.Check(ctx, candidates); err != nil {
		return failure(err)
	}
	if len(candidates) == 0 {
		fmt.Fprintln(stdout, "PHPWarden: no staged files in the configured scope.")
		return 0
	}
	inventory, err := scope.Build(ctx, options)
	if err != nil {
		return failure(err)
	}
	selected := []string{}
	for _, name := range candidates {
		if slices.Contains(inventory.Files, name) || slices.Contains(inventory.ProjectFiles, name) {
			selected = append(selected, name)
			continue
		}
		info, err := os.Lstat(filepath.Join(snapshot.Root, filepath.FromSlash(name)))
		if err != nil {
			return failure(err)
		}
		if info.Mode().IsRegular() {
			return failure(fmt.Errorf("cannot include staged file %q in the worker scope; use the normal Git index or adjust ignore/source-root settings", name))
		}
	}
	if len(selected) == 0 {
		fmt.Fprintln(stdout, "PHPWarden: no staged files in the configured scope.")
		return 0
	}
	if err := snapshot.Check(ctx, selected); err != nil {
		return failure(err)
	}
	options.Mode, options.Paths = "paths", selected
	fmt.Fprintf(stdout, "PHPWarden pre-commit · %d staged file(s) · whole files\n", len(selected))
	ctx, display := startProgress(ctx, "text", stdout, stderr)
	defer display.Stop()
	result := launcher.Check(ctx, location, effective, options, nil, runner.Exec{})
	err = snapshot.Check(ctx, selected)
	if ctx.Err() != nil {
		display.Stop()
		return failure(ctx.Err())
	}
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "stale", Stage: "pre-commit", Message: err.Error()})
		result.Finalize()
	}
	display.Stop()
	return writeReport(stdout, stderr, "text", args.color, result, args.all)
}
