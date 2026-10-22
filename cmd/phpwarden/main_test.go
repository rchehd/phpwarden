package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestBuiltCLI(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "phpwarden")
	build := exec.Command("go", "build", "-o", binary, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v: %s", err, output)
	}
	project := filepath.Join(root, "project with spaces")
	if err := os.Mkdir(project, 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(project, ".phpwarden.yaml")
	data := "version: 1\nprofile: drupal\nphp:\n  runtime: \"8.3\"\n  target: {min: \"8.3\", max: \"8.3\"}\n"
	if err := os.WriteFile(configPath, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, testcase := range []struct {
		name string
		args []string
		code int
	}{
		{"valid", []string{"config", "validate", "--config", configPath, "--format", "json"}, 0},
		{"show relative config", []string{"config", "show", "--config", ".phpwarden.yaml", "--format", "json"}, 0},
		{"missing config", []string{"config", "validate", "--config", "missing.yaml", "--format", "json"}, 2},
		{"unimplemented check", []string{"check", "--format", "json"}, 2},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			command := exec.Command(binary, testcase.args...)
			command.Dir = project
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			err := command.Run()
			code := 0
			if err != nil {
				var exit *exec.ExitError
				if !errors.As(err, &exit) {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			var result struct {
				Valid    bool `json:"valid"`
				ExitCode int  `json:"exit_code"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("stdout must contain one JSON document: %v: %s", err, &stdout)
			}
			if code != testcase.code || result.ExitCode != code || result.Valid != (code == 0) || stderr.Len() != 0 {
				t.Fatalf("exit=%d result=%+v stderr=%s", code, result, &stderr)
			}
		})
	}
}
