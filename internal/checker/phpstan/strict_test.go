package phpstan

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func TestStrictRulesAreOnlyIncludedWhenRequested(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		root := t.TempDir()
		write(t, root, "demo.php", "<?php\n")
		cfg := settings(t)
		cfg.StrictRules = config.StrictRules{Enabled: enabled, All: false, Rules: map[string]bool{"disallowedLooseComparison": true}}
		process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
			if slices.Contains(cmd.Args, "analyse") {
				return runner.Result{Stdout: []byte(`{"totals":{"errors":0,"file_errors":0},"files":[],"errors":[]}`)}
			}
			var input struct {
				Includes      []string           `json:"includes"`
				StrictEnabled bool               `json:"strict_enabled"`
				StrictRules   config.StrictRules `json:"strict_rules"`
			}
			if err := json.Unmarshal(cmd.Input, &input); err != nil {
				t.Fatal(err)
			}
			included := slices.ContainsFunc(input.Includes, func(name string) bool { return strings.Contains(name, "phpstan-strict-rules/rules.neon") })
			if included != enabled || input.StrictEnabled != enabled || input.StrictRules.All || !input.StrictRules.Rules["disallowedLooseComparison"] {
				t.Fatalf("incorrect strict configuration: %+v", input)
			}
			return runner.Result{Stdout: metadataResponse()}
		})
		result := run(context.Background(), root, ".", scope.Selection{Files: []string{"demo.php"}, LineMap: map[string][]scope.Range{"demo.php": nil}}, cfg,
			config.Target{Min: "8.4", Max: "8.4"}, "", fakeBundle(t), process)
		if result.Check.Status != "passed" {
			t.Fatalf("strict preparation failed: %+v", result)
		}
	}
}

func TestActualStrictRulesSelection(t *testing.T) {
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/phpstan")
	write(t, root, "demo.php", "<?php\nfunction compare(int $left, int $right): bool {\n    return $left == $right;\n}\n")
	selected := scope.Selection{Mode: "full", Files: []string{"demo.php"}, LineMap: map[string][]scope.Range{"demo.php": nil}}
	cfg := settings(t)
	for _, current := range []struct {
		Settings config.StrictRules
		Want     int
	}{
		{config.StrictRules{All: true, Rules: map[string]bool{}}, 0},
		{config.StrictRules{Enabled: true, All: false, Rules: map[string]bool{"disallowedLooseComparison": true}}, 1},
		{config.StrictRules{Enabled: true, All: false, Rules: map[string]bool{}}, 0},
		{config.StrictRules{Enabled: true, All: true, Rules: map[string]bool{"disallowedLooseComparison": false}}, 0},
	} {
		cfg.StrictRules = current.Settings
		result := run(context.Background(), root, ".", selected, cfg, config.Target{Min: "8.4", Max: "8.4"}, "", bundle, runner.Exec{})
		if len(result.Diagnostics) != 0 || len(result.Findings) != current.Want {
			t.Fatalf("strict selection %+v: %+v", current.Settings, result)
		}
	}
}
