package phpcs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

func response(name string) []byte {
	data := helperReport{Version: 1, Runtime: "8.4", PHPVersion: "8.4.26", PHPCSVersion: Version, CoderVersion: CoderVersion, CompatibilityVersion: CompatibilityVersion, ExitCode: 1}
	data.Report.Totals = totals{Errors: 1, Fixable: 1}
	data.Report.Files = map[string]fileReport{name: {Errors: 1, Warnings: 0, Messages: []message{{Source: "Example.Debug.Echo.Found", Message: "Forbidden echo", Type: "ERROR", Line: 2, Column: 1, Severity: 5, Fixable: true}}}}
	encoded, _ := json.Marshal(data)
	return encoded
}

func TestPHPCSFailuresCannotPass(t *testing.T) {
	for _, name := range []string{"missing PHP", "timeout", "crash", "malformed", "trailing document", "stderr", "version", "foreign file", "line zero", "totals", "internal outside diff"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, "demo.php", "<?php\necho 1;\n")
			selection := scope.Selection{Files: []string{"demo.php"}, LineMap: map[string][]scope.Range{"demo.php": {{Start: 20, End: 20}}}}
			var script string
			process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
				if cmd.Name != "php" || cmd.Directory != root {
					t.Fatalf("unsafe invocation: %+v", cmd)
				}
				script = cmd.Args[len(cmd.Args)-2]
				if _, err := os.Stat(script); err != nil {
					t.Fatal(err)
				}
				result := runner.Result{Stdout: response("demo.php")}
				switch name {
				case "missing PHP":
					result.Unavailable = true
				case "timeout":
					result.TimedOut = true
				case "crash":
					result.ExitCode = 70
					result.Err = fmt.Errorf("crash")
				case "malformed":
					result.Stdout = []byte("{")
				case "trailing document":
					result.Stdout = append(result.Stdout, []byte(" {}")...)
				case "stderr":
					result.Stderr = []byte("warning")
				case "version":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), Version, "4.0.3", 1))
				case "foreign file":
					result.Stdout = response("../outside.php")
				case "line zero":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"line":2`, `"line":0`, 1))
				case "totals":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"errors":1`, `"errors":2`, 1))
				case "internal outside diff":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), "Example.Debug.Echo.Found", "Internal.Exception", 1))
				}
				return result
			})
			result := run(context.Background(), root, ".", selection, settings(t), config.Target{Min: "8.4", Max: "8.4"}, fakeBundle(t), process)
			if result.Check.Status != "error" && result.Check.Status != "unavailable" || len(result.Diagnostics) != 1 || len(result.Findings) != 0 {
				t.Fatalf("failure passed: %+v", result)
			}
			if _, err := os.Stat(filepath.Dir(script)); !os.IsNotExist(err) {
				t.Fatal("helper temporary directory leaked")
			}
		})
	}
}

func TestMissingBundleAndCancellation(t *testing.T) {
	root := t.TempDir()
	write(t, root, "demo.php", "<?php\n")
	selection := scope.Selection{Files: []string{"demo.php"}, LineMap: map[string][]scope.Range{"demo.php": nil}}
	unused := processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("unexpected process")
		return runner.Result{}
	})
	result := run(context.Background(), root, ".", selection, settings(t), config.Target{}, "/nonexistent-phpwarden-bundle", unused)
	if result.Check.Status != "unavailable" || result.Diagnostics[0].Code != "dependencies_missing" {
		t.Fatalf("bundle failure hidden: %+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = run(ctx, root, ".", selection, settings(t), config.Target{}, fakeBundle(t), unused)
	if result.Check.Status != "error" || result.Diagnostics[0].Code != "checker_timeout" {
		t.Fatalf("cancelled check passed: %+v", result)
	}
	cfg := settings(t)
	cfg.Enabled = false
	result = run(ctx, root, ".", selection, cfg, config.Target{}, "", unused)
	if result.Check.Status != "skipped" || *result.Check.Reason != "disabled" {
		t.Fatalf("disabled checker ran: %+v", result)
	}
}
