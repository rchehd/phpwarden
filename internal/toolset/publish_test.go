package toolset

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestPublishedManifestPinsEveryRuntimeAndKeepsCompatibility(t *testing.T) {
	images := map[string]string{}
	for _, runtime := range []string{"8.1", "8.2", "8.3", "8.4"} {
		images[runtime] = fmt.Sprintf("ghcr.io/rchehd/phpwarden@sha256:%x", sha256.Sum256([]byte(runtime)))
	}
	data, err := PublishedManifest("v1.0.0", images)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := ParseManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	selected, err := catalog.Resolve(effectiveVersion("11.4.10", "8.4", "8.4", "8.4"), "linux/amd64")
	if err != nil || selected.Development || selected.Image != images["8.4"] || selected.ID != DevelopmentID("8.4") || selected.Release != "v1.0.0" {
		t.Fatalf("%+v %v", selected, err)
	}
	if _, err := catalog.Resolve(effectiveVersion("12.0.0", "8.4", "8.4", "8.4"), "linux/amd64"); err == nil {
		t.Fatal("unverified future Drupal version matched")
	}
	delete(images, "8.1")
	if _, err := PublishedManifest("v1.0.0", images); err == nil {
		t.Fatal("incomplete runtime publication accepted")
	}
	images["8.1"] = "ghcr.io/rchehd/phpwarden:latest"
	if _, err := PublishedManifest("v1.0.0", images); err == nil {
		t.Fatal("mutable image tag accepted")
	}
	images["8.1"] = "ghcr.io/other/image@sha256:" + strings.Repeat("a", 64)
	if _, err := PublishedManifest("v1.0.0", images); err == nil {
		t.Fatal("foreign worker repository accepted")
	}
}
