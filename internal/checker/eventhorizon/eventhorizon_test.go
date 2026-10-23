package eventhorizon

import (
	"context"
	"encoding/json"
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

type processFunc func(context.Context, runner.Command) runner.Result

func (f processFunc) Run(ctx context.Context, command runner.Command) runner.Result {
	return f(ctx, command)
}

func settings(t *testing.T) config.EventHorizon {
	t.Helper()
	effective, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}},"tools":{"eventhorizon":{"enabled":true}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return effective.Config.Tools.EventHorizon
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func selected(names ...string) scope.Selection {
	result := scope.Selection{Files: names, LineMap: map[string][]scope.Range{}}
	for _, name := range names {
		result.LineMap[name] = nil
	}
	return result
}

func TestActualCachingAdvisoryAndDiffFiltering(t *testing.T) {
	root := t.TempDir()
	name := "custom/with space\nand newline.module"
	before := "<?php\nfunction demo() {\n  return ['#markup' => 'demo'];\n}\n\n"
	write(t, root, name, before)
	bundle, _ := filepath.Abs("../../../tools/eventhorizon/vendor")
	cfg := settings(t)
	result := run(context.Background(), root, selected(name), cfg, bundle, runner.Exec{})
	if result.Check.Status != "passed" || !slices.ContainsFunc(result.Findings, func(row report.Finding) bool {
		return row.Rule == "caching.missing_cache_metadata" && row.Severity == "warning"
	}) {
		t.Fatalf("advisory caching result: %+v", result)
	}
	cfg.FailOn = "warning"
	result = run(context.Background(), root, selected(name), cfg, bundle, runner.Exec{})
	if result.Check.Status != "failed" {
		t.Fatalf("warning policy did not block: %+v", result)
	}
	selection := selected(name)
	selection.LineMap[name] = []scope.Range{{Start: 5, End: 5}}
	result = run(context.Background(), root, selection, cfg, bundle, runner.Exec{})
	if result.Check.Status != "passed" || len(result.Findings) != 0 {
		t.Fatalf("old caching findings escaped the diff filter: %+v", result)
	}
	content, _ := os.ReadFile(filepath.Join(root, name))
	if string(content) != before {
		t.Fatal("analysis modified the source")
	}
}

func TestActualConfigUsesWholeDirectoryAndReportsGlobalFindings(t *testing.T) {
	root := t.TempDir()
	name := "config/site one/field.storage.node.field_lonely.yml"
	write(t, root, name, "field_name: field_lonely\nentity_type: node\ntype: string\n")
	cfg := settings(t)
	cfg.ConfigSync = []string{"config/site one"}
	bundle, _ := filepath.Abs("../../../tools/eventhorizon/vendor")
	selection := selected(name)
	selection.LineMap[name] = []scope.Range{}
	result := run(context.Background(), root, selection, cfg, bundle, runner.Exec{})
	if result.Check.Status != "passed" || len(result.Findings) != 1 || result.Findings[0].Rule != "config.unused_field" || result.Findings[0].Kind != "global" || result.Findings[0].Path != nil {
		t.Fatalf("config finding: %+v", result)
	}
	before := ContextHash(root, selection, cfg)
	write(t, root, "config/site one/node.type.article.yml", "name: Article\n")
	write(t, root, "config/site one/field.field.node.article.field_lonely.yml", "description: Editor help\n")
	if ContextHash(root, selection, cfg) == before {
		t.Fatal("new unselected config did not invalidate the context")
	}
	result = run(context.Background(), root, selection, cfg, bundle, runner.Exec{})
	if result.Check.Status != "passed" || len(result.Findings) != 0 {
		t.Fatalf("unselected field instance was omitted from analysis: %+v", result)
	}
	write(t, root, "config/site one/broken.yml", "broken: [\n")
	result = run(context.Background(), root, selection, cfg, bundle, runner.Exec{})
	if result.Check.Status != "error" || len(result.Findings) != 0 {
		t.Fatalf("partial config analysis looked clean: %+v", result)
	}
	write(t, root, "config/site one/broken.yml", "key: 1\nkey: 2\n")
	result = run(context.Background(), root, selection, cfg, bundle, runner.Exec{})
	if result.Check.Status != "error" {
		t.Fatal("duplicate config keys were accepted")
	}
}

func TestPrivateInputsAndStrictReportBoundary(t *testing.T) {
	for _, kind := range []string{"valid", "missing python", "timeout", "crash", "stderr", "malformed", "revision", "foreign", "line", "omitted files", "trailing JSON"} {
		t.Run(kind, func(t *testing.T) {
			root, bundle := t.TempDir(), t.TempDir()
			write(t, root, "custom/demo.module", "<?php\nreturn [];\n")
			write(t, bundle, "source.json", "{}")
			t.Setenv("PYTHONPATH", "/must/not/be/imported")
			var temporary string
			process := processFunc(func(_ context.Context, command runner.Command) runner.Result {
				if command.Name != "python3" || !slices.Contains(command.Args, "-I") || !slices.Contains(command.Args, "-S") || !slices.Contains(command.Args, "-B") || slices.ContainsFunc(command.Env, func(value string) bool { return strings.HasPrefix(value, "PYTHON") }) {
					t.Fatalf("unsafe Python invocation: %+v", command)
				}
				temporary = command.Directory
				private := command.Args[len(command.Args)-1]
				if private == root {
					t.Fatal("helper received the original project")
				}
				if data, err := os.ReadFile(filepath.Join(private, "custom/demo.module")); err != nil || string(data) != "<?php\nreturn [];\n" {
					t.Fatal("private source copy missing")
				}
				r := response{Version: 1, ToolVersion: Version, Revision: Revision, YAML: YAMLVersion, Files: []string{"custom/demo.module"}, ConfigDirs: []string{}, Findings: []finding{}}
				switch kind {
				case "missing python":
					return runner.Result{Unavailable: true}
				case "timeout":
					return runner.Result{TimedOut: true}
				case "crash":
					return runner.Result{ExitCode: 70}
				case "stderr":
					return runner.Result{Stderr: []byte("native details")}
				case "malformed":
					return runner.Result{Stdout: []byte("not json")}
				case "revision":
					r.Revision = "different"
				case "foreign":
					r.Findings = []finding{{File: "other.php", Line: 1, Rule: "caching.missing_cache_metadata", Severity: "warning", Message: "issue"}}
				case "line":
					r.Findings = []finding{{File: "custom/demo.module", Line: 100, Rule: "caching.missing_cache_metadata", Severity: "warning", Message: "issue"}}
				case "omitted files":
					r.Files = nil
				}
				data, _ := json.Marshal(r)
				if kind == "trailing JSON" {
					data = append(data, []byte("{}")...)
				}
				return runner.Result{Stdout: data}
			})
			result := run(context.Background(), root, selected("custom/demo.module"), settings(t), bundle, process)
			if (result.Check.Status == "passed") != (kind == "valid") {
				t.Fatalf("wrong result for %s: %+v", kind, result)
			}
			if _, err := os.Stat(temporary); !os.IsNotExist(err) {
				t.Fatal("private helper directory leaked")
			}
		})
	}
}

func TestInputsRejectSymlinksAndTrackUnselectedConfigContent(t *testing.T) {
	root := t.TempDir()
	name := "config/site/field.storage.node.field_demo.yml"
	write(t, root, name, "type: string\n")
	write(t, root, "config/site/other.yml", "key: original\n")
	cfg := settings(t)
	cfg.ConfigSync = []string{"config/site"}
	selection := selected(name)
	before := ContextHash(root, selection, cfg)
	write(t, root, "config/site/other.yml", "key: changed!\n")
	if before == ContextHash(root, selection, cfg) {
		t.Fatal("unselected config content is not fingerprinted")
	}
	if err := os.Symlink("other.yml", filepath.Join(root, "config/site/link.yml")); err != nil {
		t.Fatal(err)
	}
	if _, err := prepare(root, selection, cfg); err == nil {
		t.Fatal("config symlink was accepted")
	}
}

func TestHelperRejectsWarningsSwallowedByNativeAnalyzer(t *testing.T) {
	root, bundle := t.TempDir(), t.TempDir()
	write(t, root, "demo.module", "<?php\n")
	metadata, _ := json.Marshal(map[string]string{"version": Version, "revision": Revision, "pyyaml": YAMLVersion})
	write(t, bundle, "source.json", string(metadata))
	write(t, bundle, "yaml.py", "__version__ = '6.0.3'\nclass SafeLoader:\n    pass\n")
	for _, name := range []string{"eventhorizon/__init__.py", "eventhorizon/scanner/__init__.py", "eventhorizon/scanner/config_analyzer.py", "eventhorizon/scanner/config_validator.py"} {
		write(t, bundle, name, "")
	}
	write(t, bundle, "eventhorizon/__init__.py", "__version__ = '0.1.0'\n")
	write(t, bundle, "eventhorizon/scanner/caching_analyzer.py", "import logging\nclass SmartCachingAnalyzer:\n    def analyze_file(self, path):\n        logging.getLogger('EventHorizon.CachingAnalyzer').warning('skipped input')\n        return []\n")
	result := run(context.Background(), root, selected("demo.module"), settings(t), bundle, runner.Exec{})
	if result.Check.Status != "error" || len(result.Findings) != 0 || len(result.Diagnostics) == 0 {
		t.Fatalf("swallowed analyzer warning looked clean: %+v", result)
	}
}

func TestDisabledInapplicableAndMissingBundle(t *testing.T) {
	root, missing := t.TempDir(), filepath.Join(t.TempDir(), "missing")
	write(t, root, "demo.module", "<?php\n")
	process := processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("unnecessary analyzer invocation")
		return runner.Result{}
	})
	cfg := settings(t)
	cfg.Enabled = false
	result := run(context.Background(), root, selected("demo.module"), cfg, missing, process)
	if result.Check.Status != "skipped" || *result.Check.Reason != "disabled" {
		t.Fatalf("disabled checker: %+v", result)
	}
	cfg.Enabled, cfg.Caching = true, false
	result = run(context.Background(), root, selected("demo.module"), cfg, missing, process)
	if result.Check.Status != "skipped" || *result.Check.Reason != "no_files" {
		t.Fatalf("inapplicable checker: %+v", result)
	}
	cfg.Caching = true
	result = run(context.Background(), root, selected("demo.module"), cfg, missing, process)
	if result.Check.Status != "unavailable" || *result.Check.Reason != "dependencies_missing" {
		t.Fatalf("missing bundle: %+v", result)
	}
}
