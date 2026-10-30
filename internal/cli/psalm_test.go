package cli

import (
	"os"
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

func TestPsalmScopeIncludesNativeInputsOutsideSourceRoots(t *testing.T) {
	root := project(t)
	for name, data := range map[string]string{"psalm.xml": `<psalm errorBaseline="taint.xml"><stubs><file name="types.php"/></stubs></psalm>`, "taint.xml": `<files/>`, "types.php": "<?php class Metadata {}\n"} {
		if err := os.WriteFile(root+"/"+name, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	location := config.Location{Root: root, Path: root + "/.phpwarden.yaml"}
	effective, err := config.Load(location)
	if err != nil {
		t.Fatal(err)
	}
	scoped, err := scopeOptions(options{command: []string{"check"}, only: "psalm"}, root, location, effective.Config)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"psalm.xml", "taint.xml", "types.php"} {
		if !slices.Contains(scoped.ProjectFiles, name) {
			t.Fatalf("native input %s missing from shared project-file scope", name)
		}
	}
}
