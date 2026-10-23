package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/schemas"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	Version   int       `json:"version"`
	Profile   string    `json:"profile"`
	Framework Framework `json:"framework"`
	PHP       PHP       `json:"php"`
	Scope     Scope     `json:"scope"`
	Runner    Runner    `json:"runner"`
	Tools     Tools     `json:"tools"`
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
	TimeoutSeconds int `json:"timeout_seconds"`
}

type Tools struct {
	Debug  Debug  `json:"debug"`
	Syntax Syntax `json:"syntax"`
}

type Syntax struct {
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
		return Effective{}, fmt.Errorf("read YAML: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		if err != nil {
			return Effective{}, fmt.Errorf("read YAML: %w", err)
		}
		return Effective{}, fmt.Errorf("expected one YAML document")
	}
	if len(document.Content) != 1 {
		return Effective{}, fmt.Errorf("expected a YAML mapping")
	}
	root := document.Content[0]
	if root.Kind == yaml.MappingNode {
		for i := 0; i < len(root.Content); i += 2 {
			if root.Content[i].Value == "version" && root.Content[i+1].ShortTag() != "!!int" {
				return Effective{}, fmt.Errorf("version must be an integer")
			}
		}
	}
	value, err := jsonValue(document.Content[0], "$")
	if err != nil {
		return Effective{}, err
	}
	if err := schemas.ValidateConfig(value); err != nil {
		return Effective{}, fmt.Errorf("invalid configuration: %w", err)
	}
	input := value.(map[string]any)
	defaults, ok := profile.Lookup(input["profile"].(string))
	if !ok {
		return Effective{}, fmt.Errorf("unsupported profile")
	}
	result := Effective{
		Config: Config{
			Framework: Framework{Version: "auto"},
			Scope:     Scope{Mode: "diff", Exclude: []string{}},
			Runner:    Runner{TimeoutSeconds: 300},
			Tools: Tools{Syntax: Syntax{Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120,
				Exclude: []string{}, ExtraExtensions: []string{}}, Debug: Debug{
				Enabled: true, Required: true, FailOn: "error", TimeoutSeconds: 120,
				Exclude: []string{}, Functions: defaults.DebugFunctions,
			}},
		},
		Provenance: map[string]string{
			"framework.version": "core_default", "scope.mode": "core_default",
			"scope.exclude": "core_default", "scope.whole_files": "core_default",
			"runner.timeout_seconds": "core_default", "tools.debug.enabled": "profile_default",
			"tools.debug.required": "core_default", "tools.debug.fail_on": "core_default",
			"tools.debug.timeout_seconds": "core_default", "tools.debug.exclude": "core_default",
			"tools.debug.functions": "profile_default",
			"tools.syntax.enabled":  "profile_default", "tools.syntax.required": "core_default",
			"tools.syntax.fail_on": "core_default", "tools.syntax.timeout_seconds": "core_default",
			"tools.syntax.exclude": "core_default", "tools.syntax.extra_extensions": "core_default",
		},
		Pending: []string{},
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
	if err := validateValues(result.Config); err != nil {
		return Effective{}, err
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
		return nil, fmt.Errorf("%s: YAML anchors and aliases are not supported", location)
	}
	switch node.Kind {
	case yaml.MappingNode:
		if node.ShortTag() != "!!map" {
			break
		}
		result := make(map[string]any)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode || key.ShortTag() != "!!str" || key.Value == "<<" || key.Anchor != "" {
				return nil, fmt.Errorf("%s: expected string keys without merge keys", location)
			}
			if _, exists := result[key.Value]; exists {
				return nil, fmt.Errorf("%s: duplicate key %q", location, key.Value)
			}
			value, err := jsonValue(node.Content[i+1], location+"."+key.Value)
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
				return nil, fmt.Errorf("%s: %w", location, err)
			}
			if number, ok := value.(float64); ok && (math.IsNaN(number) || math.IsInf(number, 0)) {
				return nil, fmt.Errorf("%s: numbers must be finite", location)
			}
			return value, nil
		}
	}
	return nil, fmt.Errorf("%s: unsupported YAML type %q", location, node.Tag)
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
	for _, values := range [][]string{cfg.Scope.Include, cfg.Tools.Debug.Include, cfg.Tools.Syntax.Include} {
		for _, value := range values {
			if err := projectpath.Validate(value); err != nil {
				return err
			}
		}
	}
	for _, values := range [][]string{cfg.Scope.Exclude, cfg.Tools.Debug.Exclude, cfg.Tools.Syntax.Exclude} {
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
