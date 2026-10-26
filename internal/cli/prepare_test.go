package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rchehd/phpwarden/internal/launcher"
)

func TestPrepareRejectsAnalysisOptionsAndPaths(t *testing.T) {
	for _, args := range [][]string{
		{"prepare", "src"}, {"prepare", "--only=syntax"}, {"prepare", "--fix"}, {"prepare", "--all"},
		{"prepare", "--mode=full"}, {"prepare", "--commits=2"}, {"prepare", "--base=main"},
		{"prepare", "--whole-files"}, {"prepare", "--no-cache"}, {"prepare", "--pre-commit"},
		{"prepare", "--agents=all"}, {"prepare", "--hooks=all"}, {"prepare", "--no-interaction"},
		{"prepare", "--php-runtime=8.4"}, {"prepare", "--php-target=8.3"}, {"prepare", "--composer-root=app"},
		{"prepare", "--security"}, {"prepare", "--direct"}, {"prepare", "--no-dev"}, {"prepare", "--offline"},
		{"prepare", "--runtime-version=8.4.26"}, {"prepare", "--format=compact"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Run(args, t.TempDir(), &stdout, &stderr); code != 2 {
				t.Fatalf("%d %s %s", code, &stdout, &stderr)
			}
		})
	}
}

func TestPrepareInputErrorsAndDockerFailuresAreSingleJSONDocuments(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DOCKER_HOST", "")
	t.Setenv("DOCKER_CONTEXT", "")
	for _, test := range []struct {
		name string
		code int
	}{{"no config", 2}, {"invalid config", 2}, {"docker missing", 3}, {"scope refs irrelevant", 3}, {"explicit config", 3}} {
		t.Run(test.name, func(t *testing.T) {
			root := project(t)
			args := []string{"prepare", "--project", root, "--format=json"}
			switch test.name {
			case "no config":
				os.Remove(filepath.Join(root, ".phpwarden.yaml"))
			case "invalid config":
				os.WriteFile(filepath.Join(root, ".phpwarden.yaml"), []byte("bad: yaml"), 0o644)
			case "scope refs irrelevant":
				file, _ := os.OpenFile(filepath.Join(root, ".phpwarden.yaml"), os.O_APPEND|os.O_WRONLY, 0o644)
				file.WriteString("scope: {base: definitely-missing-ref}\n")
				file.Close()
			case "explicit config":
				os.Rename(filepath.Join(root, ".phpwarden.yaml"), filepath.Join(root, "quality.yaml"))
				args = []string{"prepare", "--config", "quality.yaml", "--format=json"}
			}
			var stdout, stderr bytes.Buffer
			code := Run(args, root, &stdout, &stderr)
			var result launcher.Preparation
			if code != test.code || json.Unmarshal(stdout.Bytes(), &result) != nil || result.Command != "prepare" || result.Ready || result.ExitCode != code || len(result.Diagnostics) != 1 || stderr.Len() != 0 {
				t.Fatalf("%d %s %s", code, &stdout, &stderr)
			}
		})
	}
}

func TestPrepareCancellationDoesNotRequireConfigOrDocker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	code := RunContext(ctx, []string{"prepare", "--format=json"}, t.TempDir(), strings.NewReader(""), &stdout, &stderr)
	var result launcher.Preparation
	if code != 130 || json.Unmarshal(stdout.Bytes(), &result) != nil || result.Ready || result.ExitCode != 130 || stderr.Len() != 0 {
		t.Fatalf("%d %s %s", code, &stdout, &stderr)
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	stdout.Reset()
	code = RunContext(ctx, []string{"prepare", "--format=json"}, t.TempDir(), strings.NewReader(""), &stdout, &stderr)
	if code != 3 || json.Unmarshal(stdout.Bytes(), &result) != nil || result.Ready || result.Diagnostics[0].Code != "prepare_timeout" {
		t.Fatalf("%d %s %s", code, &stdout, &stderr)
	}
}
