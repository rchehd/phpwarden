package agenthook

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func put(t *testing.T, root, name, data string) string {
	t.Helper()
	filename := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func TestInstallMergesPreservesAndRelocates(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	for _, client := range []string{"codex", "claude", "cursor"} {
		t.Run(client, func(t *testing.T) {
			root := t.TempDir()
			foreign := `{"custom":{"enabled":true},"hooks":{"Stop":[{"hooks":[{"type":"command","command":"existing-command"}]}]}}`
			if client == "cursor" {
				foreign = `{"version":1,"custom":{"enabled":true},"hooks":{"stop":[{"type":"command","command":"existing-command"}]}}`
			}
			filename := put(t, root, settingsPath(client), foreign)
			for attempt, executable := range []string{"/tmp/path with spaces/phpwarden", "/tmp/path with spaces/phpwarden", "/tmp/a'b/phpwarden"} {
				plan, err := Prepare(root, executable, []string{client})
				if err != nil {
					t.Fatal(err)
				}
				rows, err := plan.Install(context.Background())
				if err != nil || len(rows) != 1 || rows[0].Changed != (attempt != 1) {
					t.Fatalf("%+v %v", rows, err)
				}
				data, _ := os.ReadFile(filename)
				var value map[string]any
				if json.Unmarshal(data, &value) != nil || value["custom"] == nil || strings.Count(string(data), "existing-command") != 1 || strings.Count(string(data), "--managed") != 1 {
					t.Fatalf("foreign settings lost: %s", data)
				}
				if attempt == 2 && !strings.Contains(string(data), `a'\"'\"'b`) {
					t.Fatalf("executable not shell quoted: %s", data)
				}
			}
		})
	}
}

func TestInstallRejectsAmbiguousAndUnsafeSettingsBeforeWriting(t *testing.T) {
	for _, raw := range []string{`[]`, `null`, `{"hooks":[],"hooks":{}}`, `{"hooks":{"Stop":{}}}`, `{"hooks":{"Stop":[{}]}}`, `{"hooks":{}} {}`, `{"hooks":{"Stop":[{"hooks":[{"command":"x hook stop --client codex --managed","async":true}]}]}}`} {
		root := t.TempDir()
		filename := put(t, root, settingsPath("codex"), raw)
		if _, err := Prepare(root, "/bin/phpwarden", []string{"codex", "claude"}); err == nil {
			t.Fatalf("accepted %s", raw)
		}
		if data, _ := os.ReadFile(filename); string(data) != raw {
			t.Fatal("foreign settings changed")
		}
		if _, err := os.Stat(filepath.Join(root, ".claude")); !os.IsNotExist(err) {
			t.Fatal("preflight wrote files")
		}
	}
	for _, parent := range []bool{false, true} {
		root, outside := t.TempDir(), t.TempDir()
		name, target := filepath.Join(root, ".codex"), outside
		if !parent {
			name = put(t, root, ".codex/hooks.json", `{}`)
			os.Remove(name)
			target = put(t, outside, "hooks.json", `{}`)
		}
		if err := os.Symlink(target, name); err != nil {
			t.Fatal(err)
		}
		if _, err := Prepare(root, "/bin/phpwarden", []string{"codex"}); err == nil {
			t.Fatal("symlink accepted")
		}
	}
}

func TestConcurrentInstallAndCancelledInstall(t *testing.T) {
	root := t.TempDir()
	plan, err := Prepare(root, "/bin/phpwarden", []string{"codex", "cursor", "claude"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := plan.Install(ctx); err == nil {
		t.Fatal("cancelled install succeeded")
	}
	if entries, _ := os.ReadDir(root); len(entries) != 0 {
		t.Fatal("cancelled install wrote files")
	}
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			if _, err := plan.Install(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	group.Wait()
	for _, client := range []string{"codex", "cursor", "claude"} {
		data, err := os.ReadFile(filepath.Join(root, settingsPath(client)))
		if err != nil || strings.Count(string(data), "--managed") != 1 {
			t.Fatalf("duplicate or missing hook: %s %v", data, err)
		}
	}
}
