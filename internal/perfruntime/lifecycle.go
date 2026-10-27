package perfruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/cache"
	"github.com/rchehd/phpwarden/internal/performance"
	"github.com/rchehd/phpwarden/internal/progress"
	"github.com/rchehd/phpwarden/internal/runner"
)

func Invalid(action, message string) Report {
	return Report{Version: 1, Command: "performance " + action, State: "error", ExitCode: 2, NextSteps: []string{}, Diagnostics: []string{message}}
}

func Run(ctx context.Context, action string, options Options) Report {
	result := Report{Version: 1, Command: "performance " + action, State: "absent", NextSteps: []string{}, Diagnostics: []string{}}
	if action != "setup" && action != "status" && action != "teardown" {
		return Invalid(action, "Unknown performance lifecycle command.")
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return Invalid(action, "Local SPX setup currently supports Linux amd64.")
	}
	w, err := locate(options)
	if err != nil {
		return Invalid(action, err.Error())
	}
	if w.process == nil {
		w.process = runner.Exec{}
	}
	result.Project = w.project
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	fail := func(err error) Report {
		result.State = "error"
		result.ExitCode = 3
		result.Diagnostics = append(result.Diagnostics, err.Error())
		if ctx.Err() == context.Canceled {
			result.ExitCode = 130
		}
		return result
	}
	if ctx.Err() != nil {
		return fail(fmt.Errorf("performance operation cancelled"))
	}
	if action == "setup" {
		if err := privateDirectory(filepath.Dir(w.directory)); err != nil {
			return fail(err)
		}
		if err := privateDirectory(w.directory); err != nil {
			return fail(err)
		}
	} else {
		if _, err := os.Lstat(w.directory); os.IsNotExist(err) {
			return result
		} else if err != nil {
			return fail(fmt.Errorf("cannot inspect runtime state"))
		}
		if err := privateDirectory(w.directory); err != nil {
			return fail(err)
		}
	}
	unlock, err := cache.LockProject(ctx, w.directory)
	if err != nil {
		return fail(fmt.Errorf("cannot lock runtime state"))
	}
	defer unlock()
	err = w.load()
	if os.IsNotExist(err) {
		if action != "setup" {
			return result
		}
		w.state = receipt{Version: 1, Project: w.project, Owner: w.owner, Producer: source().Commit, Phase: "prepared", Files: map[string]string{}, Inputs: map[string]string{}}
		if err := w.discover(ctx, options); err != nil {
			return fail(err)
		}
		var before string
		if w.state.Binding.Environment == "ddev" {
			before, err = w.gitSnapshot(ctx)
			if err != nil {
				return fail(err)
			}
		}
		progress.Stage(ctx, "Preparing local SPX integration")
		if err := w.prepareFiles(ctx, options); err != nil {
			return fail(err)
		}
		if before != "" {
			after, err := w.gitSnapshot(ctx)
			if err != nil || before != after {
				return fail(fmt.Errorf("Git status or diff changed during local setup; review the project before applying"))
			}
		}
	} else if err != nil {
		return fail(fmt.Errorf("runtime receipt is invalid or unsafe"))
	} else if action == "setup" && w.state.Phase == "removed" {
		if err := w.verifyFiles(true); err != nil {
			return fail(err)
		}
		w.state = receipt{Version: 1, Project: w.project, Owner: w.owner, Producer: source().Commit, Phase: "prepared", Files: map[string]string{}, Inputs: map[string]string{}}
		if err := w.discover(ctx, options); err != nil {
			return fail(err)
		}
		if err := w.prepareFiles(ctx, options); err != nil {
			return fail(err)
		}
	} else if action == "setup" {
		if err := w.sameSelection(ctx, options); err != nil {
			return fail(err)
		}
	}
	result.Environment = w.state.Binding.Environment
	result.Reports = filepath.Join(w.directory, "reports")
	result.ReadCommand = []string{"phpwarden", "performance", "--reports", result.Reports, "--producer", performance.Producer, "--project", w.project, "--container-root", w.state.Binding.ContainerRoot}
	if w.state.Binding.HTTP {
		result.HTTPKeyFile = filepath.Join(w.directory, "http.key")
	}
	if w.state.Binding.Environment != "ddev" {
		result.Image = w.imageTag()
	}
	if action == "teardown" {
		if w.state.Phase == "removed" {
			if options.Apply && !w.state.Restored {
				if w.state.Binding.Environment == "docker" {
					return fail(fmt.Errorf("restore the original bare Docker launch manually"))
				}
				if err := w.verifyInputs(); err != nil {
					return fail(err)
				}
				if err := w.validateRuntimeInputs(ctx); err != nil {
					return fail(err)
				}
				if err := w.restore(ctx); err != nil {
					return fail(err)
				}
				w.state.Restored = true
				if err := w.save(); err != nil {
					return fail(err)
				}
			}
			result.State = "removed"
			result.HTTPKeyFile = ""
			if !w.state.Restored {
				result.NextSteps = append(result.NextSteps, "Adapters are removed; restore/restart the original runtime to unload SPX. Runtime was not checked.")
			}
			return result
		}
		if options.Apply && w.state.Binding.Environment == "docker" {
			return fail(fmt.Errorf("bare Docker recreation is manual: restore the original launch configuration, then run teardown without --apply"))
		}
		if err := w.verifyFiles(true); err != nil {
			return fail(err)
		}
		if options.Apply {
			if err := w.verifyInputs(); err != nil {
				return fail(err)
			}
			if err := w.validateRuntimeInputs(ctx); err != nil {
				return fail(err)
			}
		}
		w.state.Phase = "removing"
		if err := w.save(); err != nil {
			return fail(err)
		}
		if w.state.Binding.Environment == "ddev" {
			if err := w.exclude(ctx, false); err != nil {
				return fail(err)
			}
			if err := w.save(); err != nil {
				return fail(err)
			}
		}
		for name := range w.state.Files {
			filename, err := w.filename(name)
			if err != nil {
				return fail(err)
			}
			if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
				return fail(fmt.Errorf("could not remove an owned adapter"))
			}
		}
		if options.Apply {
			if err := w.restore(ctx); err != nil {
				return fail(err)
			}
		}
		w.state.Phase = "removed"
		w.state.Restored = options.Apply
		if err := w.save(); err != nil {
			return fail(err)
		}
		result.State = "removed"
		result.HTTPKeyFile = ""
		result.NextSteps = append(result.NextSteps, "Recorded profiles and local images are preserved.")
		if !options.Apply {
			result.NextSteps = append(result.NextSteps, "Restore/restart the original runtime to unload SPX; deleting adapters does not change a running container.")
		}
		return result
	}
	if action == "status" && w.state.Phase == "removed" {
		result.State = "removed"
		result.HTTPKeyFile = ""
		if !w.state.Restored {
			result.NextSteps = append(result.NextSteps, "Adapters are removed; restore/restart the original runtime to unload SPX. Runtime was not checked.")
		}
		return result
	}
	if w.state.Phase != "prepared" {
		return fail(fmt.Errorf("finish teardown before preparing this checkout again"))
	}
	if err := w.verifyFiles(false); err != nil {
		return fail(err)
	}
	if err := w.verifyInputs(); err != nil {
		return fail(err)
	}
	result.Prepared = true
	result.State = "prepared"
	if action == "setup" && options.Apply {
		progress.Stage(ctx, "Applying local SPX integration")
		if err := w.apply(ctx); err != nil {
			return fail(err)
		}
		if err := w.verifyFiles(false); err != nil {
			return fail(err)
		}
		if err := w.verifyInputs(); err != nil {
			return fail(err)
		}
	}
	if options.Apply || action == "status" {
		loaded, err := w.loaded(ctx)
		if err != nil {
			return fail(err)
		}
		result.Loaded = loaded
		result.RuntimeChecked = true
		if loaded {
			result.State = "active"
		}
		if action == "setup" && options.Apply && w.state.Binding.Environment != "docker" && !loaded {
			return fail(fmt.Errorf("runtime restarted but the selected PHP process did not pass SPX verification"))
		}
	}
	if w.state.Binding.Environment == "docker" {
		result.LaunchArguments = []string{"--mount", "type=bind,src=" + result.Reports + ",dst=" + containerReports, "--mount", "type=bind,src=" + filepath.Join(w.directory, "spx.ini") + ",dst=/usr/local/etc/php/conf.d/99-phpwarden-spx.ini,readonly", w.imageTag()}
		result.NextSteps = append(result.NextSteps, "Bare Docker: use the derived image and launch arguments with your original mounts, networks, ports, user and command. The existing container is never cloned or recreated automatically.")
	}
	if !result.Loaded {
		result.NextSteps = append(result.NextSteps, "setup --apply builds the local SPX image and restarts DDEV or the selected Compose service; bare Docker requires manual launch.")
	}
	result.NextSteps = append(result.NextSteps, "Capture CLI commands with SPX_ENABLED=1 SPX_REPORT=full SPX_METRICS=wt,ct,zm SPX_SAMPLING_PERIOD=0. Profiling stays off for normal commands/requests.")
	if w.state.Binding.HTTP {
		result.NextSteps = append(result.NextSteps, "For an allowed HTTP client, send SPX_ENABLED=1 and SPX_KEY from the private key file as cookies; do not put the key into URLs or reports.")
	}
	return result
}

func (w *workspace) sameSelection(ctx context.Context, options Options) error {
	b := w.state.Binding
	for _, pair := range [][2]string{{options.Environment, b.Environment}, {options.PHP, b.PHP}, {options.Service, b.Service}, {options.Container, b.Container}, {options.ComposeProject, b.ComposeProject}, {options.ContainerRoot, b.ContainerRoot}, {options.HTTPAllow, b.HTTPAllow}} {
		if pair[0] != "" && pair[0] != pair[1] {
			return fmt.Errorf("an existing local integration selects another runtime; teardown before changing it")
		}
	}
	if options.HTTP && !b.HTTP {
		return fmt.Errorf("existing integration settings differ; teardown before changing them")
	}
	if options.Image != "" {
		image, err := w.inspectImage(ctx, options.Image)
		if err != nil || image.ID != b.BaseID {
			return fmt.Errorf("existing integration base image differs")
		}
	}
	for _, pair := range [][2][]string{{options.ComposeFiles, b.ComposeFiles}, {options.EnvFiles, b.EnvFiles}} {
		if len(pair[0]) > 0 {
			resolved, err := absoluteFiles(w.project, pair[0])
			if err != nil || !slices.Equal(resolved, pair[1]) {
				return fmt.Errorf("existing Compose invocation differs")
			}
		}
	}
	if len(options.Profiles) > 0 && !slices.Equal(options.Profiles, b.Profiles) {
		return fmt.Errorf("existing Compose profiles differ")
	}
	return nil
}

func (w *workspace) validateRuntimeInputs(ctx context.Context) error {
	if err := w.localDocker(ctx); err != nil {
		return err
	}
	if w.state.Binding.Environment == "compose" {
		_, hash, err := w.composeConfig(ctx)
		if err != nil {
			return err
		}
		if hash != w.state.Binding.ComposeHash {
			return fmt.Errorf("resolved Compose inputs or environment changed; refusing to apply a stale overlay")
		}
	}
	return nil
}

func (w *workspace) apply(ctx context.Context) error {
	if err := w.validateRuntimeInputs(ctx); err != nil {
		return err
	}
	if loaded, err := w.loaded(ctx); err != nil {
		return err
	} else if loaded {
		return nil
	}
	b := w.state.Binding
	if b.Environment == "ddev" {
		before, err := w.gitSnapshot(ctx)
		if err != nil {
			return err
		}
		if _, err := w.command(ctx, "ddev", "--skip-hooks", "restart"); err != nil {
			return err
		}
		after, err := w.gitSnapshot(ctx)
		if err != nil || before != after {
			return fmt.Errorf("Git status/diff changed during DDEV restart; inspect the project")
		}
		return nil
	}
	if _, err := w.command(ctx, "docker", "image", "tag", b.BaseID, w.baseTag()); err != nil {
		return err
	}
	if _, err := w.command(ctx, "docker", "build", "--pull=false", "--tag", w.imageTag(), "--file", filepath.Join(w.directory, "Dockerfile"), w.directory); err != nil {
		return err
	}
	data, err := w.command(ctx, "docker", "image", "inspect", "--format", "{{.Id}}", w.imageTag())
	if err != nil {
		return err
	}
	w.state.ImageID = strings.TrimSpace(string(data))
	if err := w.save(); err != nil {
		return err
	}
	if b.Environment == "compose" {
		_, err = w.command(ctx, "docker", append(w.composeArgs(true), "up", "--detach", "--no-deps", "--no-build", "--pull", "never", b.Service)...)
		return err
	}
	return nil
}

func (w *workspace) restore(ctx context.Context) error {
	progress.Stage(ctx, "Restoring original PHP runtime")
	if w.state.Binding.Environment == "ddev" {
		_, err := w.command(ctx, "ddev", "--skip-hooks", "restart")
		return err
	}
	_, err := w.command(ctx, "docker", append(w.composeArgs(false), "up", "--detach", "--no-deps", "--no-build", "--pull", "never", w.state.Binding.Service)...)
	return err
}

func (w *workspace) loaded(ctx context.Context) (bool, error) {
	if err := w.validateRuntimeInputs(ctx); err != nil {
		return false, err
	}
	container, err := w.container(ctx)
	if err != nil {
		return false, err
	}
	if container.ID == "" || !container.State.Running {
		return false, nil
	}
	owner := container.Config.Labels[ownerLabel]
	if owner != "" && owner != w.owner {
		return false, fmt.Errorf("selected container belongs to another SPX checkout")
	}
	if owner == "" {
		return false, nil
	}
	if container.Config.Labels[producerLabel] != source().Commit {
		return false, fmt.Errorf("runtime producer label does not match the pinned SPX source")
	}
	if w.state.ImageID != "" && container.Image != w.state.ImageID {
		return false, fmt.Errorf("selected container does not use the verified local SPX image")
	}
	reports, ini := false, false
	for _, mount := range container.Mounts {
		if mount.Destination == containerReports && mount.Source == filepath.Join(w.directory, "reports") && mount.RW {
			reports = true
		}
		if mount.Source == filepath.Join(w.directory, "spx.ini") && !mount.RW {
			ini = true
		}
	}
	if !reports || !ini {
		return false, fmt.Errorf("runtime mounts do not match this checkout's reports and private configuration")
	}
	php := "php"
	if w.state.Binding.Environment == "ddev" {
		php = "php" + w.state.Binding.PHP
	}
	configPath := "/usr/local/etc/php/conf.d/99-phpwarden-spx.ini"
	if w.state.Binding.Environment == "ddev" {
		configPath = "/etc/php/" + w.state.Binding.PHP + "/cli/conf.d/99-phpwarden-spx.ini"
	}
	encodedPath, _ := json.Marshal(configPath)
	program := `echo json_encode(["loaded"=>extension_loaded("spx"),"php"=>PHP_MAJOR_VERSION.".".PHP_MINOR_VERSION,"producer"=>is_file("/opt/phpwarden-spx/producer")?trim(file_get_contents("/opt/phpwarden-spx/producer")):"","reports"=>ini_get("spx.data_dir"),"writable"=>is_writable("/var/lib/phpwarden-spx"),"config_hash"=>is_file(` + string(encodedPath) + `)?hash_file("sha256",` + string(encodedPath) + `):""]);`
	data, err := w.command(ctx, "docker", "exec", container.ID, php, "-d", "auto_prepend_file=", "-d", "auto_append_file=", "-r", program)
	if err != nil {
		return false, err
	}
	var probe struct {
		Loaded     bool   `json:"loaded"`
		PHP        string `json:"php"`
		Producer   string `json:"producer"`
		Reports    string `json:"reports"`
		Writable   bool   `json:"writable"`
		ConfigHash string `json:"config_hash"`
	}
	if json.Unmarshal(data, &probe) != nil {
		return false, fmt.Errorf("invalid PHP runtime probe response")
	}
	return probe.Loaded && probe.PHP == w.state.Binding.PHP && probe.Producer == source().Commit && probe.Reports == containerReports && probe.Writable && probe.ConfigHash == w.state.Files["spx.ini"], nil
}
