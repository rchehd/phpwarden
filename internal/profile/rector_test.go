package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRectorPresetUsesPinnedSetsThroughCurrentMinor(t *testing.T) {
	for _, version := range []string{"10.0.1", "10.6.0", "11.0.9", "11.4.10", "12.0.0"} {
		t.Run(version, func(t *testing.T) {
			preset, err := RectorPresetFor("drupal", version)
			if err != nil || preset.DrupalVersion != version || len(preset.Sets) == 0 {
				t.Fatalf("preset: %+v %v", preset, err)
			}
			latest := map[string]string{"10.0.1": "10.0", "10.6.0": "10.3", "11.0.9": "11.0", "11.4.10": "11.4", "12.0.0": "12.0"}[version]
			if !strings.HasSuffix(preset.Sets[len(preset.Sets)-1], "/drupal-"+latest+"-deprecations.php") {
				t.Fatalf("incorrect minor boundary: %+v", preset)
			}
			for _, name := range preset.Sets {
				if strings.Contains(name, "breaking") || strings.Contains(name, "php8") {
					t.Fatalf("unexpected automatic set: %s", name)
				}
				if _, err := os.Stat(filepath.Join("../../tools/rector", name)); err != nil {
					t.Fatalf("pinned set missing: %s: %v", name, err)
				}
				if strings.HasPrefix(version, "10.") && strings.Contains(name, "/drupal-11/") || !strings.HasPrefix(version, "12.") && strings.Contains(name, "/drupal-12/") {
					t.Fatalf("future set enabled: %s", name)
				}
			}
		})
	}
	for _, version := range []string{"", "auto", "^11.4", "11.4", "11.4.0-dev", "v11.4.0", "9.5.0", "10.7.0", "11.5.0", "12.1.0", "13.0.0", "11.9999999999999999999999999.0"} {
		if _, err := RectorPresetFor("drupal", version); err == nil {
			t.Errorf("unsupported version accepted: %q", version)
		}
	}
	if _, err := RectorPresetFor("laravel", "11.4.0"); err == nil {
		t.Fatal("unknown profile fell back to Drupal")
	}
}
