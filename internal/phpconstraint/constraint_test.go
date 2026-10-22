package phpconstraint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestComposerReferenceCases(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "tests", "fixtures", "composer-constraints.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Reference string `json:"reference"`
		Cases     []struct {
			Constraint string          `json:"constraint"`
			Versions   map[string]bool `json:"versions"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.Reference != "composer/semver@3.4.4" || len(document.Cases) < 20 {
		t.Fatal("missing Composer conformance reference")
	}
	for _, row := range document.Cases {
		t.Run(row.Constraint, func(t *testing.T) {
			set, err := Parse(row.Constraint)
			if err != nil {
				t.Fatal(err)
			}
			for version, want := range row.Versions {
				got, err := set.Allows(version)
				if err != nil || got != want {
					t.Errorf("%s matches %s: got %v, want %v (%v)", row.Constraint, version, got, want, err)
				}
			}
		})
	}
}

func TestMinorRangesDetectGapsAndPartialPatches(t *testing.T) {
	for _, row := range []struct {
		constraint, min, max string
		allowed, full        bool
	}{
		{"^8.1", "8.1", "8.4", true, true},
		{"8.1.* || 8.4.*", "8.1", "8.4", false, false},
		{">=8.3.12 <9", "8.3", "8.4", true, false},
		{">=8.3.12 <9", "8.4", "8.4", true, true},
		{">=8.3 <9 !=8.4.26", "8.4", "8.4", true, false},
		{"<8.4", "8.4", "8.4", false, false},
		{"8.4", "8.4", "8.4", true, false},
		{"*", "8.0", "99.0", true, true},
		{"<8.3 || >=8.4", "8.2", "8.4", false, false},
		{">=8.4.30 <8.4.20", "8.4", "8.4", false, false},
	} {
		set, err := Parse(row.constraint)
		if err != nil {
			t.Fatal(err)
		}
		allowed, full, err := set.MinorRange(row.min, row.max)
		if err != nil || allowed != row.allowed || full != row.full {
			t.Errorf("%s for %s..%s: got %v/%v want %v/%v (%v)", row.constraint, row.min, row.max, allowed, full, row.allowed, row.full, err)
		}
	}
}

func TestUnsupportedConstraintsAreRejected(t *testing.T) {
	for _, text := range []string{"", "^", "dev-main", "8.x-dev", "^8.4@dev", "8.4.0-RC1", "(>=8.3 <9)", "8.4 as 8.3", ">=8.3junk", ">=8.3 ||", "8.3 ||| 8.4", "8.*.1", ">=8.*", "!=8.3.*", "8.4,", ",8.4", "8.4,,<9", "8.4 -", "999999999999.0", "8.4.0.1"} {
		if _, err := Parse(text); err == nil {
			t.Errorf("unsupported constraint %q accepted", text)
		}
	}
}
