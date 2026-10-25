package agenthook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rchehd/phpwarden/internal/agentskills"
	"github.com/rchehd/phpwarden/internal/cache"
)

type Installation struct {
	Client  string `json:"client"`
	Path    string `json:"path"`
	Changed bool   `json:"changed"`
}

type Plan struct {
	root       string
	executable string
	clients    []string
}

func Prepare(root, executable string, clients []string) (Plan, error) {
	selected, err := agentskills.ParseClients(strings.Join(clients, ","))
	if err != nil {
		return Plan{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Plan{}, err
	}
	if executable == "" {
		executable, err = os.Executable()
		if err != nil {
			return Plan{}, err
		}
	}
	plan := Plan{root: root, executable: executable, clients: selected}
	for _, client := range selected {
		if _, _, _, err := plan.updated(client); err != nil {
			return Plan{}, err
		}
	}
	return plan, nil
}

func settingsPath(client string) string {
	if client == "claude" {
		return ".claude/settings.json"
	}
	return "." + client + "/hooks.json"
}

func (plan Plan) updated(client string) (string, []byte, []byte, error) {
	name := settingsPath(client)
	parent := filepath.Join(plan.root, filepath.Dir(name))
	if info, err := os.Lstat(parent); err == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
		return "", nil, nil, fmt.Errorf("hook settings parent must be a real directory: %s", parent)
	} else if err != nil && !os.IsNotExist(err) {
		return "", nil, nil, err
	}
	filename := filepath.Join(plan.root, name)
	var before []byte
	value := map[string]any{}
	if info, err := os.Lstat(filename); err == nil {
		if !info.Mode().IsRegular() || info.Size() > 2<<20 {
			return "", nil, nil, fmt.Errorf("hook settings must be a regular JSON file at most 2 MiB: %s", filename)
		}
		before, err = os.ReadFile(filename)
		if err != nil {
			return "", nil, nil, err
		}
		if err := unambiguousJSON(before); err != nil {
			return "", nil, nil, fmt.Errorf("invalid hook settings %s: %w", filename, err)
		}
		decoder := json.NewDecoder(bytes.NewReader(before))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil || value == nil {
			return "", nil, nil, fmt.Errorf("invalid hook settings JSON: %s", filename)
		}
		if _, err := decoder.Token(); err != io.EOF {
			return "", nil, nil, fmt.Errorf("hook settings contain trailing JSON: %s", filename)
		}
	} else if !os.IsNotExist(err) {
		return "", nil, nil, err
	}
	hooks, ok := value["hooks"].(map[string]any)
	if !ok && value["hooks"] != nil {
		return "", nil, nil, fmt.Errorf("hooks must be an object in %s", filename)
	}
	if hooks == nil {
		hooks = map[string]any{}
		value["hooks"] = hooks
	}
	event := "Stop"
	if client == "cursor" {
		event = "stop"
		if version, exists := value["version"]; exists && version != json.Number("1") {
			return "", nil, nil, fmt.Errorf("unsupported Cursor hook schema version in %s", filename)
		}
		value["version"] = 1
	}
	rows, ok := hooks[event].([]any)
	if !ok && hooks[event] != nil {
		return "", nil, nil, fmt.Errorf("%s hooks must be an array in %s", event, filename)
	}
	suffix := " hook stop --client " + client + " --managed"
	command := "'" + strings.ReplaceAll(plan.executable, "'", "'\"'\"'") + "'" + suffix
	handler := map[string]any{"type": "command", "command": command, "timeout": 600}
	if client == "cursor" {
		handler["loop_limit"] = 5
	}
	managed := false
	for _, row := range rows {
		object, ok := row.(map[string]any)
		if !ok {
			return "", nil, nil, fmt.Errorf("invalid %s hook entry in %s", event, filename)
		}
		entries := []any{object}
		if client != "cursor" {
			entries, ok = object["hooks"].([]any)
			if !ok {
				return "", nil, nil, fmt.Errorf("invalid %s hook group in %s", event, filename)
			}
		}
		for _, entry := range entries {
			object, ok := entry.(map[string]any)
			if !ok {
				return "", nil, nil, fmt.Errorf("invalid hook handler in %s", filename)
			}
			old, _ := object["command"].(string)
			if strings.HasSuffix(old, suffix) {
				if object["async"] == true {
					return "", nil, nil, fmt.Errorf("PHPWarden completion hook cannot run asynchronously in %s", filename)
				}
				if managed {
					return "", nil, nil, fmt.Errorf("duplicate PHPWarden hooks in %s; resolve them before installing", filename)
				}
				managed = true
				object["command"], object["timeout"], object["type"] = command, 600, "command"
			}
		}
	}
	if !managed {
		if client == "cursor" {
			rows = append(rows, handler)
		} else {
			rows = append(rows, map[string]any{"hooks": []any{handler}})
		}
	}
	hooks[event] = rows
	after, err := json.MarshalIndent(value, "", "  ")
	return filename, before, append(after, '\n'), err
}

func (plan Plan) Install(ctx context.Context) ([]Installation, error) {
	if len(plan.clients) == 0 {
		return nil, nil
	}
	release, err := cache.LockProject(ctx, plan.root)
	if err != nil {
		return nil, err
	}
	defer release()
	results := []Installation{}
	for _, client := range plan.clients {
		if err := ctx.Err(); err != nil {
			return results, err
		}
		filename, before, after, err := plan.updated(client)
		if err != nil {
			return results, err
		}
		row := Installation{Client: client, Path: filename, Changed: !bytes.Equal(before, after)}
		if row.Changed {
			if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
				return results, err
			}
			file, err := os.CreateTemp(filepath.Dir(filename), ".phpwarden-hooks-*")
			if err != nil {
				return results, err
			}
			temporary := file.Name()
			_, err = file.Write(after)
			if closeErr := file.Close(); err == nil {
				err = closeErr
			}
			current, readErr := os.ReadFile(filename)
			if readErr != nil && !os.IsNotExist(readErr) || !bytes.Equal(current, before) {
				err = fmt.Errorf("hook settings changed during installation: %s", filename)
			}
			if err == nil {
				err = os.Rename(temporary, filename)
			}
			os.Remove(temporary)
			if err != nil {
				return results, err
			}
		}
		results = append(results, row)
	}
	return results, nil
}
