package debug

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
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
