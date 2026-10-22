package compatibility

import (
	"slices"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/profile"
)

func settings(runtime, minimum, maximum string) config.Config {
	return config.Config{PHP: config.PHP{Runtime: runtime, Target: config.Target{Min: minimum, Max: maximum}}}
}

func TestPHPProductionAndDevelopmentRequirements(t *testing.T) {
	for _, row := range []struct {
		name         string
		cfg          config.Config
		requirements []profile.PHPRequirement
		valid        bool
	}{
		{"higher dev PHP does not raise production target", settings("8.4", "8.1", "8.4"), []profile.PHPRequirement{{Source: "root", Constraint: "^8.1"}, {Source: "dev", Constraint: ">=8.4", Dev: true}}, true},
		{"runtime conflict", settings("8.2", "8.1", "8.4"), []profile.PHPRequirement{{Source: "root", Constraint: ">=8.3"}}, false},
		{"target conflict", settings("8.4", "8.2", "8.4"), []profile.PHPRequirement{{Source: "root", Constraint: ">=8.3"}}, false},
		{"dev runtime conflict", settings("8.4", "8.3", "8.4"), []profile.PHPRequirement{{Source: "dev", Constraint: "^8.5", Dev: true}}, false},
		{"target gap", settings("8.4", "8.1", "8.4"), []profile.PHPRequirement{{Source: "root", Constraint: "8.1.* || 8.4.*"}}, false},
		{"joint patch conflict", settings("8.4", "8.4", "8.4"), []profile.PHPRequirement{{Source: "a", Constraint: "<8.4.20"}, {Source: "b", Constraint: ">=8.4.30"}}, false},
		{"production conflict", settings("8.4", "8.4", "8.4"), []profile.PHPRequirement{{Source: "root.conflict", Constraint: "8.4.*", Conflict: true}}, false},
		{"dev conflict only limits runtime", settings("8.4", "8.3", "8.4"), []profile.PHPRequirement{{Source: "dev.conflict", Constraint: "8.3.*", Dev: true, Conflict: true}}, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			_, err := ValidatePHP(row.cfg, &profile.Facts{PHP: row.requirements})
			if (err == nil) != row.valid {
				t.Fatalf("valid=%v err=%v", row.valid, err)
			}
		})
	}
}

func TestPatchRequirementsRemainExplicitAndActualRuntimeIsChecked(t *testing.T) {
	facts := &profile.Facts{PHP: []profile.PHPRequirement{{Source: "root", Constraint: ">=8.4.27 <9"}}}
	pending, err := ValidatePHP(settings("8.4", "8.4", "8.4"), facts)
	if err != nil || !slices.Equal(pending, []string{"php.runtime.patch", "php.target.patch"}) {
		t.Fatalf("missing patch uncertainty: %v %v", pending, err)
	}
	if err := CheckRuntime(facts, "8.4.26"); err == nil {
		t.Fatal("incompatible patch accepted")
	}
	if err := CheckRuntime(facts, "8.4.27"); err != nil {
		t.Fatal(err)
	}
	if err := CheckRuntime(facts, "8.4.27-dev"); err == nil {
		t.Fatal("unsupported prerelease accepted")
	}
}

func TestComposerPlatformIsNotToolRuntime(t *testing.T) {
	platform := "8.3.12"
	facts := &profile.Facts{PHP: []profile.PHPRequirement{{Source: "root", Constraint: "^8.3"}}, PlatformPHP: &platform}
	if _, err := ValidatePHP(settings("8.4", "8.3", "8.4"), facts); err != nil {
		t.Fatal(err)
	}
	platform = "8.2.0"
	if _, err := ValidatePHP(settings("8.4", "8.3", "8.4"), facts); err == nil {
		t.Fatal("inconsistent platform pin accepted")
	}
}
