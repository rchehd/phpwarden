package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigSourceLocations(t *testing.T) {
	base := "version: 1\nprofile: drupal\n"
	for _, item := range []struct {
		name, extra string
		want        []string
	}{
		{"duplicate", "tools:\n  debug:\n    enabled: true\n    enabled: false\n",
			[]string{"line 6, column 5", "$.tools.debug.enabled", "first defined at line 5, column 5"}},
		{"unknown", "runner:\n  concurrncy: 2\n",
			[]string{"line 4, column 3", "$.runner.concurrncy", "not allowed"}},
		{"type", "runner:\n  concurrency: wrong\n",
			[]string{"line 4, column 16", "$.runner.concurrency", "invalid configuration"}},
		{"array", "tools:\n  debug:\n    functions:\n      - false\n",
			[]string{"line 6, column 9", "$.tools.debug.functions[0]"}},
		{"tag", "scope:\n  include: !custom something\n",
			[]string{"line 4, column 12", "$.scope.include", "unsupported YAML type"}},
		{"merge", "scope:\n  <<: {mode: full}\n",
			[]string{"line 4, column 3", "merge keys"}},
		{"syntax", "scope:\n  include: [\n",
			[]string{"line ", "read YAML"}},
		{"native config", "tools:\n  deptrac:\n    config: wrong.php\n",
			[]string{"line 5, column 13", `$.tools.deptrac.config`, "local YAML file"}},
		{"path", "scope:\n  include:\n    - ../outside\n",
			[]string{"line 5, column 7", `$.scope.include[0]`}},
		{"relationship", "updates:\n  max_age_hours: 20\n  max_stale_hours: 1\n",
			[]string{"line 5, column 20", "$.updates.max_stale_hours", "at least"}},
	} {
		t.Run(item.name, func(t *testing.T) {
			_, err := Parse([]byte(base + item.extra + "php: {runtime: \"8.3\", target: {min: \"8.3\", max: \"8.3\"}}\n"))
			if err == nil {
				t.Fatal("invalid config accepted")
			}
			for _, want := range item.want {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("missing %q: %v", want, err)
				}
			}
		})
	}
}

func TestLoadNamesTheConfigurationFile(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, ".phpwarden.yaml")
	if err := os.WriteFile(name, []byte("version: 1\nprofile: drupal\nprofile: drupal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(Location{Root: root, Path: name})
	if err == nil || !strings.Contains(err.Error(), name+":3:1") || !strings.Contains(err.Error(), "$.profile") {
		t.Fatalf("missing file/position/path: %v", err)
	}
}

func TestLoadNamesTheEscapingIncludeField(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(root, ".phpwarden.yaml")
	data := "version: 1\nprofile: drupal\nphp: {runtime: \"8.3\", target: {min: \"8.3\", max: \"8.3\"}}\nscope: {include: [escape]}\n"
	if err := os.WriteFile(name, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Load(Location{Root: root, Path: name})
	if err == nil || !strings.Contains(err.Error(), name) || !strings.Contains(err.Error(), "$.scope.include[0]") {
		t.Fatalf("missing configuration file or include path: %v", err)
	}
}
