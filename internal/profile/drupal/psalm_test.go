package drupal

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/profile"
)

func TestPsalmManifestRejectsIncompleteAndOutOfScopeInputs(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{ComposerRoot: ".", Scope: config.Scope{Include: []string{"src"}}}
	facts := &profile.Facts{WebRoot: new("web"), SourceRoots: []string{"src"}}
	inputs := map[string]string{}
	for _, filename := range []string{"composer.json", "composer.lock", "web/core/core.services.yml", "src/demo.info.yml", "src/demo.routing.yml", "src/a.php", "src/b.php", "outside.php"} {
		content := "<?php\n"
		write(t, root, filename, content)
		inputs[filename] = fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
	}
	base := psalmManifest{Version: 1, PreparationComplete: new(true), AnalysisComplete: new(false), Selection: "source inventory", Extensions: []psalmExtension{{"demo", "module", "src"}}, Sources: []string{"src/a.php", "src/b.php"}, Routes: []psalmRoute{{"demo.route", "src/demo.routing.yml", true}}, Services: map[string]string{}, Unresolved: map[string]string{}, Inputs: inputs}
	base.Diagnostics = []struct {
		Code   string `json:"code"`
		File   string `json:"file"`
		Detail string `json:"detail"`
	}{}
	for _, variant := range []string{"valid", "missing-flag", "claimed-analysis", "unmodeled-route", "outside", "duplicate", "omitted-source", "missing-input", "wrong-hash", "trailing"} {
		t.Run(variant, func(t *testing.T) {
			encoded, _ := json.Marshal(base)
			var value psalmManifest
			json.Unmarshal(encoded, &value)
			switch variant {
			case "missing-flag":
				value.PreparationComplete = nil
			case "claimed-analysis":
				value.AnalysisComplete = new(true)
			case "unmodeled-route":
				value.Routes[0].Modeled = false
			case "outside":
				value.Sources = []string{"outside.php"}
			case "duplicate":
				value.Sources = []string{"src/a.php", "src/a.php"}
			case "omitted-source":
				value.Sources = []string{"src/a.php"}
			case "missing-input":
				delete(value.Inputs, "src/a.php")
			case "wrong-hash":
				value.Inputs["src/a.php"] = fmt.Sprintf("%064d", 0)
			}
			encoded, _ = json.Marshal(value)
			if variant == "trailing" {
				encoded = append(encoded, []byte(" {}")...)
			}
			_, err := validatePsalmManifest(root, encoded, cfg, facts)
			if (err == nil) != (variant == "valid") {
				t.Fatalf("validation: %v", err)
			}
		})
	}
}

func TestPsalmInventoryUsesCoreGlobBoundaries(t *testing.T) {
	root := t.TempDir()
	for _, filename := range []string{"src/a.php", "src/nested/b.php", "src/vendor/c.php", "src/tests/Test.php", "src/demo.info.yml"} {
		write(t, root, filename, "<?php\n")
	}
	facts := &profile.Facts{SourceRoots: []string{"src"}}
	cfg := config.Config{Scope: config.Scope{Include: []string{"."}, Exclude: []string{"src/*.php"}}}
	inventory, sources, err := psalmInventory(root, cfg, facts)
	if err != nil || !slices.Equal(sources, []string{"src/nested/b.php"}) || !slices.Contains(inventory, "src/demo.info.yml") {
		t.Fatalf("wrong graph inventory: %v %v %v", inventory, sources, err)
	}
	cfg.Scope.Exclude = []string{"src/nested"}
	_, sources, err = psalmInventory(root, cfg, facts)
	if err != nil || !slices.Contains(sources, "src/nested/b.php") {
		t.Fatal("literal directory exclusion became a subtree glob")
	}
	cfg.Scope.Exclude = []string{"src/nested/**"}
	_, sources, err = psalmInventory(root, cfg, facts)
	if err != nil || slices.Contains(sources, "src/nested/b.php") {
		t.Fatal("recursive exclusion ignored")
	}
	if err := os.Symlink(filepath.Join(root, "src/a.php"), filepath.Join(root, "src/link.php")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := psalmInventory(root, cfg, facts); err == nil {
		t.Fatal("source symlink accepted")
	}
}
