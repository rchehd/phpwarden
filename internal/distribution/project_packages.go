package distribution

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	buildinfo "github.com/rchehd/phpwarden"
)

//go:embed templates/launcher.sh
var packageLauncher string

var composerPrerelease = regexp.MustCompile(`(?i)^(?:dev|(?:alpha|beta|rc)(?:[.-]?[0-9]+)?)$`)

func composerPackageVersion(version string) string {
	version = strings.TrimPrefix(version, "v")
	_, suffix, prerelease := strings.Cut(version, "-")
	if !prerelease || composerPrerelease.MatchString(suffix) {
		return version
	}
	return "dev-" + version
}

func projectPackages(files map[string][]byte, info buildinfo.Info, output string) error {
	version := strings.TrimPrefix(info.Version, "v")
	for _, manager := range []string{"composer", "npm"} {
		description := "PHPWarden Linux amd64 CLI with pinned worker compatibility"
		if files["compatibility.json"] == nil {
			description = "PHPWarden development CLI requiring matching preloaded workers"
		}
		contents := map[string][]byte{
			"bin/phpwarden":           []byte(strings.ReplaceAll(packageLauncher, "@@INSTALLATION@@", manager)),
			"payload/phpwarden":       files["phpwarden"],
			"payload/SHA256SUMS":      fmt.Appendf(nil, "%x  phpwarden\n", sha256.Sum256(files["phpwarden"])),
			"release.json":            files["release.json"],
			"README.md":               files["README.md"],
			"THIRD_PARTY_NOTICES.txt": files["THIRD_PARTY_NOTICES.txt"],
			"dependencies.json":       files["dependencies.json"],
		}
		if manifest, exists := files["compatibility.json"]; exists {
			contents["compatibility.json"] = manifest
		}
		var metadata map[string]any
		if manager == "composer" {
			metadata = map[string]any{
				"name": "rchehd/phpwarden", "version": composerPackageVersion(info.Version), "type": "library",
				"description": description,
				"bin":         []string{"bin/phpwarden"},
			}
		} else {
			metadata = map[string]any{
				"name": "@rchehd/phpwarden", "version": version,
				"description": description,
				"bin":         map[string]string{"phpwarden": "bin/phpwarden"},
				"os":          []string{"linux"}, "cpu": []string{"x64"},
				"files": []string{"bin", "payload", "release.json", "compatibility.json", "README.md", "THIRD_PARTY_NOTICES.txt", "dependencies.json"},
			}
		}
		data, _ := json.MarshalIndent(metadata, "", "  ")
		name := "package.json"
		if manager == "composer" {
			name = "composer.json"
		}
		contents[name] = append(data, '\n')
		if manager == "composer" {
			data, err := zipFiles(contents)
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(output, "phpwarden-composer-linux-amd64.zip"), data, 0o644); err != nil {
				return err
			}
		} else {
			data, err := tarFiles(contents, "package/")
			if err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(output, "phpwarden-npm-linux-amd64.tgz"), data, 0o644); err != nil {
				return err
			}
		}
	}
	if files["compatibility.json"] == nil {
		return nil
	}
	var manifest struct {
		Entries []struct {
			PHP struct {
				Runtime string `json:"runtime"`
			} `json:"php"`
			Images []struct {
				Platform  string `json:"platform"`
				Reference string `json:"reference"`
			} `json:"images"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(files["compatibility.json"], &manifest); err != nil {
		return err
	}
	images := map[string]string{}
	for _, entry := range manifest.Entries {
		for _, image := range entry.Images {
			if image.Platform != "linux/amd64" {
				continue
			}
			if previous, exists := images[entry.PHP.Runtime]; exists && previous != image.Reference {
				return fmt.Errorf("ambiguous Compose image for PHP %s", entry.PHP.Runtime)
			}
			images[entry.PHP.Runtime] = image.Reference
		}
	}
	wrapper, _ := buildinfo.ComposeAssets.ReadFile("examples/compose/phpwarden")
	compose, _ := buildinfo.ComposeAssets.ReadFile("examples/compose/phpwarden.compose.yaml")
	for runtime, image := range images {
		data, err := tarFiles(map[string][]byte{"phpwarden": wrapper, ".phpwarden.compose.yaml": compose, ".phpwarden-image": []byte(image + "\n")}, "")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(output, "phpwarden-compose-php"+runtime+".tar.gz"), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func archiveNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func archiveMode(name string) int64 {
	if name == "bin/phpwarden" || name == "payload/phpwarden" || name == "phpwarden" {
		return 0o755
	}
	return 0o644
}

func zipFiles(files map[string][]byte) ([]byte, error) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range archiveNames(files) {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetMode(os.FileMode(archiveMode(name)))
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return nil, err
		}
		if _, err := entry.Write(files[name]); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func tarFiles(files map[string][]byte, prefix string) ([]byte, error) {
	var buffer bytes.Buffer
	compressed := gzip.NewWriter(&buffer)
	writer := tar.NewWriter(compressed)
	for _, name := range archiveNames(files) {
		data := files[name]
		if err := writer.WriteHeader(&tar.Header{Name: prefix + name, Mode: archiveMode(name), Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return nil, err
		}
		if _, err := writer.Write(data); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	if err := compressed.Close(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}
