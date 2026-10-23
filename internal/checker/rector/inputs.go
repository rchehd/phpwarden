package rector

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/checker/phpstan"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/edit"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/internal/scope"
)

var ConfigCandidates = []string{"rector.php", "rector.dist.php"}
var className = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\\[A-Za-z_][A-Za-z0-9_]*)+$`)

type plan struct {
	Config     string   `json:"config"`
	Sets       []string `json:"sets"`
	PHP        int      `json:"php_version"`
	Drupal     string   `json:"drupal_version"`
	Rules      []string `json:"rules"`
	SkipRules  []string `json:"skip_rules"`
	Files      []string `json:"files"`
	Extensions []string `json:"extensions"`
	Memory     int      `json:"memory_limit_mb"`
	Project    string   `json:"project"`
	Autoload   string   `json:"autoload"`
}

func Select(selected scope.Selection, cfg config.Rector) scope.Selection {
	defaults, _ := profile.Lookup("drupal")
	extensions := append(slices.Clone(defaults.PHPExtensions), cfg.ExtraExtensions...)
	return selected.Filter(cfg.Include, cfg.Exclude, func(name string) bool { return slices.Contains(extensions, strings.ToLower(path.Ext(name))) })
}

func InputRequired(cfg config.Config, selected scope.Selection, only []string) bool {
	return checker.Enabled("rector", cfg.Tools.Rector.Enabled, only) && len(Select(selected, cfg.Tools.Rector).Files) > 0
}

func ContextHash(root string) string { return phpstan.ContextHash(root) }

func prepare(directory, composerRoot, targetPHP, bundle string, selected scope.Selection, cfg config.Rector) (plan, error) {
	result := plan{Sets: []string{}, Rules: slices.Clone(cfg.Rules), SkipRules: slices.Clone(cfg.SkipRules), Files: []string{}, Extensions: []string{}, Memory: cfg.MemoryLimitMB, Project: directory}
	if !cfg.DryRun {
		return result, fmt.Errorf("Rector supports dry_run: true only; applying upgrades is a separate action")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return result, err
	}
	defer root.Close()
	regular := func(name string) (string, error) {
		if _, _, err := edit.Read(root, name); err != nil {
			return "", fmt.Errorf("Rector input %q must be a readable regular project file without symlinks", name)
		}
		return filepath.Join(directory, filepath.FromSlash(name)), nil
	}
	configFile := cfg.Config
	if configFile == "" && cfg.DiscoverConfig {
		for _, dir := range slices.Compact([]string{".", composerRoot}) {
			for _, candidate := range ConfigCandidates {
				name := path.Join(dir, candidate)
				if _, _, err := edit.Read(root, name); os.IsNotExist(err) {
					continue
				} else if err != nil {
					return result, fmt.Errorf("Rector config %q is unreadable or unsafe", name)
				}
				configFile = name
				break
			}
			if configFile != "" {
				break
			}
		}
	}
	if configFile != "" {
		if result.Config, err = regular(configFile); err != nil {
			return result, err
		}
	}
	for _, name := range cfg.ExtraInputs {
		if _, err := regular(name); err != nil {
			return result, err
		}
	}
	for _, name := range selected.Files {
		filename, err := regular(name)
		if err != nil {
			return result, err
		}
		result.Files = append(result.Files, filename)
		result.Extensions = append(result.Extensions, strings.TrimPrefix(strings.ToLower(path.Ext(name)), "."))
	}
	slices.Sort(result.Extensions)
	result.Extensions = slices.Compact(result.Extensions)
	if cfg.TargetPHP != "" {
		targetPHP = cfg.TargetPHP
	}
	var major, minor int
	if _, err := fmt.Sscanf(targetPHP, "%d.%d", &major, &minor); err != nil {
		return result, fmt.Errorf("invalid Rector PHP target")
	}
	result.PHP = major*10000 + minor*100
	if cfg.TargetPHP != "" {
		result.Sets = append(result.Sets, filepath.Join(bundle, "vendor/rector/rector/config/set/level/up-to-php"+strings.ReplaceAll(targetPHP, ".", "")+".php"))
	}
	if cfg.TargetDrupal != "" {
		if !slices.Contains([]string{"10.0", "10.1", "10.2", "10.3", "10.4", "10.5", "10.6", "11.0", "11.1", "11.2", "11.3", "11.4", "12.0"}, cfg.TargetDrupal) {
			return result, fmt.Errorf("unsupported Drupal Rector target %q for this pinned toolset", cfg.TargetDrupal)
		}
		result.Drupal = cfg.TargetDrupal + ".0"
		for _, version := range drupalVersions {
			parts := strings.Split(version, ".")
			vMajor, _ := strconv.Atoi(parts[0])
			vMinor, _ := strconv.Atoi(parts[1])
			var tMajor, tMinor int
			fmt.Sscanf(cfg.TargetDrupal, "%d.%d", &tMajor, &tMinor)
			if vMajor < tMajor || vMajor == tMajor && vMinor <= tMinor {
				result.Sets = append(result.Sets, drupalSet(bundle, version))
			}
		}
	}
	for _, name := range cfg.Sets {
		if strings.HasPrefix(name, "drupal-") && slices.Contains(drupalVersions, strings.TrimPrefix(name, "drupal-")) {
			result.Sets = append(result.Sets, drupalSet(bundle, strings.TrimPrefix(name, "drupal-")))
		} else {
			filename, err := regular(name)
			if err != nil {
				return result, err
			}
			result.Sets = append(result.Sets, filename)
		}
	}
	for _, rule := range append(slices.Clone(cfg.Rules), cfg.SkipRules...) {
		if !className.MatchString(rule) {
			return result, fmt.Errorf("Rector rules must be fully qualified class names")
		}
	}
	if result.Config == "" && len(result.Sets) == 0 && len(result.Rules) == 0 {
		return result, fmt.Errorf("Rector needs an explicit target_php, target_drupal, sets, rules or project rector.php")
	}
	autoload, err := projectAutoload(root, composerRoot)
	if err != nil {
		return result, err
	}
	if autoload != "" {
		result.Autoload, err = regular(autoload)
	}
	return result, err
}

func drupalSet(bundle, version string) string {
	return filepath.Join(bundle, "vendor/palantirnet/drupal-rector/config/drupal-"+strings.Split(version, ".")[0]+"/drupal-"+version+"-deprecations.php")
}

func projectAutoload(root *os.Root, composerRoot string) (string, error) {
	vendor := "vendor"
	data, _, err := edit.Read(root, path.Join(composerRoot, "composer.json"))
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("Rector cannot read project Composer metadata")
	}
	if err == nil {
		var document struct {
			Config struct {
				Vendor string `json:"vendor-dir"`
			} `json:"config"`
		}
		if json.Unmarshal(data, &document) != nil {
			return "", fmt.Errorf("Rector cannot parse project Composer metadata")
		}
		if document.Config.Vendor != "" {
			vendor = document.Config.Vendor
		}
	}
	if err := projectpath.Validate(vendor); err != nil {
		return "", fmt.Errorf("Rector vendor-dir must be a relative project path")
	}
	name := path.Join(composerRoot, vendor, "autoload.php")
	if _, _, err := edit.Read(root, name); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("Rector project autoloader is unreadable or unsafe")
	}
	return name, nil
}

var drupalVersions = []string{"8.0", "8.1", "8.2", "8.3", "8.4", "8.5", "8.6", "8.7", "8.8", "9.0", "9.1", "9.2", "9.3", "9.4", "10.0", "10.1", "10.2", "10.3", "11.0", "11.1", "11.2", "11.3", "11.4", "12.0"}
