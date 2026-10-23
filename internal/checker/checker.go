package checker

import (
	"fmt"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/edit"
	"github.com/rchehd/phpwarden/internal/report"
)

type Outcome struct {
	Check       report.Check
	Findings    []report.Finding
	Diagnostics []report.Diagnostic
	Runtime     *string
	PHPVersion  *string
	Advisories  *report.Advisories
	Edits       []edit.Edit
}

func IDs() []string {
	return []string{"composer-audit", "composer-require-checker", "composer-unused", "composer-validate", "dead-code", "debug", "deptrac", "drupal-config", "eslint", "eventhorizon", "gitleaks", "jscpd", "phpcs", "phpmd", "phpstan", "rector", "semgrep", "stylelint", "syntax", "twig-cs-fixer", "whitespace", "yaml"}
}

func ParseOnly(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	ids := strings.Split(value, ",")
	if err := ValidateOnly(ids); err != nil {
		return nil, err
	}
	slices.Sort(ids)
	return ids, nil
}

func ValidateOnly(ids []string) error {
	seen := make(map[string]bool)
	for _, id := range ids {
		if !slices.Contains(IDs(), id) || seen[id] {
			return fmt.Errorf("unknown or duplicate checker %q; available: %s", id, strings.Join(IDs(), ", "))
		}
		seen[id] = true
	}
	return nil
}

func Requested(only []string) []string {
	if len(only) == 0 {
		return IDs()
	}
	return slices.Clone(only)
}

func Enabled(id string, configured bool, only []string) bool {
	if len(only) > 0 {
		return slices.Contains(only, id)
	}
	return configured
}

func AnyEnabled(cfg config.Config, only []string) bool {
	return Enabled("composer-require-checker", cfg.Tools.ComposerRequireChecker.Enabled, only) || Enabled("composer-unused", cfg.Tools.ComposerUnused.Enabled, only) || Enabled("deptrac", cfg.Tools.Deptrac.Enabled, only) || Enabled("rector", cfg.Tools.Rector.Enabled, only) || Enabled("semgrep", cfg.Tools.Semgrep.Enabled, only) || Enabled("eventhorizon", cfg.Tools.EventHorizon.Enabled, only) || Enabled("gitleaks", cfg.Tools.Gitleaks.Enabled, only) || Enabled("eslint", cfg.Tools.ESLint.Enabled, only) || Enabled("stylelint", cfg.Tools.Stylelint.Enabled, only) || Enabled("jscpd", cfg.Tools.JSCPD.Enabled, only) || Enabled("composer-audit", cfg.Tools.ComposerAudit.Enabled, only) || Enabled("composer-validate", cfg.Tools.ComposerValidate.Enabled, only) || Enabled("dead-code", cfg.Tools.DeadCode.Enabled, only) || Enabled("debug", cfg.Tools.Debug.Enabled, only) || Enabled("drupal-config", cfg.Tools.DrupalConfig.Enabled, only) || Enabled("phpcs", cfg.Tools.PHPCS.Enabled, only) || Enabled("phpmd", cfg.Tools.PHPMD.Enabled, only) || Enabled("phpstan", cfg.Tools.PHPStan.Enabled, only) || Enabled("syntax", cfg.Tools.Syntax.Enabled, only) || Enabled("twig-cs-fixer", cfg.Tools.TwigCSFixer.Enabled, only) || Enabled("yaml", cfg.Tools.YAML.Enabled, only) || Enabled("whitespace", cfg.Tools.Whitespace.Enabled, only)
}
