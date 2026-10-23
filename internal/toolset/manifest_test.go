package toolset

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	yamlcheck "github.com/rchehd/phpwarden/internal/checker/yaml"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/profile"
)

func fixtureEntry(id, framework, runtime, patch, minimum, maximum string) entry {
	item := entry{ID: id, Profile: "drupal", Preset: Preset{ID: "drupal-fixture", Version: "1.0.0"},
		Framework: &framework, Toolset: "fixture-" + runtime, Tools: map[string]string{"php": patch, "phpstan": "2.1.0"},
		Capabilities: []string{"debug", "phpstan"}, Images: []manifestImage{
			{Platform: "linux/amd64", Reference: "ghcr.io/example/fixture@sha256:" + strings.Repeat("a", 64)},
			{Platform: "linux/arm64", Reference: "ghcr.io/example/fixture@sha256:" + strings.Repeat("b", 64)},
		}}
	item.PHP.Runtime, item.PHP.Version = runtime, patch
	item.PHP.Targets.Min, item.PHP.Targets.Max = minimum, maximum
	return item
}

func parseFixture(t *testing.T, entries ...entry) Catalog {
	t.Helper()
	if entries == nil {
		entries = []entry{}
	}
	data, err := json.Marshal(manifest{Version: 1, Release: "fixture-1", Protocol: 2, Entries: entries})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func effectiveVersion(framework, runtime, minimum, maximum string) config.Effective {
	return config.Effective{Config: config.Config{Profile: "drupal", Framework: config.Framework{Version: framework},
		PHP: config.PHP{Runtime: runtime, Target: config.Target{Min: minimum, Max: maximum}}}}
}

func TestReleaseResolutionMatrix(t *testing.T) {
	first := fixtureEntry("d10", ">=10.3 <11", "8.3", "8.3.20", "8.2", "8.3")
	second := fixtureEntry("d11", ">=11 <12", "8.4", "8.4.26", "8.3", "8.4")
	second.Images[0].Reference = strings.ReplaceAll(second.Images[0].Reference, strings.Repeat("a", 64), strings.Repeat("c", 64))
	second.Images[1].Reference = strings.ReplaceAll(second.Images[1].Reference, strings.Repeat("b", 64), strings.Repeat("d", 64))
	catalog := parseFixture(t, first, second)
	for _, tc := range []struct {
		name, framework, runtime, min, max, platform, want string
	}{
		{"first minimum", "10.3.0", "8.3", "8.2", "8.3", "linux/amd64", "d10"},
		{"first latest fixture", "10.9.99", "8.3", "8.3", "8.3", "linux/arm64", "d10"},
		{"second minimum", "11.0.0", "8.4", "8.3", "8.4", "linux/amd64", "d11"},
		{"second upper target", "11.5.1", "8.4", "8.4", "8.4", "linux/arm64", "d11"},
		{"old framework", "10.2.99", "8.3", "8.2", "8.3", "linux/amd64", ""},
		{"upper framework", "12.0.0", "8.4", "8.3", "8.4", "linux/amd64", ""},
		{"wrong runtime", "11.0.0", "8.3", "8.3", "8.3", "linux/amd64", ""},
		{"target starts too low", "11.0.0", "8.4", "8.2", "8.4", "linux/amd64", ""},
		{"target ends too high", "11.0.0", "8.4", "8.3", "8.5", "linux/amd64", ""},
		{"invalid target order", "11.0.0", "8.4", "8.4", "8.3", "linux/amd64", ""},
		{"unknown framework", "auto", "8.4", "8.3", "8.4", "linux/amd64", ""},
		{"missing framework", "", "8.4", "8.3", "8.4", "linux/amd64", ""},
		{"framework range", "^11", "8.4", "8.3", "8.4", "linux/amd64", ""},
		{"prerelease framework", "11.0.0-beta1", "8.4", "8.3", "8.4", "linux/amd64", ""},
		{"noncanonical framework", "v11.0.0", "8.4", "8.3", "8.4", "linux/amd64", ""},
		{"unsupported OS", "11.0.0", "8.4", "8.3", "8.4", "darwin/arm64", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selected, err := catalog.Resolve(effectiveVersion(tc.framework, tc.runtime, tc.min, tc.max), tc.platform)
			if tc.want == "" {
				if err == nil {
					t.Fatalf("unsupported combination resolved: %+v", selected)
				}
				return
			}
			if err != nil || selected.Entry != tc.want || selected.Platform != tc.platform || selected.Release != "fixture-1" ||
				selected.Preset.ID != "drupal-fixture" || selected.FrameworkVersion == nil || *selected.FrameworkVersion != tc.framework || selected.Development ||
				!strings.Contains(selected.Image, "@sha256:") || selected.Tools["phpstan"] != "2.1.0" || selected.Protocol != 2 {
				t.Fatalf("wrong release selection: %+v %v", selected, err)
			}
		})
	}
	unsupported := effectiveVersion("11.0.0", "8.4", "8.3", "8.4")
	unsupported.Config.Profile = "laravel"
	if _, err := catalog.Resolve(unsupported, "linux/amd64"); err == nil {
		t.Fatal("unsupported profile resolved")
	}
}

func TestAmbiguityDoesNotPickFirstOrLatest(t *testing.T) {
	first := fixtureEntry("first", ">=11 <12", "8.4", "8.4.26", "8.3", "8.4")
	second := first
	second.ID, second.Preset.ID = "second", "another-preset"
	for _, entries := range [][]entry{{first, second}, {second, first}} {
		catalog := parseFixture(t, entries...)
		_, err := catalog.Resolve(effectiveVersion("11.1.0", "8.4", "8.3", "8.4"), "linux/amd64")
		if err == nil || !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "first, second") {
			t.Fatalf("ambiguity not rejected deterministically: %v", err)
		}
	}
	shared := first
	shared.ID, shared.Framework = "shared", new(">=12 <13")
	selected, err := parseFixture(t, first, shared).Resolve(effectiveVersion("12.0.0", "8.4", "8.3", "8.4"), "linux/amd64")
	if err != nil || selected.Entry != "shared" || selected.ID != first.Toolset || selected.Image != first.Images[0].Reference {
		t.Fatalf("image reuse across framework ranges failed: %+v %v", selected, err)
	}
}

func TestComposerRuntimeConstraintsFilterExactImagePatch(t *testing.T) {
	first := fixtureEntry("old-patch", ">=11 <12", "8.4", "8.4.26", "8.3", "8.4")
	second := fixtureEntry("new-patch", ">=11 <12", "8.4", "8.4.27", "8.3", "8.4")
	second.Toolset = "fixture-new-patch"
	second.Images[0].Reference = "ghcr.io/example/fixture@sha256:" + strings.Repeat("c", 64)
	second.Images[1].Reference = "ghcr.io/example/fixture@sha256:" + strings.Repeat("d", 64)
	effective := effectiveVersion("11.0.0", "8.4", "8.3", "8.4")
	effective.Project = &profile.Facts{PHP: []profile.PHPRequirement{{Source: "composer.json require-dev.php", Constraint: ">=8.4.27", Dev: true}}}
	selected, err := parseFixture(t, first, second).Resolve(effective, "linux/amd64")
	if err != nil || selected.Entry != "new-patch" {
		t.Fatalf("exact Composer restriction did not select compatible patch: %+v %v", selected, err)
	}
	_, err = parseFixture(t, first).Resolve(effective, "linux/amd64")
	if err == nil || !strings.Contains(err.Error(), "composer.json require-dev.php") || !strings.Contains(err.Error(), "8.4.26") {
		t.Fatalf("missing Composer incompatibility reason: %v", err)
	}
	effective.Project.PHP = []profile.PHPRequirement{{Source: "composer.json require.php", Constraint: ">=8.4"}}
	if _, err := parseFixture(t, first).Resolve(effective, "linux/amd64"); err == nil || !strings.Contains(err.Error(), "target") {
		t.Fatalf("production target conflict was ignored: %v", err)
	}
}

func TestInvalidManifests(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*manifest)
	}{
		{"manifest version", func(m *manifest) { m.Version = 2 }},
		{"protocol version", func(m *manifest) { m.Protocol = 1 }},
		{"duplicate ID", func(m *manifest) { m.Entries = append(m.Entries, m.Entries[0]) }},
		{"missing framework", func(m *manifest) { m.Entries[0].Framework = nil }},
		{"invalid framework", func(m *manifest) { m.Entries[0].Framework = new("dev-main") }},
		{"unknown profile", func(m *manifest) { m.Entries[0].Profile = "laravel" }},
		{"wrong runtime patch", func(m *manifest) { m.Entries[0].PHP.Version = "8.3.20" }},
		{"prerelease PHP", func(m *manifest) { m.Entries[0].PHP.Version = "8.4.26-RC1" }},
		{"inverted targets", func(m *manifest) { m.Entries[0].PHP.Targets.Min = "8.5" }},
		{"bad target", func(m *manifest) { m.Entries[0].PHP.Targets.Max = "8.04" }},
		{"wrong tool version", func(m *manifest) { m.Entries[0].Tools["php"] = "8.4.25" }},
		{"floating tool version", func(m *manifest) { m.Entries[0].Tools["phpstan"] = "^2" }},
		{"missing PHP tool", func(m *manifest) { delete(m.Entries[0].Tools, "php") }},
		{"duplicate capability", func(m *manifest) { m.Entries[0].Capabilities = []string{"debug", "debug"} }},
		{"no capabilities", func(m *manifest) { m.Entries[0].Capabilities = []string{} }},
		{"no images", func(m *manifest) { m.Entries[0].Images = []manifestImage{} }},
		{"duplicate platform", func(m *manifest) { m.Entries[0].Images = append(m.Entries[0].Images, m.Entries[0].Images[0]) }},
		{"unsupported platform", func(m *manifest) { m.Entries[0].Images[0].Platform = "windows/amd64" }},
		{"mutable tag", func(m *manifest) { m.Entries[0].Images[0].Reference = "ghcr.io/example/fixture:latest" }},
		{"tag with digest", func(m *manifest) {
			m.Entries[0].Images[0].Reference = "ghcr.io/example/fixture:v1@sha256:" + strings.Repeat("a", 64)
		}},
		{"missing repository", func(m *manifest) { m.Entries[0].Images[0].Reference = "sha256:" + strings.Repeat("a", 64) }},
		{"unqualified registry", func(m *manifest) {
			m.Entries[0].Images[0].Reference = "example/fixture@sha256:" + strings.Repeat("a", 64)
		}},
		{"bad digest", func(m *manifest) { m.Entries[0].Images[0].Reference = "ghcr.io/example/fixture@sha256:abcd" }},
		{"development in release", func(m *manifest) { m.Entries[0].Development = true }},
		{"inconsistent toolset", func(m *manifest) {
			other := fixtureEntry("other", ">=12 <13", "8.4", "8.4.27", "8.3", "8.4")
			m.Entries = append(m.Entries, other)
		}},
		{"inconsistent image", func(m *manifest) {
			other := fixtureEntry("other", ">=12 <13", "8.4", "8.4.26", "8.3", "8.4")
			other.Toolset = "different-toolset"
			m.Entries = append(m.Entries, other)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := manifest{Version: 1, Protocol: 2, Release: "fixture-1", Entries: []entry{fixtureEntry("d11", ">=11 <12", "8.4", "8.4.26", "8.3", "8.4")}}
			tc.edit(&source)
			data, _ := json.Marshal(source)
			if _, err := ParseManifest(data); err == nil {
				t.Fatalf("invalid manifest accepted: %s", data)
			}
		})
	}
	for _, data := range []string{
		"", "null", "[]", "{}", string(bundledManifest) + "{}", strings.TrimSuffix(strings.TrimSpace(string(bundledManifest)), "}"),
		strings.Replace(string(bundledManifest), `"version": 1`, `"version": 1, "version": 1`, 1),
		strings.Replace(string(bundledManifest), `"php": "8.4.26"`, `"php": "8.4.26", "php": "8.4.26"`, 1),
		strings.Replace(string(bundledManifest), `"version": 1`, `"version": 1, "unexpected": true`, 1),
		strings.Replace(string(bundledManifest), `"runtime": "8.4"`, `"runtime": "8.4", "typo": true`, 1),
		strings.Replace(string(bundledManifest), `"version": "1.4.0"`, `"version": null`, 1),
		strings.Replace(string(bundledManifest), `"framework": null`, `"framework": "^11"`, 1),
		strings.Replace(string(bundledManifest), `"debug"`, `"phpstan"`, 1),
		strings.Replace(string(bundledManifest), `"linux/amd64"`, `"linux/arm64"`, 1),
		strings.Replace(string(bundledManifest), `"development": true`, `"development": false`, 1),
		strings.Replace(string(bundledManifest), `"release": "development"`, `"release": "v1"`, 1),
		strings.Repeat(" ", 1<<20+1), strings.Repeat("[", 40) + "0" + strings.Repeat("]", 40), string([]byte{0xff}),
	} {
		if _, err := ParseManifest([]byte(data)); err == nil {
			t.Fatalf("malformed manifest accepted: %.200s", data)
		}
	}
}

func TestBundledDevelopmentAndConcurrentSelections(t *testing.T) {
	catalog, err := bundled()
	if err != nil {
		t.Fatal(err)
	}
	effective := effectiveVersion("auto", "8.4", "8.4", "8.4")
	var group sync.WaitGroup
	for range 20 {
		group.Go(func() {
			selected, err := catalog.Resolve(effective, "linux/amd64")
			if err != nil || !selected.Development || selected.Release != "development" || selected.ID != DevelopmentID || selected.PHPVersion != DevelopmentPHPVersion || selected.FrameworkVersion != nil {
				t.Errorf("wrong bundled selection: %+v %v", selected, err)
				return
			}
			selected.Tools["php"] = "modified"
			selected.Capabilities[0] = "modified"
		})
	}
	group.Wait()
	if _, err := catalog.Resolve(effective, "linux/arm64"); err == nil {
		t.Fatal("unverified development platform selected")
	}
	empty := parseFixture(t)
	if _, err := empty.Resolve(effective, "linux/amd64"); err == nil {
		t.Fatal("empty catalog silently fell back to development image")
	}
}

func TestBundledYAMLVersionsMatchDependencyLock(t *testing.T) {
	catalog, err := bundled()
	if err != nil {
		t.Fatal(err)
	}
	selected, err := catalog.Resolve(effectiveVersion("auto", "8.4", "8.4", "8.4"), "linux/amd64")
	if err != nil {
		t.Fatal(err)
	}
	if selected.Tools["symfony-yaml"] != yamlcheck.Version {
		t.Fatal("YAML checker and manifest versions differ")
	}
	for _, name := range []string{"yaml", "phpcs"} {
		data, err := os.ReadFile("../../tools/" + name + "/composer.lock")
		if err != nil {
			t.Fatal(err)
		}
		var lock struct {
			Packages []struct {
				Name    string
				Version string
			}
		}
		if err := json.Unmarshal(data, &lock); err != nil {
			t.Fatal(err)
		}
		for _, pkg := range lock.Packages {
			if selected.Tools[strings.ReplaceAll(pkg.Name, "/", "-")] != strings.TrimPrefix(pkg.Version, "v") {
				t.Fatalf("manifest version differs from lock for %s", pkg.Name)
			}
		}
	}
}
