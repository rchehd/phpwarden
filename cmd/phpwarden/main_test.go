package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/scope"
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
	t.Run("scope through built binary", func(t *testing.T) {
		git := func(args ...string) {
			t.Helper()
			command := exec.Command("git", append([]string{"-C", project}, args...)...)
			for _, variable := range os.Environ() {
				if !strings.HasPrefix(variable, "GIT_") {
					command.Env = append(command.Env, variable)
				}
			}
			command.Env = append(command.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
				"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.invalid",
				"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.invalid")
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("git: %v: %s", err, output)
			}
		}
		if err := os.WriteFile(configPath, []byte(data+"scope: {include: [src], base: main}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		source := filepath.Join(project, "src")
		if err := os.Mkdir(source, 0o755); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(source, "demo.php")
		if err := os.WriteFile(file, []byte("<?php\n\nvar_dump('legacy');\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		git("init", "--quiet", "--initial-branch=main")
		git("add", "--all")
		git("-c", "core.hooksPath="+os.DevNull, "commit", "--quiet", "-m", "baseline")
		if err := os.WriteFile(file, []byte("<?php\n\nvar_dump('legacy');\n$safe = 1;\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, testcase := range []struct {
			name   string
			cwd    string
			args   []string
			code   int
			legacy bool
		}{
			{"diff", project, []string{"scope"}, 0, false},
			{"whole files", project, []string{"scope", "--whole-files"}, 0, true},
			{"full ignores configured base", project, []string{"scope", "--mode", "full"}, 0, true},
			{"relative from subdir", source, []string{"scope", "demo.php"}, 0, true},
			{"invalid combination", project, []string{"scope", "--mode", "full", "--base", "main"}, 2, false},
			{"missing base", project, []string{"scope", "--base", "absent"}, 2, false},
			{"missing path", project, []string{"scope", "missing.php"}, 2, false},
		} {
			t.Run(testcase.name, func(t *testing.T) {
				command := exec.Command(binary, append(testcase.args, "--format", "json")...)
				command.Dir = testcase.cwd
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
					Valid bool            `json:"valid"`
					Scope scope.Selection `json:"scope"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
					t.Fatalf("invalid JSON: %v: %s", err, &stdout)
				}
				if code != testcase.code || result.Valid != (code == 0) || stderr.Len() > 0 {
					t.Fatalf("exit=%d stdout=%s stderr=%s", code, &stdout, &stderr)
				}
				if code == 0 && (!result.Scope.Contains("src/demo.php", 4) || result.Scope.Contains("src/demo.php", 3) != testcase.legacy) {
					t.Fatalf("incorrect reporting scope: %+v", result.Scope)
				}
			})
		}
	})
}
