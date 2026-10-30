package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAgentDocumentedScopeSelections(t *testing.T) {
	root := project(t)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	git := func(args ...string) {
		t.Helper()
		command := exec.Command("git", append([]string{"-C", root, "-c", "core.hooksPath=" + os.DevNull, "-c", "commit.gpgSign=false", "-c", "user.name=Fixture", "-c", "user.email=fixture@localhost"}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, output)
		}
	}
	write := func(name, data string) {
		t.Helper()
		filename := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	selected, clean := "src/task/Selected.php", "src/task/Clean.php"
	other, added := "src/unrelated/Other.php", "src/task/New.php"
	for _, name := range []string{selected, clean, other} {
		write(name, "<?php\necho 'base';\n")
	}
	git("init", "--quiet", "--initial-branch=main")
	git("add", "--all")
	git("commit", "--quiet", "-m", "Base")
	write(selected, "<?php\necho 'task change';\n")
	git("add", selected)
	git("commit", "--quiet", "-m", "Task")
	write(other, "<?php\necho 'unrelated local change';\n")
	write(added, "<?php\necho 'new task file';\n")
	for _, test := range []struct {
		name, configMode, resultMode string
		args                         []string
		files                        []string
		exit                         int
	}{
		{"one", "diff", "paths", []string{selected}, []string{selected}, 0},
		{"clean-file", "diff", "paths", []string{clean}, []string{clean}, 0},
		{"multiple", "diff", "paths", []string{selected, added}, []string{added, selected}, 0},
		{"directory", "diff", "paths", []string{"src/task"}, []string{clean, added, selected}, 0},
		{"commits-with-local", "diff", "diff", []string{"--commits", "1"}, []string{added, selected, other}, 0},
		{"full-config-paths", "full", "paths", []string{selected}, []string{selected}, 0},
		{"full-config-commits", "full", "diff", []string{"--mode", "diff", "--commits", "1"}, []string{added, selected, other}, 0},
		{"full", "diff", "full", []string{"--mode", "full"}, []string{clean, added, selected, other}, 0},
		{"paths-with-mode", "diff", "", []string{selected, "--mode", "diff"}, nil, 2},
		{"paths-with-commits", "diff", "", []string{selected, "--commits", "1"}, nil, 2},
		{"missing-file", "diff", "", []string{"src/missing.php"}, nil, 2},
		{"full-config-needs-diff", "full", "", []string{"--commits", "1"}, nil, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			write(".phpwarden.yaml", "version: 1\nprofile: drupal\nphp:\n  runtime: '8.3'\n  target: {min: '8.3', max: '8.3'}\nscope:\n  include: [src]\n  mode: "+test.configMode+"\n")
			args := append([]string{"scope", "--format", "json"}, test.args...)
			var stdout, stderr bytes.Buffer
			code := Run(args, root, &stdout, &stderr)
			if code != test.exit {
				t.Fatalf("exit %d, expected %d: %s %s", code, test.exit, &stdout, &stderr)
			}
			var result response
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if code != 0 {
				if result.Valid || result.ExitCode != code {
					t.Fatal("invalid scope was reported valid", &stdout)
				}
				return
			}
			if result.Scope == nil || result.Scope.Mode != test.resultMode {
				t.Fatalf("wrong scope mode: %s", &stdout)
			}
			names := slices.Clone(result.Scope.Files)
			slices.Sort(names)
			slices.Sort(test.files)
			if !slices.Equal(names, test.files) {
				t.Fatalf("unexpected selection: %v, want %v", names, test.files)
			}
			if test.resultMode == "paths" && slices.Contains(names, other) {
				t.Fatal("task paths included unrelated working-tree edits")
			}
		})
	}
	data, _ := os.ReadFile(filepath.Join(root, other))
	if !strings.Contains(string(data), "unrelated local change") {
		t.Fatal("scope inspection modified unrelated edits")
	}
}
