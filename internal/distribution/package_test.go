package distribution

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	buildinfo "github.com/rchehd/phpwarden"
	"github.com/rchehd/phpwarden/internal/selfupdate"
	"github.com/rchehd/phpwarden/internal/toolset"
)

type transportFunc func(*http.Request) (*http.Response, error)

func (transport transportFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func buildRelease(t *testing.T, root, version string) (string, string, buildinfo.Info) {
	t.Helper()
	source, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]string{}
	for _, runtime := range []string{"8.1", "8.2", "8.3", "8.4"} {
		refs[runtime] = fmt.Sprintf("ghcr.io/rchehd/phpwarden@sha256:%x", sha256.Sum256([]byte(version+runtime)))
	}
	manifest, err := toolset.PublishedManifest(version, refs)
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, version)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	manifestFile := filepath.Join(directory, "compatibility.json")
	os.WriteFile(manifestFile, manifest, 0o644)
	binary := filepath.Join(directory, "phpwarden")
	flags := "-X github.com/rchehd/phpwarden.Version=" + version + " -X github.com/rchehd/phpwarden.Commit=" + strings.Repeat("b", 40) + " -X github.com/rchehd/phpwarden.Distribution=standalone -X github.com/rchehd/phpwarden.ManifestBase64=" + base64.StdEncoding.EncodeToString(manifest)
	command := exec.Command("go", "build", "-buildvcs=false", "-trimpath", "-ldflags", flags, "-o", binary, "./cmd/phpwarden")
	command.Dir = source
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build release fixture: %v: %s", err, output)
	}
	output, err := exec.Command(binary, "--build-info").Output()
	var metadata buildinfo.Info
	if err != nil || json.Unmarshal(output, &metadata) != nil {
		t.Fatalf("build metadata %v %s", err, output)
	}
	return binary, manifestFile, metadata
}

func TestBuiltReleaseArchiveCanAtomicallyUpdateRealCLI(t *testing.T) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("initial release platform is linux/amd64")
	}
	root := t.TempDir()
	oldBinary, _, current := buildRelease(t, root, "v1.0.0")
	newBinary, manifest, target := buildRelease(t, root, "v1.1.0")
	readme := filepath.Join(root, "README.md")
	os.WriteFile(readme, []byte("fixture README\n"), 0o644)
	directory := filepath.Join(root, "archive")
	if err := Package(context.Background(), newBinary, readme, manifest, directory); err != nil {
		t.Fatal(err)
	}
	archive, _ := os.ReadFile(filepath.Join(directory, "phpwarden-linux-amd64.tar.gz"))
	sums, _ := os.ReadFile(filepath.Join(directory, "SHA256SUMS"))
	base := "https://github.com/rchehd/phpwarden/releases/download/" + target.Version + "/"
	release := selfupdate.Release{Tag: target.Version, Assets: []selfupdate.Asset{{Name: "phpwarden-linux-amd64.tar.gz", URL: base + "phpwarden-linux-amd64.tar.gz", Size: int64(len(archive))}, {Name: "SHA256SUMS", URL: base + "SHA256SUMS", Size: int64(len(sums))}}}
	client := selfupdate.Client{HTTP: &http.Client{Transport: transportFunc(func(request *http.Request) (*http.Response, error) {
		var data []byte
		switch request.URL.String() {
		case "https://api.github.com/repos/rchehd/phpwarden/releases/latest":
			data, _ = json.Marshal(release)
		case release.Assets[0].URL:
			data = archive
		case release.Assets[1].URL:
			data = sums
		default:
			return nil, fmt.Errorf("unexpected URL %s", request.URL)
		}
		return &http.Response{StatusCode: 200, ContentLength: int64(len(data)), Body: io.NopCloser(bytes.NewReader(data)), Header: http.Header{}}, nil
	})}}
	settings := filepath.Join(filepath.Dir(oldBinary), ".phpwarden.yaml")
	os.WriteFile(settings, []byte("preserve project settings\n"), 0o644)
	result := selfupdate.Run(context.Background(), selfupdate.Options{Current: current, Executable: oldBinary, Client: client})
	if result.ExitCode != 0 || !result.Updated {
		t.Fatalf("%+v", result)
	}
	output, err := exec.Command(oldBinary, "--build-info").Output()
	if err != nil || !selfupdate.SameInfo(output, target) {
		t.Fatalf("installed binary is wrong: %v %s", err, output)
	}
	project := filepath.Join(root, "project")
	os.Mkdir(project, 0o755)
	os.WriteFile(filepath.Join(project, ".phpwarden.yaml"), []byte("version: 1\nprofile: drupal\nframework: {version: '11.4.10'}\nphp:\n  runtime: '8.4'\n  target: {min: '8.4', max: '8.4'}\n"), 0o644)
	os.WriteFile(filepath.Join(project, "composer.json"), []byte(`{"require":{"drupal/core":"^11"}}`), 0o644)
	os.WriteFile(filepath.Join(project, "composer.lock"), []byte(`{"packages":[{"name":"drupal/core","version":"11.4.10","require":{"php":">=8.3"}}]}`), 0o644)
	resolved, err := exec.Command(oldBinary, "resolve", "--project", project, "--format=json").CombinedOutput()
	var resolution struct {
		Selection struct {
			Release     string `json:"release"`
			Development bool   `json:"development"`
			Image       string `json:"image"`
		} `json:"selection"`
	}
	if err != nil || json.Unmarshal(resolved, &resolution) != nil || resolution.Selection.Release != target.Version || resolution.Selection.Development || !strings.HasPrefix(resolution.Selection.Image, "ghcr.io/rchehd/phpwarden@sha256:") {
		t.Fatalf("updated CLI lost embedded image resolution: %v %s", err, resolved)
	}
	if value, _ := os.ReadFile(settings); string(value) != "preserve project settings\n" {
		t.Fatal("updater modified project config")
	}
	if err := Package(context.Background(), oldBinary, readme, "", filepath.Join(root, "invalid")); err == nil {
		t.Fatal("release archive omitted required embedded manifest")
	}
}
