package phpstan

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func deadCodeConfig(t *testing.T) config.Config {
	t.Helper()
	cfg, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}},` +
		`"scope":{"include":["src"]},"tools":{"phpstan":{"drupal":false}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Config
}

func TestActualDeadCodeUsesWholeRootsButReportsSelection(t *testing.T) {
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/phpstan")
	write(t, root, "src/Service.php", "<?php\n\nnamespace Acme;\n\nfinal class Service {\n  public function used(): int { return 1; }\n\n  public function unused(): int { return 2; }\n}\n")
	write(t, root, "src/Caller.php", "<?php\n\nnamespace Acme;\n\nfinal class Caller {\n  public function __construct(private Service $service) {}\n\n  public function run(): int { return $this->service->used(); }\n}\n")
	write(t, root, "src/caller.module", "<?php\n\n(new \\Acme\\Caller(new \\Acme\\Service()))->run();\n")
	write(t, root, "src/vendor/Library.php", "<?php\n\nfinal class Library { public function unused(): void {} }\n")
	check := func(cfg config.Config, files map[string][]scope.Range, status string, want ...string) {
		t.Helper()
		selection := scope.Selection{Mode: "diff", Files: slices.Sorted(func(yield func(string) bool) {
			for name := range files {
				if !yield(name) {
					return
				}
			}
		}), LineMap: files}
		result := deadCode(context.Background(), root, selection, cfg, "", bundle, runner.Exec{})
		got := []string{}
		for _, finding := range result.Findings {
			got = append(got, fmt.Sprintf("%s:%d %s %s", *finding.Path, *finding.Line, finding.Rule, finding.Severity))
		}
		if result.Check.Status != status || len(result.Diagnostics) > 0 || !slices.Equal(got, want) {
			t.Fatalf("got %s %q, want %s %q: %+v", result.Check.Status, got, status, want, result.Diagnostics)
		}
	}
	check(deadCodeConfig(t), map[string][]scope.Range{"src/Service.php": nil}, "passed", "src/Service.php:8 shipmonk.deadMethod warning")
	check(deadCodeConfig(t), map[string][]scope.Range{"src/Caller.php": nil}, "passed")
	check(deadCodeConfig(t), map[string][]scope.Range{"src/Service.php": {{Start: 6, End: 6}}}, "passed")
	cfg := deadCodeConfig(t)
	cfg.Tools.DeadCode.FailOn = "warning"
	check(cfg, map[string][]scope.Range{"src/Service.php": {{Start: 8, End: 8}}}, "failed", "src/Service.php:8 shipmonk.deadMethod warning")
	cfg = deadCodeConfig(t)
	cfg.Tools.DeadCode.Enabled = false
	if result := deadCode(context.Background(), root, scope.Selection{Files: []string{"src/Service.php"}, LineMap: map[string][]scope.Range{"src/Service.php": nil}}, cfg, "", bundle, runner.Exec{}); *result.Check.Reason != "disabled" {
		t.Fatalf("disabled dead code ran: %+v", result)
	}
}

func TestActualDeadCodeKnowsDrupalEntryPoints(t *testing.T) {
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/phpstan")
	write(t, root, "src/demo/demo.info.yml", "name: Demo\ntype: module\ncore_version_requirement: ^11\n")
	write(t, root, "src/demo/demo.routing.yml", "demo.page:\n  path: /demo\n  defaults:\n    _controller: '\\Acme\\Controller::page'\n    _title_callback: 'demo.controller:title'\n  requirements:\n    _permission: 'access content'\n")
	write(t, root, "src/demo/demo.services.yml", "services:\n  demo.controller:\n    class: Acme\\Controller\n")
	write(t, root, "src/demo/src/Controller.php", `<?php

namespace Acme;

final class Controller {
  public function __construct(private int $count = 0) {}

  public function page(): array { return ['#markup' => $this->count, '#submit' => ['::submitStep']]; }

  public function title(): string { return 'Demo'; }

  public function submitStep(): void {}

  #[\Drupal\Core\Hook\Attribute\Hook('form_alter')]
  public function formAlter(): void {}

  public function forgotten(): void {}
}
`)
	write(t, root, "src/demo/src/Hook.php", `<?php

namespace Acme;

#[\Drupal\Core\Hook\Attribute\Hook('cron', method: 'cron')]
final class Hook {
  public function cron(): void {}
}
`)
	cfg := deadCodeConfig(t)
	files := []string{"src/demo/src/Controller.php", "src/demo/src/Hook.php"}
	selection := scope.Selection{Mode: "full", Files: files, LineMap: map[string][]scope.Range{files[0]: nil, files[1]: nil}}
	result := deadCode(context.Background(), root, selection, cfg, "", bundle, runner.Exec{})
	if len(result.Diagnostics) > 0 || len(result.Findings) != 1 || *result.Findings[0].Line != 17 || result.Findings[0].Message != "Unused Acme\\Controller::forgotten" {
		t.Fatalf("Drupal entry points reported as dead: %+v %+v", result.Findings, result.Diagnostics)
	}
}
