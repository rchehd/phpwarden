package deptrac

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

const nativeConfig = `deptrac:
  paths: [src]
  layers:
    - name: Domain
      collectors:
        - {type: classLike, value: 'App\\Domain\\.*'}
    - name: Infrastructure
      collectors:
        - {type: classLike, value: 'App\\Infrastructure\\.*'}
  ruleset:
    Domain: []
    Infrastructure: []
`

const source = "<?php\nnamespace App\\Domain;\nclass Service extends \\App\\Infrastructure\\Store {}\n"

func settings(t *testing.T) config.Deptrac {
	t.Helper()
	effective, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := effective.Config.Tools.Deptrac
	cfg.Enabled = true
	return cfg
}

func write(t *testing.T, root, name, text string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func selected(lines []scope.Range) scope.Selection {
	return scope.Selection{Mode: "paths", Files: []string{"src/Domain/Service.php"}, LineMap: map[string][]scope.Range{"src/Domain/Service.php": lines}}
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "deptrac.yaml", nativeConfig)
	write(t, root, "src/Domain/Service.php", source)
	write(t, root, "src/Infrastructure/Store.php", "<?php\nnamespace App\\Infrastructure;\nclass Store {}\n")
	return root
}

type processFunc func(context.Context, runner.Command) runner.Result

func (f processFunc) Run(ctx context.Context, cmd runner.Command) runner.Result { return f(ctx, cmd) }

func nativeReport() []byte {
	data, _ := json.Marshal(response{Version: 1, Runtime: "8.4", PHP: "8.4.26", Tools: map[string]string{"deptrac/deptrac": Version, "phpstan/phpstan": PHPStanVersion}, Files: []string{"src/Domain/Service.php", "src/Infrastructure/Store.php"}, Counts: map[string]int{"violation": 1, "uncovered": 0, "skipped": 0, "allowed": 0}, Errors: new(0), Warnings: []string{}, Findings: []finding{{File: "src/Domain/Service.php", Line: 3, Kind: "violation", Depender: `App\Domain\Service`, Dependent: `App\Infrastructure\Store`, From: "Domain", To: new("Infrastructure")}}})
	return data
}

func TestDeptracFailuresNeverPass(t *testing.T) {
	for _, name := range []string{"valid", "missing PHP", "timeout", "crash", "stderr", "malformed", "trailing", "version", "partial", "counts", "foreign", "bad line", "missing fields"} {
		t.Run(name, func(t *testing.T) {
			root, bundle := fixture(t), t.TempDir()
			write(t, bundle, "vendor/autoload.php", "<?php")
			process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
				if cmd.Name != "php" || cmd.Directory != root || !slices.Contains(cmd.Args, "memory_limit=1024M") {
					t.Fatalf("unsafe Deptrac command: %+v", cmd)
				}
				var input map[string]string
				if json.Unmarshal(cmd.Input, &input) != nil || input["config"] != filepath.Join(root, "deptrac.yaml") || input["project_root"] != root {
					t.Fatal("Deptrac did not receive the full project configuration")
				}
				result := runner.Result{Stdout: nativeReport()}
				switch name {
				case "missing PHP":
					result.Unavailable = true
				case "timeout":
					result.TimedOut = true
				case "crash":
					result.ExitCode = 70
				case "stderr":
					result.Stderr = []byte("unexpected warning")
				case "malformed":
					result.Stdout = []byte("{")
				case "trailing":
					result.Stdout = append(result.Stdout, []byte(" {}")...)
				case "version":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), Version, "0.0.0", 1))
				case "partial":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"errors":0`, `"errors":1`, 1))
				case "counts":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"violation":1`, `"violation":2`, 1))
				case "foreign":
					result.Stdout = []byte(strings.ReplaceAll(string(result.Stdout), "src/Domain/Service.php", "../foreign.php"))
				case "bad line":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"line":3`, `"line":999`, 1))
				case "missing fields":
					result.Stdout = []byte(`{"version":1}`)
				}
				return result
			})
			result := run(context.Background(), root, ".", selected(nil), settings(t), bundle, process)
			if name == "valid" {
				if result.Check.Status != "failed" || len(result.Findings) != 1 || result.Findings[0].Rule != "deptrac.violation" {
					t.Fatalf("valid report rejected: %+v", result)
				}
			} else if result.Check.Status != "error" && result.Check.Status != "unavailable" || len(result.Findings) != 0 || len(result.Diagnostics) != 1 {
				t.Fatalf("failure accepted: %+v", result)
			}
		})
	}
}

func TestScopeAndUncoveredPolicy(t *testing.T) {
	root, bundle := fixture(t), t.TempDir()
	write(t, bundle, "vendor/autoload.php", "<?php")
	for _, tc := range []struct {
		kind                  string
		report, fail, skipped bool
		severity, status      string
	}{
		{"uncovered", false, false, false, "", "passed"},
		{"uncovered", true, false, false, "warning", "passed"},
		{"uncovered", false, true, false, "error", "failed"},
		{"skipped", false, false, true, "info", "passed"},
	} {
		var parsed response
		json.Unmarshal(nativeReport(), &parsed)
		parsed.Findings[0].Kind = tc.kind
		parsed.Counts["violation"], parsed.Counts[tc.kind] = 0, 1
		if tc.kind == "uncovered" {
			parsed.Findings[0].To = nil
		}
		data, _ := json.Marshal(parsed)
		cfg := settings(t)
		cfg.ReportUncovered, cfg.FailOnUncovered, cfg.ReportSkipped = tc.report, tc.fail, tc.skipped
		process := processFunc(func(context.Context, runner.Command) runner.Result { return runner.Result{Stdout: data} })
		result := run(context.Background(), root, ".", selected(nil), cfg, bundle, process)
		if result.Check.Status != tc.status || tc.severity == "" && len(result.Findings) != 0 || tc.severity != "" && (len(result.Findings) != 1 || result.Findings[0].Severity != tc.severity) {
			t.Fatalf("policy %+v: %+v", tc, result)
		}
		result = run(context.Background(), root, ".", selected([]scope.Range{{Start: 1, End: 1}}), cfg, bundle, process)
		if result.Check.Status != "passed" || len(result.Findings) != 0 {
			t.Fatalf("diff filtering: %+v", result)
		}
	}
}

func TestNativeArchitectureGraphAndImportedBaseline(t *testing.T) {
	root := fixture(t)
	bundle, _ := filepath.Abs("../../../tools/deptrac")
	result := run(context.Background(), root, ".", selected(nil), settings(t), bundle, runner.Exec{})
	if result.Check.Status != "failed" || len(result.Findings) == 0 || len(result.Diagnostics) != 0 {
		t.Fatalf("native Deptrac: %+v", result)
	}
	for _, finding := range result.Findings {
		if finding.Rule != "deptrac.violation" || *finding.Path != "src/Domain/Service.php" {
			t.Fatalf("foreign finding: %+v", finding)
		}
	}
	write(t, root, "deptrac.yaml", "imports:\n  - resource: baseline.yaml\n"+nativeConfig)
	write(t, root, "baseline.yaml", "deptrac:\n  skip_violations:\n    'App\\Domain\\Service': ['App\\Infrastructure\\Store']\n")
	result = run(context.Background(), root, ".", selected(nil), settings(t), bundle, runner.Exec{})
	if result.Check.Status != "passed" || len(result.Findings) != 0 || len(result.Diagnostics) != 0 {
		t.Fatalf("native baseline: %+v", result)
	}
	data, _ := os.ReadFile(filepath.Join(root, "src/Domain/Service.php"))
	if string(data) != source {
		t.Fatal("architecture analysis modified source")
	}
}

func TestNoConfigAndUnsafeConfiguration(t *testing.T) {
	unused := processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("unexpected Deptrac process")
		return runner.Result{}
	})
	root := t.TempDir()
	write(t, root, "src/Domain/Service.php", source)
	result := run(context.Background(), root, ".", selected(nil), settings(t), "", unused)
	if result.Check.Status != "skipped" || *result.Check.Reason != "no_config" {
		t.Fatalf("missing config: %+v", result)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "architecture_config_missing" {
		t.Fatalf("missing architecture setup guidance: %+v", result)
	}
	cfg := settings(t)
	cfg.Config = "missing.yaml"
	result = run(context.Background(), root, ".", selected(nil), cfg, "", unused)
	if result.Check.Status != "error" {
		t.Fatalf("missing explicit config silently skipped: %+v", result)
	}
	for _, source := range []string{"services: {}\n", "imports: [../outside.yaml]\n", "imports: [deptrac.yaml]\n", "deptrac: {paths: [/tmp]}\n", "deptrac: {feature_flags: {phpstan_parser: true}}\n", "deptrac: {}\ndeptrac: {}\n"} {
		write(t, root, "deptrac.yaml", source)
		if _, err := prepare(root, ".", settings(t)); err == nil {
			t.Fatalf("unsafe config accepted: %s", source)
		}
	}
	write(t, root, "deptrac.yaml", nativeConfig)
	before := ContextHash(root)
	write(t, root, "baseline.yaml", "deptrac: {skip_violations: {}}\n")
	if before == ContextHash(root) {
		t.Fatal("import/baseline change did not invalidate context")
	}
	if err := os.Symlink(root, filepath.Join(root, "src/link")); err != nil {
		t.Fatal(err)
	}
	if _, err := prepare(root, ".", settings(t)); err == nil {
		t.Fatal("symlink graph accepted")
	}
}
