package composervalidate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func settings(t *testing.T) config.ComposerValidate {
	t.Helper()
	cfg, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Config.Tools.ComposerValidate
}

func write(t *testing.T, root, name, data string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func rules(findings []report.Finding) []string {
	result := []string{}
	for _, finding := range findings {
		result = append(result, finding.Rule)
	}
	slices.Sort(result)
	return slices.Compact(result)
}

func TestSelectRootAndModuleComposerFiles(t *testing.T) {
	selection := scope.Selection{Files: []string{"web/modules/custom/demo/composer.json", "web/modules/custom/demo/demo.module"}, ProjectFiles: []string{"composer.lock"}}
	targets := Select(selection, settings(t), ".")
	if !slices.Equal(targets, []Target{{Path: "composer.json", Lock: true}, {Path: "web/modules/custom/demo/composer.json"}}) {
		t.Fatalf("unexpected targets: %+v", targets)
	}
	cfg := settings(t)
	cfg.Exclude = []string{"composer.json"}
	if targets := Select(selection, cfg, "."); !slices.Equal(targets, []Target{{Path: "web/modules/custom/demo/composer.json"}}) {
		t.Fatalf("exclusion ignored: %+v", targets)
	}
	selection.ProjectFiles = []string{}
	if targets := Select(selection, settings(t), "."); len(targets) != 1 || targets[0].Lock {
		t.Fatalf("unchanged root validated: %+v", targets)
	}
}

func TestActualComposerValidate(t *testing.T) {
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/composer")
	write(t, root, "composer.json", `{"name":"acme/site","type":"project","description":"Site","license":"MIT","require":{"acme/lib":">=1.0"}}`)
	write(t, root, "web/modules/custom/demo/composer.json", `{"name":"Demo Module","type":"drupal-module"}`)
	targets := []Target{{Path: "composer.json", Lock: true}, {Path: "web/modules/custom/demo/composer.json"}}
	check := func(cfg config.ComposerValidate, status string, want ...string) []report.Finding {
		t.Helper()
		result := run(context.Background(), root, targets, cfg, bundle, runner.Exec{})
		if len(result.Diagnostics) > 0 || result.Check.Status != status || !slices.Equal(rules(result.Findings), want) {
			t.Fatalf("Composer validate result %v, want %s %v: %+v", rules(result.Findings), status, want, result)
		}
		for _, finding := range result.Findings {
			if finding.Kind != "global" || finding.Path != nil || strings.Contains(finding.Message, root) ||
				!strings.HasPrefix(finding.Message, `Composer file "composer.json": `) && !strings.HasPrefix(finding.Message, `Composer file "web/modules/custom/demo/composer.json": `) {
				t.Fatalf("invalid Composer finding: %+v", finding)
			}
		}
		return result.Findings
	}
	check(settings(t), "failed", "composer.validate.error", "composer.validate.warning")
	write(t, root, "web/modules/custom/demo/composer.json", `{"name":"acme/demo","type":"drupal-module","description":"Demo","license":"MIT"}`)
	check(settings(t), "passed", "composer.validate.warning")
	cfg := settings(t)
	cfg.Strict = true
	check(cfg, "failed", "composer.validate.warning")
	write(t, root, "composer.lock", `{"content-hash":"00000000000000000000000000000000","packages":[],"packages-dev":[],"aliases":[],"minimum-stability":"stable","stability-flags":{},"prefer-stable":false,"prefer-lowest":false,"platform":{},"platform-dev":{}}`)
	findings := check(settings(t), "failed", "composer.lock.error", "composer.validate.warning")
	if !slices.ContainsFunc(findings, func(finding report.Finding) bool { return strings.Contains(finding.Message, "not up to date") }) {
		t.Fatalf("stale lock not reported: %+v", findings)
	}
	write(t, root, "composer.json", `{"name":`)
	check(settings(t), "failed", "composer.validate.error")
}

type processFunc func(context.Context, runner.Command) runner.Result

func (f processFunc) Run(ctx context.Context, cmd runner.Command) runner.Result { return f(ctx, cmd) }

func TestComposerValidateFailuresCannotPass(t *testing.T) {
	for _, name := range []string{"valid", "missing PHP", "timeout", "crash", "stderr", "malformed", "trailing document", "version", "runtime", "foreign file", "missing list", "empty message"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			bundle := t.TempDir()
			write(t, bundle, "vendor/autoload.php", "<?php\n")
			absolute := filepath.Join(root, "composer.json")
			process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
				if cmd.Name != "php" || cmd.Directory != root || !slices.Contains(cmd.Env, "COMPOSER_DISABLE_NETWORK=1") || !strings.Contains(string(cmd.Input), `"lock":true`) {
					t.Fatalf("unsafe invocation: %+v", cmd)
				}
				result := runner.Result{Stdout: fmt.Appendf(nil, `{"version":1,"runtime":"8.4","php_version":"8.4.26","composer_version":%q,"files":{%q:{"errors":[],"warnings":["Warning"],"lock":[]}}}`, ComposerVersion, absolute)}
				switch name {
				case "missing PHP":
					result.Unavailable = true
				case "timeout":
					result.TimedOut = true
				case "crash":
					result.ExitCode, result.Err = 70, fmt.Errorf("crash")
				case "stderr":
					result.Stderr = []byte("warning")
				case "malformed":
					result.Stdout = []byte("{")
				case "trailing document":
					result.Stdout = append(result.Stdout, []byte(" {}")...)
				case "version":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), ComposerVersion, "2.10.2", 1))
				case "runtime":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"runtime":"8.4"`, `"runtime":"8.3"`, 1))
				case "foreign file":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), absolute, filepath.Join(root, "other.json"), 1))
				case "missing list":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `,"lock":[]`, ``, 1))
				case "empty message":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"Warning"`, `""`, 1))
				}
				return result
			})
			result := run(context.Background(), root, []Target{{Path: "composer.json", Lock: true}}, settings(t), bundle, process)
			if name == "valid" {
				if result.Check.Status != "passed" || len(result.Findings) != 1 || result.Findings[0].Message != `Composer file "composer.json": Warning` {
					t.Fatalf("valid report rejected: %+v", result)
				}
			} else if result.Check.Status != "error" && result.Check.Status != "unavailable" || len(result.Diagnostics) != 1 || len(result.Findings) != 0 {
				t.Fatalf("failure passed: %+v", result)
			}
		})
	}
}

func TestSkippedMissingBundleAndCancellation(t *testing.T) {
	root := t.TempDir()
	unused := processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("unexpected process")
		return runner.Result{}
	})
	targets := []Target{{Path: "composer.json", Lock: true}}
	if result := run(context.Background(), root, nil, settings(t), "", unused); result.Check.Status != "skipped" || *result.Check.Reason != "no_files" {
		t.Fatalf("empty selection ran: %+v", result)
	}
	cfg := settings(t)
	cfg.Enabled = false
	if result := run(context.Background(), root, targets, cfg, "", unused); result.Check.Status != "skipped" || *result.Check.Reason != "disabled" {
		t.Fatalf("disabled checker ran: %+v", result)
	}
	if result := run(context.Background(), root, targets, settings(t), "/nonexistent-phpwarden-bundle", unused); result.Check.Status != "unavailable" || result.Diagnostics[0].Code != "dependencies_missing" {
		t.Fatalf("missing bundle hidden: %+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := run(ctx, root, targets, settings(t), "", unused); result.Check.Status != "error" || result.Diagnostics[0].Code != "checker_timeout" {
		t.Fatalf("cancelled check passed: %+v", result)
	}
}
