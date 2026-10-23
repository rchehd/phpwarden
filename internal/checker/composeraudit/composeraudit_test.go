package composeraudit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
)

const auditReport = `{"advisories":{"acme/lib":[{"advisoryId":"PKSA-0001","packageName":"acme/lib","title":"Remote code execution","cve":"CVE-2026-0001","link":"https://example.test/1","severity":"high"}],
"drupal/core":{"0":{"advisoryId":"SA-CORE-2026-016","packageName":"drupal/core","title":"Drupal core - Cross-site scripting","cve":null,"link":"https://www.drupal.org/sa-core-2026-016","severity":null}}},
"abandoned":{"acme/old":"acme/new","acme/gone":null},"filter":[]}`

func settings(t *testing.T) config.ComposerAudit {
	t.Helper()
	cfg, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Config.Tools.ComposerAudit
}

func project(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range map[string]string{
		"composer.json": `{"require":{"acme/lib":"^1.0"}}`,
		"composer.lock": `{"packages":[{"name":"acme/lib","version":"1.2.0"},{"name":"drupal/core","version":"11.3.16"}],"packages-dev":[{"name":"acme/old","version":"v2.0.0"}]}`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func fetcher(calls *int, data string, err error) Fetcher {
	return func(_ context.Context, composerJSON, composerLock []byte, noDev bool) ([]byte, error) {
		*calls++
		if !strings.Contains(string(composerJSON), "acme/lib") || !strings.Contains(string(composerLock), "drupal/core") || noDev {
			panic("unexpected audit input")
		}
		return []byte(data), err
	}
}

func TestAuditFindingsCacheAndFreshness(t *testing.T) {
	root, cacheDir, calls := project(t), t.TempDir(), 0
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	cfg := settings(t)
	result := Run(context.Background(), root, cfg, fetcher(&calls, auditReport, nil), cacheDir, now)
	messages := []string{}
	for _, finding := range result.Findings {
		if finding.Kind != "global" || finding.Path != nil || finding.Line != nil {
			t.Fatalf("audit finding must be global: %+v", finding)
		}
		messages = append(messages, finding.Rule+"|"+finding.Severity+"|"+finding.Message)
	}
	want := []string{
		`PKSA-0001|warning|Composer file "composer.lock": acme/lib 1.2.0: Remote code execution (CVE-2026-0001) https://example.test/1`,
		`SA-CORE-2026-016|warning|Composer file "composer.lock": drupal/core 11.3.16: Drupal core - Cross-site scripting https://www.drupal.org/sa-core-2026-016`,
		`composer.abandoned|info|Composer file "composer.lock": acme/gone is abandoned; no replacement is suggested`,
		`composer.abandoned|info|Composer file "composer.lock": acme/old is abandoned; use acme/new instead`,
	}
	if calls != 1 || result.Check.Status != "passed" || len(result.Diagnostics) != 0 || !slices.Equal(messages, want) ||
		result.Advisories.Source != "network" || result.Advisories.FetchedAt != "2026-10-23T12:00:00Z" {
		t.Fatalf("unexpected audit result %v: %+v", messages, result)
	}
	failing := fetcher(&calls, "", errors.New("network down"))
	result = Run(context.Background(), root, cfg, failing, cacheDir, now.Add(23*time.Hour))
	if calls != 1 || result.Advisories.Source != "cache" || len(result.Findings) != 4 || len(result.Diagnostics) != 0 {
		t.Fatalf("fresh cache not used: %+v", result)
	}
	result = Run(context.Background(), root, cfg, failing, cacheDir, now.Add(25*time.Hour))
	if calls != 2 || result.Check.Status != "passed" || result.Advisories.Source != "cache" || result.Diagnostics[0].Code != "advisories_stale" ||
		!strings.Contains(result.Diagnostics[0].Message, "network down") {
		t.Fatalf("stale cache not reported: %+v", result)
	}
	result = Run(context.Background(), root, cfg, failing, cacheDir, now.Add(169*time.Hour))
	if result.Check.Status != "unavailable" || result.Diagnostics[0].Code != "advisories_unavailable" || len(result.Findings) != 0 {
		t.Fatalf("expired cache reported as clean: %+v", result)
	}
	cfg.Offline = true
	result = Run(context.Background(), root, cfg, failing, cacheDir, now.Add(time.Hour))
	if calls != 3 || result.Advisories.Source != "cache" {
		t.Fatalf("offline mode with fresh cache failed: %+v", result)
	}
	result = Run(context.Background(), root, cfg, failing, cacheDir, now.Add(30*time.Hour))
	if calls != 3 || result.Diagnostics[0].Code != "advisories_stale" || !strings.Contains(result.Diagnostics[0].Message, "offline") {
		t.Fatalf("offline mode fetched advisories: %+v", result)
	}
	if err := os.WriteFile(filepath.Join(root, "composer.lock"), []byte(`{"packages":[{"name":"drupal/core","version":"11.3.17"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if result = Run(context.Background(), root, cfg, failing, cacheDir, now.Add(time.Hour)); result.Check.Status != "unavailable" {
		t.Fatalf("cache ignored a changed lock file: %+v", result)
	}
}

func TestAuditPolicies(t *testing.T) {
	root, calls := project(t), 0
	now := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	run := func(cfg config.ComposerAudit) []string {
		t.Helper()
		result := Run(context.Background(), root, cfg, fetcher(&calls, auditReport, nil), t.TempDir(), now)
		rules := []string{result.Check.Status}
		for _, finding := range result.Findings {
			rules = append(rules, finding.Rule)
			if strings.Contains(finding.Message, "ignore expired") {
				rules = append(rules, "expired")
			}
		}
		return rules
	}
	cfg := settings(t)
	cfg.Ignore = []config.ComposerAuditIgnore{{ID: "cve-2026-0001", Reason: "Patched", Until: "2026-10-23"}, {ID: "SA-CORE-2026-016", Reason: "Old", Until: "2026-10-22"}}
	cfg.Abandoned = "ignore"
	if got := run(cfg); !slices.Equal(got, []string{"passed", "SA-CORE-2026-016", "expired"}) {
		t.Fatalf("ignore policy failed: %v", got)
	}
	cfg = settings(t)
	cfg.FailOn = "warning"
	if got := run(cfg); got[0] != "failed" {
		t.Fatalf("fail_on warning did not block advisories: %v", got)
	}
	cfg.Ignore = []config.ComposerAuditIgnore{{ID: "PKSA-0001", Reason: "Accepted"}, {ID: "SA-CORE-2026-016", Reason: "Accepted"}}
	if got := run(cfg); got[0] != "passed" {
		t.Fatalf("abandoned packages blocked the audit: %v", got)
	}
}

func TestAuditSkipsAndRejectsInvalidReports(t *testing.T) {
	root, calls := project(t), 0
	now := time.Now()
	cfg := settings(t)
	cfg.Enabled = false
	if result := Run(context.Background(), root, cfg, fetcher(&calls, auditReport, nil), t.TempDir(), now); *result.Check.Reason != "disabled" {
		t.Fatalf("disabled audit ran: %+v", result)
	}
	if result := Delegated(root, settings(t)); *result.Check.Reason != "delegated" || result.Check.Status != "skipped" {
		t.Fatalf("audit not delegated: %+v", result)
	}
	for _, data := range []string{`{`, `{"abandoned":[]}`, `{"advisories":{"acme/lib":[{"advisoryId":""}]}}`, `{"advisories":{"acme/lib":"x"}}`} {
		cacheDir := t.TempDir()
		result := Run(context.Background(), root, settings(t), fetcher(&calls, data, nil), cacheDir, now)
		entries, _ := os.ReadDir(cacheDir)
		if result.Check.Status != "error" || result.Diagnostics[0].Code != "invalid_report" || len(entries) != 0 {
			t.Fatalf("invalid report %s accepted or cached: %+v", data, result)
		}
	}
	if result := Run(context.Background(), root, settings(t), fetcher(&calls, `{"advisories":[],"abandoned":[]}`, nil), "", now); result.Check.Status != "passed" ||
		result.Diagnostics[0].Code != "advisories_cache_unwritable" {
		t.Fatalf("empty report or missing cache directory mishandled: %+v", result)
	}
	if err := os.Remove(filepath.Join(root, "composer.lock")); err != nil {
		t.Fatal(err)
	}
	if result := Run(context.Background(), root, settings(t), fetcher(&calls, auditReport, nil), t.TempDir(), now); *result.Check.Reason != "no_files" {
		t.Fatalf("audit without composer.lock ran: %+v", result)
	}
}

type processFunc func(context.Context, runner.Command) runner.Result

func (f processFunc) Run(ctx context.Context, cmd runner.Command) runner.Result { return f(ctx, cmd) }

func TestNativeFetcherIsolatesComposerFiles(t *testing.T) {
	bundle := t.TempDir()
	if err := os.MkdirAll(filepath.Join(bundle, "vendor/composer/composer/bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bundle, "vendor/composer/composer/bin/composer"), []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"valid", "vulnerable", "missing PHP", "crash", "invalid"} {
		t.Run(name, func(t *testing.T) {
			var workdir string
			process := processFunc(func(_ context.Context, cmd runner.Command) runner.Result {
				workdir = cmd.Directory
				data, err := os.ReadFile(filepath.Join(workdir, "composer.lock"))
				if err != nil || string(data) != "lock" || !slices.Contains(cmd.Args, "--no-plugins") || !slices.Contains(cmd.Args, "--no-scripts") ||
					!slices.Contains(cmd.Args, "--locked") || !slices.Contains(cmd.Args, "--no-dev") || !slices.ContainsFunc(cmd.Env, func(value string) bool { return strings.HasPrefix(value, "COMPOSER_HOME="+filepath.Dir(workdir)) }) {
					t.Fatalf("unsafe Composer invocation: %+v", cmd)
				}
				switch name {
				case "vulnerable":
					return runner.Result{Stdout: []byte(auditReport), ExitCode: 3}
				case "missing PHP":
					return runner.Result{Unavailable: true}
				case "crash":
					return runner.Result{ExitCode: 100, Stderr: []byte("\nIn HttpDownloader.php line 332:\n  Network disabled\naudit [--no-dev]\n")}
				case "invalid":
					return runner.Result{Stdout: []byte(`{"filter":[]}`)}
				}
				return runner.Result{Stdout: []byte(`{"advisories":[],"abandoned":[]}` + "\n")}
			})
			output, err := NativeFetcher(bundle, process)(context.Background(), []byte("json"), []byte("lock"), true)
			if _, statErr := os.Stat(workdir); !os.IsNotExist(statErr) {
				t.Fatal("audit working directory leaked")
			}
			switch name {
			case "valid", "vulnerable":
				if err != nil || len(output) == 0 {
					t.Fatalf("valid audit rejected: %v", err)
				}
			case "crash":
				if err == nil || !strings.Contains(err.Error(), "Network disabled") {
					t.Fatalf("Composer failure hidden: %v", err)
				}
			default:
				if err == nil {
					t.Fatal("Composer failure accepted")
				}
			}
		})
	}
}
