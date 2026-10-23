package phpstan

import (
	"context"
	"fmt"
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

func (f processFunc) Run(ctx context.Context, cmd runner.Command) runner.Result { return f(ctx, cmd) }

func fakeBundle(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "vendor/autoload.php", "<?php\n")
	return root
}

func metadataResponse() []byte {
	return []byte(`{"version":1,"runtime":"8.4","php_version":"8.4.26","tools":{"phpstan/phpstan":"` + Version + `","mglaman/phpstan-drupal":"` + DrupalVersion + `","phpstan/phpstan-deprecation-rules":"` + DeprecationVersion + `","shipmonk/dead-code-detector":"` + DeadCodeVersion + `","phpstan/phpstan-strict-rules":"` + StrictRulesVersion + `"}}`)
}

func analysisResponse(name string) []byte {
	return []byte(`{"totals":{"errors":0,"file_errors":1},"files":{` + fmt.Sprintf("%q", name) + `:{"errors":1,"messages":[{"message":"Bad return","line":2,"ignorable":true,"identifier":"return.type"}]}},"errors":[]}`)
}

func TestPHPStanFailuresCannotPass(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	cases := []string{"valid", "missing PHP", "helper timeout", "helper crash", "helper stderr", "malformed metadata", "version", "runtime",
		"analysis timeout", "analysis crash", "malformed", "trailing document", "analysis stderr", "foreign file", "line zero", "totals",
		"exit mismatch", "global error", "non-ignorable outside diff", "missing identifier"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, "demo.php", "<?php\nfunction demo(): int { return 'wrong'; }\n")
			selection := scope.Selection{Files: []string{"demo.php"}, LineMap: map[string][]scope.Range{"demo.php": {{Start: 20, End: 20}}}}
			var temporary string
			process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
				if cmd.Name != "php" || cmd.Directory != root {
					t.Fatalf("unsafe invocation: %+v", cmd)
				}
				if !slices.Contains(cmd.Args, "analyse") {
					temporary = cmd.Args[len(cmd.Args)-1]
					if _, err := os.Stat(filepath.Join(temporary, "prepare.php")); err != nil {
						t.Fatal(err)
					}
					result := runner.Result{Stdout: metadataResponse()}
					switch name {
					case "missing PHP":
						result.Unavailable = true
					case "helper timeout":
						result.TimedOut = true
					case "helper crash":
						result.ExitCode = 70
						result.Err = fmt.Errorf("crash")
					case "helper stderr":
						result.Stderr = []byte("warning")
					case "malformed metadata":
						result.Stdout = []byte("{")
					case "version":
						result.Stdout = []byte(strings.Replace(string(result.Stdout), Version, "2.3.0", 1))
					case "runtime":
						result.Stdout = []byte(strings.Replace(string(result.Stdout), `"runtime":"8.4"`, `"runtime":"8.3"`, 1))
					}
					return result
				}
				if slices.ContainsFunc(cmd.Env, func(variable string) bool { return strings.HasPrefix(variable, "CLAUDECODE=") }) {
					t.Fatal("agent variables leaked into PHPStan")
				}
				result := runner.Result{Stdout: analysisResponse(filepath.Join(root, "demo.php")), ExitCode: 1, Err: fmt.Errorf("exit status 1")}
				switch name {
				case "analysis timeout":
					result.TimedOut = true
				case "analysis crash":
					result.ExitCode = 255
					result.Err = fmt.Errorf("exit status 255")
				case "malformed":
					result.Stdout = []byte("{")
				case "trailing document":
					result.Stdout = append(result.Stdout, []byte(" {}")...)
				case "analysis stderr":
					result.Stderr = []byte("warning")
				case "foreign file":
					result.Stdout = analysisResponse(filepath.Join(root, "../outside.php"))
				case "line zero":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"line":2`, `"line":0`, 1))
				case "totals":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"errors":1`, `"errors":2`, 1))
				case "exit mismatch":
					result.ExitCode, result.Err = 0, nil
				case "global error":
					result.Stdout = []byte(strings.Replace(strings.Replace(string(result.Stdout), `"errors":0`, `"errors":1`, 1), `"errors":[]`, `"errors":["Internal error"]`, 1))
				case "non-ignorable outside diff":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"ignorable":true`, `"ignorable":false`, 1))
				case "missing identifier":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"return.type"`, `""`, 1))
				}
				return result
			})
			result := run(context.Background(), root, ".", selection, settings(t), config.Target{Min: "8.4", Max: "8.4"}, "", fakeBundle(t), process)
			if name == "valid" {
				if result.Check.Status != "passed" || len(result.Diagnostics) != 0 || len(result.Findings) != 0 {
					t.Fatalf("valid report outside diff rejected: %+v", result)
				}
			} else if result.Check.Status != "error" && result.Check.Status != "unavailable" || len(result.Diagnostics) != 1 || len(result.Findings) != 0 {
				t.Fatalf("failure passed: %+v", result)
			}
			if _, err := os.Stat(temporary); !os.IsNotExist(err) {
				t.Fatal("helper temporary directory leaked")
			}
		})
	}
}

func TestMissingInputsAndCancellation(t *testing.T) {
	root := t.TempDir()
	write(t, root, "demo.php", "<?php\n")
	selection := scope.Selection{Files: []string{"demo.php"}, LineMap: map[string][]scope.Range{"demo.php": nil}}
	unused := processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("unexpected process")
		return runner.Result{}
	})
	result := run(context.Background(), root, ".", selection, settings(t), config.Target{}, "", "/nonexistent-phpwarden-bundle", unused)
	if result.Check.Status != "unavailable" || result.Diagnostics[0].Code != "dependencies_missing" {
		t.Fatalf("bundle failure hidden: %+v", result)
	}
	cfg := settings(t)
	cfg.Drupal = true
	result = run(context.Background(), root, ".", selection, cfg, config.Target{}, "", fakeBundle(t), unused)
	if result.Check.Status != "unavailable" || result.Diagnostics[0].Code != "project_dependencies_missing" {
		t.Fatalf("missing project dependencies hidden: %+v", result)
	}
	write(t, root, "vendor/autoload.php", "<?php\n")
	result = run(context.Background(), root, ".", selection, settings(t), config.Target{}, "installed drupal/core 10.4.4 conflicts with composer.lock version 11.3.16", fakeBundle(t), unused)
	if result.Check.Status != "unavailable" || result.Diagnostics[0].Code != "project_dependencies_mismatch" || !strings.Contains(result.Diagnostics[0].Message, "10.4.4") {
		t.Fatalf("mismatched project dependencies hidden: %+v", result)
	}
	if err := os.RemoveAll(filepath.Join(root, "vendor")); err != nil {
		t.Fatal(err)
	}
	cfg = settings(t)
	cfg.Config = "missing.neon"
	result = run(context.Background(), root, ".", selection, cfg, config.Target{}, "", fakeBundle(t), unused)
	if result.Check.Status != "error" || result.Diagnostics[0].Code != "config_invalid" {
		t.Fatalf("missing config hidden: %+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = run(ctx, root, ".", selection, settings(t), config.Target{}, "", fakeBundle(t), unused)
	if result.Check.Status != "error" || result.Diagnostics[0].Code != "checker_timeout" {
		t.Fatalf("cancelled check passed: %+v", result)
	}
	cfg = settings(t)
	cfg.Enabled = false
	result = run(ctx, root, ".", selection, cfg, config.Target{}, "", "", unused)
	if result.Check.Status != "skipped" || *result.Check.Reason != "disabled" {
		t.Fatalf("disabled checker ran: %+v", result)
	}
}
