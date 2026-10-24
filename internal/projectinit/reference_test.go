package projectinit

import (
	"context"
	"os"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/checker/phpstan"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/scope"
	"go.yaml.in/yaml/v3"
)

func TestInitShowsAllCheckersWithoutChangingProfileDefaults(t *testing.T) {
	root := drupalProject(t, ">=8.1", nil)
	result, err := Run(context.Background(), Options{CWD: root, Project: root, Runtime: "8.4", Target: "8.4"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Location.Path)
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Tools map[string]map[string]any `yaml:"tools"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	for _, id := range checker.Requested(nil) {
		if _, ok := document.Tools[id]["enabled"]; !ok {
			t.Errorf("generated configuration has no editable %s toggle", id)
		}
	}
	want, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	got, err := config.Parse(data)
	if err != nil || !reflect.DeepEqual(got.Config, want.Config) {
		t.Fatalf("annotated configuration changed default behavior: %v", err)
	}
}

func TestInitOptionalSettingsCanBeUncommentedAndApplied(t *testing.T) {
	root := drupalProject(t, ">=8.1", nil)
	result, err := Run(context.Background(), Options{CWD: root, Project: root, Runtime: "8.4", Target: "8.4"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(result.Location.Path)
	if err != nil {
		t.Fatal(err)
	}
	uncomment := regexp.MustCompile(`(?m)^([ \t]*)# ([a-z][a-z0-9_-]*:)`)
	expanded := uncomment.ReplaceAllString(string(data), "$1$2")
	if expanded == string(data) {
		t.Fatal("init provided no optional settings to uncomment")
	}
	if _, err := config.Parse([]byte(expanded)); err != nil {
		t.Fatalf("uncommenting optional examples produces invalid configuration: %v", err)
	}
	lines := strings.Split(string(data), "\n")
	current := ""
	for i, line := range lines {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(line, ":") {
			current = strings.TrimSpace(strings.TrimSuffix(line, ":"))
		}
		if current != "phpstan" {
			continue
		}
		if strings.HasPrefix(line, "    # include:") {
			lines[i] = "    include: [web/modules/custom/demo]"
		}
		if strings.HasPrefix(line, "    # exclude:") {
			lines[i] = `    exclude: ["**/tests/**"]`
		}
		if strings.HasPrefix(line, "    # level:") {
			lines[i] = "    level: 6"
		}
	}
	effective, err := config.Parse([]byte(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	selected := phpstan.Select(scope.Selection{Files: []string{"web/modules/custom/demo/demo.module", "web/modules/custom/demo/tests/Test.php", "web/modules/custom/other/other.module"}}, effective.Config.Tools.PHPStan, "drupal")
	if !reflect.DeepEqual(selected.Files, []string{"web/modules/custom/demo/demo.module"}) || effective.Config.Tools.PHPStan.Level == nil || *effective.Config.Tools.PHPStan.Level != 6 {
		t.Fatalf("uncommented PHPStan settings did not take effect: %+v %+v", selected.Files, effective.Config.Tools.PHPStan)
	}
}
