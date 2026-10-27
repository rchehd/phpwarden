package drupal

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/projectupdates"
	"go.yaml.in/yaml/v3"
)

var updateProjectID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,127}$`)
var packagedLegacyVersion = regexp.MustCompile(`^8\.x-([0-9]+)\.([0-9]+)(.*)$`)
var coreVersionDeclaration = regexp.MustCompile(`\bconst VERSION\s*=\s*['"]([^'"]+)['"]`)

type releaseLockPackage struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Type    string `json:"type"`
}

func releaseIdentity(version string) string {
	version = strings.TrimPrefix(version, "v")
	if parts := packagedLegacyVersion.FindStringSubmatch(version); parts != nil {
		return parts[1] + "." + parts[2] + ".0" + parts[3]
	}
	return version
}

func updateInventory(directory, composerRoot string, facts *profile.Facts) ([]projectupdates.Project, string, []string, error) {
	root, err := os.OpenRoot(directory)
	if err != nil {
		return nil, "", nil, err
	}
	defer root.Close()
	inputs := map[string]string{}
	read := func(name string) ([]byte, error) {
		info, err := root.Lstat(name)
		if os.IsNotExist(err) {
			inputs[name] = "missing"
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if !info.Mode().IsRegular() || info.Size() > 16<<20 {
			return nil, fmt.Errorf("update metadata %s must be a regular file at most 16 MiB", name)
		}
		data, err := root.ReadFile(name)
		inputs[name] = fmt.Sprintf("%x", sha256.Sum256(data))
		return data, err
	}
	projects := map[string]projectupdates.Project{}
	add := func(id, version, origin, filename string, direct, dev bool) error {
		if !updateProjectID.MatchString(id) {
			return fmt.Errorf("invalid Drupal.org project identifier in %s", filename)
		}
		version = strings.TrimPrefix(version, "v")
		key := id + "/" + releaseIdentity(version)
		item, exists := projects[key]
		if !exists {
			item = projectupdates.Project{Ecosystem: "drupal", ID: id, Current: version, Origin: origin, Paths: []string{}, Status: "unknown", Reasons: []string{}}
			item.Dev = dev
		}
		item.Direct = item.Direct || direct
		if !slices.Contains(item.Paths, filename) {
			item.Paths = append(item.Paths, filename)
		}
		projects[key] = item
		if len(projects) > 512 {
			return fmt.Errorf("Drupal release inventory exceeds 512 projects")
		}
		return nil
	}
	manifest, err := read(path.Join(composerRoot, "composer.json"))
	if err != nil {
		return nil, "", nil, err
	}
	var requirements struct {
		Require map[string]string `json:"require"`
		Dev     map[string]string `json:"require-dev"`
	}
	if manifest != nil && json.Unmarshal(manifest, &requirements) != nil {
		return nil, "", nil, fmt.Errorf("invalid Composer manifest for Drupal release inventory")
	}
	lockName := path.Join(composerRoot, "composer.lock")
	lock, err := read(lockName)
	if err != nil {
		return nil, "", nil, err
	}
	var locked struct {
		Packages []releaseLockPackage `json:"packages"`
		Dev      []releaseLockPackage `json:"packages-dev"`
	}
	if lock != nil && json.Unmarshal(lock, &locked) != nil {
		return nil, "", nil, fmt.Errorf("invalid Composer lock for Drupal release inventory")
	}
	for index, group := range [][]releaseLockPackage{locked.Packages, locked.Dev} {
		for _, item := range group {
			id := ""
			if item.Name == "drupal/core" {
				id = "drupal"
			} else if strings.HasPrefix(item.Name, "drupal/") && slices.Contains([]string{"drupal-module", "drupal-theme", "drupal-profile"}, item.Type) {
				id = strings.TrimPrefix(item.Name, "drupal/")
			}
			if id == "" {
				continue
			}
			_, direct := requirements.Require[item.Name]
			_, devDirect := requirements.Dev[item.Name]
			if err := add(id, item.Version, "composer.lock", lockName, direct || devDirect, index == 1); err != nil {
				return nil, "", nil, err
			}
		}
	}
	web := ""
	if facts != nil && facts.WebRoot != nil {
		web = *facts.WebRoot
	} else {
		for _, candidate := range []string{composerRoot, path.Join(composerRoot, "web"), path.Join(composerRoot, "docroot")} {
			for _, marker := range []string{"core/lib/Drupal.php", "modules", "themes"} {
				if _, err := root.Lstat(path.Join(candidate, marker)); err == nil {
					if web != "" && web != candidate {
						return nil, "", nil, fmt.Errorf("ambiguous Drupal web roots for release inventory")
					}
					web = candidate
				}
			}
		}
	}
	custom := 0
	if web != "" {
		name := path.Join(web, "core/lib/Drupal.php")
		data, err := read(name)
		if err != nil {
			return nil, "", nil, err
		}
		if data != nil {
			match := coreVersionDeclaration.FindSubmatch(data)
			version := ""
			if match != nil {
				version = string(match[1])
			}
			if err := add("drupal", version, "core/lib/Drupal.php", name, true, false); err != nil {
				return nil, "", nil, err
			}
		}
		visited := 0
		for _, folder := range []string{"modules", "themes", "profiles", "sites"} {
			start := path.Join(web, folder)
			err := fs.WalkDir(root.FS(), start, func(name string, entry fs.DirEntry, walkErr error) error {
				if os.IsNotExist(walkErr) && name == start {
					inputs[start] = "missing"
					return nil
				}
				if walkErr != nil {
					return walkErr
				}
				visited++
				if visited > 100000 {
					return fmt.Errorf("Drupal release inventory exceeds 100000 filesystem entries")
				}
				if entry.IsDir() && slices.Contains([]string{".git", "node_modules", "vendor", "files", "private", "tests", "fixtures"}, entry.Name()) {
					return fs.SkipDir
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("Drupal release inventory cannot assess symlinked metadata directories/files: %s", name)
				}
				if entry.IsDir() || !strings.HasSuffix(name, ".info.yml") {
					return nil
				}
				data, err := read(name)
				if err != nil {
					return err
				}
				var info struct {
					Project string `yaml:"project"`
					Version string `yaml:"version"`
				}
				if len(data) > 1<<20 || yaml.Unmarshal(data, &info) != nil {
					return fmt.Errorf("invalid Drupal project metadata %s", name)
				}
				if info.Project == "" {
					custom++
					return nil
				}
				return add(info.Project, info.Version, "packaged .info.yml", name, false, false)
			})
			if err != nil {
				return nil, "", nil, err
			}
		}
	}
	rows := make([]projectupdates.Project, 0, len(projects))
	for _, item := range projects {
		slices.Sort(item.Paths)
		rows = append(rows, item)
	}
	slices.SortFunc(rows, func(a, b projectupdates.Project) int { return strings.Compare(a.ID+"/"+a.Current, b.ID+"/"+b.Current) })
	serialized, _ := json.Marshal(inputs)
	notes := []string{"Drupal release status covers locked packages and packaged code in modules/themes/profiles/sites beneath the detected web root; enabled modules and deployed runtime state are not established. External extension locations and custom code without Drupal.org project metadata are outside this assessment. Missing exact releases are unknown, never inferred revoked."}
	if custom > 0 {
		notes = append(notes, fmt.Sprintf("%d .info.yml files have no Drupal.org project identifier; no release status was invented for them.", custom))
	}
	return rows, fmt.Sprintf("%x", sha256.Sum256(serialized)), notes, nil
}
