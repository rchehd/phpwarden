package distribution

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	buildinfo "github.com/rchehd/phpwarden"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/selfupdate"
	"github.com/rchehd/phpwarden/internal/toolset"
)

func Package(ctx context.Context, binary, readme, manifest, output string) error {
	binary, err := filepath.Abs(binary)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	probe := (runner.Exec{}).Run(ctx, runner.Command{Name: binary, Args: []string{"--build-info"}})
	var info buildinfo.Info
	if probe.Err != nil || probe.ExitCode != 0 || json.Unmarshal(probe.Stdout, &info) != nil || info.Platform != "linux/amd64" || !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(info.Commit) {
		return fmt.Errorf("binary must report a linux/amd64 build with an exact Git commit")
	}
	if normalized, err := selfupdate.Version(info.Version); err != nil || normalized != info.Version {
		return fmt.Errorf("package binary requires a canonical release or development version")
	}
	if manifest == "" {
		for _, pattern := range []string{"compatibility.json", "phpwarden-compose-php*.tar.gz"} {
			matches, err := filepath.Glob(filepath.Join(output, pattern))
			if err != nil {
				return err
			}
			if len(matches) != 0 {
				return fmt.Errorf("development package output contains published compatibility files; use a fresh directory")
			}
		}
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return err
	}
	if len(data) > selfupdate.MaxArchive {
		return fmt.Errorf("binary exceeds release size limit")
	}
	doc, err := os.ReadFile(readme)
	if err != nil {
		return err
	}
	metadata, _ := json.MarshalIndent(info, "", "  ")
	files := map[string][]byte{"phpwarden": data, "README.md": doc, "COMMIT": []byte(info.Commit + "\n"), "release.json": append(metadata, '\n')}
	for _, entry := range []struct {
		name string
		args []string
	}{
		{"THIRD_PARTY_NOTICES.txt", []string{"--licenses"}},
		{"dependencies.json", []string{"--licenses", "--format=json"}},
	} {
		result := (runner.Exec{}).Run(ctx, runner.Command{Name: binary, Args: entry.args})
		if result.Err != nil || result.ExitCode != 0 || len(result.Stdout) == 0 || len(result.Stdout) > 1<<20 || entry.name == "dependencies.json" && !json.Valid(result.Stdout) {
			return fmt.Errorf("binary must supply bounded embedded license material")
		}
		files[entry.name] = result.Stdout
	}
	if manifest != "" {
		data, err := os.ReadFile(manifest)
		if err != nil {
			return err
		}
		if _, err := toolset.ParseManifest(data); err != nil {
			return err
		}
		var identity struct {
			Release string `json:"release"`
		}
		json.Unmarshal(data, &identity)
		if identity.Release != info.Version || info.ManifestSHA256 != fmt.Sprintf("%x", sha256.Sum256(data)) || info.Distribution != "standalone" {
			return fmt.Errorf("binary and release manifest identity differ")
		}
		files["compatibility.json"] = data
	} else if info.ManifestSHA256 != "" || info.Distribution != "source" {
		return fmt.Errorf("development archive must not claim standalone release compatibility")
	}
	var buffer bytes.Buffer
	zip := gzip.NewWriter(&buffer)
	archive := tar.NewWriter(zip)
	for _, name := range []string{"phpwarden", "README.md", "COMMIT", "release.json", "compatibility.json", "THIRD_PARTY_NOTICES.txt", "dependencies.json"} {
		data, exists := files[name]
		if !exists {
			continue
		}
		mode := int64(0o644)
		if name == "phpwarden" {
			mode = 0o755
		}
		if err := archive.WriteHeader(&tar.Header{Name: name, Mode: mode, Size: int64(len(data)), Typeflag: tar.TypeReg}); err != nil {
			return err
		}
		if _, err := archive.Write(data); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	if err := zip.Close(); err != nil {
		return err
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		return err
	}
	name := "phpwarden-linux-amd64.tar.gz"
	if err := os.WriteFile(filepath.Join(output, name), buffer.Bytes(), 0o644); err != nil {
		return err
	}
	if data, exists := files["compatibility.json"]; exists {
		if err := os.WriteFile(filepath.Join(output, "compatibility.json"), data, 0o644); err != nil {
			return err
		}
	}
	if err := projectPackages(files, info, output); err != nil {
		return err
	}
	names := []string{name, "phpwarden-composer-linux-amd64.zip", "phpwarden-npm-linux-amd64.tgz"}
	if files["compatibility.json"] != nil {
		names = append(names, "compatibility.json")
		compose, err := filepath.Glob(filepath.Join(output, "phpwarden-compose-php*.tar.gz"))
		if err != nil {
			return err
		}
		for _, path := range compose {
			names = append(names, filepath.Base(path))
		}
	}
	var sums []byte
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(output, name))
		if err != nil {
			return err
		}
		sums = fmt.Appendf(sums, "%x  %s\n", sha256.Sum256(data), name)
	}
	return os.WriteFile(filepath.Join(output, "SHA256SUMS"), sums, 0o644)
}
