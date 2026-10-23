package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/report"
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
		{"scope", "--fix"}, {"config", "show", "--fix"}, {"check", "--fix", "--fix"}, {"--version", "--fix"},
		{"check"}, {"init"}, {"config"}, {"config", "shwo"}, {"config", "show", "extra"},
		{"config", "show", "--project"}, {"config", "show", "--config="},
		{"config", "show", "--config", "--help"}, {"config", "show", "--only", "phpstna"},
		{"config", "show", "--format", "xml"}, {"config", "show", "--format", "json", "--format", "text"},
		{"--version", "config", "show"}, {"check", "--color", "purple"}, {"config", "show", "--format", "compact"},
		{"scope", "--format", "compact"}, {"init", "--only", "debug"}, {"init", "--mode", "full"}, {"init", "--config", "x.yaml"},
		{"init", "--format", "compact"}, {"init", "--php-runtime", "8"}, {"init", "--php-target", "8.3.1"}, {"check", "--php-runtime", "8.3"},
		{"init", "extra"}, {"check", "--commits", "0"}, {"check", "--commits", "x"}, {"check", "--commits", "2", "--base", "main"},
		{"check", "--commits", "2", "--mode", "full"}, {"check", "--commits", "1", "demo.php"}, {"config", "show", "--commits", "1"},
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

func TestResolveDoesNotInvokeDockerAndRejectsScopeFlags(t *testing.T) {
	root := project(t)
	filename := filepath.Join(root, ".phpwarden.yaml")
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, bytes.ReplaceAll(data, []byte("8.3"), []byte("8.4")), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := Run([]string{"resolve", "--project", root, "--format", "json"}, root, &stdout, &stderr)
	want := 0
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		want = 2
	}
	if code != want || stderr.Len() != 0 {
		t.Fatalf("offline resolve failed: %d %s %s", code, &stdout, &stderr)
	}
	var result response
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code == 0 && (result.Selection == nil || !result.Selection.Development) {
		t.Fatalf("missing development selection: %+v", result)
	}
	if code == 0 && (result.Selection.Release != "development" || result.Selection.Entry != "drupal-static-dev-php8.4" ||
		result.Selection.Preset.ID != "drupal-static" || result.Selection.Tools["php"] != "8.4.26") {
		t.Fatalf("missing bundled manifest provenance: %+v", result.Selection)
	}
	for _, args := range [][]string{{"resolve", "--base", "main"}, {"resolve", "--only", "debug"}} {
		stdout.Reset()
		stderr.Reset()
		if code := Run(args, root, &stdout, &stderr); code != 2 {
			t.Fatalf("unsupported resolve options accepted: %v", args)
		}
	}
}

func TestWorkerExecuteRejectsInvalidStdinRequests(t *testing.T) {
	for _, input := range []string{"", "{}", `{"unknown":1}`, "{} {}"} {
		var stdout, stderr bytes.Buffer
		code := RunContext(context.Background(), []string{"worker", "execute"}, t.TempDir(), strings.NewReader(input), &stdout, &stderr)
		var result report.Report
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("missing JSON error for %q: %v: %s", input, err, &stderr)
		}
		if code != 2 && code != 3 || result.Complete || result.ExitCode != code || result.Verdict != "error" {
			t.Fatalf("invalid request accepted: %q: %+v", input, result)
		}
	}
}

func TestGlobalYAMLFailureIsVisibleInText(t *testing.T) {
	result := report.New()
	result.Checks = []report.Check{{ID: "yaml", Status: "failed", Required: true, FailOn: "error"}}
	result.Findings = []report.Finding{{Checker: "yaml", Rule: "yaml.syntax", Kind: "global", Severity: "error", Message: `YAML file "config.yml": invalid UTF-8`}}
	result.Finalize()
	var stdout, stderr bytes.Buffer
	if code := writeReport(&stdout, &stderr, "text", "never", result); code != 1 || !strings.Contains(stdout.String(), "  config.yml\n") || !strings.Contains(stdout.String(), "invalid UTF-8") || stderr.Len() != 0 {
		t.Fatalf("global finding is not visible: exit=%d %s %s", code, &stdout, &stderr)
	}
}

func TestInitWritesConfigurationOnce(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("development images exist only for linux/amd64")
	}
	root := t.TempDir()
	for name, data := range map[string]string{
		"composer.json":           `{"require":{"drupal/core":"^11"},"config":{"platform":{"php":"8.2.0"}}}`,
		"composer.lock":           `{"packages":[{"name":"drupal/core","version":"11.3.16","require":{"php":">=8.1"}}]}`,
		".ddev/config.yaml":       "php_version: \"8.3\"\n",
		"web/core/lib/Drupal.php": "<?php\n",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", "--project", root, "--format", "json"}, t.TempDir(), &stdout, &stderr); code != 0 || stderr.Len() != 0 {
		t.Fatalf("init failed: exit=%d %s %s", code, &stdout, &stderr)
	}
	var value response
	if err := json.Unmarshal(stdout.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if !value.Valid || value.Init == nil || !value.Init.Created || value.Init.Runtime.Version != "8.3" || value.Init.Target.Version != "8.2" {
		t.Fatalf("wrong init response: %s", &stdout)
	}
	stdout.Reset()
	if code := Run([]string{"init"}, root, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "already exists") {
		t.Fatalf("second init: exit=%d %s %s", code, &stdout, &stderr)
	}
	stdout.Reset()
	if code := Run([]string{"config", "validate", "--project", root}, root, &stdout, &stderr); code != 0 {
		t.Fatalf("generated configuration is invalid: %s %s", &stdout, &stderr)
	}
}
