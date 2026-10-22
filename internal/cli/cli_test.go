package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func project(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	data := "version: 1\nprofile: drupal\nphp:\n  runtime: \"8.3\"\n  target: {min: \"8.3\", max: \"8.3\"}\n"
	if err := os.WriteFile(filepath.Join(root, ".phpwarden.yaml"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestConfigCommands(t *testing.T) {
	root := project(t)
	for _, command := range []string{"validate", "show"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run([]string{"--project", root, "config", command, "--format=json"}, root, &stdout, &stderr)
			if code != 0 || stderr.Len() != 0 {
				t.Fatalf("exit %d stderr=%s stdout=%s", code, &stderr, &stdout)
			}
			var value response
			if err := json.Unmarshal(stdout.Bytes(), &value); err != nil {
				t.Fatalf("stdout must be one JSON document: %v", err)
			}
			if !value.Valid || value.Version != 1 || value.Location.Root != root {
				t.Fatalf("wrong response: %+v", value)
			}
			if command == "show" && (value.Effective == nil || value.Effective.Provenance["php.runtime"] != "project_config") {
				t.Fatalf("missing effective configuration: %+v", value)
			}
		})
	}
}

func TestUsageErrorsNeverReturnSuccess(t *testing.T) {
	cases := [][]string{
		{"check"}, {"init"}, {"config"}, {"config", "shwo"}, {"config", "show", "extra"},
		{"config", "show", "--project"}, {"config", "show", "--config="},
		{"config", "show", "--config", "--help"}, {"config", "show", "--only", "phpstna"},
		{"config", "show", "--format", "xml"}, {"config", "show", "--format", "json", "--format", "text"},
		{"--version", "config", "show"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run(args, t.TempDir(), &stdout, &stderr); code != 2 {
				t.Fatalf("expected exit 2, got %d (%s%s)", code, &stdout, &stderr)
			}
		})
	}
}

func TestJSONInputErrors(t *testing.T) {
	root := project(t)
	if err := os.WriteFile(filepath.Join(root, ".phpwarden.yaml"), []byte("profile: laravel\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"config", "validate", "--project", root, "--format", "json"}, root, &stdout, &stderr)
	var value response
	if err := json.Unmarshal(stdout.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if code != 2 || value.Valid || value.ExitCode != 2 || len(value.Diagnostics) != 1 || stderr.Len() != 0 {
		t.Fatalf("wrong JSON error: exit=%d response=%+v stderr=%s", code, value, &stderr)
	}
}

func TestHelpAndVersionDoNotNeedConfig(t *testing.T) {
	for _, args := range [][]string{nil, {"--help"}, {"--version"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, t.TempDir(), &stdout, &stderr); code != 0 || stdout.Len() == 0 || stderr.Len() != 0 {
			t.Fatalf("unexpected help/version output: exit=%d stdout=%s stderr=%s", code, &stdout, &stderr)
		}
	}
}
