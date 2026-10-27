package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestLicenseNoticesAreAvailableWithoutExternalTools(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"--licenses"}, t.TempDir(), &stdout, &stderr); code != 0 || stderr.Len() != 0 || !strings.Contains(stdout.String(), "github.com/modelcontextprotocol/go-sdk") {
		t.Fatalf("%d %s %s", code, &stdout, &stderr)
	}
	stdout.Reset()
	if code := Run([]string{"--licenses", "--format=json"}, t.TempDir(), &stdout, &stderr); code != 0 {
		t.Fatal(code, &stderr)
	}
	var inventory struct {
		Version    int                                `json:"version"`
		Components []struct{ Module, Version string } `json:"components"`
	}
	if json.Unmarshal(stdout.Bytes(), &inventory) != nil || inventory.Version != 1 || len(inventory.Components) < 2 {
		t.Fatal("license inventory is unavailable")
	}
	stdout.Reset()
	if code := Run([]string{"container", "--licenses"}, t.TempDir(), &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "github.com/modelcontextprotocol/go-sdk") {
		t.Fatal("Compose cannot access embedded notices", code, &stderr)
	}
	for _, args := range [][]string{{"--licenses", "--format=compact"}, {"--licenses", "--project", "/tmp"}} {
		stdout.Reset()
		if code := Run(args, t.TempDir(), &stdout, &stderr); code != 2 || stdout.Len() != 0 {
			t.Fatalf("invalid license options accepted: %v", args)
		}
	}
}
