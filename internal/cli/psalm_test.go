package cli

import (
	"slices"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
)

func TestPsalmScopeIncludesProjectConfigurationOnlyWhenSelected(t *testing.T) {
	root := project(t)
	location := config.Location{Root: root, Path: root + "/.phpwarden.yaml"}
	effective, err := config.Load(location)
	if err != nil {
		t.Fatal(err)
	}
	for _, only := range []string{"", "psalm", "debug"} {
		for _, enabled := range []bool{false, true} {
			effective.Config.Tools.Psalm.Enabled = enabled
			args := options{command: []string{"check"}, only: only}
			scoped, err := scopeOptions(args, root, location, effective.Config)
			if err != nil {
				t.Fatal(err)
			}
			want := only == "psalm" || only == "" && enabled
			if slices.Contains(scoped.ProjectFiles, ".phpwarden.yaml") != want {
				t.Fatalf("config metadata selection mismatch: %+v", scoped)
			}
		}
	}
}
