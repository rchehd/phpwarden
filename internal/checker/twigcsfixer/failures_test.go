package twigcsfixer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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

func response(name, rows string) []byte {
	return []byte(fmt.Sprintf(`{"version":1,"runtime":"8.4","php_version":"8.4.26","tools":{"vincentlanglet/twig-cs-fixer":%q,"twig/twig":%q},"files":{%q:[%s]}}`, Version, TwigVersion, name, rows))
}

const finding = `{"level":2,"rule":"DelimiterSpacing.After","message":"Expecting 1 whitespace","line":2,"column":1}`

func TestTwigCSFixerFailuresCannotPass(t *testing.T) {
	for _, name := range []string{"valid", "deprecation", "missing PHP", "timeout", "crash", "malformed", "trailing document", "stderr", "version",
		"twig version", "runtime", "foreign file", "missing file", "line zero", "missing rule", "unknown level", "other fatal"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, "demo.html.twig", "<p>{{ title }}</p>\n<p>{{title}}</p>\n")
			selection := scope.Selection{Files: []string{"demo.html.twig"}, LineMap: map[string][]scope.Range{"demo.html.twig": {{Start: 20, End: 20}}}}
			absolute := filepath.Join(root, "demo.html.twig")
			var script string
			process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
				if cmd.Name != "php" || cmd.Directory != root || !strings.Contains(string(cmd.Input), `"config":null`) || !strings.Contains(string(cmd.Input), "IncludeFunctionRule") {
					t.Fatalf("unsafe invocation: %+v %s", cmd, cmd.Input)
				}
				script = cmd.Args[len(cmd.Args)-2]
				if _, err := os.Stat(script); err != nil {
					t.Fatal(err)
				}
				result := runner.Result{Stdout: response(absolute, finding)}
				switch name {
				case "deprecation":
					result.Stdout = response(absolute, `{"level":0,"rule":"","message":"Twig deprecation","line":null,"column":null}`)
				case "missing PHP":
					result.Unavailable = true
				case "timeout":
					result.TimedOut = true
				case "crash":
					result.ExitCode, result.Err = 70, fmt.Errorf("crash")
				case "malformed":
					result.Stdout = []byte("{")
				case "trailing document":
					result.Stdout = append(result.Stdout, []byte(" {}")...)
				case "stderr":
					result.Stderr = []byte("warning")
				case "version":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), Version, "4.1.0", 1))
				case "twig version":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), TwigVersion, "3.29.0", 1))
				case "runtime":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"runtime":"8.4"`, `"runtime":"8.3"`, 1))
				case "foreign file":
					result.Stdout = response(filepath.Join(root, "other.html.twig"), finding)
				case "missing file":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"files":{`+fmt.Sprintf("%q", absolute)+`:[`+finding+`]}`, `"files":{}`, 1))
				case "line zero":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"line":2`, `"line":0`, 1))
				case "missing rule":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"rule":"DelimiterSpacing.After"`, `"rule":""`, 1))
				case "unknown level":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"level":2`, `"level":4`, 1))
				case "other fatal":
					result.Stdout = response(absolute, `{"level":3,"rule":"","message":"Unable to read file.","line":null,"column":null}`)
				}
				return result
			})
			result := run(context.Background(), root, selection, settings(t), "drupal", fakeBundle(t), process)
			switch name {
			case "valid":
				if result.Check.Status != "passed" || len(result.Diagnostics) != 0 || len(result.Findings) != 0 {
					t.Fatalf("valid report outside diff rejected: %+v", result)
				}
			case "deprecation":
				if result.Check.Status != "passed" || len(result.Findings) != 1 || result.Findings[0].Kind != "global" || result.Findings[0].Severity != "info" ||
					result.Findings[0].Message != `Twig template "demo.html.twig": Twig deprecation` {
					t.Fatalf("deprecation notice mishandled: %+v", result)
				}
			default:
				if result.Check.Status != "error" && result.Check.Status != "unavailable" || len(result.Diagnostics) != 1 || len(result.Findings) != 0 {
					t.Fatalf("failure passed: %+v", result)
				}
			}
			if _, err := os.Stat(filepath.Dir(script)); !os.IsNotExist(err) {
				t.Fatal("helper temporary directory leaked")
			}
		})
	}
}

func TestMissingInputsAndCancellation(t *testing.T) {
	root := t.TempDir()
	write(t, root, "demo.html.twig", "<p>{{ title }}</p>\n")
	selection := scope.Selection{Files: []string{"demo.html.twig"}, LineMap: map[string][]scope.Range{"demo.html.twig": nil}}
	unused := processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("unexpected process")
		return runner.Result{}
	})
	result := run(context.Background(), root, selection, settings(t), "drupal", "/nonexistent-phpwarden-bundle", unused)
	if result.Check.Status != "unavailable" || result.Diagnostics[0].Code != "dependencies_missing" {
		t.Fatalf("bundle failure hidden: %+v", result)
	}
	cfg := settings(t)
	cfg.Config = "missing.php"
	result = run(context.Background(), root, selection, cfg, "drupal", fakeBundle(t), unused)
	if result.Check.Status != "error" || result.Diagnostics[0].Code != "config_invalid" {
		t.Fatalf("missing config hidden: %+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = run(ctx, root, selection, settings(t), "drupal", fakeBundle(t), unused)
	if result.Check.Status != "error" || result.Diagnostics[0].Code != "checker_timeout" {
		t.Fatalf("cancelled check passed: %+v", result)
	}
	cfg = settings(t)
	cfg.Enabled = false
	result = run(ctx, root, selection, cfg, "drupal", "", unused)
	if result.Check.Status != "skipped" || *result.Check.Reason != "disabled" {
		t.Fatalf("disabled checker ran: %+v", result)
	}
}
