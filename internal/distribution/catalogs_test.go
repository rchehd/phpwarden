package distribution

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/agentskills"
)

func TestCatalogSourcesMatchRepositoryAndResolveSharedSkills(t *testing.T) {
	output := filepath.Join(t.TempDir(), "catalogs")
	if err := CatalogSources(output); err != nil {
		t.Fatal(err)
	}
	files, err := catalogFiles("")
	if err != nil {
		t.Fatal(err)
	}
	for name, expected := range files {
		generated, err := os.ReadFile(filepath.Join(output, filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(generated, expected) {
			t.Fatalf("generated source %s differs: %v", name, err)
		}
		actual, err := os.ReadFile(filepath.Join("../..", filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(actual, expected) {
			t.Fatalf("repository catalog drift at %s; regenerate with phpwarden-dist catalogs: %v", name, err)
		}
	}
	verifyCatalogTrees(t, os.DirFS("../.."), "")
	foreign := filepath.Join(output, "user.txt")
	if err := os.WriteFile(foreign, []byte("preserve"), 0o644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(filepath.Dir(output), "alias")
	if err := os.Symlink(output, alias); err != nil {
		t.Fatal(err)
	}
	for _, occupied := range []string{output, alias, foreign} {
		if err := CatalogSources(occupied); err == nil {
			t.Fatalf("overwrote occupied catalog output %s", occupied)
		}
	}
	if data, _ := os.ReadFile(foreign); string(data) != "preserve" {
		t.Fatal("modified foreign content")
	}
}

func TestCatalogBundleIsSelfContainedAndMatchesClientPackages(t *testing.T) {
	output := filepath.Join(t.TempDir(), "packages")
	version, commit := "v1.2.3-rc.1", strings.Repeat("c", 40)
	if err := PluginPackages(version, commit, output); err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(filepath.Join(output, "phpwarden-plugin-catalogs.zip"))
	if err != nil {
		t.Fatal(err)
	}
	sums, err := os.ReadFile(filepath.Join(output, "SHA256SUMS"))
	if err != nil || len(strings.Fields(string(sums))) != 8 || !bytes.Contains(sums, fmt.Appendf(nil, "%x  phpwarden-plugin-catalogs.zip\n", sha256.Sum256(archive))) {
		t.Fatal("missing catalog bundle checksum", err)
	}
	files := readPluginArchive(t, archive)
	root := t.TempDir()
	for name, data := range files {
		target := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	verifyCatalogTrees(t, os.DirFS(root), strings.TrimPrefix(version, "v"))
	count := 0
	if err := fs.WalkDir(agentskills.Resources(), "skills", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := fs.ReadFile(agentskills.Resources(), name)
		if err != nil {
			return err
		}
		if !bytes.Equal(files[pluginSource+"/"+name], data) {
			t.Fatalf("missing or modified bundled resource %s", name)
		}
		count++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(files) != count+9 {
		t.Fatalf("unexpected bundled content: %d resources, %d files", count, len(files))
	}
	for _, client := range pluginClients {
		data, err := os.ReadFile(filepath.Join(output, "phpwarden-plugin-"+client+".zip"))
		if err != nil {
			t.Fatal(err)
		}
		clientFiles := readPluginArchive(t, data)
		name := "." + client + "-plugin/plugin.json"
		if !bytes.Equal(files[pluginSource+"/"+name], clientFiles[name]) {
			t.Fatalf("manifest identity differs for %s", client)
		}
	}
	var identity pluginIdentity
	if err := json.Unmarshal(files[pluginSource+"/phpwarden-plugin.json"], &identity); err != nil || identity.Client != "catalogs" || identity.Version != version || identity.Commit != commit {
		t.Fatalf("incorrect catalog identity: %+v (%v)", identity, err)
	}
}

func verifyCatalogTrees(t *testing.T, root fs.FS, version string) {
	t.Helper()
	for _, client := range pluginClients {
		catalogPath := "." + client + "-plugin/marketplace.json"
		if client == "codex" {
			catalogPath = ".agents/plugins/marketplace.json"
		}
		data, err := fs.ReadFile(root, catalogPath)
		if err != nil {
			t.Fatal(err)
		}
		var catalog struct {
			Name    string
			Plugins []struct {
				Name, Category string
				Source         json.RawMessage
				Policy         map[string]string
			}
		}
		if err := json.Unmarshal(data, &catalog); err != nil || catalog.Name != "phpwarden-dev" || len(catalog.Plugins) != 1 || catalog.Plugins[0].Name != "phpwarden" {
			t.Fatalf("invalid %s catalog: %s (%v)", client, data, err)
		}
		entry := catalog.Plugins[0]
		var source string
		if client == "codex" {
			var local struct{ Source, Path string }
			if err := json.Unmarshal(entry.Source, &local); err != nil || local.Source != "local" || entry.Policy["installation"] != "AVAILABLE" || entry.Policy["authentication"] != "ON_INSTALL" || entry.Category == "" {
				t.Fatalf("invalid Codex local source/policy: %s", data)
			}
			source = local.Path
		} else if err := json.Unmarshal(entry.Source, &source); err != nil {
			t.Fatal(err)
		}
		source = strings.TrimPrefix(source, "./")
		if source != pluginSource || !fs.ValidPath(source) || path.Clean(source) != source {
			t.Fatalf("unsafe or unexpected catalog source %q", source)
		}
		manifestData, err := fs.ReadFile(root, source+"/."+client+"-plugin/plugin.json")
		if err != nil {
			t.Fatal("catalog cannot reach native manifest", err)
		}
		var manifest map[string]any
		if err := json.Unmarshal(manifestData, &manifest); err != nil || manifest["name"] != entry.Name {
			t.Fatal("native manifest does not match catalog", err)
		}
		if version == "" && manifest["version"] != nil || version != "" && manifest["version"] != version {
			t.Fatalf("unexpected %s manifest version: %v", client, manifest["version"])
		}
		for _, name := range agentskills.Names {
			if _, err := fs.ReadFile(root, source+"/skills/"+name+"/SKILL.md"); err != nil {
				t.Fatalf("%s catalog cannot reach skill %s: %v", client, name, err)
			}
		}
	}
}
