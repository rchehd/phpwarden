package launcher

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/runner"
)

func TestDockerFetcherUsesIsolatedNetworkedContainer(t *testing.T) {
	for _, name := range []string{"valid", "failure"} {
		t.Run(name, func(t *testing.T) {
			var mounted string
			removed := false
			process := processFunc(func(_ context.Context, command runner.Command) runner.Result {
				if command.Name != "docker" {
					t.Fatalf("unexpected command: %+v", command)
				}
				if command.Args[0] == "rm" {
					removed = true
					return runner.Result{}
				}
				for _, argument := range []string{"--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges:true", "--pull=never", testImage, "--no-dev"} {
					if !slices.Contains(command.Args, argument) {
						t.Fatalf("audit container misses %s: %v", argument, command.Args)
					}
				}
				if slices.Contains(command.Args, "--network=none") || !slices.Equal(command.Args[len(command.Args)-3:], []string{"worker", "audit", "--no-dev"}) {
					t.Fatalf("audit container cannot reach advisories: %v", command.Args)
				}
				for _, argument := range command.Args {
					if strings.HasPrefix(argument, "type=bind,source=") {
						if !strings.HasSuffix(argument, ",target=/audit,readonly") {
							t.Fatalf("audit files are writable: %s", argument)
						}
						mounted = strings.TrimSuffix(strings.TrimPrefix(argument, "type=bind,source="), ",target=/audit,readonly")
					}
				}
				entries, _ := os.ReadDir(mounted)
				lock, _ := os.ReadFile(filepath.Join(mounted, "composer.lock"))
				if len(entries) != 2 || string(lock) != "lock" {
					t.Fatalf("audit container sees more than the Composer files: %v", entries)
				}
				if name == "failure" {
					return runner.Result{ExitCode: 3, Err: os.ErrInvalid, Stderr: []byte("Network unreachable")}
				}
				return runner.Result{Stdout: []byte(`{"advisories":[]}` + "\n")}
			})
			output, err := dockerFetcher(testImage, process)(context.Background(), []byte("json"), []byte("lock"), true)
			if _, statErr := os.Stat(mounted); !os.IsNotExist(statErr) || !removed {
				t.Fatal("audit container resources leaked")
			}
			if name == "valid" && (err != nil || string(output) != `{"advisories":[]}`) || name == "failure" && (err == nil || !strings.Contains(err.Error(), "Network unreachable")) {
				t.Fatalf("unexpected audit container result %q: %v", output, err)
			}
		})
	}
}
