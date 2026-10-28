package launcher

import (
	"context"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/projecttest"
	"github.com/rchehd/phpwarden/internal/runner"
)

func TestProjectTestDockerProcessOnlyMountsPrivateCopiesAndCleansUp(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "failure"}[failure], func(t *testing.T) {
			var calls []runner.Command
			process := processFunc(func(ctx context.Context, command runner.Command) runner.Result {
				calls = append(calls, command)
				if command.Args[0] == "rm" {
					return runner.Result{}
				}
				for _, flag := range []string{"--network=none", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges:true", "--pull=never", testImage} {
					if !slices.Contains(command.Args, flag) {
						t.Fatalf("missing %s: %v", flag, command.Args)
					}
				}
				var mounts [][]string
				for i, arg := range command.Args {
					if arg == "--mount" {
						fields, err := csv.NewReader(strings.NewReader(command.Args[i+1])).Read()
						if err != nil {
							t.Fatal(err)
						}
						mounts = append(mounts, fields)
					}
				}
				if len(mounts) != 2 || !slices.Equal(mounts[0], []string{"type=bind", "source=/private/copy, with spaces", "target=/workspace"}) || !slices.Equal(mounts[1], []string{"type=bind", "source=/private/results", "target=/artifacts"}) {
					t.Fatalf("%v", mounts)
				}
				if failure {
					return runner.Result{ExitCode: -1, Err: errors.New("fixture failure")}
				}
				return runner.Result{}
			})
			runTests(context.Background(), process, testImage, "backend", "/private/copy, with spaces", "/private/results", []string{"/workspace/backend/vendor/bin/phpunit", "--filter", "name with spaces"})
			if len(calls) != 2 || calls[1].Args[0] != "rm" || calls[1].Args[2] != calls[0].Args[slices.Index(calls[0].Args, "--name")+1] {
				t.Fatalf("missing owned cleanup: %v", calls)
			}
		})
	}
}

func TestProjectTestsRejectMissingPHPUnitBeforeDocker(t *testing.T) {
	location, effective, _ := fixture(t)
	os.WriteFile(filepath.Join(location.Root, "composer.json"), []byte(`{}`), 0o600)
	process := processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("missing test prerequisites reached Docker")
		return runner.Result{}
	})
	result := Test(context.Background(), location, effective, projecttest.Options{}, process)
	if result.ExitCode != 3 || result.Complete || !strings.Contains(result.Diagnostics[0], "dev dependencies") {
		t.Fatalf("%+v", result)
	}
	if _, err := config.Load(location); err != nil {
		t.Fatal(err)
	}
}
