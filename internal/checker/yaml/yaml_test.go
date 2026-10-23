package yamlcheck

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

type processFunc func(context.Context, runner.Command) runner.Result

func (f processFunc) Run(ctx context.Context, command runner.Command) runner.Result {
	return f(ctx, command)
}

func settings() config.YAML {
	return config.YAML{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 5}
}

func fakeAutoload(t *testing.T) string {
	t.Helper()
	filename := filepath.Join(t.TempDir(), "autoload.php")
	if err := os.WriteFile(filename, []byte("<?php"), 0o600); err != nil {
		t.Fatal(err)
	}
	return filename
}

func response(findings string) []byte {
	return []byte(`{"version":1,"runtime":"8.4","php_version":"8.4.26","yaml_version":"7.4.21","findings":` + findings + `}`)
}

func TestYAMLFindingsIncludeUnchangedLinesAndMissingPositions(t *testing.T) {
	selection := scope.Selection{Files: []string{"config.yml", "odd \nfile.yaml"}, LineMap: map[string][]scope.Range{
		"config.yml": {}, "odd \nfile.yaml": {{Start: 20, End: 20}},
	}}
	rows, _ := json.Marshal([]any{map[string]any{"path": "config.yml", "line": 2, "message": "duplicate key"}, map[string]any{"path": "odd \nfile.yaml", "line": nil, "message": "invalid UTF-8"}})
	for _, policy := range []string{"error", "never"} {
		cfg := settings()
		cfg.FailOn = policy
		autoload := fakeAutoload(t)
		var script string
		process := processFunc(func(_ context.Context, command runner.Command) runner.Result {
			if command.Name != "php" || command.Args[0] != "-n" || command.Args[len(command.Args)-1] != autoload {
				t.Fatalf("unsafe YAML invocation: %+v", command)
			}
			script = command.Args[len(command.Args)-2]
			if _, err := os.Stat(script); err != nil {
				t.Fatal(err)
			}
			return runner.Result{Stdout: response(string(rows))}
		})
		result := run(context.Background(), t.TempDir(), selection, cfg, autoload, process)
		want := "failed"
		if policy == "never" {
			want = "passed"
		}
		if result.Check.Status != want || len(result.Findings) != 2 || result.Findings[1].Kind != "global" ||
			result.Findings[1].Path != nil || result.Findings[1].Line != nil || *result.Findings[0].Line != 2 ||
			result.Runtime == nil || *result.Runtime != "8.4" {
			t.Fatalf("whole-file findings were lost: %+v", result)
		}
		if _, err := os.Stat(filepath.Dir(script)); !os.IsNotExist(err) {
			t.Fatal("helper directory was not removed")
		}
	}
}

func TestYAMLFailuresCannotPass(t *testing.T) {
	for _, tc := range []struct {
		name      string
		execution runner.Result
		status    string
	}{
		{"missing PHP", runner.Result{Unavailable: true}, "unavailable"},
		{"timeout", runner.Result{TimedOut: true}, "error"},
		{"crash", runner.Result{ExitCode: 70}, "error"},
		{"invalid JSON", runner.Result{Stdout: []byte("{")}, "error"},
		{"missing fields", runner.Result{Stdout: []byte("{}")}, "error"},
		{"extra fields", runner.Result{Stdout: []byte(strings.Replace(string(response("[]")), `"version":1`, `"version":1,"extra":true`, 1))}, "error"},
		{"trailing document", runner.Result{Stdout: append(response("[]"), []byte(" {}")...)}, "error"},
		{"unexpected stderr", runner.Result{Stdout: response("[]"), Stderr: []byte("warning")}, "error"},
		{"wrong Symfony", runner.Result{Stdout: []byte(strings.Replace(string(response("[]")), Version, "7.4.20", 1))}, "error"},
		{"wrong PHP minor", runner.Result{Stdout: []byte(strings.Replace(string(response("[]")), `"runtime":"8.4"`, `"runtime":"8"`, 1))}, "error"},
		{"outside file", runner.Result{Stdout: response(`[{"path":"../other.yaml","line":2,"message":"bad"}]`)}, "error"},
		{"zero line", runner.Result{Stdout: response(`[{"path":"config.yml","line":0,"message":"bad"}]`)}, "error"},
		{"empty message", runner.Result{Stdout: response(`[{"path":"config.yml","line":2,"message":""}]`)}, "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := run(context.Background(), t.TempDir(), scope.Selection{Files: []string{"config.yml"}, LineMap: map[string][]scope.Range{"config.yml": nil}}, settings(), fakeAutoload(t),
				processFunc(func(context.Context, runner.Command) runner.Result { return tc.execution }))
			if result.Check.Status != tc.status || len(result.Diagnostics) != 1 || len(result.Findings) != 0 {
				t.Fatalf("failure hidden: %+v", result)
			}
		})
	}
}

func TestYAMLApplicabilityAndMissingBundledDependencies(t *testing.T) {
	unused := processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("PHP must not run")
		return runner.Result{}
	})
	selection := scope.Selection{Files: []string{"config.yml"}}
	for _, cfg := range []config.YAML{{}, settings()} {
		selected := selection
		if cfg.Enabled {
			selected.Files = nil
		}
		result := run(context.Background(), t.TempDir(), selected, cfg, "", unused)
		if result.Check.Status != "skipped" {
			t.Fatalf("wrong applicability: %+v", result)
		}
	}
	result := run(context.Background(), t.TempDir(), selection, settings(), filepath.Join(t.TempDir(), "missing.php"), unused)
	if result.Check.Status != "unavailable" || *result.Check.Reason != "dependencies_missing" {
		t.Fatalf("missing tool dependencies hidden: %+v", result)
	}
}

func TestActualSymfonyHandlesTagsAliasesAndErrorsWithoutProjectAutoload(t *testing.T) {
	if _, err := exec.LookPath("php"); err != nil {
		t.Skip("PHP is not installed")
	}
	autoload, err := filepath.Abs("../../../tools/yaml/vendor/autoload.php")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(autoload); err != nil {
		t.Fatal("install tools/yaml dependencies before running native YAML tests")
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{
		"valid.yml":           []byte("defaults: &base\n  enabled: true\nservice:\n  <<: *base\n  args: !tagged_iterator service.tag\n  value: !php/const Unknown::VALUE\nobject: !php/object 'O:8:\"stdClass\":0:{}'\n"),
		"duplicate.yml":       []byte("name: first\nname: second\n"),
		"invalid.yaml":        []byte("items: [broken\n"),
		"odd \nfile.yaml":     []byte("key: \xff\n"),
		"vendor/autoload.php": []byte("<?php file_put_contents(__DIR__ . '/../executed', 'bad');"),
	}
	for name, source := range files {
		if err := os.WriteFile(filepath.Join(root, name), source, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	selection := scope.Selection{Files: []string{"valid.yml", "duplicate.yml", "invalid.yaml", "odd \nfile.yaml"}, LineMap: map[string][]scope.Range{
		"valid.yml": nil, "duplicate.yml": {}, "invalid.yaml": nil, "odd \nfile.yaml": nil,
	}}
	var group sync.WaitGroup
	for range 2 {
		group.Go(func() {
			result := run(context.Background(), root, selection, settings(), autoload, runner.Exec{})
			if result.Check.Status != "failed" || len(result.Findings) != 3 || len(result.Diagnostics) != 0 {
				t.Errorf("actual Symfony failed: %+v", result)
			}
		})
	}
	group.Wait()
	if _, err := os.Stat(filepath.Join(root, "executed")); !os.IsNotExist(err) {
		t.Fatal("project autoload executed")
	}
}
