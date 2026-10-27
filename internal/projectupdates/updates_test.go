package projectupdates

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rchehd/phpwarden/internal/cache"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/runner"
)

const supportFixture = `<table><tr><td><a>8.3</a></td><td>23 Nov 2023</td><td>age</td><td>31 Dec 2025</td><td>age</td><td>31 Dec 2027</td></tr><tr><td>8.4</td><td>21 Nov 2024</td><td>age</td><td>31 Dec 2026</td><td>age</td><td>31 Dec 2028</td></tr></table>`
const manifestFixture = `{"require":{"drupal/core":"^11","acme/direct":"^1"},"require-dev":{"acme/dev":"^1"}}`
const lockFixture = `{"packages":[{"name":"drupal/core","version":"11.0.0"},{"name":"acme/direct","version":"1.0.0"},{"name":"acme/transitive","version":"1.0.0"}],"packages-dev":[{"name":"acme/dev","version":"1.0.0"}]}`
const outdatedFixture = `{"locked":[{"name":"drupal/core","version":"11.0.0","latest":"11.0.1"},{"name":"acme/direct","version":"1.0.0","latest":"2.0.0"},{"name":"acme/dev","version":"1.0.0","latest":"1.2.0"}]}`
const auditFixture = `{"advisories":{"drupal/core":[{"advisoryId":"PKSA-demo","title":"Example advisory","affectedVersions":"<11.0.1","link":"https://example.test/advisory","severity":"high"}]},"ignored-advisories":{"acme/dev":[{"advisoryId":"PKSA-ignored","title":"Accepted project exception","affectedVersions":"<1.1.0"}]},"abandoned":{"acme/transitive":null}}`

func envelope(outdated, audit string) []byte {
	data, _ := json.Marshal(ComposerData{Outdated: json.RawMessage(outdated), Audit: json.RawMessage(audit)})
	return data
}

func requestFixture(t *testing.T) Request {
	t.Helper()
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "platform"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"composer.json": manifestFixture, "composer.lock": lockFixture} {
		if err := os.WriteFile(filepath.Join(root, "platform", name), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return Request{Location: config.Location{Root: root}, ComposerRoot: "platform", Toolset: "test-v1", Branch: "8.3", Settings: config.Updates{TimeoutSeconds: 5, MaxAgeHours: 24, MaxStaleHours: 168}, Now: time.Date(2026, 10, 25, 0, 0, 0, 0, time.UTC), HTTP: func(_ context.Context, address string) ([]byte, error) {
		if strings.Contains(address, "supported-versions") {
			return []byte(supportFixture), nil
		}
		return []byte(`{"8.3.36":{"date":"21 Oct 2026","tags":["security"]}}`), nil
	}, Composer: func(context.Context, []byte, []byte) ([]byte, error) {
		return envelope(outdatedFixture, auditFixture), nil
	}}
}

func TestUpdateInventoryFiltersAndRuntimeCertainty(t *testing.T) {
	for _, test := range []struct {
		options Options
		names   []string
	}{
		{Options{}, []string{"acme/dev", "drupal/core", "acme/direct", "acme/transitive"}},
		{Options{Security: true}, []string{"acme/dev", "drupal/core"}},
		{Options{Direct: true, NoDev: true}, []string{"drupal/core", "acme/direct"}},
		{Options{Security: true, NoDev: true, RuntimeVersion: "8.3.20"}, []string{"drupal/core"}},
	} {
		t.Run(strings.Join(test.names, ","), func(t *testing.T) {
			request := requestFixture(t)
			request.Options = test.options
			result := Run(context.Background(), request)
			if !result.Complete || result.ExitCode != 0 || len(result.Packages) != len(test.names) {
				t.Fatalf("%+v", result)
			}
			for i, name := range test.names {
				if result.Packages[i].Name != name {
					t.Fatalf("%+v", result.Packages)
				}
			}
			if result.PHP.Support != "security" || !result.PHP.SecurityRelease {
				t.Fatalf("PHP: %+v", result.PHP)
			}
			if test.options.RuntimeVersion == "" {
				if result.PHP.Current != nil || result.PHP.UpdateAvailable != nil {
					t.Fatal("invented application runtime")
				}
			} else if result.PHP.UpdateAvailable == nil || !*result.PHP.UpdateAvailable {
				t.Fatal("missed PHP patch update")
			}
			for _, item := range result.Packages {
				if item.Name == "acme/dev" && !item.Advisories[0].Ignored {
					t.Fatal("lost accepted advisory marker")
				}
			}
			data, _ := os.ReadFile(filepath.Join(request.Location.Root, "platform/composer.json"))
			if string(data) != manifestFixture {
				t.Fatal("update query modified manifest")
			}
		})
	}
}

func TestCacheFreshOfflineStaleExpiredAndInputIdentity(t *testing.T) {
	request := requestFixture(t)
	calls := 0
	fetch := request.Composer
	request.Composer = func(ctx context.Context, a, b []byte) ([]byte, error) { calls++; return fetch(ctx, a, b) }
	if result := Run(context.Background(), request); !result.Complete {
		t.Fatalf("%+v", result)
	}
	request.Settings.Offline = true
	request.HTTP = func(context.Context, string) ([]byte, error) { t.Fatal("offline queried network"); return nil, nil }
	request.Composer = func(context.Context, []byte, []byte) ([]byte, error) {
		t.Fatal("offline queried Composer")
		return nil, nil
	}
	if result := Run(context.Background(), request); !result.Complete || result.Sources[2].Status != "cache" {
		t.Fatalf("%+v", result)
	}
	request.Now = request.Now.Add(48 * time.Hour)
	if result := Run(context.Background(), request); result.Complete || result.ExitCode != 3 || result.Sources[2].Status != "stale" || len(result.Packages) == 0 {
		t.Fatalf("%+v", result)
	}
	request.Now = request.Now.Add(168 * time.Hour)
	if result := Run(context.Background(), request); result.Complete || len(result.Packages) > 0 || result.Sources[2].Status != "unavailable" {
		t.Fatalf("%+v", result)
	}
	request.Now = request.Now.Add(-216 * time.Hour)
	request.Toolset = "changed"
	if result := Run(context.Background(), request); result.Complete || result.Sources[2].Status != "unavailable" {
		t.Fatalf("toolset identity ignored %+v", result)
	}
	if calls != 1 {
		t.Fatalf("fetch count %d", calls)
	}
}

func TestPartialSourcesAndChangedInputsNeverClaimComplete(t *testing.T) {
	for _, cause := range []string{"network", "malformed", "unreachable", "diagnostics", "path", "changed", "none_matched"} {
		t.Run(cause, func(t *testing.T) {
			request := requestFixture(t)
			switch cause {
			case "network":
				request.Composer = func(context.Context, []byte, []byte) ([]byte, error) { return nil, errors.New("unavailable") }
			case "malformed":
				request.HTTP = func(context.Context, string) ([]byte, error) { return []byte("unexpected upstream response"), nil }
			case "unreachable":
				request.Composer = func(context.Context, []byte, []byte) ([]byte, error) {
					return envelope(outdatedFixture, `{"advisories":[],"unreachable-repositories":["private"]}`), nil
				}
			case "diagnostics":
				request.Composer = func(context.Context, []byte, []byte) ([]byte, error) {
					data, _ := json.Marshal(ComposerData{Outdated: json.RawMessage(outdatedFixture), Audit: json.RawMessage(auditFixture), Diagnostics: true})
					return data, nil
				}
			case "path":
				data := strings.Replace(lockFixture, `"name":"acme/direct"`, `"dist":{"type":"path"},"name":"acme/direct"`, 1)
				os.WriteFile(filepath.Join(request.Location.Root, "platform/composer.lock"), []byte(data), 0o644)
			case "changed":
				request.Composer = func(context.Context, []byte, []byte) ([]byte, error) {
					os.WriteFile(filepath.Join(request.Location.Root, "platform/composer.json"), []byte(manifestFixture+"\n"), 0o644)
					return envelope(outdatedFixture, auditFixture), nil
				}
			case "none_matched":
				request.Composer = func(context.Context, []byte, []byte) ([]byte, error) {
					return envelope(strings.Replace(outdatedFixture, `"2.0.0"`, `"[none matched]"`, 1), auditFixture), nil
				}
			}
			if result := Run(context.Background(), request); result.Complete || result.ExitCode != 3 {
				t.Fatalf("false complete: %+v", result)
			}
		})
	}
}

func TestInvalidComposerRowsAndRuntime(t *testing.T) {
	for _, outdated := range []string{`{}`, `{"locked":[{"name":"other/package","version":"1.0.0","latest":"2.0.0"}]}`, strings.Replace(outdatedFixture, `"version":"11.0.0"`, `"version":"10.0.0"`, 1)} {
		packages, local, _ := inventory([]byte(manifestFixture), []byte(lockFixture))
		if _, err := parseComposer(envelope(outdated, auditFixture), packages, local); err == nil {
			t.Fatalf("accepted %s", outdated)
		}
	}
	for _, version := range []string{"8.3", "8.4.1", "garbage", "8.3.1-dev"} {
		if ValidateRuntime(version, "8.3") == nil {
			t.Fatalf("accepted %s", version)
		}
	}
	if _, err := parseSupport([]byte(strings.Replace(supportFixture, "31 Dec 2027", "bad date", 1))); err == nil {
		t.Fatal("accepted invalid lifecycle")
	}
	if _, _, err := parseRelease([]byte(`{"8.4.1":{"date":"today"}}`), "8.3"); err == nil {
		t.Fatal("accepted wrong release branch")
	}
}

func TestPHPSupportBoundariesMissingRowsAndEndOfLife(t *testing.T) {
	for _, test := range []struct{ branch, day, want string }{
		{"8.3", "2025-12-31", "active"},
		{"8.3", "2026-01-01", "security"},
		{"8.3", "2028-01-01", "eol"},
		{"8.1", "2026-10-25", "eol"},
		{"8.5", "2026-10-25", "unknown"},
	} {
		t.Run(test.branch+test.day, func(t *testing.T) {
			request := requestFixture(t)
			request.Branch = test.branch
			request.Now, _ = time.Parse("2006-01-02", test.day)
			request.HTTP = func(_ context.Context, address string) ([]byte, error) {
				if strings.Contains(address, "supported-versions") {
					return []byte(supportFixture), nil
				}
				return []byte(`{"` + test.branch + `.36":{"date":"21 Oct 2026"}}`), nil
			}
			result := Report{Complete: true}
			phpMetadata(context.Background(), request, &result)
			if result.PHP.Support != test.want || result.Complete != (test.want != "unknown") {
				t.Fatalf("%+v", result)
			}
		})
	}
	if _, err := parseSupport([]byte(supportFixture + `<tr><td>8.5</td><td>changed format</td></tr>`)); err == nil {
		t.Fatal("partially changed support table accepted")
	}
}

type processFunc func(context.Context, runner.Command) runner.Result

func (process processFunc) Run(ctx context.Context, command runner.Command) runner.Result {
	return process(ctx, command)
}

func TestNativeComposerUsesPrivateCopiesAndDisablesProjectExecution(t *testing.T) {
	bundle := t.TempDir()
	binary := filepath.Join(bundle, "vendor/composer/composer/bin/composer")
	if err := os.MkdirAll(filepath.Dir(binary), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := `{"require":{"acme/direct":"*"},"repositories":[{"type":"path","url":"../private/*"},{"packagist.org":false}]}`
	lock := `{"packages":[{"name":"acme/direct","version":"1.0.0","dist":{"type":"path"}}]}`
	directories := []string{}
	process := processFunc(func(_ context.Context, command runner.Command) runner.Result {
		directories = append(directories, command.Directory)
		args := strings.Join(command.Args, " ")
		if !strings.Contains(args, "--locked") || !strings.Contains(args, "--no-plugins") || !strings.Contains(args, "--no-scripts") {
			t.Fatalf("unsafe invocation %+v", command)
		}
		data, err := os.ReadFile(filepath.Join(command.Directory, "composer.json"))
		if err != nil || strings.Contains(string(data), "../private") {
			t.Fatalf("unsafe copied manifest %s %v", data, err)
		}
		if strings.Contains(args, "outdated") {
			if !strings.Contains(args, "--ignore-platform-reqs") {
				t.Fatal("candidate lookup uses the worker platform instead of release metadata")
			}
			if !strings.Contains(args, "--ignore=acme/direct") {
				t.Fatal("path package not excluded from remote lookup")
			}
			return runner.Result{Stdout: []byte(`{"locked":[]}`)}
		}
		if strings.Contains(args, "--ignore-platform-reqs") {
			t.Fatal("candidate lookup options leaked into the advisory query")
		}
		return runner.Result{Stdout: []byte(`{"advisories":[],"abandoned":[]}`), Stderr: []byte("partial repository warning")}
	})
	data, err := NativeComposer(bundle, process)(context.Background(), []byte(manifest), []byte(lock))
	if err != nil {
		t.Fatal(err)
	}
	var result ComposerData
	json.Unmarshal(data, &result)
	if len(directories) != 2 || !result.Diagnostics {
		t.Fatalf("lost audit diagnostics %s", data)
	}
	for _, name := range directories {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatal("temporary project copies remain")
		}
	}
}

func TestWorkerPlatformFilteredCacheIsRefreshed(t *testing.T) {
	request := requestFixture(t)
	directory, err := cache.Directory("updates")
	if err != nil {
		t.Fatal(err)
	}
	store, err := cache.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	identity := fmt.Sprintf("%x", sha256.Sum256([]byte(manifestFixture+lockFixture))) + "/" + request.Toolset
	key := fmt.Sprintf("%x", sha256.Sum256([]byte("updates-v1/composer/"+identity)))
	data, _ := json.Marshal(entry{Version: 1, Fetched: request.Now, Data: envelope(outdatedFixture, auditFixture)})
	if err := store.Write(key, data); err != nil {
		t.Fatal(err)
	}
	fetched := false
	request.Composer = func(context.Context, []byte, []byte) ([]byte, error) {
		fetched = true
		return envelope(strings.Replace(outdatedFixture, `"2.0.0"`, `"3.0.0"`, 1), auditFixture), nil
	}
	result := Run(context.Background(), request)
	if !result.Complete || !fetched {
		t.Fatalf("worker-filtered candidates reused: %+v", result)
	}
	for _, item := range result.Packages {
		if item.Name == "acme/direct" && item.Latest != "3.0.0" {
			t.Fatalf("stale candidate retained: %+v", item)
		}
	}
}

func TestProfileReleaseStatusFilters(t *testing.T) {
	for _, test := range []struct {
		options  Options
		expected string
	}{
		{Options{Security: true}, "insecure,policy,unsupported"},
		{Options{Security: true, Direct: true}, "insecure,unsupported"},
		{Options{Security: true, Direct: true, NoDev: true}, "insecure"},
	} {
		request := requestFixture(t)
		request.Options = test.options
		uncovered := false
		request.Profile = func(context.Context, Metadata) ProfileReport {
			return ProfileReport{Complete: true, Projects: []Project{
				{ID: "insecure", Status: "insecure", Direct: true},
				{ID: "policy", Status: "supported", SecurityCovered: &uncovered},
				{ID: "unsupported", Status: "unsupported", Direct: true, Dev: true},
				{ID: "supported", Status: "supported", Direct: true},
			}}
		}
		result := Run(context.Background(), request)
		ids := []string{}
		for _, item := range result.Projects {
			ids = append(ids, item.ID)
		}
		if !result.Complete || strings.Join(ids, ",") != test.expected {
			t.Fatalf("filters=%+v result=%+v", test.options, result)
		}
	}
}

func TestLivePublicMetadataAndComposer(t *testing.T) {
	if os.Getenv("PHPWARDEN_UPDATES_LIVE") != "1" {
		t.Skip("explicit opt-in network assessment")
	}
	request := requestFixture(t)
	request.HTTP = nil
	request.Now = time.Now()
	request.Settings.TimeoutSeconds = 90
	manifest := `{"require":{"psr/log":"^3.0"},"config":{"platform":{"php":"5.6.0"}}}`
	lock := `{"packages":[{"name":"psr/log","version":"3.0.0","require":{"php":">=8.0.0"},"type":"library","license":["MIT"]}],"packages-dev":[]}`
	os.WriteFile(filepath.Join(request.Location.Root, "platform/composer.json"), []byte(manifest), 0o644)
	os.WriteFile(filepath.Join(request.Location.Root, "platform/composer.lock"), []byte(lock), 0o644)
	bundle, err := filepath.Abs("../../tools/composer")
	if err != nil {
		t.Fatal(err)
	}
	request.Composer = NativeComposer(bundle, runner.Exec{})
	result := Run(context.Background(), request)
	encoded, _ := json.Marshal(result)
	t.Log(string(encoded))
	if !result.Complete || result.ExitCode != 0 || result.PHP.Latest == "" || result.PHP.Support == "unknown" || len(result.Packages) != 1 || result.Packages[0].Name != "psr/log" || result.Packages[0].UpdateType != "patch" {
		t.Fatalf("live assessment incomplete: %s", encoded)
	}
}
