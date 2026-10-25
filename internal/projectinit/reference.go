package projectinit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/rchehd/phpwarden/internal/config"
	"go.yaml.in/yaml/v3"
)

type setting struct {
	name  string
	value any
}

func configurationReference(cfg config.Config) ([]byte, error) {
	var output bytes.Buffer
	output.WriteString("\n# Uncomment optional examples to override defaults; enabled toggles select checks.\n# Paths start at the Git root: include uses paths, exclude accepts globs.\n\n")
	if cfg.ComposerRoot == "." {
		output.WriteString("# composer_root: \".\"\n\n")
	}
	output.WriteString("# framework:\n  # version: auto\n\n")
	roots := cfg.Scope.Include
	if len(roots) == 0 {
		roots = []string{"web/modules/custom"}
	}
	writeSection := func(indent string, fields []setting, active map[string]bool) error {
		comments := 0
		for _, field := range fields {
			if field.name == "include" && len(field.value.([]string)) == 0 {
				field.value = roots
			}
			value, err := inlineYAML(field.value)
			if err != nil {
				return err
			}
			if field.name == "concurrency" {
				value += "  # 0 = all, 1 = sequential, N = limit"
			}
			if active[field.name] {
				fmt.Fprintf(&output, "%s%s: %s\n", indent, field.name, value)
				comments = 0
				continue
			}
			if comments == 2 {
				output.WriteByte('\n')
				comments = 0
			}
			fmt.Fprintf(&output, "%s# %s: %s\n", indent, field.name, value)
			comments++
		}
		return nil
	}
	output.WriteString("scope:\n")
	fields := configurationFields(cfg.Scope)
	for i := range fields {
		switch fields[i].name {
		case "base":
			fields[i].value = "origin/HEAD"
		case "exclude":
			fields[i].value = []string{"**/generated/**", "**/tests/**"}
		}
	}
	if err := writeSection("  ", fields, map[string]bool{"mode": true}); err != nil {
		return nil, err
	}
	output.WriteString("\nrunner:\n")
	if err := writeSection("  ", configurationFields(cfg.Runner), map[string]bool{"timeout_seconds": true}); err != nil {
		return nil, err
	}
	output.WriteString("\n# Per-tool include/exclude only narrow scope; fail_on: error, warning, or never.\n# Config paths below are examples: uncomment only when those files exist.\ntools:\n")
	tools := configurationFields(cfg.Tools)
	sort.SliceStable(tools, func(i, j int) bool {
		return tools[i].name < tools[j].name
	})
	configs := map[string]string{
		"composer-require-checker": "composer-require-checker.json", "composer-unused": "composer-unused.php",
		"deptrac": "deptrac.yaml", "eslint": "eslint.config.mjs", "gitleaks": ".gitleaks.toml", "jscpd": ".jscpd.json",
		"phpcs": "phpcs.xml", "phpmd": "phpmd.xml", "phpstan": "phpstan.neon", "rector": "rector.php",
		"semgrep": ".semgrep.yaml", "stylelint": ".stylelintrc.json", "twig-cs-fixer": ".twig-cs-fixer.php",
	}
	for i, tool := range tools {
		if i > 0 {
			output.WriteByte('\n')
		}
		if tool.name == "rector" {
			output.WriteString("  # When enabled, use upstream sets for the current Drupal version unless overridden.\n")
		}
		fmt.Fprintf(&output, "  %s:\n", tool.name)
		fields := configurationFields(tool.value)
		for j := range fields {
			if fields[j].name == "config" && fields[j].value == "" {
				fields[j].value = configs[tool.name]
			}
			if tool.name == "gitleaks" && fields[j].name == "baseline" {
				fields[j].value = ".gitleaks-baseline.json"
			}
			if tool.name == "gitleaks" && fields[j].name == "ignore_file" {
				fields[j].value = ".gitleaksignore"
			}
		}
		if err := writeSection("    ", fields, map[string]bool{"enabled": true, "required": true, "fail_on": true}); err != nil {
			return nil, err
		}
	}
	return output.Bytes(), nil
}

func configurationFields(value any) []setting {
	object := reflect.ValueOf(value)
	fields := []setting{}
	for i := range object.NumField() {
		field, metadata := object.Field(i), object.Type().Field(i)
		if metadata.Anonymous {
			fields = append(fields, configurationFields(field.Interface())...)
			continue
		}
		name, _, _ := strings.Cut(metadata.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		if field.Kind() == reflect.Slice && field.IsNil() {
			field = reflect.MakeSlice(field.Type(), 0, 0)
		}
		item := field.Interface()
		if name == "level" && field.IsNil() {
			item = 3
		}
		fields = append(fields, setting{name, item})
	}
	return fields
}

func inlineYAML(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return "", err
	}
	var flow func(*yaml.Node)
	flow = func(node *yaml.Node) {
		node.Style = 0
		if node.Kind == yaml.MappingNode || node.Kind == yaml.SequenceNode {
			node.Style = yaml.FlowStyle
		}
		for _, child := range node.Content {
			flow(child)
		}
	}
	flow(&document)
	data, err = yaml.Marshal(&document)
	return strings.TrimSpace(string(data)), err
}
