package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/inputsource"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/schemas"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	Version int    `json:"version"`
	Profile string `json:"profile"`
	// Directory with composer.json, composer.lock and vendor, relative to the project root.
	ComposerRoot string    `json:"composer_root"`
	Framework    Framework `json:"framework"`
	PHP          PHP       `json:"php"`
	Scope        Scope     `json:"scope"`
	Runner       Runner    `json:"runner"`
	Agents       Agents    `json:"agents"`
	Updates      Updates   `json:"updates"`
	Tests        Tests     `json:"tests"`
	Mutation     Mutation  `json:"mutation"`
	Tools        Tools     `json:"tools"`
}

type Mutation struct {
	Config               string   `json:"config"`
	TimeoutSeconds       int      `json:"timeout_seconds"`
	Threads              *int     `json:"threads,omitempty"`
	MutantTimeoutSeconds *int     `json:"mutant_timeout_seconds,omitempty"`
	Mutators             []string `json:"mutators"`
	MinMSI               *float64 `json:"min_msi,omitempty"`
	MinCoveredMSI        *float64 `json:"min_covered_msi,omitempty"`
}

type Coverage struct {
	Enabled         bool     `json:"enabled"`
	Include         []string `json:"include"`
	Exclude         []string `json:"exclude"`
	MinLineCoverage *float64 `json:"min_line_coverage,omitempty"`
	MaxCRAP         *float64 `json:"max_crap,omitempty"`
}

type Tests struct {
	Coverage       Coverage `json:"coverage"`
	PHPUnit        string   `json:"phpunit"`
	Config         string   `json:"config"`
	Paths          []string `json:"paths"`
	Suite          string   `json:"suite"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

type Agents struct {
	Completion Completion `json:"completion"`
}

type Completion struct {
	Enabled        bool `json:"enabled"`
	MaxAttempts    int  `json:"max_attempts"`
	TimeoutSeconds int  `json:"timeout_seconds"`
}

type Updates struct {
	TimeoutSeconds int  `json:"timeout_seconds"`
	MaxAgeHours    int  `json:"max_age_hours"`
	MaxStaleHours  int  `json:"max_stale_hours"`
	Offline        bool `json:"offline"`
}

type Framework struct {
	Version string `json:"version"`
}

type PHP struct {
	Runtime string `json:"runtime"`
	Target  Target `json:"target"`
}

type Target struct {
	Min string `json:"min"`
	Max string `json:"max"`
}

type Scope struct {
	Mode       string   `json:"mode"`
	Base       string   `json:"base,omitempty"`
	Include    []string `json:"include,omitempty"`
	Exclude    []string `json:"exclude"`
	WholeFiles bool     `json:"whole_files"`
}

type Runner struct {
	ImagePull      string `json:"image_pull"`
	Concurrency    int    `json:"concurrency"`
	ResultCache    bool   `json:"result_cache"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

type Tools struct {
	ComposerRequireChecker ComposerRequireChecker `json:"composer-require-checker"`
	ComposerUnused         ComposerUnused         `json:"composer-unused"`
	Deptrac                Deptrac                `json:"deptrac"`
	Rector                 Rector                 `json:"rector"`
	Semgrep                Semgrep                `json:"semgrep"`
	EventHorizon           EventHorizon           `json:"eventhorizon"`
	Gitleaks               Gitleaks               `json:"gitleaks"`
	ESLint                 NodeLint               `json:"eslint"`
	Stylelint              NodeLint               `json:"stylelint"`
	JSCPD                  JSCPD                  `json:"jscpd"`
	PHPStan                PHPStan                `json:"phpstan"`
	DeadCode               DeadCode               `json:"dead-code"`
	TwigCSFixer            TwigCSFixer            `json:"twig-cs-fixer"`
	ComposerValidate       ComposerValidate       `json:"composer-validate"`
	ComposerAudit          ComposerAudit          `json:"composer-audit"`
	PHPCS                  PHPCS                  `json:"phpcs"`
	PHPMD                  PHPMD                  `json:"phpmd"`
	DrupalConfig           DrupalConfig           `json:"drupal-config"`
	Debug                  Debug                  `json:"debug"`
	Syntax                 Syntax                 `json:"syntax"`
	YAML                   YAML                   `json:"yaml"`
	Whitespace             Whitespace             `json:"whitespace"`
}

type NodeLint struct {
	Enabled         bool     `json:"enabled"`
	Required        bool     `json:"required"`
	FailOn          string   `json:"fail_on"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	Include         []string `json:"include,omitempty"`
	Exclude         []string `json:"exclude"`
	Config          string   `json:"config"`
	DiscoverConfig  bool     `json:"discover_config"`
	ExtraExtensions []string `json:"extra_extensions"`
	ExtraInputs     []string `json:"extra_inputs"`
}

type JSCPD struct {
	Enabled        bool     `json:"enabled"`
	Required       bool     `json:"required"`
	FailOn         string   `json:"fail_on"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Include        []string `json:"include,omitempty"`
	Exclude        []string `json:"exclude"`
	Config         string   `json:"config"`
	DiscoverConfig bool     `json:"discover_config"`
	MinTokens      int      `json:"min_tokens"`
	MinLines       int      `json:"min_lines"`
	Mode           string   `json:"mode"`
}

type PHPStan struct {
	Enabled         bool        `json:"enabled"`
	Required        bool        `json:"required"`
	FailOn          string      `json:"fail_on"`
	TimeoutSeconds  int         `json:"timeout_seconds"`
	Include         []string    `json:"include,omitempty"`
	Exclude         []string    `json:"exclude"`
	Config          string      `json:"config"`
	DiscoverConfig  bool        `json:"discover_config"`
	Level           *int        `json:"level,omitempty"`
	Includes        []string    `json:"includes"`
	Baseline        string      `json:"baseline"`
	BootstrapFiles  []string    `json:"bootstrap_files"`
	ScanDirectories []string    `json:"scan_directories"`
	StubFiles       []string    `json:"stub_files"`
	Autoload        string      `json:"autoload"`
	Drupal          bool        `json:"drupal"`
	Deprecations    bool        `json:"deprecations"`
	MemoryLimitMB   int         `json:"memory_limit_mb"`
	ExtraExtensions []string    `json:"extra_extensions"`
	StrictRules     StrictRules `json:"strict_rules"`
}

type StrictRules struct {
	Enabled bool            `json:"enabled"`
	All     bool            `json:"all"`
	Rules   map[string]bool `json:"rules"`
}

type Gitleaks struct {
	Enabled        bool     `json:"enabled"`
	Required       bool     `json:"required"`
	FailOn         string   `json:"fail_on"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Include        []string `json:"include,omitempty"`
	Exclude        []string `json:"exclude"`
	Config         string   `json:"config"`
	DiscoverConfig bool     `json:"discover_config"`
	Baseline       string   `json:"baseline"`
	IgnoreFile     string   `json:"ignore_file"`
	Rules          []string `json:"rules"`
	ExtraInputs    []string `json:"extra_inputs"`
}

type EventHorizon struct {
	Enabled        bool     `json:"enabled"`
	Required       bool     `json:"required"`
	FailOn         string   `json:"fail_on"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Include        []string `json:"include,omitempty"`
	Exclude        []string `json:"exclude"`
	Caching        bool     `json:"caching"`
	ConfigSync     []string `json:"config_sync"`
	ExcludeRules   []string `json:"exclude_rules"`
}

type PackageTool struct {
	Enabled        bool     `json:"enabled"`
	Required       bool     `json:"required"`
	FailOn         string   `json:"fail_on"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	ComposerRoot   string   `json:"composer_root"`
	Config         string   `json:"config"`
	DiscoverConfig bool     `json:"discover_config"`
	ExtraInputs    []string `json:"extra_inputs"`
	MemoryLimitMB  int      `json:"memory_limit_mb"`
}

type ComposerRequireChecker struct {
	PackageTool
	ScanFiles      []string `json:"scan_files"`
	AllowedSymbols []string `json:"allowed_symbols"`
}

type ComposerUnused struct {
	PackageTool
	ExcludePackages    []string `json:"exclude_packages"`
	ExcludeDirectories []string `json:"exclude_directories"`
}

type Deptrac struct {
	Enabled         bool     `json:"enabled"`
	Required        bool     `json:"required"`
	FailOn          string   `json:"fail_on"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	Include         []string `json:"include,omitempty"`
	Exclude         []string `json:"exclude"`
	Config          string   `json:"config"`
	DiscoverConfig  bool     `json:"discover_config"`
	ExtraInputs     []string `json:"extra_inputs"`
	ReportUncovered bool     `json:"report_uncovered"`
	FailOnUncovered bool     `json:"fail_on_uncovered"`
	ReportSkipped   bool     `json:"report_skipped"`
	MemoryLimitMB   int      `json:"memory_limit_mb"`
}

type Rector struct {
	Enabled         bool     `json:"enabled"`
	Required        bool     `json:"required"`
	FailOn          string   `json:"fail_on"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	Include         []string `json:"include,omitempty"`
	Exclude         []string `json:"exclude"`
	Config          string   `json:"config"`
	DiscoverConfig  bool     `json:"discover_config"`
	TargetPHP       string   `json:"target_php"`
	TargetDrupal    string   `json:"target_drupal"`
	Sets            []string `json:"sets"`
	Rules           []string `json:"rules"`
	SkipRules       []string `json:"skip_rules"`
	ExtraInputs     []string `json:"extra_inputs"`
	ExtraExtensions []string `json:"extra_extensions"`
	MemoryLimitMB   int      `json:"memory_limit_mb"`
	DryRun          bool     `json:"dry_run"`
}

type Semgrep struct {
	Enabled            bool     `json:"enabled"`
	Required           bool     `json:"required"`
	FailOn             string   `json:"fail_on"`
	TimeoutSeconds     int      `json:"timeout_seconds"`
	Include            []string `json:"include,omitempty"`
	Exclude            []string `json:"exclude"`
	Config             string   `json:"config"`
	DiscoverConfig     bool     `json:"discover_config"`
	UseDefaults        bool     `json:"use_defaults"`
	Rulesets           []string `json:"rulesets"`
	Rules              []string `json:"rules"`
	ExcludeRules       []string `json:"exclude_rules"`
	Severities         []string `json:"severities"`
	Baseline           string   `json:"baseline"`
	ExtraInputs        []string `json:"extra_inputs"`
	ExtraExtensions    []string `json:"extra_extensions"`
	RuleTimeoutSeconds int      `json:"rule_timeout_seconds"`
	MemoryLimitMB      int      `json:"memory_limit_mb"`
}

type DeadCode struct {
	Enabled        bool     `json:"enabled"`
	Required       bool     `json:"required"`
	FailOn         string   `json:"fail_on"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Include        []string `json:"include,omitempty"`
	Exclude        []string `json:"exclude"`
	MemoryLimitMB  int      `json:"memory_limit_mb"`
}

type ComposerValidate struct {
	Enabled        bool     `json:"enabled"`
	Required       bool     `json:"required"`
	FailOn         string   `json:"fail_on"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Include        []string `json:"include,omitempty"`
	Exclude        []string `json:"exclude"`
	Strict         bool     `json:"strict"`
}

type ComposerAudit struct {
	Enabled        bool                  `json:"enabled"`
	Required       bool                  `json:"required"`
	FailOn         string                `json:"fail_on"`
	TimeoutSeconds int                   `json:"timeout_seconds"`
	NoDev          bool                  `json:"no_dev"`
	Abandoned      string                `json:"abandoned"`
	Offline        bool                  `json:"offline"`
	MaxAgeHours    int                   `json:"max_age_hours"`
	MaxStaleHours  int                   `json:"max_stale_hours"`
	Ignore         []ComposerAuditIgnore `json:"ignore"`
}

type ComposerAuditIgnore struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
	Until  string `json:"until,omitempty"`
}

type TwigCSFixer struct {
	Autoload        string   `json:"autoload"`
	Enabled         bool     `json:"enabled"`
	Required        bool     `json:"required"`
	FailOn          string   `json:"fail_on"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	Include         []string `json:"include,omitempty"`
	Exclude         []string `json:"exclude"`
	Config          string   `json:"config"`
	DiscoverConfig  bool     `json:"discover_config"`
	ExtraExtensions []string `json:"extra_extensions"`
	ExtraInputs     []string `json:"extra_inputs"`
}

type PHPCS struct {
	Enabled         bool               `json:"enabled"`
	Required        bool               `json:"required"`
	FailOn          string             `json:"fail_on"`
	TimeoutSeconds  int                `json:"timeout_seconds"`
	Include         []string           `json:"include,omitempty"`
	Exclude         []string           `json:"exclude"`
	Config          string             `json:"config"`
	DiscoverConfig  bool               `json:"discover_config"`
	Standards       []string           `json:"standards"`
	Sniffs          []string           `json:"sniffs"`
	ExcludeSniffs   []string           `json:"exclude_sniffs"`
	StandardPaths   []string           `json:"standard_paths"`
	ExtraInputs     []string           `json:"extra_inputs"`
	ExtraExtensions []string           `json:"extra_extensions"`
	ErrorSeverity   int                `json:"error_severity"`
	WarningSeverity int                `json:"warning_severity"`
	Compatibility   PHPCSCompatibility `json:"compatibility"`
}

type PHPMD struct {
	Enabled         bool     `json:"enabled"`
	Required        bool     `json:"required"`
	FailOn          string   `json:"fail_on"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	Include         []string `json:"include,omitempty"`
	Exclude         []string `json:"exclude"`
	Config          string   `json:"config"`
	DiscoverConfig  bool     `json:"discover_config"`
	Rulesets        []string `json:"rulesets"`
	ExcludeRules    []string `json:"exclude_rules"`
	ExtraInputs     []string `json:"extra_inputs"`
	ExtraExtensions []string `json:"extra_extensions"`
	MemoryLimitMB   int      `json:"memory_limit_mb"`
}

type PHPCSCompatibility struct {
	Enabled     bool   `json:"enabled"`
	TestVersion string `json:"test_version"`
}

type DrupalConfig struct {
	Enabled        bool     `json:"enabled"`
	Required       bool     `json:"required"`
	FailOn         string   `json:"fail_on"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Include        []string `json:"include,omitempty"`
	Exclude        []string `json:"exclude"`
	Directories    []string `json:"directories"`
	ForbiddenKeys  []string `json:"forbidden_keys"`
	ModuleRoots    []string `json:"module_roots"`
}

type Whitespace struct {
	Enabled                   bool     `json:"enabled"`
	Required                  bool     `json:"required"`
	FailOn                    string   `json:"fail_on"`
	TimeoutSeconds            int      `json:"timeout_seconds"`
	Include                   []string `json:"include,omitempty"`
	Exclude                   []string `json:"exclude"`
	TrailingWhitespace        bool     `json:"trailing_whitespace"`
	ConflictMarkers           bool     `json:"conflict_markers"`
	TrailingWhitespaceExclude []string `json:"trailing_whitespace_exclude"`
	ConflictMarkersExclude    []string `json:"conflict_markers_exclude"`
}

type YAML struct {
	Enabled        bool     `json:"enabled"`
	Required       bool     `json:"required"`
	FailOn         string   `json:"fail_on"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Include        []string `json:"include,omitempty"`
	Exclude        []string `json:"exclude"`
}

type Syntax struct {
	Backend         string   `json:"backend"`
	ParallelJobs    int      `json:"parallel_jobs"`
	Enabled         bool     `json:"enabled"`
	Required        bool     `json:"required"`
	FailOn          string   `json:"fail_on"`
	TimeoutSeconds  int      `json:"timeout_seconds"`
	Include         []string `json:"include,omitempty"`
	Exclude         []string `json:"exclude"`
	ExtraExtensions []string `json:"extra_extensions"`
}

type Debug struct {
	Enabled        bool     `json:"enabled"`
	Required       bool     `json:"required"`
	FailOn         string   `json:"fail_on"`
	TimeoutSeconds int      `json:"timeout_seconds"`
	Include        []string `json:"include,omitempty"`
	Exclude        []string `json:"exclude"`
	Functions      []string `json:"functions"`
	JSFunctions    []string `json:"js_functions"`
	TwigFunctions  []string `json:"twig_functions"`
}

type Effective struct {
	Config     Config            `json:"config"`
	Provenance map[string]string `json:"provenance"`
	Pending    []string          `json:"pending"`
	SourceHash string            `json:"-"`
	Project    *profile.Facts    `json:"project,omitempty"`
}

var coreExcludes = []string{"**/.git", "**/.git/**", "**/vendor/**", "**/node_modules/**"}

func Parse(data []byte) (Effective, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return Effective{}, yamlSyntax(err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return Effective{}, yamlSyntax(err)
		}
		return Effective{}, yamlSource(&extra, "", "expected one YAML document")
	}
	if len(document.Content) != 1 {
		return Effective{}, fmt.Errorf("expected a YAML mapping")
	}
	root := document.Content[0]
	if root.Kind == yaml.MappingNode {
		for i := 0; i < len(root.Content); i += 2 {
			if root.Content[i].Value == "version" && root.Content[i+1].ShortTag() != "!!int" {
				return Effective{}, yamlSource(root.Content[i+1], "$.version", "version must be an integer")
			}
		}
	}
	value, err := jsonValue(document.Content[0], "$")
	if err != nil {
		return Effective{}, err
	}
	if err := schemas.ValidateConfig(value); err != nil {
		return Effective{}, schemaSource(root, err)
	}
	input := value.(map[string]any)
	defaults, ok := profile.Lookup(input["profile"].(string))
	if !ok {
		return Effective{}, fmt.Errorf("unsupported profile")
	}
	result := Effective{
		Config: Config{
			ComposerRoot: ".",
			Framework:    Framework{Version: "auto"},
			Scope:        Scope{Mode: "diff", Exclude: []string{}},
			Runner:       Runner{Concurrency: 1, TimeoutSeconds: 300, ImagePull: "missing"},
			Agents:       Agents{Completion: Completion{Enabled: true, MaxAttempts: 2, TimeoutSeconds: 300}},
			Updates:      Updates{TimeoutSeconds: 120, MaxAgeHours: 24, MaxStaleHours: 168},
			Mutation:     Mutation{Config: "auto", TimeoutSeconds: 1800, Mutators: []string{}},
			Tests:        Tests{Coverage: Coverage{Include: []string{}, Exclude: []string{}}, PHPUnit: "auto", Config: "auto", Paths: []string{}, TimeoutSeconds: 300},
			Tools: Tools{PHPStan: PHPStan{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120, Exclude: []string{}, DiscoverConfig: true,
				Includes: []string{}, BootstrapFiles: []string{}, ScanDirectories: []string{}, StubFiles: []string{}, Autoload: "auto",
				Drupal: defaults.PHPStanDrupal, Deprecations: true, MemoryLimitMB: 512, ExtraExtensions: []string{}, StrictRules: StrictRules{All: true, Rules: map[string]bool{}}},
				Gitleaks:               Gitleaks{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120, Exclude: []string{}, DiscoverConfig: true, Rules: []string{}, ExtraInputs: []string{}},
				EventHorizon:           EventHorizon{Enabled: defaults.EventHorizonEnabled, Required: true, FailOn: "never", TimeoutSeconds: 120, Exclude: []string{}, Caching: true, ConfigSync: []string{}, ExcludeRules: []string{}},
				ComposerRequireChecker: ComposerRequireChecker{PackageTool: PackageTool{Enabled: false, Required: true, FailOn: "never", TimeoutSeconds: 300, DiscoverConfig: true, ExtraInputs: []string{}, MemoryLimitMB: 1024}, ScanFiles: []string{}, AllowedSymbols: []string{}},
				ComposerUnused:         ComposerUnused{PackageTool: PackageTool{Enabled: false, Required: true, FailOn: "never", TimeoutSeconds: 300, DiscoverConfig: true, ExtraInputs: []string{}, MemoryLimitMB: 1024}, ExcludePackages: []string{}, ExcludeDirectories: []string{}},
				Deptrac:                Deptrac{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 300, Exclude: []string{}, DiscoverConfig: true, ExtraInputs: []string{}, MemoryLimitMB: 1024},
				Rector:                 Rector{Enabled: defaults.RectorEnabled, Required: true, FailOn: "never", TimeoutSeconds: 600, Exclude: []string{}, DiscoverConfig: true, Sets: []string{}, Rules: []string{}, SkipRules: []string{}, ExtraInputs: []string{}, ExtraExtensions: []string{}, MemoryLimitMB: 1024, DryRun: true},
				Semgrep:                Semgrep{Enabled: defaults.SemgrepEnabled, Required: true, FailOn: "error", TimeoutSeconds: 300, Exclude: []string{}, DiscoverConfig: true, UseDefaults: true, Rulesets: []string{}, Rules: []string{}, ExcludeRules: []string{}, Severities: []string{"ERROR", "WARNING", "INFO"}, ExtraInputs: []string{}, ExtraExtensions: []string{}, RuleTimeoutSeconds: 5, MemoryLimitMB: 1024},
				DeadCode:               DeadCode{Enabled: defaults.DeadCodeEnabled, Required: true, FailOn: "error", TimeoutSeconds: 600, Exclude: []string{}, MemoryLimitMB: 1024},
				ESLint:                 NodeLint{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120, Exclude: []string{"**/*.min.js"}, DiscoverConfig: true, ExtraExtensions: []string{}, ExtraInputs: []string{}},
				Stylelint:              NodeLint{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120, Exclude: []string{"**/*.min.css"}, DiscoverConfig: true, ExtraExtensions: []string{}, ExtraInputs: []string{}},
				JSCPD:                  JSCPD{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 300, Exclude: []string{"**/*.min.js", "**/*.min.css"}, DiscoverConfig: true, MinTokens: 70, MinLines: 6, Mode: "mild"},
				ComposerValidate:       ComposerValidate{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120, Exclude: []string{}},
				ComposerAudit: ComposerAudit{Enabled: true, Required: false, FailOn: "error", TimeoutSeconds: 120, Abandoned: "report",
					MaxAgeHours: 24, MaxStaleHours: 168, Ignore: []ComposerAuditIgnore{}},
				TwigCSFixer: TwigCSFixer{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120, Exclude: []string{}, DiscoverConfig: true,
					ExtraExtensions: []string{}, ExtraInputs: []string{}},
				PHPCS: PHPCS{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120, Exclude: []string{}, DiscoverConfig: true,
					Standards: defaults.PHPCSStandards, Sniffs: []string{}, ExcludeSniffs: []string{}, StandardPaths: []string{}, ExtraInputs: []string{}, ExtraExtensions: []string{}, ErrorSeverity: 5, WarningSeverity: 5},
				PHPMD: PHPMD{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120, Exclude: []string{}, DiscoverConfig: true,
					Rulesets: defaults.PHPMDRulesets, ExcludeRules: defaults.PHPMDExcludeRules, ExtraInputs: []string{}, ExtraExtensions: []string{}, MemoryLimitMB: 512},
				DrupalConfig: DrupalConfig{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120, Exclude: []string{},
					Directories: defaults.ModuleConfigDirectories, ForbiddenKeys: defaults.ModuleConfigForbiddenKeys, ModuleRoots: []string{}},
				Whitespace: Whitespace{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120, Exclude: []string{},
					TrailingWhitespace: true, ConflictMarkers: true, TrailingWhitespaceExclude: []string{"**/*.md", "**/*.patch", "**/*.diff"}, ConflictMarkersExclude: []string{}},
				YAML: YAML{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120, Exclude: []string{}},
				Syntax: Syntax{Backend: "php", ParallelJobs: 4, Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120,
					Exclude: []string{}, ExtraExtensions: []string{}}, Debug: Debug{
					Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120,
					Exclude: []string{}, Functions: defaults.DebugFunctions, JSFunctions: defaults.DebugJSFunctions, TwigFunctions: defaults.DebugTwigFunctions,
				}},
		},
		Provenance: map[string]string{
			"tests.phpunit": "core_default", "tests.config": "core_default", "tests.paths": "core_default", "tests.suite": "core_default", "tests.timeout_seconds": "core_default",
			"runner.image_pull": "core_default",
			"framework.version": "core_default", "composer_root": "core_default", "scope.mode": "core_default",
			"scope.exclude": "core_default", "scope.whole_files": "core_default",
			"runner.concurrency": "core_default", "runner.result_cache": "core_default", "runner.timeout_seconds": "core_default", "tools.debug.enabled": "profile_default",
			"tools.debug.required": "core_default", "tools.debug.fail_on": "core_default",
			"tools.debug.timeout_seconds": "core_default", "tools.debug.exclude": "core_default",
			"tools.debug.functions": "profile_default", "tools.debug.js_functions": "profile_default", "tools.debug.twig_functions": "profile_default",
			"tools.syntax.backend": "core_default", "tools.syntax.parallel_jobs": "core_default",
			"tools.syntax.enabled": "profile_default", "tools.syntax.required": "core_default",
			"tools.syntax.fail_on": "core_default", "tools.syntax.timeout_seconds": "core_default",
			"tools.syntax.exclude": "core_default", "tools.syntax.extra_extensions": "core_default",
			"tools.yaml.enabled": "profile_default", "tools.yaml.required": "core_default",
			"tools.yaml.fail_on": "core_default", "tools.yaml.timeout_seconds": "core_default",
			"tools.yaml.exclude":       "core_default",
			"tools.whitespace.enabled": "profile_default", "tools.whitespace.required": "core_default",
			"tools.whitespace.fail_on": "core_default", "tools.whitespace.timeout_seconds": "core_default",
			"tools.whitespace.exclude": "core_default", "tools.whitespace.trailing_whitespace": "core_default",
			"tools.whitespace.conflict_markers": "core_default", "tools.whitespace.trailing_whitespace_exclude": "core_default",
			"tools.whitespace.conflict_markers_exclude": "core_default",
			"tools.drupal-config.enabled":               "profile_default", "tools.drupal-config.required": "core_default",
			"tools.drupal-config.fail_on": "core_default", "tools.drupal-config.timeout_seconds": "core_default",
			"tools.drupal-config.exclude": "core_default", "tools.drupal-config.directories": "profile_default",
			"tools.drupal-config.forbidden_keys": "profile_default", "tools.drupal-config.module_roots": "core_default",
		},
		Pending: []string{},
	}
	for _, field := range []string{"config", "timeout_seconds", "threads", "mutant_timeout_seconds", "mutators", "min_msi", "min_covered_msi"} {
		result.Provenance["mutation."+field] = "core_default"
	}
	for _, field := range []string{"enabled", "include", "exclude", "min_line_coverage", "max_crap"} {
		result.Provenance["tests.coverage."+field] = "core_default"
	}

	for _, field := range []string{"enabled", "max_attempts", "timeout_seconds"} {
		result.Provenance["agents.completion."+field] = "core_default"
	}
	for _, field := range []string{"timeout_seconds", "max_age_hours", "max_stale_hours", "offline"} {
		result.Provenance["updates."+field] = "core_default"
	}
	for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "exclude", "config", "discover_config", "standards", "sniffs", "exclude_sniffs", "standard_paths", "extra_inputs", "extra_extensions", "error_severity", "warning_severity", "compatibility.enabled", "compatibility.test_version"} {
		result.Provenance["tools.phpcs."+field] = "core_default"
	}
	result.Provenance["tools.phpcs.enabled"], result.Provenance["tools.phpcs.standards"] = "profile_default", "profile_default"
	for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "exclude", "config", "discover_config", "includes", "baseline", "bootstrap_files", "scan_directories", "stub_files", "autoload", "drupal", "deprecations", "memory_limit_mb", "extra_extensions"} {
		result.Provenance["tools.phpstan."+field] = "core_default"
	}
	result.Provenance["tools.phpstan.enabled"], result.Provenance["tools.phpstan.drupal"] = "profile_default", "profile_default"
	for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "exclude", "config", "discover_config", "extra_extensions", "extra_inputs", "autoload"} {
		result.Provenance["tools.twig-cs-fixer."+field] = "core_default"
	}
	for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "exclude", "config", "discover_config", "rulesets", "exclude_rules", "extra_inputs", "extra_extensions", "memory_limit_mb"} {
		result.Provenance["tools.phpmd."+field] = "core_default"
	}
	result.Provenance["tools.phpmd.enabled"], result.Provenance["tools.phpmd.rulesets"], result.Provenance["tools.phpmd.exclude_rules"] = "profile_default", "profile_default", "profile_default"
	result.Provenance["tools.twig-cs-fixer.enabled"] = "profile_default"
	for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "exclude", "memory_limit_mb"} {
		result.Provenance["tools.dead-code."+field] = "core_default"
	}
	result.Provenance["tools.dead-code.enabled"] = "profile_default"
	for _, field := range []string{"enabled", "all", "rules"} {
		result.Provenance["tools.phpstan.strict_rules."+field] = "core_default"
	}
	for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "exclude", "config", "discover_config", "target_php", "target_drupal", "sets", "rules", "skip_rules", "extra_inputs", "extra_extensions", "memory_limit_mb", "dry_run"} {
		result.Provenance["tools.rector."+field] = "core_default"
	}
	result.Provenance["tools.rector.enabled"] = "profile_default"
	for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "exclude", "config", "discover_config", "extra_inputs", "report_uncovered", "fail_on_uncovered", "report_skipped", "memory_limit_mb"} {
		result.Provenance["tools.deptrac."+field] = "core_default"
	}
	for _, id := range []string{"composer-require-checker", "composer-unused"} {
		for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "composer_root", "config", "discover_config", "extra_inputs", "memory_limit_mb"} {
			result.Provenance["tools."+id+"."+field] = "core_default"
		}
	}
	for _, field := range []string{"scan_files", "allowed_symbols"} {
		result.Provenance["tools.composer-require-checker."+field] = "core_default"
	}
	for _, field := range []string{"exclude_packages", "exclude_directories"} {
		result.Provenance["tools.composer-unused."+field] = "core_default"
	}
	for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "exclude", "caching", "config_sync", "exclude_rules"} {
		result.Provenance["tools.eventhorizon."+field] = "core_default"
	}
	result.Provenance["tools.eventhorizon.enabled"] = "profile_default"
	for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "exclude", "config", "discover_config", "use_defaults", "rulesets", "rules", "exclude_rules", "severities", "baseline", "extra_inputs", "extra_extensions", "rule_timeout_seconds", "memory_limit_mb"} {
		result.Provenance["tools.semgrep."+field] = "core_default"
	}
	result.Provenance["tools.semgrep.enabled"] = "profile_default"
	for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "exclude", "config", "discover_config", "baseline", "ignore_file", "rules", "extra_inputs"} {
		result.Provenance["tools.gitleaks."+field] = "core_default"
	}
	for _, id := range []string{"eslint", "stylelint", "jscpd"} {
		for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "exclude", "config", "discover_config"} {
			result.Provenance["tools."+id+"."+field] = "core_default"
		}
	}
	for _, id := range []string{"eslint", "stylelint"} {
		for _, field := range []string{"extra_extensions", "extra_inputs"} {
			result.Provenance["tools."+id+"."+field] = "core_default"
		}
	}
	for _, field := range []string{"min_tokens", "min_lines", "mode"} {
		result.Provenance["tools.jscpd."+field] = "core_default"
	}
	for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "exclude", "strict"} {
		result.Provenance["tools.composer-validate."+field] = "core_default"
	}
	for _, field := range []string{"enabled", "required", "fail_on", "timeout_seconds", "no_dev", "abandoned", "offline", "max_age_hours", "max_stale_hours", "ignore"} {
		result.Provenance["tools.composer-audit."+field] = "core_default"
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return Effective{}, err
	}
	if err := json.Unmarshal(encoded, &result.Config); err != nil {
		return Effective{}, fmt.Errorf("decode configuration: %w", err)
	}
	markProvenance(input, "", result.Provenance)
	if len(result.Config.Scope.Exclude) > 0 {
		result.Provenance["scope.exclude"] = "core_default+project_config"
	} else {
		result.Provenance["scope.exclude"] = "core_default"
	}
	result.Config.Scope.Exclude = append(append([]string{}, coreExcludes...), result.Config.Scope.Exclude...)
	result.Config.ComposerRoot = path.Clean(result.Config.ComposerRoot)
	if err := validateValues(result.Config); err != nil {
		return Effective{}, semanticSource(root, err)
	}
	if result.Config.Framework.Version == "auto" {
		result.Pending = append(result.Pending, "framework.version")
	}
	if len(result.Config.Scope.Include) == 0 {
		result.Pending = append(result.Pending, "scope.include")
	}
	result.Pending = append(result.Pending, "profile.exclusions")
	return result, nil
}

func jsonValue(node *yaml.Node, location string) (any, error) {
	if node.Anchor != "" || node.Kind == yaml.AliasNode {
		return nil, yamlSource(node, location, "YAML anchors and aliases are not supported")
	}
	switch node.Kind {
	case yaml.MappingNode:
		if node.ShortTag() != "!!map" {
			break
		}
		result := make(map[string]any)
		keys := make(map[string]*yaml.Node)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" || key.Value == "<<" || key.Anchor != "" {
				return nil, yamlSource(key, location, "expected string keys without merge keys")
			}
			if _, exists := result[key.Value]; exists {
				err := yamlSource(key, inputsource.Member(location, key.Value), fmt.Sprintf("duplicate key %q; keep a single definition", key.Value))
				err.Previous = &inputsource.Position{Line: keys[key.Value].Line, Column: keys[key.Value].Column}
				return nil, err
			}
			keys[key.Value] = key
			value, err := jsonValue(node.Content[i+1], inputsource.Member(location, key.Value))
			if err != nil {
				return nil, err
			}
			result[key.Value] = value
		}
		return result, nil
	case yaml.SequenceNode:
		if node.ShortTag() != "!!seq" {
			break
		}
		result := make([]any, len(node.Content))
		for i, child := range node.Content {
			value, err := jsonValue(child, fmt.Sprintf("%s[%d]", location, i))
			if err != nil {
				return nil, err
			}
			result[i] = value
		}
		return result, nil
	case yaml.ScalarNode:
		switch node.ShortTag() {
		case "!!str", "!!bool", "!!int", "!!float", "!!null":
			var value any
			if err := node.Decode(&value); err != nil {
				return nil, yamlSource(node, location, err.Error())
			}
			if number, ok := value.(float64); ok && (math.IsNaN(number) || math.IsInf(number, 0)) {
				return nil, yamlSource(node, location, "numbers must be finite")
			}
			return value, nil
		}
	}
	return nil, yamlSource(node, location, fmt.Sprintf("unsupported YAML type %q", node.Tag))
}

func markProvenance(values map[string]any, prefix string, sources map[string]string) {
	for key, value := range values {
		name := key
		if prefix != "" {
			name = prefix + "." + key
		}
		if nested, ok := value.(map[string]any); ok {
			markProvenance(nested, name, sources)
		} else {
			sources[name] = "project_config"
		}
	}
}

func validateValues(cfg Config) error {
	if cfg.Mutation.Config != "auto" {
		if err := projectpath.Validate(cfg.Mutation.Config); err != nil {
			return fmt.Errorf("mutation.config: %w", err)
		}
	}
	for _, name := range cfg.Mutation.Mutators {
		if strings.ContainsAny(name, "\x00\r\n ,") {
			return fmt.Errorf("mutation.mutators must contain individual mutator/profile names")
		}
	}

	for _, name := range append(slices.Clone(cfg.Tests.Coverage.Include), cfg.Tests.Coverage.Exclude...) {
		if err := projectpath.Validate(name); err != nil {
			return fmt.Errorf("tests.coverage: %w", err)
		}
	}
	if strings.ContainsAny(cfg.Tests.Suite, "\x00\r\n") {
		return fmt.Errorf("tests.suite must be a single-line suite name")
	}
	for _, name := range append([]string{cfg.Tests.PHPUnit, cfg.Tests.Config}, cfg.Tests.Paths...) {
		if name != "auto" {
			if err := projectpath.Validate(name); err != nil {
				return fmt.Errorf("tests: %w", err)
			}
		}
	}
	if len(cfg.Tests.Paths) > 0 && cfg.Tests.Suite != "" {
		return fmt.Errorf("tests.paths and tests.suite are mutually exclusive")
	}
	if cfg.Updates.MaxStaleHours < cfg.Updates.MaxAgeHours {
		return fmt.Errorf("updates.max_stale_hours must be at least updates.max_age_hours")
	}
	if cfg.Tools.Deptrac.Config != "" && !strings.HasSuffix(cfg.Tools.Deptrac.Config, ".yaml") && !strings.HasSuffix(cfg.Tools.Deptrac.Config, ".yml") && !strings.HasSuffix(cfg.Tools.Deptrac.Config, ".yaml.dist") && !strings.HasSuffix(cfg.Tools.Deptrac.Config, ".yml.dist") {
		return fmt.Errorf("tools.deptrac.config must be a local YAML file")
	}
	if cfg.Tools.Rector.Config != "" && path.Ext(cfg.Tools.Rector.Config) != ".php" {
		return fmt.Errorf("tools.rector.config must be a local PHP file")
	}

	for _, name := range append(append([]string{cfg.Tools.Semgrep.Config}, cfg.Tools.Semgrep.Rulesets...), cfg.Tools.Semgrep.Baseline) {
		if strings.Contains(name, "://") || name == "auto" {
			return fmt.Errorf("Semgrep accepts local project rules and baselines only")
		}
	}
	for _, name := range append([]string{cfg.Tools.Semgrep.Config}, cfg.Tools.Semgrep.Rulesets...) {
		if name != "" && !slices.Contains([]string{".yml", ".yaml", ".json"}, strings.ToLower(path.Ext(name))) {
			return fmt.Errorf("Semgrep rulesets must be local YAML or JSON files")
		}
	}
	for _, values := range [][]string{cfg.Scope.Include, cfg.Tools.Debug.Include, cfg.Tools.Syntax.Include, cfg.Tools.YAML.Include, cfg.Tools.Whitespace.Include,
		cfg.Tools.DrupalConfig.Include, cfg.Tools.DrupalConfig.Directories, cfg.Tools.DrupalConfig.ModuleRoots,
		cfg.Tools.PHPCS.Include, cfg.Tools.PHPCS.StandardPaths, cfg.Tools.PHPCS.ExtraInputs,
		cfg.Tools.PHPStan.Include, cfg.Tools.PHPStan.Includes, cfg.Tools.PHPStan.BootstrapFiles, cfg.Tools.PHPStan.ScanDirectories, cfg.Tools.PHPStan.StubFiles,
		cfg.Tools.TwigCSFixer.Include, cfg.Tools.TwigCSFixer.ExtraInputs, cfg.Tools.ComposerValidate.Include, cfg.Tools.PHPMD.Include, cfg.Tools.PHPMD.ExtraInputs, cfg.Tools.DeadCode.Include,
		cfg.Tools.ESLint.Include, cfg.Tools.ESLint.ExtraInputs, cfg.Tools.Stylelint.Include, cfg.Tools.Stylelint.ExtraInputs, cfg.Tools.JSCPD.Include,
		cfg.Tools.Gitleaks.Include, cfg.Tools.Gitleaks.ExtraInputs, cfg.Tools.EventHorizon.Include, cfg.Tools.EventHorizon.ConfigSync, cfg.Tools.Semgrep.Include, cfg.Tools.Semgrep.Rulesets, cfg.Tools.Semgrep.ExtraInputs, cfg.Tools.Rector.Include, cfg.Tools.Rector.Sets, cfg.Tools.Rector.ExtraInputs, cfg.Tools.Deptrac.Include, cfg.Tools.Deptrac.ExtraInputs, cfg.Tools.ComposerRequireChecker.ExtraInputs, cfg.Tools.ComposerRequireChecker.ScanFiles, cfg.Tools.ComposerUnused.ExtraInputs, cfg.Tools.ComposerUnused.ExcludeDirectories} {
		for _, value := range values {
			if err := projectpath.Validate(value); err != nil {
				return err
			}
		}
	}
	if err := projectpath.Validate(cfg.ComposerRoot); err != nil {
		return fmt.Errorf("composer_root: %w", err)
	}
	for _, name := range []string{cfg.Tools.PHPStan.Config, cfg.Tools.PHPStan.Baseline, cfg.Tools.PHPStan.Autoload, cfg.Tools.TwigCSFixer.Autoload, cfg.Tools.TwigCSFixer.Config, cfg.Tools.PHPMD.Config, cfg.Tools.ESLint.Config, cfg.Tools.Stylelint.Config, cfg.Tools.JSCPD.Config,
		cfg.Tools.Gitleaks.Config, cfg.Tools.Gitleaks.Baseline, cfg.Tools.Gitleaks.IgnoreFile, cfg.Tools.Semgrep.Config, cfg.Tools.Semgrep.Baseline, cfg.Tools.Rector.Config, cfg.Tools.Deptrac.Config, cfg.Tools.ComposerRequireChecker.Config, cfg.Tools.ComposerRequireChecker.ComposerRoot, cfg.Tools.ComposerUnused.Config, cfg.Tools.ComposerUnused.ComposerRoot} {
		if name != "" && name != "auto" {
			if err := projectpath.Validate(name); err != nil {
				return err
			}
		}
	}
	for _, item := range []struct{ name, value, suffix string }{{"composer-require-checker", cfg.Tools.ComposerRequireChecker.Config, ".json"}, {"composer-unused", cfg.Tools.ComposerUnused.Config, ".php"}} {
		if item.value != "" && !strings.HasSuffix(item.value, item.suffix) {
			return fmt.Errorf("tools.%s.config must be a %s file", item.name, item.suffix)
		}
	}
	if cfg.Tools.PHPCS.Config != "" {
		if err := projectpath.Validate(cfg.Tools.PHPCS.Config); err != nil {
			return err
		}
	}
	for _, name := range cfg.Tools.PHPCS.StandardPaths {
		if strings.Contains(name, ",") {
			return fmt.Errorf("PHPCS standard_paths cannot contain commas")
		}
	}
	if cfg.Tools.PHPCS.Compatibility.TestVersion != "" {
		parts := strings.Split(cfg.Tools.PHPCS.Compatibility.TestVersion, "-")
		min, err := parseVersion(parts[0])
		if err != nil {
			return err
		}
		if len(parts) == 2 {
			max, err := parseVersion(parts[1])
			if err != nil {
				return err
			}
			if min[0] > max[0] || min[0] == max[0] && min[1] > max[1] {
				return fmt.Errorf("PHPCS compatibility test_version range is reversed")
			}
		}
	}
	if cfg.Tools.ComposerAudit.MaxStaleHours < cfg.Tools.ComposerAudit.MaxAgeHours {
		return fmt.Errorf("tools.composer-audit.max_stale_hours must not be lower than max_age_hours")
	}
	for _, ignore := range cfg.Tools.ComposerAudit.Ignore {
		if ignore.Until != "" {
			if _, err := time.Parse(time.DateOnly, ignore.Until); err != nil {
				return fmt.Errorf("tools.composer-audit.ignore %q: until must be a YYYY-MM-DD date", ignore.ID)
			}
		}
	}
	for _, directory := range cfg.Tools.DrupalConfig.Directories {
		if path.Clean(directory) == "." {
			return fmt.Errorf("tools.drupal-config.directories must name subdirectories of a module")
		}
	}
	for _, values := range [][]string{cfg.Scope.Exclude, cfg.Tools.Debug.Exclude, cfg.Tools.Syntax.Exclude, cfg.Tools.YAML.Exclude,
		cfg.Tools.Whitespace.Exclude, cfg.Tools.Whitespace.TrailingWhitespaceExclude, cfg.Tools.Whitespace.ConflictMarkersExclude, cfg.Tools.DrupalConfig.Exclude, cfg.Tools.PHPCS.Exclude, cfg.Tools.PHPStan.Exclude, cfg.Tools.TwigCSFixer.Exclude, cfg.Tools.ComposerValidate.Exclude,
		cfg.Tools.ESLint.Exclude, cfg.Tools.Stylelint.Exclude, cfg.Tools.JSCPD.Exclude, cfg.Tools.Gitleaks.Exclude, cfg.Tools.EventHorizon.Exclude, cfg.Tools.Semgrep.Exclude, cfg.Tools.Rector.Exclude, cfg.Tools.Deptrac.Exclude} {
		if err := validatePatterns(values); err != nil {
			return err
		}
	}
	minimum, err := parseVersion(cfg.PHP.Target.Min)
	if err != nil {
		return err
	}
	maximum, err := parseVersion(cfg.PHP.Target.Max)
	if err != nil {
		return err
	}
	if _, err := parseVersion(cfg.PHP.Runtime); err != nil {
		return err
	}
	if minimum[0] > maximum[0] || minimum[0] == maximum[0] && minimum[1] > maximum[1] {
		return fmt.Errorf("php.target.min must not exceed php.target.max")
	}
	if cfg.Scope.Base != "" && strings.TrimSpace(cfg.Scope.Base) == "" {
		return fmt.Errorf("scope.base must not be blank")
	}
	seen := map[string]bool{}
	for _, function := range cfg.Tools.Debug.Functions {
		name := strings.ToLower(function)
		if seen[name] {
			return fmt.Errorf("duplicate debug function %q (case-insensitive)", function)
		}
		seen[name] = true
	}
	return nil
}
