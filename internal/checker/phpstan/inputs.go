package phpstan

import (
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
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/internal/scope"
)

var ConfigCandidates = []string{"phpstan.neon", "phpstan.neon.dist", "phpstan.dist.neon"}

func Select(selection scope.Selection, cfg config.PHPStan, profileID string) scope.Selection {
	defaults, _ := profile.Lookup(profileID)
	extensions := append(slices.Clone(defaults.PHPExtensions), cfg.ExtraExtensions...)
	return selection.Filter(cfg.Include, cfg.Exclude, func(name string) bool { return slices.Contains(extensions, strings.ToLower(path.Ext(name))) })
}

func InputRequired(cfg config.Config, selection scope.Selection, only []string) bool {
	return checker.Enabled("phpstan", cfg.Tools.PHPStan.Enabled, only) && len(Select(selection, cfg.Tools.PHPStan, cfg.Profile).Files) > 0
}

func ContextHash(root string) string {
	digest, err := contextDigest(root)
	if err != nil {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(digest+err.Error())))
	}
	return digest
}

// Hashes entry metadata, not content: PHPStan may read any project file, so every change must invalidate the run.
func contextDigest(root string) (string, error) {
	hash := sha256.New()
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return fmt.Errorf("cannot inspect PHPStan context %q", current)
		}
		if current == root {
			return nil
		}
		if entry.Name() == ".git" {
			if entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		name, _ := filepath.Rel(root, current)
		name = filepath.ToSlash(name)
		fmt.Fprintf(hash, "%d:%s:%s", len(name), name, entry.Type())
		if entry.IsDir() {
			hash.Write([]byte{0})
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("cannot inspect PHPStan context %q", name)
		}
		target := ""
		if entry.Type()&fs.ModeSymlink != 0 {
			if target, err = os.Readlink(current); err != nil {
				return fmt.Errorf("cannot inspect PHPStan context %q", name)
			}
		}
		fmt.Fprintf(hash, ":%d:%d:%d:%s\x00", info.Size(), info.ModTime().UnixNano(), len(target), target)
		return nil
	})
	return fmt.Sprintf("%x", hash.Sum(nil)), err
}

func prepare(root string, cfg config.PHPStan) (string, string, error) {
	regular := func(name string) error {
		if err := projectpath.Validate(name); err != nil {
			return err
		}
		if err := projectpath.Within(root, name); err != nil {
			return err
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("PHPStan input %q must be an existing regular file", name)
		}
		return nil
	}
	selected := cfg.Config
	if selected == "" && cfg.DiscoverConfig {
		for _, candidate := range ConfigCandidates {
			if _, err := os.Lstat(filepath.Join(root, candidate)); os.IsNotExist(err) {
				continue
			}
			selected = candidate
			break
		}
	}
	for _, name := range append(append(slices.Clone(cfg.Includes), cfg.BootstrapFiles...), cfg.StubFiles...) {
		if err := regular(name); err != nil {
			return "", "", err
		}
	}
	for _, name := range []string{selected, cfg.Baseline} {
		if name != "" {
			if err := regular(name); err != nil {
				return "", "", err
			}
		}
	}
	for _, name := range cfg.ScanDirectories {
		if err := projectpath.Within(root, name); err != nil {
			return "", "", err
		}
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil || !info.IsDir() {
			return "", "", fmt.Errorf("PHPStan scan directory %q is unavailable", name)
		}
	}
	autoload := cfg.Autoload
	if autoload == "auto" {
		vendor := "vendor"
		if data, err := os.ReadFile(filepath.Join(root, "composer.json")); err == nil {
			var composer struct {
				Config struct {
					VendorDir string `json:"vendor-dir"`
				} `json:"config"`
			}
			if err := json.Unmarshal(data, &composer); err != nil {
				return "", "", fmt.Errorf("PHPStan cannot read composer.json: %w", err)
			}
			if composer.Config.VendorDir != "" {
				vendor = composer.Config.VendorDir
			}
		}
		autoload = path.Join(vendor, "autoload.php")
		if err := projectpath.Validate(autoload); err != nil {
			return "", "", err
		}
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(autoload))); os.IsNotExist(err) {
			autoload = ""
		}
	}
	if autoload != "" {
		if err := regular(autoload); err != nil {
			return "", "", err
		}
	}
	return selected, autoload, nil
}
