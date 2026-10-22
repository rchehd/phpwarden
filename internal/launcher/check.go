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

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/runrequest"
	"github.com/rchehd/phpwarden/internal/scope"
	"github.com/rchehd/phpwarden/internal/toolset"
	"github.com/rchehd/phpwarden/internal/worker"
	"github.com/rchehd/phpwarden/schemas"
)

func Check(ctx context.Context, location config.Location, effective config.Effective, options scope.Options, onlyDebug bool, process runner.Process) report.Report {
	ctx, cancel := context.WithTimeout(ctx, time.Duration(effective.Config.Runner.TimeoutSeconds)*time.Second)
	defer cancel()
	failure := func(code string, err error) report.Report {
		result := report.New()
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "launcher", Message: err.Error()})
		result.Finalize()
		if ctx.Err() == context.Canceled {
			result.ExitCode = 130
		}
		return result
	}
	selected, err := toolset.Resolve(effective.Config)
	if err != nil {
		return report.InputError("compatibility", err)
	}
	selection, err := scope.Build(ctx, options)
	if err != nil {
		if ctx.Err() != nil {
			return failure("launcher_cancelled", ctx.Err())
		}
		return report.InputError("scope", err)
	}
	if len(selection.Files) == 0 || !effective.Config.Tools.Debug.Enabled && !onlyDebug {
		return worker.Check(ctx, location, effective, options, onlyDebug, process)
	}
	if err := discovery.Unchanged(location, effective); err != nil {
		return failure("stale", err)
	}
	if err := localEndpoint(ctx, process); err != nil {
		return failure("docker_unavailable", err)
	}
	image, err := inspect(ctx, selected, process)
	if err != nil {
		return failure("image_unavailable", err)
	}
	request, err := runrequest.Build(location, effective, selection, onlyDebug, image, selected.ID)
	if err != nil {
		return failure("stale", err)
	}
	input, err := json.Marshal(request)
	if err != nil {
		return failure("request_invalid", err)
	}
	name := "phpwarden-run-" + strings.ToLower(rand.Text())
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		process.Run(cleanup, runner.Command{Name: "docker", Args: []string{"rm", "--force", name}})
	}()
	var mount bytes.Buffer
	writer := csv.NewWriter(&mount)
	if err := writer.Write([]string{"type=bind", "source=" + location.Root, "target=/workspace", "readonly"}); err != nil {
		return failure("mount_invalid", err)
	}
	writer.Flush()
	args := []string{"run", "--rm", "--interactive", "--pull=never", "--name", name,
		"--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges:true",
		"--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=64m,mode=1777",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--mount", strings.TrimSuffix(mount.String(), "\n"),
		"--entrypoint", "/usr/local/bin/phpwarden", image, "worker", "execute"}
	execution := process.Run(ctx, runner.Command{Name: "docker", Args: args, Input: input})
	if ctx.Err() != nil {
		return failure("launcher_cancelled", ctx.Err())
	}
	result, err := decodeReport(execution, request)
	if err != nil {
		return failure("worker_report_invalid", err)
	}
	refreshed, err := scope.Build(ctx, options)
	if err == nil && !reflect.DeepEqual(selection, refreshed) {
		err = fmt.Errorf("Git scope changed during the check")
	}
	if err == nil {
		err = request.Matches(location)
	}
	if err == nil {
		err = discovery.Unchanged(location, effective)
	}
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "stale", Stage: "launcher", Message: err.Error()})
		result.Finalize()
	}
	if ctx.Err() != nil {
		return failure("launcher_cancelled", ctx.Err())
	}
	return result
}

func localEndpoint(ctx context.Context, process runner.Process) error {
	endpoint := os.Getenv("DOCKER_HOST")
	if endpoint == "" || os.Getenv("DOCKER_CONTEXT") != "" {
		result := process.Run(ctx, runner.Command{Name: "docker", Args: []string{"context", "inspect", "--format", "{{json .Endpoints.docker.Host}}"}})
		if result.Err != nil || result.ExitCode != 0 {
			return fmt.Errorf("Docker context is unavailable: %s %v", strings.TrimSpace(string(result.Stderr)), result.Err)
		}
		if err := json.Unmarshal(result.Stdout, &endpoint); err != nil {
			return fmt.Errorf("invalid Docker context response: %w", err)
		}
	}
	if !strings.HasPrefix(endpoint, "unix:///") {
		return fmt.Errorf("check currently requires a local Docker Unix socket; remote contexts cannot access local bind mounts")
	}
	return nil
}

func inspect(ctx context.Context, selection toolset.Selection, process runner.Process) (string, error) {
	result := process.Run(ctx, runner.Command{Name: "docker", Args: []string{"image", "inspect", selection.Image}})
	if result.Err != nil || result.ExitCode != 0 {
		return "", fmt.Errorf("development image %s is unavailable; build it with docker build -f docker/Dockerfile -t %s .: %s %v", selection.Image, selection.Image, strings.TrimSpace(string(result.Stderr)), result.Err)
	}
	var images []struct {
		ID           string `json:"Id"`
		OS           string `json:"Os"`
		Architecture string `json:"Architecture"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(result.Stdout, &images); err != nil || len(images) != 1 {
		return "", fmt.Errorf("expected exactly one inspected Docker image")
	}
	image := images[0]
	if image.OS+"/"+image.Architecture != selection.Platform || image.Config.Labels["io.phpwarden.protocol"] != "1" ||
		image.Config.Labels["io.phpwarden.toolset"] != selection.ID || image.Config.Labels["io.phpwarden.checkers"] != "debug" {
		return "", fmt.Errorf("local image platform, protocol or capabilities do not match the selected toolset")
	}
	return image.ID, nil
}

func decodeReport(execution runner.Result, request runrequest.Request) (report.Report, error) {
	var result report.Report
	if execution.Unavailable || execution.TimedOut || execution.ExitCode < 0 {
		return result, fmt.Errorf("Docker worker did not complete: %v", execution.Err)
	}
	decoder := json.NewDecoder(bytes.NewReader(execution.Stdout))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return result, fmt.Errorf("worker did not return a valid report (Docker exit %d): %w; %s", execution.ExitCode, err, strings.TrimSpace(string(execution.Stderr)))
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return result, fmt.Errorf("expected one worker report")
	}
	var document any
	if err := json.Unmarshal(execution.Stdout, &document); err != nil {
		return result, err
	}
	if err := schemas.ValidateReport(document); err != nil {
		return result, fmt.Errorf("worker report schema: %w", err)
	}
	if result.RunID != request.RunID || result.ExitCode != execution.ExitCode {
		return result, fmt.Errorf("worker identity or exit code does not match the container result")
	}
	if result.ExitCode != 3 && result.ExitCode != 130 {
		if result.Profile == nil || *result.Profile != request.Config.Profile || !reflect.DeepEqual(result.Scope, &request.Selection) ||
			result.Environment == nil || result.Environment.Image == nil || *result.Environment.Image != request.Image ||
			result.Environment.Toolset == nil || *result.Environment.Toolset != request.Toolset ||
			result.Environment.RequestedRuntime != request.Config.PHP.Runtime || result.Environment.PHPRuntime != nil && *result.Environment.PHPRuntime != request.Config.PHP.Runtime ||
			result.Environment.TargetMin != request.Config.PHP.Target.Min || result.Environment.TargetMax != request.Config.PHP.Target.Max ||
			len(result.Checks) != 1 || result.Checks[0].ID != "debug" {
			return result, fmt.Errorf("worker report does not match the requested inputs and toolset")
		}
		if request.Config.Framework.Version != "auto" && request.Config.Framework.Version != "" &&
			(result.Environment.FrameworkVersion == nil || *result.Environment.FrameworkVersion != request.Config.Framework.Version) {
			return result, fmt.Errorf("worker report does not match the resolved framework version")
		}
	}
	for _, finding := range result.Findings {
		if finding.Path == nil || finding.Line == nil || finding.Checker != "debug" || !request.Selection.Contains(*finding.Path, *finding.Line) {
			return result, fmt.Errorf("worker finding is outside the requested scope")
		}
	}
	finalized := result
	finalized.Finalize()
	if result.ExitCode != 130 && (finalized.ExitCode != result.ExitCode || finalized.Complete != result.Complete || finalized.Verdict != result.Verdict) {
		return result, fmt.Errorf("worker verdict conflicts with its checker statuses")
	}
	return result, nil
}
