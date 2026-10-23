package toolset

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/rchehd/phpwarden/internal/phpconstraint"
	"github.com/rchehd/phpwarden/internal/protocol"
	"github.com/rchehd/phpwarden/schemas"
)

//go:embed manifest-v1.json
var bundledManifest []byte

var bundled = sync.OnceValues(func() (Catalog, error) { return ParseManifest(bundledManifest) })

type manifest struct {
	Version  int     `json:"version"`
	Release  string  `json:"release"`
	Protocol int     `json:"protocol"`
	Entries  []entry `json:"entries"`
}

type Preset struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

type entry struct {
	ID           string            `json:"id"`
	Profile      string            `json:"profile"`
	Preset       Preset            `json:"preset"`
	Framework    *string           `json:"framework"`
	PHP          manifestPHP       `json:"php"`
	Toolset      string            `json:"toolset"`
	Tools        map[string]string `json:"tools"`
	Capabilities []string          `json:"capabilities"`
	Development  bool              `json:"development"`
	Images       []manifestImage   `json:"images"`
}

type manifestPHP struct {
	Runtime string `json:"runtime"`
	Version string `json:"version"`
	Targets struct {
		Min string `json:"min"`
		Max string `json:"max"`
	} `json:"targets"`
}

type manifestImage struct {
	Platform  string `json:"platform"`
	Reference string `json:"reference"`
}

type candidate struct {
	entry
	framework phpconstraint.Set
	targets   phpconstraint.Set
}

type Catalog struct {
	release  string
	protocol int
	entries  []candidate
}

var publishedImage = regexp.MustCompile(`^(?:localhost|[a-z0-9]+(?:[.-][a-z0-9]+)+)(?::[0-9]+)?(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)+@sha256:[0-9a-f]{64}$`)

func ParseManifest(data []byte) (Catalog, error) {
	if len(data) > 1<<20 {
		return Catalog{}, fmt.Errorf("compatibility manifest exceeds 1 MiB")
	}
	if !utf8.Valid(data) {
		return Catalog{}, fmt.Errorf("compatibility manifest must be UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	document, err := jsonDocument(decoder, 0)
	if err != nil {
		return Catalog{}, fmt.Errorf("read compatibility manifest: %w", err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return Catalog{}, fmt.Errorf("expected exactly one compatibility manifest document")
	}
	if err := schemas.ValidateManifest(document); err != nil {
		return Catalog{}, fmt.Errorf("invalid compatibility manifest: %w", err)
	}
	var source manifest
	if err := json.Unmarshal(data, &source); err != nil {
		return Catalog{}, err
	}
	if source.Protocol != protocol.Version {
		return Catalog{}, fmt.Errorf("unsupported worker protocol %d", source.Protocol)
	}
	result := Catalog{release: source.Release, protocol: source.Protocol}
	seen := make(map[string]bool)
	toolsets := make(map[string]string)
	images := make(map[string]string)
	for _, item := range source.Entries {
		if seen[item.ID] {
			return Catalog{}, fmt.Errorf("duplicate manifest entry %q", item.ID)
		}
		seen[item.ID] = true
		compiled, err := compileEntry(source.Release, item)
		if err != nil {
			return Catalog{}, fmt.Errorf("manifest entry %q: %w", item.ID, err)
		}
		capabilities := slices.Clone(item.Capabilities)
		slices.Sort(capabilities)
		signature, _ := json.Marshal(struct {
			Runtime      string
			Version      string
			Tools        map[string]string
			Capabilities []string
		}{item.PHP.Runtime, item.PHP.Version, item.Tools, capabilities})
		if previous, exists := toolsets[item.Toolset]; exists && previous != string(signature) {
			return Catalog{}, fmt.Errorf("toolset %q has inconsistent runtime, tools or capabilities", item.Toolset)
		}
		toolsets[item.Toolset] = string(signature)
		for _, image := range item.Images {
			if previous, exists := images[image.Platform+":"+image.Reference]; exists && previous != item.Toolset {
				return Catalog{}, fmt.Errorf("image %q has inconsistent toolset identities", image.Reference)
			}
			images[image.Platform+":"+image.Reference] = item.Toolset
		}
		result.entries = append(result.entries, compiled)
	}
	return result, nil
}

func compileEntry(release string, item entry) (candidate, error) {
	result := candidate{entry: item}
	if release == "development" && !item.Development {
		return result, fmt.Errorf("release entries require a release manifest identity")
	}
	version, err := phpconstraint.StableVersion(item.PHP.Version)
	if err != nil || version != item.PHP.Version || !strings.HasPrefix(version, item.PHP.Runtime+".") || item.Tools["php"] != version {
		return result, fmt.Errorf("PHP runtime, exact stable version and tools.php must agree")
	}
	result.targets, err = phpconstraint.Parse(">=" + item.PHP.Targets.Min + " <" + item.PHP.Targets.Max + " || " + item.PHP.Targets.Max + ".*")
	if err != nil {
		return result, err
	}
	if covers, _, err := result.targets.MinorRange(item.PHP.Targets.Min, item.PHP.Targets.Max); err != nil || !covers {
		return result, fmt.Errorf("invalid supported PHP target range")
	}
	if item.Framework != nil {
		result.framework, err = phpconstraint.Parse(*item.Framework)
		if err != nil {
			return result, fmt.Errorf("framework constraint: %w", err)
		}
	} else if !item.Development {
		return result, fmt.Errorf("release entries require an explicit framework constraint")
	}
	if item.Development && (release != "development" || item.Framework != nil || item.Toolset != DevelopmentID(item.PHP.Runtime) ||
		item.PHP.Targets.Min != minimumPHP || item.PHP.Targets.Max != item.PHP.Runtime ||
		!slices.Equal(item.Capabilities, []string{"composer-audit", "composer-validate", "debug", "drupal-config", "phpcs", "phpmd", "phpstan", "syntax", "twig-cs-fixer", "whitespace", "yaml"})) {
		return result, fmt.Errorf("only the bundled framework-independent static development toolset is supported")
	}
	platforms := make(map[string]bool)
	for _, image := range item.Images {
		if platforms[image.Platform] {
			return result, fmt.Errorf("duplicate image platform %q", image.Platform)
		}
		platforms[image.Platform] = true
		if item.Development {
			if image.Platform != "linux/amd64" || image.Reference != "phpwarden:static-php"+item.PHP.Runtime+"-dev" {
				return result, fmt.Errorf("unsupported development image or platform")
			}
		} else if !publishedImage.MatchString(image.Reference) {
			return result, fmt.Errorf("release images require a fully qualified registry/repository@sha256 digest without a tag")
		}
	}
	return result, nil
}

func jsonDocument(decoder *json.Decoder, depth int) (any, error) {
	if depth > 32 {
		return nil, fmt.Errorf("manifest nesting exceeds 32 levels")
	}
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return token, nil
	}
	switch delimiter {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("expected a JSON object key")
			}
			if _, duplicate := object[name]; duplicate {
				return nil, fmt.Errorf("duplicate JSON key %q", name)
			}
			value, err := jsonDocument(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			object[name] = value
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return object, nil
	case '[':
		array := []any{}
		for decoder.More() {
			value, err := jsonDocument(decoder, depth+1)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if _, err := decoder.Token(); err != nil {
			return nil, err
		}
		return array, nil
	default:
		return nil, fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
}
