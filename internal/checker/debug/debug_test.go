package debug

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func settings() config.Debug {
	return config.Debug{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 10, Functions: []string{"var_dump", "print_r", "dpm", "custom\\debug"}}
}

func TestTokenizerRecognizesDirectCalls(t *testing.T) {
	for _, testcase := range []struct {
		name  string
		code  string
		count int
	}{
		{"plain", "var_dump($value);", 1},
		{"case and global", "\\VAR_DUMP($value);", 1},
		{"multiline", "var_dump\n(\n$value\n);", 1},
		{"trivia", "var_dump /* comment */ ($value);", 1},
		{"Drupal", "dpm($value);", 1},
		{"custom qualified", "custom\\debug($value);", 1},
		{"unknown qualified", "other\\var_dump($value);", 0},
		{"line comment", "// var_dump($value);", 0},
		{"block comment", "/* var_dump($value); */", 0},
		{"string", "$label = 'var_dump($value)';", 0},
		{"heredoc", "$label = <<<'TEXT'\nvar_dump($value);\nTEXT;", 0},
		{"method", "$object->var_dump($value);", 0},
		{"nullsafe", "$object?->var_dump($value);", 0},
		{"static", "Demo::var_dump($value);", 0},
		{"declaration", "function var_dump($value) {}", 0},
		{"reference declaration", "function & /* comment */ var_dump($value) {}", 0},
		{"new class", "new var_dump($value);", 0},
		{"attributes", "#[Other, var_dump(1)] class Demo {}", 0},
		{"callable acquisition", "$callable = var_dump(...);", 0},
		{"variadic call", "var_dump(...$values);", 1},
		{"variable call", "$var_dump($value);", 0},
		{"function alias", "use function var_dump as other; other($value);", 0},
		{"body of declaration", "function demo() { var_dump($value); }", 1},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "example.php"), []byte("<?php\n"+testcase.code+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			selection := scope.Selection{Files: []string{"example.php"}, LineMap: map[string][]scope.Range{"example.php": nil}}
			result := Run(context.Background(), root, selection, settings(), runner.Exec{})
			if result.Check.Status == "error" || result.Check.Status == "unavailable" || len(result.Findings) != testcase.count {
				t.Fatalf("unexpected result: %+v", result)
			}
			if len(result.Findings) > 0 && *result.Findings[0].Line != 2 {
				t.Fatalf("wrong call line: %+v", result.Findings[0])
			}
		})
	}
}

type fakeProcess func(context.Context, runner.Command) runner.Result

func (fake fakeProcess) Run(ctx context.Context, command runner.Command) runner.Result {
	return fake(ctx, command)
}

func TestMalformedReportsAndProcessFailures(t *testing.T) {
	selection := scope.Selection{Files: []string{"example.php"}, LineMap: map[string][]scope.Range{"example.php": nil}}
	for _, testcase := range []struct {
		name    string
		process runner.Result
		status  string
	}{
		{"missing executable", runner.Result{Unavailable: true}, "unavailable"},
		{"timeout", runner.Result{TimedOut: true}, "error"},
		{"nonzero despite valid report", runner.Result{ExitCode: 70, Stdout: []byte(`{"version":1,"runtime":"8.4","findings":[]}`)}, "error"},
		{"empty object", runner.Result{Stdout: []byte(`{}`)}, "error"},
		{"invalid JSON", runner.Result{Stdout: []byte(`not JSON`)}, "error"},
		{"null findings", runner.Result{Stdout: []byte(`{"version":1,"runtime":"8.4","findings":null}`)}, "error"},
		{"trailing data", runner.Result{Stdout: []byte(`{"version":1,"runtime":"8.4","findings":[]} {}`)}, "error"},
		{"outside selection", runner.Result{Stdout: []byte(`{"version":1,"runtime":"8.4","findings":[{"path":"other.php","line":1,"function":"var_dump"}]}`)}, "error"},
		{"unknown function", runner.Result{Stdout: []byte(`{"version":1,"runtime":"8.4","findings":[{"path":"example.php","line":1,"function":"unknown"}]}`)}, "error"},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			process := fakeProcess(func(context.Context, runner.Command) runner.Result { return testcase.process })
			result := Run(context.Background(), t.TempDir(), selection, settings(), process)
			if result.Check.Status != testcase.status || len(result.Diagnostics) == 0 || len(result.Findings) != 0 {
				t.Fatalf("invalid execution accepted: %+v", result)
			}
		})
	}
}

func TestScopeFilteringAndAdvisoryPolicy(t *testing.T) {
	selection := scope.Selection{Files: []string{"example.php"}, LineMap: map[string][]scope.Range{"example.php": {{Start: 4, End: 4}}}}
	process := fakeProcess(func(context.Context, runner.Command) runner.Result {
		return runner.Result{Stdout: []byte(`{"version":1,"runtime":"8.4","findings":[{"path":"example.php","line":3,"function":"var_dump"},{"path":"example.php","line":4,"function":"print_r"}]}`)}
	})
	cfg := settings()
	cfg.FailOn = "never"
	result := Run(context.Background(), t.TempDir(), selection, cfg, process)
	if result.Check.Status != "passed" || len(result.Findings) != 1 || *result.Findings[0].Line != 4 {
		t.Fatalf("scope/policy mismatch: %+v", result)
	}
}

func TestProjectSourceIsNeverExecuted(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "example.php"), []byte("<?php\nfile_put_contents('executed', 'bad');\nvar_dump(1);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	selection := scope.Selection{Files: []string{"example.php"}, LineMap: map[string][]scope.Range{"example.php": nil}}
	result := Run(context.Background(), root, selection, settings(), runner.Exec{})
	if len(result.Findings) != 1 {
		t.Fatalf("debug call missing: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "executed")); !os.IsNotExist(err) {
		t.Fatal("project source was executed")
	}
}

func drupalSettings() config.Debug {
	defaults, _ := profile.Lookup("drupal")
	return config.Debug{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 10,
		Functions: defaults.DebugFunctions, JSFunctions: defaults.DebugJSFunctions, TwigFunctions: defaults.DebugTwigFunctions}
}

func findings(t *testing.T, filename, code string, process runner.Process) []string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, filename), []byte(code), 0o644); err != nil {
		t.Fatal(err)
	}
	selection := scope.Selection{Files: []string{filename}, LineMap: map[string][]scope.Range{filename: nil}}
	result := Run(context.Background(), root, selection, drupalSettings(), process)
	if result.Check.Status == "error" || result.Check.Status == "unavailable" {
		t.Fatalf("unexpected result: %+v", result)
	}
	rows := []string{}
	for _, finding := range result.Findings {
		rows = append(rows, fmt.Sprintf("%d %s %s", *finding.Line, finding.Rule, finding.Message))
	}
	return rows
}

func TestDevelFunctionsDieAndPrintRReturnMode(t *testing.T) {
	for _, testcase := range []struct {
		name  string
		code  string
		count int
	}{
		{"Devel", "dvm($a); ksm($a); kpr($a); dd($a); ddm($a); ddebug_backtrace(); xdebug_break();", 7},
		{"die call", "die('stop');", 1},
		{"bare die", "if ($a) { DIE; }", 1},
		{"exit is legitimate", "exit(1);", 0},
		{"die method", "$object->die(); $object?->die(); Demo::die(); class A { function die() {} const DIE = 1; }", 0},
		{"print_r prints", "print_r($a);", 1},
		{"print_r with false", "print_r($a, FALSE); print_r($a, \\false);", 2},
		{"print_r returns", "$text = print_r($a, TRUE); $text = print_r($a, $return); $text = print_r(['x' => f($a, 1)], true);", 0},
		{"nested comma", "print_r(f($a, true));", 1},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			if rows := findings(t, "example.php", "<?php\n"+testcase.code+"\n", runner.Exec{}); len(rows) != testcase.count {
				t.Fatalf("expected %d findings, got %v", testcase.count, rows)
			}
		})
	}
}

func TestJavaScriptDebugLeftovers(t *testing.T) {
	for _, testcase := range []struct {
		name string
		code string
		want []string
	}{
		{"calls", "console.log(a);\nconsole . debug(a);\nwindow.console.log(a);\nconsole.info(a);\n", []string{"1 debug.js_call Debug call console.log() is left in JavaScript", "2 debug.js_call Debug call console.debug() is left in JavaScript"}},
		{"debugger", "function f() {\n  debugger;\n  debugger\n}\nconst o = {debugger: 1, x: o.debugger};\n", []string{"2 debug.debugger debugger statement is left in JavaScript", "3 debug.debugger debugger statement is left in JavaScript"}},
		{"comments and strings", "// console.log(a);\n/* debugger;\nconsole.log(a); */\nconst s = 'console.log(a)' + \"debugger;\";\n", []string{}},
		{"template", "const s = `console.log(a) ${console.log(b)} debugger`;\nconsole.trace();\n", []string{"1 debug.js_call Debug call console.log() is left in JavaScript", "2 debug.js_call Debug call console.trace() is left in JavaScript"}},
		{"nested template braces", "const s = `${ {a: `x${y}`}.a } console.log(a)`; console.dir(s);\n", []string{"1 debug.js_call Debug call console.dir() is left in JavaScript"}},
		{"regex and division", "const r = /console.log\\(x\\)/g; const d = a / b; console.log(d / 2);\n", []string{"1 debug.js_call Debug call console.log() is left in JavaScript"}},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			if rows := findings(t, "example.js", testcase.code, unexpectedProcess(t)); !slices.Equal(rows, testcase.want) {
				t.Fatalf("got %q, want %q", rows, testcase.want)
			}
		})
	}
}

func TestTwigDebugLeftovers(t *testing.T) {
	for _, testcase := range []struct {
		name string
		code string
		want []string
	}{
		{"print and tags", "<p>{{ dump(item) }}</p>\n{% set x = kint(item) %}\n{{- dump() -}}\n", []string{"1 debug.twig_call Debug function dump() is called in a Twig template", "2 debug.twig_call Debug function kint() is called in a Twig template", "3 debug.twig_call Debug function dump() is called in a Twig template"}},
		{"text comments strings verbatim", "dump(x) outside\n{# {{ dump(x) }} #}\n{{ 'dump(x)' ~ item.dump(x) }}\n{% verbatim %}{{ dump(x) }}{% endverbatim %}\n", []string{}},
		{"multiline", "{{\n  kint(\n item) }}\n", []string{"2 debug.twig_call Debug function kint() is called in a Twig template"}},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			if rows := findings(t, "card.html.twig", testcase.code, unexpectedProcess(t)); !slices.Equal(rows, testcase.want) {
				t.Fatalf("got %q, want %q", rows, testcase.want)
			}
		})
	}
}

func unexpectedProcess(t *testing.T) fakeProcess {
	return func(context.Context, runner.Command) runner.Result {
		t.Fatal("PHP must not run for JavaScript or Twig files")
		return runner.Result{}
	}
}

func TestSelectsJavaScriptAndTwigButNotMinified(t *testing.T) {
	files := []string{"a.module", "b.js", "c.min.js", "d.html.twig", "e.css", "F.JS"}
	selection := scope.Selection{Files: files, LineMap: map[string][]scope.Range{}}
	for _, name := range files {
		selection.LineMap[name] = nil
	}
	selected := Select(selection, drupalSettings(), "drupal")
	if !slices.Equal(selected.Files, []string{"a.module", "b.js", "d.html.twig", "F.JS"}) {
		t.Fatalf("wrong selection: %v", selected.Files)
	}
}

func TestSourceFindingsFollowChangedLines(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a.js"), []byte("console.log(1);\nconsole.log(2);\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	selection := scope.Selection{Files: []string{"a.js"}, LineMap: map[string][]scope.Range{"a.js": {{Start: 2, End: 2}}}}
	result := Run(context.Background(), root, selection, drupalSettings(), unexpectedProcess(t))
	if result.Check.Status != "failed" || len(result.Findings) != 1 || *result.Findings[0].Line != 2 || result.Runtime != nil {
		t.Fatalf("changed-line filter ignored: %+v", result)
	}
}
