package drupalconfig

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/scope"
	"go.yaml.in/yaml/v3"
)

func Run(ctx context.Context, root string, selection scope.Selection, cfg config.DrupalConfig) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "drupal-config", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn},
		Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	if !cfg.Enabled || len(selection.Files) == 0 || len(cfg.ForbiddenKeys) == 0 {
		reason := "no_files"
		if !cfg.Enabled {
			reason = "disabled"
		} else if len(cfg.ForbiddenKeys) == 0 {
			reason = "rules_disabled"
		}
		result.Check.Status, result.Check.Reason = "skipped", &reason
		return result
	}
	fail := func(code, message string) checker.Outcome {
		result.Check.Status, result.Check.Reason = "error", &code
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Message: message, Checker: new("drupal-config")})
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	for _, filename := range selection.Files {
		if ctx.Err() != nil {
			return fail("checker_timeout", "Drupal config checker did not complete within its execution context")
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(filename)))
		if err != nil {
			return fail("input_read", fmt.Sprintf("Cannot read %q: %v", filename, err))
		}
		node, values, err := parse(data)
		if ctx.Err() != nil {
			return fail("checker_timeout", "Drupal config checker did not complete within its execution context")
		}
		if err != nil {
			return fail("yaml_invalid", fmt.Sprintf("Cannot inspect module configuration %q: %v", filename, err))
		}
		for _, key := range cfg.ForbiddenKeys {
			present := false
			switch mapping := values.(type) {
			case map[string]any:
				_, present = mapping[key]
			case map[any]any:
				_, present = mapping[key]
			}
			if !present {
				continue
			}
			position := keyPosition(node, key)
			result.Findings = append(result.Findings, report.Finding{Checker: "drupal-config", Rule: "drupal.config.forbidden_key", Kind: "code", Severity: "error",
				Message: fmt.Sprintf("Remove forbidden top-level key %q from module configuration", key),
				Path:    new(filename), Line: new(position.Line), Column: new(position.Column)})
		}
	}
	if len(result.Findings) > 0 && cfg.FailOn != "never" {
		result.Check.Status = "failed"
	}
	return result
}

func parse(data []byte) (*yaml.Node, any, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if err == io.EOF {
			return &document, nil, nil
		}
		return nil, nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, nil, fmt.Errorf("expected one YAML document")
	}
	if !withinDepth(&document, 0) {
		return nil, nil, fmt.Errorf("YAML nesting exceeds the limit of 128")
	}
	var values any
	if err := document.Decode(&values); err != nil {
		return nil, nil, err
	}
	return document.Content[0], values, nil
}

func withinDepth(node *yaml.Node, depth int) bool {
	if depth > 128 {
		return false
	}
	for _, child := range node.Content {
		if !withinDepth(child, depth+1) {
			return false
		}
	}
	return true
}

func keyPosition(node *yaml.Node, name string) *yaml.Node {
	position := node
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			resolved := key
			for resolved.Kind == yaml.AliasNode {
				resolved = resolved.Alias
			}
			if resolved.Kind == yaml.ScalarNode && resolved.Tag != "!!merge" && resolved.Value == name {
				return key
			}
			if key.Tag == "!!merge" {
				position = key
			}
		}
	}
	return position
}
