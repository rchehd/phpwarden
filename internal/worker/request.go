package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/checker/composeraudit"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/runrequest"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func Execute(ctx context.Context, root string, request runrequest.Request, process runner.Process) report.Report {
	failure := func(code string, err error) report.Report {
		result := report.New()
		if request.RunID != "" {
			result.RunID = request.RunID
		}
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "worker", Message: err.Error()})
		result.Finalize()
		return result
	}
	if err := request.Validate(); err != nil {
		return failure("request_invalid", err)
	}
	if request.Toolset != toolset.DevelopmentID(request.Config.PHP.Runtime) {
		return failure("toolset_mismatch", fmt.Errorf("worker does not support requested toolset %q", request.Toolset))
	}
	location := config.Location{Root: root, Path: filepath.Join(root, request.ConfigFile)}
	if err := request.Matches(location); err != nil {
		return failure("stale", err)
	}
	effective, err := config.Load(location)
	if err != nil {
		return failure("config_invalid", err)
	}
	if request.Project != nil {
		effective, err = discovery.Apply(location, effective)
		if err != nil {
			return failure("stale", fmt.Errorf("project metadata changed or is no longer valid: %w", err))
		}
		if !reflect.DeepEqual(request.Project, effective.Project) {
			return failure("stale", fmt.Errorf("project discovery does not match the host request"))
		}
	}
	loadedJSON, _ := json.Marshal(effective.Config)
	requestedJSON, _ := json.Marshal(request.Config)
	if !bytes.Equal(loadedJSON, requestedJSON) || effective.SourceHash != request.Fingerprint["config"] {
		return failure("stale", fmt.Errorf("mounted configuration does not match the host request"))
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(effective.Config.Runner.TimeoutSeconds)*time.Second)
	defer cancel()
	delegate := func(_ context.Context, settings config.ComposerAudit) checker.Outcome {
		return composeraudit.Delegated(location.Root, settings)
	}
	result := analyze(ctx, location, effective.Config, effective.Project, request.Selection, request.Only, delegate, process)
	validateRuntime(&result, effective)
	result.RunID = request.RunID
	result.Environment.Image, result.Environment.Toolset = &request.Image, &request.Toolset
	if result.Environment.PHPRuntime != nil && request.RuntimeVersion != "" &&
		(result.Environment.PHPVersion == nil || *result.Environment.PHPVersion != request.RuntimeVersion) {
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "runtime_version_mismatch", Stage: "worker", Message: "actual PHP version does not match the selected image version"})
		result.Finalize()
	}
	if err := request.Matches(location); err != nil {
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "stale", Stage: "worker", Message: err.Error()})
		result.Finalize()
	}
	if err := discovery.Unchanged(location, effective); err != nil {
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "stale", Stage: "worker", Message: err.Error()})
		result.Finalize()
	}
	return result
}
