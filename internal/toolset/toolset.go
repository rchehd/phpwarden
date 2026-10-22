package toolset

import (
	"fmt"
	"runtime"

	"github.com/rchehd/phpwarden/internal/config"
)

const DebugID = "debug-dev-v1-php8.4"

type Selection struct {
	FrameworkVersion *string  `json:"framework_version,omitempty"`
	ID               string   `json:"id"`
	Image            string   `json:"image"`
	Platform         string   `json:"platform"`
	Protocol         int      `json:"protocol"`
	Capabilities     []string `json:"capabilities"`
	Development      bool     `json:"development"`
}

func Resolve(cfg config.Config) (Selection, error) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return Selection{}, fmt.Errorf("development toolset is currently tested only on Linux amd64")
	}
	if cfg.Profile != "drupal" || cfg.PHP.Runtime != "8.4" || cfg.PHP.Target.Min != "8.4" || cfg.PHP.Target.Max != "8.4" {
		return Selection{}, fmt.Errorf("development debug toolset requires profile drupal and PHP runtime/target 8.4; no release compatibility catalog is available yet")
	}
	selected := Selection{ID: DebugID, Image: "phpwarden:debug-php8.4-dev", Platform: "linux/amd64", Protocol: 1, Capabilities: []string{"debug"}, Development: true}
	if cfg.Framework.Version != "auto" && cfg.Framework.Version != "" {
		selected.FrameworkVersion = &cfg.Framework.Version
	}
	return selected, nil
}
