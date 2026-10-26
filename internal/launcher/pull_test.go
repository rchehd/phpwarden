package launcher

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func publishedSelection() toolset.Selection {
	return toolset.Selection{ID: "fixture", Image: "ghcr.io/rchehd/phpwarden@" + testImage, Platform: "linux/amd64", Protocol: 2, PHPVersion: "8.4.26", Capabilities: []string{"debug"}}
}

func inspected(selection toolset.Selection, toolsetID string) []byte {
	data, _ := json.Marshal([]any{map[string]any{"Id": testImage, "Os": "linux", "Architecture": "amd64", "RepoDigests": []string{selection.Image}, "Config": map[string]any{"Labels": map[string]string{"io.phpwarden.protocol": "2", "io.phpwarden.toolset": toolsetID, "io.phpwarden.php-version": selection.PHPVersion, "io.phpwarden.checkers": "debug"}}}})
	return data
}

func TestPullMissingPinnedImageAndVerifyBeforeUse(t *testing.T) {
	for _, test := range []string{"success", "pull failure", "wrong downloaded identity", "policy never", "daemon failure", "already present", "missing development"} {
		t.Run(test, func(t *testing.T) {
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			selection := publishedSelection()
			policy := "missing"
			available := test == "already present"
			pulls := 0
			if test == "policy never" {
				policy = "never"
			}
			if test == "missing development" {
				selection.Development = true
			}
			process := processFunc(func(ctx context.Context, command runner.Command) runner.Result {
				if command.Args[0] == "pull" {
					pulls++
					want := fmt.Sprintf("pull --platform %s %s", selection.Platform, selection.Image)
					if strings.Join(command.Args, " ") != want {
						t.Fatalf("wrong pull %+v", command)
					}
					if test == "pull failure" {
						return runner.Result{ExitCode: 1, Err: errors.New("failed"), Stderr: []byte("network unreachable")}
					}
					available = true
					return runner.Result{}
				}
				if !available {
					message := "Error response from daemon: No such image: " + selection.Image
					if test == "daemon failure" {
						message = "permission denied connecting to daemon"
					}
					return runner.Result{ExitCode: 1, Err: errors.New("missing"), Stderr: []byte(message)}
				}
				id := selection.ID
				if test == "wrong downloaded identity" {
					id = "other"
				}
				return runner.Result{Stdout: inspected(selection, id)}
			})
			image, err := ensureImage(context.Background(), selection, policy, process)
			wantSuccess := test == "success" || test == "already present"
			if wantSuccess != (err == nil) || wantSuccess && image != testImage {
				t.Fatalf("%s %v", image, err)
			}
			wantPull := test == "success" || test == "pull failure" || test == "wrong downloaded identity"
			if (pulls == 1) != wantPull {
				t.Fatalf("unexpected pull count %d", pulls)
			}
		})
	}
}

func TestConcurrentPullsShareImageLock(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	selection := publishedSelection()
	var mutex sync.Mutex
	available := false
	pulls := 0
	process := processFunc(func(_ context.Context, command runner.Command) runner.Result {
		mutex.Lock()
		defer mutex.Unlock()
		if command.Args[0] == "pull" {
			pulls++
			available = true
			return runner.Result{}
		}
		if !available {
			return runner.Result{ExitCode: 1, Stderr: []byte("No such image")}
		}
		return runner.Result{Stdout: inspected(selection, selection.ID)}
	})
	var group sync.WaitGroup
	for range 5 {
		group.Go(func() {
			if _, err := ensureImage(context.Background(), selection, "missing", process); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	if pulls != 1 {
		t.Fatalf("downloaded image %d times", pulls)
	}
}

func TestPullCancellationDoesNotStartDownload(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ensureImage(ctx, publishedSelection(), "missing", processFunc(func(ctx context.Context, command runner.Command) runner.Result {
		if command.Args[0] == "pull" {
			t.Fatal("cancelled pull started")
		}
		return runner.Result{ExitCode: 1, Stderr: []byte("No such image")}
	}))
	if err == nil {
		t.Fatal("cancelled lookup returned success")
	}
}
