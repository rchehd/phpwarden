package distribution

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageRejectsOccupiedOutputBeforeExecutingBinary(t *testing.T) {
	for _, name := range []string{
		"phpwarden", "phpwarden-linux-amd64.tar.gz", "phpwarden-composer-linux-amd64.zip",
		"phpwarden-npm-linux-amd64.tgz", "phpwarden-compose-php8.0.tar.gz", "compatibility.json", "SHA256SUMS", "install.sh",
	} {
		t.Run(name, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), "dist [previous]")
			if err := os.Mkdir(output, 0o755); err != nil {
				t.Fatal(err)
			}
			existing := filepath.Join(output, name)
			if err := os.WriteFile(existing, []byte("preserve previous artifact"), 0o644); err != nil {
				t.Fatal(err)
			}
			err := Package(context.Background(), "/missing/executable", "/missing/README", "", output)
			if err == nil || !strings.Contains(err.Error(), "use a fresh directory") {
				t.Fatalf("did not reject occupied output before probing the binary: %v", err)
			}
			if data, _ := os.ReadFile(existing); string(data) != "preserve previous artifact" {
				t.Fatal("modified existing artifact")
			}
		})
	}
}

func TestPackageOutputAllowsOnlyExplicitRegularInputs(t *testing.T) {
	output := t.TempDir()
	binary, manifest := filepath.Join(output, "phpwarden"), filepath.Join(output, "compatibility.json")
	for _, name := range []string{binary, manifest} {
		if err := os.WriteFile(name, []byte("explicit input"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkPackageOutput(binary, manifest, output); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(output, alias); err != nil {
		t.Fatal(err)
	}
	if err := checkPackageOutput(binary, manifest, alias); err == nil {
		t.Fatal("accepted symlink output directory")
	}
	for _, name := range []string{binary, manifest} {
		if err := os.Remove(name); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("missing", name); err != nil {
			t.Fatal(err)
		}
		if err := checkPackageOutput(binary, manifest, output); err == nil {
			t.Fatal("accepted symlink output target", name)
		}
		os.Remove(name)
	}
}
