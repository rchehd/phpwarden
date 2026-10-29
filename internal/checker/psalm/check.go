package psalm

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/psalmanalysis"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/scope"
)

func Bundle() string {
	executable, _ := os.Executable()
	return filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", "psalm", "vendor"))
}

func phpFile(name, profileID string) bool {
	defaults, _ := profile.Lookup(profileID)
	return sourcePath(name) && slices.Contains(defaults.PHPExtensions, strings.ToLower(path.Ext(name)))
}

func metadataFile(name string) bool {
	return sourcePath(name) && (strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".yaml"))
}

func sourcePath(name string) bool {
	return !slices.ContainsFunc(strings.Split(name, "/"), func(component string) bool {
		return slices.Contains([]string{".git", "vendor", "node_modules", "files", "tests"}, component)
	})
}

func Select(selection scope.Selection, cfg config.Config) scope.Selection {
	selected := selection.Filter(nil, nil, func(name string) bool {
		if metadataFile(name) {
			return true
		}
		return len((scope.Selection{Files: []string{name}}).Filter(cfg.Tools.Psalm.Include, cfg.Tools.Psalm.Exclude, func(name string) bool { return phpFile(name, cfg.Profile) }).Files) > 0
	})
	return selected
}

func InputRequired(cfg config.Config, selection scope.Selection, only []string) bool {
	selected := Select(selection, cfg)
	return checker.Enabled("psalm", cfg.Tools.Psalm.Enabled, only) && (len(selected.Files) > 0 || len(selected.ProjectFiles) > 0)
}

func Run(selection scope.Selection, cfg config.Config, analyze func() psalmanalysis.Result) checker.Outcome {
	settings := cfg.Tools.Psalm
	result := checker.Outcome{Check: report.Check{ID: "psalm", Status: "skipped", Required: settings.Required, FailOn: settings.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	selected := Select(selection, cfg)
	if !settings.Enabled || len(selected.Files) == 0 && len(selected.ProjectFiles) == 0 {
		reason := "no_files"
		if !settings.Enabled {
			reason = "disabled"
		}
		result.Check.Reason = new(reason)
		return result
	}
	if cfg.PHP.Runtime != "8.3" {
		result.Check.Status, result.Check.Reason = "unavailable", new("runtime_unsupported")
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "runtime_unsupported", Stage: "runtime", Checker: new("psalm"), Message: "The pinned Psalm integration currently requires PHP 8.3.16 or later in the 8.3 series; it does not fall back to another runtime"})
		return result
	}
	native := analyze()
	eligible := (scope.Selection{Files: native.SourceFiles}).Filter(settings.Include, settings.Exclude, func(string) bool { return true })
	return normalize(native, selected, eligible.Files, Policy{Required: settings.Required, FailOn: settings.FailOn})
}

func ValidFinding(finding report.Finding, selection scope.Selection, cfg config.Config) bool {
	if finding.Kind != "code" || !strings.HasPrefix(finding.Rule, "psalm.Tainted") || finding.Path == nil || finding.Line == nil || finding.Fingerprint == nil || len(finding.Trace) == 0 || report.ValidateTrace(finding.Trace) != nil {
		return false
	}
	files := []string{*finding.Path}
	for _, step := range finding.Trace {
		if step.Origin == "project" && step.Path != nil {
			files = append(files, *step.Path)
		}
	}
	eligible := (scope.Selection{Files: files}).Filter(cfg.Scope.Include, cfg.Scope.Exclude, func(name string) bool { return phpFile(name, cfg.Profile) })
	eligible = eligible.Filter(cfg.Tools.Psalm.Include, cfg.Tools.Psalm.Exclude, func(string) bool { return true })
	if len(eligible.Files) == 0 {
		return false
	}
	selected := Select(selection, cfg)
	if len(selected.ProjectFiles) > 0 || slices.ContainsFunc(selected.Files, metadataFile) {
		return true
	}
	return Relevant(finding, selected, nil)
}
