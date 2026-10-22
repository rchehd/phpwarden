package drupal

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rchehd/phpwarden/internal/projectpath"
)

type reader struct {
	root   string
	inputs map[string]string
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
	value, err := jsonValue(decoder, 0)
	if err != nil {
		return nil, true, fmt.Errorf("%s: %w", name, err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, true, fmt.Errorf("%s: expected one JSON document", name)
	}
	return value, true, nil
}

func jsonValue(decoder *json.Decoder, depth int) (any, error) {
	if depth > 128 {
		return nil, fmt.Errorf("JSON nesting exceeds discovery limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	if token == json.Delim('{') {
		result := map[string]any{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("expected a JSON object key")
			}
			if _, exists := result[name]; exists {
				return nil, fmt.Errorf("duplicate JSON key %q", name)
			}
			value, err := jsonValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			result[name] = value
		}
		_, err := decoder.Token()
		return result, err
	}
	if token == json.Delim('[') {
		result := []any{}
		for decoder.More() {
			value, err := jsonValue(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			result = append(result, value)
		}
		_, err := decoder.Token()
		return result, err
	}
	if _, isDelimiter := token.(json.Delim); isDelimiter {
		return nil, fmt.Errorf("unexpected JSON delimiter")
	}
	return token, nil
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
