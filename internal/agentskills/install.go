package agentskills

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const Name = "phpwarden-architecture"

//go:embed skills/phpwarden-architecture
var content embed.FS

var clients = []string{"codex", "cursor", "claude"}

type Installation struct {
	Path          string   `json:"path"`
	Clients       []string `json:"clients"`
	Created       bool     `json:"created"`
	ClaudePath    string   `json:"claude_path,omitempty"`
	ClaudeCreated bool     `json:"claude_created,omitempty"`
}

type Plan struct {
	root    string
	clients []string
}

func ParseClients(value string) ([]string, error) {
	switch value {
	case "", "none":
		return nil, nil
	case "all":
		return slices.Clone(clients), nil
	}
	selected := strings.Split(value, ",")
	for i, id := range selected {
		if !slices.Contains(clients, id) || slices.Contains(selected[:i], id) {
			return nil, fmt.Errorf("invalid agent selection %q; use all, none, or codex,cursor,claude", value)
		}
	}
	result := []string{}
	for _, id := range clients {
		if slices.Contains(selected, id) {
			result = append(result, id)
		}
	}
	return result, nil
}

func Prepare(root string, selected []string) (Plan, error) {
	normalized, err := ParseClients(strings.Join(selected, ","))
	if err != nil {
		return Plan{}, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return Plan{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Plan{}, err
	}
	plan := Plan{root: root, clients: normalized}
	return plan, plan.check()
}

func (plan Plan) files() (map[string][]byte, error) {
	files := map[string][]byte{}
	if len(plan.clients) == 0 {
		return files, nil
	}
	prefix := "skills/" + Name + "/"
	err := fs.WalkDir(content, "skills/"+Name, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := content.ReadFile(name)
		if err == nil {
			files[filepath.Join(".agents", "skills", Name, filepath.FromSlash(strings.TrimPrefix(name, prefix)))] = data
		}
		return err
	})
	return files, err
}

func (plan Plan) check() error {
	files, err := plan.files()
	if err != nil {
		return err
	}
	for name, data := range files {
		if err := plan.parents(filepath.Dir(name), false); err != nil {
			return err
		}
		filename := filepath.Join(plan.root, name)
		if _, err := matchingFile(filename, data); err != nil {
			return err
		}
	}
	if slices.Contains(plan.clients, "claude") {
		if err := plan.parents(filepath.Join(".claude", "skills"), false); err != nil {
			return err
		}
		_, err = plan.claudeLink(false)
	}
	return err
}

func matchingFile(filename string, data []byte) (bool, error) {
	info, err := os.Lstat(filename)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Mode().IsRegular() && info.Size() == int64(len(data)) {
		existing, err := os.ReadFile(filename)
		if err != nil {
			return false, err
		}
		if bytes.Equal(existing, data) {
			return true, nil
		}
	}
	return false, fmt.Errorf("existing agent skill %q differs; leaving it unchanged", filename)
}

func (plan Plan) parents(name string, create bool) error {
	current := plan.root
	for _, part := range strings.Split(name, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		if create {
			if err := os.Mkdir(current, 0o755); err != nil && !os.IsExist(err) {
				return err
			}
		}
		info, err := os.Lstat(current)
		if os.IsNotExist(err) && !create {
			return nil
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("agent skill parent %q must be a directory without symlinks", current)
		}
	}
	return nil
}

func (plan Plan) claudeLink(create bool) (bool, error) {
	filename := filepath.Join(plan.root, ".claude", "skills", Name)
	target := filepath.Join("..", "..", ".agents", "skills", Name)
	if create {
		if err := os.Symlink(target, filename); err == nil {
			return true, nil
		} else if !os.IsExist(err) {
			return false, err
		}
	}
	link, err := os.Readlink(filename)
	if os.IsNotExist(err) && !create {
		return false, nil
	}
	if err != nil || filepath.Clean(link) != target {
		return false, fmt.Errorf("existing Claude skill %q differs; leaving it unchanged", filename)
	}
	return false, nil
}

func (plan Plan) Install() (*Installation, error) {
	if len(plan.clients) == 0 {
		return nil, nil
	}
	if err := plan.check(); err != nil {
		return nil, err
	}
	result := &Installation{Path: filepath.Join(plan.root, ".agents", "skills", Name), Clients: slices.Clone(plan.clients)}
	files, err := plan.files()
	if err != nil {
		return nil, err
	}
	for name, data := range files {
		if err := plan.parents(filepath.Dir(name), true); err != nil {
			return result, err
		}
		filename := filepath.Join(plan.root, name)
		created, err := installFile(filename, data)
		if err != nil {
			return result, err
		}
		result.Created = result.Created || created
	}
	if slices.Contains(plan.clients, "claude") {
		if err := plan.parents(filepath.Join(".claude", "skills"), true); err != nil {
			return result, err
		}
		result.ClaudePath = filepath.Join(plan.root, ".claude", "skills", Name)
		result.ClaudeCreated, err = plan.claudeLink(true)
	}
	return result, err
}

func installFile(filename string, data []byte) (bool, error) {
	if exists, err := matchingFile(filename, data); err != nil || exists {
		return false, err
	}
	file, err := os.CreateTemp(filepath.Dir(filename), ".phpwarden-skill-*")
	if err != nil {
		return false, err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(data)
	if err == nil {
		err = file.Chmod(0o644)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return false, err
	}
	if err := os.Link(file.Name(), filename); os.IsExist(err) {
		_, err = matchingFile(filename, data)
		return false, err
	} else if err != nil {
		return false, err
	}
	return true, nil
}
