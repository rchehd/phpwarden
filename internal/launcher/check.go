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
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/checker/composeraudit"
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

func Check(ctx context.Context, location config.Location, effective config.Effective, options scope.Options, only []string, process runner.Process) report.Report {
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
	selected, err := toolset.Resolve(effective)
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
	audits := composeraudit.Delegated(location.Root, auditSettings(effective.Config, only))
	if len(selection.Files) == 0 && len(selection.ProjectFiles) == 0 && *audits.Check.Reason != "delegated" || !checker.AnyEnabled(effective.Config, only) {
		return worker.Check(ctx, location, effective, options, only, process)
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
	request, err := runrequest.Build(location, effective, selection, only, image, selected.ID)
	if err != nil {
		return failure("stale", err)
	}
	request.RuntimeVersion = selected.PHPVersion
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
		"--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=256m,mode=1777",
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
	for i, check := range result.Checks {
		if check.ID == "composer-audit" && check.Reason != nil && *check.Reason == "delegated" {
			cacheDir, _ := composeraudit.CacheDir()
			outcome := composeraudit.Run(ctx, location.Root, auditSettings(request.Config, only), dockerFetcher(image, process), cacheDir, time.Now())
			result.Checks[i] = outcome.Check
			result.Findings = append(result.Findings, outcome.Findings...)
			result.Diagnostics = append(result.Diagnostics, outcome.Diagnostics...)
			result.Environment.Advisories = outcome.Advisories
			result.Finalize()
		}
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

func auditSettings(cfg config.Config, only []string) config.ComposerAudit {
	settings := cfg.Tools.ComposerAudit
	settings.Enabled = checker.Enabled("composer-audit", settings.Enabled, only)
	return settings
}

// dockerFetcher runs Composer audit in a separate container that has network access but sees only copies of the Composer files.
func dockerFetcher(image string, process runner.Process) composeraudit.Fetcher {
	return func(ctx context.Context, composerJSON, composerLock []byte, noDev bool) ([]byte, error) {
		directory, err := os.MkdirTemp("", "phpwarden-audit-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(directory)
		for name, data := range map[string][]byte{"composer.json": composerJSON, "composer.lock": composerLock} {
			if err := os.WriteFile(filepath.Join(directory, name), data, 0o600); err != nil {
				return nil, err
			}
		}
		name := "phpwarden-audit-" + strings.ToLower(rand.Text())
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			process.Run(cleanup, runner.Command{Name: "docker", Args: []string{"rm", "--force", name}})
		}()
		var mount bytes.Buffer
		writer := csv.NewWriter(&mount)
		if err := writer.Write([]string{"type=bind", "source=" + directory, "target=/audit", "readonly"}); err != nil {
			return nil, err
		}
		writer.Flush()
		args := []string{"run", "--rm", "--pull=never", "--name", name,
			"--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges:true",
			"--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=256m,mode=1777",
			"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
			"--mount", strings.TrimSuffix(mount.String(), "\n"), "--workdir", "/audit",
			"--entrypoint", "/usr/local/bin/phpwarden", image, "worker", "audit"}
		if noDev {
			args = append(args, "--no-dev")
		}
		execution := process.Run(ctx, runner.Command{Name: "docker", Args: args})
		if execution.Unavailable || execution.TimedOut || ctx.Err() != nil {
			return nil, fmt.Errorf("Composer audit container did not complete")
		}
		if execution.Err != nil || execution.ExitCode != 0 {
			return nil, fmt.Errorf("Composer audit container exited %d: %s", execution.ExitCode, strings.TrimSpace(string(execution.Stderr)))
		}
		return bytes.TrimSpace(execution.Stdout), nil
	}
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
		if !selection.Development {
			return "", fmt.Errorf("selected release image %s is unavailable locally; registry pull is not implemented yet: %s %v", selection.Image, strings.TrimSpace(string(result.Stderr)), result.Err)
		}
		return "", fmt.Errorf("development image %s is unavailable; build it with docker build -f docker/Dockerfile -t %s .: %s %v", selection.Image, selection.Image, strings.TrimSpace(string(result.Stderr)), result.Err)
	}
	var images []struct {
		ID           string   `json:"Id"`
		OS           string   `json:"Os"`
		Architecture string   `json:"Architecture"`
		RepoDigests  []string `json:"RepoDigests"`
		Config       struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(result.Stdout, &images); err != nil || len(images) != 1 {
		return "", fmt.Errorf("expected exactly one inspected Docker image")
	}
	image := images[0]
	actualCapabilities := strings.Split(image.Config.Labels["io.phpwarden.checkers"], ",")
	expectedCapabilities := slices.Clone(selection.Capabilities)
	slices.Sort(actualCapabilities)
	slices.Sort(expectedCapabilities)
	if image.OS+"/"+image.Architecture != selection.Platform || image.Config.Labels["io.phpwarden.protocol"] != strconv.Itoa(selection.Protocol) ||
		image.Config.Labels["io.phpwarden.toolset"] != selection.ID || !slices.Equal(actualCapabilities, expectedCapabilities) {
		return "", fmt.Errorf("local image platform, protocol or capabilities do not match the selected toolset")
	}
	if image.Config.Labels["io.phpwarden.php-version"] != selection.PHPVersion {
		return "", fmt.Errorf("local image PHP version label does not match the selected toolset")
	}
	if version, ok := selection.Tools["symfony-yaml"]; ok && image.Config.Labels["io.phpwarden.symfony-yaml-version"] != version {
		return "", fmt.Errorf("local image Symfony YAML version label does not match the selected toolset")
	}
	for tool, label := range map[string]string{"squizlabs-php_codesniffer": "phpcs", "drupal-coder": "coder", "phpcompatibility-php-compatibility": "phpcompatibility", "phpstan-phpstan": "phpstan", "mglaman-phpstan-drupal": "phpstan-drupal", "phpstan-phpstan-deprecation-rules": "phpstan-deprecations", "vincentlanglet-twig-cs-fixer": "twig-cs-fixer", "twig-twig": "twig", "composer-composer": "composer"} {
		if version, ok := selection.Tools[tool]; ok && image.Config.Labels["io.phpwarden."+label+"-version"] != version {
			return "", fmt.Errorf("local image %s version label does not match the selected toolset", label)
		}
	}
	if !selection.Development && !slices.Contains(image.RepoDigests, selection.Image) {
		return "", fmt.Errorf("local image repository digest does not match the selected manifest image")
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
			result.Environment.Protocol != request.Version || len(result.Checks) != len(checker.Requested(request.Only)) {
			return result, fmt.Errorf("worker report does not match the requested inputs and toolset")
		}
		seen := make(map[string]bool)
		for _, check := range result.Checks {
			required, failOn := request.Config.Tools.Debug.Required, request.Config.Tools.Debug.FailOn
			if check.ID == "syntax" {
				required, failOn = request.Config.Tools.Syntax.Required, request.Config.Tools.Syntax.FailOn
			} else if check.ID == "yaml" {
				required, failOn = request.Config.Tools.YAML.Required, request.Config.Tools.YAML.FailOn
			} else if check.ID == "whitespace" {
				required, failOn = request.Config.Tools.Whitespace.Required, request.Config.Tools.Whitespace.FailOn
			} else if check.ID == "drupal-config" {
				required, failOn = request.Config.Tools.DrupalConfig.Required, request.Config.Tools.DrupalConfig.FailOn
			} else if check.ID == "phpcs" {
				required, failOn = request.Config.Tools.PHPCS.Required, request.Config.Tools.PHPCS.FailOn
			} else if check.ID == "phpstan" {
				required, failOn = request.Config.Tools.PHPStan.Required, request.Config.Tools.PHPStan.FailOn
			} else if check.ID == "twig-cs-fixer" {
				required, failOn = request.Config.Tools.TwigCSFixer.Required, request.Config.Tools.TwigCSFixer.FailOn
			} else if check.ID == "composer-validate" {
				required, failOn = request.Config.Tools.ComposerValidate.Required, request.Config.Tools.ComposerValidate.FailOn
			} else if check.ID == "composer-audit" {
				required, failOn = request.Config.Tools.ComposerAudit.Required, request.Config.Tools.ComposerAudit.FailOn
			}
			if !slices.Contains(checker.Requested(request.Only), check.ID) || seen[check.ID] || check.Required != required || check.FailOn != failOn {
				return result, fmt.Errorf("worker checker selection or policy does not match the request")
			}
			seen[check.ID] = true
		}
		if request.Config.Framework.Version != "auto" && request.Config.Framework.Version != "" &&
			(result.Environment.FrameworkVersion == nil || *result.Environment.FrameworkVersion != request.Config.Framework.Version) {
			return result, fmt.Errorf("worker report does not match the resolved framework version")
		}
		if result.Environment.PHPRuntime != nil && request.RuntimeVersion != "" &&
			(result.Environment.PHPVersion == nil || *result.Environment.PHPVersion != request.RuntimeVersion) {
			return result, fmt.Errorf("worker PHP version does not match the selected image")
		}
	}
	for _, finding := range result.Findings {
		if !slices.Contains(checker.Requested(request.Only), finding.Checker) {
			return result, fmt.Errorf("worker finding is outside the requested scope")
		}
		if (finding.Checker == "yaml" || finding.Checker == "twig-cs-fixer" || finding.Checker == "composer-validate") && finding.Kind == "global" {
			prefix := map[string]string{"yaml": "YAML file %q: ", "twig-cs-fixer": "Twig template %q: ", "composer-validate": "Composer file %q: "}[finding.Checker]
			filenames := request.Selection.Files
			if finding.Checker == "composer-validate" && len(request.Selection.ProjectFiles) > 0 {
				filenames = append(slices.Clone(filenames), "composer.json")
			}
			matched := false
			for _, filename := range filenames {
				if strings.HasPrefix(finding.Message, fmt.Sprintf(prefix, filename)) {
					matched = true
					break
				}
			}
			if !matched {
				return result, fmt.Errorf("global %s finding does not match a selected file", finding.Checker)
			}
			continue
		}
		if finding.Path == nil || finding.Line == nil {
			return result, fmt.Errorf("worker finding is outside the requested scope")
		}
		if _, exists := request.Selection.LineMap[*finding.Path]; !exists ||
			(finding.Checker == "debug" || finding.Checker == "whitespace" || finding.Checker == "phpcs" || finding.Checker == "phpstan" || finding.Checker == "twig-cs-fixer") && (finding.Kind != "code" || !request.Selection.Contains(*finding.Path, *finding.Line)) ||
			finding.Checker == "drupal-config" && finding.Kind != "code" ||
			(finding.Checker == "syntax" || finding.Checker == "yaml") && finding.Kind != "syntax" {
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
