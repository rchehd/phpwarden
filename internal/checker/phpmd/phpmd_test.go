package phpmd

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

const source = `<?php

function demo_hook($used, $unused) {
  $local = 1;
  return $used;
}

class Demo {
  private $never;
  private function hidden() {}
}
`

func settings(t *testing.T) config.PHPMD {
	t.Helper()
	cfg, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Config.Tools.PHPMD
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

func summary(findings []report.Finding) []string {
	result := []string{}
	for _, finding := range findings {
		if finding.Kind == "global" {
			result = append(result, finding.Rule+" "+finding.Message)
			continue
		}
		result = append(result, fmt.Sprintf("%s:%d %s %s", *finding.Path, *finding.Line, finding.Rule, finding.Severity))
	}
	slices.Sort(result)
	return result
}

func TestActualPHPMDProfileProjectRulesetAndParseErrors(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "true")
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/phpmd")
	name, broken := "modules/custom/demo/demo.module", "modules/custom/demo/broken.php"
	write(t, root, name, source)
	write(t, root, broken, "<?php\nfunction broken( {\n")
	selection := scope.Selection{Mode: "full", Files: []string{broken, name}, LineMap: map[string][]scope.Range{name: nil, broken: nil}}
	check := func(cfg config.PHPMD, status string, want ...string) {
		t.Helper()
		result := run(context.Background(), root, ".", selection, cfg, bundle, runner.Exec{})
		if result.Check.Status != status || len(result.Diagnostics) != 0 || !slices.Equal(summary(result.Findings), want) {
			t.Fatalf("got %s %q, want %s %q: %+v", result.Check.Status, summary(result.Findings), status, want, result.Diagnostics)
		}
	}
	parse := `phpmd.parse_error PHP file "modules/custom/demo/broken.php": PHPMD could not parse it: Unexpected token: {, line: 2, col: 18`
	check(settings(t), "passed", name+":10 UnusedPrivateMethod warning", name+":4 UnusedLocalVariable warning", name+":9 UnusedPrivateField warning", parse)
	cfg := settings(t)
	cfg.ExcludeRules, cfg.FailOn = []string{}, "warning"
	check(cfg, "failed", name+":10 UnusedPrivateMethod warning", name+":3 UnusedFormalParameter warning", name+":4 UnusedLocalVariable warning", name+":9 UnusedPrivateField warning", parse)
	write(t, root, "rules/extra.xml", `<?xml version="1.0"?><ruleset name="extra"><rule ref="rulesets/unusedcode.xml/UnusedPrivateField"/></ruleset>`)
	write(t, root, "modules/custom/phpmd.xml", `<?xml version="1.0"?><ruleset name="project"><rule ref="rules/extra.xml"/></ruleset>`)
	check(settings(t), "passed", name+":9 UnusedPrivateField warning", parse)
	write(t, root, "modules/custom/phpmd.baseline.xml", `<?xml version="1.0"?><phpmd-baseline><violation rule="PHPMD\Rule\UnusedPrivateField" file="modules/custom/demo/demo.module"/></phpmd-baseline>`)
	check(settings(t), "passed", parse)
	selection.Files, selection.LineMap = []string{name}, map[string][]scope.Range{name: {{Start: 4, End: 9}}}
	if err := os.Remove(filepath.Join(root, "modules/custom/phpmd.baseline.xml")); err != nil {
		t.Fatal(err)
	}
	check(settings(t), "passed", name+":9 UnusedPrivateField warning")
	write(t, root, "modules/custom/phpmd.xml", `<?xml version="1.0"?><ruleset name="project"><rule ref="rules/missing.xml"/></ruleset>`)
	if result := run(context.Background(), root, ".", selection, settings(t), bundle, runner.Exec{}); result.Check.Status != "error" || result.Diagnostics[0].Code != "config_invalid" {
		t.Fatalf("missing ruleset accepted: %+v", result)
	}
}

func TestContextHashTracksRulesetsAndBaseline(t *testing.T) {
	root := t.TempDir()
	write(t, root, "src/demo.php", "<?php\n")
	write(t, root, "phpmd.xml", `<ruleset name="p"><rule ref="rules/extra.xml"/></ruleset>`)
	write(t, root, "rules/extra.xml", `<ruleset name="x"/>`)
	cfg, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	selection := scope.Selection{Mode: "full", Files: []string{"src/demo.php"}, LineMap: map[string][]scope.Range{"src/demo.php": nil}}
	hashes := []string{ContextHash(root, selection, cfg.Config)}
	for _, change := range []func(){
		func() { write(t, root, "rules/extra.xml", `<ruleset name="y"/>`) },
		func() { write(t, root, "phpmd.baseline.xml", `<phpmd-baseline/>`) },
		func() { write(t, root, "src/phpmd.xml.dist", `<ruleset name="nearer"/>`) },
	} {
		change()
		hash := ContextHash(root, selection, cfg.Config)
		if slices.Contains(hashes, hash) {
			t.Fatal("PHPMD input change did not change the context hash")
		}
		hashes = append(hashes, hash)
	}
}

type processFunc func(context.Context, runner.Command) runner.Result

func (f processFunc) Run(ctx context.Context, cmd runner.Command) runner.Result { return f(ctx, cmd) }

func TestPHPMDFailuresCannotPass(t *testing.T) {
	valid := `{"version":"3.0.0","package":"phpmd","timestamp":"t","files":[{"file":"%s","relativePath":"a.php","violations":[{"beginLine":2,"endLine":2,"package":null,"function":null,"class":null,"method":null,"description":"Unused","rule":"UnusedLocalVariable","ruleSet":"Unused Code Rules","externalInfoUrl":"u","priority":3}]}],"errors":[]}`
	for _, name := range []string{"valid", "missing PHP", "timeout", "crash", "stderr", "malformed", "trailing", "version", "foreign file", "exit mismatch", "unknown error", "unknown field"} {
		t.Run(name, func(t *testing.T) {
			root, bundle := t.TempDir(), t.TempDir()
			write(t, root, "a.php", "<?php\n$a = 1;\n")
			write(t, bundle, "vendor/phpmd/phpmd/bin/phpmd", "<?php\n")
			process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
				if cmd.Name != "php" || cmd.Directory != root || !slices.Contains(cmd.Args, "--threads=1") || !slices.Contains(cmd.Args, "--no-cache") || cmd.Args[len(cmd.Args)-1] != "a.php" {
					t.Fatalf("unsafe invocation: %+v", cmd)
				}
				result := runner.Result{ExitCode: 2, Stdout: fmt.Appendf(nil, valid, filepath.Join(root, "a.php"))}
				switch name {
				case "missing PHP":
					result.Unavailable = true
				case "timeout":
					result.TimedOut = true
				case "crash":
					result.ExitCode = 1
				case "stderr":
					result.Stderr = []byte("Deprecated")
				case "malformed":
					result.Stdout = []byte("{")
				case "trailing":
					result.Stdout = append(result.Stdout, []byte(" {}")...)
				case "version":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"3.0.0"`, `"2.15.0"`, 1))
				case "foreign file":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), "a.php\"", "b.php\"", 1))
				case "exit mismatch":
					result.ExitCode = 0
				case "unknown error":
					result.ExitCode = 3
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"errors":[]`, `"errors":[{"fileName":"","message":"Out of memory"}]`, 1))
				case "unknown field":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"priority":3`, `"priority":3,"extra":1`, 1))
				}
				return result
			})
			selection := scope.Selection{Mode: "full", Files: []string{"a.php"}, LineMap: map[string][]scope.Range{"a.php": nil}}
			result := run(context.Background(), root, ".", selection, settings(t), bundle, process)
			if name == "valid" {
				if result.Check.Status != "passed" || len(result.Findings) != 1 || result.Findings[0].Severity != "warning" {
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
	selection := scope.Selection{Mode: "full", Files: []string{"a.php"}, LineMap: map[string][]scope.Range{"a.php": nil}}
	if result := run(context.Background(), root, ".", scope.Selection{}, settings(t), "", unused); result.Check.Status != "skipped" || *result.Check.Reason != "no_files" {
		t.Fatalf("empty selection ran: %+v", result)
	}
	cfg := settings(t)
	cfg.Enabled = false
	if result := run(context.Background(), root, ".", selection, cfg, "", unused); *result.Check.Reason != "disabled" {
		t.Fatalf("disabled checker ran: %+v", result)
	}
	if result := run(context.Background(), root, ".", selection, settings(t), "/nonexistent-phpwarden-bundle", unused); result.Check.Status != "unavailable" || result.Diagnostics[0].Code != "dependencies_missing" {
		t.Fatalf("missing bundle hidden: %+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := run(ctx, root, ".", selection, settings(t), "", unused); result.Diagnostics[0].Code != "checker_timeout" {
		t.Fatalf("cancelled check passed: %+v", result)
	}
}
