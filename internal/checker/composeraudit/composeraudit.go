package composeraudit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/cache"
	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
)

const ComposerVersion = "2.10.3"

// Fetcher returns Composer's raw `audit --format=json` output for the given composer.json and composer.lock contents.
type Fetcher func(ctx context.Context, composerJSON, composerLock []byte, noDev bool) ([]byte, error)

type cacheEntry struct {
	Version   int             `json:"version"`
	Key       string          `json:"key"`
	FetchedAt time.Time       `json:"fetched_at"`
	Report    json.RawMessage `json:"report"`
}

type advisory struct {
	AdvisoryID  string  `json:"advisoryId"`
	PackageName string  `json:"packageName"`
	Title       string  `json:"title"`
	CVE         *string `json:"cve"`
	Link        *string `json:"link"`
}

func CacheDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "phpwarden", "advisories"), nil
}

// Delegated reports whether the whole-lock audit applies, so a network-less worker can leave it to the launcher.
func Delegated(root string, cfg config.ComposerAudit) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "composer-audit", Status: "skipped", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	reason := "delegated"
	if !cfg.Enabled {
		reason = "disabled"
	} else if _, err := os.Lstat(filepath.Join(root, "composer.lock")); errors.Is(err, os.ErrNotExist) {
		reason = "no_files"
	}
	result.Check.Reason = &reason
	return result
}

func Run(ctx context.Context, root string, cfg config.ComposerAudit, fetch Fetcher, cacheDir string, now time.Time) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "composer-audit", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	if skipped := Delegated(root, cfg); *skipped.Check.Reason != "delegated" {
		return skipped
	}
	fail := func(status, code, text string) checker.Outcome {
		result.Check.Status, result.Check.Reason = status, &code
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Checker: new("composer-audit"), Message: text})
		return result
	}
	composerJSON, err := os.ReadFile(filepath.Join(root, "composer.json"))
	if err != nil {
		return fail("error", "config_invalid", "Composer audit needs composer.json beside composer.lock")
	}
	composerLock, err := os.ReadFile(filepath.Join(root, "composer.lock"))
	if err != nil {
		return fail("error", "config_invalid", "Composer audit cannot read composer.lock")
	}
	key := fmt.Sprintf("%x", sha256.Sum256(fmt.Appendf(nil, "%d:%s\x00%d:%s\x00%s\x00%t", len(composerJSON), composerJSON, len(composerLock), composerLock, ComposerVersion, cfg.NoDev)))
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	var shared *cache.Store
	cacheErr := fmt.Errorf("no user cache directory is available")
	if cacheDir != "" {
		shared, cacheErr = cache.Open(cacheDir)
		if cacheErr == nil {
			defer shared.Close()
			release, lockErr := shared.Lock(ctx, key)
			cacheErr = lockErr
			if lockErr == nil {
				defer release()
			} else {
				shared = nil
			}
		} else {
			shared = nil
		}
	}
	if ctx.Err() != nil {
		return fail("error", "checker_timeout", "Composer audit cache wait was interrupted")
	}
	var cached *cacheEntry
	if shared != nil {
		if data, readErr := shared.Read(key); readErr == nil {
			var entry cacheEntry
			if json.Unmarshal(data, &entry) == nil && entry.Version == 1 && entry.Key == key && !entry.FetchedAt.After(now) {
				cached = &entry
			}
		}
	}
	age := func(hours int) bool {
		return cached != nil && now.Sub(cached.FetchedAt) <= time.Duration(hours)*time.Hour
	}
	var raw json.RawMessage
	advisories := &report.Advisories{Source: "cache"}
	if age(cfg.MaxAgeHours) {
		raw, advisories.FetchedAt = cached.Report, cached.FetchedAt.UTC().Format(time.RFC3339)
	} else {
		fetchErr := errors.New("offline mode is enabled")
		if !cfg.Offline {
			fetchCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
			raw, fetchErr = fetch(fetchCtx, composerJSON, composerLock, cfg.NoDev)
			cancel()
		}
		if ctx.Err() != nil {
			return fail("error", "checker_timeout", "Composer audit execution was interrupted")
		}
		if fetchErr == nil {
			if _, _, err := findings(raw, composerLock, cfg, now); err != nil {
				return fail("error", "invalid_report", err.Error())
			}
			advisories.Source, advisories.FetchedAt = "network", now.UTC().Format(time.RFC3339)
			err := cacheErr
			if shared != nil {
				data, _ := json.Marshal(cacheEntry{Version: 1, Key: key, FetchedAt: now.UTC(), Report: raw})
				err = shared.Write(key, data)
			}
			if err != nil {
				result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "advisories_cache_unwritable", Stage: "checker", Checker: new("composer-audit"), Message: err.Error()})
			}
		} else if age(cfg.MaxStaleHours) {
			raw, advisories.FetchedAt = cached.Report, cached.FetchedAt.UTC().Format(time.RFC3339)
			result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "advisories_stale", Stage: "checker", Checker: new("composer-audit"),
				Message: fmt.Sprintf("Using Composer advisories cached at %s: %v", advisories.FetchedAt, fetchErr)})
		} else {
			return fail("unavailable", "advisories_unavailable", fmt.Sprintf("Composer advisories are unavailable and no cached result is recent enough: %v", fetchErr))
		}
	}
	rows, blocking, err := findings(raw, composerLock, cfg, now)
	if err != nil {
		return fail("error", "invalid_report", err.Error())
	}
	result.Findings, result.Advisories = rows, advisories
	if blocking {
		result.Check.Status = "failed"
	}
	return result
}

// Composer encodes an empty map as [] and advisory lists either as arrays or as objects keyed by index.
func values(raw json.RawMessage) ([]json.RawMessage, map[string]json.RawMessage, error) {
	var list []json.RawMessage
	if err := json.Unmarshal(raw, &list); err == nil {
		return list, nil, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, nil, err
	}
	return nil, object, nil
}

func findings(raw json.RawMessage, composerLock []byte, cfg config.ComposerAudit, now time.Time) ([]report.Finding, bool, error) {
	var parsed struct {
		Advisories *json.RawMessage `json:"advisories"`
		Abandoned  *json.RawMessage `json:"abandoned"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil || parsed.Advisories == nil {
		return nil, false, fmt.Errorf("expected a Composer audit JSON report with advisories")
	}
	var lock struct {
		Packages    []struct{ Name, Version string } `json:"packages"`
		PackagesDev []struct{ Name, Version string } `json:"packages-dev"`
	}
	if err := json.Unmarshal(composerLock, &lock); err != nil {
		return nil, false, fmt.Errorf("cannot read composer.lock: %w", err)
	}
	versions := map[string]string{}
	for _, pkg := range append(lock.Packages, lock.PackagesDev...) {
		versions[strings.ToLower(pkg.Name)] = pkg.Version
	}
	_, packages, err := values(*parsed.Advisories)
	if err != nil {
		return nil, false, fmt.Errorf("invalid Composer advisories: %w", err)
	}
	today := now.UTC().Format(time.DateOnly)
	result := []report.Finding{}
	blocking := false
	for _, name := range slices.Sorted(maps.Keys(packages)) {
		list, object, err := values(packages[name])
		if err != nil {
			return nil, false, fmt.Errorf("invalid Composer advisories for %s: %w", name, err)
		}
		for _, key := range slices.Sorted(maps.Keys(object)) {
			list = append(list, object[key])
		}
		for _, item := range list {
			var row advisory
			if err := json.Unmarshal(item, &row); err != nil || row.AdvisoryID == "" || row.Title == "" {
				return nil, false, fmt.Errorf("invalid Composer advisory for %s", name)
			}
			note := ""
			ignored := false
			for _, ignore := range cfg.Ignore {
				if strings.EqualFold(ignore.ID, row.AdvisoryID) || row.CVE != nil && strings.EqualFold(ignore.ID, *row.CVE) {
					if ignore.Until == "" || today <= ignore.Until {
						ignored = true
					} else {
						note = fmt.Sprintf(" (ignore expired %s: %s)", ignore.Until, ignore.Reason)
					}
				}
			}
			if ignored {
				continue
			}
			message := fmt.Sprintf("Composer file \"composer.lock\": %s %s: %s", name, versions[strings.ToLower(name)], row.Title)
			if row.CVE != nil && *row.CVE != "" {
				message += " (" + *row.CVE + ")"
			}
			if row.Link != nil && *row.Link != "" {
				message += " " + *row.Link
			}
			result = append(result, report.Finding{Checker: "composer-audit", Rule: row.AdvisoryID, Kind: "global", Severity: "warning", Message: message + note})
			blocking = blocking || cfg.FailOn == "warning"
		}
	}
	if parsed.Abandoned != nil && cfg.Abandoned == "report" {
		_, abandoned, err := values(*parsed.Abandoned)
		if err != nil {
			return nil, false, fmt.Errorf("invalid Composer abandoned packages: %w", err)
		}
		for _, name := range slices.Sorted(maps.Keys(abandoned)) {
			var replacement *string
			if err := json.Unmarshal(abandoned[name], &replacement); err != nil {
				return nil, false, fmt.Errorf("invalid Composer abandoned package %s", name)
			}
			message := fmt.Sprintf("Composer file \"composer.lock\": %s is abandoned; no replacement is suggested", name)
			if replacement != nil && *replacement != "" {
				message = fmt.Sprintf("Composer file \"composer.lock\": %s is abandoned; use %s instead", name, *replacement)
			}
			result = append(result, report.Finding{Checker: "composer-audit", Rule: "composer.abandoned", Kind: "global", Severity: "info", Message: message})
		}
	}
	return result, blocking, nil
}
