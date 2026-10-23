package phpstan

import (
	"context"
	_ "embed"
	"path"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

//go:embed drupal_usage.php
var drupalUsage []byte

const drupalUsageNeon = `services:
    -
        class: PHPWardenDrupalUsageProvider
        tags:
            - shipmonk.deadCode.memberUsageProvider
`

func SelectDeadCode(selection scope.Selection, cfg config.DeadCode, profileID string) scope.Selection {
	defaults, _ := profile.Lookup(profileID)
	return selection.Filter(cfg.Include, cfg.Exclude, func(name string) bool {
		return slices.Contains(defaults.PHPExtensions, strings.ToLower(path.Ext(name)))
	})
}

func DeadCodeInputRequired(cfg config.Config, selection scope.Selection, only []string) bool {
	return checker.Enabled("dead-code", cfg.Tools.DeadCode.Enabled, only) && len(SelectDeadCode(selection, cfg.Tools.DeadCode, cfg.Profile).Files) > 0
}

// RunDeadCode analyses the whole custom code, because a member is dead only if nothing uses it, and reports findings in the selection.
func RunDeadCode(ctx context.Context, root string, selection scope.Selection, cfg config.Config, dependencies string, process runner.Process) checker.Outcome {
	return deadCode(ctx, root, selection, cfg, dependencies, bundlePath(), process)
}

func deadCode(ctx context.Context, root string, selection scope.Selection, cfg config.Config, dependencies, bundle string, process runner.Process) checker.Outcome {
	settings := cfg.Tools.PHPStan
	dead := cfg.Tools.DeadCode
	settings.Enabled, settings.Required, settings.FailOn, settings.TimeoutSeconds, settings.MemoryLimitMB, settings.Deprecations =
		dead.Enabled, dead.Required, dead.FailOn, dead.TimeoutSeconds, dead.MemoryLimitMB, false
	defaults, _ := profile.Lookup(cfg.Profile)
	extensions := []string{}
	for _, extension := range defaults.PHPExtensions {
		extensions = append(extensions, strings.TrimPrefix(extension, "."))
	}
	return analyse(ctx, root, cfg.ComposerRoot, SelectDeadCode(selection, dead, cfg.Profile), settings, cfg.PHP.Target, dependencies, bundle, process, analysis{
		id: "dead-code", paths: slices.Clone(cfg.Scope.Include), extensions: extensions, includes: []string{"vendor/shipmonk/dead-code-detector/rules.neon"},
		identifier: "shipmonk.dead", severity: "warning", level: new(0), exclude: []string{"*/vendor/*", "*/node_modules/*"},
		files: map[string][]byte{"drupal-usage.php": drupalUsage, "drupal-usage.neon": []byte(drupalUsageNeon)},
	})
}
