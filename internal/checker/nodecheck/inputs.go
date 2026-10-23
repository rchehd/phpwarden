package nodecheck

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/internal/scope"
)

var candidates = map[string][]string{
	"eslint":    {"eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", ".eslintrc.js", ".eslintrc.cjs", ".eslintrc.yaml", ".eslintrc.yml", ".eslintrc.json", ".eslintrc", "package.json"},
	"stylelint": {"stylelint.config.js", "stylelint.config.mjs", "stylelint.config.cjs", ".stylelintrc", ".stylelintrc.json", ".stylelintrc.yml", ".stylelintrc.yaml", ".stylelintrc.js", ".stylelintrc.cjs", "package.json"},
	"jscpd":     {".jscpd.json"},
}

func DiscoverConfig(root, composerRoot, id string) (string, error) {
	return findConfig(root, ".", composerRoot, id)
}

type group struct {
	Config string   `json:"config"`
	Files  []string `json:"files"`
}

func Settings(cfg config.Config, id string) config.NodeLint {
	switch id {
	case "eslint":
		return cfg.Tools.ESLint
	case "stylelint":
		return cfg.Tools.Stylelint
	default:
		d := cfg.Tools.JSCPD
		return config.NodeLint{Enabled: d.Enabled, Required: d.Required, FailOn: d.FailOn, TimeoutSeconds: d.TimeoutSeconds,
			Include: d.Include, Exclude: d.Exclude, Config: d.Config, DiscoverConfig: d.DiscoverConfig}
	}
}

func Select(selection scope.Selection, cfg config.NodeLint, id string) scope.Selection {
	extensions := []string{".js", ".mjs", ".cjs", ".jsx"}
	if id == "stylelint" {
		extensions = []string{".css", ".scss", ".sass", ".less"}
	} else if id == "jscpd" {
		extensions = []string{".php", ".module", ".inc", ".install", ".theme", ".profile", ".engine", ".test", ".js", ".mjs", ".cjs", ".jsx", ".ts", ".tsx", ".css", ".scss", ".sass", ".less", ".twig", ".html", ".yml", ".yaml"}
	}
	extensions = append(extensions, cfg.ExtraExtensions...)
	return selection.Filter(cfg.Include, cfg.Exclude, func(name string) bool {
		return slices.Contains(extensions, strings.ToLower(path.Ext(name)))
	})
}

func InputRequired(cfg config.Config, selection scope.Selection, only []string, id string) bool {
	settings := Settings(cfg, id)
	return checker.Enabled(id, settings.Enabled, only) && len(Select(selection, settings, id).Files) > 0
}

// Native JS configs can import any project file, including installed packages and shared configs.
func ContextHash(root string) string {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == ".git" {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		name, _ := filepath.Rel(root, current)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		target := ""
		if entry.Type()&os.ModeSymlink != 0 {
			if target, err = os.Readlink(current); err != nil {
				return err
			}
		}
		fmt.Fprintf(hash, "%q:%s:%d:%d:%q\x00", filepath.ToSlash(name), entry.Type(), info.Size(), info.ModTime().UnixNano(), target)
		return nil
	})
	if err != nil {
		fmt.Fprintf(hash, "error:%s", err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func regular(root, name string) error {
	if err := projectpath.Validate(name); err != nil {
		return err
	}
	if err := projectpath.Within(root, name); err != nil {
		return err
	}
	info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("Node checker input %q must be a regular file", name)
	}
	return nil
}

func findConfig(root, start, composerRoot, id string) (string, error) {
	directories := []string{}
	for directory := start; ; directory = path.Dir(directory) {
		directories = append(directories, directory)
		if directory == "." {
			break
		}
	}
	if !slices.Contains(directories, composerRoot) {
		directories = append(directories, composerRoot)
	}
	for _, directory := range directories {
		for _, candidate := range candidates[id] {
			name := path.Join(directory, candidate)
			if err := regular(root, name); os.IsNotExist(err) {
				continue
			} else if err != nil {
				return "", err
			}
			if candidate == "package.json" {
				data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
				if err != nil {
					return "", err
				}
				var pkg map[string]json.RawMessage
				if err := json.Unmarshal(data, &pkg); err != nil {
					return "", fmt.Errorf("invalid package.json %q: %w", name, err)
				}
				key := id
				if id == "eslint" {
					key = "eslintConfig"
				}
				if _, exists := pkg[key]; !exists {
					continue
				}
			}
			return name, nil
		}
	}
	return "", nil
}

func groups(root, composerRoot, id string, selection scope.Selection, cfg config.NodeLint) ([]group, error) {
	for _, name := range append(slices.Clone(cfg.ExtraInputs), cfg.Config) {
		if name != "" {
			if err := regular(root, name); err != nil {
				return nil, err
			}
		}
	}
	result := []group{}
	indices := map[string]int{}
	for _, name := range selection.Files {
		selected := cfg.Config
		if selected == "" && cfg.DiscoverConfig {
			var err error
			selected, err = findConfig(root, path.Dir(name), composerRoot, id)
			if err != nil {
				return nil, err
			}
		}
		index, exists := indices[selected]
		if !exists {
			index = len(result)
			indices[selected] = index
			result = append(result, group{Config: selected, Files: []string{}})
		}
		result[index].Files = append(result[index].Files, name)
	}
	return result, nil
}

func inventory(ctx context.Context, root string, cfg config.Config) (scope.Selection, error) {
	result := scope.Selection{Mode: "full", Files: []string{}, LineMap: map[string][]scope.Range{}}
	seen := map[string]bool{}
	for _, name := range cfg.Scope.Include {
		if err := projectpath.Validate(name); err != nil {
			return result, err
		}
		if err := projectpath.Within(root, name); err != nil {
			return result, err
		}
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(name)), func(current string, entry fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				return err
			}
			relative, _ := filepath.Rel(root, current)
			filename := filepath.ToSlash(relative)
			if entry.Type()&os.ModeSymlink != 0 {
				return nil
			}
			probe := scope.Selection{Files: []string{filename}, LineMap: map[string][]scope.Range{filename: nil}}
			if len(probe.Filter(nil, cfg.Scope.Exclude, func(string) bool { return true }).Files) == 0 {
				if entry.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if !entry.IsDir() && entry.Type().IsRegular() && !seen[filename] {
				seen[filename] = true
				result.Files = append(result.Files, filename)
				result.LineMap[filename] = nil
			}
			return nil
		})
		if err != nil {
			return result, err
		}
	}
	slices.Sort(result.Files)
	return result, nil
}
