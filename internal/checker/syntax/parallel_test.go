package syntax

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func TestParallelLintNativeCoverageAndNoExecution(t *testing.T) {
	bundle, _ := filepath.Abs("../../../tools/parallel-lint")
	root := t.TempDir()
	for name, data := range map[string]string{"good.module": "<?php\nfile_put_contents(__DIR__.'/executed','bad');\n", "bad.install": "<?php\n$value = ;\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	cfg := settings()
	cfg.Backend = "parallel-lint"
	cfg.ParallelJobs = 2
	selected := scope.Selection{Files: []string{"good.module", "bad.install"}}
	result := runParallel(context.Background(), root, selected, cfg, bundle, runner.Exec{})
	if result.Check.Status != "failed" || len(result.Findings) != 1 || *result.Findings[0].Path != "bad.install" || *result.Findings[0].Line != 2 {
		t.Fatalf("native parallel lint: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(root, "executed")); !os.IsNotExist(err) {
		t.Fatal("lint executed project source")
	}
	os.WriteFile(filepath.Join(root, "good.module"), []byte("<?php // lint > 99.0\n$value = ;\n"), 0600)
	result = runParallel(context.Background(), root, selected, cfg, bundle, runner.Exec{})
	if result.Check.Status != "error" || len(result.Findings) != 0 {
		t.Fatalf("native skip accepted: %+v", result)
	}
}

func TestParallelLintRejectsPartialAndFailedReports(t *testing.T) {
	root, bundle := t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(bundle, "vendor"), 0700)
	os.WriteFile(filepath.Join(bundle, "vendor/autoload.php"), []byte("<?php"), 0600)
	os.WriteFile(filepath.Join(root, "demo.php"), []byte("<?php"), 0600)
	cfg := settings()
	cfg.Backend = "parallel-lint"
	cfg.ParallelJobs = 2
	for _, name := range []string{"valid", "partial", "version", "trailing", "timeout", "stderr", "unavailable", "resource"} {
		t.Run(name, func(t *testing.T) {
			process := processFunc(func(context.Context, runner.Command) runner.Result {
				data := map[string]any{"version": 1, "tool_version": "1.4.0", "php_version": "8.4.26", "checked": []string{filepath.Join(root, "demo.php")}, "errors": []any{}}
				switch name {
				case "partial":
					data["checked"] = []string{}
				case "version":
					data["tool_version"] = "1.0.0"
				case "resource":
					data["errors"] = []any{map[string]any{"file": filepath.Join(root, "demo.php"), "line": 1, "message": "Allowed memory size exhausted"}}
				}
				output, _ := json.Marshal(data)
				r := runner.Result{Stdout: output}
				switch name {
				case "trailing":
					r.Stdout = append(output, []byte("{}")...)
				case "timeout":
					r.TimedOut = true
				case "stderr":
					r.Stderr = []byte("warning")
				case "unavailable":
					r.Unavailable = true
				}
				return r
			})
			result := runParallel(context.Background(), root, scope.Selection{Files: []string{"demo.php"}}, cfg, bundle, process)
			if name == "valid" {
				if result.Check.Status != "passed" {
					t.Fatalf("valid report: %+v", result)
				}
			} else if result.Check.Status != "error" && result.Check.Status != "unavailable" || len(result.Findings) != 0 {
				t.Fatalf("failed report passed: %+v", result)
			}
		})
	}
}
