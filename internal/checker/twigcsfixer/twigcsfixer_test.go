package twigcsfixer

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func settings(t *testing.T) config.TwigCSFixer {
	t.Helper()
	cfg, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Config.Tools.TwigCSFixer
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

func rules(result []string) []string {
	slices.Sort(result)
	return result
}

func TestActualTwigCSFixerFallbackProjectConfigAndFilters(t *testing.T) {
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/twig-cs-fixer")
	name, broken := "templates/new \nfile.html.twig", "templates/broken.html.twig"
	write(t, root, name, "<p>{{title}}</p>\n{% include 'card.html.twig' with {title: title} %}\n{% trans %}Hello {{ name }}{% endtrans %}\n")
	write(t, root, broken, "<p>{{ title </p>\n")
	selection := scope.Selection{Mode: "full", Files: []string{broken, name}, LineMap: map[string][]scope.Range{name: nil, broken: nil}}
	check := func(cfg config.TwigCSFixer, want ...string) {
		t.Helper()
		result := run(context.Background(), root, selection, cfg, "drupal", bundle, runner.Exec{})
		actual := []string{}
		for _, finding := range result.Findings {
			if finding.Kind == "global" {
				if finding.Rule != "File.Invalid" || finding.Path != nil || !strings.HasPrefix(finding.Message, `Twig template "templates/broken.html.twig": Unable to tokenize file`) {
					t.Fatalf("invalid template finding: %+v", finding)
				}
			} else if *finding.Path != name || finding.Column == nil || finding.Severity != "error" {
				t.Fatalf("unexpected finding: %+v", finding)
			}
			actual = append(actual, finding.Rule)
		}
		if len(result.Diagnostics) > 0 || result.Check.Status != "failed" || !slices.Equal(rules(actual), rules(want)) {
			t.Fatalf("Twig CS Fixer result %v, want %v: %+v", actual, want, result)
		}
	}
	cfg := settings(t)
	check(cfg, "DelimiterSpacing.After", "DelimiterSpacing.Before", "File.Invalid")
	selection.LineMap[name] = []scope.Range{{Start: 2, End: 3}}
	check(cfg, "File.Invalid")
	selection.LineMap[name] = nil
	write(t, root, ".twig-cs-fixer.dist.php", "<?php\nreturn new TwigCsFixer\\Config\\Config();\n")
	write(t, root, ".twig-cs-fixer.php", "<?php\n$ruleset = new TwigCsFixer\\Ruleset\\Ruleset();\n$ruleset->addRule(new TwigCsFixer\\Rules\\Function\\IncludeFunctionRule());\nreturn (new TwigCsFixer\\Config\\Config())->setRuleset($ruleset);\n")
	check(cfg, "IncludeFunction.Error", "File.Invalid")
	cfg.DiscoverConfig = false
	check(cfg, "DelimiterSpacing.After", "DelimiterSpacing.Before", "File.Invalid")
	cfg.Config = ".twig-cs-fixer.dist.php"
	check(cfg, "DelimiterSpacing.After", "DelimiterSpacing.Before", "IncludeFunction.Error", "File.Invalid")
	write(t, root, "quality/bad.php", "<?php\necho 'noise';\nreturn [];\n")
	cfg.Config = "quality/bad.php"
	result := run(context.Background(), root, selection, cfg, "drupal", bundle, runner.Exec{})
	if result.Check.Status != "error" || result.Diagnostics[0].Code != "process_failed" || !strings.Contains(result.Diagnostics[0].Message, "must return") {
		t.Fatalf("invalid project config accepted: %+v", result)
	}
}

func TestContextHashIsIndependentOfProjectLocation(t *testing.T) {
	cfg := settings(t)
	cfg.Config, cfg.ExtraInputs = "missing.php", []string{"quality"}
	first, second := t.TempDir(), t.TempDir()
	if ContextHash(first, cfg) != ContextHash(second, cfg) {
		t.Fatal("Twig CS Fixer context depends on the absolute project path")
	}
}

func TestContextHashCoversConfigsAndExtraInputs(t *testing.T) {
	root := t.TempDir()
	cfg := settings(t)
	cfg.ExtraInputs = []string{"quality"}
	write(t, root, "quality/rules.php", "<?php\nreturn [];\n")
	before := ContextHash(root, cfg)
	for _, change := range []func(){
		func() { write(t, root, ".twig-cs-fixer.dist.php", "<?php\n") },
		func() { write(t, root, ".twig-cs-fixer.php", "<?php\n") },
		func() { write(t, root, ".twig-cs-fixer.php", "<?php\n// changed\n") },
		func() { write(t, root, "quality/rules.php", "<?php\nreturn ['changed'];\n") },
		func() { write(t, root, "quality/nested/added.php", "<?php\n") },
	} {
		change()
		if after := ContextHash(root, cfg); after == before {
			t.Fatal("Twig CS Fixer input change was not fingerprinted")
		} else {
			before = after
		}
	}
	write(t, root, "web/unrelated.php", "<?php\n")
	if ContextHash(root, cfg) != before {
		t.Fatal("unrelated project file changed the Twig CS Fixer context")
	}
	if err := os.Symlink("rules.php", filepath.Join(root, "quality/link.php")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := prepare(root, cfg); err == nil {
		t.Fatal("symlinked extra input accepted")
	}
}
