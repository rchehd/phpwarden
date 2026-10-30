package distribution

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	buildinfo "github.com/rchehd/phpwarden"
	"github.com/rchehd/phpwarden/internal/agentskills"
)

func verifyDeliveryIdentity(t *testing.T, directory, binary string, identity buildinfo.Info) {
	t.Helper()
	executable, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(directory, "compatibility.json"))
	if err != nil {
		t.Fatal(err)
	}
	var notices, inventory []byte
	for _, format := range []struct{ archive, prefix, payload string }{
		{"phpwarden-linux-amd64.tar.gz", "", "phpwarden"},
		{"phpwarden-composer-linux-amd64.zip", "", "payload/phpwarden"},
		{"phpwarden-npm-linux-amd64.tgz", "package/", "payload/phpwarden"},
	} {
		files := readDeliveryArchive(t, filepath.Join(directory, format.archive))
		if !bytes.Equal(files[format.prefix+format.payload], executable) || !bytes.Equal(files[format.prefix+"compatibility.json"], manifest) {
			t.Fatal("binary or worker manifest differs between distribution formats", format.archive)
		}
		var actual buildinfo.Info
		if err := json.Unmarshal(files[format.prefix+"release.json"], &actual); err != nil || actual != identity {
			t.Fatalf("release identity mismatch in %s: %+v (%v)", format.archive, actual, err)
		}
		if notices == nil {
			notices, inventory = files[format.prefix+"THIRD_PARTY_NOTICES.txt"], files[format.prefix+"dependencies.json"]
		}
		if len(notices) == 0 || !json.Valid(inventory) || !bytes.Equal(files[format.prefix+"THIRD_PARTY_NOTICES.txt"], notices) || !bytes.Equal(files[format.prefix+"dependencies.json"], inventory) {
			t.Fatal("dependency material differs between delivery formats", format.archive)
		}
		if format.payload != "phpwarden" {
			want := fmt.Appendf(nil, "%x  phpwarden\n", sha256.Sum256(executable))
			if !bytes.Equal(files[format.prefix+"payload/SHA256SUMS"], want) {
				t.Fatal("managed package does not verify the same executable", format.archive)
			}
		}
	}
	for _, runtime := range []string{"8.1", "8.2", "8.3", "8.4"} {
		files := readDeliveryArchive(t, filepath.Join(directory, "phpwarden-compose-php"+runtime+".tar.gz"))
		want := fmt.Appendf(nil, "ghcr.io/rchehd/phpwarden@sha256:%x\n", sha256.Sum256([]byte(identity.Version+runtime)))
		wrapper, _ := buildinfo.ComposeAssets.ReadFile("examples/compose/phpwarden")
		compose, _ := buildinfo.ComposeAssets.ReadFile("examples/compose/phpwarden.compose.yaml")
		if len(files) != 3 || !bytes.Equal(files[".phpwarden-image"], want) || !bytes.Equal(files["phpwarden"], wrapper) || !bytes.Equal(files[".phpwarden.compose.yaml"], compose) {
			t.Fatal("Compose bundle differs from the CLI's pinned worker selection", runtime)
		}
	}
	plugins := filepath.Join(t.TempDir(), "plugins")
	if err := PluginPackages(identity.Version, identity.Commit, plugins); err != nil {
		t.Fatal(err)
	}
	for _, client := range []string{"codex", "claude", "cursor", "catalogs"} {
		files := readDeliveryArchive(t, filepath.Join(plugins, "phpwarden-plugin-"+client+".zip"))
		prefix := ""
		if client == "catalogs" {
			prefix = pluginSource + "/"
		}
		var actual pluginIdentity
		if err := json.Unmarshal(files[prefix+"phpwarden-plugin.json"], &actual); err != nil || actual.Version != identity.Version || actual.Commit != identity.Commit || actual.RecommendedCLI != identity.Version {
			t.Fatalf("plugin does not identify the matching CLI: %s %+v (%v)", client, actual, err)
		}
		for _, name := range agentskills.Names {
			if len(files[prefix+"skills/"+name+"/SKILL.md"]) == 0 {
				t.Fatal("missing shared skill", client, name)
			}
		}
	}
	for _, component := range []struct {
		directory string
		count     int
	}{{directory, 8}, {plugins, 4}} {
		sums, err := os.ReadFile(filepath.Join(component.directory, "SHA256SUMS"))
		if err != nil {
			t.Fatal(err)
		}
		rows := strings.Split(strings.TrimSpace(string(sums)), "\n")
		if len(rows) != component.count {
			t.Fatalf("checksum inventory is incomplete: %d entries, want %d", len(rows), component.count)
		}
		for _, row := range rows {
			fields := strings.Fields(row)
			if len(fields) != 2 {
				t.Fatal("invalid checksum entry", row)
			}
			data, err := os.ReadFile(filepath.Join(component.directory, fields[1]))
			if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != fields[0] {
				t.Fatal("incorrect delivery checksum", row, err)
			}
		}
	}
}

func readDeliveryArchive(t *testing.T, filename string) map[string][]byte {
	t.Helper()
	files := map[string][]byte{}
	if strings.HasSuffix(filename, ".zip") {
		archive, err := zip.OpenReader(filename)
		if err != nil {
			t.Fatal(err)
		}
		defer archive.Close()
		for _, entry := range archive.File {
			reader, err := entry.Open()
			if err != nil {
				t.Fatal(err)
			}
			files[entry.Name], err = io.ReadAll(reader)
			reader.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		return files
	}
	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer compressed.Close()
	archive := tar.NewReader(compressed)
	for {
		entry, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name], err = io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}
