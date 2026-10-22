package worker

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/runrequest"
	"github.com/rchehd/phpwarden/internal/scope"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func TestRequestWorkerDoesNotNeedGitAndRejectsChangedInputs(t *testing.T) {
	for _, which := range []string{"no git", "wire roundtrip", "file", "config", "traversal", "symlink", "missing range", "bad range", "protocol", "toolset", "effective config"} {
		t.Run(which, func(t *testing.T) {
			location, effective, options := fixture(t)
			selection, err := scope.Build(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			request, err := runrequest.Build(location, effective, selection, false, "sha256:"+strings.Repeat("a", 64), toolset.DebugID)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(filepath.Join(location.Root, ".git")); err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(location.Root, "src", "demo.module")
			switch which {
			case "wire roundtrip":
				request.Config.Tools.Debug.Include = []string{}
				data, err := json.Marshal(request)
				if err != nil {
					t.Fatal(err)
				}
				var decoded runrequest.Request
				if err := json.Unmarshal(data, &decoded); err != nil {
					t.Fatal(err)
				}
				request = decoded
			case "file", "config":
				if which == "config" {
					filename = location.Path
				}
				if err := os.WriteFile(filename, []byte("changed\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "traversal":
				request.Selection.Files = []string{"../outside.php"}
			case "symlink":
				if err := os.Remove(filename); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(location.Path, filename); err != nil {
					t.Fatal(err)
				}
			case "missing range":
				delete(request.Selection.LineMap, "src/demo.module")
			case "bad range":
				request.Selection.LineMap["src/demo.module"] = []scope.Range{{Start: 0, End: 2}}
			case "protocol":
				request.Version = 2
			case "toolset":
				request.Toolset = "other"
			case "effective config":
				request.Config.Tools.Debug.Enabled = false
			}
			process := processFunc(func(context.Context, runner.Command) runner.Result {
				if which != "no git" && which != "wire roundtrip" {
					t.Fatal("invalid request must fail before running PHP")
				}
				return cleanResult()
			})
			result := Execute(context.Background(), location.Root, request, process)
			want := 3
			if which == "no git" || which == "wire roundtrip" {
				want = 0
			}
			if result.ExitCode != want || result.RunID != request.RunID || result.Complete != (want == 0) {
				t.Fatalf("unexpected request result: %+v", result)
			}
		})
	}
}

func TestComposerChangesDuringWorkerExecutionAreStale(t *testing.T) {
	for _, before := range []bool{false, true} {
		for _, metadata := range []string{"composer.lock", "vendor/composer/installed.json"} {
			location, effective, options := fixture(t)
			filename := filepath.Join(location.Root, "composer.lock")
			if err := os.WriteFile(filename, []byte(`{"packages":[{"name":"drupal/core","version":"11.2.5"}]}`), 0o644); err != nil {
				t.Fatal(err)
			}
			if metadata != "composer.lock" {
				filename = filepath.Join(location.Root, metadata)
				if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filename, []byte(`{"packages":[{"name":"drupal/core","version":"11.2.5"}]}`), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var err error
			effective, err = discovery.Apply(location, effective)
			if err != nil {
				t.Fatal(err)
			}
			selection, err := scope.Build(context.Background(), options)
			if err != nil {
				t.Fatal(err)
			}
			request, err := runrequest.Build(location, effective, selection, false, "sha256:"+strings.Repeat("a", 64), toolset.DebugID)
			if err != nil {
				t.Fatal(err)
			}
			mutate := func() {
				if err := os.WriteFile(filename, []byte(`{"packages":[{"name":"drupal/core","version":"11.2.6"}]}`), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if before {
				mutate()
			}
			process := processFunc(func(context.Context, runner.Command) runner.Result {
				if before {
					t.Fatal("changed metadata must be rejected before running PHP")
				}
				mutate()
				return cleanResult()
			})
			result := Execute(context.Background(), location.Root, request, process)
			if result.ExitCode != 3 || result.Complete || result.Diagnostics[len(result.Diagnostics)-1].Code != "stale" {
				t.Fatalf("changed Composer metadata accepted: %+v", result)
			}
		}
	}
}
