package rector

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

const messageRule = `DrupalRector\Drupal8\Rector\Deprecation\DrupalSetMessageRector`
const original = "<?php\ndrupal_set_message('hello');\necho 'untouched';\n"
const suggestion = "--- Original\n+++ New\n@@ -1,3 +1,3 @@\n <?php\n-drupal_set_message('hello');\n+\\Drupal::messenger()->addStatus('hello');\n echo 'untouched';\n"

func settings(t *testing.T) config.Rector {
	t.Helper()
	effective, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	result := effective.Config.Tools.Rector
	result.Enabled, result.Rules = true, []string{messageRule}
	return result
}

func projectConfig(settings config.Rector, composerRoot, targetPHP string) config.Config {
	return config.Config{Profile: "drupal", ComposerRoot: composerRoot, Framework: config.Framework{Version: "11.4.10"}, PHP: config.PHP{Target: config.Target{Min: targetPHP, Max: targetPHP}}, Tools: config.Tools{Rector: settings}}
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

func selection(lines []scope.Range) scope.Selection {
	return scope.Selection{Mode: "paths", Files: []string{"demo.module"}, LineMap: map[string][]scope.Range{"demo.module": lines}}
}

type processFunc func(context.Context, runner.Command) runner.Result

func (f processFunc) Run(ctx context.Context, cmd runner.Command) runner.Result { return f(ctx, cmd) }

func metadata() []byte {
	data, _ := json.Marshal(map[string]any{"version": 1, "runtime": "8.4", "php_version": "8.4.26", "tools": map[string]string{"rector/rector": Version, "palantirnet/drupal-rector": DrupalVersion, "phpstan/phpstan": PHPStanVersion, "webflo/drupal-finder": FinderVersion}})
	return data
}

func nativeReport(filename string) []byte {
	data, _ := json.Marshal(map[string]any{"totals": map[string]int{"changed_files": 1, "errors": 0}, "changed_files": []string{filename}, "file_diffs": []any{map[string]any{"file": filename, "diff": suggestion, "applied_rectors": []string{messageRule}}}})
	return data
}

func TestDiffReportsOnlyChangedOriginalLines(t *testing.T) {
	for _, tc := range []struct {
		diff, source string
		want         []int
	}{
		{suggestion, original, []int{2}},
		{"--- Original\n+++ New\n@@ -1,2 +1,3 @@\n <?php\n+echo 1;\n echo 2;\n", "<?php\necho 2;\n", []int{2}},
		{"--- Original\n+++ New\n@@ -1,3 +1,2 @@\n <?php\n-echo 1;\n echo 2;\n", "<?php\necho 1;\necho 2;\n", []int{2}},
	} {
		got, err := changedLines(tc.diff, []byte(tc.source))
		if err != nil || !slices.Equal(got, tc.want) {
			t.Fatalf("diff lines %v, %v; want %v", got, err, tc.want)
		}
	}
	for _, invalid := range []string{"", strings.Replace(suggestion, "-1,3", "-1,4", 1), strings.Replace(suggestion, " <?php", " <?forged", 1), strings.Replace(suggestion, "+1,3", "+1,2", 1), suggestion + "garbage\n"} {
		if _, err := changedLines(invalid, []byte(original)); err == nil {
			t.Fatal("invalid native diff accepted")
		}
	}
	for _, lines := range [][]scope.Range{nil, {{Start: 2, End: 2}}, {{Start: 3, End: 3}}} {
		got, err := decode(nativeReport("demo.module"), 2, "/project", selection(lines), map[string][]byte{"demo.module": []byte(original)})
		want := 1
		if len(lines) > 0 && lines[0].Start == 3 {
			want = 0
		}
		if err != nil || len(got) != want {
			t.Fatalf("scope %v: %v %v", lines, got, err)
		}
	}
}

func TestRectorFailuresCannotBecomeSuccessfulSuggestions(t *testing.T) {
	for _, name := range []string{"valid", "missing PHP", "timeout", "versions", "crash", "stderr", "malformed", "trailing", "partial", "foreign", "exit mismatch", "changed input"} {
		t.Run(name, func(t *testing.T) {
			root, bundle := t.TempDir(), t.TempDir()
			write(t, root, "demo.module", original)
			write(t, bundle, "vendor/rector/rector/bin/rector.php", "<?php")
			write(t, bundle, "vendor/autoload.php", "<?php")
			calls := 0
			process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
				calls++
				if calls == 1 {
					result := runner.Result{Stdout: metadata()}
					switch name {
					case "missing PHP":
						result.Unavailable = true
					case "timeout":
						result.TimedOut = true
					case "versions":
						result.Stdout = []byte(strings.Replace(string(result.Stdout), Version, "0.0.0", 1))
					}
					return result
				}
				if cmd.Name != "php" || cmd.Directory != root || !slices.Contains(cmd.Args, "--dry-run") || !slices.Contains(cmd.Args, "--output-format=json") || !slices.Contains(cmd.Args, "--memory-limit=1024M") {
					t.Fatalf("unsafe Rector command: %+v", cmd)
				}
				result := runner.Result{ExitCode: 2, Stdout: nativeReport(filepath.Join(root, "demo.module"))}
				switch name {
				case "crash":
					result.ExitCode = 1
				case "stderr":
					result.Stderr = []byte("unexpected warning")
				case "malformed":
					result.Stdout = []byte("{")
				case "trailing":
					result.Stdout = append(result.Stdout, []byte(" {}")...)
				case "partial":
					result.Stdout = []byte(strings.Replace(string(result.Stdout), `"errors":0`, `"errors":1`, 1))
				case "foreign":
					result.Stdout = nativeReport("foreign.php")
				case "exit mismatch":
					result.ExitCode = 0
				case "changed input":
					write(t, root, "demo.module", "<?php\necho 1;\n")
				}
				return result
			})
			result := run(context.Background(), root, selection(nil), projectConfig(settings(t), ".", "8.4"), bundle, process)
			if name == "valid" {
				if result.Check.Status != "passed" || len(result.Findings) != 1 || result.Findings[0].Severity != "warning" || *result.Findings[0].Line != 2 {
					t.Fatalf("valid dry-run rejected: %+v", result)
				}
			} else if result.Check.Status != "error" && result.Check.Status != "unavailable" || len(result.Findings) != 0 || len(result.Diagnostics) != 1 {
				t.Fatalf("failure accepted: %+v", result)
			}
		})
	}
}

func TestNativeDrupalRuleDryRunPreservesOriginal(t *testing.T) {
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/rector")
	write(t, root, "demo.module", original)
	result := run(context.Background(), root, selection(nil), projectConfig(settings(t), ".", "8.4"), bundle, runner.Exec{})
	if result.Check.Status != "passed" || len(result.Diagnostics) != 0 || len(result.Findings) != 1 || *result.Findings[0].Line != 2 {
		t.Fatalf("native Drupal Rector: %+v", result)
	}
	data, err := os.ReadFile(filepath.Join(root, "demo.module"))
	if err != nil || string(data) != original {
		t.Fatal("Rector dry-run changed original source")
	}
}

func TestNativeProfilePresetDryRunAndSkipRules(t *testing.T) {
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/rector")
	write(t, root, "demo.module", original)
	write(t, root, "vendor/composer/installed.json", `{"packages":[]}`)
	cfg := settings(t)
	cfg.Rules = []string{}
	for _, skip := range []bool{false, true} {
		if skip {
			cfg.SkipRules = []string{messageRule}
		}
		process := processFunc(func(ctx context.Context, cmd runner.Command) runner.Result {
			result := runner.Exec{}.Run(ctx, cmd)
			if result.ExitCode != 0 && result.ExitCode != 2 {
				t.Logf("native Rector failure: %s %s", result.Stdout, result.Stderr)
			}
			return result
		})
		result := run(context.Background(), root, selection(nil), projectConfig(cfg, ".", "8.4"), bundle, process)
		want := 1
		if skip {
			want = 0
		}
		if result.Check.Status != "passed" || len(result.Diagnostics) != 0 || len(result.Findings) != want {
			t.Fatalf("native profile preset (skip=%v): %+v", skip, result)
		}
		data, err := os.ReadFile(filepath.Join(root, "demo.module"))
		if err != nil || string(data) != original {
			t.Fatal("automatic preset changed original source")
		}
	}
}

func TestProfilePresetFallbackAndOverrides(t *testing.T) {
	for _, name := range []string{"automatic", "skip rules", "explicit config", "root config", "composer config", "discovery off", "PHP target", "Drupal target", "sets", "rules", "unknown version", "unknown profile", "missing config", "missing dependencies", "unsafe dependencies"} {
		t.Run(name, func(t *testing.T) {
			root, bundle := t.TempDir(), t.TempDir()
			write(t, root, "demo.module", original)
			cfg := settings(t)
			cfg.Rules = []string{}
			project := projectConfig(cfg, "platform", "8.4")
			write(t, root, "platform/vendor/composer/installed.json", `{"packages":[]}`)
			switch name {
			case "skip rules":
				project.Tools.Rector.SkipRules = []string{messageRule}
			case "explicit config":
				project.Framework.Version = "auto"
				project.Tools.Rector.Config = "custom.php"
				write(t, root, "custom.php", "<?php")
				write(t, root, "rector.php", "<?php")
			case "root config":
				write(t, root, "rector.php", "<?php")
				write(t, root, "platform/rector.php", "<?php")
			case "composer config":
				write(t, root, "platform/rector.dist.php", "<?php")
			case "discovery off":
				write(t, root, "rector.php", "<?php")
				project.Tools.Rector.DiscoverConfig = false
			case "PHP target":
				project.Tools.Rector.TargetPHP = "8.5"
			case "Drupal target":
				project.Tools.Rector.TargetDrupal = "12.0"
			case "sets":
				project.Tools.Rector.Sets = []string{"drupal-11.0"}
			case "rules":
				project.Tools.Rector.Rules = []string{messageRule}
			case "unknown version":
				project.Framework.Version = "auto"
			case "unknown profile":
				project.Profile = "laravel"
			case "missing config":
				project.Tools.Rector.Config = "missing.php"
			case "missing dependencies", "unsafe dependencies":
				metadata := filepath.Join(root, "platform/vendor/composer/installed.json")
				if err := os.Remove(metadata); err != nil {
					t.Fatal(err)
				}
				if name == "unsafe dependencies" {
					write(t, root, "metadata.json", `{"packages":[]}`)
					if err := os.Symlink(filepath.Join(root, "metadata.json"), metadata); err != nil {
						t.Fatal(err)
					}
				}
			}
			prepared, err := prepare(root, bundle, selection(nil), project)
			if name == "missing dependencies" || name == "unsafe dependencies" {
				if !errors.Is(err, errDependencies) {
					t.Fatalf("dependency error lost: %v", err)
				}
				return
			}
			if name == "unknown version" || name == "unknown profile" || name == "missing config" {
				if err == nil {
					t.Fatal("invalid preset/config silently accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			switch name {
			case "automatic", "skip rules", "discovery off":
				if prepared.Drupal != "11.4.10" || prepared.PHP != 80400 || prepared.Config != "" || !slices.Contains(prepared.Sets, drupalSet(bundle, "11.4")) || slices.Contains(prepared.Sets, drupalSet(bundle, "12.0")) {
					t.Fatalf("incorrect automatic preset: %+v", prepared)
				}
			case "explicit config", "root config", "composer config":
				want := map[string]string{"explicit config": "custom.php", "root config": "rector.php", "composer config": "platform/rector.dist.php"}[name]
				if prepared.Config != filepath.Join(root, want) || len(prepared.Sets) != 0 || prepared.Drupal != "" {
					t.Fatalf("project config was overridden: %+v", prepared)
				}
			case "PHP target":
				if prepared.PHP != 80500 || len(prepared.Sets) != 1 || prepared.Drupal != "" {
					t.Fatalf("PHP target was overridden: %+v", prepared)
				}
			case "Drupal target":
				if prepared.Drupal != "12.0.0" || !slices.Contains(prepared.Sets, drupalSet(bundle, "12.0")) {
					t.Fatalf("Drupal target was overridden: %+v", prepared)
				}
			case "sets":
				if !slices.Equal(prepared.Sets, []string{drupalSet(bundle, "11.0")}) || prepared.Drupal != "" {
					t.Fatalf("explicit sets were overridden: %+v", prepared)
				}
			case "rules":
				if len(prepared.Sets) != 0 || !slices.Equal(prepared.Rules, []string{messageRule}) || prepared.Drupal != "" {
					t.Fatalf("explicit rules were overridden: %+v", prepared)
				}
			}
		})
	}
}

func TestPrepareDiscoveryTargetsAndUnsafeInputs(t *testing.T) {
	root, bundle := t.TempDir(), t.TempDir()
	write(t, root, "demo.module", original)
	write(t, root, "nested/rector.dist.php", "<?php")
	cfg := settings(t)
	cfg.TargetDrupal, cfg.TargetPHP = "11.1", "8.5"
	prepared, err := prepare(root, bundle, selection(nil), projectConfig(cfg, "nested", "8.1"))
	if err != nil || !strings.HasSuffix(prepared.Config, "nested/rector.dist.php") || prepared.PHP != 80500 || prepared.Drupal != "11.1.0" || !slices.Contains(prepared.Sets, drupalSet(bundle, "11.1")) || slices.Contains(prepared.Sets, drupalSet(bundle, "11.2")) {
		t.Fatalf("upgrade targets/discovery: %+v %v", prepared, err)
	}
	before := ContextHash(root)
	write(t, root, "nested/imported.php", "<?php")
	if ContextHash(root) == before {
		t.Fatal("imported config change did not invalidate context")
	}
	if err := os.Symlink(filepath.Join(root, "nested/rector.dist.php"), filepath.Join(root, "rector.php")); err != nil {
		t.Fatal(err)
	}
	if _, err := prepare(root, bundle, selection(nil), projectConfig(cfg, "nested", "8.4")); err == nil {
		t.Fatal("symlink config accepted")
	}
	cfg.DiscoverConfig, cfg.DryRun = false, false
	if _, err := prepare(root, bundle, selection(nil), projectConfig(cfg, ".", "8.4")); err == nil {
		t.Fatal("apply mode accepted")
	}
}

func TestDisabledEmptyAndCancelledRectorNeverExecutes(t *testing.T) {
	unused := processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("unexpected Rector process")
		return runner.Result{}
	})
	cfg := settings(t)
	cfg.Enabled = false
	result := run(context.Background(), t.TempDir(), selection(nil), projectConfig(cfg, ".", "8.4"), "", unused)
	if result.Check.Status != "skipped" || *result.Check.Reason != "disabled" {
		t.Fatalf("disabled Rector: %+v", result)
	}
	cfg.Enabled = true
	result = run(context.Background(), t.TempDir(), scope.Selection{}, projectConfig(cfg, ".", "8.4"), "", unused)
	if result.Check.Status != "skipped" || *result.Check.Reason != "no_files" {
		t.Fatalf("empty Rector: %+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = run(ctx, t.TempDir(), selection(nil), projectConfig(cfg, ".", "8.4"), "", unused)
	if result.Check.Status != "error" || *result.Check.Reason != "checker_timeout" {
		t.Fatalf("cancelled Rector: %+v", result)
	}
}

func TestMonorepoRunsFromComposerRoot(t *testing.T) {
	root, bundle := t.TempDir(), t.TempDir()
	write(t, root, "demo.module", original)
	write(t, root, "platform/composer.json", `{"require":{}}`)
	write(t, bundle, "vendor/rector/rector/bin/rector.php", "<?php")
	write(t, bundle, "vendor/autoload.php", "<?php")
	process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
		if slices.Contains(cmd.Args, "--dry-run") {
			if cmd.Directory != filepath.Join(root, "platform") {
				t.Errorf("wrong Composer context: %s", cmd.Directory)
			}
			return runner.Result{ExitCode: 2, Stdout: nativeReport(filepath.Join(root, "demo.module"))}
		}
		return runner.Result{Stdout: metadata()}
	})
	result := run(context.Background(), root, selection(nil), projectConfig(settings(t), "platform", "8.4"), bundle, process)
	if result.Check.Status != "passed" || len(result.Findings) != 1 {
		t.Fatal(result)
	}
}

func TestNativeChangedFileListMayIncludeUnchangedVisitedFile(t *testing.T) {
	data := strings.Replace(string(nativeReport("demo.module")), `"changed_files":["demo.module"]`, `"changed_files":["demo.module","unchanged.php"]`, 1)
	selected := scope.Selection{Files: []string{"demo.module", "unchanged.php"}, LineMap: map[string][]scope.Range{"demo.module": nil, "unchanged.php": nil}}
	contents := map[string][]byte{"demo.module": []byte(original), "unchanged.php": []byte("<?php\n")}
	findings, err := decode([]byte(data), 2, "/project", selected, contents)
	if err != nil || len(findings) != 1 {
		t.Fatalf("native visited-file list was treated as an incomplete report: %v", err)
	}
	delete(contents, "unchanged.php")
	if _, err := decode([]byte(data), 2, "/project", selected, contents); err == nil {
		t.Fatal("unknown visited path accepted")
	}
}
