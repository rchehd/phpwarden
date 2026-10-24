package twigcsfixer

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
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
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/internal/scope"
)

var ConfigCandidates = []string{".twig-cs-fixer.php", ".twig-cs-fixer.dist.php"}

func Select(selection scope.Selection, cfg config.TwigCSFixer, profileID string) scope.Selection {
	defaults, _ := profile.Lookup(profileID)
	extensions := append(slices.Clone(defaults.TwigExtensions), cfg.ExtraExtensions...)
	return selection.Filter(cfg.Include, cfg.Exclude, func(name string) bool { return slices.Contains(extensions, strings.ToLower(path.Ext(name))) })
}

func InputRequired(cfg config.Config, selection scope.Selection, only []string) bool {
	return checker.Enabled("twig-cs-fixer", cfg.Tools.TwigCSFixer.Enabled, only) && len(Select(selection, cfg.Tools.TwigCSFixer, cfg.Profile).Files) > 0
}

func ContextHash(root, composerRoot string, cfg config.TwigCSFixer) string {
	_, inputs, err := prepare(root, composerRoot, cfg)
	if cfg.Autoload != "" {
		inputs["project_context"] = phpstan.ContextHash(root)
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	data, _ := json.Marshal(struct {
		Inputs map[string]string
		Error  string
	}{inputs, message})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

// Returns the selected project config ("" for the profile fallback) and content hashes of every input it may read.
func prepare(root, composerRoot string, cfg config.TwigCSFixer) (string, map[string]string, error) {
	inputs := map[string]string{}
	record := func(name string) error {
		if err := projectpath.Validate(name); err != nil {
			return err
		}
		current := root
		for _, part := range strings.Split(path.Clean(name), "/") {
			current = filepath.Join(current, part)
			info, err := os.Lstat(current)
			if os.IsNotExist(err) {
				inputs[name] = "missing"
				return fmt.Errorf("Twig CS Fixer input %q is missing: %w", name, fs.ErrNotExist)
			}
			if err != nil || info.Mode()&os.ModeSymlink != 0 {
				inputs[name] = "unsupported"
				return fmt.Errorf("Twig CS Fixer input %q must be readable without symlinks", name)
			}
		}
		info, _ := os.Lstat(current)
		if info.IsDir() {
			listing := []string{}
			err := filepath.WalkDir(current, func(entryPath string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return fmt.Errorf("cannot inspect Twig CS Fixer input %q", name)
				}
				relative, _ := filepath.Rel(root, entryPath)
				relative = filepath.ToSlash(relative)
				if !entry.IsDir() && !entry.Type().IsRegular() {
					return fmt.Errorf("Twig CS Fixer input %q must contain only regular files", name)
				}
				listing = append(listing, relative)
				if entry.IsDir() {
					return nil
				}
				data, err := os.ReadFile(entryPath)
				if err != nil {
					return fmt.Errorf("cannot read Twig CS Fixer input %q", relative)
				}
				inputs[relative] = fmt.Sprintf("%x", sha256.Sum256(data))
				return nil
			})
			inputs[name+"/"] = fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(listing, "\x00"))))
			return err
		}
		if !info.Mode().IsRegular() {
			inputs[name] = "unsupported"
			return fmt.Errorf("Twig CS Fixer input %q must be a regular file", name)
		}
		data, err := os.ReadFile(current)
		if err != nil {
			inputs[name] = "unreadable"
			return fmt.Errorf("cannot read Twig CS Fixer input %q", name)
		}
		inputs[name] = fmt.Sprintf("%x", sha256.Sum256(data))
		return nil
	}
	selected := ""
	if cfg.Config != "" {
		if err := record(cfg.Config); err != nil {
			return "", inputs, err
		}
		selected = cfg.Config
	} else if cfg.DiscoverConfig {
		candidates := slices.Clone(ConfigCandidates)
		if composerRoot != "." {
			for _, name := range ConfigCandidates {
				candidates = append(candidates, path.Join(composerRoot, name))
			}
		}
		for _, name := range candidates {
			err := record(name)
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			if err != nil {
				return "", inputs, err
			}
			if selected == "" {
				selected = name
			}
		}
	}
	if cfg.Autoload != "" {
		loader, err := projectAutoload(root, composerRoot, cfg.Autoload)
		if err != nil {
			return "", inputs, err
		}
		name := path.Join(composerRoot, "composer.json")
		if err := record(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return "", inputs, err
		}
		if loader != "" {
			if err := record(loader); err != nil {
				return "", inputs, err
			}
		}
	}
	for _, name := range cfg.ExtraInputs {
		if err := record(name); err != nil {
			return "", inputs, err
		}
	}
	return selected, inputs, nil
}

func projectAutoload(root, composerRoot, setting string) (string, error) {
	if setting == "" {
		return "", nil
	}
	name := setting
	if setting == "auto" {
		vendor := "vendor"
		metadata := path.Join(composerRoot, "composer.json")
		if err := projectpath.Within(root, metadata); err != nil {
			return "", err
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(metadata)))
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		if err == nil {
			var doc struct {
				Config struct {
					Vendor string `json:"vendor-dir"`
				} `json:"config"`
			}
			if err = json.Unmarshal(data, &doc); err != nil {
				return "", err
			}
			if doc.Config.Vendor != "" {
				vendor = doc.Config.Vendor
			}
		}
		if err := projectpath.Validate(vendor); err != nil {
			return "", err
		}
		name = path.Join(composerRoot, vendor, "autoload.php")
	}
	if err := projectpath.Validate(name); err != nil {
		return "", err
	}
	if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name))); setting == "auto" && os.IsNotExist(err) {
		return "", nil
	}
	return name, nil
}
