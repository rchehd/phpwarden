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
		"unimplemented checker": minimal + "tools: {phpstan: {}}\n",
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
