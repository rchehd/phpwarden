package compatibility

import (
	"fmt"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/phpconstraint"
	"github.com/rchehd/phpwarden/internal/profile"
)

func ValidatePHP(cfg config.Config, facts *profile.Facts) ([]string, error) {
	pending := []string{}
	if facts == nil {
		return pending, nil
	}
	runtime, target := phpconstraint.Any(), phpconstraint.Any()
	for _, requirement := range facts.PHP {
		allowed, err := constraint(requirement)
		if err != nil {
			return nil, err
		}
		runtime = runtime.Intersect(allowed)
		if ok, _, err := runtime.MinorRange(cfg.PHP.Runtime, cfg.PHP.Runtime); err != nil || !ok {
			return nil, fmt.Errorf("PHP runtime %s conflicts with %s %q and preceding PHP requirements", cfg.PHP.Runtime, requirement.Source, requirement.Constraint)
		}
		if !requirement.Dev {
			target = target.Intersect(allowed)
			if ok, _, err := target.MinorRange(cfg.PHP.Target.Min, cfg.PHP.Target.Max); err != nil || !ok {
				return nil, fmt.Errorf("PHP target %s..%s conflicts with %s %q and preceding production PHP requirements", cfg.PHP.Target.Min, cfg.PHP.Target.Max, requirement.Source, requirement.Constraint)
			}
		}
		if facts.PlatformPHP != nil {
			ok, err := allowed.Allows(*facts.PlatformPHP)
			if err != nil || !ok {
				return nil, fmt.Errorf("Composer config.platform.php %s conflicts with %s %q", *facts.PlatformPHP, requirement.Source, requirement.Constraint)
			}
		}
	}
	if _, full, _ := runtime.MinorRange(cfg.PHP.Runtime, cfg.PHP.Runtime); !full {
		pending = append(pending, "php.runtime.patch")
	}
	if _, full, _ := target.MinorRange(cfg.PHP.Target.Min, cfg.PHP.Target.Max); !full {
		pending = append(pending, "php.target.patch")
	}
	if facts.Installed != nil && facts.Installed.Status == "unverified" {
		pending = append(pending, "framework.installed_version")
	}
	return pending, nil
}

func CheckRuntime(facts *profile.Facts, actual string) error {
	if facts == nil {
		return nil
	}
	for _, requirement := range facts.PHP {
		allowed, err := constraint(requirement)
		if err != nil {
			return err
		}
		ok, err := allowed.Allows(actual)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("PHP runtime %s conflicts with %s %q", actual, requirement.Source, requirement.Constraint)
		}
	}
	return nil
}

func constraint(requirement profile.PHPRequirement) (phpconstraint.Set, error) {
	allowed, err := phpconstraint.Parse(requirement.Constraint)
	if err != nil {
		return allowed, fmt.Errorf("%s: %w", requirement.Source, err)
	}
	if requirement.Conflict {
		allowed = allowed.Complement()
	}
	return allowed, nil
}
