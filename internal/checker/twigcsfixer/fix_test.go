package twigcsfixer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rchehd/phpwarden/internal/edit"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func TestActualFixPreparesEditsWithoutWritingSource(t *testing.T) {
	root := t.TempDir()
	bundle, _ := filepath.Abs("../../../tools/twig-cs-fixer")
	name, before := "templates/demo.twig", "<p>{{title}}</p>\n"
	write(t, root, name, before)
	selected := scope.Selection{Files: []string{name}, LineMap: map[string][]scope.Range{name: nil}}
	result := runMode(context.Background(), root, ".", selected, settings(t), "drupal", bundle, runner.Exec{}, true)
	if result.Check.Status == "error" || len(result.Edits) != 1 {
		t.Fatalf("fix preparation: %+v", result)
	}
	content, _ := os.ReadFile(filepath.Join(root, name))
	if string(content) != before {
		t.Fatal("native formatter wrote the source")
	}
	if _, err := edit.Apply(root, result.Edits, map[string]string{name: edit.Hash([]byte(before))}); err != nil {
		t.Fatal(err)
	}
	checked := run(context.Background(), root, ".", selected, settings(t), "drupal", bundle, runner.Exec{})
	if checked.Check.Status != "passed" || len(checked.Findings) != 0 {
		t.Fatalf("fixed file did not pass: %+v", checked)
	}
}
