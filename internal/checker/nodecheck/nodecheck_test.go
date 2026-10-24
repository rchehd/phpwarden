package nodecheck

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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

func write(t *testing.T, root, name, content string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func settings(t *testing.T) config.Config {
	t.Helper()
	effective, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}},"scope":{"include":["src"]}}`))
	if err != nil {
		t.Fatal(err)
	}
	return effective.Config
}

func selection(files ...string) scope.Selection {
	result := scope.Selection{Mode: "full", Files: files, LineMap: map[string][]scope.Range{}}
	for _, name := range files {
		result.LineMap[name] = nil
	}
	return result
}

func TestNearestConfigsAndMissingConfig(t *testing.T) {
	root := t.TempDir()
	write(t, root, ".eslintrc.json", `{"rules":{}}`)
	write(t, root, "src/theme/eslint.config.cjs", `module.exports = [];`)
	cfg := settings(t)
	actual, err := groups(root, ".", "eslint", selection("src/a.js", "src/theme/b.js"), cfg.Tools.ESLint)
	want := []group{{Config: ".eslintrc.json", Files: []string{"src/a.js"}}, {Config: "src/theme/eslint.config.cjs", Files: []string{"src/theme/b.js"}}}
	if err != nil || !reflect.DeepEqual(actual, want) {
		t.Fatalf("nearest configs: %+v, %v", actual, err)
	}
	result := run(context.Background(), root, selection("src/a.css"), cfg, "stylelint", "", processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("checker without a config must not launch Node")
		return runner.Result{}
	}))
	if result.Check.Status != "skipped" || *result.Check.Reason != "no_config" || len(result.Diagnostics) != 1 {
		t.Fatalf("missing config: %+v", result)
	}
	write(t, root, "platform/.stylelintrc.json", `{"rules":{"block-no-empty":true}}`)
	actual, err = groups(root, "platform", "stylelint", selection("src/a.css"), cfg.Tools.Stylelint)
	if err != nil || actual[0].Config != "platform/.stylelintrc.json" {
		t.Fatalf("Composer root fallback: %+v, %v", actual, err)
	}
}

func TestCodeLinesAndWholeFileSyntax(t *testing.T) {
	root := t.TempDir()
	write(t, root, "eslint.config.cjs", `module.exports = [];`)
	cfg := settings(t)
	selected := selection("src/a.js")
	selected.LineMap["src/a.js"] = []scope.Range{{Start: 8, End: 8}}
	process := processFunc(func(_ context.Context, command runner.Command) runner.Result {
		for _, value := range command.Env {
			if strings.HasPrefix(value, "NODE_OPTIONS=") || strings.HasPrefix(value, "NODE_PATH=") {
				t.Fatalf("unsafe inherited Node environment: %s", value)
			}
		}
		data, _ := json.Marshal(helperReport{Version: 1, Tool: "eslint", ToolVersion: "9.39.5", Files: []fileResult{{Path: "src/a.js", Messages: []message{
			{Rule: "no-unused-vars", Severity: "error", Kind: "code", Text: "old", Line: 2, EndLine: 2, Column: 1},
			{Rule: "no-unused-vars", Severity: "warning", Kind: "code", Text: "new", Line: 8, EndLine: 8, Column: 1},
			{Rule: "eslint.parse_error", Severity: "error", Kind: "syntax", Text: "syntax", Line: 3, EndLine: 3, Column: 1},
		}}}})
		return runner.Result{Stdout: data}
	})
	result := run(context.Background(), root, selected, cfg, "eslint", "", process)
	if result.Check.Status != "failed" || len(result.Findings) != 2 || result.Findings[0].Message != "new" || result.Findings[1].Kind != "syntax" {
		t.Fatalf("line filtering: %+v", result)
	}
}

func TestDuplicatesAnalyzeOtherFilesAndAnchorChangedLine(t *testing.T) {
	root := t.TempDir()
	write(t, root, "src/a.module", "<?php\n")
	write(t, root, "src/b.module", "<?php\n")
	write(t, root, "src/vendor/ignored.php", "<?php\n")
	cfg := settings(t)
	cfg.Tools.JSCPD.FailOn = "warning"
	selected := selection("src/a.module")
	selected.LineMap["src/a.module"] = []scope.Range{{Start: 9, End: 9}}
	process := processFunc(func(_ context.Context, command runner.Command) runner.Result {
		var request struct {
			Groups []group `json:"groups"`
		}
		if err := json.Unmarshal(command.Input, &request); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(request.Groups[0].Files, []string{"src/a.module", "src/b.module"}) {
			t.Fatalf("wrong analysis inputs: %+v", request.Groups)
		}
		data, _ := json.Marshal(helperReport{Version: 1, Tool: "jscpd", ToolVersion: "4.3.0", Files: []fileResult{
			{Path: "src/a.module", Messages: []message{{Rule: "jscpd.duplicate", Severity: "warning", Kind: "code", Text: "duplicate", Line: 4, EndLine: 12, Column: 1}}},
			{Path: "src/b.module", Messages: []message{}},
		}})
		return runner.Result{Stdout: data}
	})
	result := run(context.Background(), root, selected, cfg, "jscpd", "", process)
	if result.Check.Status != "failed" || len(result.Findings) != 1 || *result.Findings[0].Line != 9 {
		t.Fatalf("duplicate overlap: %+v", result)
	}
}

func TestFailureReportsAndOmittedInputs(t *testing.T) {
	root := t.TempDir()
	write(t, root, "eslint.config.cjs", `module.exports = [];`)
	cfg := settings(t)
	for _, which := range []string{"missing node", "timeout", "stderr", "malformed", "omitted", "missing dependency", "foreign path"} {
		t.Run(which, func(t *testing.T) {
			process := processFunc(func(context.Context, runner.Command) runner.Result {
				data := helperReport{Version: 1, Tool: "eslint", ToolVersion: "9.39.5", Files: []fileResult{}}
				switch which {
				case "missing node":
					return runner.Result{Unavailable: true}
				case "timeout":
					return runner.Result{TimedOut: true}
				case "stderr":
					return runner.Result{Stderr: []byte("crash")}
				case "malformed":
					return runner.Result{Stdout: []byte("{} {}")}
				case "missing dependency":
					data.Error, data.Code = "missing package", "dependencies_missing"
				case "foreign path":
					data.Files = []fileResult{{Path: "../outside.js", Messages: []message{}}}
				}
				encoded, _ := json.Marshal(data)
				return runner.Result{Stdout: encoded}
			})
			result := run(context.Background(), root, selection("src/a.js"), cfg, "eslint", "", process)
			if result.Check.Status != "error" && result.Check.Status != "unavailable" || len(result.Findings) != 0 {
				t.Fatalf("failure looked clean: %+v", result)
			}
		})
	}
}

func TestContextTracksNativeConfigsDependenciesAndUnselectedSources(t *testing.T) {
	root := t.TempDir()
	before := ContextHash(root)
	for _, name := range []string{"eslint.config.cjs", "src/unselected.js", "node_modules/plugin/index.js", ".jscpd.json"} {
		write(t, root, name, "changed")
		after := ContextHash(root)
		if before == after {
			t.Fatalf("did not track %s", name)
		}
		before = after
	}
}

func TestActualNodeTools(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("Node.js is not installed")
	}
	bundle, _ := filepath.Abs("../../../tools/node")
	if _, err := os.Stat(filepath.Join(bundle, "node_modules")); err != nil {
		t.Fatal("install locked tools/node dependencies before running Node checker tests")
	}
	for _, id := range []string{"eslint", "stylelint", "jscpd"} {
		t.Run(id, func(t *testing.T) {
			root := t.TempDir()
			cfg := settings(t)
			name := "src/a.js"
			switch id {
			case "eslint":
				write(t, root, "eslint.config.cjs", `module.exports = [{ rules: { 'no-unused-vars': 'error' } }];`)
				write(t, root, name, "const unused = 1;\n")
			case "stylelint":
				name = "src/a.css"
				write(t, root, ".stylelintrc.json", `{"rules":{"block-no-empty":true}}`)
				write(t, root, name, "a {}\n")
			case "jscpd":
				name = "src/a [copy].module"
				content := "<?php\nfunction demo(): void {\n  $first = 1;\n  $second = 2;\n  echo $first + $second;\n}\n"
				write(t, root, name, content)
				write(t, root, "src/other.module", content)
				cfg.Tools.JSCPD.MinTokens, cfg.Tools.JSCPD.MinLines = 10, 3
			}
			result := run(context.Background(), root, selection(name), cfg, id, bundle, runner.Exec{})
			if len(result.Findings) == 0 || result.Check.Status == "error" || result.Check.Status == "unavailable" {
				t.Fatalf("actual %s: %+v", id, result)
			}
		})
	}
}

func TestJscpdRepeatedYamlBlocksKeepContiguousCloneRanges(t *testing.T) {
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/node")
	seed := uint32(9)
	rand := func(n uint32) uint32 { seed = seed*1664525 + 1013904223; return seed % n }
	names := []string{"src/a.yml", "src/b.yml", "src/c.yml"}
	for _, name := range names {
		var text strings.Builder
		text.WriteString("root:\n")
		for range 20 {
			fmt.Fprintf(&text, "  block_%d:\n", rand(5))
			for k := range 12 {
				fmt.Fprintf(&text, "    field_%d: value_%d\n", k, rand(3))
			}
		}
		write(t, root, name, text.String())
	}
	cfg := settings(t)
	cfg.Tools.JSCPD.MinTokens = 10
	cfg.Tools.JSCPD.MinLines = 2
	result := run(context.Background(), root, selection(names...), cfg, "jscpd", bundle, runner.Exec{})
	if result.Check.Status != "passed" || len(result.Diagnostics) > 0 || len(result.Findings) == 0 {
		t.Fatalf("repeated YAML clone ranges failed: %+v", result)
	}
}

func TestMissingFlatConfigImportIsDependencyFailure(t *testing.T) {
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/node")
	write(t, root, "eslint.config.mjs", "import plugin from 'phpwarden-missing-fixture-plugin'; export default [plugin];\n")
	write(t, root, "src/a.js", "let value = 1;\n")
	result := run(context.Background(), root, selection("src/a.js"), settings(t), "eslint", bundle, runner.Exec{})
	if result.Check.Status != "unavailable" || result.Diagnostics[0].Code != "dependencies_missing" || !strings.Contains(result.Diagnostics[0].Message, "npm ci") {
		t.Fatalf("missing project plugin was misclassified: %+v", result)
	}
}
