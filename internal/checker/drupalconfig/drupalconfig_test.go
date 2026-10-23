package drupalconfig

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/scope"
)

func settings() config.DrupalConfig {
	return config.DrupalConfig{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 5, ForbiddenKeys: []string{"uuid", "_core"}}
}

func check(t *testing.T, data string, cfg config.DrupalConfig) checker.Outcome {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "config.yml"), []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	selection := scope.Selection{Files: []string{"config.yml"}, LineMap: map[string][]scope.Range{"config.yml": {}}}
	return Run(context.Background(), root, selection, cfg)
}

func TestStructuredTopLevelKeysIgnoreDiffHunks(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		lines      []int
	}{
		{"block", "uuid: value\n_core: {default_config_hash: x}\n", []int{1, 2}},
		{"quoted flow", "# header\n{\"uuid\": value, '_core': {default_config_hash: x}}\n", []int{2, 2}},
		{"escaped key", "\"\\u0075uid\": value\n", []int{1}},
		{"tagged key", "status: true\n!label uuid: value\n", []int{2}},
		{"aliased key", "base: &name uuid\n? *name\n: value\n", []int{2}},
		{"BOM CRLF indentation", "\ufeff  uuid: null\r\n  _core: !tag value\r\n", []int{1, 2}},
		{"merge", "base: &base\n  uuid: inherited\n<<: *base\n", []int{3}},
		{"merge sequence", "a: &a {uuid: one}\nb: &b {_core: {hash: two}}\n<<: [*a, *b]\n", []int{3, 3}},
		{"explicit override", "base: &base {uuid: inherited}\n<<: *base\nuuid: local\n", []int{3}},
		{"numeric neighbor", "1: first\nuuid: value\n", []int{2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := check(t, tc.data, settings())
			var lines []int
			for _, finding := range result.Findings {
				lines = append(lines, *finding.Line)
				if finding.Kind != "code" || finding.Checker != "drupal-config" || finding.Rule != "drupal.config.forbidden_key" || *finding.Column < 1 {
					t.Fatalf("incorrect finding: %+v", finding)
				}
			}
			if result.Check.Status != "failed" || !slices.Equal(lines, tc.lines) || len(result.Diagnostics) != 0 {
				t.Fatalf("top-level keys lost: %+v", result)
			}
		})
	}
}

func TestNestedKeysAndTextAreNotTopLevelKeys(t *testing.T) {
	for _, data := range []string{
		"# uuid: comment\nvalue: 'uuid: text'\ntext: |\n  _core: literal\nnested: {uuid: allowed, _core: allowed}\n",
		"base: &base {uuid: nested}\ncopy: *base\n",
		"'<<': {uuid: nested}\n",
		"'uuid:': literal key\n",
		"- {uuid: nested}\n",
		"!php/object 'O:8:\"stdClass\":0:{}'\n",
		"", "# comment only\n", "---\n", "null\n",
	} {
		if result := check(t, data, settings()); result.Check.Status != "passed" || len(result.Findings)+len(result.Diagnostics) != 0 {
			t.Fatalf("non-top-level key was flagged: %+v", result)
		}
	}
	cfg := settings()
	cfg.ForbiddenKeys = []string{"status"}
	if result := check(t, "uuid: allowed\nstatus: true\n", cfg); len(result.Findings) != 1 || *result.Findings[0].Line != 2 {
		t.Fatalf("custom keys did not replace defaults: %+v", result)
	}
}

func TestMalformedInputsDoNotPassSemanticAnalysis(t *testing.T) {
	for name, data := range map[string]string{
		"parse": "uuid: [broken\n", "duplicate": "uuid: first\nuuid: second\n",
		"documents": "---\nuuid: first\n---\nuuid: second\n", "encoding": "name: \xff\n",
		"undefined alias": "<<: *missing\n", "alias cycle": "a: &a [*a]\n", "invalid merge": "<<: value\n",
		"depth": strings.Repeat("[", 130) + "value" + strings.Repeat("]", 130),
	} {
		t.Run(name, func(t *testing.T) {
			cfg := settings()
			cfg.FailOn = "never"
			result := check(t, data, cfg)
			if result.Check.Status != "error" || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "yaml_invalid" || len(result.Findings) != 0 {
				t.Fatalf("invalid input produced a clean analysis: %+v", result)
			}
		})
	}
}

func TestExecutionPolicies(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "first.yml"), []byte("uuid: old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	selection := scope.Selection{Files: []string{"first.yml", "missing.yml"}, LineMap: map[string][]scope.Range{"first.yml": {}, "missing.yml": nil}}
	for _, required := range []bool{false, true} {
		cfg := settings()
		cfg.Required = required
		outcome := Run(context.Background(), root, selection, cfg)
		result := report.New()
		result.Checks, result.Findings, result.Diagnostics = []report.Check{outcome.Check}, outcome.Findings, outcome.Diagnostics
		result.Finalize()
		want := 1
		if required {
			want = 3
		}
		if result.Complete || result.ExitCode != want || len(result.Findings) != 1 || outcome.Diagnostics[0].Code != "input_read" {
			t.Fatalf("partial failure lost known findings: %+v", result)
		}
	}
	selection.Files = []string{"first.yml"}
	cfg := settings()
	cfg.FailOn = "never"
	if result := Run(context.Background(), root, selection, cfg); result.Check.Status != "passed" || len(result.Findings) != 1 {
		t.Fatalf("nonblocking findings were lost: %+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := Run(ctx, root, selection, cfg); result.Check.Status != "error" || result.Diagnostics[0].Code != "checker_timeout" {
		t.Fatalf("cancelled analysis passed: %+v", result)
	}
	cfg.Enabled = false
	if result := Run(ctx, root, selection, cfg); result.Check.Status != "skipped" || *result.Check.Reason != "disabled" {
		t.Fatalf("disabled checker ran: %+v", result)
	}
	cfg.Enabled, cfg.ForbiddenKeys = true, []string{}
	if result := Run(ctx, root, selection, cfg); result.Check.Status != "skipped" || *result.Check.Reason != "rules_disabled" {
		t.Fatalf("empty rules did not skip: %+v", result)
	}
}
