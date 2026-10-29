package projecttest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/internal/scope"
)

var ErrUnavailable = errors.New("test prerequisites unavailable")

type Options struct {
	MutationScopeOptions scope.Options
	Mutation             bool
	MutationConfig       string
	MutationScope        scope.Selection
	Coverage             bool
	Config               string
	Paths                []string
	Suite                string
	Filter               string
	Output               string
}

type Plan struct {
	Mutation                   *MutationPlan   `json:"mutation,omitempty"`
	Coverage                   config.Coverage `json:"coverage"`
	CoverageExtensions         []string        `json:"coverage_extensions,omitempty"`
	CoverageExcludeDirectories []string        `json:"coverage_exclude_directories,omitempty"`
	Root                       string          `json:"-"`
	ComposerRoot               string          `json:"composer_root"`
	Executable                 string          `json:"phpunit"`
	Config                     string          `json:"config"`
	Paths                      []string        `json:"paths"`
	Suite                      string          `json:"suite,omitempty"`
	Filter                     string          `json:"filter,omitempty"`
	TimeoutSeconds             int             `json:"timeout_seconds"`
}

func Build(ctx context.Context, root string, effective config.Effective, options Options) (Plan, error) {
	cfg := effective.Config
	plan := Plan{Coverage: cfg.Tests.Coverage, Root: root, ComposerRoot: cfg.ComposerRoot, Executable: cfg.Tests.PHPUnit, Config: cfg.Tests.Config,
		Paths: slices.Clone(cfg.Tests.Paths), Suite: cfg.Tests.Suite, Filter: options.Filter, TimeoutSeconds: cfg.Tests.TimeoutSeconds}
	if options.Coverage || options.Mutation {
		plan.Coverage.Enabled = true
	}
	if options.Config != "" {
		plan.Config = options.Config
	}
	if len(options.Paths) > 0 {
		plan.Paths, plan.Suite = slices.Clone(options.Paths), ""
	}
	if options.Suite != "" {
		plan.Suite, plan.Paths = options.Suite, nil
	}
	defaults, ok := profile.Lookup(cfg.Profile)
	if !ok {
		return plan, fmt.Errorf("unsupported test profile %q", cfg.Profile)
	}
	if effective.Project != nil && effective.Project.Installed != nil && effective.Project.Installed.Status == "invalid" {
		detail := "invalid installed framework metadata"
		if effective.Project.Installed.Error != nil {
			detail = *effective.Project.Installed.Error
		}
		return plan, fmt.Errorf("%w: %s; restore the project's locked dependencies with composer install", ErrUnavailable, detail)
	}
	if plan.Executable == "auto" {
		var composer struct {
			Config struct {
				Vendor string `json:"vendor-dir"`
			} `json:"config"`
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(cfg.ComposerRoot), "composer.json"))
		if err != nil {
			return plan, fmt.Errorf("PHPUnit discovery needs composer.json: %w", err)
		}
		if err := json.Unmarshal(data, &composer); err != nil {
			return plan, err
		}
		if composer.Config.Vendor == "" {
			composer.Config.Vendor = "vendor"
		}
		if err := projectpath.Validate(composer.Config.Vendor); err != nil {
			return plan, err
		}
		plan.Executable = path.Join(cfg.ComposerRoot, composer.Config.Vendor, "bin/phpunit")
	}
	if err := regularFile(root, plan.Executable); err != nil {
		return plan, fmt.Errorf("%w: project PHPUnit is unavailable at %s; install the project's locked dev dependencies (composer install): %v", ErrUnavailable, plan.Executable, err)
	}
	profileConfig := ""
	if effective.Project != nil && effective.Project.WebRoot != nil {
		profileConfig = path.Join(*effective.Project.WebRoot, defaults.PHPUnitConfig)
	}
	if plan.Config == "auto" {
		candidates := []string{path.Join(cfg.ComposerRoot, "phpunit.xml"), path.Join(cfg.ComposerRoot, "phpunit.xml.dist")}
		if effective.Project != nil && effective.Project.WebRoot != nil {
			candidates = append(candidates, path.Join(*effective.Project.WebRoot, defaults.PHPUnitConfig))
		}
		plan.Config = ""
		for _, candidate := range candidates {
			if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(candidate))); err == nil {
				plan.Config = candidate
				break
			} else if !os.IsNotExist(err) {
				return plan, err
			}
		}
		if plan.Config == "" {
			return plan, fmt.Errorf("no project PHPUnit configuration found; use tests.config or --phpunit-config")
		}
	}
	if err := regularFile(root, plan.Config); err != nil {
		return plan, fmt.Errorf("PHPUnit configuration: %w", err)
	}
	if len(plan.Paths) == 0 && plan.Suite == "" {
		if effective.Project == nil {
			return plan, fmt.Errorf("test discovery requires project source roots; set tests.paths or tests.suite")
		}
		for _, source := range effective.Project.SourceRoots {
			if err := projectpath.Within(root, source); err != nil {
				return plan, err
			}
			err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(source)), func(name string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				if !entry.IsDir() {
					return nil
				}
				relative, _ := filepath.Rel(root, name)
				if filepath.ToSlash(relative) == defaults.UnitTestDirectory || strings.HasSuffix(filepath.ToSlash(relative), "/"+defaults.UnitTestDirectory) {
					plan.Paths = append(plan.Paths, filepath.ToSlash(relative))
					return filepath.SkipDir
				}
				return nil
			})
			if err != nil && !os.IsNotExist(err) {
				return plan, err
			}
		}
		if len(plan.Paths) == 0 {
			return plan, fmt.Errorf("%w: no custom unit-test directories found; set tests.paths or --testsuite explicitly", ErrUnavailable)
		}
	}
	for i, name := range plan.Paths {
		if err := projectpath.Validate(name); err != nil {
			return plan, err
		}
		if err := projectpath.Within(root, name); err != nil {
			return plan, err
		}
		info, err := os.Stat(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil {
			return plan, err
		}
		if !info.IsDir() && !info.Mode().IsRegular() {
			return plan, fmt.Errorf("test path is not a file or directory: %s", name)
		}
		plan.Paths[i] = path.Clean(name)
	}
	slices.Sort(plan.Paths)
	plan.Paths = slices.Compact(plan.Paths)
	var unique []string
	for _, name := range plan.Paths {
		name = path.Clean(name)
		if !slices.ContainsFunc(unique, func(parent string) bool { return name == parent || strings.HasPrefix(name, parent+"/") }) {
			unique = append(unique, name)
		}
	}
	plan.Paths = unique
	if plan.Coverage.Enabled {
		plan.CoverageExtensions = slices.Clone(defaults.PHPExtensions)
		plan.CoverageExcludeDirectories = slices.Clone(defaults.CoverageExcludeDirectories)
		plan.Coverage.Include = slices.Clone(plan.Coverage.Include)
		if len(plan.Coverage.Include) == 0 && plan.Config == profileConfig && effective.Project != nil {
			for _, name := range effective.Project.SourceRoots {
				if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err == nil {
					plan.Coverage.Include = append(plan.Coverage.Include, name)
				}
			}
			if len(plan.Coverage.Include) == 0 {
				return plan, fmt.Errorf("coverage requires custom source roots or tests.coverage.include")
			}
		}
		for _, name := range append(slices.Clone(plan.Coverage.Include), plan.Coverage.Exclude...) {
			if err := projectpath.Validate(name); err != nil {
				return plan, err
			}
			if err := projectpath.Within(root, name); err != nil {
				return plan, err
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
				return plan, err
			}
		}
	}
	if options.Mutation {
		mutation, err := buildMutation(ctx, root, effective, options.MutationScope, options.MutationConfig)
		if err != nil {
			return plan, err
		}
		plan.Mutation = mutation
		plan.Coverage.MinLineCoverage, plan.Coverage.MaxCRAP = nil, nil
		plan.TimeoutSeconds = effective.Config.Mutation.TimeoutSeconds
	}
	return plan, nil
}

func regularFile(root, name string) error {
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
		return fmt.Errorf("not a regular project file: %s", name)
	}
	return nil
}
