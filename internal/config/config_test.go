package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/schemas"
)

const minimal = `version: 1
profile: drupal
php:
  runtime: "8.3"
  target:
    min: "8.3"
    max: "8.4"
`

func TestDefaultsAndProvenance(t *testing.T) {
	result, err := Parse([]byte(minimal))
	if err != nil {
		t.Fatal(err)
	}
	if result.Config.Scope.Mode != "diff" || result.Config.Runner.TimeoutSeconds != 300 {
		t.Fatalf("unexpected defaults: %+v", result.Config)
	}
	debug := result.Config.Tools.Debug
	if !debug.Enabled || !debug.Required || debug.FailOn != "error" || debug.TimeoutSeconds != 120 {
		t.Fatalf("unexpected debug defaults: %+v", debug)
	}
	if !slices.Contains(debug.Functions, "var_dump") || !slices.Contains(debug.Functions, "dpm") {
		t.Fatalf("missing base/Drupal debug functions: %v", debug.Functions)
	}
	syntax := result.Config.Tools.Syntax
	if !syntax.Enabled || !syntax.Required || syntax.FailOn != "error" || syntax.TimeoutSeconds != 120 || syntax.ExtraExtensions == nil ||
		result.Provenance["tools.syntax.enabled"] != "profile_default" {
		t.Fatalf("unexpected syntax defaults: %+v", syntax)
	}
	yaml := result.Config.Tools.YAML
	if !yaml.Enabled || !yaml.Required || yaml.FailOn != "error" || yaml.TimeoutSeconds != 120 || yaml.Exclude == nil ||
		result.Provenance["tools.yaml.enabled"] != "profile_default" {
		t.Fatalf("unexpected YAML defaults: %+v", yaml)
	}
	whitespace := result.Config.Tools.Whitespace
	if !whitespace.Enabled || !whitespace.Required || whitespace.FailOn != "error" || whitespace.TimeoutSeconds != 120 ||
		!whitespace.TrailingWhitespace || !whitespace.ConflictMarkers || len(whitespace.TrailingWhitespaceExclude) != 3 || whitespace.ConflictMarkersExclude == nil ||
		result.Provenance["tools.whitespace.enabled"] != "profile_default" {
		t.Fatalf("unexpected whitespace defaults: %+v", whitespace)
	}
	drupal := result.Config.Tools.DrupalConfig
	if !drupal.Enabled || !drupal.Required || drupal.FailOn != "error" || drupal.TimeoutSeconds != 120 ||
		!slices.Equal(drupal.ForbiddenKeys, []string{"uuid", "_core"}) || len(drupal.Directories) != 3 || drupal.ModuleRoots == nil || drupal.Exclude == nil ||
		result.Provenance["tools.drupal-config.forbidden_keys"] != "profile_default" {
		t.Fatalf("unexpected Drupal config defaults: %+v", drupal)
	}
	phpcs := result.Config.Tools.PHPCS
	if !phpcs.Enabled || !phpcs.Required || phpcs.FailOn != "error" || !phpcs.DiscoverConfig || phpcs.Compatibility.Enabled ||
		!slices.Equal(phpcs.Standards, []string{"Drupal", "DrupalPractice"}) || phpcs.ErrorSeverity != 5 || phpcs.Sniffs == nil || result.Provenance["tools.phpcs.standards"] != "profile_default" {
		t.Fatalf("unexpected PHPCS defaults: %+v", phpcs)
	}
	phpstan := result.Config.Tools.PHPStan
	if !phpstan.Enabled || !phpstan.Required || phpstan.FailOn != "error" || !phpstan.DiscoverConfig || phpstan.Level != nil || phpstan.Autoload != "auto" ||
		!phpstan.Drupal || !phpstan.Deprecations || phpstan.MemoryLimitMB != 512 || phpstan.Includes == nil || phpstan.Exclude == nil ||
		result.Provenance["tools.phpstan.drupal"] != "profile_default" || result.Provenance["tools.phpstan.level"] != "" {
		t.Fatalf("unexpected PHPStan defaults: %+v", phpstan)
	}
	twig := result.Config.Tools.TwigCSFixer
	if !twig.Enabled || !twig.Required || twig.FailOn != "error" || !twig.DiscoverConfig || twig.Config != "" || twig.ExtraExtensions == nil || twig.ExtraInputs == nil ||
		result.Provenance["tools.twig-cs-fixer.enabled"] != "profile_default" || result.Provenance["tools.twig-cs-fixer.config"] != "core_default" {
		t.Fatalf("unexpected Twig CS Fixer defaults: %+v", twig)
	}
	validate, audit := result.Config.Tools.ComposerValidate, result.Config.Tools.ComposerAudit
	if !validate.Enabled || !validate.Required || validate.Strict || !audit.Enabled || audit.Required || audit.FailOn != "error" || audit.Abandoned != "report" ||
		audit.Offline || audit.MaxAgeHours != 24 || audit.MaxStaleHours != 168 || audit.Ignore == nil || result.Provenance["tools.composer-audit.required"] != "core_default" {
		t.Fatalf("unexpected Composer defaults: %+v %+v", validate, audit)
	}
	if result.Provenance["php.runtime"] != "project_config" ||
		result.Provenance["tools.debug.functions"] != "profile_default" ||
		result.Provenance["scope.mode"] != "core_default" {
		t.Fatalf("incorrect provenance: %v", result.Provenance)
	}
	if !slices.Equal(result.Pending, []string{"framework.version", "scope.include", "profile.exclusions"}) {
		t.Fatalf("discovery must remain explicit: %v", result.Pending)
	}
}

func TestExplicitOverridesPreserveFalseAndReplaceLists(t *testing.T) {
	result, err := Parse([]byte(minimal + `framework:
  version: "11.0.0"
scope:
  mode: full
  include: [custom]
  exclude: ["**/generated/**"]
runner:
  timeout_seconds: 60
tools:
  debug:
    enabled: false
    required: false
    fail_on: never
    timeout_seconds: 12
    functions: [custom_debug]
`))
	if err != nil {
		t.Fatal(err)
	}
	debug := result.Config.Tools.Debug
	if debug.Enabled || debug.Required || debug.FailOn != "never" || debug.TimeoutSeconds != 12 {
		t.Fatalf("explicit values lost: %+v", debug)
	}
	if !slices.Equal(debug.Functions, []string{"custom_debug"}) {
		t.Fatalf("functions must replace defaults: %v", debug.Functions)
	}
	if !slices.Contains(result.Config.Scope.Exclude, "**/vendor/**") ||
		!slices.Contains(result.Config.Scope.Exclude, "**/generated/**") {
		t.Fatalf("exclusions must be additive: %v", result.Config.Scope.Exclude)
	}
	if result.Provenance["scope.exclude"] != "core_default+project_config" ||
		result.Provenance["tools.debug.enabled"] != "project_config" {
		t.Fatalf("incorrect override provenance: %v", result.Provenance)
	}
	if !slices.Equal(result.Pending, []string{"profile.exclusions"}) {
		t.Fatalf("unexpected unresolved fields: %v", result.Pending)
	}
}

func TestInvalidConfigurations(t *testing.T) {
	cases := map[string]string{
		"empty":                 "",
		"sequence root":         "[version, 1]",
		"malformed YAML":        minimal + "scope: [",
		"unknown field":         minimal + "profiel: drupal\n",
		"nested unknown field":  minimal + "runner: {timeot: 1}\n",
		"unknown checker":       minimal + "tools: {phpstna: {}}\n",
		"unimplemented checker": minimal + "tools: {jscpd: {}}\n",
		"unsupported profile":   strings.Replace(minimal, "profile: drupal", "profile: laravel", 1),
		"missing profile":       strings.Replace(minimal, "profile: drupal\n", "", 1),
		"unknown version":       strings.Replace(minimal, "version: 1", "version: 2", 1),
		"string version":        strings.Replace(minimal, "version: 1", "version: \"1\"", 1),
		"float version":         strings.Replace(minimal, "version: 1", "version: 1.0", 1),
		"numeric PHP":           strings.Replace(minimal, "runtime: \"8.3\"", "runtime: 8.3", 1),
		"invalid PHP":           strings.Replace(minimal, "runtime: \"8.3\"", "runtime: \"8.3.1\"", 1),
		"overflow PHP":          strings.Replace(minimal, "runtime: \"8.3\"", "runtime: \"999999999999999.3\"", 1),
		"missing target max":    strings.Replace(minimal, "    max: \"8.4\"\n", "", 1),
		"reversed target":       strings.Replace(minimal, "min: \"8.3\"", "min: \"8.5\"", 1),
		"null PHP":              "version: 1\nprofile: drupal\nphp: null\n",
		"null optional section": minimal + "tools: null\n",
		"null optional field":   minimal + "scope: {base: null}\n",
		"duplicate root key":    minimal + "profile: drupal\n",
		"duplicate nested key":  minimal + "tools: {debug: {enabled: true, enabled: false}}\n",
		"nonstring key":         minimal + "42: value\n",
		"alias":                 minimal + "scope: &settings {mode: diff}\nrunner: *settings\n",
		"anchor only":           minimal + "scope: &settings {mode: diff}\n",
		"merge":                 minimal + "scope: {<<: {mode: diff}}\n",
		"custom tag":            minimal + "scope: !custom {mode: diff}\n",
		"custom scalar tag":     strings.Replace(minimal, "profile: drupal", "profile: !custom drupal", 1),
		"date":                  minimal + "scope: {base: 2026-10-22}\n",
		"NaN":                   minimal + "runner: {timeout_seconds: .nan}\n",
		"multiple documents":    minimal + "---\n" + minimal,
		"empty second document": minimal + "---\n",
		"empty include":         minimal + "scope: {include: []}\n",
		"parent path":           minimal + "scope: {include: [../outside]}\n",
		"absolute path":         minimal + "scope: {include: [/outside]}\n",
		"Windows path":          minimal + "scope: {include: ['C:/outside']}\n",
		"backslash path":        minimal + "scope: {include: ['dir\\file.php']}\n",
		"bad glob":              minimal + "scope: {exclude: ['[']}\n",
		"traversing glob":       minimal + "scope: {exclude: ['../**']}\n",
		"empty base":            minimal + "scope: {base: ''}\n",
		"blank base":            minimal + "scope: {base: '  '}\n",
		"zero timeout":          minimal + "runner: {timeout_seconds: 0}\n",
		"string bool":           minimal + "tools: {debug: {enabled: 'false'}}\n",
		"YAML 1.1 bool":         minimal + "tools: {debug: {enabled: yes}}\n",
		"unknown fail policy":   minimal + "tools: {debug: {fail_on: errors}}\n",
		"empty functions":       minimal + "tools: {debug: {functions: []}}\n",
		"duplicate function":    minimal + "tools: {debug: {functions: [var_dump, VAR_DUMP]}}\n",
		"invalid function":      minimal + "tools: {debug: {functions: ['var_dump()']}}\n",
		"syntax level":          minimal + "tools: {syntax: {level: 5}}\n",
		"syntax zero timeout":   minimal + "tools: {syntax: {timeout_seconds: 0}}\n",
		"syntax bad policy":     minimal + "tools: {syntax: {fail_on: errors}}\n",
		"syntax bad extension":  minimal + "tools: {syntax: {extra_extensions: [php]}}\n",
		"syntax path extension": minimal + "tools: {syntax: {extra_extensions: ['../php']}}\n",
		"syntax duplicate ext":  minimal + "tools: {syntax: {extra_extensions: [.phtml, .phtml]}}\n",
		"syntax upper ext":      minimal + "tools: {syntax: {extra_extensions: [.PHTML]}}\n",
		"syntax bad include":    minimal + "tools: {syntax: {include: [../outside]}}\n",
		"syntax bad exclude":    minimal + "tools: {syntax: {exclude: ['[']}}\n",
		"yaml parser flags":     minimal + "tools: {yaml: {flags: 1}}\n",
		"yaml zero timeout":     minimal + "tools: {yaml: {timeout_seconds: 0}}\n",
		"yaml bad policy":       minimal + "tools: {yaml: {fail_on: errors}}\n",
		"yaml bad include":      minimal + "tools: {yaml: {include: [../outside]}}\n",
		"yaml bad exclude":      minimal + "tools: {yaml: {exclude: ['[']}}\n",
		"whitespace unknown":    minimal + "tools: {whitespace: {git_options: []}}\n",
		"whitespace bool":       minimal + "tools: {whitespace: {conflict_markers: 'false'}}\n",
		"whitespace timeout":    minimal + "tools: {whitespace: {timeout_seconds: 0}}\n",
		"whitespace policy":     minimal + "tools: {whitespace: {fail_on: errors}}\n",
		"whitespace include":    minimal + "tools: {whitespace: {include: [../outside]}}\n",
		"whitespace exclude":    minimal + "tools: {whitespace: {exclude: ['[']}}\n",
		"trailing exclude":      minimal + "tools: {whitespace: {trailing_whitespace_exclude: ['../**']}}\n",
		"marker exclude":        minimal + "tools: {whitespace: {conflict_markers_exclude: ['/outside']}}\n",
		"drupal unknown":        minimal + "tools: {drupal-config: {rules: []}}\n",
		"drupal timeout":        minimal + "tools: {drupal-config: {timeout_seconds: 0}}\n",
		"drupal policy":         minimal + "tools: {drupal-config: {fail_on: errors}}\n",
		"drupal include":        minimal + "tools: {drupal-config: {include: [../outside]}}\n",
		"drupal exclude":        minimal + "tools: {drupal-config: {exclude: ['[']}}\n",
		"drupal directories":    minimal + "tools: {drupal-config: {directories: [../outside]}}\n",
		"drupal root dir":       minimal + "tools: {drupal-config: {directories: ['./']}}\n",
		"drupal empty dirs":     minimal + "tools: {drupal-config: {directories: []}}\n",
		"drupal module path":    minimal + "tools: {drupal-config: {module_roots: [/outside]}}\n",
		"drupal module parent":  minimal + "tools: {drupal-config: {module_roots: [../outside]}}\n",
		"drupal duplicate":      minimal + "tools: {drupal-config: {forbidden_keys: [uuid, uuid]}}\n",
		"drupal bad key":        minimal + "tools: {drupal-config: {forbidden_keys: ['']}}\n",
		"phpcs unknown":         minimal + "tools: {phpcs: {args: []}}\n",
		"phpcs timeout":         minimal + "tools: {phpcs: {timeout_seconds: 0}}\n",
		"phpcs config path":     minimal + "tools: {phpcs: {config: ../outside.xml}}\n",
		"phpcs config null":     minimal + "tools: {phpcs: {config: null}}\n",
		"phpcs standards":       minimal + "tools: {phpcs: {standards: []}}\n",
		"phpcs standard path":   minimal + "tools: {phpcs: {standard_paths: [../outside]}}\n",
		"phpcs comma path":      minimal + "tools: {phpcs: {standard_paths: ['code,standard']}}\n",
		"phpcs extra input":     minimal + "tools: {phpcs: {extra_inputs: [/outside]}}\n",
		"phpcs severity":        minimal + "tools: {phpcs: {error_severity: 11}}\n",
		"phpcs sniff":           minimal + "tools: {phpcs: {sniffs: ['../file.php']}}\n",
		"phpcs target":          minimal + "tools: {phpcs: {compatibility: {test_version: '8.4-8.3'}}}\n",
		"phpcs target format":   minimal + "tools: {phpcs: {compatibility: {test_version: '8.4-'}}}\n",
		"phpstan unknown":       minimal + "tools: {phpstan: {paths: []}}\n",
		"phpstan timeout":       minimal + "tools: {phpstan: {timeout_seconds: 0}}\n",
		"phpstan level":         minimal + "tools: {phpstan: {level: 11}}\n",
		"phpstan level null":    minimal + "tools: {phpstan: {level: null}}\n",
		"phpstan memory":        minimal + "tools: {phpstan: {memory_limit_mb: 32}}\n",
		"phpstan config path":   minimal + "tools: {phpstan: {config: ../phpstan.neon}}\n",
		"phpstan baseline path": minimal + "tools: {phpstan: {baseline: /baseline.neon}}\n",
		"phpstan autoload":      minimal + "tools: {phpstan: {autoload: ''}}\n",
		"phpstan autoload path": minimal + "tools: {phpstan: {autoload: ../vendor/autoload.php}}\n",
		"phpstan include path":  minimal + "tools: {phpstan: {includes: [../outside.neon]}}\n",
		"phpstan bootstrap":     minimal + "tools: {phpstan: {bootstrap_files: [/bootstrap.php]}}\n",
		"phpstan scan":          minimal + "tools: {phpstan: {scan_directories: [../outside]}}\n",
		"phpstan stub":          minimal + "tools: {phpstan: {stub_files: [../stub.php]}}\n",
		"phpstan extension":     minimal + "tools: {phpstan: {extra_extensions: [inc]}}\n",
		"twig unknown":          minimal + "tools: {twig-cs-fixer: {rules: []}}\n",
		"twig timeout":          minimal + "tools: {twig-cs-fixer: {timeout_seconds: 0}}\n",
		"twig config path":      minimal + "tools: {twig-cs-fixer: {config: ../.twig-cs-fixer.php}}\n",
		"twig empty config":     minimal + "tools: {twig-cs-fixer: {config: ''}}\n",
		"twig extra input":      minimal + "tools: {twig-cs-fixer: {extra_inputs: [/outside]}}\n",
		"twig extension":        minimal + "tools: {twig-cs-fixer: {extra_extensions: [tpl]}}\n",
		"twig exclude":          minimal + "tools: {twig-cs-fixer: {exclude: ['[']}}\n",
		"validate unknown":      minimal + "tools: {composer-validate: {check_publish: true}}\n",
		"validate include":      minimal + "tools: {composer-validate: {include: [../outside]}}\n",
		"audit abandoned":       minimal + "tools: {composer-audit: {abandoned: fail}}\n",
		"audit stale":           minimal + "tools: {composer-audit: {max_age_hours: 48, max_stale_hours: 24}}\n",
		"audit ignore reason":   minimal + "tools: {composer-audit: {ignore: [{id: CVE-2026-1}]}}\n",
		"audit ignore id":       minimal + "tools: {composer-audit: {ignore: [{id: '../x', reason: test}]}}\n",
		"audit ignore date":     minimal + "tools: {composer-audit: {ignore: [{id: CVE-2026-1, reason: test, until: 2026-02-30}]}}\n",
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(input)); err == nil {
				t.Fatal("invalid input was accepted")
			}
		})
	}
}

func TestWhitespaceOverrides(t *testing.T) {
	result, err := Parse([]byte(minimal + `tools:
  whitespace:
    enabled: false
    trailing_whitespace: false
    conflict_markers: false
    trailing_whitespace_exclude: []
    conflict_markers_exclude: ['**/fixtures/**']
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := result.Config.Tools.Whitespace
	if cfg.Enabled || cfg.TrailingWhitespace || cfg.ConflictMarkers || len(cfg.TrailingWhitespaceExclude) != 0 || len(cfg.ConflictMarkersExclude) != 1 ||
		result.Provenance["tools.whitespace.trailing_whitespace_exclude"] != "project_config" {
		t.Fatalf("whitespace overrides lost: %+v", result)
	}
}

func TestDrupalConfigOverrides(t *testing.T) {
	result, err := Parse([]byte(minimal + `tools:
  drupal-config:
    enabled: false
    required: false
    fail_on: never
    forbidden_keys: []
    directories: [defaults]
    module_roots: ['.', packages/demo]
`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := result.Config.Tools.DrupalConfig
	if cfg.Enabled || cfg.Required || cfg.FailOn != "never" || len(cfg.ForbiddenKeys) != 0 ||
		!slices.Equal(cfg.Directories, []string{"defaults"}) || len(cfg.ModuleRoots) != 2 ||
		result.Provenance["tools.drupal-config.forbidden_keys"] != "project_config" {
		t.Fatalf("Drupal config overrides lost: %+v", result)
	}
}

func TestPHPVersionsCompareNumerically(t *testing.T) {
	input := strings.Replace(minimal, "max: \"8.4\"", "max: \"8.10\"", 1)
	if _, err := Parse([]byte(input)); err != nil {
		t.Fatalf("8.10 must be greater than 8.3: %v", err)
	}
}

func TestExportedSchemaAndExample(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "drupal.phpwarden.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(data); err != nil {
		t.Fatalf("example does not satisfy embedded schema: %v", err)
	}
	value := map[string]any{"version": 1, "profile": "drupal", "php": map[string]any{
		"runtime": 8.3, "target": map[string]any{"min": "8.3", "max": "8.3"},
	}}
	if err := schemas.ValidateConfig(value); err == nil {
		t.Fatal("schema must reject a numeric PHP version before Go decoding")
	}
}

func TestConcurrentParsingDoesNotShareMutableDefaults(t *testing.T) {
	for range 8 {
		t.Run("isolated", func(t *testing.T) {
			t.Parallel()
			first, err := Parse([]byte(minimal))
			if err != nil {
				t.Fatal(err)
			}
			first.Config.Tools.Debug.Functions[0] = "changed"
			second, err := Parse([]byte(minimal))
			if err != nil {
				t.Fatal(err)
			}
			if second.Config.Tools.Debug.Functions[0] != "var_dump" {
				t.Fatal("defaults leaked between configs")
			}
		})
	}
}
