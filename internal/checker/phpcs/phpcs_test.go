package phpcs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func settings(t *testing.T) config.PHPCS {
	t.Helper()
	cfg, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Config.Tools.PHPCS
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

func TestActualPHPCSStandardsAndCompatibility(t *testing.T) {
	bundle, _ := filepath.Abs("../../../tools/phpcs")
	root := t.TempDir()
	name := "demo.php"
	write(t, root, name, "<?php\nif (true) {\n\techo 1;\n}\n")
	selection := scope.Selection{Files: []string{name}, LineMap: map[string][]scope.Range{name: nil}}
	cfg := settings(t)
	cfg.Standards = []string{"PSR12"}
	cfg.Sniffs = []string{"Generic.WhiteSpace.DisallowTabIndent"}
	result := run(context.Background(), root, ".", selection, cfg, config.Target{Min: "8.4", Max: "8.4"}, bundle, runner.Exec{})
	if result.Check.Status != "failed" || len(result.Findings) != 1 || !strings.HasPrefix(result.Findings[0].Rule, "Generic.WhiteSpace.DisallowTabIndent") {
		t.Fatalf("actual PHPCS failed: %+v", result)
	}
	selection.LineMap[name] = []scope.Range{{Start: 4, End: 4}}
	if result = run(context.Background(), root, ".", selection, cfg, config.Target{Min: "8.4", Max: "8.4"}, bundle, runner.Exec{}); result.Check.Status != "passed" || len(result.Findings) != 0 {
		t.Fatalf("old style issue leaked into diff: %+v", result)
	}
	selection.LineMap[name] = nil
	write(t, root, name, "<?php\narray_find([], fn ($value) => true);\n")
	cfg.Standards = []string{"PHPCompatibility"}
	cfg.Sniffs = []string{}
	cfg.Compatibility.Enabled = true
	for _, target := range []string{"8.3", "8.4"} {
		result = run(context.Background(), root, ".", selection, cfg, config.Target{Min: target, Max: target}, bundle, runner.Exec{})
		if len(result.Diagnostics) > 0 || (len(result.Findings) > 0) != (target == "8.3") {
			t.Fatalf("compatibility target %s: %+v", target, result)
		}
	}
	cfg.Compatibility.Enabled = false
	result = run(context.Background(), root, ".", selection, cfg, config.Target{Min: "8.3", Max: "8.3"}, bundle, runner.Exec{})
	if len(result.Diagnostics) > 0 || len(result.Findings) != 1 {
		t.Fatalf("project-selected PHPCompatibility lost the shared target: %+v", result)
	}
	cfg = settings(t)
	cfg.Sniffs = []string{"Drupal.ControlStructures.ControlSignature"}
	write(t, root, name, "<?php\nif(true) {\n  echo 1;\n}\n")
	result = run(context.Background(), root, ".", selection, cfg, config.Target{Min: "8.4", Max: "8.4"}, bundle, runner.Exec{})
	if len(result.Diagnostics) > 0 || len(result.Findings) == 0 || !strings.HasPrefix(result.Findings[0].Rule, "Drupal.") {
		t.Fatalf("actual Drupal Coder failed: %+v", result)
	}
}

func TestRulesetContextTracksMissingConfigsAndCustomInputs(t *testing.T) {
	root := t.TempDir()
	write(t, root, "src/demo.php", "<?php\n")
	selection := scope.Selection{Files: []string{"src/demo.php"}, LineMap: map[string][]scope.Range{"src/demo.php": nil}}
	cfg := config.Config{Profile: "drupal"}
	cfg.Tools.PHPCS = settings(t)
	before := ContextHash(root, selection, cfg)
	write(t, root, "phpcs.xml", "<ruleset><rule ref=\"PSR12\"/></ruleset>")
	if after := ContextHash(root, selection, cfg); after == before {
		t.Fatal("new ancestor ruleset was not fingerprinted")
	}
	write(t, root, "src/.phpcs.xml", "<ruleset><rule ref=\"../standards/base.xml\"/></ruleset>")
	write(t, root, "standards/base.xml", "<ruleset><rule ref=\"PSR12\"/></ruleset>")
	prepared, err := prepare(root, ".", selection, cfg.Tools.PHPCS)
	if err != nil || prepared.Groups[0].Config != "src/.phpcs.xml" {
		t.Fatalf("nearest ruleset: %+v %v", prepared, err)
	}
	before = ContextHash(root, selection, cfg)
	write(t, root, "standards/base.xml", "<ruleset><rule ref=\"Drupal\"/></ruleset>")
	if ContextHash(root, selection, cfg) == before {
		t.Fatal("transitive ruleset was not fingerprinted")
	}
	cfg.Tools.PHPCS.ExtraInputs = []string{"standards"}
	write(t, root, "standards/fixture.xml", "broken fixture XML <")
	if _, err := prepare(root, ".", selection, cfg.Tools.PHPCS); err != nil {
		t.Fatalf("arbitrary extra input XML must only be fingerprinted: %v", err)
	}
	before = ContextHash(root, selection, cfg)
	write(t, root, "standards/extra.php", "<?php\n")
	if ContextHash(root, selection, cfg) == before {
		t.Fatal("new custom input was not fingerprinted")
	}
	write(t, root, "src/.phpcs.xml", "<ruleset><autoload>../../outside.php</autoload></ruleset>")
	if _, err = prepare(root, ".", selection, cfg.Tools.PHPCS); err == nil {
		t.Fatal("outside input reference accepted")
	}
	if err := os.Remove(filepath.Join(root, "src/.phpcs.xml")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "src/.phpcs.xml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err = prepare(root, ".", selection, cfg.Tools.PHPCS); err == nil {
		t.Fatal("nonregular ruleset candidate accepted")
	}
}
