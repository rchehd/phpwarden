package psalmanalysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/projectpath"
)

const Version = "6.20.3"

type Pin struct {
	Version   string
	Reference string
}

type installedPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  struct {
		Reference string `json:"reference"`
	} `json:"source"`
}

func readPackages(filename string) ([]installedPackage, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var document struct {
		Packages []installedPackage `json:"packages"`
	}
	if err := json.Unmarshal(data, &document); err != nil || document.Packages == nil {
		return nil, fmt.Errorf("invalid dependency metadata %s", filename)
	}
	return document.Packages, nil
}

func Dependencies(root, composerRoot, bundle string, pins map[string]Pin) error {
	packages, err := readPackages(filepath.Join(bundle, "composer/installed.json"))
	if err != nil {
		return fmt.Errorf("isolated Psalm dependencies are unavailable: %w", err)
	}
	want := maps.Clone(pins)
	if want == nil {
		want = map[string]Pin{}
	}
	want["vimeo/psalm"] = Pin{Version: Version}
	seen := map[string]bool{}
	for _, item := range packages {
		if seen[item.Name] || item.Name == "" || item.Version == "" {
			return fmt.Errorf("invalid or duplicate tool package")
		}
		seen[item.Name] = true
		if pin, ok := want[item.Name]; ok && (pin.Version != "" && strings.TrimPrefix(item.Version, "v") != pin.Version || pin.Reference != "" && item.Source.Reference != pin.Reference) {
			return fmt.Errorf("unexpected %s version or revision", item.Name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(want)) {
		if !seen[name] {
			return fmt.Errorf("missing tool package %s", name)
		}
	}
	for _, name := range []string{"autoload.php", "bin/psalm"} {
		if info, err := os.Stat(filepath.Join(bundle, name)); err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("missing isolated Psalm entrypoint %s", name)
		}
	}
	composerFile := path.Join(composerRoot, "composer.json")
	data, err := Read(root, composerFile)
	if err != nil {
		return err
	}
	var composer struct {
		Config struct {
			Vendor string `json:"vendor-dir"`
		} `json:"config"`
	}
	if json.Unmarshal(data, &composer) != nil {
		return fmt.Errorf("invalid composer.json")
	}
	vendor := composer.Config.Vendor
	if vendor == "" {
		vendor = "vendor"
	}
	if err := projectpath.Validate(vendor); err != nil || path.Clean(vendor) == "." {
		return fmt.Errorf("unsupported project vendor directory")
	}
	if _, err := Read(root, path.Join(composerRoot, vendor, "autoload.php")); err != nil {
		return fmt.Errorf("project Composer autoloader is unavailable: %w", err)
	}
	lockedData, err := Read(root, path.Join(composerRoot, "composer.lock"))
	if err != nil {
		return err
	}
	var locked struct {
		Packages []installedPackage `json:"packages"`
	}
	if json.Unmarshal(lockedData, &locked) != nil || locked.Packages == nil {
		return fmt.Errorf("invalid composer.lock")
	}
	installed, err := readPackages(filepath.Join(root, filepath.FromSlash(path.Join(composerRoot, vendor, "composer/installed.json"))))
	if err != nil {
		return fmt.Errorf("project installed dependency metadata is unavailable: %w", err)
	}
	actual := map[string]installedPackage{}
	for _, item := range installed {
		if _, ok := actual[item.Name]; ok || item.Name == "" || item.Version == "" {
			return fmt.Errorf("invalid installed project package")
		}
		actual[item.Name] = item
	}
	for _, item := range locked.Packages {
		found, ok := actual[item.Name]
		if !ok || strings.TrimPrefix(item.Version, "v") != strings.TrimPrefix(found.Version, "v") || item.Source.Reference != "" && item.Source.Reference != found.Source.Reference {
			return fmt.Errorf("installed dependency %s differs from composer.lock", item.Name)
		}
	}
	return nil
}

func Read(root, name string) ([]byte, error) {
	if err := projectpath.Validate(name); err != nil {
		return nil, err
	}
	if err := projectpath.Within(root, name); err != nil {
		return nil, err
	}
	filename := filepath.Join(root, filepath.FromSlash(name))
	if info, err := os.Lstat(filename); err != nil || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("input %s must be an existing regular file", name)
	}
	return os.ReadFile(filename)
}

func HashesUnchanged(root string, inputs map[string]string) error {
	if len(inputs) == 0 {
		return fmt.Errorf("preparation reported no consumed inputs")
	}
	seen := map[string]bool{}
	for name, expected := range inputs {
		canonical := path.Clean(name)
		if seen[canonical] {
			return fmt.Errorf("duplicate consumed input %s", name)
		}
		seen[canonical] = true
		decoded, err := hex.DecodeString(expected)
		if err != nil || len(decoded) != sha256.Size || hex.EncodeToString(decoded) != expected {
			return fmt.Errorf("invalid input hash %s", name)
		}
		data, err := Read(root, name)
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != expected {
			return fmt.Errorf("consumed input %s changed", name)
		}
	}
	return nil
}

func Snapshot(root string) (string, error) {
	digest := sha256.New()
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if current == root {
			return nil
		}
		if entry.Name() == ".git" && entry.IsDir() {
			return fs.SkipDir
		}
		name, _ := filepath.Rel(root, current)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		target := ""
		if entry.Type()&fs.ModeSymlink != 0 {
			target, err = os.Readlink(current)
			if err != nil {
				return err
			}
		}
		fmt.Fprintf(digest, "%d:%s:%d:%d:%d:%d:%s\x00", len(name), name, info.Mode(), info.Size(), info.ModTime().UnixNano(), len(target), target)
		return nil
	})
	return fmt.Sprintf("%x", digest.Sum(nil)), err
}
