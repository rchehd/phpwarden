package packagecheck

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/phpconstraint"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

//go:embed helper.php
var helper []byte

func Run(ctx context.Context, directory string, selected scope.Selection, cfg config.Config, id string, process runner.Process) checker.Outcome {
	executable, _ := os.Executable()
	bundle := filepath.Clean(filepath.Join(filepath.Dir(executable), "..", "lib", "phpwarden", id))
	if id == "composer-require-checker" {
		bundle = filepath.Join(bundle, "php"+cfg.PHP.Runtime)
	}
	return run(ctx, directory, selected, cfg, id, bundle, process)
}

func run(ctx context.Context, directory string, selected scope.Selection, cfg config.Config, id, bundle string, process runner.Process) checker.Outcome {
	settings := Settings(cfg, id)
	result := checker.Outcome{Check: report.Check{ID: id, Status: "passed", Required: settings.Required, FailOn: settings.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	skip := func(reason string) checker.Outcome {
		result.Check.Status, result.Check.Reason = "skipped", &reason
		return result
	}
	fail := func(status, code, message string) checker.Outcome {
		result.Check.Status, result.Check.Reason, result.Findings = status, &code, []report.Finding{}
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: &id, Message: message})
		return result
	}
	if !settings.Enabled {
		return skip("disabled")
	}
	if selected.Mode != "full" {
		return skip("full_scan_required")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(settings.TimeoutSeconds)*time.Second)
	defer cancel()
	prepared, reason, err := prepare(directory, cfg, id)
	if err != nil {
		status := "error"
		if reason == "dependencies_missing" {
			status = "unavailable"
		}
		return fail(status, reason, err.Error())
	}
	if reason != "" {
		return skip(reason)
	}
	if _, err := os.Stat(filepath.Join(bundle, "vendor/autoload.php")); err != nil {
		return fail("unavailable", "dependencies_missing", "The isolated dependency checker bundle is unavailable; rebuild the pinned toolset")
	}
	temporary, err := os.MkdirTemp("", "phpwarden-package-")
	if err != nil {
		return fail("error", "helper_prepare", "Cannot prepare dependency checker helper")
	}
	defer os.RemoveAll(temporary)
	script := filepath.Join(temporary, "helper.php")
	if os.WriteFile(script, helper, 0o600) != nil {
		return fail("error", "helper_prepare", "Cannot write dependency checker helper")
	}
	input, _ := json.Marshal(prepared)
	env := []string{"HOME=" + temporary, "TMPDIR=" + temporary, "LANG=C.UTF-8"}
	for _, key := range []string{"PATH", "SYSTEMROOT", "WINDIR"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	execution := process.Run(ctx, runner.Command{Name: "php", Args: []string{"-d", "auto_prepend_file=", "-d", "auto_append_file=", "-d", "display_errors=stderr", "-d", "log_errors=0", "-d", fmt.Sprintf("memory_limit=%dM", settings.MemoryLimitMB), script, bundle}, Directory: prepared.Root, Env: env, Input: input})
	if execution.Unavailable {
		return fail("unavailable", "executable_missing", "PHP is unavailable in the worker")
	}
	if execution.TimedOut || ctx.Err() != nil {
		return fail("error", "checker_timeout", "Dependency analysis exceeded its execution context")
	}
	if execution.ExitCode != 0 || execution.Err != nil || len(execution.Stderr) > 0 {
		return fail("error", "process_failed", "Dependency analysis could not complete; no partial findings accepted")
	}
	findings, runtime, php, err := decode(execution.Stdout, prepared, path.Join(Root(cfg, id), "composer.json"))
	if err != nil {
		return fail("error", "invalid_report", err.Error())
	}
	result.Findings, result.Runtime, result.PHPVersion = findings, &runtime, &php
	if len(findings) > 0 && settings.FailOn == "warning" {
		result.Check.Status = "failed"
	}
	return result
}

func strictJSON(data []byte, target any) error {
	if len(data) > 32<<20 {
		return fmt.Errorf("oversized dependency checker report")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(target); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("expected one dependency checker response")
	}
	return nil
}

func decode(data []byte, prepared plan, composer string) ([]report.Finding, string, string, error) {
	failure := func() ([]report.Finding, string, string, error) {
		return nil, "", "", fmt.Errorf("Dependency checker returned an invalid, incomplete or mismatched package report")
	}
	var envelope struct {
		Version int             `json:"version"`
		Tool    string          `json:"tool_version"`
		Runtime string          `json:"runtime"`
		PHP     string          `json:"php_version"`
		Status  *int            `json:"status"`
		Data    json.RawMessage `json:"data"`
	}
	if strictJSON(data, &envelope) != nil || envelope.Version != 1 || envelope.Tool != prepared.Version || envelope.Runtime == "" || !strings.HasPrefix(envelope.PHP, envelope.Runtime+".") || envelope.Status == nil || (*envelope.Status != 0 && *envelope.Status != 1) {
		return failure()
	}
	canonical, versionErr := phpconstraint.StableVersion(envelope.PHP)
	if versionErr != nil || canonical != envelope.PHP || envelope.Runtime != envelope.PHP[:strings.LastIndex(envelope.PHP, ".")] {
		return failure()
	}
	result := []report.Finding{}
	add := func(rule, message string) {
		result = append(result, report.Finding{Checker: prepared.ID, Rule: rule, Kind: "global", Severity: "warning", Message: fmt.Sprintf("Composer package %q: %s", composer, message)})
	}
	if prepared.ID == "composer-require-checker" {
		var native struct {
			Meta struct {
				Tool struct {
					Version string `json:"version"`
				} `json:"composer-require-checker"`
				Date string `json:"date"`
			} `json:"_meta"`
			Unknown json.RawMessage `json:"unknown-symbols"`
		}
		if strictJSON(envelope.Data, &native) != nil || native.Meta.Tool.Version != prepared.Version || native.Meta.Date == "" || native.Unknown == nil {
			return failure()
		}
		unknown := map[string][]string{}
		if string(native.Unknown) != "[]" && strictJSON(native.Unknown, &unknown) != nil {
			return failure()
		}
		if unknown == nil || (*envelope.Status == 0) != (len(unknown) == 0) {
			return failure()
		}
		names := []string{}
		for name := range unknown {
			if strings.TrimSpace(name) == "" {
				return failure()
			}
			names = append(names, name)
		}
		slices.Sort(names)
		for _, name := range names {
			add("composer-require-checker.unknown_symbol", "Unknown or undeclared dependency symbol: "+name)
		}
	} else {
		var native struct {
			Used []struct {
				Name      string   `json:"name"`
				Required  []string `json:"required-by,omitempty"`
				Suggested []string `json:"suggested-by,omitempty"`
			} `json:"used-packages"`
			Unused  []string `json:"unused-packages"`
			Ignored []string `json:"ignored-packages"`
			Zombie  []string `json:"zombie-exclusions"`
		}
		if string(envelope.Data) == "[]" {
			envelope.Data = []byte("{}")
		}
		if strictJSON(envelope.Data, &native) != nil || (*envelope.Status == 0) != (len(native.Unused)+len(native.Zombie) == 0) {
			return failure()
		}
		seen := map[string]bool{}
		all := append(slices.Clone(native.Unused), native.Ignored...)
		for _, row := range native.Used {
			all = append(all, row.Name)
		}
		for _, name := range all {
			if !slices.Contains(prepared.Requires, name) || seen[name] {
				return failure()
			}
			seen[name] = true
		}
		if len(seen) != len(prepared.Requires) {
			return failure()
		}
		for _, name := range native.Unused {
			add("composer-unused.candidate", "Dependency may be unused; review Drupal discovery and configuration before removing: "+name)
		}
		for _, name := range native.Zombie {
			if strings.TrimSpace(name) == "" {
				return failure()
			}
			add("composer-unused.unmatched_exclusion", "Unused package exclusion: "+name)
		}
	}
	return result, envelope.Runtime, envelope.PHP, nil
}
