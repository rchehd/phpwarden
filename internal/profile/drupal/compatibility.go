package drupal

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/phpconstraint"
	"github.com/rchehd/phpwarden/internal/profile"
)

func compatibility(reader reader, composer, lock map[string]any, facts *profile.Facts) error {
	facts.PHP = []profile.PHPRequirement{}
	for _, section := range []string{"require", "require-dev", "conflict"} {
		if err := phpRequirement(composer, section, "composer.json:"+section, section == "require-dev", facts); err != nil {
			return err
		}
	}
	for _, section := range []string{"platform", "platform-dev"} {
		if err := phpRequirement(lock, section, "composer.lock:"+section, section == "platform-dev", facts); err != nil {
			return err
		}
	}
	for _, section := range []string{"packages", "packages-dev"} {
		packages, _ := lock[section].([]any)
		for _, value := range packages {
			entry := value.(map[string]any)
			for _, link := range []string{"require", "conflict"} {
				if err := phpRequirement(entry, link, fmt.Sprintf("composer.lock:%s:%v:%s", section, entry["name"], link), section == "packages-dev", facts); err != nil {
					return err
				}
			}
		}
	}
	config, err := objectAt(composer, "config")
	if err != nil {
		return err
	}
	platform, err := objectAt(config, "platform")
	if err != nil {
		return err
	}
	if value, exists := platform["php"]; exists {
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("composer.json:config.platform.php must be a stable numeric version string")
		}
		normalized, err := phpconstraint.StableVersion(text)
		if err != nil {
			return err
		}
		facts.PlatformPHP = &normalized
	}
	vendor := "vendor"
	if value, exists := config["vendor-dir"]; exists {
		var ok bool
		vendor, ok = value.(string)
		if !ok || safePath(vendor) != nil || path.Clean(vendor) == "." {
			return fmt.Errorf("Composer vendor-dir must be a project-relative subdirectory")
		}
	}
	slices.SortFunc(facts.PHP, func(a, b profile.PHPRequirement) int { return strings.Compare(a.Source, b.Source) })
	return installed(reader, path.Clean(vendor), lock, facts)
}

func phpRequirement(object map[string]any, section, source string, dev bool, facts *profile.Facts) error {
	value, exists := object[section]
	if !exists {
		return nil
	}
	links, ok := value.(map[string]any)
	if !ok {
		if empty, ok := value.([]any); ok && len(empty) == 0 {
			return nil
		}
		return fmt.Errorf("%s must be an object of package constraints", source)
	}
	if value, exists := links["php"]; exists {
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return fmt.Errorf("%s.php must be a constraint string", source)
		}
		if _, err := phpconstraint.Parse(text); err != nil {
			return fmt.Errorf("%s.php: %w", source, err)
		}
		facts.PHP = append(facts.PHP, profile.PHPRequirement{Source: source + ".php", Constraint: text, Dev: dev, Conflict: section == "conflict"})
	}
	return nil
}

func installed(reader reader, vendor string, lock map[string]any, facts *profile.Facts) error {
	filename := path.Join(vendor, "composer/installed.json")
	facts.Installed = &profile.Installed{Path: filename, Status: "absent"}
	data, exists, err := reader.document(filename)
	if err != nil {
		return err
	}
	if !exists {
		marker, err := reader.probe(path.Join(vendor, "composer/installed.php"))
		if err != nil {
			return err
		}
		if marker != nil {
			facts.Installed.Status = "unverified"
		}
		return nil
	}
	var packages []any
	dev := true
	switch value := data.(type) {
	case map[string]any:
		var ok bool
		packages, ok = value["packages"].([]any)
		if !ok {
			return fmt.Errorf("%s must contain a packages array", filename)
		}
		if value, exists := value["dev"]; exists {
			dev, ok = value.(bool)
			if !ok {
				return fmt.Errorf("%s dev must be a boolean", filename)
			}
		}
	case []any:
		packages = value
	default:
		return fmt.Errorf("%s must be Composer 1/2 installed JSON metadata", filename)
	}
	versions := map[string]string{}
	for _, value := range packages {
		entry, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s packages must contain objects", filename)
		}
		name, _ := entry["name"].(string)
		if name != "drupal/core" && name != "drupal/core-recommended" {
			continue
		}
		text, ok := entry["version"].(string)
		if !ok || text == "" {
			return fmt.Errorf("%s:%s requires a version string", filename, name)
		}
		if _, exists := versions[name]; exists {
			return fmt.Errorf("duplicate installed Drupal package %s", name)
		}
		versions[name] = strings.TrimPrefix(text, "v")
	}
	for name, text := range versions {
		for _, section := range []string{"packages", "packages-dev"} {
			entries, _ := lock[section].([]any)
			for _, value := range entries {
				entry := value.(map[string]any)
				if entry["name"] == name && strings.TrimPrefix(entry["version"].(string), "v") != text {
					return fmt.Errorf("installed %s %s conflicts with composer.lock version %s", name, text, entry["version"])
				}
			}
		}
	}
	lockedCore := false
	for _, section := range []string{"packages", "packages-dev"} {
		entries, _ := lock[section].([]any)
		for _, value := range entries {
			entry := value.(map[string]any)
			lockedCore = lockedCore || entry["name"] == "drupal/core" && (section == "packages" || dev)
		}
	}
	if lockedCore && versions["drupal/core"] == "" {
		return fmt.Errorf("%s is missing locked drupal/core; install the project's dependencies", filename)
	}
	if len(versions) == 0 {
		facts.Installed.Status = "not_installed"
		return nil
	}
	facts.Installed.Status = "unverified"
	core := versions["drupal/core"]
	if core == "" {
		return nil
	}
	facts.Installed.Version = &core
	if facts.Version != nil && *facts.Version != core {
		return fmt.Errorf("installed Drupal %s conflicts with locked %s", core, *facts.Version)
	}
	if recommended, exists := versions["drupal/core-recommended"]; exists && recommended != core {
		return fmt.Errorf("conflicting installed Drupal core/core-recommended versions")
	}
	match := exactVersion.FindStringSubmatch(core)
	if match != nil {
		facts.Installed.Status = "observed"
		if facts.Version != nil {
			facts.Installed.Status = "verified"
		} else {
			source := filename + ":drupal/core"
			facts.Version, facts.VersionSource = &match[1], &source
		}
	}
	return nil
}
