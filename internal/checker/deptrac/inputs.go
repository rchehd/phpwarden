package deptrac

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/checker/phpstan"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/edit"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/internal/scope"
	"go.yaml.in/yaml/v3"
)

var ConfigCandidates = []string{"deptrac.yaml", "deptrac.yml", "deptrac.yaml.dist", "deptrac.yml.dist"}

func Select(selected scope.Selection, cfg config.Deptrac) scope.Selection {
	return selected.Filter(cfg.Include, cfg.Exclude, func(name string) bool { return path.Ext(name) == ".php" })
}

func InputRequired(cfg config.Config, selected scope.Selection, only []string) bool {
	return checker.Enabled("deptrac", cfg.Tools.Deptrac.Enabled, only) && len(Select(selected, cfg.Tools.Deptrac).Files) > 0
}

func ContextHash(root string) string { return phpstan.ContextHash(root) }

func prepare(directory, composerRoot string, cfg config.Deptrac) (string, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", err
	}
	defer root.Close()
	filename := cfg.Config
	if filename == "" && cfg.DiscoverConfig {
		for _, dir := range slices.Compact([]string{".", composerRoot}) {
			for _, candidate := range ConfigCandidates {
				name := path.Join(dir, candidate)
				if _, _, err := edit.Read(root, name); os.IsNotExist(err) {
					continue
				} else if err != nil {
					return "", fmt.Errorf("Deptrac config %q is unreadable or unsafe", name)
				}
				filename = name
				break
			}
			if filename != "" {
				break
			}
		}
	}
	if filename == "" {
		return "", nil
	}
	loaded, active := map[string]bool{}, map[string]bool{}
	var load func(string) error
	load = func(name string) error {
		if active[name] {
			return fmt.Errorf("Deptrac YAML imports contain a cycle")
		}
		if loaded[name] {
			return nil
		}
		if len(loaded) >= 128 || !yamlFile(name) {
			return fmt.Errorf("Deptrac accepts at most 128 local YAML config/import files")
		}
		data, _, err := edit.Read(root, name)
		if err != nil || len(data) > 4<<20 {
			return fmt.Errorf("Deptrac config/import %q must be a readable regular file at most 4 MiB", name)
		}
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		var document yaml.Node
		if err := decoder.Decode(&document); err != nil || validateNode(&document) != nil {
			return fmt.Errorf("Deptrac config/import %q contains invalid or unsupported YAML", name)
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return fmt.Errorf("Deptrac config must contain one YAML document")
		}
		var values map[string]any
		if err := document.Decode(&values); err != nil || values == nil {
			return fmt.Errorf("Deptrac config must be a YAML object")
		}
		for key := range values {
			if key != "deptrac" && key != "imports" {
				return fmt.Errorf("Deptrac supports native deptrac settings and local YAML imports; services, parameters and PHP configs are unsupported")
			}
		}
		loaded[name], active[name] = true, true
		if imports, exists := values["imports"]; exists {
			rows, ok := imports.([]any)
			if !ok {
				return fmt.Errorf("Deptrac imports must be a list")
			}
			for _, item := range rows {
				resource, ok := item.(string)
				if entry, mapping := item.(map[string]any); mapping && len(entry) == 1 {
					resource, ok = entry["resource"].(string)
				}
				if !ok || resource == "" || strings.ContainsAny(resource, "%*?{}") || strings.Contains(resource, "://") || path.IsAbs(resource) {
					return fmt.Errorf("Deptrac imports must name explicit local YAML resources")
				}
				imported := path.Join(path.Dir(name), resource)
				if err := projectpath.Validate(imported); err != nil {
					return err
				}
				if err := load(imported); err != nil {
					return err
				}
			}
		}
		if raw, exists := values["deptrac"]; exists {
			settings, ok := raw.(map[string]any)
			if !ok {
				return fmt.Errorf("Deptrac settings must be an object")
			}
			if flags, ok := settings["feature_flags"].(map[string]any); ok && flags["phpstan_parser"] == true {
				return fmt.Errorf("Deptrac uses its standard AST parser; phpstan_parser is unsupported")
			}
			if paths, exists := settings["paths"]; exists {
				rows, ok := paths.([]any)
				if !ok {
					return fmt.Errorf("Deptrac paths must be a list of project directories")
				}
				for _, item := range rows {
					value, ok := item.(string)
					if !ok || strings.Contains(value, "%") || projectpath.Validate(value) != nil {
						return fmt.Errorf("Deptrac paths must be relative project directories without parameters")
					}
					if err := validateTree(directory, path.Join(path.Dir(filename), value)); err != nil {
						return err
					}
				}
			}
			if err := validateCollectors(root, settings); err != nil {
				return err
			}
		}
		delete(active, name)
		return nil
	}
	if err := load(filename); err != nil {
		return "", err
	}
	for _, name := range cfg.ExtraInputs {
		if _, _, err := edit.Read(root, name); err != nil {
			return "", fmt.Errorf("Deptrac extra input %q is missing or unsafe", name)
		}
	}
	return filepath.Join(directory, filepath.FromSlash(filename)), nil
}

func yamlFile(name string) bool {
	return strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml.dist") || strings.HasSuffix(name, ".yml.dist")
}

func validateNode(node *yaml.Node) error {
	if node.Anchor != "" || node.Kind == yaml.AliasNode || node.Kind != yaml.DocumentNode && !slices.Contains([]string{"!!map", "!!seq", "!!str", "!!bool", "!!int", "!!float", "!!null", "!!timestamp"}, node.ShortTag()) {
		return fmt.Errorf("unsupported YAML anchors, aliases or tags")
	}
	seen := map[string]bool{}
	for i, child := range node.Content {
		if node.Kind == yaml.MappingNode && i%2 == 0 {
			if child.Kind != yaml.ScalarNode || child.ShortTag() != "!!str" || seen[child.Value] {
				return fmt.Errorf("duplicate or non-string YAML key")
			}
			seen[child.Value] = true
		}
		if err := validateNode(child); err != nil {
			return err
		}
	}
	return nil
}

func validateTree(directory, name string) error {
	if err := projectpath.Validate(name); err != nil {
		return err
	}
	current := directory
	for _, part := range strings.Split(name, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("Deptrac graph directories must be readable project directories without symlinks")
		}
	}
	return filepath.WalkDir(current, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("Deptrac graph contains an unreadable directory")
		}
		if entry.Name() == ".git" && entry.IsDir() {
			return fs.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Deptrac graph paths must not traverse symlinks")
		}
		if !entry.IsDir() && !entry.Type().IsRegular() {
			return fmt.Errorf("Deptrac graph must contain regular files only")
		}
		return nil
	})
}

func validateCollectors(root *os.Root, value any) error {
	switch value := value.(type) {
	case map[string]any:
		for key, item := range value {
			if key == "composerPath" || key == "composerLockPath" {
				name, ok := item.(string)
				if !ok {
					return fmt.Errorf("Deptrac Composer collector paths must be project-relative files")
				}
				if _, _, err := edit.Read(root, name); err != nil {
					return fmt.Errorf("Deptrac Composer collector input is unreadable or unsafe")
				}
			}
			if err := validateCollectors(root, item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range value {
			if err := validateCollectors(root, item); err != nil {
				return err
			}
		}
	}
	return nil
}
