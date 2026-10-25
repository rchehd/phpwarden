package launcher

import (
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/runner"
)

func TestDiagnosisInspectsWithoutRunningOrPulling(t *testing.T) {
	location, effective, _ := fixture(t)
	var calls []runner.Command
	image, err := DiagnoseImage(context.Background(), effective, dockerDouble(t, location, &calls))
	if err != nil || image != testImage {
		t.Fatalf("%s %v", image, err)
	}
	for _, call := range calls {
		if call.Args[0] != "context" && call.Args[0] != "image" {
			t.Fatalf("diagnosis mutated Docker: %+v", call)
		}
	}
}

func TestUpdatesUsesOnlyCopiedComposerFilesAndPinnedWorker(t *testing.T) {
	location, effective, _ := fixture(t)
	var calls []runner.Command
	base := dockerDouble(t, location, &calls)
	mounted := ""
	removed := false
	process := processFunc(func(ctx context.Context, command runner.Command) runner.Result {
		if command.Args[0] == "context" || command.Args[0] == "image" {
			return base(ctx, command)
		}
		if command.Args[0] == "rm" {
			removed = true
			return runner.Result{}
		}
		if command.Args[0] != "run" || !slices.Equal(command.Args[len(command.Args)-2:], []string{"worker", "updates"}) {
			t.Fatalf("wrong command %+v", command)
		}
		for _, flag := range []string{testImage, "--pull=never", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges:true"} {
			if !slices.Contains(command.Args, flag) {
				t.Fatalf("missing %s: %+v", flag, command)
			}
		}
		if slices.Contains(command.Args, "--network=none") {
			t.Fatal("updates cannot reach repository metadata")
		}
		for i, arg := range command.Args {
			if arg != "--mount" {
				continue
			}
			fields, err := csv.NewReader(strings.NewReader(command.Args[i+1])).Read()
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range fields {
				if strings.HasPrefix(field, "source=") {
					mounted = strings.TrimPrefix(field, "source=")
				}
			}
			if !slices.Contains(fields, "readonly") {
				t.Fatal("Composer copies are writable")
			}
		}
		if mounted == "" || mounted == location.Root {
			t.Fatal("mounted project instead of private copies")
		}
		entries, _ := os.ReadDir(mounted)
		data, _ := os.ReadFile(filepath.Join(mounted, "composer.lock"))
		if len(entries) != 2 || string(data) != "lock" {
			t.Fatalf("unexpected mount %v", entries)
		}
		return runner.Result{Stdout: []byte(`{"outdated":{"locked":[]},"audit":{"advisories":[]},"diagnostics":false}`)}
	})
	data, err := UpdatesFetcher(effective, process)(context.Background(), []byte("manifest"), []byte("lock"))
	if err != nil || len(data) == 0 || !removed {
		t.Fatalf("%s %v cleanup=%v", data, err, removed)
	}
	if _, err := os.Stat(mounted); !os.IsNotExist(err) {
		t.Fatal("temporary copies leaked")
	}
}
