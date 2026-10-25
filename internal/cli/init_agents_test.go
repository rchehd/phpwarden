package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/projectinit"
)

func TestInitAgentsCLIInstallsAllClientsWithoutPrompts(t *testing.T) {
	root := project(t)
	before, _ := os.ReadFile(filepath.Join(root, ".phpwarden.yaml"))
	for attempt := range 2 {
		var stdout, stderr bytes.Buffer
		code := RunContext(context.Background(), []string{"init", "--agents", "all", "--no-interaction", "--format", "json"}, root, strings.NewReader(""), &stdout, &stderr)
		if code != 0 || stderr.Len() != 0 {
			t.Fatalf("noninteractive install: %d %s %s", code, &stdout, &stderr)
		}
		var value response
		if err := json.Unmarshal(stdout.Bytes(), &value); err != nil {
			t.Fatal(err)
		}
		if value.Init == nil || value.Init.AgentSkills == nil || value.Init.AgentSkills.Created != (attempt == 0) || !slices.Equal(value.Init.AgentSkills.Clients, []string{"codex", "cursor", "claude"}) {
			t.Fatalf("missing integration state: %s", &stdout)
		}
	}
	after, _ := os.ReadFile(filepath.Join(root, ".phpwarden.yaml"))
	if !bytes.Equal(before, after) {
		t.Fatal("agent installation changed existing config")
	}
}

func TestInitAgentsRequiresExplicitSelectionOutsideTerminals(t *testing.T) {
	for _, flags := range [][]string{{"init", "-n"}, {"init", "--format", "json"}, {"init", "--agents", "none"}} {
		root := project(t)
		var stdout, stderr bytes.Buffer
		if code := RunContext(context.Background(), flags, root, strings.NewReader("all\n"), &stdout, &stderr); code != 0 {
			t.Fatalf("default init: %d %s %s", code, &stdout, &stderr)
		}
		if _, err := os.Stat(filepath.Join(root, ".agents")); !os.IsNotExist(err) {
			t.Fatal("noninteractive init implicitly installed skills")
		}
	}
	for _, flags := range [][]string{{"init", "--agents", "invalid"}, {"init", "--agents", "all", "--agents", "codex"}, {"config", "show", "--agents", "all"}, {"check", "--agents", "all"}, {"hook", "pre-commit", "--agents", "all"}, {"--version", "--agents", "all"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(flags, t.TempDir(), &stdout, &stderr); code != 2 {
			t.Fatalf("invalid flags %v returned %d: %s %s", flags, code, &stdout, &stderr)
		}
	}
}

func TestInitAgentAndCheckPromptsShareBufferedInput(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("invalid\nall\n\n"))
	clients, err := chooseInitAgents(context.Background(), reader, io.Discard)
	if err != nil || !slices.Equal(clients, []string{"codex", "cursor", "claude"}) {
		t.Fatalf("agent prompt: %v %v", clients, err)
	}
	selected, err := chooseInitChecks(context.Background(), reader, io.Discard, []projectinit.CheckChoice{{ID: "deptrac", Enabled: true}})
	if err != nil || !slices.Equal(selected, []string{"deptrac"}) {
		t.Fatalf("check prompt lost buffered input: %v %v", selected, err)
	}
	for _, input := range []string{"", "q\n", "all"} {
		if _, err := chooseInitAgents(context.Background(), bufio.NewReader(strings.NewReader(input)), io.Discard); !errors.Is(err, context.Canceled) {
			t.Fatalf("input %q did not cancel: %v", input, err)
		}
	}
}
