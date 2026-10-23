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

func TestSemgrepRequestRejectsNewDiscoveredConfig(t *testing.T) {
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
	request, err := Build(location, effective, selected, []string{"semgrep"}, "sha256:"+strings.Repeat("a", 64), toolset.DevelopmentID("8.4"))
	if err != nil {
		t.Fatal(err)
	}
	if request.Fingerprint["semgrep"] == "" || request.Matches(location) != nil {
		t.Fatal("missing Semgrep context in request")
	}
	rules := "rules:\n  - id: custom.echo\n    languages: [php]\n    severity: INFO\n    message: Review echo\n    pattern: echo 42;\n"
	if err := os.WriteFile(filepath.Join(root, ".semgrep.yml"), []byte(rules), 0o600); err != nil {
		t.Fatal(err)
	}
	if request.Matches(location) == nil {
		t.Fatal("new local ruleset was accepted by an old request")
	}
}
