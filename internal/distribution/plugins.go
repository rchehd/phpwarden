package distribution

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rchehd/phpwarden/internal/agentskills"
	"github.com/rchehd/phpwarden/internal/selfupdate"
)

//go:embed templates/plugin-readme.md
var pluginReadme string

type pluginIdentity struct {
	SchemaVersion   int      `json:"schema_version"`
	Client          string   `json:"client"`
	Version         string   `json:"version"`
	Commit          string   `json:"commit"`
	Stage           string   `json:"stage"`
	RecommendedCLI  string   `json:"recommended_cli_version"`
	Skills          []string `json:"skills"`
	CompletionHooks bool     `json:"completion_hooks"`
	PerformanceMCP  bool     `json:"performance_mcp"`
}

func PluginPackages(version, commit, output string) error {
	normalized, err := selfupdate.Version(version)
	if err != nil || normalized != version {
		return fmt.Errorf("plugins require a canonical vMAJOR.MINOR.PATCH release or development version")
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(commit) {
		return fmt.Errorf("plugins require an exact Git commit")
	}
	packages := map[string][]byte{}
	for _, client := range []string{"codex", "cursor", "claude"} {
		files, err := pluginFiles(client, version, commit)
		if err != nil {
			return err
		}
		data, err := zipFiles(files)
		if err != nil {
			return err
		}
		packages["phpwarden-plugin-"+client+".zip"] = data
	}
	var sums []byte
	for _, name := range archiveNames(packages) {
		sums = fmt.Appendf(sums, "%x  %s\n", sha256.Sum256(packages[name]), name)
	}
	packages["SHA256SUMS"] = sums
	if err := os.Mkdir(output, 0o755); err != nil {
		return fmt.Errorf("plugin output must be a new directory with an existing parent: %w", err)
	}
	for _, name := range archiveNames(packages) {
		if err := os.WriteFile(filepath.Join(output, name), packages[name], 0o644); err != nil {
			return err
		}
	}
	return nil
}

func pluginFiles(client, version, commit string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := fs.WalkDir(agentskills.Resources(), "skills", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := fs.ReadFile(agentskills.Resources(), name)
		if err == nil {
			files[name] = data
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	manifest := map[string]any{
		"name": "phpwarden", "version": strings.TrimPrefix(version, "v"),
		"description": "Project quality review, architecture, upgrades, tests and Markdown reports with PHPWarden",
		"author":      map[string]string{"name": "rchehd"},
		"homepage":    "https://github.com/rchehd/phpwarden",
		"repository":  "https://github.com/rchehd/phpwarden",
		"keywords":    []string{"php", "drupal", "quality", "security", "reports"},
	}
	if client == "codex" {
		manifest["skills"] = "./skills/"
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	files["."+client+"-plugin/plugin.json"] = append(data, '\n')
	identity := pluginIdentity{
		SchemaVersion: 1, Client: client, Version: version, Commit: commit,
		Stage: "skills-only", RecommendedCLI: version, Skills: agentskills.Names,
	}
	data, err = json.MarshalIndent(identity, "", "  ")
	if err != nil {
		return nil, err
	}
	files["phpwarden-plugin.json"] = append(data, '\n')
	files["README.md"] = []byte(strings.NewReplacer("@@CLIENT@@", client, "@@VERSION@@", version, "@@COMMIT@@", commit).Replace(pluginReadme))
	return files, nil
}
