package drupal

import (
	"errors"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/inputsource"
)

func TestJSONDuplicateLocationsAndUnambiguousPaths(t *testing.T) {
	for _, item := range []struct{ data, location, previous, path string }{
		{`{"я":0,"x":1,"x":2}`, "composer.json:1:14", "line 1, column 8", "$.x"},
		{"{\r\n  \"repositories\": [\r\n    {\"a.b\":1, \"a.b\":2},\r\n    {\"a.b\":3}\r\n  ]\r\n}",
			"composer.json:3:15", "line 3, column 6", `$.repositories[0]["a.b"]`},
		{`{"\u0078":1,"x":2}`, "composer.json:1:13", "line 1, column 2", "$.x"},
	} {
		root := t.TempDir()
		write(t, root, "composer.json", item.data)
		warnings := []string{}
		_, _, err := (reader{root: root, inputs: map[string]string{}, warnings: &warnings}).document("composer.json")
		if err != nil || len(warnings) != 1 {
			t.Fatalf("%v %v", warnings, err)
		}
		for _, want := range []string{item.location, item.previous, item.path, "last value is used"} {
			if !strings.Contains(warnings[0], want) {
				t.Errorf("missing %q: %s", want, warnings[0])
			}
		}
	}
}

func TestJSONSyntaxPositions(t *testing.T) {
	for _, item := range []struct {
		data         string
		line, column int
		path         string
	}{
		{`{"require": }`, 1, 13, "$.require"},
		{"{\n  \"require\": ", 2, 14, "$.require"},
		{"{}\n[]", 2, 1, "$"},
	} {
		root := t.TempDir()
		write(t, root, "composer.json", item.data)
		_, _, err := (reader{root: root, inputs: map[string]string{}, warnings: new([]string)}).document("composer.json")
		var source *inputsource.Error
		if !errors.As(err, &source) || source.File != "composer.json" || source.Position.Line != item.line || source.Position.Column != item.column || source.Path != item.path {
			t.Errorf("wrong location for %q: %v", item.data, err)
		}
	}
}

func TestMonorepoJSONErrorNamesTheComposerRoot(t *testing.T) {
	root := t.TempDir()
	write(t, root, "apps/site/composer.json", `{"require": }`)
	_, err := DiscoverIn(root, "apps/site")
	if err == nil || !strings.Contains(err.Error(), "apps/site/composer.json:1:13") {
		t.Fatal(err)
	}
}
