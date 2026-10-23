package runrequest

import (
	"reflect"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/scope"
)

func TestFixIDsRequireSupportedEnabledFormatters(t *testing.T) {
	cfg := config.Config{}
	if _, err := FixIDs(cfg, nil); err == nil {
		t.Fatal("disabled formatters were selected by default")
	}
	ids, err := FixIDs(cfg, []string{"twig-cs-fixer", "phpcs"})
	if err != nil || !reflect.DeepEqual(ids, []string{"phpcs", "twig-cs-fixer"}) {
		t.Fatalf("explicit selection: %v, %v", ids, err)
	}
	for _, only := range [][]string{{"syntax"}, {"phpcs", "phpstan"}, {"unknown"}} {
		if _, err := FixIDs(cfg, only); err == nil {
			t.Fatalf("unsupported fixer accepted: %v", only)
		}
	}
}

func TestFixVerificationKeepsOriginalFilesAfterLineShifts(t *testing.T) {
	options := scope.Options{Root: "/project", CWD: "/project/sub", Mode: "diff", Base: "origin/main", Commits: 2, WholeFiles: true}
	selected := scope.Selection{Files: []string{"sub/demo.php"}}
	result := Verification(options, selected)
	if result.Mode != "paths" || result.CWD != options.Root || result.Base != "" || result.Commits != 0 || result.WholeFiles || !reflect.DeepEqual(result.Paths, selected.Files) {
		t.Fatalf("post-fix scope: %+v", result)
	}
	result.Paths[0] = "other.php"
	if selected.Files[0] != "sub/demo.php" {
		t.Fatal("verification modified the original selection")
	}
}
