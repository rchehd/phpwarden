package worker

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/checker/composeraudit"
	"github.com/rchehd/phpwarden/internal/checker/composervalidate"
	"github.com/rchehd/phpwarden/internal/checker/debug"
	"github.com/rchehd/phpwarden/internal/checker/deptrac"
	"github.com/rchehd/phpwarden/internal/checker/drupalconfig"
	"github.com/rchehd/phpwarden/internal/checker/eventhorizon"
	"github.com/rchehd/phpwarden/internal/checker/gitleaks"
	"github.com/rchehd/phpwarden/internal/checker/nodecheck"
	"github.com/rchehd/phpwarden/internal/checker/packagecheck"
	"github.com/rchehd/phpwarden/internal/checker/phpcs"
	"github.com/rchehd/phpwarden/internal/checker/phpmd"
	"github.com/rchehd/phpwarden/internal/checker/phpstan"
	"github.com/rchehd/phpwarden/internal/checker/rector"
	"github.com/rchehd/phpwarden/internal/checker/semgrep"
	"github.com/rchehd/phpwarden/internal/checker/syntax"
	"github.com/rchehd/phpwarden/internal/checker/twigcsfixer"
	"github.com/rchehd/phpwarden/internal/checker/whitespace"
	yamlcheck "github.com/rchehd/phpwarden/internal/checker/yaml"
	"github.com/rchehd/phpwarden/internal/compatibility"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/profile/drupal"
	"github.com/rchehd/phpwarden/internal/progress"
	"github.com/rchehd/phpwarden/internal/protocol"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/runrequest"
	"github.com/rchehd/phpwarden/internal/scope"
)

func Check(ctx context.Context, location config.Location, effective config.Effective, options scope.Options, only []string, process runner.Process) report.Report {
	cfg := effective.Config
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Runner.TimeoutSeconds)*time.Second)
	defer cancel()
	progress.Stage(ctx, "Reading Git changes")
	selection, err := scope.Build(ctx, options)
	if err != nil {
		if ctx.Err() != nil {
			result := report.New()
			result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "worker_cancelled", Stage: "worker", Message: ctx.Err().Error()})
			result.Finalize()
			return result
		}
		return report.InputError("scope", err)
	}
	result := newReport(cfg, selection)
	failure := func(code string, err error) report.Report {
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "worker", Message: err.Error()})
		result.Finalize()
		return result
	}
	progress.Stage(ctx, "Preparing analysis inputs")
	before, err := runrequest.Snapshot(location, selection, only)
	if err != nil {
		return failure("input_read", err)
	}
	if before["config"] != effective.SourceHash {
		return failure("stale", fmt.Errorf("configuration changed after loading"))
	}
	if err := discovery.Unchanged(location, effective); err != nil {
		return failure("stale", err)
	}
	audit := func(ctx context.Context, settings config.ComposerAudit) checker.Outcome {
		if cached, _ := ctx.Value(cachedAuditKey{}).(bool); cached {
			settings.Offline = true
		}
		cacheDir, _ := composeraudit.CacheDir()
		return composeraudit.Run(ctx, config.ComposerDir(location.Root, cfg), settings, composeraudit.NativeFetcher(composeraudit.Bundle(), process), cacheDir, time.Now())
	}
	result = analyze(ctx, location, cfg, effective.Project, selection, only, audit, process)
	validateRuntime(&result, effective)
	progress.Stage(ctx, "Verifying project inputs are unchanged")
	after, err := runrequest.Snapshot(location, selection, only)
	if err != nil {
		return failure("stale", err)
	}
	refreshed, err := scope.Build(ctx, options)
	if err != nil {
		return failure("stale", err)
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(selection, refreshed) {
		return failure("stale", fmt.Errorf("project inputs changed during the check; run it again"))
	}
	if err := discovery.Unchanged(location, effective); err != nil {
		return failure("stale", err)
	}
	return result
}

func newReport(cfg config.Config, selection scope.Selection) report.Report {
	result := report.New()
	result.Profile, result.Scope = &cfg.Profile, &selection
	result.Environment = &report.Environment{RequestedRuntime: cfg.PHP.Runtime, TargetMin: cfg.PHP.Target.Min, TargetMax: cfg.PHP.Target.Max, Protocol: protocol.Version}
	if cfg.Framework.Version != "auto" && cfg.Framework.Version != "" {
		result.Environment.FrameworkVersion = &cfg.Framework.Version
	}
	return result
}

// Auditor runs composer-audit; a network-less worker delegates it to the launcher.
type Auditor func(context.Context, config.ComposerAudit) checker.Outcome

func analyze(ctx context.Context, location config.Location, cfg config.Config, project *profile.Facts, selection scope.Selection, only []string, audit Auditor, process runner.Process) report.Report {
	result := newReport(cfg, selection)
	defaults, ok := profile.Lookup(cfg.Profile)
	if !ok {
		return report.InputError("profile", fmt.Errorf("unsupported profile %q", cfg.Profile))
	}
	if err := checker.ValidateOnly(only); err != nil {
		return report.InputError("selection", err)
	}
	outcomes := runCheckers(ctx, checker.Requested(only), cfg.Runner.Concurrency, func(id string) checker.Outcome {
		var outcome checker.Outcome
		if id == "phpstan" {
			settings := cfg.Tools.PHPStan
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			dependencies := ""
			if project != nil && project.Installed != nil && project.Installed.Error != nil {
				dependencies = *project.Installed.Error
			}
			outcome = phpstan.Run(ctx, location.Root, cfg.ComposerRoot, phpstan.Select(selection, settings, cfg.Profile), settings, cfg.PHP.Target, dependencies, process)
		} else if id == "eslint" || id == "stylelint" || id == "jscpd" {
			settings := nodecheck.Settings(cfg, id)
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			current := cfg
			switch id {
			case "eslint":
				current.Tools.ESLint = settings
			case "stylelint":
				current.Tools.Stylelint = settings
			case "jscpd":
				current.Tools.JSCPD.Enabled = settings.Enabled
			}
			outcome = nodecheck.Run(ctx, location.Root, selection, current, id, process)
		} else if id == "composer-require-checker" || id == "composer-unused" {
			current := cfg
			if id == "composer-require-checker" {
				current.Tools.ComposerRequireChecker.Enabled = checker.Enabled(id, cfg.Tools.ComposerRequireChecker.Enabled, only)
			} else {
				current.Tools.ComposerUnused.Enabled = checker.Enabled(id, cfg.Tools.ComposerUnused.Enabled, only)
			}
			outcome = packagecheck.Run(ctx, location.Root, selection, current, id, process)
		} else if id == "deptrac" {
			settings := cfg.Tools.Deptrac
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			outcome = deptrac.Run(ctx, location.Root, cfg.ComposerRoot, selection, settings, process)
		} else if id == "rector" {
			current := cfg
			current.Tools.Rector.Enabled = checker.Enabled(id, cfg.Tools.Rector.Enabled, only)
			outcome = rector.Run(ctx, location.Root, selection, current, process)
		} else if id == "semgrep" {
			settings := cfg.Tools.Semgrep
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			outcome = semgrep.Run(ctx, location.Root, cfg.ComposerRoot, selection, settings, process)
		} else if id == "eventhorizon" {
			settings := cfg.Tools.EventHorizon
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			outcome = eventhorizon.Run(ctx, location.Root, selection, settings, process)
		} else if id == "gitleaks" {
			settings := cfg.Tools.Gitleaks
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			outcome = gitleaks.Run(ctx, location.Root, cfg.ComposerRoot, selection, settings, process)
		} else if id == "dead-code" {
			cfg := cfg
			cfg.Tools.DeadCode.Enabled = checker.Enabled(id, cfg.Tools.DeadCode.Enabled, only)
			dependencies := ""
			if project != nil && project.Installed != nil && project.Installed.Error != nil {
				dependencies = *project.Installed.Error
			}
			outcome = phpstan.RunDeadCode(ctx, location.Root, selection, cfg, dependencies, process)
		} else if id == "composer-audit" {
			settings := cfg.Tools.ComposerAudit
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			outcome = audit(ctx, settings)
		} else if id == "composer-validate" {
			settings := cfg.Tools.ComposerValidate
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			outcome = composervalidate.Run(ctx, location.Root, composervalidate.Select(selection, settings, cfg.ComposerRoot), settings, process)
		} else if id == "twig-cs-fixer" {
			settings := cfg.Tools.TwigCSFixer
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			outcome = twigcsfixer.Run(ctx, location.Root, cfg.ComposerRoot, twigcsfixer.Select(selection, settings, cfg.Profile), settings, cfg.Profile, process)
		} else if id == "phpcs" {
			settings := cfg.Tools.PHPCS
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			outcome = phpcs.Run(ctx, location.Root, cfg.ComposerRoot, phpcs.Select(selection, settings, cfg.Profile), settings, cfg.PHP.Target, process)
		} else if id == "phpmd" {
			settings := cfg.Tools.PHPMD
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			outcome = phpmd.Run(ctx, location.Root, cfg.ComposerRoot, phpmd.Select(selection, settings, cfg.Profile), settings, process)
		} else if id == "drupal-config" {
			settings := cfg.Tools.DrupalConfig
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			selected := selection.Filter(settings.Include, settings.Exclude, func(filename string) bool {
				return cfg.Profile == "drupal" && drupal.ModuleConfigFile(filename, settings.Directories, settings.ModuleRoots)
			})
			outcome = drupalconfig.Run(ctx, location.Root, selected, settings)
		} else if id == "whitespace" {
			settings := cfg.Tools.Whitespace
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			selected := selection.Filter(settings.Include, settings.Exclude, func(string) bool { return true })
			outcome = whitespace.Run(ctx, location.Root, selected, settings)
		} else if id == "yaml" {
			settings := cfg.Tools.YAML
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			selected := selection.Filter(settings.Include, settings.Exclude, func(filename string) bool {
				extension := strings.ToLower(filepath.Ext(filename))
				return extension == ".yml" || extension == ".yaml"
			})
			outcome = yamlcheck.Run(ctx, location.Root, selected, settings, process)
		} else if id == "syntax" {
			settings := cfg.Tools.Syntax
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			extensions := append(slices.Clone(defaults.PHPExtensions), settings.ExtraExtensions...)
			selected := selection.Filter(settings.Include, settings.Exclude, func(filename string) bool {
				return slices.Contains(extensions, strings.ToLower(filepath.Ext(filename)))
			})
			outcome = syntax.Run(ctx, location.Root, selected, settings, process)
		} else {
			settings := cfg.Tools.Debug
			settings.Enabled = checker.Enabled(id, settings.Enabled, only)
			outcome = debug.Run(ctx, location.Root, debug.Select(selection, settings, cfg.Profile), settings, process)
		}
		return outcome
	})
	for _, outcome := range outcomes {
		result.Checks = append(result.Checks, outcome.Check)
		result.Findings = append(result.Findings, outcome.Findings...)
		result.Diagnostics = append(result.Diagnostics, outcome.Diagnostics...)
		if outcome.Advisories != nil {
			result.Environment.Advisories = outcome.Advisories
		}
		if outcome.Runtime != nil {
			if *outcome.Runtime != cfg.PHP.Runtime || result.Environment.PHPRuntime != nil && *outcome.Runtime != *result.Environment.PHPRuntime ||
				result.Environment.PHPVersion != nil && outcome.PHPVersion != nil && *result.Environment.PHPVersion != *outcome.PHPVersion {
				result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "runtime_mismatch", Stage: "worker", Message: fmt.Sprintf("%s checker PHP runtime does not match the configured runtime or other checkers", outcome.Check.ID)})
			}
			result.Environment.PHPRuntime = outcome.Runtime
			if outcome.PHPVersion != nil {
				result.Environment.PHPVersion = outcome.PHPVersion
			}
		}
	}
	if ctx.Err() != nil {
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "worker_cancelled", Stage: "worker", Message: ctx.Err().Error()})
	}
	result.Finalize()
	return result
}

func validateRuntime(result *report.Report, effective config.Effective) {
	if result.Environment == nil {
		return
	}
	for _, pending := range effective.Pending {
		if pending == "php.runtime.patch" || pending == "php.target.patch" || pending == "framework.installed_version" {
			if pending != "php.runtime.patch" || result.Environment.PHPVersion == nil {
				result.Environment.CompatibilityPending = append(result.Environment.CompatibilityPending, pending)
			}
		}
	}
	if effective.Project != nil && len(effective.Project.Warnings) > 0 {
		result.Environment.DiscoveryWarnings = slices.Clone(effective.Project.Warnings)
	}
	if effective.Project == nil || len(effective.Project.PHP) == 0 || result.Environment.PHPRuntime == nil {
		return
	}
	var err error
	if result.Environment.PHPVersion == nil {
		err = fmt.Errorf("worker must report its full PHP version to verify Composer requirements")
	} else {
		err = compatibility.CheckRuntime(effective.Project, *result.Environment.PHPVersion)
	}
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "php_compatibility", Stage: "worker", Message: err.Error()})
		result.Finalize()
	}
}
