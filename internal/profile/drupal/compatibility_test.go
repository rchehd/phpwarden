package drupal

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/profile"
)

func TestInstalledDrupalMetadataFormatsAndCustomVendor(t *testing.T) {
	for _, row := range []struct{ name, composer, installed, status string }{
		{"composer2", `{}`, `{"packages":[{"name":"drupal/core","version":"v11.2.5"}],"dev":true}`, "verified"},
		{"composer1", `{}`, `[{"name":"drupal/core","version":"11.2.5"}]`, "verified"},
		{"custom vendor", `{"config":{"vendor-dir":"dependencies"}}`, `{"packages":[{"name":"drupal/core","version":"11.2.5"}]}`, "verified"},
	} {
		t.Run(row.name, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, "composer.json", row.composer)
			write(t, root, "composer.lock", `{"packages":[{"name":"drupal/core","version":"11.2.5"}]}`)
			vendor := "vendor"
			if row.name == "custom vendor" {
				vendor = "dependencies"
			}
			filename := vendor + "/composer/installed.json"
			write(t, root, filename, row.installed)
			write(t, root, vendor+"/autoload.php", "<?php file_put_contents('MUST_NOT_EXIST', 'executed');\n")
			facts, err := Discover(root)
			if err != nil {
				t.Fatal(err)
			}
			if facts.Installed == nil || facts.Installed.Status != row.status || *facts.Installed.Version != "11.2.5" || !strings.HasPrefix(facts.Inputs[filename], "sha256:") {
				t.Fatalf("wrong installed facts: %+v", facts)
			}
			if _, err := os.Stat(filepath.Join(root, "MUST_NOT_EXIST")); !os.IsNotExist(err) {
				t.Fatal("project code was executed")
			}
		})
	}
}

func TestOptionalMissingAndUnverifiableInstalledMetadata(t *testing.T) {
	root := t.TempDir()
	facts, err := Discover(root)
	if err != nil || facts.Installed.Status != "absent" {
		t.Fatalf("missing vendor must be optional: %+v %v", facts, err)
	}
	write(t, root, "vendor/composer/installed.php", "<?php throw new Exception('MUST NOT EXECUTE');\n")
	facts, err = Discover(root)
	if err != nil || facts.Installed.Status != "unverified" {
		t.Fatalf("PHP metadata must not be executed: %+v %v", facts, err)
	}
	write(t, root, "composer.lock", `{"packages":[],"packages-dev":[{"name":"drupal/core","version":"11.2.5"}]}`)
	write(t, root, "vendor/composer/installed.json", `{"packages":[],"dev":false}`)
	facts, err = Discover(root)
	if err != nil || facts.Installed.Status != "not_installed" {
		t.Fatalf("no-dev install treated as inconsistent: %+v %v", facts, err)
	}
}

func TestInstalledVersionCanSupplyMissingLockVersion(t *testing.T) {
	root := t.TempDir()
	write(t, root, "vendor/composer/installed.json", `{"packages":[{"name":"drupal/core","version":"11.2.5"}]}`)
	facts, err := Discover(root)
	if err != nil || facts.Version == nil || *facts.Version != "11.2.5" || *facts.VersionSource != "vendor/composer/installed.json:drupal/core" || facts.Installed.Status != "observed" {
		t.Fatalf("installed fallback missing: %+v %v", facts, err)
	}
}

func TestConflictingMalformedAndUnsafeInstalledMetadataIsInvalid(t *testing.T) {
	for _, contents := range []string{
		`{"packages":[{"name":"drupal/core","version":"11.2.6"}]}`,
		`{"packages":[{"name":"drupal/core","version":"11.x-dev"}]}`,
		`{"packages":[]}`,
		`{"packages":{}}`,
		`{"packages":[{"name":"drupal/core","version":"11.2.5"},{"name":"drupal/core","version":"11.2.5"}]}`,
		`{"packages":[{"name":"drupal/core","version":"11.2.5"}],"packages":[]}`,
		`null`,
		`{`,
	} {
		root := t.TempDir()
		write(t, root, "composer.lock", `{"packages":[{"name":"drupal/core","version":"11.2.5"}]}`)
		write(t, root, "vendor/composer/installed.json", contents)
		facts, err := Discover(root)
		if err != nil || facts.Installed.Status != "invalid" || facts.Installed.Error == nil || !slices.Contains(facts.Warnings, *facts.Installed.Error) ||
			facts.Version == nil || *facts.Version != "11.2.5" {
			t.Fatalf("inconsistent installed metadata accepted: %s: %+v %v", contents, facts, err)
		}
	}
	root, outside := t.TempDir(), t.TempDir()
	write(t, outside, "installed.json", `{"packages":[]}`)
	if err := os.MkdirAll(filepath.Join(root, "vendor", "composer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "installed.json"), filepath.Join(root, "vendor", "composer", "installed.json")); err != nil {
		t.Fatal(err)
	}
	if facts, err := Discover(root); err != nil || facts.Installed.Status != "invalid" {
		t.Fatalf("installed metadata symlink followed: %+v %v", facts, err)
	}
}

func TestPHPRequirementsHaveProductionAndDevProvenance(t *testing.T) {
	root := t.TempDir()
	write(t, root, "composer.json", `{"require":{"php":"^8.3"},"require-dev":{"php":">=8.4"},"conflict":{"php":"8.4.0"},"config":{"platform":{"php":"8.4.26"}}}`)
	write(t, root, "composer.lock", `{"packages":[{"name":"drupal/core","version":"11.2.5","require":{"php":">=8.3"},"conflict":{"php":"8.3.0"}}],"packages-dev":[{"name":"test/tool","require":{"php":"^8.4"}}],"platform":{"php":"^8.3"},"platform-dev":[]}`)
	facts, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.PHP) != 7 || facts.PlatformPHP == nil || *facts.PlatformPHP != "8.4.26" {
		t.Fatalf("missing requirements: %+v", facts)
	}
	for _, source := range []string{"composer.json:require.php", "composer.json:require-dev.php", "composer.lock:platform.php", "composer.lock:packages:drupal/core:require.php", "composer.lock:packages-dev:test/tool:require.php"} {
		if !slices.ContainsFunc(facts.PHP, func(requirement profile.PHPRequirement) bool { return requirement.Source == source }) {
			t.Errorf("missing source %s", source)
		}
	}
}

func TestInvalidInstalledMetadataIsDeterministic(t *testing.T) {
	root := t.TempDir()
	write(t, root, "composer.lock", `{"packages":[{"name":"drupal/core","version":"11.3.16"},{"name":"drupal/core-recommended","version":"11.3.16"}]}`)
	write(t, root, "vendor/composer/installed.json", `{"packages":[{"name":"drupal/core-recommended","version":"10.4.4"},{"name":"drupal/core","version":"10.4.4"}]}`)
	for range 20 {
		facts, err := Discover(root)
		if err != nil || facts.Installed.Error == nil || *facts.Installed.Error != "installed drupal/core 10.4.4 conflicts with composer.lock version 11.3.16" {
			t.Fatalf("installed metadata error is not stable: %+v %v", facts.Installed, err)
		}
	}
}
