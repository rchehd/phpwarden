package worker

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

type processFunc func(context.Context, runner.Command) runner.Result

func (process processFunc) Run(ctx context.Context, command runner.Command) runner.Result {
	return process(ctx, command)
}

func TestActualPHPPatchMustSatisfyComposerRequirements(t *testing.T) {
	for _, patch := range []string{"8.4.26", "8.4.27", ""} {
		t.Run(patch, func(t *testing.T) {
			location, effective, options := fixture(t)
			if err := os.WriteFile(filepath.Join(location.Root, "composer.json"), []byte(`{"require":{"php":">=8.4.27 <9"}}`), 0o644); err != nil {
				t.Fatal(err)
			}
			var err error
			effective, err = discovery.Apply(location, effective)
			if err != nil {
				t.Fatal(err)
			}
			process := processFunc(func(context.Context, runner.Command) runner.Result {
				data := `{"version":1,"runtime":"8.4","findings":[]}`
				if patch != "" {
					data = `{"version":1,"runtime":"8.4","php_version":"` + patch + `","findings":[]}`
				}
				return runner.Result{Stdout: []byte(data)}
			})
			result := Check(context.Background(), location, effective, options, nil, process)
			want := 3
			if patch == "8.4.27" {
				want = 0
			}
			if result.ExitCode != want || result.Complete != (want == 0) {
				t.Fatalf("patch check failed: %+v", result)
			}
			if want == 0 && !slices.Contains(result.Environment.CompatibilityPending, "php.target.patch") {
				t.Fatal("target patch uncertainty was hidden")
			}
		})
	}
}

func fixture(t *testing.T) (config.Location, config.Effective, scope.Options) {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "demo.module"), []byte("<?php\nvar_dump(1);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, ".phpwarden.yaml")
	data := `{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}},"scope":{"mode":"full","include":["src"]},"tools":{"syntax":{"enabled":false},"phpcs":{"enabled":false},"phpmd":{"enabled":false},"phpstan":{"enabled":false},"twig-cs-fixer":{"enabled":false}}}`
	if err := os.WriteFile(filename, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("git", "-C", root, "init", "--quiet")
	for _, variable := range os.Environ() {
		if !strings.HasPrefix(variable, "GIT_") {
			command.Env = append(command.Env, variable)
		}
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	location := config.Location{Root: root, Path: filename}
	effective, err := config.Load(location)
	if err != nil {
		t.Fatal(err)
	}
	return location, effective, scope.Options{Root: root, CWD: root, Mode: "full", Include: []string{"src"}}
}

func cleanResult() runner.Result {
	return runner.Result{Stdout: []byte(`{"version":1,"runtime":"8.4","findings":[]}`)}
}

func TestChangingInputsCannotProduceFreshPass(t *testing.T) {
	for _, which := range []string{"file", "new file", "config"} {
		t.Run(which, func(t *testing.T) {
			location, effective, options := fixture(t)
			process := processFunc(func(context.Context, runner.Command) runner.Result {
				filename := filepath.Join(location.Root, "src", "demo.module")
				if which == "new file" {
					filename = filepath.Join(location.Root, "src", "new.php")
				}
				if which == "config" {
					filename = location.Path
				}
				if err := os.WriteFile(filename, []byte("changed\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				return cleanResult()
			})
			result := Check(context.Background(), location, effective, options, nil, process)
			if result.ExitCode != 3 || result.Complete || result.Diagnostics[len(result.Diagnostics)-1].Code != "stale" {
				t.Fatalf("stale result accepted: %+v", result)
			}
		})
	}
}

func TestConcurrentWorkersOwnTheirTemporaryDirectories(t *testing.T) {
	location, effective, options := fixture(t)
	var mutex sync.Mutex
	var directories []string
	process := processFunc(func(_ context.Context, command runner.Command) runner.Result {
		directory := filepath.Dir(command.Args[len(command.Args)-1])
		mutex.Lock()
		directories = append(directories, directory)
		mutex.Unlock()
		if _, err := os.Stat(directory); err != nil {
			t.Error(err)
		}
		return cleanResult()
	})
	var group sync.WaitGroup
	for range 2 {
		group.Go(func() {
			result := Check(context.Background(), location, effective, options, nil, process)
			if result.ExitCode != 0 || !result.Complete {
				t.Errorf("concurrent check failed: %+v", result)
			}
		})
	}
	group.Wait()
	if len(directories) != 2 || directories[0] == directories[1] {
		t.Fatalf("shared run directory: %v", directories)
	}
	for _, directory := range directories {
		if _, err := os.Stat(directory); !os.IsNotExist(err) {
			t.Fatalf("run directory not removed: %s", directory)
		}
	}
}

func TestDisabledAndNonPHPFilesDoNotRequirePHP(t *testing.T) {
	for _, disable := range []bool{false, true} {
		location, effective, options := fixture(t)
		if disable {
			effective.Config.Tools.Debug.Enabled = false
		} else {
			if err := os.Rename(filepath.Join(location.Root, "src", "demo.module"), filepath.Join(location.Root, "src", "demo.txt")); err != nil {
				t.Fatal(err)
			}
		}
		process := processFunc(func(context.Context, runner.Command) runner.Result {
			t.Fatal("PHP must not run")
			return runner.Result{}
		})
		result := Check(context.Background(), location, effective, options, nil, process)
		if result.ExitCode != 0 || !result.Complete || result.Checks[0].Status != "skipped" {
			t.Fatalf("bad applicability: %+v", result)
		}
	}
}
