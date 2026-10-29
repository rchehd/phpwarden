package worker

import (
	"context"
	"path/filepath"
	"slices"
	"time"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/profile/drupal"
	"github.com/rchehd/phpwarden/internal/psalmanalysis"
	"github.com/rchehd/phpwarden/internal/runner"
)

type PsalmOptions struct {
	Bundle   string
	PHP      string
	MemoryMB int
}

func AnalyzePsalm(ctx context.Context, location config.Location, effective config.Effective, options PsalmOptions, process runner.Process) psalmanalysis.Result {
	fail := func(message string) psalmanalysis.Result {
		return psalmanalysis.Result{Diagnostics: []psalmanalysis.Diagnostic{{Stage: "discovery", Code: "discovery_incomplete", Message: message}}}
	}
	if effective.Config.Profile != "drupal" {
		return fail("Psalm profile preparation is unsupported for this framework")
	}
	current, err := config.Load(location)
	if err != nil {
		return fail(err.Error())
	}
	if current.SourceHash != effective.SourceHash {
		return psalmanalysis.Result{Diagnostics: []psalmanalysis.Diagnostic{{Stage: "freshness", Code: "stale", Message: "Project configuration changed after loading"}}}
	}
	if err := discovery.Unchanged(location, effective); err != nil {
		return fail(err.Error())
	}
	request := psalmanalysis.Request{
		Root: location.Root, ComposerRoot: effective.Config.ComposerRoot, Bundle: options.Bundle,
		PHP: options.PHP, Runtime: effective.Config.PHP.Runtime, Pins: drupal.PsalmPins(), MemoryMB: options.MemoryMB, Timeout: time.Duration(effective.Config.Runner.TimeoutSeconds) * time.Second,
		Prepare: func(ctx context.Context, directory string, process runner.Process) (psalmanalysis.Prepared, error) {
			return drupal.PreparePsalm(ctx, location.Root, options.Bundle, directory, options.PHP, options.MemoryMB, effective.Config, effective.Project, process)
		},
	}
	result := psalmanalysis.Run(ctx, request, process)
	if name, err := filepath.Rel(location.Root, location.Path); err == nil && location.Path != "" {
		result.ContextFiles = append(result.ContextFiles, filepath.ToSlash(name))
		slices.Sort(result.ContextFiles)
		result.ContextFiles = slices.Compact(result.ContextFiles)
	}
	current, err = config.Load(location)
	if err != nil || current.SourceHash != effective.SourceHash {
		result.Complete, result.Findings = false, nil
		result.Diagnostics = append(result.Diagnostics, psalmanalysis.Diagnostic{Stage: "freshness", Code: "stale", Message: "Project configuration changed during analysis"})
	}
	if err := discovery.Unchanged(location, effective); err != nil {
		result.Complete, result.Findings = false, nil
		result.Diagnostics = append(result.Diagnostics, psalmanalysis.Diagnostic{Stage: "freshness", Code: "stale", Message: err.Error()})
	}
	return result
}
