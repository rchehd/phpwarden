package packagecheck

import (
	"encoding/json"
	"fmt"
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
)

const UnusedVersion = "0.9.7"

func RequireVersion(runtime string) string {
	if runtime == "8.1" {
		return "4.7.1"
	}
	return "4.16.1"
}

func Settings(cfg config.Config, id string) config.PackageTool {
	if id == "composer-require-checker" {
		return cfg.Tools.ComposerRequireChecker.PackageTool
	}
	return cfg.Tools.ComposerUnused.PackageTool
}

func Root(cfg config.Config, id string) string {
	if value := Settings(cfg, id).ComposerRoot; value != "" {
		return path.Clean(value)
	}
	return cfg.ComposerRoot
}

func Candidates(id string) []string {
	if id == "composer-unused" {
		return []string{"composer-unused.php"}
	}
	return []string{"composer-require-checker.json"}
}

func InputRequired(cfg config.Config, selected scope.Selection, only []string, id string) bool {
	return checker.Enabled(id, Settings(cfg, id).Enabled, only) && selected.Mode == "full"
}

func ContextHash(root string) string { return phpstan.ContextHash(root) }

type plan struct {
	Vendor             string   `json:"vendor"`
	ID                 string   `json:"id"`
	Root               string   `json:"root"`
	Composer           string   `json:"composer"`
	Config             string   `json:"config"`
	Version            string   `json:"tool_version"`
	ScanFiles          []string `json:"scan_files"`
	Allowed            []string `json:"allowed_symbols"`
	ExcludePackages    []string `json:"exclude_packages"`
	ExcludeDirectories []string `json:"exclude_directories"`
	Requires           []string `json:"requires"`
}

func prepare(directory string, cfg config.Config, id string) (plan, string, error) {
	settings := Settings(cfg, id)
	base := Root(cfg, id)
	result := plan{ID: id, Root: filepath.Join(directory, filepath.FromSlash(base)), Version: UnusedVersion,
		ScanFiles: slices.Clone(cfg.Tools.ComposerRequireChecker.ScanFiles), Allowed: slices.Clone(cfg.Tools.ComposerRequireChecker.AllowedSymbols), ExcludePackages: slices.Clone(cfg.Tools.ComposerUnused.ExcludePackages), ExcludeDirectories: []string{}, Requires: []string{}}
	if id == "composer-require-checker" {
		result.Version = RequireVersion(cfg.PHP.Runtime)
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return result, "input_read", err
	}
	defer root.Close()
	regular := func(name string) ([]byte, error) { data, _, err := edit.Read(root, name); return data, err }
	data, err := regular(path.Join(base, "composer.json"))
	if err != nil {
		return result, "input_read", fmt.Errorf("Dependency checks require a readable composer.json inside composer_root")
	}
	result.Composer = filepath.Join(result.Root, "composer.json")
	var composer struct {
		Name     string                     `json:"name"`
		Require  map[string]string          `json:"require"`
		Autoload map[string]json.RawMessage `json:"autoload"`
		Config   struct {
			Vendor string `json:"vendor-dir"`
		} `json:"config"`
	}
	if json.Unmarshal(data, &composer) != nil {
		return result, "config_invalid", fmt.Errorf("Cannot parse composer.json for dependency analysis")
	}
	if err := validateAutoload(composer.Autoload); err != nil {
		return result, "config_invalid", err
	}
	if composer.Config.Vendor == "" {
		composer.Config.Vendor = "vendor"
	}
	if projectpath.Validate(composer.Config.Vendor) != nil {
		return result, "config_invalid", fmt.Errorf("Dependency checker vendor-dir must be project-relative")
	}

	vendor := path.Join(base, composer.Config.Vendor)
	result.Vendor = filepath.Join(directory, filepath.FromSlash(vendor))
	data, err = regular(path.Join(vendor, "composer/installed.json"))
	if err != nil {
		return result, "dependencies_missing", fmt.Errorf("Dependency checks require installed Composer 2 metadata and dependencies")
	}
	var installed struct {
		Packages []struct {
			Name     string                     `json:"name"`
			Install  string                     `json:"install-path"`
			Replace  map[string]string          `json:"replace"`
			Provide  map[string]string          `json:"provide"`
			Autoload map[string]json.RawMessage `json:"autoload"`
		} `json:"packages"`
	}
	if json.Unmarshal(data, &installed) != nil || installed.Packages == nil {
		return result, "dependencies_invalid", fmt.Errorf("Dependency checks require Composer 2 installed.json metadata")
	}
	for name := range composer.Require {
		result.Requires = append(result.Requires, name)
		if name == "php" || name == "php-64bit" || strings.HasPrefix(name, "ext-") || name == "composer-runtime-api" || name == "composer-plugin-api" {
			continue
		}
		found := false
		for _, pkg := range installed.Packages {
			if pkg.Name != name {
				if _, ok := pkg.Replace[name]; ok {
					return result, "unsupported_layout", nil
				}
				if _, ok := pkg.Provide[name]; ok {
					return result, "unsupported_layout", nil
				}
				continue
			}
			found = true
			if err := validateAutoload(pkg.Autoload); err != nil {
				return result, "config_invalid", err
			}
			expected := path.Join(vendor, name)
			if pkg.Install != "" && path.Clean(path.Join(vendor, "composer", pkg.Install)) != expected {
				return result, "unsupported_layout", nil
			}
			info, err := root.Lstat(expected)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return result, "unsupported_layout", nil
			}
			packageData, err := regular(path.Join(expected, "composer.json"))
			var packageJSON struct {
				Name     string                     `json:"name"`
				Autoload map[string]json.RawMessage `json:"autoload"`
			}
			if err != nil || json.Unmarshal(packageData, &packageJSON) != nil || packageJSON.Name != name {
				return result, "dependencies_invalid", fmt.Errorf("Installed dependency Composer metadata is missing or invalid")
			}
			if err := validateAutoload(packageJSON.Autoload); err != nil {
				return result, "config_invalid", err
			}

		}
		if !found {
			return result, "dependencies_missing", fmt.Errorf("A declared direct dependency is missing from installed Composer metadata")
		}
	}
	slices.Sort(result.Requires)
	if id == "composer-unused" {
		if _, err := regular(path.Join(vendor, "composer/installed.php")); err != nil {
			return result, "dependencies_missing", fmt.Errorf("composer-unused requires Composer 2 installed.php metadata")
		}
	}
	configFile := settings.Config
	if configFile == "" && settings.DiscoverConfig {
		for _, name := range Candidates(id) {
			for _, search := range []string{"", base} {
				candidate := path.Join(search, name)
				if _, err := regular(candidate); os.IsNotExist(err) {
					continue
				} else if err != nil {
					return result, "config_invalid", fmt.Errorf("Native dependency checker config is unreadable or unsafe")
				}
				configFile = candidate
				break
			}
			if configFile != "" {
				break
			}
		}
	}
	for _, name := range append(slices.Clone(settings.ExtraInputs), configFile) {
		if name == "" {
			continue
		}
		if _, err := regular(name); err != nil {
			return result, "config_invalid", fmt.Errorf("Dependency checker config or extra input %q is unreadable or unsafe", name)
		}
	}
	if configFile != "" {
		result.Config = filepath.Join(directory, filepath.FromSlash(configFile))
		if id == "composer-require-checker" {
			data, _ := regular(configFile)
			var options struct {
				Scan       []string `json:"scan-files"`
				Allowed    []string `json:"symbol-whitelist"`
				Extensions []string `json:"php-core-extensions"`
			}
			if strictJSON(data, &options) != nil {
				return result, "config_invalid", fmt.Errorf("ComposerRequireChecker config must contain native JSON scan-files, symbol-whitelist and php-core-extensions options")
			}
			for _, name := range options.Scan {
				if projectpath.Validate(name) != nil {
					return result, "config_invalid", fmt.Errorf("ComposerRequireChecker scan-files must stay inside composer_root")
				}
			}
		}
	}
	if len(composer.Autoload) == 0 && len(result.ScanFiles) == 0 && result.Config == "" {
		return result, "no_sources", nil
	}
	for _, name := range cfg.Tools.ComposerUnused.ExcludeDirectories {
		result.ExcludeDirectories = append(result.ExcludeDirectories, filepath.Join(result.Root, filepath.FromSlash(name)))
	}
	err = filepath.WalkDir(result.Root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("Dependency checker context contains unreadable files")
		}
		if entry.Name() == ".git" && entry.IsDir() {
			return fs.SkipDir
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("Dependency checker context must not traverse symlinks")
		}
		if !entry.IsDir() {
			if !entry.Type().IsRegular() {
				return fmt.Errorf("Dependency checker context must contain regular files")
			}
			f, err := os.Open(filename)
			if err != nil {
				return fmt.Errorf("Dependency checker context contains unreadable files")
			}
			f.Close()
		}
		return nil
	})
	if err != nil {
		return result, "unsupported_input", err
	}
	return result, "", nil
}

func validateAutoload(values map[string]json.RawMessage) error {
	var visit func(any) error
	visit = func(value any) error {
		switch value := value.(type) {
		case string:
			if value != "" && projectpath.Validate(value) != nil {
				return fmt.Errorf("Dependency checker autoload sources must stay inside their package")
			}
		case []any:
			for _, item := range value {
				if err := visit(item); err != nil {
					return err
				}
			}
		case map[string]any:
			for _, item := range value {
				if err := visit(item); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("Invalid Composer autoload source path")
		}
		return nil
	}
	for _, key := range []string{"psr-0", "psr-4", "classmap", "files"} {
		if data, ok := values[key]; ok {
			var value any
			if json.Unmarshal(data, &value) != nil {
				return fmt.Errorf("Invalid Composer autoload data")
			}
			if err := visit(value); err != nil {
				return err
			}
		}
	}
	return nil
}
