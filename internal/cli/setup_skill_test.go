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

func TestSetupSkillProjectCommandsKeepIntegrationsExplicit(t *testing.T) {
	data, err := fs.ReadFile(agentskills.Resources(), "skills/phpwarden-setup/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	files := map[string]string{
		"composer.json":     `{"require":{"drupal/core":"^11"}}`,
		"composer.lock":     `{"packages":[{"name":"drupal/core","version":"11.4.1","require":{"php":">=8.3"}}]}`,
		".ddev/config.yaml": "name: setup-fixture\nphp_version: '8.3'\n",
		"AGENTS.md":         "project-owned instructions\n",
	}
	for name, content := range files {
		filename := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	commands := regexp.MustCompile(`(?m)^phpwarden (?:init|config show|resolve|doctor) [^\n]+$`).FindAllString(string(data), -1)
	if len(commands) != 4 {
		t.Fatalf("expected four native project examples, got %d", len(commands))
	}
	var config []byte
	for _, command := range commands {
		args := strings.Fields(command)[1:]
		for i, value := range args {
			if value == "PROJECT" {
				args[i] = root
			}
		}
		if args[0] == "init" {
			args = append(args, "--format=json")
		}
		var stdout, stderr bytes.Buffer
		code := Run(args, root, &stdout, &stderr)
		if !json.Valid(stdout.Bytes()) || stderr.Len() != 0 || strings.Contains(stdout.String(), "unknown option") {
			t.Fatalf("documented project command failed: %v (%d) %s %s", args, code, &stdout, &stderr)
		}
		if args[0] == "init" || args[0] == "config" || args[0] == "resolve" {
			if code != 0 {
				t.Fatalf("setup/config/resolve should work without Docker or analyzers: %v (%d) %s", args, code, &stdout)
			}
		} else if code != 3 {
			t.Fatalf("doctor should report missing runtime tools: %d %s", code, &stdout)
		}
		after, err := os.ReadFile(filepath.Join(root, ".phpwarden.yaml"))
		if err != nil {
			t.Fatal("init did not create configuration", err)
		}
		if config == nil {
			config = after
		} else if !bytes.Equal(config, after) {
			t.Fatal("read-only status command changed project configuration")
		}
	}
	for _, name := range []string{".agents", ".claude", ".codex", ".cursor"} {
		if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatal("setup implicitly installed client integration", name, err)
		}
	}
	for name, content := range files {
		data, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(data) != content {
			t.Fatal("setup changed supplied project inputs", name, err)
		}
	}
}
