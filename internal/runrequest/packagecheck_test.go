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

func TestPackageRequestsTrackUnselectedDependencies(t *testing.T) {
	for _, id := range []string{"composer-require-checker", "composer-unused"} {
		t.Run(id, func(t *testing.T) {
			root := t.TempDir()
			filename := filepath.Join(root, config.Filename)
			os.WriteFile(filename, []byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`), 0600)
			os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"require":{"php":">=8.1"}}`), 0600)
			location := config.Location{Root: root, Path: filename}
			effective, err := config.Load(location)
			if err != nil {
				t.Fatal(err)
			}
			selected := scope.Selection{Mode: "full", Files: []string{}, ProjectFiles: []string{}, LineMap: map[string][]scope.Range{}}
			request, err := Build(location, effective, selected, []string{id}, "sha256:"+strings.Repeat("a", 64), toolset.DevelopmentID("8.4"))
			if err != nil {
				t.Fatal(err)
			}
			if request.Fingerprint[id] == "" || request.Matches(location) != nil {
				t.Fatal("missing package context")
			}
			os.WriteFile(filepath.Join(root, "composer-unused.php"), []byte("<?php"), 0600)
			if request.Matches(location) == nil {
				t.Fatal("changed unselected package input accepted")
			}
		})
	}
}
