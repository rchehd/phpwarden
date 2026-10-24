package worker

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/cache"
	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/checker/phpcs"
	"github.com/rchehd/phpwarden/internal/checker/twigcsfixer"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/edit"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/runrequest"
	"github.com/rchehd/phpwarden/internal/scope"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func PrepareFix(ctx context.Context, root string, request runrequest.Request, process runner.Process) runrequest.FixResponse {
	response := runrequest.FixResponse{Report: report.New(), Edits: []edit.Edit{}}
	failure := func(code string, err error) runrequest.FixResponse {
		response.Edits = nil
		response.Report.Diagnostics = append(response.Report.Diagnostics, report.Diagnostic{Code: code, Stage: "fix", Message: err.Error()})
		response.Report.Finalize()
		return response
	}
	ids, err := runrequest.FixIDs(request.Config, request.Only)
	if err != nil || !reflect.DeepEqual(ids, request.Only) {
		return failure("fix_invalid", fmt.Errorf("fix request must explicitly select supported formatters"))
	}
	response.Report = Execute(ctx, root, request, process)
	if response.Report.ExitCode > 1 || !response.Report.Complete {
		response.Edits = nil
		return response
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(request.Config.Runner.TimeoutSeconds)*time.Second)
	defer cancel()
	cfg := request.Config
	for _, id := range ids {
		var outcome checker.Outcome
		switch id {
		case "phpcs":
			settings := cfg.Tools.PHPCS
			settings.Enabled = true
			outcome = phpcs.Fix(ctx, root, cfg.ComposerRoot, phpcs.Select(request.Selection, settings, cfg.Profile), settings, cfg.PHP.Target, process)
		case "twig-cs-fixer":
			settings := cfg.Tools.TwigCSFixer
			settings.Enabled = true
			outcome = twigcsfixer.Fix(ctx, root, cfg.ComposerRoot, twigcsfixer.Select(request.Selection, settings, cfg.Profile), settings, cfg.Profile, process)
		}
		if outcome.Check.Status == "error" || outcome.Check.Status == "unavailable" {
			response.Report.Diagnostics = append(response.Report.Diagnostics, outcome.Diagnostics...)
			return failure("fix_failed", fmt.Errorf("%s could not prepare fixes; no files were written", id))
		}
		if outcome.PHPVersion != nil && request.RuntimeVersion != "" && *outcome.PHPVersion != request.RuntimeVersion {
			return failure("runtime_version_mismatch", fmt.Errorf("fixer runtime does not match the selected image"))
		}
		response.Edits = append(response.Edits, outcome.Edits...)
	}
	location := config.Location{Root: root, Path: filepath.Join(root, filepath.FromSlash(request.ConfigFile))}
	if err := request.Matches(location); err != nil {
		return failure("stale", err)
	}
	if err := edit.Validate(response.Edits, request.FixInputs()); err != nil {
		return failure("fix_invalid", err)
	}
	return response
}

func Fix(ctx context.Context, location config.Location, effective config.Effective, options scope.Options, only []string, process runner.Process) report.Report {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(effective.Config.Runner.TimeoutSeconds)*time.Second)
	defer cancel()
	ids, err := runrequest.FixIDs(effective.Config, only)
	if err != nil {
		return report.InputError("fix", err)
	}
	release, err := cache.LockProject(ctx, location.Root)
	if err != nil {
		result := report.New()
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "fix_lock", Stage: "fix", Message: err.Error()})
		result.Finalize()
		if ctx.Err() == context.Canceled {
			result.ExitCode = 130
		}
		return result
	}
	defer release()
	selected, err := scope.Build(ctx, options)
	if err != nil {
		return report.InputError("scope", err)
	}
	request, err := runrequest.Build(location, effective, selected, ids, "sha256:"+strings.Repeat("0", 64), toolset.DevelopmentID(effective.Config.PHP.Runtime))
	if err != nil {
		return report.InputError("fix", err)
	}
	prepared := PrepareFix(ctx, location.Root, request, process)
	if prepared.Edits == nil || prepared.Report.ExitCode > 1 || !prepared.Report.Complete {
		return prepared.Report
	}
	failure := func(code string, err error, changed []string) report.Report {
		result := prepared.Report
		result.FixedFiles = changed
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "fix", Message: err.Error()})
		result.Finalize()
		if ctx.Err() == context.Canceled {
			result.ExitCode = 130
		}
		return result
	}
	refreshed, err := scope.Build(ctx, options)
	if err != nil || !reflect.DeepEqual(selected, refreshed) {
		return failure("stale", fmt.Errorf("Git scope changed while preparing fixes"), nil)
	}
	if err := request.Matches(location); err != nil {
		return failure("stale", err, nil)
	}
	if err := discovery.Unchanged(location, effective); err != nil {
		return failure("stale", err, nil)
	}
	if ctx.Err() != nil {
		return failure("fix_cancelled", ctx.Err(), nil)
	}
	changed, err := edit.Apply(location.Root, prepared.Edits, request.FixInputs())
	if err != nil {
		return failure("fix_write_failed", err, changed)
	}
	if _, err := scope.Build(ctx, options); err != nil {
		return failure("scope_invalid", err, changed)
	}
	result := Check(ctx, location, effective, runrequest.Verification(options, selected), only, process)
	result.FixedFiles = changed
	return result
}
