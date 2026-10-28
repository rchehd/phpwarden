package launcher

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/progress"
	"github.com/rchehd/phpwarden/internal/projecttest"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func Test(ctx context.Context, location config.Location, effective config.Effective, options projecttest.Options, process runner.Process) projecttest.Report {
	plan, err := projecttest.Build(ctx, location.Root, effective, options)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return projecttest.Invalid(ctx.Err().Error(), 130)
		}
		if ctx.Err() != nil {
			return projecttest.Invalid(ctx.Err().Error(), 3)
		}
		if errors.Is(err, projecttest.ErrUnavailable) {
			return projecttest.Invalid(err.Error(), 3)
		}
		return projecttest.Invalid(err.Error(), 2)
	}
	selection, err := toolset.Resolve(effective)
	if err != nil {
		return projecttest.Invalid(err.Error(), 2)
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(plan.TimeoutSeconds)*time.Second)
	defer cancel()
	failed := func(err error) projecttest.Report {
		code := 3
		if ctx.Err() == context.Canceled {
			code = 130
		}
		return projecttest.Invalid(err.Error(), code)
	}
	progress.Stage(ctx, "Selecting project test runtime")
	if err := localEndpoint(ctx, process); err != nil {
		return failed(err)
	}
	image, err := ensureImage(ctx, selection, effective.Config.Runner.ImagePull, process)
	if err != nil {
		return failed(err)
	}
	if err := discovery.Unchanged(location, effective); err != nil {
		return failed(err)
	}
	result := projecttest.Run(ctx, plan, options.Output, func(ctx context.Context, checkout, artifacts string, args []string) runner.Result {
		return runTests(ctx, process, image, plan.ComposerRoot, checkout, artifacts, args)
	})
	if err := discovery.Unchanged(location, effective); err != nil {
		result.Complete, result.Status, result.ExitCode = false, "error", 3
		result.Diagnostics = append(result.Diagnostics, err.Error())
		if result.Artifacts != "" {
			if err := projecttest.Save(result); err != nil {
				result.Diagnostics = append(result.Diagnostics, err.Error())
			}
		}
	}
	return result
}

func runTests(ctx context.Context, process runner.Process, image, composerRoot, checkout, artifacts string, phpunit []string) runner.Result {
	name := "phpwarden-test-" + strings.ToLower(rand.Text())
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		process.Run(cleanup, runner.Command{Name: "docker", Args: []string{"rm", "--force", name}})
	}()
	args := []string{"run", "--rm", "--pull=never", "--name", name, "--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges:true",
		"--tmpfs", "/tmp:rw,nosuid,nodev,noexec,size=256m,mode=1777", "--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--env", "HOME=/tmp", "--env", "XDG_CACHE_HOME=/tmp/cache", "--env", "TMPDIR=/tmp", "--workdir", path.Join("/workspace", composerRoot)}
	for _, mount := range [][2]string{{checkout, "/workspace"}, {artifacts, "/artifacts"}} {
		var encoded bytes.Buffer
		writer := csv.NewWriter(&encoded)
		if err := writer.Write([]string{"type=bind", "source=" + mount[0], "target=" + mount[1]}); err != nil {
			return runner.Result{Err: err, ExitCode: -1}
		}
		writer.Flush()
		args = append(args, "--mount", strings.TrimSuffix(encoded.String(), "\n"))
	}
	args = append(args, "--entrypoint", "php", image)
	args = append(args, phpunit...)
	return process.Run(ctx, runner.Command{Name: "docker", Args: args})
}
