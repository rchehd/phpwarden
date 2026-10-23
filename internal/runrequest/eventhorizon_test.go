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

func TestEventHorizonRequestRejectsChangedUnselectedConfig(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "config", "sync")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}},"tools":{"eventhorizon":{"config_sync":["config/sync"]}}}`)
	filename := filepath.Join(root, config.Filename)
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
	name := "config/sync/field.storage.node.field_demo.yml"
	if err := os.WriteFile(filepath.Join(root, name), []byte("type: string\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	location := config.Location{Root: root, Path: filename}
	effective, err := config.Load(location)
	if err != nil {
		t.Fatal(err)
	}
	selection := scope.Selection{Mode: "paths", Files: []string{name}, ProjectFiles: []string{}, LineMap: map[string][]scope.Range{name: nil}}
	request, err := Build(location, effective, selection, []string{"eventhorizon"}, "sha256:"+strings.Repeat("a", 64), toolset.DevelopmentID("8.4"))
	if err != nil {
		t.Fatal(err)
	}
	if request.Fingerprint["eventhorizon"] == "" || request.Matches(location) != nil {
		t.Fatal("EventHorizon context is missing from a valid request")
	}
	if err := os.WriteFile(filepath.Join(directory, "new.yml"), []byte("key: value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if request.Matches(location) == nil {
		t.Fatal("new unselected config was accepted by the old request")
	}
}
