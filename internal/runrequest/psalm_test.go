package runrequest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/discovery"
	"github.com/rchehd/phpwarden/internal/scope"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func TestPsalmRequestFingerprintsUnselectedGraphContent(t *testing.T) {
	for _, name := range []string{"src/Helper.php", "src/demo.services.yml", "composer.json", "composer.lock", "deps/composer/installed.json", "deps/autoload.php"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.Mkdir(filepath.Join(root, "src"), 0o700); err != nil {
				t.Fatal(err)
			}
			settings := `version: 1
profile: drupal
php: {runtime: "8.3", target: {min: "8.3", max: "8.3"}}
scope: {include: [src]}
tools: {psalm: {enabled: true, include: [src/Sink.php]}}
`
			files := map[string]string{config.Filename: settings, "src/Sink.php": "<?php\necho input();\n", "src/Helper.php": "<?php\nfunction input() { return 1; }\n", "src/demo.services.yml": "services: {demo: {class: ClassA}}\n"}
			files["composer.json"] = `{"config":{"vendor-dir":"deps"},"extra":{"marker":"aaaa"}}`
			files["composer.lock"] = `{"packages":[],"marker":"aaaa"}`
			files["deps/composer/installed.json"] = `{"packages":[],"marker":"aaaa"}`
			files["deps/autoload.php"] = "<?php // aaaa\n"
			for path, data := range files {
				if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, path), []byte(data), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			location := config.Location{Root: root, Path: filepath.Join(root, config.Filename)}
			effective, err := config.Load(location)
			if err != nil {
				t.Fatal(err)
			}
			selection := scope.Selection{Mode: "paths", Files: []string{"src/Sink.php"}, ProjectFiles: []string{}, LineMap: map[string][]scope.Range{"src/Sink.php": nil}}
			request, err := Build(location, effective, selection, []string{"psalm"}, "sha256:"+strings.Repeat("a", 64), toolset.DevelopmentID("8.3"))
			if err != nil || request.Matches(location) != nil || request.Fingerprint["psalm"] == "" {
				t.Fatalf("missing graph fingerprint: %v", err)
			}
			filename := filepath.Join(root, name)
			before, _ := os.Stat(filename)
			data := strings.ReplaceAll(files[name], "return 1", "return 2")
			data = strings.ReplaceAll(data, "ClassA", "ClassB")
			data = strings.ReplaceAll(data, "aaaa", "bbbb")
			if err := os.WriteFile(filename, []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(filename, before.ModTime(), before.ModTime()); err != nil {
				t.Fatal(err)
			}
			if request.Matches(location) == nil {
				t.Fatal("same-stat unselected graph change was accepted")
			}
			delete(request.Fingerprint, "psalm")
			if request.Validate() == nil {
				t.Fatal("missing graph fingerprint was accepted")
			}
		})
	}
}

func TestPsalmFingerprintUsesDiscoveredRootsWithoutExplicitInclude(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		config.Filename:                          "version: 1\nprofile: drupal\nphp: {runtime: \"8.3\", target: {min: \"8.3\", max: \"8.3\"}}\ntools: {psalm: {enabled: true}}\n",
		"composer.json":                          `{"require":{"drupal/core":"11.4.1"},"extra":{"drupal-scaffold":{"locations":{"web-root":"web"}}}}`,
		"composer.lock":                          `{"packages":[{"name":"drupal/core","version":"11.4.1","type":"drupal-core"}]}`,
		"web/core/lib/Drupal.php":                "<?php class Drupal { const VERSION = '11.4.1'; }\n",
		"web/modules/custom/demo/demo.info.yml":  "name: Demo\ntype: module\ncore_version_requirement: ^11\n",
		"web/modules/custom/demo/demo.module":    "<?php echo input();\n",
		"web/modules/custom/demo/src/Helper.php": "<?php function input() { return 1; }\n",
	}
	for name, data := range files {
		filename := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	location := config.Location{Root: root, Path: filepath.Join(root, config.Filename)}
	effective, err := config.Load(location)
	if err != nil {
		t.Fatal(err)
	}
	if len(effective.Config.Scope.Include) != 0 {
		t.Fatal("fixture must rely on discovery")
	}
	effective, err = discovery.Apply(location, effective)
	if err != nil || len(effective.Config.Scope.Include) == 0 {
		t.Fatalf("source discovery failed: %v", err)
	}
	name := "web/modules/custom/demo/demo.module"
	selection := scope.Selection{Mode: "paths", Files: []string{name}, ProjectFiles: []string{}, LineMap: map[string][]scope.Range{name: nil}}
	request, err := Build(location, effective, selection, []string{"psalm"}, "sha256:"+strings.Repeat("a", 64), toolset.DevelopmentID("8.3"))
	if err != nil || request.Matches(location) != nil {
		t.Fatalf("discovered request failed: %v", err)
	}
	helper := filepath.Join(root, "web/modules/custom/demo/src/Helper.php")
	before, _ := os.Stat(helper)
	if err := os.WriteFile(helper, []byte("<?php function input() { return 2; }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(helper, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	if request.Matches(location) == nil {
		t.Fatal("same-stat helper change under discovered roots was accepted")
	}
}
