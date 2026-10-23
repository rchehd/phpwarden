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
}

func IDs() []string { return []string{"debug", "syntax", "yaml"} }

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
	return Enabled("debug", cfg.Tools.Debug.Enabled, only) || Enabled("syntax", cfg.Tools.Syntax.Enabled, only) || Enabled("yaml", cfg.Tools.YAML.Enabled, only)
}
