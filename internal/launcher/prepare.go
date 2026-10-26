package launcher

import (
	"context"
	"fmt"
	"time"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/progress"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/toolset"
)

type Preparation struct {
	Version     int                 `json:"version"`
	Command     string              `json:"command"`
	Ready       bool                `json:"ready"`
	ExitCode    int                 `json:"exit_code"`
	Selection   *toolset.Selection  `json:"selection,omitempty"`
	ImageID     string              `json:"image_id,omitempty"`
	Diagnostics []report.Diagnostic `json:"diagnostics"`
}

func Prepare(ctx context.Context, location config.Location, effective config.Effective, process runner.Process) Preparation {
	if err := ctx.Err(); err != nil {
		return preparationFailure(ctx, Preparation{}, "prepare_cancelled", err)
	}
	progress.Stage(ctx, "Selecting compatible toolset")
	selection, err := toolset.Resolve(effective)
	if err != nil {
		result := preparationFailure(ctx, Preparation{}, "compatibility_invalid", err)
		result.ExitCode = 2
		return result
	}
	return prepareSelected(ctx, location, effective, selection, process)
}

func prepareSelected(ctx context.Context, location config.Location, effective config.Effective, selection toolset.Selection, process runner.Process) Preparation {
	result := Preparation{Version: 1, Command: "prepare", Selection: &selection, Diagnostics: []report.Diagnostic{}}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(effective.Config.Runner.TimeoutSeconds)*time.Second)
	defer cancel()
	failure := func(code string, err error) Preparation { return preparationFailure(ctx, result, code, err) }
	unchanged := func() error {
		loaded, err := config.Load(location)
		if err != nil {
			return err
		}
		if loaded.SourceHash != effective.SourceHash {
			return fmt.Errorf("project configuration changed during image preparation; run prepare again")
		}
		return discovery.Unchanged(location, effective)
	}
	if err := ctx.Err(); err != nil {
		return failure("prepare_cancelled", err)
	}
	if err := unchanged(); err != nil {
		return failure("stale", err)
	}
	progress.Stage(ctx, "Checking Docker and worker image")
	if err := localEndpoint(ctx, process); err != nil {
		return failure("docker_unavailable", err)
	}
	image, err := ensureImage(ctx, selection, "missing", process)
	if err != nil {
		return failure(imageErrorCode(selection), err)
	}
	if err := ctx.Err(); err != nil {
		return failure("prepare_cancelled", err)
	}
	progress.Stage(ctx, "Verifying project settings are unchanged")
	if err := unchanged(); err != nil {
		return failure("stale", err)
	}
	if err := ctx.Err(); err != nil {
		return failure("prepare_cancelled", err)
	}
	result.Ready, result.ImageID = true, image
	return result
}

func preparationFailure(ctx context.Context, result Preparation, code string, err error) Preparation {
	result.Version, result.Command, result.Ready, result.ExitCode = 1, "prepare", false, 3
	if ctx.Err() == context.Canceled {
		result.ExitCode, code = 130, "prepare_cancelled"
	} else if ctx.Err() == context.DeadlineExceeded {
		code = "prepare_timeout"
	}
	result.Diagnostics = []report.Diagnostic{{Code: code, Stage: "prepare", Message: err.Error()}}
	return result
}
