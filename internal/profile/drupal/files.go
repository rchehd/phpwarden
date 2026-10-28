package drupal

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rchehd/phpwarden/internal/inputsource"
	"github.com/rchehd/phpwarden/internal/projectpath"
)

type reader struct {
	root     string
	inputs   map[string]string
	warnings *[]string
}

func (reader reader) probe(name string) (os.FileInfo, error) {
	if err := projectpath.Validate(name); err != nil {
		return nil, err
	}
	current := reader.root
	for _, part := range strings.Split(name, "/") {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			reader.inputs[name] = "missing"
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("discovery path %q must not traverse symlinks", name)
		}
	}
	info, err := os.Stat(current)
	if err != nil {
		return nil, err
	}
	reader.inputs[name] = "file"
	if info.IsDir() {
		reader.inputs[name] = "directory"
	}
	return info, nil
}

func (reader reader) directory(name string) (bool, error) {
	info, err := reader.probe(name)
	if err != nil || info == nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("discovery path %q must be a directory", name)
	}
	return true, nil
}

func (reader reader) object(name string) (map[string]any, error) {
	value, exists, err := reader.document(name)
	if err != nil || !exists {
		return nil, err
	}
	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected a JSON object", name)
	}
	return object, nil
}

func (reader reader) document(name string) (any, bool, error) {
	info, err := reader.probe(name)
	if err != nil || info == nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, true, fmt.Errorf("discovery input %q must be a regular JSON file", name)
	}
	data, err := os.ReadFile(filepath.Join(reader.root, filepath.FromSlash(name)))
	if err != nil {
		return nil, true, err
	}
	reader.inputs[name] = fmt.Sprintf("sha256:%x", sha256.Sum256(data))
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	duplicates := []*inputsource.Error{}
	value, err := jsonValue(decoder, data, 0, "$", &duplicates)
	if err != nil {
		return nil, true, inputsource.WithFile(err, name)
	}
	for _, duplicate := range duplicates {
		*reader.warnings = append(*reader.warnings, inputsource.WithFile(duplicate, name).Error()+"; the last value is used, as in Composer")
	}
	offset := inputsource.TokenStart(data, decoder.InputOffset())
	if _, err := decoder.Token(); err != io.EOF {
		return nil, true, &inputsource.Error{File: name, Position: inputsource.At(data, offset), Path: "$", Message: "expected one JSON document"}
	}
	return value, true, nil
}

func jsonValue(decoder *json.Decoder, data []byte, depth int, parent string, duplicates *[]*inputsource.Error) (any, error) {
	offset := inputsource.TokenStart(data, decoder.InputOffset())
	if depth > 128 {
		return nil, &inputsource.Error{Position: inputsource.At(data, offset), Path: parent, Message: "JSON nesting exceeds discovery limit"}
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, jsonSource(data, decoder, parent, err)
	}
	if token == json.Delim('{') {
		result := map[string]any{}
		keys := map[string]int64{}
		for decoder.More() {
			keyOffset := inputsource.TokenStart(data, decoder.InputOffset())
			key, err := decoder.Token()
			if err != nil {
				return nil, jsonSource(data, decoder, parent, err)
			}
			name, ok := key.(string)
			if !ok {
				return nil, &inputsource.Error{Position: inputsource.At(data, keyOffset), Path: parent, Message: "expected a JSON object key"}
			}
			child := inputsource.Member(parent, name)
			if previous, exists := keys[name]; exists {
				first := inputsource.At(data, previous)
				*duplicates = append(*duplicates, &inputsource.Error{Position: inputsource.At(data, keyOffset), Path: child,
					Previous: &first, Message: fmt.Sprintf("duplicate JSON key %q", name)})
			} else {
				keys[name] = keyOffset
			}
			value, err := jsonValue(decoder, data, depth+1, child, duplicates)
			if err != nil {
				return nil, err
			}
			result[name] = value
		}
		_, err := decoder.Token()
		if err != nil {
			return nil, jsonSource(data, decoder, parent, err)
		}
		return result, nil
	}
	if token == json.Delim('[') {
		result := []any{}
		for decoder.More() {
			value, err := jsonValue(decoder, data, depth+1, fmt.Sprintf("%s[%d]", parent, len(result)), duplicates)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		_, err := decoder.Token()
		if err != nil {
			return nil, jsonSource(data, decoder, parent, err)
		}
		return result, nil
	}
	if _, isDelimiter := token.(json.Delim); isDelimiter {
		return nil, &inputsource.Error{Position: inputsource.At(data, offset), Path: parent, Message: "unexpected JSON delimiter"}
	}
	return token, nil
}

func jsonSource(data []byte, decoder *json.Decoder, path string, err error) error {
	offset := inputsource.TokenStart(data, decoder.InputOffset())
	var syntax *json.SyntaxError
	if errors.As(err, &syntax) {
		var raw json.RawMessage
		if absolute := json.Unmarshal(data, &raw); absolute != nil {
			errors.As(absolute, &syntax)
		}
		offset = syntax.Offset - 1
	} else if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		offset = int64(len(data))
	}
	return &inputsource.Error{Position: inputsource.At(data, offset), Path: path, Message: err.Error(), Err: err}
}

func objectAt(object map[string]any, keys ...string) (map[string]any, error) {
	for _, key := range keys {
		value, exists := object[key]
		if !exists {
			return nil, nil
		}
		var ok bool
		object, ok = value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Composer %s must be an object", strings.Join(keys, "."))
		}
	}
	return object, nil
}
