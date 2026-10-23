package runrequest

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/phpconstraint"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/internal/protocol"
	"github.com/rchehd/phpwarden/internal/scope"
)

type Request struct {
	Version        int               `json:"version"`
	RunID          string            `json:"run_id"`
	ConfigFile     string            `json:"config_file"`
	Config         config.Config     `json:"config"`
	Project        *profile.Facts    `json:"project,omitempty"`
	Selection      scope.Selection   `json:"selection"`
	Fingerprint    map[string]string `json:"fingerprint"`
	Only           []string          `json:"only,omitempty"`
	Image          string            `json:"image"`
	Toolset        string            `json:"toolset"`
	RuntimeVersion string            `json:"runtime_version,omitempty"`
}

var imageID = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var runID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

func Build(location config.Location, effective config.Effective, selection scope.Selection, only []string, image, toolset string) (Request, error) {
	fingerprint, err := Snapshot(location, selection)
	if err != nil {
		return Request{}, err
	}
	if fingerprint["config"] != effective.SourceHash {
		return Request{}, fmt.Errorf("configuration changed after loading")
	}
	result := Request{Version: protocol.Version, RunID: rand.Text(), ConfigFile: filepath.Base(location.Path), Config: effective.Config,
		Project: effective.Project, Selection: selection, Fingerprint: fingerprint, Only: only, Image: image, Toolset: toolset}
	return result, result.Validate()
}

func (request Request) Validate() error {
	if request.Version != protocol.Version || !runID.MatchString(request.RunID) || !imageID.MatchString(request.Image) || request.Toolset == "" {
		return fmt.Errorf("invalid run request version, identity, or image/toolset")
	}
	if err := checker.ValidateOnly(request.Only); err != nil {
		return err
	}
	if request.RuntimeVersion != "" {
		canonical, err := phpconstraint.StableVersion(request.RuntimeVersion)
		if err != nil || canonical != request.RuntimeVersion {
			return fmt.Errorf("expected a canonical stable image PHP version")
		}
	}
	if err := projectpath.Validate(request.ConfigFile); err != nil {
		return err
	}
	if path.Base(request.ConfigFile) != request.ConfigFile || request.ConfigFile == "." {
		return fmt.Errorf("configuration must be directly inside the project root")
	}
	if err := ValidateSelection(request.Selection); err != nil {
		return err
	}
	if len(request.Fingerprint) != len(request.Selection.Files)+1 {
		return fmt.Errorf("fingerprint does not match selected inputs")
	}
	for _, key := range append([]string{"config"}, fileKeys(request.Selection.Files)...) {
		value := request.Fingerprint[key]
		if decoded, err := hex.DecodeString(value); err != nil || len(decoded) != sha256.Size {
			return fmt.Errorf("invalid fingerprint for %q", key)
		}
	}
	return nil
}

func ValidateSelection(selection scope.Selection) error {
	if selection.Files == nil || selection.LineMap == nil || len(selection.Files) != len(selection.LineMap) {
		return fmt.Errorf("selection requires matching files and line map")
	}
	if selection.Mode != "diff" && selection.Mode != "full" && selection.Mode != "paths" {
		return fmt.Errorf("invalid selection mode")
	}
	if selection.Mode == "diff" {
		if selection.BaseRef == nil || selection.MergeBase == nil || *selection.BaseRef == "" || *selection.MergeBase == "" {
			return fmt.Errorf("diff requires base and merge base")
		}
	} else if selection.BaseRef != nil || selection.MergeBase != nil || selection.WholeFiles {
		return fmt.Errorf("non-diff selection cannot contain diff options")
	}
	previous := ""
	for _, filename := range selection.Files {
		if err := projectpath.Validate(filename); err != nil {
			return err
		}
		if !utf8.ValidString(filename) || filename == "." || path.Clean(filename) != filename || filename <= previous {
			return fmt.Errorf("selection paths must be canonical, unique and sorted")
		}
		previous = filename
		ranges, exists := selection.LineMap[filename]
		if !exists || (selection.Mode != "diff" || selection.WholeFiles) && ranges != nil {
			return fmt.Errorf("invalid line map for %q", filename)
		}
		end := 0
		for _, interval := range ranges {
			if interval.Start <= end || interval.End < interval.Start {
				return fmt.Errorf("invalid line range for %q", filename)
			}
			end = interval.End
		}
	}
	return nil
}

func Snapshot(location config.Location, selection scope.Selection) (map[string]string, error) {
	if err := ValidateSelection(selection); err != nil {
		return nil, err
	}
	result := make(map[string]string)
	files := append([]string{filepath.Base(location.Path)}, selection.Files...)
	for i, filename := range files {
		if err := projectpath.Validate(filename); err != nil {
			return nil, err
		}
		current := location.Root
		for _, part := range strings.Split(filename, "/") {
			current = filepath.Join(current, part)
			info, err := os.Lstat(current)
			if err != nil {
				return nil, err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("input %q must not traverse symlinks", filename)
			}
		}
		info, err := os.Stat(current)
		if err != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("input %q is not a readable regular file", filename)
		}
		data, err := os.ReadFile(current)
		if err != nil {
			return nil, err
		}
		key := "config"
		if i > 0 {
			key = "file:" + filename
		}
		result[key] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	return result, nil
}

func (request Request) Matches(location config.Location) error {
	actual, err := Snapshot(location, request.Selection)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, request.Fingerprint) {
		return fmt.Errorf("project inputs changed since the host request was prepared")
	}
	return nil
}

func fileKeys(files []string) []string {
	keys := make([]string, len(files))
	for i, filename := range files {
		keys[i] = "file:" + filename
	}
	return keys
}
