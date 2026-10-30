package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginsCommand(t *testing.T) {
	output := filepath.Join(t.TempDir(), "plugins")
	if err := run([]string{"plugins", "--version", "v0.0.0-dev.abcdef0", "--commit", strings.Repeat("a", 40), "--output", output}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"phpwarden-plugin-codex.zip", "phpwarden-plugin-cursor.zip", "phpwarden-plugin-claude.zip", "phpwarden-plugin-catalogs.zip", "SHA256SUMS"} {
		if info, err := os.Stat(filepath.Join(output, name)); err != nil || info.Size() == 0 {
			t.Fatalf("missing plugin artifact %s: %v", name, err)
		}
	}
}

func TestCatalogsCommand(t *testing.T) {
	output := filepath.Join(t.TempDir(), "catalogs")
	if err := run([]string{"catalogs", "--output", output}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(output, ".agents/plugins/marketplace.json")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"catalogs"}, {"catalogs", "--output", output, "extra"}, {"catalogs", "--unknown"}} {
		if err := run(args); err == nil {
			t.Fatal("accepted invalid catalog arguments", args)
		}
	}
}

func TestPluginsCommandRequiresExplicitIdentityAndOutput(t *testing.T) {
	for _, args := range [][]string{
		{}, {"plugins"}, {"plugins", "--version", "v1.2.3"},
		{"plugins", "--commit", strings.Repeat("a", 40), "--output", "unused"},
		{"plugins", "--version", "v1.2.3", "--commit", strings.Repeat("a", 40)},
		{"plugins", "--version", "v1.2.3", "--commit", strings.Repeat("a", 40), "--output", "unused", "extra"},
		{"plugins", "--unknown"},
	} {
		if err := run(args); err == nil {
			t.Fatalf("accepted invalid arguments %q", args)
		}
	}
}
