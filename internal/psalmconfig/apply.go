package psalmconfig

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/psalmanalysis"
	"github.com/rchehd/phpwarden/internal/runner"
)

//go:embed validate.php
var validator []byte

func (native Native) Apply(ctx context.Context, root, bundle, directory, php string, memory int, prepared psalmanalysis.Prepared, process runner.Process) (psalmanalysis.Prepared, error) {
	if native.Config == "" && native.Baseline == "" {
		return prepared, nil
	}
	if err := psalmanalysis.HashesUnchanged(root, native.Inputs); err != nil {
		return prepared, err
	}
	data, err := os.ReadFile(prepared.Config)
	if err != nil {
		return prepared, err
	}
	base, err := parse(data)
	if err != nil || base.Name != "psalm" || base.attr("resolveFromConfigFile") != "true" {
		return prepared, fmt.Errorf("invalid profile Psalm configuration")
	}
	var stubs *element
	for _, node := range base.Children {
		if node.Name == "stubs" {
			stubs = node
		}
	}
	if stubs == nil && len(native.stubs) > 0 {
		stubs = &element{Name: "stubs"}
		base.Children = append(base.Children, stubs)
	}
	projectStubs := []*element{}
	for _, stub := range native.stubs {
		cloned := clone(stub)
		cloned.set("name", filepath.Join(root, filepath.FromSlash(stub.attr("name"))))
		projectStubs = append(projectStubs, cloned)
		prepared.SyntaxFiles = append(prepared.SyntaxFiles, stub.attr("name"))
	}
	if stubs != nil {
		stubs.Children = append(projectStubs, stubs.Children...)
	}
	slices.Sort(prepared.SyntaxFiles)
	prepared.SyntaxFiles = slices.Compact(prepared.SyntaxFiles)
	write := func(name string, node *element) (string, error) {
		data, err := encode(node)
		if err != nil {
			return "", err
		}
		filename := filepath.Join(filepath.Dir(prepared.Config), name)
		return filename, os.WriteFile(filename, data, 0o600)
	}
	for _, name := range slices.Sorted(maps.Keys(native.attributes)) {
		base.set(name, native.attributes[name])
	}
	readiness, err := write("psalm-readiness.xml", base)
	if err != nil {
		return prepared, err
	}
	if native.handlers != nil {
		handlers := clone(native.handlers)
		var absolute func(*element)
		absolute = func(node *element) {
			if node.Name == "file" || node.Name == "directory" {
				node.set("name", filepath.Join(root, filepath.FromSlash(node.attr("name"))))
			}
			for _, child := range node.Children {
				absolute(child)
			}
		}
		absolute(handlers)
		base.Children = append(base.Children, handlers)
	}
	if native.baseline != nil {
		baseline := clone(native.baseline)
		for _, file := range baseline.Children {
			filename := file.attr("src")
			if strings.HasPrefix(filename, "@generated/") {
				filename = filepath.Join(filepath.Dir(prepared.Config), strings.TrimPrefix(filename, "@generated/"))
			} else {
				filename = filepath.Join(root, filepath.FromSlash(filename))
			}
			relative, err := filepath.Rel(filepath.Dir(prepared.Config), filename)
			if err != nil {
				return prepared, err
			}
			file.set("src", filepath.ToSlash(relative))
		}
		filename, err := write("psalm-baseline.xml", baseline)
		if err != nil {
			return prepared, err
		}
		base.set("errorBaseline", filename)
	}
	taint, err := write("psalm-native.xml", base)
	if err != nil {
		return prepared, err
	}
	script := filepath.Join(directory, "psalm-validate.php")
	if err := os.WriteFile(script, validator, 0o600); err != nil {
		return prepared, err
	}
	input, _ := json.Marshal([]string{readiness, taint})
	output := process.Run(ctx, runner.Command{Name: php, Args: append(psalmanalysis.PHPArgs(memory), script, filepath.Join(bundle, "vimeo/psalm/config.xsd")), Input: input, Directory: directory, Env: psalmanalysis.Environment(directory, bundle)})
	var validation struct {
		Version int      `json:"version"`
		Checked int      `json:"checked"`
		Errors  []string `json:"errors"`
	}
	decodeErr := json.Unmarshal(output.Stdout, &validation)
	if output.Err != nil || output.Unavailable || output.TimedOut || ctx.Err() != nil || output.ExitCode != 0 || len(output.Stderr) > 0 || decodeErr != nil || validation.Version != 1 || validation.Checked != 2 || validation.Errors == nil || len(validation.Errors) > 0 {
		message := "Native Psalm configuration could not be validated against the pinned schema"
		if len(validation.Errors) > 0 {
			message += ": " + strings.Join(validation.Errors, "; ")
		}
		return prepared, fmt.Errorf("%s", message)
	}
	prepared.Config, prepared.ReadinessConfig = taint, readiness
	prepared.Inputs = maps.Clone(prepared.Inputs)
	for name, hash := range native.Inputs {
		if previous, exists := prepared.Inputs[name]; exists && previous != hash {
			return prepared, fmt.Errorf("native Psalm input changed during profile preparation")
		}
		prepared.Inputs[name] = hash
	}
	return prepared, nil
}
