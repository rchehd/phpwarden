package phpcs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/edit"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func TestActualFixPreparesEditsWithoutWritingSource(t *testing.T) {
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/phpcs")
	name := "src/demo.php"
	before := "<?php\nif (true) {\n\techo 1;\n}\n"
	write(t, root, name, before)
	cfg := settings(t)
	cfg.Standards, cfg.Sniffs = []string{"PSR12"}, []string{"Generic.WhiteSpace.DisallowTabIndent"}
	selected := scope.Selection{Files: []string{name}, LineMap: map[string][]scope.Range{name: {{Start: 4, End: 4}}}}
	result := runMode(context.Background(), root, ".", selected, cfg, config.Target{Min: "8.4", Max: "8.4"}, bundle, runner.Exec{}, true)
	if result.Check.Status == "error" || result.Check.Status == "unavailable" || len(result.Edits) != 1 || strings.Contains(string(result.Edits[0].After), "\t") {
		t.Fatalf("fix preparation: %+v", result)
	}
	content, _ := os.ReadFile(filepath.Join(root, name))
	if string(content) != before {
		t.Fatal("native formatter wrote the source")
	}
	if _, err := edit.Apply(root, result.Edits, map[string]string{name: edit.Hash([]byte(before))}); err != nil {
		t.Fatal(err)
	}
	selected.LineMap[name] = nil
	checked := run(context.Background(), root, ".", selected, cfg, config.Target{Min: "8.4", Max: "8.4"}, bundle, runner.Exec{})
	if checked.Check.Status != "passed" || len(checked.Findings) != 0 {
		t.Fatalf("fixed file did not pass: %+v", checked)
	}
}
