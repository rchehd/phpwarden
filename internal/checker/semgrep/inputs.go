package semgrep

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/edit"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/scope"
	"go.yaml.in/yaml/v3"
)

const RulesVersion = "1.0.0"

//go:embed rules.yaml
var defaultRules []byte

var ruleID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,191}$`)
var fingerprintID = regexp.MustCompile(`^[0-9a-f]{64}$`)
var ConfigCandidates = []string{".semgrep.yml", ".semgrep.yaml"}

type rule struct {
	ID, Message, Severity string
	Value                 map[string]any
}

type plan struct {
	Inputs   map[string]string
	Rules    []rule
	Baseline map[string]bool
}

func Select(selected scope.Selection, cfg config.Semgrep) scope.Selection {
	defaults, _ := profile.Lookup("drupal")
	extensions := slices.Clone(defaults.PHPExtensions)
	for _, extension := range cfg.ExtraExtensions {
		extensions = append(extensions, strings.ToLower(extension))
	}
	return selected.Filter(cfg.Include, cfg.Exclude, func(name string) bool { return slices.Contains(extensions, strings.ToLower(path.Ext(name))) })
}

func InputRequired(cfg config.Config, selected scope.Selection, only []string) bool {
	return checker.Enabled("semgrep", cfg.Tools.Semgrep.Enabled, only) && len(Select(selected, cfg.Tools.Semgrep).Files) > 0
}

func ContextHash(root, composerRoot string, cfg config.Semgrep) string {
	prepared, err := prepare(root, composerRoot, cfg)
	message := ""
	if err != nil {
		message = err.Error()
	}
	data, _ := json.Marshal(struct {
		Inputs map[string]string
		Error  string
	}{prepared.Inputs, message})
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func prepare(directory, composerRoot string, cfg config.Semgrep) (plan, error) {
	result := plan{Inputs: map[string]string{}, Rules: []rule{}, Baseline: map[string]bool{}}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return result, err
	}
	defer root.Close()
	read := func(name string) ([]byte, error) {
		result.Inputs[name] = "unreadable"
		data, _, err := edit.Read(root, name)
		if err != nil {
			if os.IsNotExist(err) {
				result.Inputs[name] = "missing"
			}
			return nil, err
		}
		result.Inputs[name] = edit.Hash(data)
		if len(data) > 4<<20 || !utf8.Valid(data) {
			return nil, fmt.Errorf("Semgrep config inputs must be UTF-8 and at most 4 MiB each")
		}
		return data, nil
	}
	configFile := cfg.Config
	if configFile == "" && cfg.DiscoverConfig {
		for _, directory := range slices.Compact([]string{".", composerRoot}) {
			for _, candidate := range ConfigCandidates {
				name := path.Join(directory, candidate)
				if _, err := read(name); err == nil {
					configFile = name
					break
				} else if !os.IsNotExist(err) {
					return result, fmt.Errorf("Semgrep config %q is unreadable or unsafe", name)
				}
			}
			if configFile != "" {
				break
			}
		}
	}
	loaded := []rule{}
	if cfg.UseDefaults {
		loaded, err = parseRules(defaultRules)
		if err != nil {
			return result, err
		}
	}
	seenFiles := map[string]bool{}
	for _, name := range append(append(slices.Clone(cfg.Rulesets), configFile), cfg.ExtraInputs...) {
		if name == "" {
			continue
		}
		isRulesFile := name == configFile || slices.Contains(cfg.Rulesets, name)
		name = path.Clean(name)
		if seenFiles[name] {
			continue
		}
		seenFiles[name] = true
		data, err := read(name)
		if err != nil {
			return result, fmt.Errorf("Semgrep ruleset or extra input %q is missing, unreadable or unsafe", name)
		}
		if isRulesFile {
			parsed, err := parseRules(data)
			if err != nil {
				return result, err
			}
			loaded = append(loaded, parsed...)
		}
	}
	known := map[string]bool{}
	for _, rule := range loaded {
		if known[rule.ID] {
			return result, fmt.Errorf("duplicate Semgrep rule ID %q", rule.ID)
		}
		known[rule.ID] = true
	}
	for _, id := range append(slices.Clone(cfg.Rules), cfg.ExcludeRules...) {
		if !known[id] {
			return result, fmt.Errorf("unknown Semgrep rule ID %q", id)
		}
	}
	for _, rule := range loaded {
		if (len(cfg.Rules) == 0 || slices.Contains(cfg.Rules, rule.ID)) && !slices.Contains(cfg.ExcludeRules, rule.ID) && slices.Contains(cfg.Severities, rule.Severity) {
			result.Rules = append(result.Rules, rule)
		}
	}
	if cfg.Baseline != "" {
		data, err := read(cfg.Baseline)
		if err != nil {
			return result, fmt.Errorf("Semgrep baseline is missing, unreadable or unsafe")
		}
		var baseline struct {
			Version      int      `json:"version"`
			Fingerprints []string `json:"fingerprints"`
		}
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&baseline); err != nil {
			return result, fmt.Errorf("invalid Semgrep baseline")
		}
		if _, err := decoder.Token(); err != io.EOF || baseline.Version != 1 || baseline.Fingerprints == nil {
			return result, fmt.Errorf("invalid Semgrep baseline")
		}
		for _, id := range baseline.Fingerprints {
			if !fingerprintID.MatchString(id) || result.Baseline[id] {
				return result, fmt.Errorf("invalid or duplicate Semgrep baseline fingerprint")
			}
			result.Baseline[id] = true
		}
	}
	return result, nil
}

func parseRules(data []byte) ([]rule, error) {
	var document struct {
		Rules []map[string]any `yaml:"rules"`
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("invalid local Semgrep YAML ruleset")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF || len(document.Rules) == 0 {
		return nil, fmt.Errorf("Semgrep requires one nonempty local rules document")
	}
	result := []rule{}
	for _, value := range document.Rules {
		id, _ := value["id"].(string)
		message, _ := value["message"].(string)
		severity, _ := value["severity"].(string)
		languages, _ := value["languages"].([]any)
		mode, _ := value["mode"].(string)
		if !ruleID.MatchString(id) || strings.TrimSpace(message) == "" || !slices.Contains([]string{"ERROR", "WARNING", "INFO"}, severity) || len(languages) != 1 || languages[0] != "php" || mode != "" && mode != "search" && mode != "taint" {
			return nil, fmt.Errorf("Semgrep supports local PHP search/taint rules with ID, message and ERROR/WARNING/INFO severity")
		}
		if err := validateRuleKeys(value); err != nil {
			return nil, err
		}
		result = append(result, rule{ID: id, Message: message, Severity: severity, Value: value})
	}
	return result, nil
}

func validateRuleKeys(value any) error {
	switch value := value.(type) {
	case map[string]any:
		for key, item := range value {
			if slices.Contains([]string{"validators", "validator", "pattern-where-python", "pattern-propagators", "interfile", "interprocedural", "join", "extract", "paths", "fix", "fix-regex", "r2c-internal-project-depends-on"}, key) {
				return fmt.Errorf("Semgrep rules cannot use validators, executable/remote features, Pro analysis, fixes or native paths; use checker include/exclude")
			}
			if err := validateRuleKeys(item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range value {
			if err := validateRuleKeys(item); err != nil {
				return err
			}
		}
	case map[any]any:
		return fmt.Errorf("Semgrep rule keys must be strings")
	}
	return nil
}
