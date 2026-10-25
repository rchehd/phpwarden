package rector

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
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
var errDependencies = errors.New("Rector dependencies unavailable")

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

func Select(selected scope.Selection, cfg config.Rector, profileID string) scope.Selection {
	defaults, _ := profile.Lookup(profileID)
	extensions := append(slices.Clone(defaults.PHPExtensions), cfg.ExtraExtensions...)
	return selected.Filter(cfg.Include, cfg.Exclude, func(name string) bool { return slices.Contains(extensions, strings.ToLower(path.Ext(name))) })
}

func InputRequired(cfg config.Config, selected scope.Selection, only []string) bool {
	return checker.Enabled("rector", cfg.Tools.Rector.Enabled, only) && len(Select(selected, cfg.Tools.Rector, cfg.Profile).Files) > 0
}

func ContextHash(root string) string { return phpstan.ContextHash(root) }

func prepare(directory, bundle string, selected scope.Selection, project config.Config) (plan, error) {
	cfg, composerRoot, targetPHP := project.Tools.Rector, project.ComposerRoot, project.PHP.Target.Max
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
		preset, err := profile.RectorPresetFor("drupal", cfg.TargetDrupal+".0")
		if err != nil {
			return result, err
		}
		result.Drupal = preset.DrupalVersion
		for _, filename := range preset.Sets {
			result.Sets = append(result.Sets, filepath.Join(bundle, filepath.FromSlash(filename)))
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
	automatic := result.Config == "" && len(result.Sets) == 0 && len(result.Rules) == 0
	if automatic {
		preset, err := profile.RectorPresetFor(project.Profile, project.Framework.Version)
		if err != nil {
			return result, err
		}
		result.Drupal = preset.DrupalVersion
		for _, filename := range preset.Sets {
			result.Sets = append(result.Sets, filepath.Join(bundle, filepath.FromSlash(filename)))
		}
		vendor, err := projectVendor(root, composerRoot)
		if err != nil {
			return result, err
		}
		if _, _, err := edit.Read(root, path.Join(vendor, "composer/installed.json")); err != nil {
			return result, fmt.Errorf("%w: the upstream Drupal preset needs readable Composer-installed package metadata; restore locked dependencies with composer install in %q", errDependencies, composerRoot)
		}
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

func projectVendor(root *os.Root, composerRoot string) (string, error) {
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
	return path.Join(composerRoot, vendor), nil
}

func projectAutoload(root *os.Root, composerRoot string) (string, error) {
	vendor, err := projectVendor(root, composerRoot)
	if err != nil {
		return "", err
	}
	name := path.Join(vendor, "autoload.php")
	if _, _, err := edit.Read(root, name); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("Rector project autoloader is unreadable or unsafe")
	}
	return name, nil
}

var drupalVersions = profile.DrupalRectorVersions()
