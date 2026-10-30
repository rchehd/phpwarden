package drupal

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/internal/psalmanalysis"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

//go:embed psalm/*.php
var psalmAssets embed.FS

func PsalmPins() map[string]psalmanalysis.Pin {
	return map[string]psalmanalysis.Pin{
		"mortenson/psalm-plugin-drupal": {Reference: "073e7a4d5fc159fbf47decf16296edd7f8a545d1"},
		"psalm/plugin-symfony":          {Version: "5.3.0"},
		"symfony/yaml":                  {Version: "7.4.21"},
	}
}

type psalmManifest struct {
	Version             int               `json:"version"`
	PreparationComplete *bool             `json:"preparation_complete"`
	AnalysisComplete    *bool             `json:"analysis_complete"`
	Selection           string            `json:"selection"`
	Extensions          []psalmExtension  `json:"extensions"`
	Sources             []string          `json:"sources"`
	Routes              []psalmRoute      `json:"routes"`
	Services            map[string]string `json:"services"`
	Unresolved          map[string]string `json:"unresolved_services"`
	Diagnostics         []struct {
		Code   string `json:"code"`
		File   string `json:"file"`
		Detail string `json:"detail"`
	} `json:"diagnostics"`
	Inputs map[string]string `json:"inputs"`
}

type psalmExtension struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Path string `json:"path"`
}

type psalmRoute struct {
	ID      string `json:"id"`
	File    string `json:"file"`
	Modeled bool   `json:"modeled"`
}

func PreparePsalm(ctx context.Context, root, bundle, temporary, php string, memory int, cfg config.Config, facts *profile.Facts, process runner.Process) (psalmanalysis.Prepared, error) {
	failure := func(message string) (psalmanalysis.Prepared, error) {
		return psalmanalysis.Prepared{}, fmt.Errorf("Drupal Psalm preparation: %s", message)
	}
	if facts == nil || facts.WebRoot == nil || facts.Version == nil || len(facts.SourceRoots) == 0 {
		return failure("framework/source-root discovery is incomplete")
	}
	if facts.Installed == nil || facts.Installed.Status != "verified" {
		return failure("installed Drupal dependencies are unverified")
	}
	inventory, sources, err := psalmInventory(root, cfg, facts)
	if err != nil {
		return failure(err.Error())
	}
	helpers, output := filepath.Join(temporary, "profile"), filepath.Join(temporary, "prepared")
	for _, directory := range []string{helpers, output} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			return failure(err.Error())
		}
	}
	assets, _ := fs.ReadDir(psalmAssets, "psalm")
	for _, entry := range assets {
		data, err := psalmAssets.ReadFile("psalm/" + entry.Name())
		if err != nil {
			return failure(err.Error())
		}
		if err := os.WriteFile(filepath.Join(helpers, entry.Name()), data, 0o600); err != nil {
			return failure(err.Error())
		}
	}
	request := map[string]any{
		"root": root, "composer_root": cfg.ComposerRoot, "web_root": *facts.WebRoot,
		"source_roots": facts.SourceRoots, "include": cfg.Scope.Include, "exclude": cfg.Scope.Exclude,
		"cache_directory": filepath.Join(temporary, "cache"),
		"inventory":       inventory, "files": sources,
	}
	data, err := json.Marshal(request)
	if err != nil {
		return failure(err.Error())
	}
	filename := filepath.Join(temporary, "request.json")
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		return failure(err.Error())
	}
	arguments := append(psalmanalysis.PHPArgs(memory), filepath.Join(helpers, "prepare.php"), filename, output, bundle)
	execution := process.Run(ctx, runner.Command{Name: php, Args: arguments, Directory: temporary, Env: psalmanalysis.Environment(temporary, bundle)})
	if execution.ExitCode != 0 || execution.Err != nil || execution.Unavailable || execution.TimedOut || len(execution.Stderr) > 0 || ctx.Err() != nil {
		var feedback struct {
			Diagnostics []struct{ Code, File, Detail string } `json:"diagnostics"`
		}
		parts := []string{}
		if json.Unmarshal(execution.Stdout, &feedback) == nil {
			for i, diagnostic := range feedback.Diagnostics {
				if i == 10 {
					parts = append(parts, fmt.Sprintf("%d additional preparation diagnostics in native evidence", len(feedback.Diagnostics)-i))
					break
				}
				if diagnostic.Code == "" || diagnostic.Detail == "" || diagnostic.File != "" && projectpath.Validate(diagnostic.File) != nil {
					continue
				}
				parts = append(parts, strings.TrimSpace(diagnostic.File+": "+diagnostic.Code+": "+diagnostic.Detail))
			}
		}
		if len(parts) == 0 && len(execution.Stderr) > 0 {
			parts = append(parts, strings.TrimSpace(string(execution.Stderr[:min(len(execution.Stderr), 2048)])))
		}
		if len(parts) == 0 {
			parts = append(parts, "profile metadata could not be prepared; see native preparation output")
		}
		return failure(strings.Join(parts, "; "))
	}
	manifestFile := filepath.Join(output, "manifest.json")
	stored, err := os.ReadFile(manifestFile)
	if err != nil || !bytes.Equal(bytes.TrimSpace(stored), bytes.TrimSpace(execution.Stdout)) {
		// Formatting differs between the persisted manifest and stdout.
		var a, b any
		if err != nil || json.Unmarshal(stored, &a) != nil || json.Unmarshal(execution.Stdout, &b) != nil {
			return failure("missing or invalid persisted manifest")
		}
		canonicalA, _ := json.Marshal(a)
		canonicalB, _ := json.Marshal(b)
		if !bytes.Equal(canonicalA, canonicalB) {
			return failure("persisted manifest does not match native output")
		}
	}
	manifest, err := validatePsalmManifest(root, execution.Stdout, cfg, facts)
	if err != nil {
		return failure(err.Error())
	}
	if !slices.Equal(manifest.Sources, sources) {
		return failure("profile omitted or added selected PHP source files")
	}
	return psalmanalysis.Prepared{Config: filepath.Join(output, "psalm.xml"), Launcher: filepath.Join(helpers, "psalm.php"), Files: manifest.Sources, Inputs: manifest.Inputs}, nil
}

func psalmInventory(root string, cfg config.Config, facts *profile.Facts) ([]string, []string, error) {
	files := []string{}
	for _, directory := range facts.SourceRoots {
		if err := projectpath.Validate(directory); err != nil {
			return nil, nil, err
		}
		if err := projectpath.Within(root, directory); err != nil {
			return nil, nil, err
		}
		err := filepath.WalkDir(filepath.Join(root, filepath.FromSlash(directory)), func(filename string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				if slices.Contains([]string{"tests", "vendor", "node_modules", "files", ".git"}, entry.Name()) {
					return fs.SkipDir
				}
				return nil
			}
			relative, _ := filepath.Rel(root, filename)
			relative = filepath.ToSlash(relative)
			selected := scope.Selection{Files: []string{relative}}
			if len(selected.Filter(nil, cfg.Scope.Exclude, func(string) bool { return true }).Files) == 0 {
				return nil
			}
			if entry.Type()&fs.ModeSymlink != 0 || !entry.Type().IsRegular() {
				return fmt.Errorf("unsupported source entry %s", relative)
			}
			files = append(files, relative)
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	slices.Sort(files)
	files = slices.Compact(files)
	selection := scope.Selection{Files: files}
	selected := selection.Filter(cfg.Scope.Include, cfg.Scope.Exclude, func(filename string) bool {
		return slices.Contains([]string{".php", ".module", ".theme", ".inc", ".install", ".profile", ".engine", ".test"}, strings.ToLower(path.Ext(filename)))
	})
	return files, selected.Files, nil
}

func validatePsalmManifest(root string, data []byte, cfg config.Config, facts *profile.Facts) (psalmManifest, error) {
	failure := func() (psalmManifest, error) {
		return psalmManifest{}, fmt.Errorf("invalid or incomplete profile manifest")
	}
	if len(data) > 32<<20 {
		return failure()
	}
	var manifest psalmManifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&manifest) != nil {
		return failure()
	}
	if _, err := decoder.Token(); err != io.EOF || manifest.Version != 1 || manifest.PreparationComplete == nil || !*manifest.PreparationComplete || manifest.AnalysisComplete == nil || *manifest.AnalysisComplete || manifest.Selection == "" || manifest.Diagnostics == nil || len(manifest.Diagnostics) != 0 || manifest.Services == nil || manifest.Unresolved == nil || manifest.Routes == nil || manifest.Extensions == nil || len(manifest.Sources) == 0 {
		return failure()
	}
	if err := psalmanalysis.HashesUnchanged(root, manifest.Inputs); err != nil {
		return psalmManifest{}, err
	}
	required := []string{path.Join(cfg.ComposerRoot, "composer.json"), path.Join(cfg.ComposerRoot, "composer.lock"), path.Join(*facts.WebRoot, "core/core.services.yml")}
	for _, extension := range manifest.Extensions {
		if extension.Name == "" || path.Base(extension.Name) != extension.Name || !slices.Contains([]string{"module", "theme", "profile"}, extension.Type) || projectpath.Validate(extension.Path) != nil {
			return failure()
		}
		required = append(required, path.Join(extension.Path, extension.Name+".info.yml"))
	}
	for _, route := range manifest.Routes {
		if !route.Modeled || route.ID == "" {
			return failure()
		}
		required = append(required, route.File)
	}
	previous := ""
	selection := scope.Selection{Files: manifest.Sources}
	selected := selection.Filter(cfg.Scope.Include, cfg.Scope.Exclude, func(name string) bool {
		return slices.Contains([]string{".php", ".module", ".theme", ".inc", ".install", ".profile", ".engine", ".test"}, path.Ext(name))
	})
	if !slices.Equal(selected.Files, manifest.Sources) {
		return failure()
	}
	_, expected, err := psalmInventory(root, cfg, facts)
	if err != nil || !slices.Equal(expected, manifest.Sources) {
		return failure()
	}
	for _, source := range manifest.Sources {
		if source <= previous || path.Clean(source) != source {
			return failure()
		}
		previous = source
		required = append(required, source)
	}
	for _, name := range required {
		found := false
		for input := range manifest.Inputs {
			found = found || path.Clean(input) == path.Clean(name)
		}
		if !found {
			return failure()
		}
	}
	return manifest, nil
}
