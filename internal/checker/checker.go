package checker

import (
	"fmt"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
)

type Outcome struct {
	Check       report.Check
	Findings    []report.Finding
	Diagnostics []report.Diagnostic
	Runtime     *string
	PHPVersion  *string
	Advisories  *report.Advisories
}

func IDs() []string {
	return []string{"composer-audit", "composer-validate", "debug", "drupal-config", "phpcs", "phpmd", "phpstan", "syntax", "twig-cs-fixer", "whitespace", "yaml"}
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
	return Enabled("composer-audit", cfg.Tools.ComposerAudit.Enabled, only) || Enabled("composer-validate", cfg.Tools.ComposerValidate.Enabled, only) || Enabled("debug", cfg.Tools.Debug.Enabled, only) || Enabled("drupal-config", cfg.Tools.DrupalConfig.Enabled, only) || Enabled("phpcs", cfg.Tools.PHPCS.Enabled, only) || Enabled("phpmd", cfg.Tools.PHPMD.Enabled, only) || Enabled("phpstan", cfg.Tools.PHPStan.Enabled, only) || Enabled("syntax", cfg.Tools.Syntax.Enabled, only) || Enabled("twig-cs-fixer", cfg.Tools.TwigCSFixer.Enabled, only) || Enabled("yaml", cfg.Tools.YAML.Enabled, only) || Enabled("whitespace", cfg.Tools.Whitespace.Enabled, only)
}
