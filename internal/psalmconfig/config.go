package psalmconfig

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/internal/psalmanalysis"
)

var Candidates = []string{"psalm.xml", "psalm.xml.dist", "psalm.dist.xml"}
var issueName = regexp.MustCompile(`^[A-Z][A-Za-z0-9]+$`)

type Options struct {
	Config   string
	Discover bool
	Baseline string
}

type Native struct {
	Config     string
	Baseline   string
	Inputs     map[string]string
	attributes map[string]string
	stubs      []*element
	handlers   *element
	baseline   *element
}

func Load(root, composerRoot string, options Options) (native Native, err error) {
	native.Inputs, native.attributes = map[string]string{}, map[string]string{}
	read := func(name string) ([]byte, error) {
		data, err := psalmanalysis.Read(root, name)
		if err != nil {
			return nil, fmt.Errorf("native Psalm input %q is missing, unreadable or unsafe", name)
		}
		native.Inputs[name] = fmt.Sprintf("%x", sha256.Sum256(data))
		return data, nil
	}
	native.Config = options.Config
	if native.Config == "" && options.Discover {
		for _, directory := range slices.Compact([]string{".", composerRoot}) {
			for _, candidate := range Candidates {
				name := path.Join(directory, candidate)
				if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(name))); os.IsNotExist(err) {
					continue
				}
				native.Config = name
				break
			}
			if native.Config != "" {
				break
			}
		}
	}
	if native.Config != "" {
		data, err := read(native.Config)
		if err != nil {
			return native, err
		}
		document, err := parse(data)
		if err != nil {
			return native, fmt.Errorf("native Psalm config %q: %w", native.Config, err)
		}
		if document.Name != "psalm" {
			return native, fmt.Errorf("native Psalm config must have a psalm root")
		}
		if err := document.attributes("errorLevel", "reportMixedIssues", "resolveFromConfigFile", "errorBaseline"); err != nil {
			return native, err
		}
		if strings.TrimSpace(document.Text) != "" {
			return native, fmt.Errorf("unexpected text in Psalm config")
		}
		base := path.Dir(native.Config)
		if value := document.attr("resolveFromConfigFile"); value != "" {
			if _, err := boolean(value); err != nil {
				return native, err
			}
			if value == "false" || value == "0" {
				base = composerRoot
			}
		}
		if value := document.attr("errorLevel"); value != "" {
			level, err := strconv.Atoi(value)
			if err != nil || level < 1 || level > 6 {
				return native, fmt.Errorf("Psalm errorLevel must be 1 to 6; readiness cannot be loosened below the profile level")
			}
			native.attributes["errorLevel"] = value
		}
		if value := document.attr("reportMixedIssues"); value != "" {
			value, err = boolean(value)
			if err != nil {
				return native, err
			}
			native.attributes["reportMixedIssues"] = value
		}
		if value := document.attr("errorBaseline"); value != "" {
			native.Baseline, err = local(base, value, false)
			if err != nil {
				return native, err
			}
		}
		seen := map[string]bool{}
		for _, section := range document.Children {
			if seen[section.Name] || section.attributes() != nil || strings.TrimSpace(section.Text) != "" {
				return native, fmt.Errorf("invalid or duplicate native Psalm section %s", section.Name)
			}
			seen[section.Name] = true
			switch section.Name {
			case "stubs":
				if len(section.Children) == 0 || len(section.Children) > 128 {
					return native, fmt.Errorf("Psalm stubs must contain 1 to 128 files")
				}
				paths := map[string]bool{}
				for _, stub := range section.Children {
					if stub.Name != "file" || stub.attributes("name", "preloadClasses") != nil || len(stub.Children) != 0 || strings.TrimSpace(stub.Text) != "" {
						return native, fmt.Errorf("unsupported native Psalm stub")
					}
					name, err := local(base, stub.attr("name"), false)
					if err != nil || paths[name] {
						return native, fmt.Errorf("invalid or duplicate native Psalm stub path")
					}
					paths[name] = true
					if _, err := read(name); err != nil {
						return native, err
					}
					stub.set("name", name)
					if value := stub.attr("preloadClasses"); value != "" {
						value, err = boolean(value)
						if err != nil {
							return native, err
						}
						stub.set("preloadClasses", value)
					}
					native.stubs = append(native.stubs, stub)
				}
			case "issueHandlers":
				if len(section.Children) == 0 {
					return native, fmt.Errorf("issueHandlers must not be empty")
				}
				issues := map[string]bool{}
				for _, handler := range section.Children {
					if !strings.HasPrefix(handler.Name, "Tainted") {
						return native, fmt.Errorf("only Tainted* issue handlers are supported; readiness diagnostics cannot be suppressed")
					}
					if !issueName.MatchString(handler.Name) || issues[handler.Name] {
						return native, fmt.Errorf("invalid or duplicate Psalm issue handler")
					}
					issues[handler.Name] = true
					if err := handlerPolicy(handler, base); err != nil {
						return native, err
					}
				}
				native.handlers = section
			default:
				return native, fmt.Errorf("native Psalm section %s is managed by the profile or unsupported; use project scope/report filters instead of projectFiles", section.Name)
			}
		}
	}
	if options.Baseline != "" {
		native.Baseline = options.Baseline
	}
	if native.Baseline != "" {
		data, err := read(native.Baseline)
		if err != nil {
			return native, err
		}
		native.baseline, err = parse(data)
		if err != nil {
			return native, fmt.Errorf("native Psalm baseline %q: %w", native.Baseline, err)
		}
		if err := baselinePolicy(native.baseline, composerRoot); err != nil {
			return native, err
		}
	}
	return native, nil
}

func boolean(value string) (string, error) {
	switch value {
	case "true", "1":
		return "true", nil
	case "false", "0":
		return "false", nil
	}
	return "", fmt.Errorf("invalid native Psalm boolean %q", value)
}

func local(base, value string, glob bool) (string, error) {
	if value == "" || strings.Contains(value, ":") || strings.HasPrefix(value, "/") || strings.Contains(value, `\`) || !glob && strings.ContainsAny(value, "*?[]{}") {
		return "", fmt.Errorf("native Psalm paths must be local project paths")
	}
	name := path.Join(base, value)
	if err := projectpath.Validate(name); err != nil {
		return "", fmt.Errorf("native Psalm path escapes the project")
	}
	if name == "." && !glob {
		return "", fmt.Errorf("native Psalm input must name a file")
	}
	return name, nil
}

func handlerPolicy(handler *element, base string) error {
	if handler.attributes("errorLevel") != nil || strings.TrimSpace(handler.Text) != "" {
		return fmt.Errorf("unsupported native Psalm issue handler")
	}
	if value := handler.attr("errorLevel"); value != "" && !slices.Contains([]string{"error", "info", "suppress"}, value) {
		return fmt.Errorf("invalid issue handler errorLevel")
	}
	if handler.attr("errorLevel") == "" && len(handler.Children) == 0 {
		return fmt.Errorf("empty native Psalm issue handler")
	}
	for _, level := range handler.Children {
		if level.Name != "errorLevel" || level.attributes("type") != nil || !slices.Contains([]string{"error", "info", "suppress"}, level.attr("type")) || len(level.Children) == 0 || strings.TrimSpace(level.Text) != "" {
			return fmt.Errorf("invalid conditional issue handler")
		}
		for _, selector := range level.Children {
			if selector.attributes("name") != nil || selector.attr("name") == "" || len(selector.Children) > 0 || strings.TrimSpace(selector.Text) != "" {
				return fmt.Errorf("invalid issue handler selector")
			}
			if slices.Contains([]string{"file", "directory"}, selector.Name) {
				name, err := local(base, selector.attr("name"), true)
				if err != nil {
					return err
				}
				selector.set("name", name)
			} else if !slices.Contains([]string{"referencedClass", "referencedMethod", "referencedFunction", "referencedProperty", "referencedConstant", "referencedVariable", "referencedArgument"}, selector.Name) {
				return fmt.Errorf("unsupported issue handler selector %s", selector.Name)
			}
		}
	}
	return nil
}

func baselinePolicy(root *element, composerRoot string) error {
	if root.Name != "files" || root.attributes("psalm-version", "php-version") != nil || strings.TrimSpace(root.Text) != "" {
		return fmt.Errorf("unsupported native taint baseline root")
	}
	files := map[string]bool{}
	for _, file := range root.Children {
		if file.Name != "file" || file.attributes("src") != nil || strings.TrimSpace(file.Text) != "" || len(file.Children) == 0 {
			return fmt.Errorf("invalid native baseline file")
		}
		name := file.attr("src")
		if strings.HasPrefix(name, "@generated/") {
			if strings.TrimPrefix(name, "@generated/") != "entrypoints.php" {
				return fmt.Errorf("unsupported generated baseline path")
			}
		} else {
			resolved, err := local(composerRoot, name, false)
			if err != nil {
				return err
			}
			name = resolved
		}
		if files[name] {
			return fmt.Errorf("duplicate native baseline file")
		}
		files[name] = true
		file.set("src", name)
		issues := map[string]bool{}
		for _, issue := range file.Children {
			if !strings.HasPrefix(issue.Name, "Tainted") || !issueName.MatchString(issue.Name) || issues[issue.Name] || issue.attributes() != nil || len(issue.Children) == 0 || strings.TrimSpace(issue.Text) != "" {
				return fmt.Errorf("baseline must contain native Tainted* issues and code samples only; readiness issues cannot be baselined")
			}
			issues[issue.Name] = true
			for _, code := range issue.Children {
				if code.Name != "code" || code.attributes() != nil || len(code.Children) != 0 || strings.TrimSpace(code.Text) == "" {
					return fmt.Errorf("invalid baseline code sample")
				}
			}
		}
	}
	return nil
}
