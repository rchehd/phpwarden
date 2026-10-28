package drupal

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func write(t *testing.T, root, name, content string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestWebRootsVersionsAndCustomSources(t *testing.T) {
	for _, webRoot := range []string{".", "web", "docroot", "public [site]"} {
		t.Run(webRoot, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, "composer.json", `{"extra":{"drupal-scaffold":{"locations":{"web-root":"`+webRoot+`/"}}}}`)
			write(t, root, "composer.lock", `{"packages":[{"name":"drupal/core-recommended","version":"11.2.5"},{"name":"drupal/core","version":"v11.2.5"}],"packages-dev":[]}`)
			for _, directory := range []string{"modules/custom/demo", "themes/custom/site", "profiles/custom/project", "sites/all/modules/custom/shared", "sites/client.example/themes/custom/client"} {
				write(t, root, filepath.ToSlash(filepath.Join(webRoot, directory, "example.php")), "<?php\n")
			}
			facts, err := Discover(root)
			if err != nil {
				t.Fatal(err)
			}
			if facts.WebRoot == nil || *facts.WebRoot != webRoot || facts.Version == nil || *facts.Version != "11.2.5" || *facts.VersionSource != "composer.lock:drupal/core" {
				t.Fatalf("wrong facts: %+v", facts)
			}
			if len(facts.SourceRoots) != 5 || !slices.Contains(facts.SourceRoots, filepath.ToSlash(filepath.Join(webRoot, "sites/client.example/themes/custom"))) {
				t.Fatalf("custom or multisite roots missing: %v", facts.SourceRoots)
			}
			if !strings.HasPrefix(facts.Inputs["composer.lock"], "sha256:") || !strings.HasPrefix(facts.Inputs[filepath.ToSlash(filepath.Join(webRoot, "sites"))], "listing:") {
				t.Fatalf("discovery inputs were not captured: %v", facts.Inputs)
			}
		})
	}
}

func TestConventionsAndInstallerPaths(t *testing.T) {
	for _, which := range []string{"conventions", "installer core", "custom installer"} {
		t.Run(which, func(t *testing.T) {
			root := t.TempDir()
			composer := `{}`
			if which == "installer core" {
				composer = `{"extra":{"installer-paths":{"public/core":["type:drupal-core"]}}}`
			} else if which == "custom installer" {
				composer = `{"extra":{"installer-paths":{"custom/extensions/{$name}":["type:drupal-custom-module"],"modules/{$name}":["type:drupal-module"]}},"config":{"vendor-dir":"dependencies"}}`
			}
			write(t, root, "composer.json", composer)
			write(t, root, "composer.lock", `{"packages":[{"name":"drupal/token","version":"1.0.0","type":"drupal-module"}]}`)
			if which == "conventions" {
				write(t, root, "docroot/core/lib/Drupal.php", "<?php\n")
				write(t, root, "docroot/modules/custom/test/demo.php", "<?php\n")
			} else if which == "installer core" {
				write(t, root, "public/modules/custom/test/demo.php", "<?php\n")
			} else {
				write(t, root, "custom/extensions/test/demo.php", "<?php\n")
				write(t, root, "modules/custom/test/demo.php", "<?php\n")
			}
			facts, err := Discover(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(facts.SourceRoots) == 0 {
				t.Fatal("source roots were not discovered")
			}
			if which == "custom installer" && (!slices.Contains(facts.Exclusions, "modules/token/**") || slices.Contains(facts.Exclusions, "modules/**") || !slices.Contains(facts.Exclusions, "dependencies/**")) {
				t.Fatalf("installer exclusion must preserve custom modules: %v", facts.Exclusions)
			}
		})
	}
}

func TestUnknownVersionsStayUnresolved(t *testing.T) {
	for _, lock := range []string{
		`{"packages":[]}`,
		`{"packages":[{"name":"drupal/core","version":"11.x-dev"}]}`,
		`{"packages":[{"name":"drupal/core-recommended","version":"11.2.5"},{"name":"drupal/core","version":"11.x-dev"}]}`,
		`{"packages":[{"name":"drupal/core","version":"11.3.0-rc1"}]}`,
	} {
		root := t.TempDir()
		write(t, root, "composer.json", `{"require":{"drupal/core":"^11"}}`)
		write(t, root, "composer.lock", lock)
		facts, err := Discover(root)
		if err != nil || facts.Version != nil {
			t.Fatalf("constraint/dev version must not be guessed: %+v %v", facts, err)
		}
	}
}

func TestDuplicateKeysUseLastValueWithWarning(t *testing.T) {
	root := t.TempDir()
	write(t, root, "composer.json", `{"extra":{"drupal-scaffold":{"locations":{"web-root":"docroot"}}},"extra":{"drupal-scaffold":{"locations":{"web-root":"web"},"locations":{"web-root":"web"}}}}`)
	facts, err := Discover(root)
	if err != nil || facts.WebRoot == nil || *facts.WebRoot != "web" || len(facts.Warnings) != 2 {
		t.Fatalf("duplicate keys not resolved like Composer: %+v %v", facts, err)
	}
	for i, want := range []string{"$.extra", `$.extra["drupal-scaffold"].locations`} {
		if !strings.Contains(facts.Warnings[i], want) || !strings.Contains(facts.Warnings[i], "first defined at line 1, column") {
			t.Fatal(facts.Warnings[i])
		}
	}
}

func TestUnsafeAmbiguousOrMalformedMetadataFails(t *testing.T) {
	cases := map[string]string{
		"malformed":          `{`,
		"non-object":         `[]`,
		"trailing JSON":      `{} {}`,
		"null extra":         `{"extra":null}`,
		"traversal":          `{"extra":{"drupal-scaffold":{"locations":{"web-root":"../outside"}}}}`,
		"absolute":           `{"extra":{"drupal-scaffold":{"locations":{"web-root":"/outside"}}}}`,
		"variable":           `{"extra":{"drupal-scaffold":{"locations":{"web-root":"${ROOT}"}}}}`,
		"conflicting roots":  `{"extra":{"drupal-scaffold":{"locations":{"web-root":"web"}},"installer-paths":{"docroot/core":["type:drupal-core"]}}}`,
		"invalid selectors":  `{"extra":{"installer-paths":{"web/modules/contrib/{$name}":true}}}`,
		"ambiguous template": `{"extra":{"installer-paths":{"custom/{$vendor}/{$name}":["type:drupal-custom-module"]}}}`,
	}
	for name, composer := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, "composer.json", composer)
			if _, err := Discover(root); err == nil {
				t.Fatal("invalid metadata accepted")
			}
		})
	}
	for _, lock := range []string{
		`{"packages":{}}`,
		`{"packages":[{"name":"drupal/core","version":"11.2.5"},{"name":"drupal/core-recommended","version":"11.1.0"}]}`,
		`{"packages":[{"name":"drupal/core","version":"11.2.5"}],"packages-dev":[{"name":"drupal/core","version":"11.2.5"}]}`,
	} {
		root := t.TempDir()
		write(t, root, "composer.lock", lock)
		if _, err := Discover(root); err == nil {
			t.Fatalf("invalid lock accepted: %s", lock)
		}
	}
	root := t.TempDir()
	write(t, root, "web/modules/custom/one/demo.php", "<?php\n")
	write(t, root, "docroot/modules/custom/two/demo.php", "<?php\n")
	if _, err := Discover(root); err == nil {
		t.Fatal("ambiguous web root was guessed")
	}
	outside := t.TempDir()
	for _, name := range []string{"composer.json", "web"} {
		root := t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
		if _, err := Discover(root); err == nil {
			t.Fatalf("symlink %q was followed", name)
		}
	}
}
