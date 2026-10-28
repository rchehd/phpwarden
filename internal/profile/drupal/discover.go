package drupal

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/rchehd/phpwarden/internal/inputsource"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/projectpath"
)

var exactVersion = regexp.MustCompile(`^v?([0-9]+\.[0-9]+\.[0-9]+)$`)

func Discover(root string) (profile.Facts, error) {
	return DiscoverIn(root, ".")
}

// DiscoverIn reads the Composer project in composerRoot and reports every path relative to root.
func DiscoverIn(root, composerRoot string) (profile.Facts, error) {
	facts, err := discover(filepath.Join(root, filepath.FromSlash(composerRoot)))
	if err != nil {
		var source *inputsource.Error
		if composerRoot != "." && errors.As(err, &source) {
			err = inputsource.WithFile(err, path.Join(composerRoot, source.File))
		}
		return facts, err
	}
	if composerRoot != "." {
		relocate(&facts, composerRoot)
	}
	return facts, pathRepositories(root, composerRoot, &facts)
}

func discover(root string) (profile.Facts, error) {
	facts := profile.Facts{SourceRoots: []string{}, Exclusions: []string{}, Inputs: map[string]string{}, Warnings: []string{}}
	reader := reader{root: root, inputs: facts.Inputs, warnings: &facts.Warnings}
	composer, err := reader.object("composer.json")
	if err != nil {
		return facts, err
	}
	lock, err := reader.object("composer.lock")
	if err != nil {
		return facts, err
	}
	if err := version(lock, &facts); err != nil {
		return facts, err
	}
	if err := compatibility(reader, composer, lock, &facts); err != nil {
		return facts, err
	}
	scaffold, err := objectAt(composer, "extra", "drupal-scaffold", "locations")
	if err != nil {
		return facts, err
	}
	if value, exists := scaffold["web-root"]; exists {
		name, ok := value.(string)
		if !ok || name == "" {
			return facts, fmt.Errorf("Composer scaffold web-root must be a nonempty relative path")
		}
		if err := safePath(name); err != nil {
			return facts, err
		}
		name, source := path.Clean(name), "composer.json:extra.drupal-scaffold.locations.web-root"
		facts.WebRoot, facts.RootSource = &name, &source
	}
	installers, err := installerPaths(composer)
	if err != nil {
		return facts, err
	}
	for name, types := range installers {
		if !slices.Contains(types, "type:drupal-core") {
			continue
		}
		if err := safePath(name); err != nil || path.Base(path.Clean(name)) != "core" {
			return facts, fmt.Errorf("unsupported Drupal core installer path %q", name)
		}
		webRoot := path.Dir(path.Clean(name))
		if facts.WebRoot != nil && *facts.WebRoot != webRoot {
			return facts, fmt.Errorf("conflicting Drupal web roots in Composer scaffold/installer paths")
		}
		if facts.WebRoot == nil {
			source := "composer.json:extra.installer-paths"
			facts.WebRoot, facts.RootSource = &webRoot, &source
		}
	}
	if facts.WebRoot == nil {
		candidates := []string{}
		for _, candidate := range []string{".", "web", "docroot"} {
			found := false
			for _, marker := range []string{"core/lib/Drupal.php", "modules/custom", "themes/custom", "profiles/custom", "sites/all/modules/custom"} {
				name := path.Join(candidate, marker)
				info, err := reader.probe(name)
				if err != nil {
					return facts, err
				}
				if info != nil && (marker == "core/lib/Drupal.php" && info.Mode().IsRegular() || marker != "core/lib/Drupal.php" && info.IsDir()) {
					found = true
				}
			}
			if found {
				candidates = append(candidates, candidate)
			}
		}
		if len(candidates) > 1 {
			return facts, fmt.Errorf("ambiguous Drupal web root (%s); declare Composer scaffold web-root", strings.Join(candidates, ", "))
		}
		if len(candidates) == 1 {
			source := "drupal_directory_conventions"
			facts.WebRoot, facts.RootSource = &candidates[0], &source
		}
	}
	if facts.WebRoot != nil {
		if _, err := reader.directory(*facts.WebRoot); err != nil {
			return facts, err
		}
		if err := layout(reader, *facts.WebRoot, &facts); err != nil {
			return facts, err
		}
	}
	for name, types := range installers {
		custom, contributed := false, false
		for _, kind := range []string{"module", "theme", "profile"} {
			custom = custom || slices.Contains(types, "type:drupal-custom-"+kind)
			contributed = contributed || slices.Contains(types, "type:drupal-"+kind)
		}
		if !custom && !contributed {
			continue
		}
		if !strings.HasSuffix(name, "/{$name}") {
			return facts, fmt.Errorf("unsupported Drupal installer template %q; expected a literal directory followed by /{$name}", name)
		}
		parent := strings.TrimSuffix(name, "/{$name}")
		if err := safePath(parent); err != nil {
			return facts, err
		}
		if contributed {
			if path.Base(path.Clean(parent)) == "contrib" {
				addExclusion(&facts, path.Clean(parent))
			} else if err := excludeLockedPackages(lock, parent, types, &facts); err != nil {
				return facts, err
			}
		}
		if custom {
			if err := addSource(reader, parent, &facts); err != nil {
				return facts, err
			}
		}
	}
	composerConfig, err := objectAt(composer, "config")
	if err != nil {
		return facts, err
	}
	if value, exists := composerConfig["vendor-dir"]; exists {
		name, ok := value.(string)
		if !ok || safePath(name) != nil || path.Clean(name) == "." {
			return facts, fmt.Errorf("Composer vendor-dir must be a project-relative subdirectory")
		}
		addExclusion(&facts, path.Clean(name))
	}
	sort.Strings(facts.SourceRoots)
	facts.SourceRoots = slices.Compact(facts.SourceRoots)
	sort.Strings(facts.Exclusions)
	facts.Exclusions = slices.Compact(facts.Exclusions)
	return facts, nil
}

func excludeLockedPackages(lock map[string]any, parent string, types []string, facts *profile.Facts) error {
	for _, section := range []string{"packages", "packages-dev"} {
		packages, _ := lock[section].([]any)
		for _, value := range packages {
			entry := value.(map[string]any)
			kind, _ := entry["type"].(string)
			if !slices.Contains(types, "type:"+kind) {
				continue
			}
			name, ok := entry["name"].(string)
			if !ok || strings.Count(name, "/") != 1 {
				return fmt.Errorf("locked Drupal package requires vendor/name")
			}
			extra, err := objectAt(entry, "extra")
			if err != nil {
				return err
			}
			name = path.Base(name)
			if value, exists := extra["installer-name"]; exists {
				name, ok = value.(string)
				if !ok {
					return fmt.Errorf("package installer-name must be a string")
				}
			}
			if err := safePath(name); err != nil || name == "." || strings.Contains(name, "/") {
				return fmt.Errorf("invalid Drupal package installer name %q", name)
			}
			addExclusion(facts, path.Join(parent, name))
		}
	}
	return nil
}

func version(lock map[string]any, facts *profile.Facts) error {
	var locked string
	coreWithoutExactVersion := false
	seen := map[string]bool{}
	for _, section := range []string{"packages", "packages-dev"} {
		value, exists := lock[section]
		if !exists {
			continue
		}
		packages, ok := value.([]any)
		if !ok {
			return fmt.Errorf("composer.lock %s must be an array", section)
		}
		for _, value := range packages {
			entry, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("composer.lock packages must contain objects")
			}
			if entry["name"] != "drupal/core" && entry["name"] != "drupal/core-recommended" {
				continue
			}
			name := entry["name"].(string)
			if seen[name] {
				return fmt.Errorf("duplicate locked Drupal package %s", name)
			}
			seen[name] = true
			text, ok := entry["version"].(string)
			if !ok {
				return fmt.Errorf("locked Drupal package requires a version string")
			}
			match := exactVersion.FindStringSubmatch(text)
			if match == nil {
				coreWithoutExactVersion = coreWithoutExactVersion || name == "drupal/core"
				continue
			}
			if locked != "" && locked != match[1] {
				return fmt.Errorf("conflicting Drupal core/core-recommended versions in composer.lock")
			}
			locked = match[1]
			if facts.Version == nil || entry["name"] == "drupal/core" {
				source := "composer.lock:" + entry["name"].(string)
				facts.Version, facts.VersionSource = &locked, &source
			}
		}
	}
	if coreWithoutExactVersion {
		facts.Version, facts.VersionSource = nil, nil
	}
	return nil
}

func installerPaths(composer map[string]any) (map[string][]string, error) {
	object, err := objectAt(composer, "extra", "installer-paths")
	if err != nil {
		return nil, err
	}
	result := map[string][]string{}
	for name, value := range object {
		values, ok := value.([]any)
		if !ok {
			return nil, fmt.Errorf("Composer installer path %q must contain an array", name)
		}
		for _, value := range values {
			kind, ok := value.(string)
			if !ok {
				return nil, fmt.Errorf("Composer installer path selectors must be strings")
			}
			result[name] = append(result[name], kind)
		}
	}
	return result, nil
}

func layout(reader reader, webRoot string, facts *profile.Facts) error {
	addExclusion(facts, path.Join(webRoot, "core"))
	for _, kind := range []string{"modules", "themes", "profiles"} {
		addExclusion(facts, path.Join(webRoot, kind, "contrib"))
		if err := addSource(reader, path.Join(webRoot, kind, "custom"), facts); err != nil {
			return err
		}
	}
	sites := path.Join(webRoot, "sites")
	exists, err := reader.directory(sites)
	if err != nil || !exists {
		return err
	}
	entries, err := os.ReadDir(filepath.Join(reader.root, filepath.FromSlash(sites)))
	if err != nil {
		return err
	}
	listing := []string{}
	for _, entry := range entries {
		listing = append(listing, entry.Name()+":"+entry.Type().String())
		if !entry.IsDir() {
			continue
		}
		for _, kind := range []string{"modules", "themes", "profiles"} {
			base := path.Join(sites, entry.Name(), kind)
			addExclusion(facts, path.Join(base, "contrib"))
			if err := addSource(reader, path.Join(base, "custom"), facts); err != nil {
				return err
			}
		}
	}
	encoded, _ := json.Marshal(listing)
	reader.inputs[sites] = fmt.Sprintf("listing:%x", sha256.Sum256(encoded))
	return nil
}

func addSource(reader reader, name string, facts *profile.Facts) error {
	exists, err := reader.directory(name)
	if err != nil {
		return err
	}
	if exists {
		facts.SourceRoots = append(facts.SourceRoots, path.Clean(name))
	}
	return nil
}

func addExclusion(facts *profile.Facts, name string) {
	escaped := strings.NewReplacer("[", "[[]", "*", "[*]", "?", "[?]").Replace(name)
	facts.Exclusions = append(facts.Exclusions, escaped, escaped+"/**")
}

func safePath(name string) error {
	if err := projectpath.Validate(name); err != nil {
		return err
	}
	if strings.ContainsAny(name, "{}$") {
		return fmt.Errorf("unsupported variable in discovery path %q", name)
	}
	return nil
}

// relocate prefixes the paths a Composer project in a subdirectory reports.
func relocate(facts *profile.Facts, composerRoot string) {
	prefix := func(value string) string { return path.Join(composerRoot, value) }
	described := func(value *string) {
		if value != nil && strings.HasPrefix(*value, "composer.") {
			*value = composerRoot + "/" + *value
		}
	}
	if facts.WebRoot != nil {
		facts.WebRoot = new(prefix(*facts.WebRoot))
	}
	described(facts.RootSource)
	described(facts.VersionSource)
	for i, name := range facts.SourceRoots {
		facts.SourceRoots[i] = prefix(name)
	}
	escaped := strings.NewReplacer("[", "[[]", "*", "[*]", "?", "[?]").Replace(composerRoot)
	for i, pattern := range facts.Exclusions {
		facts.Exclusions[i] = escaped + "/" + pattern
	}
	inputs := map[string]string{}
	for name, value := range facts.Inputs {
		inputs[prefix(name)] = value
	}
	facts.Inputs = inputs
	if facts.Installed != nil {
		facts.Installed.Path = prefix(facts.Installed.Path)
	}
	for i := range facts.PHP {
		described(&facts.PHP[i].Source)
	}
	for i := range facts.Warnings {
		described(&facts.Warnings[i])
	}
}

// pathRepositories adds Drupal packages from Composer path repositories as custom code at their real location in the project.
func pathRepositories(root, composerRoot string, facts *profile.Facts) error {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	// composer.json was already read, so its duplicate-key warnings are not repeated.
	reader := reader{root: root, inputs: facts.Inputs, warnings: new([]string)}
	composer, err := reader.object(path.Join(composerRoot, "composer.json"))
	if err != nil || composer == nil {
		return err
	}
	var repositories []any
	switch value := composer["repositories"].(type) {
	case []any:
		repositories = value
	case map[string]any:
		for _, name := range slices.Sorted(maps.Keys(value)) {
			repositories = append(repositories, value[name])
		}
	}
	for _, value := range repositories {
		repository, _ := value.(map[string]any)
		url, _ := repository["url"].(string)
		if repository["type"] != "path" || url == "" {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(root, filepath.FromSlash(composerRoot), filepath.FromSlash(url)))
		if err != nil || filepath.IsAbs(url) {
			facts.Warnings = append(facts.Warnings, fmt.Sprintf("Composer path repository %q is not a project-relative path; its code is not checked", url))
			continue
		}
		slices.Sort(matches)
		for _, match := range matches {
			real, err := filepath.EvalSymlinks(match)
			if err != nil {
				continue
			}
			name, err := filepath.Rel(root, real)
			if err != nil || name == ".." || strings.HasPrefix(name, ".."+string(filepath.Separator)) {
				facts.Warnings = append(facts.Warnings, fmt.Sprintf("Composer path repository %q is outside the project root; its code is not checked", url))
				continue
			}
			name = filepath.ToSlash(name)
			if name == "." {
				continue
			}
			pkg, err := reader.object(path.Join(name, "composer.json"))
			if err != nil {
				facts.Warnings = append(facts.Warnings, fmt.Sprintf("Composer path repository %q: %v", url, err))
				continue
			}
			kind, _ := pkg["type"].(string)
			if strings.HasPrefix(kind, "drupal-") && kind != "drupal-core" && kind != "drupal-library" && !slices.Contains(facts.SourceRoots, name) {
				facts.SourceRoots = append(facts.SourceRoots, name)
			}
		}
	}
	sort.Strings(facts.SourceRoots)
	return nil
}
