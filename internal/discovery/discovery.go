package discovery

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/profile/drupal"
)

func Apply(location config.Location, effective config.Effective) (config.Effective, error) {
	if effective.Config.Profile != "drupal" {
		return effective, fmt.Errorf("unsupported discovery profile %q", effective.Config.Profile)
	}
	facts, err := drupal.Discover(location.Root)
	if err != nil {
		return effective, err
	}
	effective.Provenance = maps.Clone(effective.Provenance)
	effective.Project = &facts
	version := effective.Config.Framework.Version
	if version != "auto" {
		if facts.Version != nil && *facts.Version != version {
			return effective, fmt.Errorf("configured Drupal version %s conflicts with locked %s", version, *facts.Version)
		}
		if facts.Version == nil {
			source := "project_config"
			facts.Version, facts.VersionSource = &version, &source
		}
	} else if facts.Version != nil {
		effective.Config.Framework.Version = *facts.Version
		effective.Provenance["framework.version"] = "discovery:" + *facts.VersionSource
	}
	if len(effective.Config.Scope.Include) == 0 && len(facts.SourceRoots) > 0 {
		effective.Config.Scope.Include = slices.Clone(facts.SourceRoots)
		effective.Provenance["scope.include"] = "profile_discovery"
	}
	effective.Config.Scope.Exclude = slices.Clone(effective.Config.Scope.Exclude)
	for _, exclusion := range facts.Exclusions {
		if !slices.Contains(effective.Config.Scope.Exclude, exclusion) {
			effective.Config.Scope.Exclude = append(effective.Config.Scope.Exclude, exclusion)
		}
	}
	if len(facts.Exclusions) > 0 {
		effective.Provenance["scope.exclude"] += "+profile_discovery"
	}
	effective.Pending = []string{}
	if effective.Config.Framework.Version == "auto" {
		effective.Pending = append(effective.Pending, "framework.version")
	}
	if len(effective.Config.Scope.Include) == 0 {
		effective.Pending = append(effective.Pending, "scope.include")
	}
	if facts.WebRoot == nil {
		effective.Pending = append(effective.Pending, "profile.exclusions")
	}
	return effective, nil
}

func Unchanged(location config.Location, expected config.Effective) error {
	if expected.Project == nil {
		return nil
	}
	loaded, err := config.Load(location)
	if err != nil {
		return err
	}
	actual, err := Apply(location, loaded)
	if err != nil {
		return err
	}
	expectedConfig, _ := json.Marshal(expected.Config)
	actualConfig, _ := json.Marshal(actual.Config)
	if string(expectedConfig) != string(actualConfig) || !reflect.DeepEqual(expected.Project, actual.Project) {
		return fmt.Errorf("project discovery inputs changed during the check")
	}
	return nil
}
