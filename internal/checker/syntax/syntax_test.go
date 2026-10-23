package syntax

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

func settings() config.Syntax {
	return config.Syntax{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 5}
}

func TestSyntaxFindingsIgnoreLineMapAndNormalizePaths(t *testing.T) {
	for _, ranges := range [][]scope.Range{nil, {}, {{Start: 10, End: 10}}} {
		root := t.TempDir()
		filename := "src/odd \nname.module"
		absolute := filepath.Join(root, filename)
		calls := 0
		process := processFunc(func(_ context.Context, command runner.Command) runner.Result {
			calls++
			if command.Name != "php" || command.Directory != root || command.Args[0] != "-n" {
				t.Fatalf("unsafe PHP invocation: %+v", command)
			}
			if slices.Contains(command.Args, "-r") {
				return runner.Result{Stdout: []byte("8.4.26")}
			}
			if !slices.Contains(command.Args, "-l") || command.Args[len(command.Args)-1] != absolute {
				t.Fatalf("expected one literal lint file: %+v", command)
			}
			return runner.Result{ExitCode: 255, Err: errors.New("exit status 255"), Stdout: []byte("Errors parsing " + absolute + "\n"),
				Stderr: []byte("Fatal error: Cannot redeclare function x() (previously declared in " + absolute + ":2) in " + absolute + " on line 3\n")}
		})
		result := Run(context.Background(), root, scope.Selection{Files: []string{filename}, LineMap: map[string][]scope.Range{filename: ranges}}, settings(), process)
		if calls != 2 || result.Check.Status != "failed" || len(result.Findings) != 1 || *result.Findings[0].Line != 3 ||
			*result.Findings[0].Path != filename || result.Findings[0].Kind != "syntax" || strings.Contains(result.Findings[0].Message, root) ||
			result.PHPVersion == nil || *result.PHPVersion != "8.4.26" {
			t.Fatalf("incorrect whole-file finding: %+v", result)
		}
	}
}

func TestSyntaxExecutionFailuresCannotPass(t *testing.T) {
	for _, tc := range []struct {
		name     string
		response runner.Result
		probe    bool
		status   string
	}{
		{"missing", runner.Result{Unavailable: true}, true, "unavailable"},
		{"timeout", runner.Result{TimedOut: true}, true, "error"},
		{"probe failure", runner.Result{ExitCode: 70}, true, "error"},
		{"invalid version", runner.Result{Stdout: []byte("8.4")}, true, "error"},
		{"probe stderr", runner.Result{Stdout: []byte("8.4.26"), Stderr: []byte("warning")}, true, "error"},
		{"lint missing", runner.Result{Unavailable: true}, false, "unavailable"},
		{"lint timeout", runner.Result{TimedOut: true}, false, "error"},
		{"empty success", runner.Result{}, false, "error"},
		{"lint crash", runner.Result{ExitCode: 70}, false, "error"},
		{"unknown failure", runner.Result{ExitCode: 255}, false, "error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			process := processFunc(func(_ context.Context, command runner.Command) runner.Result {
				if !tc.probe && slices.Contains(command.Args, "-r") {
					return runner.Result{Stdout: []byte("8.4.26")}
				}
				return tc.response
			})
			result := Run(context.Background(), t.TempDir(), scope.Selection{Files: []string{"demo.php"}}, settings(), process)
			if result.Check.Status != tc.status || len(result.Diagnostics) != 1 || len(result.Findings) != 0 {
				t.Fatalf("failure hidden: %+v", result)
			}
		})
	}
}

func TestMalformedLintFailuresAndResourceErrorsAreNotFindings(t *testing.T) {
	root := t.TempDir()
	absolute := filepath.Join(root, "demo.php")
	for _, message := range []string{
		"Parse error: bad in /other.php on line 3\n",
		"Parse error: bad in " + absolute + " on line 0\n",
		"Fatal error: Allowed memory size of 10 bytes exhausted in " + absolute + " on line 3\n",
		"Fatal error: Maximum execution time exceeded in " + absolute + " on line 3\n",
		"unrecognized error\n",
	} {
		process := processFunc(func(_ context.Context, command runner.Command) runner.Result {
			if slices.Contains(command.Args, "-r") {
				return runner.Result{Stdout: []byte("8.4.26")}
			}
			return runner.Result{ExitCode: 255, Stdout: []byte("Errors parsing " + absolute + "\n"), Stderr: []byte(message)}
		})
		result := Run(context.Background(), root, scope.Selection{Files: []string{"demo.php"}}, settings(), process)
		if result.Check.Status != "error" || len(result.Findings) > 0 {
			t.Fatalf("execution failure became a code finding: %+v", result)
		}
	}
}

func TestActualPHPDoesNotExecuteProjectAndChecksAllFiles(t *testing.T) {
	if _, err := exec.LookPath("php"); err != nil {
		t.Skip("PHP is not installed")
	}
	root := t.TempDir()
	files := map[string]string{
		"-good \nfile.php":  "<?php\nfile_put_contents(__DIR__ . '/executed', 'bad');\nrequire 'missing-autoload.php';\n",
		"bad.module":        "<?php\n$value = ;\n",
		"duplicate.install": "<?php\nfunction x() {}\nfunction x() {}\n",
	}
	for filename, source := range files {
		if err := os.WriteFile(filepath.Join(root, filename), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	selection := scope.Selection{Files: []string{"-good \nfile.php", "bad.module", "duplicate.install"}}
	for _, policy := range []string{"error", "never"} {
		cfg := settings()
		cfg.FailOn = policy
		result := Run(context.Background(), root, selection, cfg, runner.Exec{})
		want := "failed"
		if policy == "never" {
			want = "passed"
		}
		if result.Check.Status != want || len(result.Findings) != 2 || len(result.Diagnostics) != 0 {
			t.Fatalf("actual lint failed: %+v", result)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "executed")); !os.IsNotExist(err) {
		t.Fatal("lint executed project code")
	}
}

func TestDisabledOrEmptySyntaxDoesNotLaunchPHP(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		cfg := settings()
		cfg.Enabled = enabled
		result := Run(context.Background(), t.TempDir(), scope.Selection{}, cfg, processFunc(func(context.Context, runner.Command) runner.Result {
			t.Fatal("PHP must not run")
			return runner.Result{}
		}))
		if result.Check.Status != "skipped" {
			t.Fatalf("incorrect applicability: %+v", result)
		}
	}
}
