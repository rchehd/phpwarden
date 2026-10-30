package distribution

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	buildinfo "github.com/rchehd/phpwarden"
)

const pluginSource = "internal/agentskills"

var pluginClients = []string{"codex", "cursor", "claude"}

func CatalogSources(output string) error {
	files, err := catalogFiles("")
	if err != nil {
		return err
	}
	if err := os.Mkdir(output, 0o755); err != nil {
		return fmt.Errorf("catalog output must be a new directory with an existing parent: %w", err)
	}
	for _, name := range archiveNames(files) {
		target := filepath.Join(output, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, files[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func catalogFiles(version string) (map[string][]byte, error) {
	files := map[string][]byte{}
	files[pluginSource+"/skills/phpwarden-setup/scripts/install.sh"] = buildinfo.Installer
	for _, client := range pluginClients {
		entry := map[string]any{
			"name": "phpwarden", "source": "./" + pluginSource,
			"description": "PHPWarden project quality skills",
		}
		catalogPath := "." + client + "-plugin/marketplace.json"
		if client == "codex" {
			catalogPath = ".agents/plugins/marketplace.json"
			entry["source"] = map[string]string{"source": "local", "path": "./" + pluginSource}
			entry["policy"] = map[string]string{"installation": "AVAILABLE", "authentication": "ON_INSTALL"}
			entry["category"] = "Developer Tools"
		}
		catalog := map[string]any{
			"name": "phpwarden-dev", "owner": map[string]string{"name": "rchehd"},
			"plugins": []any{entry},
		}
		data, err := json.MarshalIndent(catalog, "", "  ")
		if err != nil {
			return nil, err
		}
		files[catalogPath] = append(data, '\n')
		data, err = pluginManifest(client, version)
		if err != nil {
			return nil, err
		}
		files[pluginSource+"/."+client+"-plugin/plugin.json"] = data
	}
	return files, nil
}

func catalogBundleFiles(version, commit string) (map[string][]byte, error) {
	files, err := catalogFiles(version)
	if err != nil {
		return nil, err
	}
	plugin, err := pluginFiles("codex", version, commit)
	if err != nil {
		return nil, err
	}
	for name, data := range plugin {
		if name != "phpwarden-plugin.json" && name != "README.md" {
			files[pluginSource+"/"+name] = data
		}
	}
	var identity pluginIdentity
	if err := json.Unmarshal(plugin["phpwarden-plugin.json"], &identity); err != nil {
		return nil, err
	}
	identity.Client = "catalogs"
	data, err := json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return nil, err
	}
	files[pluginSource+"/phpwarden-plugin.json"] = append(data, '\n')
	files[pluginSource+"/README.md"] = renderPluginReadme("all", version, commit)
	files["README.md"] = []byte("# PHPWarden plugin catalogs\n\nVersion: " + version + "\nCommit: " + commit + "\n\nExtract into a new directory and keep hidden files. Add that directory as a\nlocal marketplace using your client's supported flow. Catalogs resolve the\nplugin at internal/agentskills; do not move its files separately.\n\nThe bundle contains skills only. CLI installation, project instructions,\ncompletion hooks and performance MCP are separate operations.\nSee https://github.com/rchehd/phpwarden/blob/" + commit + "/plugins/README.md\n")
	return files, nil
}
