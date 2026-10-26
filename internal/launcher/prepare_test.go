package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/progress"
	"github.com/rchehd/phpwarden/internal/runner"
)

func TestPrepareFetchesOnlyTheSelectedImageAndPreservesProject(t *testing.T) {
	for _, mode := range []string{"missing", "present", "bad local image", "bad downloaded image", "pull failure", "docker missing", "remote context", "cancel pull", "cancel verification", "timeout", "changed config", "changed lock"} {
		t.Run(mode, func(t *testing.T) {
			location, _, _ := fixture(t)
			var document map[string]any
			original, _ := os.ReadFile(location.Path)
			json.Unmarshal(original, &document)
			document["runner"] = map[string]any{"image_pull": "never", "timeout_seconds": 1}
			data, _ := json.Marshal(document)
			os.WriteFile(location.Path, data, 0o644)
			manifest := filepath.Join(location.Root, "composer.json")
			lock := filepath.Join(location.Root, "composer.lock")
			os.WriteFile(manifest, []byte(`{"require":{"drupal/core":"^11"}}`), 0o644)
			lockBefore := []byte(`{"packages":[{"name":"drupal/core","version":"11.4.1","require":{"php":">=8.3"}}]}`)
			os.WriteFile(lock, lockBefore, 0o644)
			effective, err := config.Load(location)
			if err != nil {
				t.Fatal(err)
			}
			effective, err = discovery.Apply(location, effective)
			if err != nil {
				t.Fatal(err)
			}
			selection := publishedSelection()
			available := mode == "present" || mode == "bad local image"
			pulls, inspections := 0, 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel verification" {
				ctx = progress.WithReporter(ctx, func(stage string) {
					if stage == "Verifying project settings are unchanged" {
						cancel()
					}
				})
			}
			process := processFunc(func(ctx context.Context, command runner.Command) runner.Result {
				if command.Name != "docker" {
					t.Fatalf("unexpected host process: %+v", command)
				}
				switch command.Args[0] {
				case "context":
					if mode == "docker missing" {
						return runner.Result{Unavailable: true, ExitCode: -1, Err: errors.New("missing docker")}
					}
					if mode == "remote context" {
						return runner.Result{Stdout: []byte(`"tcp://remote:2375"`)}
					}
					return runner.Result{Stdout: []byte(`"unix:///var/run/docker.sock"`)}
				case "image":
					inspections++
					if strings.Join(command.Args, " ") != "image inspect "+selection.Image {
						t.Fatalf("wrong inspection: %+v", command)
					}
					if !available {
						return runner.Result{ExitCode: 1, Stderr: []byte("No such image")}
					}
					id := selection.ID
					if mode == "bad local image" || mode == "bad downloaded image" {
						id = "wrong"
					}
					return runner.Result{Stdout: inspected(selection, id)}
				case "pull":
					pulls++
					if strings.Join(command.Args, " ") != "pull --platform linux/amd64 "+selection.Image {
						t.Fatalf("wrong pull: %+v", command)
					}
					if mode == "pull failure" {
						return runner.Result{ExitCode: 1, Stderr: []byte("registry unavailable")}
					}
					if mode == "cancel pull" {
						cancel()
						return runner.Result{ExitCode: -1}
					}
					if mode == "timeout" {
						<-ctx.Done()
						return runner.Result{TimedOut: true, ExitCode: -1}
					}
					if mode == "changed config" {
						os.WriteFile(location.Path, append(data, '\n'), 0o644)
					}
					if mode == "changed lock" {
						os.WriteFile(lock, []byte(`{"packages":[{"name":"drupal/core","version":"11.4.2"}]}`), 0o644)
					}
					available = true
					return runner.Result{}
				default:
					t.Fatalf("prepare must never run a worker, audit or build: %+v", command)
					return runner.Result{}
				}
			})
			result := prepareSelected(ctx, location, effective, selection, process)
			wantSuccess := mode == "missing" || mode == "present"
			if result.Ready != wantSuccess || result.Command != "prepare" || result.Version != 1 {
				t.Fatalf("%+v", result)
			}
			if wantSuccess {
				if result.ExitCode != 0 || result.ImageID != testImage || len(result.Diagnostics) != 0 {
					t.Fatalf("%+v", result)
				}
			} else {
				wantCode := 3
				if strings.HasPrefix(mode, "cancel ") {
					wantCode = 130
				}
				if result.ExitCode != wantCode || result.ImageID != "" || len(result.Diagnostics) != 1 {
					t.Fatalf("%+v", result)
				}
			}
			if mode == "present" || mode == "bad local image" || mode == "docker missing" || mode == "remote context" {
				if pulls != 0 {
					t.Fatalf("unexpected downloads: %d", pulls)
				}
			} else if pulls != 1 {
				t.Fatalf("expected one explicit download despite image_pull=never; got %d", pulls)
			}
			if mode == "present" && inspections != 1 {
				t.Fatalf("local image was not reused: %d inspections", inspections)
			}
			if mode == "timeout" && result.Diagnostics[0].Code != "prepare_timeout" {
				t.Fatalf("%+v", result)
			}
			if strings.HasPrefix(mode, "changed ") && result.Diagnostics[0].Code != "stale" {
				t.Fatalf("%+v", result)
			}
			if mode != "changed config" {
				if after, _ := os.ReadFile(location.Path); string(after) != string(data) {
					t.Fatal("prepare modified config")
				}
			}
			if mode != "changed lock" {
				if after, _ := os.ReadFile(lock); string(after) != string(lockBefore) {
					t.Fatal("prepare modified Composer metadata")
				}
			}
		})
	}
}

func TestPreparePublicEntryPointValidatesDevelopmentImagesWithoutAnalysis(t *testing.T) {
	location, effective, _ := fixture(t)
	var calls []runner.Command
	result := Prepare(context.Background(), location, effective, dockerDouble(t, location, &calls))
	if !result.Ready || result.ExitCode != 0 || result.Selection == nil || !result.Selection.Development || len(calls) != 2 {
		t.Fatalf("%+v, calls=%+v", result, calls)
	}
	result = Prepare(context.Background(), location, effective, processFunc(func(context.Context, runner.Command) runner.Result {
		return runner.Result{Unavailable: true, ExitCode: -1}
	}))
	if result.Ready || result.ExitCode != 3 {
		t.Fatalf("%+v", result)
	}
	effective.Config.PHP.Target.Min, effective.Config.PHP.Target.Max = "9.0", "9.0"
	result = Prepare(context.Background(), location, effective, processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("Docker invoked before compatibility validation")
		return runner.Result{}
	}))
	if result.Ready || result.ExitCode != 2 {
		t.Fatalf("%+v", result)
	}
}
