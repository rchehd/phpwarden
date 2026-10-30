package cli

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/agentskills"
)

func TestPerformanceSkillConnectionCommandsRespectMissingSetup(t *testing.T) {
	reference, err := fs.ReadFile(agentskills.Resources(), "skills/phpwarden-performance/references/connection.md")
	if err != nil {
		t.Fatal(err)
	}
	project, state, cache, private := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("CLAUDE_CONFIG_DIR", private)
	t.Setenv("PATH", t.TempDir())
	for _, name := range []string{"AGENTS.md", ".phpwarden.yaml"} {
		if err := os.WriteFile(filepath.Join(project, name), []byte("project-owned sentinel"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commands := regexp.MustCompile(`(?m)^phpwarden performance (?:status|connect|disconnect) [^\n]+$`).FindAllString(string(reference), -1)
	if len(commands) != 6 {
		t.Fatalf("expected six concrete default/custom-state examples, got %d", len(commands))
	}
	for _, client := range []string{"codex", "cursor", "claude"} {
		for _, command := range commands {
			t.Run(client+"/"+command, func(t *testing.T) {
				args := strings.Fields(command)[1:]
				for i, value := range args {
					switch value {
					case "PROJECT":
						args[i] = project
					case "STATE":
						args[i] = state
					case "codex":
						args[i] = client
					}
				}
				var stdout, stderr bytes.Buffer
				code := Run(args, project, &stdout, &stderr)
				var result struct {
					Command, State string
					ExitCode       int `json:"exit_code"`
					Diagnostics    []string
				}
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.ExitCode != code || stderr.Len() != 0 {
					t.Fatalf("invalid setup result: %d %s %s (%v)", code, &stdout, &stderr, err)
				}
				if args[1] == "status" {
					if code != 0 || result.State != "absent" {
						t.Fatalf("missing setup was not reported absent: %s", &stdout)
					}
				} else if code != 2 || !strings.Contains(strings.Join(result.Diagnostics, " "), "private performance state is unavailable") {
					t.Fatalf("example did not reach the saved-setup prerequisite: %s", &stdout)
				}
			})
		}
	}
	for _, directory := range []string{cache, state, private} {
		entries, err := os.ReadDir(directory)
		if err != nil || len(entries) != 0 {
			t.Fatalf("missing setup created runtime/client state: %s %v", directory, err)
		}
	}
	entries, err := os.ReadDir(project)
	if err != nil || len(entries) != 2 {
		t.Fatal("missing setup wrote project configuration", err)
	}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(project, entry.Name()))
		if err != nil || string(data) != "project-owned sentinel" {
			t.Fatal("project instructions or config changed", err)
		}
	}
}
