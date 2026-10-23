package phpcs

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
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

var ConfigCandidates = []string{".phpcs.xml", "phpcs.xml", ".phpcs.xml.dist", "phpcs.xml.dist"}

type group struct {
	Config string   `json:"config"`
	Files  []string `json:"files"`
}

type plan struct {
	Groups []group
	Inputs map[string]string
}

func Select(selection scope.Selection, cfg config.PHPCS, profileID string) scope.Selection {
	defaults, _ := profile.Lookup(profileID)
	extensions := append(slices.Clone(defaults.PHPExtensions), cfg.ExtraExtensions...)
	return selection.Filter(cfg.Include, cfg.Exclude, func(name string) bool { return slices.Contains(extensions, strings.ToLower(path.Ext(name))) })
}

func InputRequired(cfg config.Config, selection scope.Selection, only []string) bool {
	return checker.Enabled("phpcs", cfg.Tools.PHPCS.Enabled, only) && len(Select(selection, cfg.Tools.PHPCS, cfg.Profile).Files) > 0
}

func ContextHash(root string, selection scope.Selection, cfg config.Config) string {
	prepared, err := prepare(root, Select(selection, cfg.Tools.PHPCS, cfg.Profile), cfg.Tools.PHPCS)
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

func prepare(root string, selection scope.Selection, cfg config.PHPCS) (plan, error) {
	result := plan{Inputs: map[string]string{}, Groups: []group{}}
	visited := map[string]bool{}
	var record func(string, bool) error
	read := func(name string) ([]byte, error) {
		name = path.Clean(name)
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
			if err != nil {
				result.Inputs[name] = "unreadable"
				return nil, fmt.Errorf("cannot inspect %q", name)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				result.Inputs[name] = "symlink"
				return nil, fmt.Errorf("PHPCS input %q must not traverse symlinks", name)
			}
		}
		info, err := os.Lstat(current)
		if err != nil || !info.Mode().IsRegular() {
			result.Inputs[name] = "nonregular"
			return nil, fmt.Errorf("PHPCS input %q must be a regular file", name)
		}
		data, err := os.ReadFile(current)
		if err != nil {
			result.Inputs[name] = "unreadable"
			return nil, fmt.Errorf("cannot read PHPCS input %q", name)
		}
		result.Inputs[name] = fmt.Sprintf("%x", sha256.Sum256(data))
		return data, nil
	}
	record = func(name string, follow bool) error {
		name = path.Clean(name)
		if follow && visited[name] {
			return nil
		}
		if follow {
			visited[name] = true
		}
		if err := projectpath.Validate(name); err != nil {
			return err
		}
		if err := projectpath.Within(root, name); err != nil {
			return err
		}
		absolute := filepath.Join(root, filepath.FromSlash(name))
		info, err := os.Lstat(absolute)
		if err != nil {
			result.Inputs[name] = "missing"
			return fmt.Errorf("PHPCS input %q is missing", name)
		}
		if info.IsDir() {
			listing := []string{}
			err = filepath.WalkDir(absolute, func(current string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return fmt.Errorf("cannot inspect PHPCS directory %q", name)
				}
				relative, _ := filepath.Rel(root, current)
				relative = filepath.ToSlash(relative)
				if entry.IsDir() && entry.Name() == ".git" {
					return fs.SkipDir
				}
				if entry.Type()&os.ModeSymlink != 0 {
					return fmt.Errorf("PHPCS input %q must not contain symlinks", name)
				}
				listing = append(listing, relative)
				if entry.IsDir() {
					return nil
				}
				if !entry.Type().IsRegular() {
					return fmt.Errorf("PHPCS input %q is not a regular file", relative)
				}
				return record(relative, follow && path.Base(relative) == "ruleset.xml")
			})
			result.Inputs[name+"/"] = fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(listing, "\x00"))))
			return err
		}
		data, err := read(name)
		if err != nil {
			return err
		}
		if !follow || !strings.HasSuffix(name, ".xml") && !strings.HasSuffix(name, ".xml.dist") {
			return nil
		}
		decoder := xml.NewDecoder(bytes.NewReader(data))
		for {
			token, err := decoder.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				return fmt.Errorf("invalid PHPCS XML in %q: %v", name, err)
			}
			if _, ok := token.(xml.Directive); ok {
				return fmt.Errorf("PHPCS XML directives are unsupported in %q", name)
			}
			start, ok := token.(xml.StartElement)
			if !ok {
				continue
			}
			ref := ""
			if start.Name.Local == "rule" {
				for _, attr := range start.Attr {
					if attr.Name.Local == "ref" {
						ref = attr.Value
					}
				}
			}
			if start.Name.Local == "autoload" {
				if err := decoder.DecodeElement(&ref, &start); err != nil {
					return err
				}
			}
			if ref == "" {
				continue
			}
			isFile := start.Name.Local == "autoload" || strings.ContainsAny(ref, "/\\") || strings.HasSuffix(ref, ".xml") || strings.HasSuffix(ref, ".php")
			if isFile {
				if path.IsAbs(ref) || strings.Contains(ref, "\\") || strings.Contains(ref, "://") {
					return fmt.Errorf("PHPCS ruleset %q requires project-relative references", name)
				}
				if err := record(path.Join(path.Dir(name), ref), true); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, name := range cfg.StandardPaths {
		if err := record(name, true); err != nil {
			return result, err
		}
	}
	for _, name := range cfg.ExtraInputs {
		if err := record(name, false); err != nil {
			return result, err
		}
	}
	groupIndex := map[string]int{}
	for _, filename := range selection.Files {
		selected := cfg.Config
		if selected == "" && cfg.DiscoverConfig {
			for directory := path.Dir(filename); ; directory = path.Dir(directory) {
				for _, candidate := range ConfigCandidates {
					name := path.Join(directory, candidate)
					_, err := read(name)
					if os.IsNotExist(err) {
						continue
					}
					if err != nil {
						return result, err
					}
					if selected == "" {
						selected = name
					}
				}
				if directory == "." {
					break
				}
			}
		}
		if selected != "" {
			if err := record(selected, true); err != nil {
				return result, err
			}
		}
		index, exists := groupIndex[selected]
		if !exists {
			index = len(result.Groups)
			groupIndex[selected] = index
			result.Groups = append(result.Groups, group{Config: selected, Files: []string{}})
		}
		result.Groups[index].Files = append(result.Groups[index].Files, filename)
	}
	return result, nil
}
