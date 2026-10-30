package distribution

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/agentskills"
	"go.yaml.in/yaml/v3"
)

func TestPluginArchivesPreserveSharedSkillsAndResolveReferences(t *testing.T) {
	output := filepath.Join(t.TempDir(), "packages")
	version, commit := "v0.0.0-dev.abcdef0", strings.Repeat("a", 40)
	if err := PluginPackages(version, commit, output); err != nil {
		t.Fatal(err)
	}
	sums, err := os.ReadFile(filepath.Join(output, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, client := range []string{"codex", "cursor", "claude"} {
		t.Run(client, func(t *testing.T) {
			name := "phpwarden-plugin-" + client + ".zip"
			archive, err := os.ReadFile(filepath.Join(output, name))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(sums), fmt.Sprintf("%x  %s\n", sha256.Sum256(archive), name)) {
				t.Fatal("archive checksum missing or incorrect")
			}
			files := readPluginArchive(t, archive)
			var identity pluginIdentity
			if err := json.Unmarshal(files["phpwarden-plugin.json"], &identity); err != nil {
				t.Fatal(err)
			}
			if identity.Client != client || identity.Version != version || identity.Commit != commit || identity.SchemaVersion != 1 || identity.Stage != "skills-only" || identity.RecommendedCLI != version || identity.CompletionHooks || identity.PerformanceMCP || !slices.Equal(identity.Skills, agentskills.Names) {
				t.Fatalf("incorrect package identity: %+v", identity)
			}
			var manifest map[string]any
			if err := json.Unmarshal(files["."+client+"-plugin/plugin.json"], &manifest); err != nil {
				t.Fatal(err)
			}
			if manifest["name"] != "phpwarden" || manifest["version"] != strings.TrimPrefix(version, "v") {
				t.Fatalf("invalid native manifest: %+v", manifest)
			}
			if client == "codex" && manifest["skills"] != "./skills/" {
				t.Fatal("Codex compatibility manifest does not declare skills")
			}
			for _, field := range []string{"hooks", "mcpServers", "apps", "license"} {
				if _, exists := manifest[field]; exists {
					t.Fatalf("manifest unexpectedly declares %s", field)
				}
			}
			for _, name := range agentskills.Names {
				data := files["skills/"+name+"/SKILL.md"]
				parts := bytes.SplitN(data, []byte("---"), 3)
				var frontmatter struct{ Name, Description string }
				if len(parts) != 3 || len(parts[0]) != 0 || yaml.Unmarshal(parts[1], &frontmatter) != nil || frontmatter.Name != name || frontmatter.Description == "" {
					t.Fatalf("skill %s is not discoverable", name)
				}
			}
			count := 0
			err = fs.WalkDir(agentskills.Resources(), "skills", func(name string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() {
					return err
				}
				original, err := fs.ReadFile(agentskills.Resources(), name)
				if err != nil {
					return err
				}
				if !bytes.Equal(files[name], original) {
					t.Fatalf("shared resource %s changed or missing", name)
				}
				count++
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(files) != count+3 {
				t.Fatal("archive contains undeclared content")
			}
			for name, data := range files {
				if !strings.HasSuffix(name, ".md") || name == "README.md" {
					continue
				}
				for _, link := range regexp.MustCompile(`\]\(([^)]+)\)`).FindAllSubmatch(data, -1) {
					ref, err := url.Parse(string(link[1]))
					if err != nil {
						t.Fatal(err)
					}
					if ref.IsAbs() {
						if ref.Scheme != "https" || ref.Host == "" {
							t.Fatalf("unsafe external reference %s", ref)
						}
						continue
					}
					target := path.Clean(path.Join(path.Dir(name), ref.Path))
					if !strings.HasPrefix(target, "skills/") || files[target] == nil {
						t.Fatalf("broken or escaping packaged reference: %s -> %s", name, target)
					}
				}
			}
			if bytes.Contains(files["README.md"], []byte("@@")) {
				t.Fatal("README contains unresolved placeholders")
			}
		})
	}
}

func readPluginArchive(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, entry := range reader.File {
		if !fs.ValidPath(entry.Name) || path.Clean(entry.Name) != entry.Name || !entry.Mode().IsRegular() || entry.Mode().Perm() != 0o644 || files[entry.Name] != nil {
			t.Fatalf("unsafe or duplicate archive entry %s (%s)", entry.Name, entry.Mode())
		}
		file, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		files[entry.Name], err = io.ReadAll(file)
		file.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	return files
}

func TestPluginPackagesAreReproducibleAndPreserveOccupiedOutput(t *testing.T) {
	root := t.TempDir()
	version, commit := "v1.2.3-rc.1", strings.Repeat("b", 40)
	first, second := filepath.Join(root, "first"), filepath.Join(root, "second")
	for _, output := range []string{first, second} {
		if err := PluginPackages(version, commit, output); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(first)
	for _, entry := range entries {
		a, _ := os.ReadFile(filepath.Join(first, entry.Name()))
		b, _ := os.ReadFile(filepath.Join(second, entry.Name()))
		if !bytes.Equal(a, b) {
			t.Fatalf("nondeterministic artifact %s", entry.Name())
		}
	}
	foreign := filepath.Join(first, "phpwarden-plugin-codex.zip")
	if err := os.WriteFile(foreign, []byte("user file"), 0o644); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(first, alias); err != nil {
		t.Fatal(err)
	}
	for _, output := range []string{first, alias, foreign} {
		if err := PluginPackages(version, commit, output); err == nil {
			t.Fatal("accepted occupied output", output)
		}
	}
	if data, _ := os.ReadFile(foreign); string(data) != "user file" {
		t.Fatal("packaging modified occupied output")
	}
}

func TestPluginPackagesRejectInvalidIdentityBeforeWriting(t *testing.T) {
	for _, identity := range [][2]string{
		{"", strings.Repeat("a", 40)}, {"1.2.3", strings.Repeat("a", 40)},
		{"v1.2.3-01", strings.Repeat("a", 40)}, {"v1.2.3", "HEAD"},
		{"v1.2.3", "abc1234"}, {"v1.2.3", strings.Repeat("A", 40)},
	} {
		output := filepath.Join(t.TempDir(), "packages")
		if err := PluginPackages(identity[0], identity[1], output); err == nil {
			t.Fatal("accepted invalid identity", identity)
		}
		if _, err := os.Lstat(output); !os.IsNotExist(err) {
			t.Fatal("invalid identity created output", err)
		}
	}
}
