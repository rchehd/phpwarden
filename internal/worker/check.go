package worker

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker/debug"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/runrequest"
	"github.com/rchehd/phpwarden/internal/scope"
)

func Check(ctx context.Context, location config.Location, effective config.Effective, options scope.Options, onlyDebug bool, process runner.Process) report.Report {
	cfg := effective.Config
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Runner.TimeoutSeconds)*time.Second)
	defer cancel()
	selection, err := scope.Build(ctx, options)
	if err != nil {
		if ctx.Err() != nil {
			result := report.New()
			result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "worker_cancelled", Stage: "worker", Message: ctx.Err().Error()})
			result.Finalize()
			return result
		}
		return report.InputError("scope", err)
	}
	result := newReport(cfg, selection)
	failure := func(code string, err error) report.Report {
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "worker", Message: err.Error()})
		result.Finalize()
		return result
	}
	before, err := runrequest.Snapshot(location, selection)
	if err != nil {
		return failure("input_read", err)
	}
	if before["config"] != effective.SourceHash {
		return failure("stale", fmt.Errorf("configuration changed after loading"))
	}
	if err := discovery.Unchanged(location, effective); err != nil {
		return failure("stale", err)
	}
	result = analyze(ctx, location, cfg, selection, onlyDebug, process)
	after, err := runrequest.Snapshot(location, selection)
	if err != nil {
		return failure("stale", err)
	}
	refreshed, err := scope.Build(ctx, options)
	if err != nil {
		return failure("stale", err)
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(selection, refreshed) {
		return failure("stale", fmt.Errorf("project inputs changed during the check; run it again"))
	}
	if err := discovery.Unchanged(location, effective); err != nil {
		return failure("stale", err)
	}
	return result
}

func newReport(cfg config.Config, selection scope.Selection) report.Report {
	result := report.New()
	result.Profile, result.Scope = &cfg.Profile, &selection
	result.Environment = &report.Environment{RequestedRuntime: cfg.PHP.Runtime, TargetMin: cfg.PHP.Target.Min, TargetMax: cfg.PHP.Target.Max, Protocol: 1}
	if cfg.Framework.Version != "auto" && cfg.Framework.Version != "" {
		result.Environment.FrameworkVersion = &cfg.Framework.Version
	}
	return result
}

func analyze(ctx context.Context, location config.Location, cfg config.Config, selection scope.Selection, onlyDebug bool, process runner.Process) report.Report {
	result := newReport(cfg, selection)
	defaults, ok := profile.Lookup(cfg.Profile)
	if !ok {
		return report.InputError("profile", fmt.Errorf("unsupported profile %q", cfg.Profile))
	}
	selected := selection.Filter(cfg.Tools.Debug.Include, cfg.Tools.Debug.Exclude, func(filename string) bool {
		return slices.Contains(defaults.PHPExtensions, strings.ToLower(filepath.Ext(filename)))
	})
	if onlyDebug {
		cfg.Tools.Debug.Enabled = true
	}
	outcome := debug.Run(ctx, location.Root, selected, cfg.Tools.Debug, process)
	result.Checks = append(result.Checks, outcome.Check)
	result.Findings = append(result.Findings, outcome.Findings...)
	result.Diagnostics = append(result.Diagnostics, outcome.Diagnostics...)
	result.Environment.PHPRuntime = outcome.Runtime
	if outcome.Runtime != nil && *outcome.Runtime != cfg.PHP.Runtime {
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "runtime_mismatch", Stage: "worker", Message: fmt.Sprintf("worker PHP runtime %s does not match configured %s", *outcome.Runtime, cfg.PHP.Runtime)})
	}
	result.Finalize()
	return result
}
