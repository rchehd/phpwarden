package profile

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type RectorPreset struct {
	Sets          []string
	DrupalVersion string
}

var rectorVersion = regexp.MustCompile(`^(10|11|12)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func RectorPresetFor(id, version string) (RectorPreset, error) {
	if id != "drupal" {
		return RectorPreset{}, fmt.Errorf("no Rector preset for profile %q; supply a project Rector config or explicit targets/rules", id)
	}
	parts := rectorVersion.FindStringSubmatch(version)
	if parts == nil {
		return RectorPreset{}, fmt.Errorf("Drupal Rector preset needs an exact framework.version; supply a project Rector config or explicit targets/rules")
	}
	major, _ := strconv.Atoi(parts[1])
	minor, err := strconv.Atoi(parts[2])
	if err != nil || major == 10 && minor > 6 || major == 11 && minor > 4 || major == 12 && minor > 0 {
		return RectorPreset{}, fmt.Errorf("no pinned Drupal Rector preset for %s; supply a project Rector config or explicit targets/rules", version)
	}
	result := RectorPreset{Sets: []string{}, DrupalVersion: version}
	for _, candidate := range DrupalRectorVersions() {
		var candidateMajor, candidateMinor int
		fmt.Sscanf(candidate, "%d.%d", &candidateMajor, &candidateMinor)
		if candidateMajor < major || candidateMajor == major && candidateMinor <= minor {
			result.Sets = append(result.Sets, "vendor/palantirnet/drupal-rector/config/drupal-"+strings.Split(candidate, ".")[0]+"/drupal-"+candidate+"-deprecations.php")
		}
	}
	return result, nil
}

func DrupalRectorVersions() []string {
	return []string{"8.0", "8.1", "8.2", "8.3", "8.4", "8.5", "8.6", "8.7", "8.8", "9.0", "9.1", "9.2", "9.3", "9.4", "10.0", "10.1", "10.2", "10.3", "11.0", "11.1", "11.2", "11.3", "11.4", "12.0"}
}
