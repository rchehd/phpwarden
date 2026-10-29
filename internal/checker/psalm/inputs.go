package psalm

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/internal/psalmanalysis"
	"github.com/rchehd/phpwarden/internal/scope"
)

func ContextHash(root string, cfg config.Config) (string, error) {
	metadata, err := psalmanalysis.Snapshot(root)
	if err != nil {
		return "", err
	}
	files := map[string]bool{}
	vendor := "vendor"
	composer := path.Join(cfg.ComposerRoot, "composer.json")
	if data, err := psalmanalysis.Read(root, composer); err == nil {
		var manifest struct {
			Config struct {
				Vendor string `json:"vendor-dir"`
			} `json:"config"`
		}
		if json.Unmarshal(data, &manifest) == nil && manifest.Config.Vendor != "" {
			vendor = manifest.Config.Vendor
		}
	}
	if err := projectpath.Validate(vendor); err != nil {
		return "", err
	}
	for _, name := range []string{composer, path.Join(cfg.ComposerRoot, "composer.lock"), path.Join(cfg.ComposerRoot, vendor, "autoload.php"), path.Join(cfg.ComposerRoot, vendor, "composer/installed.json")} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name))); os.IsNotExist(err) {
			continue
		} else if err != nil {
			return "", err
		}
		files[name] = true
	}
	for _, directory := range cfg.Scope.Include {
		if err := projectpath.Validate(directory); err != nil {
			return "", err
		}
		if err := projectpath.Within(root, directory); err != nil {
			return "", err
		}
		base := filepath.Join(root, filepath.FromSlash(directory))
		err := filepath.WalkDir(base, func(filename string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if filename != base && slices.Contains([]string{".git", "vendor", "node_modules", "files", "tests"}, entry.Name()) {
					return fs.SkipDir
				}
				return nil
			}
			name, _ := filepath.Rel(root, filename)
			name = filepath.ToSlash(name)
			selected := (scope.Selection{Files: []string{name}}).Filter(nil, cfg.Scope.Exclude, func(name string) bool { return phpFile(name, cfg.Profile) || metadataFile(name) })
			if len(selected.Files) > 0 {
				files[name] = true
			}
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	digest := sha256.New()
	fmt.Fprintf(digest, "%s\x00", metadata)
	for _, name := range names {
		data, err := psalmanalysis.Read(root, name)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(digest, "%d:%s:%x\x00", len(name), name, sha256.Sum256(data))
	}
	return fmt.Sprintf("%x", digest.Sum(nil)), nil
}
