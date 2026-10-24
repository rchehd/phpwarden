package launcher

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/cache"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/edit"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/runrequest"
	"github.com/rchehd/phpwarden/internal/scope"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func Fix(ctx context.Context, location config.Location, effective config.Effective, options scope.Options, only []string, process runner.Process) report.Report {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(effective.Config.Runner.TimeoutSeconds)*time.Second)
	defer cancel()
	changed := []string{}
	failure := func(code string, err error) report.Report {
		result := report.New()
		result.FixedFiles = changed
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "fix", Message: err.Error()})
		result.Finalize()
		if ctx.Err() == context.Canceled {
			result.ExitCode = 130
		}
		return result
	}
	ids, err := runrequest.FixIDs(effective.Config, only)
	if err != nil {
		return report.InputError("fix", err)
	}
	release, err := cache.LockProject(ctx, location.Root)
	if err != nil {
		return failure("fix_lock", err)
	}
	defer release()
	selected, err := scope.Build(ctx, options)
	if err != nil {
		return report.InputError("scope", err)
	}
	if len(selected.Files) == 0 {
		return CheckFresh(ctx, location, effective, options, only, process)
	}
	tool, err := toolset.Resolve(effective)
	if err != nil {
		return report.InputError("compatibility", err)
	}
	if err := discovery.Unchanged(location, effective); err != nil {
		return failure("stale", err)
	}
	if err := localEndpoint(ctx, process); err != nil {
		return failure("docker_unavailable", err)
	}
	image, err := inspect(ctx, tool, process)
	if err != nil {
		return failure("image_unavailable", err)
	}
	request, err := runrequest.Build(location, effective, selected, ids, image, tool.ID)
	if err != nil {
		return failure("stale", err)
	}
	request.RuntimeVersion = tool.PHPVersion
	input, err := json.Marshal(request)
	if err != nil {
		return failure("request_invalid", err)
	}
	execution := fixContainer(ctx, location.Root, image, input, process)
	if ctx.Err() != nil {
		return failure("fix_cancelled", ctx.Err())
	}
	if execution.Unavailable || execution.TimedOut || len(execution.Stdout) > 2*edit.MaxBytes || len(execution.Stderr) > 0 || execution.ExitCode < 0 {
		return failure("fix_report_invalid", fmt.Errorf("fix preparation returned an invalid or oversized response"))
	}
	var prepared runrequest.FixResponse
	decoder := json.NewDecoder(bytes.NewReader(execution.Stdout))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&prepared); err != nil {
		return failure("fix_report_invalid", fmt.Errorf("fix preparation did not return a valid JSON response"))
	}
	if _, err := decoder.Token(); err != io.EOF {
		return failure("fix_report_invalid", fmt.Errorf("fix preparation must return one JSON response"))
	}
	reportJSON, err := json.Marshal(prepared.Report)
	if err != nil {
		return failure("fix_report_invalid", err)
	}
	parsed, err := decodeReport(runner.Result{Stdout: reportJSON, ExitCode: execution.ExitCode}, request)
	if err != nil {
		return failure("fix_report_invalid", err)
	}
	if parsed.ExitCode > 1 || !parsed.Complete {
		return parsed
	}
	if prepared.Edits == nil {
		return failure("fix_report_invalid", fmt.Errorf("fix preparation omitted edits"))
	}
	refreshed, err := scope.Build(ctx, options)
	if err != nil || !reflect.DeepEqual(selected, refreshed) {
		return failure("stale", fmt.Errorf("Git scope changed while preparing fixes"))
	}
	if err := request.Matches(location); err != nil {
		return failure("stale", err)
	}
	if err := discovery.Unchanged(location, effective); err != nil {
		return failure("stale", err)
	}
	if ctx.Err() != nil {
		return failure("fix_cancelled", ctx.Err())
	}
	changed, err = edit.Apply(location.Root, prepared.Edits, request.FixInputs())
	if err != nil {
		return failure("fix_write_failed", err)
	}
	if _, err := scope.Build(ctx, options); err != nil {
		return failure("scope_invalid", err)
	}
	result := CheckFresh(ctx, location, effective, runrequest.Verification(options, selected), only, process)
	result.FixedFiles = changed
	return result
}

func fixContainer(ctx context.Context, root, image string, input []byte, process runner.Process) runner.Result {
	name := "phpwarden-fix-" + strings.ToLower(rand.Text())
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		process.Run(cleanup, runner.Command{Name: "docker", Args: []string{"rm", "--force", name}})
	}()
	var mount bytes.Buffer
	writer := csv.NewWriter(&mount)
	if err := writer.Write([]string{"type=bind", "source=" + root, "target=/workspace", "readonly"}); err != nil {
		return runner.Result{ExitCode: -1, Err: err}
	}
	writer.Flush()
	args := []string{"run", "--rm", "--interactive", "--pull=never", "--name", name,
		"--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges:true",
		"--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=256m,mode=1777", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--mount", strings.TrimSuffix(mount.String(), "\n"), "--entrypoint", "/usr/local/bin/phpwarden", image, "worker", "prepare-fix"}
	return process.Run(ctx, runner.Command{Name: "docker", Args: args, Input: input})
}
