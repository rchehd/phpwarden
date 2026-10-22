package discovery

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
)

func fixture(t *testing.T, extra string) (config.Location, config.Effective) {
	t.Helper()
	root := t.TempDir()
	for _, directory := range []string{"web/modules/custom/demo", "web/themes/custom/site", "src"} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		config.Filename: "version: 1\nprofile: drupal\nphp: {runtime: '8.4', target: {min: '8.4', max: '8.4'}}\n" + extra,
		"composer.json": `{"extra":{"drupal-scaffold":{"locations":{"web-root":"web"}}}}`,
		"composer.lock": `{"packages":[{"name":"drupal/core","version":"11.2.5"}]}`,
	}
	for filename, content := range files {
		if err := os.WriteFile(filepath.Join(root, filename), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	location := config.Location{Root: root, Path: filepath.Join(root, config.Filename)}
	effective, err := config.Load(location)
	if err != nil {
		t.Fatal(err)
	}
	return location, effective
}

func TestDiscoveredDefaultsAndExplicitOverrides(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		extra := ""
		if explicit {
			extra = "framework: {version: '11.2.5'}\nscope: {include: [src], exclude: ['**/generated/**']}\n"
		}
		location, effective := fixture(t, extra)
		result, err := Apply(location, effective)
		if err != nil {
			t.Fatal(err)
		}
		if result.Config.Framework.Version != "11.2.5" || len(result.Pending) != 0 || result.Project == nil {
			t.Fatalf("facts not applied: %+v", result)
		}
		if result.SourceHash != effective.SourceHash || effective.Project != nil || effective.Config.Framework.Version == "11.2.5" && !explicit {
			t.Fatal("discovery mutated the original loaded config")
		}
		if explicit {
			if !slices.Equal(result.Config.Scope.Include, []string{"src"}) || result.Provenance["framework.version"] != "project_config" {
				t.Fatalf("explicit settings lost: %+v", result)
			}
		} else if len(result.Config.Scope.Include) != 2 || result.Provenance["scope.include"] != "profile_discovery" || result.Provenance["framework.version"] != "discovery:composer.lock:drupal/core" {
			t.Fatalf("missing defaults/provenance: %+v", result)
		}
		for _, exclude := range []string{"**/vendor/**", "web/core/**", "web/modules/contrib/**"} {
			if !slices.Contains(result.Config.Scope.Exclude, exclude) {
				t.Fatalf("missing additive exclusion %s: %v", exclude, result.Config.Scope.Exclude)
			}
		}
	}
}

func TestConflictingExplicitVersionAndUnresolvedFacts(t *testing.T) {
	location, effective := fixture(t, "framework: {version: '10.4.0'}\n")
	if _, err := Apply(location, effective); err == nil {
		t.Fatal("explicit version concealed a locked version conflict")
	}
	if err := os.Remove(filepath.Join(location.Root, "composer.lock")); err != nil {
		t.Fatal(err)
	}
	result, err := Apply(location, effective)
	if err != nil || result.Config.Framework.Version != "10.4.0" || *result.Project.VersionSource != "project_config" {
		t.Fatalf("explicit exact version did not resolve missing lock: %+v %v", result, err)
	}
	location, effective = fixture(t, "")
	if err := os.Remove(filepath.Join(location.Root, "composer.lock")); err != nil {
		t.Fatal(err)
	}
	result, err = Apply(location, effective)
	if err != nil || !slices.Equal(result.Pending, []string{"framework.version"}) {
		t.Fatalf("unknown framework version must remain pending: %+v %v", result, err)
	}
}

func TestDiscoveryChangesInvalidateResults(t *testing.T) {
	for _, which := range []string{"lock", "composer", "new custom root", "new multisite", "missing lock created"} {
		t.Run(which, func(t *testing.T) {
			location, effective := fixture(t, "")
			if which == "missing lock created" {
				if err := os.Remove(filepath.Join(location.Root, "composer.lock")); err != nil {
					t.Fatal(err)
				}
			}
			before, err := Apply(location, effective)
			if err != nil {
				t.Fatal(err)
			}
			if err := Unchanged(location, before); err != nil {
				t.Fatalf("unchanged discovery rejected: %v", err)
			}
			filename, content := "composer.lock", `{"packages":[{"name":"drupal/core","version":"11.2.6"}]}`
			if which == "composer" {
				filename, content = "composer.json", "{}\n"
			} else if which == "new custom root" || which == "new multisite" {
				filename = "web/profiles/custom/project/demo.php"
				if which == "new multisite" {
					filename = "web/sites/new.example/modules/custom/demo/demo.module"
				}
				if err := os.MkdirAll(filepath.Dir(filepath.Join(location.Root, filename)), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(location.Root, filename), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := Unchanged(location, before); err == nil || !strings.Contains(err.Error(), "changed") {
				t.Fatalf("changed discovery was accepted: %v", err)
			}
		})
	}
}
