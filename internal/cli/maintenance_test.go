package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/projectupdates"
)

func TestHookProtocolErrorsAndAbortRemainJSON(t *testing.T) {
	for _, test := range []struct{ client, input string }{
		{"codex", `{"hook_event_name":"Stop","session_id":"s","status":"aborted"}`},
		{"claude", `{"hook_event_name":"Stop","session_id":"s","status":"error"}`},
		{"cursor", `{"hook_event_name":"stop","conversation_id":"s","status":"aborted","workspace_roots":["/tmp"]}`},
		{"codex", `invalid input`},
	} {
		var stdout, stderr bytes.Buffer
		code := RunContext(context.Background(), []string{"hook", "stop", "--client", test.client}, t.TempDir(), strings.NewReader(test.input), &stdout, &stderr)
		if code != 0 || !json.Valid(stdout.Bytes()) {
			t.Fatalf("exit %d stdout=%s stderr=%s", code, &stdout, &stderr)
		}
	}
	for _, args := range [][]string{{}, {"--client"}, {"--client", "--managed"}, {"--client", "codex", "--managed=yes"}, {"--client=codex", "--project="}, {"--client", "unknown"}} {
		var stdout, stderr bytes.Buffer
		code := RunContext(context.Background(), append([]string{"hook", "stop"}, args...), t.TempDir(), strings.NewReader(`{}`), &stdout, &stderr)
		if code != 2 || !json.Valid(stdout.Bytes()) {
			t.Fatalf("%v exit %d stdout=%s", args, code, &stdout)
		}
	}
}

func TestOfflineUpdatesAreExplicitlyIncompleteAndDoNotInvokeDocker(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	root := project(t)
	for name, data := range map[string]string{"composer.json": `{}`, "composer.lock": `{"packages":[]}`} {
		os.WriteFile(filepath.Join(root, name), []byte(data), 0o644)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"updates", "--project", root, "--offline", "--format=json"}, root, &stdout, &stderr)
	var result projectupdates.Report
	if json.Unmarshal(stdout.Bytes(), &result) != nil || code != 3 || result.Complete || len(result.Sources) != 3 || stderr.Len() != 0 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, &stdout, &stderr)
	}
	for _, source := range result.Sources {
		if source.Status != "unavailable" {
			t.Fatalf("%+v", source)
		}
	}
}

func TestMaintenanceFlagsCannotLeakIntoOtherCommands(t *testing.T) {
	for _, args := range [][]string{{"check", "--security"}, {"init", "--offline"}, {"doctor", "--direct"}, {"updates", "--mode=full"}, {"updates", "--format=compact"}, {"updates", "--hooks=all"}, {"config", "show", "--hooks=all"}, {"updates", "--runtime-version=8.4.1"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, project(t), &stdout, &stderr); code != 2 {
			t.Fatalf("%v: exit %d %s %s", args, code, &stdout, &stderr)
		}
	}
}

func TestCompletionWithCleanScopedDiffNeedsNoWorker(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := project(t)
	filename := filepath.Join(root, ".phpwarden.yaml")
	data, _ := os.ReadFile(filename)
	os.WriteFile(filename, append(data, []byte("scope: {base: HEAD, include: [src]}\n")...), 0o644)
	for name, data := range map[string]string{"composer.json": `{"require":{"drupal/core":"^11"}}`, "composer.lock": `{"packages":[{"name":"drupal/core","version":"11.0.0"}]}`} {
		os.WriteFile(filepath.Join(root, name), []byte(data), 0o644)
	}
	os.Mkdir(filepath.Join(root, "src"), 0o755)
	os.WriteFile(filepath.Join(root, "src/example.php"), []byte("<?php\n"), 0o644)
	for _, args := range [][]string{{"init", "--quiet"}, {"add", "."}, {"-c", "user.name=test", "-c", "user.email=test@example.test", "commit", "--quiet", "-m", "init"}} {
		if output, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("%v %s", err, output)
		}
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	for _, client := range []string{"codex", "cursor", "claude"} {
		name := "Stop"
		if client == "cursor" {
			name = "stop"
		}
		payload, _ := json.Marshal(map[string]any{"hook_event_name": name, "cwd": root, "session_id": "s", "status": "completed"})
		var stdout, stderr bytes.Buffer
		code := RunContext(context.Background(), []string{"hook", "stop", "--client", client}, root, bytes.NewReader(payload), &stdout, &stderr)
		if code != 0 || strings.TrimSpace(stdout.String()) != "{}" || stderr.Len() != 0 {
			t.Fatalf("%s exit %d stdout=%s stderr=%s", client, code, &stdout, &stderr)
		}
	}
}
