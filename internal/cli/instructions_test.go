package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitInstructionsAndRemovalAreExplicitAndPreserveProject(t *testing.T) {
	root := project(t)
	original := []byte("# User instructions\nKeep these rules.\n")
	if err := os.WriteFile(filepath.Join(root, "AGENTS.md"), original, 0o644); err != nil {
		t.Fatal(err)
	}
	config, _ := os.ReadFile(filepath.Join(root, ".phpwarden.yaml"))
	for attempt := range 2 {
		var stdout, stderr bytes.Buffer
		args := []string{"init", "--agents", "all", "--instructions", "all", "-n", "--format", "json"}
		if code := Run(args, root, &stdout, &stderr); code != 0 {
			t.Fatalf("init instructions: %d %s %s", code, &stdout, &stderr)
		}
		var value response
		if err := json.Unmarshal(stdout.Bytes(), &value); err != nil || value.Init == nil || len(value.Init.AgentInstructions) != 2 {
			t.Fatalf("missing instruction state: %v %s", err, &stdout)
		}
		if value.Init.AgentInstructions[0].Changed != (attempt == 0) {
			t.Fatal("instructions installation is not idempotent")
		}
	}
	for _, name := range []string{".codex/hooks.json", ".cursor/hooks.json", ".claude/settings.json"} {
		if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatal("instructions installed a completion hook", name)
		}
	}
	after, _ := os.ReadFile(filepath.Join(root, ".phpwarden.yaml"))
	if !bytes.Equal(config, after) {
		t.Fatal("changed project configuration")
	}
	if err := os.Remove(filepath.Join(root, ".phpwarden.yaml")); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := []string{"instructions", "remove", "--client", "all", "--project", root, "--format", "json"}
	if code := Run(args, root, &stdout, &stderr); code != 0 {
		t.Fatalf("remove without config: %d %s %s", code, &stdout, &stderr)
	}
	after, _ = os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if !bytes.Equal(original, after) {
		t.Fatal("removal changed user instructions")
	}
	if _, err := os.Stat(filepath.Join(root, ".agents/skills/phpwarden/SKILL.md")); err != nil {
		t.Fatal("removal changed skills", err)
	}
}

func TestInstructionDefaultsInvalidFlagsAndCancellation(t *testing.T) {
	root := project(t)
	for _, args := range [][]string{{"init", "-n"}, {"init", "--format", "json"}, {"init", "--instructions", "none", "-n"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, root, &stdout, &stderr); code != 0 {
			t.Fatalf("default init %v: %d %s", args, code, &stderr)
		}
		if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
			t.Fatal("default init wrote instructions")
		}
	}
	for _, args := range [][]string{
		{"check", "--instructions", "all"}, {"init", "--instructions", "bad"},
		{"init", "--instructions", "all", "--instructions", "none"},
		{"instructions"}, {"instructions", "install"}, {"instructions", "remove"},
		{"instructions", "remove", "--client", "none"},
		{"instructions", "remove", "--client", "all", "--client", "codex"},
		{"instructions", "remove", "--client", "all", "--format", "xml"},
		{"instructions", "remove", "--unknown"}, {"instructions", "remove", "extra"},
	} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, root, &stdout, &stderr); code != 2 {
			t.Fatalf("invalid flags %v: %d %s %s", args, code, &stdout, &stderr)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	if code := RunContext(ctx, []string{"instructions", "remove", "--client", "all", "--project", root, "--format", "json"}, root, strings.NewReader(""), &stdout, &stderr); code != 130 || !strings.Contains(stdout.String(), `"exit_code": 130`) {
		t.Fatalf("cancellation: %d %s %s", code, &stdout, &stderr)
	}
}
