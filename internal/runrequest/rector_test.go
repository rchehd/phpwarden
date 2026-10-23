package runrequest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/scope"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func TestRectorRequestRejectsNewDiscoveredConfig(t *testing.T) {
	root := t.TempDir()
	filename := filepath.Join(root, config.Filename)
	data := []byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`)
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "demo.php"), []byte("<?php\necho 42;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	location := config.Location{Root: root, Path: filename}
	effective, err := config.Load(location)
	if err != nil {
		t.Fatal(err)
	}
	selected := scope.Selection{Mode: "paths", Files: []string{"demo.php"}, ProjectFiles: []string{}, LineMap: map[string][]scope.Range{"demo.php": nil}}
	request, err := Build(location, effective, selected, []string{"rector"}, "sha256:"+strings.Repeat("a", 64), toolset.DevelopmentID("8.4"))
	if err != nil {
		t.Fatal(err)
	}
	if request.Fingerprint["rector"] == "" || request.Matches(location) != nil {
		t.Fatal("missing Rector context in request")
	}
	rules := "<?php\nreturn static function ($config): void {};\n"
	if err := os.WriteFile(filepath.Join(root, "rector.php"), []byte(rules), 0o600); err != nil {
		t.Fatal(err)
	}
	if request.Matches(location) == nil {
		t.Fatal("new Rector config was accepted by an old request")
	}
}
