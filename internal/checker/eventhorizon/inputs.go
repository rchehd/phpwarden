package eventhorizon

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/edit"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/scope"
)

const maxInputBytes = 32 << 20

type plan struct {
	Files      []string          `json:"files"`
	ConfigDirs []string          `json:"config_dirs"`
	Inputs     map[string]string `json:"inputs"`
	contents   map[string][]byte
}

func configDirectory(name string, cfg config.EventHorizon) string {
	if path.Ext(name) != ".yml" {
		return ""
	}
	for _, directory := range cfg.ConfigSync {
		if path.Dir(name) == path.Clean(directory) {
			return path.Clean(directory)
		}
	}
	return ""
}

func Select(selection scope.Selection, cfg config.EventHorizon) scope.Selection {
	defaults, _ := profile.Lookup("drupal")
	return selection.Filter(cfg.Include, cfg.Exclude, func(name string) bool {
		return configDirectory(name, cfg) != "" || cfg.Caching && slices.Contains(defaults.PHPExtensions, strings.ToLower(path.Ext(name)))
	})
}

func InputRequired(cfg config.Config, selected scope.Selection, only []string) bool {
	return checker.Enabled("eventhorizon", cfg.Tools.EventHorizon.Enabled, only) && len(Select(selected, cfg.Tools.EventHorizon).Files) > 0
}

func ContextHash(root string, selected scope.Selection, cfg config.EventHorizon) string {
	prepared, err := prepare(root, selected, cfg)
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

func prepare(directory string, selected scope.Selection, cfg config.EventHorizon) (plan, error) {
	result := plan{Files: []string{}, ConfigDirs: []string{}, Inputs: map[string]string{}, contents: map[string][]byte{}}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return result, err
	}
	defer root.Close()
	total := 0
	read := func(name string) error {
		if !utf8.ValidString(name) || path.Clean(name) != name {
			return fmt.Errorf("EventHorizon inputs require canonical UTF-8 paths")
		}
		if _, exists := result.contents[name]; exists {
			return nil
		}
		result.Inputs[name] = "unreadable"
		data, _, err := edit.Read(root, name)
		if err != nil {
			return fmt.Errorf("EventHorizon input %q is missing, unreadable or unsafe", name)
		}
		total += len(data)
		if total > maxInputBytes || !utf8.Valid(data) || strings.ContainsRune(string(data), '\x00') {
			return fmt.Errorf("EventHorizon inputs exceed 32 MiB or contain invalid text")
		}
		result.Inputs[name], result.contents[name] = edit.Hash(data), data
		return nil
	}
	for _, name := range Select(selected, cfg).Files {
		if sync := configDirectory(name, cfg); sync != "" {
			if !slices.Contains(result.ConfigDirs, sync) {
				result.ConfigDirs = append(result.ConfigDirs, sync)
			}
		} else {
			result.Files = append(result.Files, name)
			if err := read(name); err != nil {
				return result, err
			}
		}
	}
	slices.Sort(result.Files)
	slices.Sort(result.ConfigDirs)
	for _, directory := range result.ConfigDirs {
		for current := directory; current != "."; current = path.Dir(current) {
			info, err := root.Lstat(current)
			if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				return result, fmt.Errorf("EventHorizon config directory %q is missing or unsafe", directory)
			}
		}
		file, err := root.Open(directory)
		if err != nil {
			return result, err
		}
		entries, err := file.ReadDir(-1)
		file.Close()
		if err != nil {
			return result, err
		}
		slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
		for _, entry := range entries {
			if path.Ext(entry.Name()) == ".yml" {
				if err := read(path.Join(directory, entry.Name())); err != nil {
					return result, err
				}
			}
		}
	}
	return result, nil
}
