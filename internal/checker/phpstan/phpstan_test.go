package phpstan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func settings(t *testing.T) config.PHPStan {
	t.Helper()
	cfg, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	result := cfg.Config.Tools.PHPStan
	result.Drupal = false
	return result
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

func TestActualPHPStanTypesDeprecationsAndLineFiltering(t *testing.T) {
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/phpstan")
	name := "new \nfile.module"
	write(t, root, name, "<?php\nfunction example(): int { return 'wrong'; }\n/** @deprecated */\nfunction old_function(): void {}\nold_function();\n")
	selection := scope.Selection{Mode: "full", Files: []string{name}, LineMap: map[string][]scope.Range{name: nil}}
	cfg := settings(t)
	check := func() {
		t.Helper()
		result := run(context.Background(), root, ".", selection, cfg, config.Target{Min: "8.4", Max: "8.4"}, "", bundle, runner.Exec{})
		want := 2
		if !cfg.Deprecations {
			want = 1
		}
		if selection.LineMap[name] != nil {
			want = 0
		}
		if len(result.Diagnostics) > 0 || len(result.Findings) != want || (result.Check.Status == "failed") != (want > 0) {
			t.Fatalf("actual PHPStan unexpected result: %+v", result)
		}
	}
	check()
	cfg.Deprecations = false
	check()
	selection.LineMap[name] = []scope.Range{{Start: 6, End: 6}}
	check()
}

func TestNativeConfigIncludesBaselineAndContext(t *testing.T) {
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/phpstan")
	write(t, root, "src/demo.php", "<?php\nfunction example(): int { return 'wrong'; }\n")
	write(t, root, "phpstan.neon.dist", "includes:\n    - quality/base.neon\n")
	write(t, root, "quality/base.neon", "parameters:\n    level: 0\n")
	write(t, root, "quality/baseline.neon", "parameters:\n    ignoreErrors:\n        -\n            message: '#^Function example\\(\\) should return int but returns string\\.$#'\n            count: 1\n            path: ../src/demo.php\n")
	selection := scope.Selection{Mode: "full", Files: []string{"src/demo.php"}, LineMap: map[string][]scope.Range{"src/demo.php": nil}}
	cfg := settings(t)
	cfg.Baseline = "quality/baseline.neon"
	result := run(context.Background(), root, ".", selection, cfg, config.Target{Min: "8.4", Max: "8.4"}, "", bundle, runner.Exec{})
	if len(result.Diagnostics) > 0 || result.Check.Status != "passed" || len(result.Findings) > 0 {
		t.Fatalf("native baseline failed: %+v", result)
	}
	before := ContextHash(root)
	write(t, root, "quality/base.neon", "parameters:\n    level: 6\n")
	if ContextHash(root) == before {
		t.Fatal("transitive config was not fingerprinted")
	}
	before = ContextHash(root)
	write(t, root, "vendor/example/dependency.php", "<?php\n")
	if ContextHash(root) == before {
		t.Fatal("new context dependency was not fingerprinted")
	}
	before = ContextHash(root)
	write(t, root, ".git/HEAD", "irrelevant git change")
	if ContextHash(root) != before {
		t.Fatal("Git metadata must not affect analysis context")
	}
	link := filepath.Join(root, "vendor/bin/tool")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../example/dependency.php", link); err != nil {
		t.Fatal(err)
	}
	if _, err := contextDigest(root); err != nil {
		t.Fatalf("symlinked dependency rejected: %v", err)
	}
	result = run(context.Background(), root, ".", selection, cfg, config.Target{Min: "8.4", Max: "8.4"}, "", bundle, runner.Exec{})
	if len(result.Diagnostics) > 0 || result.Check.Status != "passed" {
		t.Fatalf("symlinked project failed: %+v", result)
	}
	before = ContextHash(root)
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../example/missing.php", link); err != nil {
		t.Fatal(err)
	}
	if ContextHash(root) == before {
		t.Fatal("retargeted symlink was not fingerprinted")
	}
}
