package toolset

import (
	"fmt"
	"maps"
	"runtime"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/compatibility"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/phpconstraint"
)

const DevelopmentID = "static-dev-v7-php8.4"
const DevelopmentPHPVersion = "8.4.26"

type Selection struct {
	Release          string            `json:"release"`
	Entry            string            `json:"entry"`
	Preset           Preset            `json:"preset"`
	Tools            map[string]string `json:"tools"`
	FrameworkVersion *string           `json:"framework_version,omitempty"`
	ID               string            `json:"id"`
	Image            string            `json:"image"`
	Platform         string            `json:"platform"`
	Protocol         int               `json:"protocol"`
	Capabilities     []string          `json:"capabilities"`
	Development      bool              `json:"development"`
	PHPVersion       string            `json:"php_version"`
}

func Resolve(effective config.Effective) (Selection, error) {
	catalog, err := bundled()
	if err != nil {
		return Selection{}, err
	}
	return catalog.Resolve(effective, runtime.GOOS+"/"+runtime.GOARCH)
}

func (catalog Catalog) Resolve(effective config.Effective, platform string) (Selection, error) {
	cfg := effective.Config
	if _, err := compatibility.ValidatePHP(cfg, effective.Project); err != nil {
		return Selection{}, err
	}
	framework := cfg.Framework.Version
	if framework != "" && framework != "auto" {
		version, err := phpconstraint.StableVersion(framework)
		if err != nil || version != framework {
			return Selection{}, fmt.Errorf("framework resolution requires a canonical exact stable version")
		}
	}
	var matches []Selection
	var rejected []string
	for _, item := range catalog.entries {
		if item.Profile != cfg.Profile || item.PHP.Runtime != cfg.PHP.Runtime {
			continue
		}
		covers, complete, err := item.targets.MinorRange(cfg.PHP.Target.Min, cfg.PHP.Target.Max)
		if err != nil {
			return Selection{}, err
		}
		if !covers || !complete {
			continue
		}
		if item.Framework != nil {
			if framework == "" || framework == "auto" {
				continue
			}
			allows, err := item.framework.Allows(framework)
			if err != nil || !allows {
				continue
			}
		}
		for _, image := range item.Images {
			if image.Platform != platform {
				continue
			}
			if err := compatibility.CheckRuntime(effective.Project, item.PHP.Version); err != nil {
				rejected = append(rejected, item.ID+": "+err.Error())
				continue
			}
			selected := Selection{Release: catalog.release, Entry: item.ID, Preset: item.Preset, Tools: maps.Clone(item.Tools),
				ID: item.Toolset, Image: image.Reference, Platform: platform, Protocol: catalog.protocol,
				Capabilities: slices.Clone(item.Capabilities), Development: item.Development, PHPVersion: item.PHP.Version}
			if framework != "" && framework != "auto" {
				selected.FrameworkVersion = new(framework)
			}
			matches = append(matches, selected)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		ids := make([]string, len(matches))
		for i, selected := range matches {
			ids[i] = selected.Entry
		}
		slices.Sort(ids)
		return Selection{}, fmt.Errorf("ambiguous compatibility manifest %q: matching entries %s", catalog.release, strings.Join(ids, ", "))
	}
	detail := ""
	if len(rejected) > 0 {
		slices.Sort(rejected)
		detail = "; " + strings.Join(rejected, "; ")
	}
	return Selection{}, fmt.Errorf("no compatible entry in manifest %q for profile=%s framework=%s PHP runtime=%s target=%s..%s platform=%s%s",
		catalog.release, cfg.Profile, framework, cfg.PHP.Runtime, cfg.PHP.Target.Min, cfg.PHP.Target.Max, platform, detail)
}
