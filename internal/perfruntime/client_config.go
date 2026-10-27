package perfruntime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

func strictClientJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := clientJSONValue(decoder, 0); err != nil {
		return fmt.Errorf("invalid or ambiguous JSON configuration")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON configuration")
	}
	decoder = json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func clientJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("JSON nesting exceeds limit")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	keys := map[string]bool{}
	for decoder.More() {
		if delim == '{' {
			token, err := decoder.Token()
			key, ok := token.(string)
			if err != nil || !ok || keys[key] {
				return fmt.Errorf("duplicate or invalid JSON key")
			}
			keys[key] = true
		}
		if err := clientJSONValue(decoder, depth+1); err != nil {
			return err
		}
	}
	_, err = decoder.Token()
	return err
}

func entryHash(value any) string {
	data, _ := json.Marshal(value)
	return digest(data)
}

func serverName(id string) string {
	if id == "codex" {
		return "phpwarden_performance"
	}
	return "phpwarden-performance"
}

func planClient(id, project, path string, before []byte, saved clientRecord, known bool, entry map[string]any, connect bool) ([]byte, clientRecord, error) {
	record := saved
	record.Path = path
	if connect && before == nil {
		record.CreatedFile = true
	}
	if connect {
		for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
			if _, err := os.Lstat(parent); err == nil {
				break
			}
			if !slices.Contains(record.CreatedDirs, parent) {
				record.CreatedDirs = append(record.CreatedDirs, parent)
			}
		}
	}
	root := map[string]any{}
	if before != nil {
		var err error
		if id == "codex" {
			err = toml.Unmarshal(before, &root)
		} else {
			err = strictClientJSON(before, &root)
		}
		if err != nil || root == nil {
			return nil, record, fmt.Errorf("configuration must be an unambiguous %s object", map[bool]string{true: "TOML", false: "JSON"}[id == "codex"])
		}
	}
	keys := []string{"mcpServers"}
	if id == "codex" {
		keys = []string{"mcp_servers"}
	} else if id == "claude" {
		keys = []string{"projects", project, "mcpServers"}
	}
	parent := root
	parents := []map[string]any{root}
	for depth, key := range keys {
		child, exists := parent[key]
		if exists {
			object, ok := child.(map[string]any)
			if !ok {
				return nil, record, fmt.Errorf("MCP scope must be an object")
			}
			parent = object
		} else {
			object := map[string]any{}
			parent[key] = object
			parent = object
			if connect && !slices.Contains(record.CreatedMaps, depth) {
				record.CreatedMaps = append(record.CreatedMaps, depth)
			}
		}
		parents = append(parents, parent)
	}
	name := serverName(id)
	current, exists := parent[name]
	if exists && known && saved.Owned && !slices.Contains(saved.Hashes, entryHash(current)) {
		return nil, record, fmt.Errorf("owned MCP entry was edited; preserve or restore it before continuing")
	}
	base := bytes.Clone(before)
	if id == "codex" && known && saved.Owned {
		matched := false
		for _, block := range saved.Blocks {
			if bytes.Count(base, []byte(block)) == 1 && block != "" {
				if matched {
					return nil, record, fmt.Errorf("multiple owned MCP blocks")
				}
				base = bytes.Replace(base, []byte(block), nil, 1)
				matched = true
			}
		}
		if exists && !matched || !exists && bytes.Contains(base, []byte("# >>> phpwarden-performance >>>")) {
			return nil, record, fmt.Errorf("owned MCP block was edited")
		}
		check := map[string]any{}
		if err := toml.Unmarshal(base, &check); err != nil {
			return nil, record, fmt.Errorf("removing the owned MCP block would invalidate the config")
		}
		if table, ok := check["mcp_servers"].(map[string]any); ok {
			if _, remains := table[name]; remains {
				return nil, record, fmt.Errorf("owned MCP table has additional settings outside its block")
			}
		}
	}
	if !connect {
		if !saved.Owned {
			return before, record, nil
		}
		if id == "codex" {
			if record.CreatedFile && len(base) == 0 {
				return nil, record, nil
			}
			return base, record, nil
		}
		if !exists {
			return before, record, nil
		}
		delete(parent, name)
		for depth := len(keys) - 1; depth >= 0; depth-- {
			if slices.Contains(saved.CreatedMaps, depth) && len(parents[depth+1]) == 0 {
				delete(parents[depth], keys[depth])
			}
		}
		if record.CreatedFile && len(root) == 0 {
			return nil, record, nil
		}
		data, err := json.MarshalIndent(root, "", "  ")
		return append(data, '\n'), record, err
	}
	wanted := map[string]any{}
	for key, value := range entry {
		wanted[key] = value
	}
	if id == "claude" {
		wanted["type"] = "stdio"
	} else if id == "codex" {
		wanted["startup_timeout_sec"] = 10
		wanted["tool_timeout_sec"] = 20
	}
	hash := entryHash(wanted)
	if exists && (!known || !saved.Owned) {
		if entryHash(current) != hash {
			return nil, record, fmt.Errorf("MCP server name is already used by another configuration")
		}
		record.Owned = false
		return before, record, nil
	}
	record.Owned = true
	record.Hashes = []string{hash}
	if id == "codex" {
		padding := ""
		if len(base) > 0 {
			padding = "\n"
			if base[len(base)-1] != '\n' {
				padding = "\n\n"
			}
		}
		body, err := toml.Marshal(wanted)
		if err != nil {
			return nil, record, err
		}
		block := padding + "# >>> phpwarden-performance >>>\n[mcp_servers." + name + "]\n" + string(body) + "# <<< phpwarden-performance <<<\n"
		record.Blocks = []string{block}
		data := append(base, block...)
		check := map[string]any{}
		if err := toml.Unmarshal(data, &check); err != nil {
			return nil, record, fmt.Errorf("cannot append an MCP table to this TOML layout; use a standard mcp_servers table")
		}
		return data, record, nil
	}
	if exists && entryHash(current) == hash {
		return before, record, nil
	}
	parent[name] = wanted
	data, err := json.MarshalIndent(root, "", "  ")
	return append(data, '\n'), record, err
}

func validateClientRecord(record clientRecord, path, project, id string) error {
	if len(record.Hashes) > 16 || len(record.Blocks) > 16 {
		return fmt.Errorf("too many pending MCP changes")
	}
	for _, hash := range record.Hashes {
		if len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" {
			return fmt.Errorf("invalid MCP ownership fingerprint")
		}
	}
	for _, dir := range record.CreatedDirs {
		if dir == filepath.Dir(dir) || dir == project || !inside(dir, filepath.Dir(path)) || !filepath.IsAbs(dir) {
			return fmt.Errorf("invalid owned config directory")
		}
		if id != "claude" && dir != filepath.Dir(path) {
			return fmt.Errorf("invalid owned project config directory")
		}
	}
	for _, depth := range record.CreatedMaps {
		if depth < 0 || depth > 2 || id != "claude" && depth != 0 {
			return fmt.Errorf("invalid owned config scope")
		}
	}
	return nil
}
