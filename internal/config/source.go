package config

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/rchehd/phpwarden/internal/inputsource"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/santhosh-tekuri/jsonschema/v6/kind"
	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

func yamlSource(node *yaml.Node, path, text string) *inputsource.Error {
	return &inputsource.Error{Position: inputsource.Position{Line: node.Line, Column: node.Column}, Path: path, Message: text}
}

var yamlLine = regexp.MustCompile(`\bline ([0-9]+)\b`)

func yamlSyntax(err error) error {
	position := inputsource.Position{}
	if match := yamlLine.FindStringSubmatch(err.Error()); len(match) == 2 {
		position.Line, _ = strconv.Atoi(match[1])
	}
	return &inputsource.Error{Position: position, Message: "read YAML: " + err.Error(), Err: err}
}

func schemaSource(root *yaml.Node, err error) error {
	var validation *jsonschema.ValidationError
	if !errors.As(err, &validation) {
		return err
	}
	for len(validation.Causes) > 0 {
		validation = validation.Causes[0]
	}
	parts := append([]string{}, validation.InstanceLocation...)
	key := false
	switch issue := validation.ErrorKind.(type) {
	case *kind.AdditionalProperties:
		if len(issue.Properties) > 0 {
			parts, key = append(parts, issue.Properties[0]), true
		}
	case *kind.Required:
		if len(issue.Missing) > 0 {
			parts = append(parts, issue.Missing[0])
		}
	}
	node, path := yamlLocation(root, parts, key)
	text := validation.ErrorKind.LocalizedString(message.NewPrinter(language.English))
	if _, unknown := validation.ErrorKind.(*kind.AdditionalProperties); unknown {
		text += "; remove or rename the unknown field"
	}
	source := yamlSource(node, path, "invalid configuration: "+text)
	source.Err = err
	return source
}

func yamlLocation(root *yaml.Node, parts []string, key bool) (*yaml.Node, string) {
	node, path := root, "$"
	for n, part := range parts {
		if node.Kind == yaml.SequenceNode {
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(node.Content) {
				return node, path
			}
			node, path = node.Content[index], fmt.Sprintf("%s[%d]", path, index)
			continue
		}
		path = inputsource.Member(path, part)
		if node.Kind != yaml.MappingNode {
			return node, path
		}
		found := false
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == part {
				if key && n == len(parts)-1 {
					return node.Content[i], path
				}
				node, found = node.Content[i+1], true
				break
			}
		}
		if !found {
			return node, path
		}
	}
	return node, path
}

func semanticSource(root *yaml.Node, err error) error {
	text := err.Error()
	field := strings.TrimSuffix(strings.Fields(text)[0], ":")
	parts := strings.Split(field, ".")
	for i := 0; i < len(root.Content); i += 2 {
		if root.Kind == yaml.MappingNode && root.Content[i].Value == parts[0] {
			node, path := yamlLocation(root, parts, false)
			return yamlSource(node, path, text)
		}
	}
	type candidate struct {
		node *yaml.Node
		path string
	}
	var matches []candidate
	var walk func(*yaml.Node, string)
	walk = func(node *yaml.Node, path string) {
		switch node.Kind {
		case yaml.MappingNode:
			for i := 0; i < len(node.Content); i += 2 {
				walk(node.Content[i+1], inputsource.Member(path, node.Content[i].Value))
			}
		case yaml.SequenceNode:
			for i, child := range node.Content {
				walk(child, fmt.Sprintf("%s[%d]", path, i))
			}
		case yaml.ScalarNode:
			if node.ShortTag() == "!!str" && node.Value != "" && strings.Contains(text, strconv.Quote(node.Value)) {
				matches = append(matches, candidate{node, path})
			}
		}
	}
	walk(root, "$")
	if len(matches) == 1 {
		return yamlSource(matches[0].node, matches[0].path, text)
	}
	return err
}
