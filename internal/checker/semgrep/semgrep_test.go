package semgrep

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

type processFunc func(context.Context, runner.Command) runner.Result

func (f processFunc) Run(ctx context.Context, command runner.Command) runner.Result {
	return f(ctx, command)
}

func settings(t *testing.T) config.Semgrep {
	t.Helper()
	effective, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}},"tools":{"semgrep":{"enabled":true}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return effective.Config.Tools.Semgrep
}

func write(t *testing.T, root, name, data string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func selected(name string) scope.Selection {
	return scope.Selection{Files: []string{name}, LineMap: map[string][]scope.Range{name: nil}}
}

func TestActualBundledSecurityRulesAndSafeControls(t *testing.T) {
	binary, _ := filepath.Abs("../../../tools/semgrep/vendor/bin/semgrep-core")
	for _, tc := range []struct{ name, code, rule string }{
		{"command", "$value = $_GET['cmd']; system($value);", "request-command-injection"},
		{"sql", "$value = $_POST['name']; $db->query('SELECT * FROM users WHERE name=' . $value);", "request-sql-injection"},
		{"xss", "$value = $_GET['html']; echo $value;", "request-xss"},
		{"markup", "$value = $_POST['html']; \\Drupal\\Core\\Render\\Markup::create($value);", "request-xss"},
		{"request", "$value = \\Drupal::request()->query->get('cmd'); system($value);", "request-command-injection"},
		{"unserialize", "$value = $_COOKIE['data']; unserialize($value);", "request-unserialize"},
		{"constant", "system('date');", ""},
		{"escaped", "echo htmlspecialchars($_GET['html'], ENT_QUOTES, 'UTF-8');", ""},
		{"parameterized", "$value = $_POST['name']; $db->query('SELECT * FROM users WHERE name=:name', [':name' => $value]);", ""},
		{"quoted argument", "system('echo ' . escapeshellarg($_GET['name']));", ""},
		{"json", "json_decode($_COOKIE['data']);", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			name := "custom/space\nand newline.module"
			source := "<?php\n" + tc.code + "\n"
			write(t, root, name, source)
			cfg := settings(t)
			cfg.FailOn = "never"
			result := run(context.Background(), root, ".", selected(name), cfg, binary, runner.Exec{})
			if result.Check.Status != "passed" {
				t.Fatalf("native scan: %+v", result)
			}
			if tc.rule == "" {
				if len(result.Findings) != 0 {
					t.Fatalf("safe control flagged: %+v", result)
				}
			} else if len(result.Findings) != 1 || result.Findings[0].Rule != "phpwarden.php."+tc.rule || result.Findings[0].Fingerprint == nil {
				t.Fatalf("missing security finding: %+v", result)
			}
			actual, _ := os.ReadFile(filepath.Join(root, name))
			if string(actual) != source {
				t.Fatal("scan modified source")
			}
		})
	}
}

func TestActualDiffBaselineAndParseFailure(t *testing.T) {
	root := t.TempDir()
	name := "custom/demo.module"
	source := "<?php\n$value = $_GET['cmd'];\nsystem($value);\n\n"
	write(t, root, name, source)
	cfg := settings(t)
	cfg.Rules = []string{"phpwarden.php.request-command-injection"}
	binary, _ := filepath.Abs("../../../tools/semgrep/vendor/bin/semgrep-core")
	selection := selected(name)
	selection.LineMap[name] = []scope.Range{{Start: 4, End: 4}}
	result := run(context.Background(), root, ".", selection, cfg, binary, runner.Exec{})
	if result.Check.Status != "passed" || len(result.Findings) != 0 {
		t.Fatalf("old finding escaped diff: %+v", result)
	}
	selection.LineMap[name] = nil
	result = run(context.Background(), root, ".", selection, cfg, binary, runner.Exec{})
	if result.Check.Status != "failed" || len(result.Findings) != 1 {
		t.Fatalf("full file scan: %+v", result)
	}
	id := *result.Findings[0].Fingerprint
	baseline, _ := json.Marshal(map[string]any{"version": 1, "fingerprints": []string{id}})
	write(t, root, "baseline.json", string(baseline))
	cfg.Baseline = "baseline.json"
	write(t, root, name, "<?php\n\n$value = $_GET['cmd'];\nsystem($value);\n")
	result = run(context.Background(), root, ".", selection, cfg, binary, runner.Exec{})
	if result.Check.Status != "passed" || len(result.Findings) != 0 {
		t.Fatalf("baseline lost after line shift: %+v", result)
	}
	write(t, root, name, "<?php\nfunction broken( {\n")
	result = run(context.Background(), root, ".", selection, cfg, binary, runner.Exec{})
	if result.Check.Status != "error" {
		t.Fatalf("partial parsing looked clean: %+v", result)
	}
}

func fakeReport(input string, rules []rule) coreReport {
	r := coreReport{Version: Version, Results: []match{}, Errors: []json.RawMessage{}, Engine: "OSS", Time: json.RawMessage(`{"fixpoint_timeouts":[]}`)}
	r.Paths.Scanned = []string{input}
	for _, rule := range rules {
		r.RulesByEngine = append(r.RulesByEngine, []string{rule.ID, "OSS"})
	}
	return r
}

func TestStrictCETransportFailuresAndIsolation(t *testing.T) {
	for _, kind := range []string{"valid", "version", "unavailable", "timeout", "stderr", "crash", "malformed", "trailing", "parse error", "skipped target", "skipped rule", "missing target", "foreign finding", "pro", "fixpoint timeout", "missing rules"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			binary := filepath.Join(t.TempDir(), "semgrep-core")
			write(t, filepath.Dir(binary), "semgrep-core", "")
			write(t, root, "demo.php", "<?php\necho 1;\n")
			cfg := settings(t)
			prepared, err := prepare(root, ".", cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("SEMGREP_CORE_EXTRA", "-trace")
			t.Setenv("LD_PRELOAD", "must-not-load")
			var temporary string
			process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
				for _, item := range cmd.Env {
					if strings.HasPrefix(item, "SEMGREP_") || strings.HasPrefix(item, "LD_") {
						t.Fatal("ambient engine settings leaked")
					}
				}
				temporary = cmd.Directory
				if slices.Equal(cmd.Args, []string{"-version"}) {
					if kind == "version" {
						return runner.Result{Stdout: []byte("wrong version")}
					}
					if kind == "unavailable" {
						return runner.Result{Unavailable: true}
					}
					return runner.Result{Stdout: []byte("semgrep-core version: " + Version + " (fixture)\n")}
				}
				for _, flag := range []string{"-json_time", "-json_nodots", "-strict", "-no_filter_irrelevant_rules"} {
					if !slices.Contains(cmd.Args, flag) {
						t.Fatalf("missing %s", flag)
					}
				}
				input := cmd.Args[len(cmd.Args)-1]
				if input == filepath.Join(root, "demo.php") {
					t.Fatal("analyzer received original source")
				}
				r := fakeReport(input, prepared.Rules)
				switch kind {
				case "timeout":
					return runner.Result{TimedOut: true}
				case "stderr":
					return runner.Result{Stderr: []byte("sensitive native output")}
				case "crash":
					return runner.Result{ExitCode: 2}
				case "malformed":
					return runner.Result{Stdout: []byte("invalid")}
				case "parse error":
					r.Errors = []json.RawMessage{json.RawMessage(`{}`)}
				case "skipped target":
					r.Paths.Skipped = []json.RawMessage{json.RawMessage(`{}`)}
				case "skipped rule":
					r.SkippedRules = []json.RawMessage{json.RawMessage(`{}`)}
				case "missing target":
					r.Paths.Scanned = nil
				case "foreign finding":
					r.Results = []match{{Path: "other.php"}}
				case "pro":
					r.Engine = "PRO"
				case "fixpoint timeout":
					r.Time = json.RawMessage(`{"fixpoint_timeouts":[{}]}`)
				case "missing rules":
					r.RulesByEngine = nil
				}
				data, _ := json.Marshal(r)
				if kind == "trailing" {
					data = append(data, []byte("{}")...)
				}
				return runner.Result{Stdout: data}
			})
			result := run(context.Background(), root, ".", selected("demo.php"), cfg, binary, process)
			if (result.Check.Status == "passed") != (kind == "valid") {
				t.Fatalf("%s: %+v", kind, result)
			}
			data, _ := json.Marshal(result)
			if strings.Contains(string(data), "sensitive native output") {
				t.Fatal("native source output leaked")
			}
			if _, err := os.Stat(temporary); !os.IsNotExist(err) {
				t.Fatal("private directory leaked")
			}
		})
	}
}
