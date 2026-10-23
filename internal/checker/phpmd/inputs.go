package phpmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/internal/scope"
)

var ConfigCandidates = []string{"phpmd.xml", "phpmd.xml.dist", ".phpmd.xml"}

// PHPMD reads this baseline next to the first ruleset when it exists.
const baselineName = "phpmd.baseline.xml"

type group struct {
	Config string
	Files  []string
}

type plan struct {
	Groups []group
	Inputs map[string]string
}

func Select(selection scope.Selection, cfg config.PHPMD, profileID string) scope.Selection {
	defaults, _ := profile.Lookup(profileID)
	extensions := append(slices.Clone(defaults.PHPExtensions), cfg.ExtraExtensions...)
	return selection.Filter(cfg.Include, cfg.Exclude, func(name string) bool { return slices.Contains(extensions, strings.ToLower(path.Ext(name))) })
}

func InputRequired(cfg config.Config, selection scope.Selection, only []string) bool {
	return checker.Enabled("phpmd", cfg.Tools.PHPMD.Enabled, only) && len(Select(selection, cfg.Tools.PHPMD, cfg.Profile).Files) > 0
}

func ContextHash(root string, selection scope.Selection, cfg config.Config) string {
	prepared, err := prepare(root, cfg.ComposerRoot, Select(selection, cfg.Tools.PHPMD, cfg.Profile), cfg.Tools.PHPMD)
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

// prepare groups files by their nearest project ruleset and hashes every project file PHPMD may read.
func prepare(root, composerRoot string, selection scope.Selection, cfg config.PHPMD) (plan, error) {
	result := plan{Inputs: map[string]string{}, Groups: []group{}}
	read := func(name string) ([]byte, error) {
		if err := projectpath.Validate(name); err != nil {
			return nil, err
		}
		current := root
		for _, part := range strings.Split(name, "/") {
			current = filepath.Join(current, part)
			info, err := os.Lstat(current)
			if os.IsNotExist(err) {
				result.Inputs[name] = "missing"
				return nil, err
			}
			if err != nil || info.Mode()&os.ModeSymlink != 0 {
				result.Inputs[name] = "symlink"
				return nil, fmt.Errorf("PHPMD input %q must not traverse symlinks", name)
			}
		}
		info, err := os.Lstat(current)
		if err != nil || !info.Mode().IsRegular() {
			result.Inputs[name] = "nonregular"
			return nil, fmt.Errorf("PHPMD input %q must be a regular file", name)
		}
		data, err := os.ReadFile(current)
		if err != nil {
			result.Inputs[name] = "unreadable"
			return nil, fmt.Errorf("cannot read PHPMD input %q", name)
		}
		result.Inputs[name] = fmt.Sprintf("%x", sha256.Sum256(data))
		return data, nil
	}
	parsed := map[string]bool{}
	var ruleset func(string) error
	ruleset = func(name string) error {
		if parsed[name] {
			return nil
		}
		parsed[name] = true
		data, err := read(name)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("PHPMD ruleset %q is missing", name)
			}
			return err
		}
		decoder := xml.NewDecoder(bytes.NewReader(data))
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				return nil
			}
			if err != nil {
				return fmt.Errorf("invalid PHPMD XML in %q: %v", name, err)
			}
			if _, ok := token.(xml.Directive); ok {
				return fmt.Errorf("PHPMD XML directives are unsupported in %q", name)
			}
			start, ok := token.(xml.StartElement)
			if !ok || start.Name.Local != "rule" {
				continue
			}
			for _, attr := range start.Attr {
				// Built-in rulesets resolve inside the bundle; PHPMD resolves other references from the project root.
				end := strings.Index(attr.Value, ".xml")
				if attr.Name.Local != "ref" || strings.HasPrefix(attr.Value, "rulesets/") || end < 0 {
					continue
				}
				if path.IsAbs(attr.Value) || strings.Contains(attr.Value, "\\") || strings.Contains(attr.Value, "://") {
					return fmt.Errorf("PHPMD ruleset %q requires project-relative references", name)
				}
				if err := ruleset(path.Clean(attr.Value[:end+4])); err != nil {
					return err
				}
			}
		}
	}
	for _, name := range cfg.ExtraInputs {
		if _, err := read(path.Clean(name)); err != nil {
			return result, fmt.Errorf("PHPMD extra input %q: %w", name, err)
		}
	}
	groupIndex := map[string]int{}
	for _, filename := range selection.Files {
		selected := cfg.Config
		if selected == "" && cfg.DiscoverConfig {
			for directory := path.Dir(filename); selected == ""; directory = path.Dir(directory) {
				for _, candidate := range ConfigCandidates {
					name := path.Join(directory, candidate)
					if _, err := read(name); err == nil {
						selected = name
						break
					} else if !os.IsNotExist(err) {
						return result, err
					}
				}
				if directory == "." {
					break
				}
			}
			// Code outside the Composer project, as in a monorepo, falls back to its config.
			for _, candidate := range ConfigCandidates {
				if selected != "" || composerRoot == "." {
					break
				}
				name := path.Join(composerRoot, candidate)
				if _, err := read(name); err == nil {
					selected = name
				} else if !os.IsNotExist(err) {
					return result, err
				}
			}
		}
		if selected != "" {
			selected = path.Clean(selected)
			if err := ruleset(selected); err != nil {
				return result, err
			}
			if _, err := read(path.Join(path.Dir(selected), baselineName)); err != nil && !os.IsNotExist(err) {
				return result, err
			}
		}
		index, exists := groupIndex[selected]
		if !exists {
			index = len(result.Groups)
			groupIndex[selected] = index
			result.Groups = append(result.Groups, group{Config: selected})
		}
		result.Groups[index].Files = append(result.Groups[index].Files, filename)
	}
	return result, nil
}
