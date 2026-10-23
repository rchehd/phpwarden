package launcher

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func TestReleaseImageInspectionVerifiesDigestAndManifestLabels(t *testing.T) {
	selected := toolset.Selection{ID: "fixture-toolset", Image: "ghcr.io/example/fixture@" + testImage,
		Platform: "linux/amd64", Protocol: 2, Capabilities: []string{"debug", "phpstan"}, PHPVersion: "8.4.26", Tools: map[string]string{"symfony-yaml": "7.4.21"}}
	for _, which := range []string{"matching", "missing digest", "wrong digest", "wrong platform", "wrong protocol", "wrong toolset", "wrong capabilities", "wrong PHP", "missing image", "wrong YAML"} {
		t.Run(which, func(t *testing.T) {
			labels := map[string]string{"io.phpwarden.protocol": "2", "io.phpwarden.toolset": selected.ID,
				"io.phpwarden.checkers": "debug,phpstan", "io.phpwarden.php-version": selected.PHPVersion, "io.phpwarden.symfony-yaml-version": "7.4.21"}
			digests := []string{selected.Image}
			architecture := "amd64"
			switch which {
			case "missing digest":
				digests = nil
			case "wrong digest":
				digests = []string{"ghcr.io/example/fixture@sha256:" + strings.Repeat("b", 64)}
			case "wrong platform":
				architecture = "arm64"
			case "wrong protocol":
				labels["io.phpwarden.protocol"] = "1"
			case "wrong toolset":
				labels["io.phpwarden.toolset"] = "other"
			case "wrong capabilities":
				labels["io.phpwarden.checkers"] = "debug"
			case "wrong PHP":
				labels["io.phpwarden.php-version"] = "8.4.25"
			case "wrong YAML":
				labels["io.phpwarden.symfony-yaml-version"] = "7.4.20"
			}
			calls := 0
			process := processFunc(func(_ context.Context, command runner.Command) runner.Result {
				calls++
				if command.Name != "docker" || strings.Join(command.Args, " ") != "image inspect "+selected.Image {
					t.Fatalf("inspection must use the manifest digest: %+v", command)
				}
				if which == "missing image" {
					return runner.Result{ExitCode: 1}
				}
				data, err := json.Marshal([]any{map[string]any{"Id": testImage, "Os": "linux", "Architecture": architecture,
					"RepoDigests": digests, "Config": map[string]any{"Labels": labels}}})
				if err != nil {
					t.Fatal(err)
				}
				return runner.Result{Stdout: data}
			})
			image, err := inspect(context.Background(), selected, process)
			if calls != 1 || which == "matching" && (err != nil || image != testImage) || which != "matching" && err == nil {
				t.Fatalf("incorrect image validation: %s %v (%d calls)", image, err, calls)
			}
			if which == "missing image" && (strings.Contains(err.Error(), "docker build") || !strings.Contains(err.Error(), "registry pull is not implemented")) {
				t.Fatalf("release image got development build instructions: %v", err)
			}
		})
	}
}
