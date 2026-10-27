package drupal

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/projectupdates"
)

func historyFixture(project, status, branches, version, releaseStatus, terms string) []byte {
	return []byte(fmt.Sprintf(`<project><short_name>%s</short_name><project_status>%s</project_status><supported_branches>%s</supported_branches><releases><release><version>%s</version><status>%s</status><release_link>https://www.drupal.org/project/%s/releases/%s</release_link><terms>%s</terms><security covered="1"/></release></releases></project>`, project, status, branches, version, releaseStatus, project, version, terms))
}

func TestDrupalReleaseStatusUsesExactUpstreamEvidence(t *testing.T) {
	for _, test := range []struct {
		name, projectStatus, branches, version, releaseStatus, term, expected string
		complete                                                              bool
	}{
		{"legacy Composer mapping", "published", "8.x-1.", "8.x-1.2", "published", "", "supported", true},
		{"project revoked", "revoked", "", "8.x-1.2", "published", "", "revoked", true},
		{"project unpublished", "unpublished", "", "8.x-1.2", "published", "", "revoked", true},
		{"project insecure", "insecure", "", "8.x-1.2", "published", "", "insecure", true},
		{"project unsupported", "unsupported", "", "8.x-1.2", "published", "", "unsupported", true},
		{"release revoked", "published", "8.x-1.", "8.x-1.2", "unpublished", "", "revoked", true},
		{"release insecure", "published", "8.x-1.", "8.x-1.2", "published", "Insecure", "insecure", true},
		{"release unsupported", "published", "8.x-1.", "8.x-1.2", "published", "Unsupported", "unsupported", true},
		{"unsupported branch", "published", "8.x-2.", "8.x-1.2", "published", "", "unsupported", true},
		{"no branches", "published", "", "8.x-1.2", "published", "", "unknown", false},
		{"missing exact release", "published", "8.x-1.", "8.x-1.3", "published", "", "unknown", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			term := ""
			if test.term != "" {
				term = "<term><name>Release type</name><value>" + test.term + "</value></term>"
			}
			history, err := parseReleaseHistory(historyFixture("token", test.projectStatus, test.branches, test.version, test.releaseStatus, term), "token")
			if err != nil {
				t.Fatal(err)
			}
			item := projectupdates.Project{Current: "1.2.0", Reasons: []string{}}
			complete := assessRelease(&item, history)
			if complete != test.complete || item.Status != test.expected {
				t.Fatalf("complete=%v item=%+v", complete, item)
			}
		})
	}
	for _, version := range []string{"", "dev-1.x", "1.x-dev", "1.2.0-beta1"} {
		item := projectupdates.Project{Current: version}
		history, _ := parseReleaseHistory(historyFixture("token", "published", "8.x-1.", "8.x-1.2", "published", ""), "token")
		if assessRelease(&item, history) || item.Status != "unknown" {
			t.Fatalf("unknown version accepted: %+v", item)
		}
	}
}

func TestMalformedReleaseHistoryDoesNotEstablishSupport(t *testing.T) {
	valid := string(historyFixture("token", "published", "8.x-1.", "8.x-1.2", "published", ""))
	for _, data := range []string{"<error>not found</error>", valid + valid, valid + "garbage", strings.Replace(valid, "token", "other", 1), strings.Replace(valid, "published", "surprise", 1), strings.Replace(valid, `covered="1"`, `covered="unknown"`, 1), strings.Replace(valid, "</releases>", strings.Split(strings.Split(valid, "<releases>")[1], "</releases>")[0]+"</releases>", 1)} {
		if _, err := parseReleaseHistory([]byte(data), "token"); err == nil {
			t.Fatalf("accepted malformed feed: %s", data)
		}
	}
}

func TestReleasePolicyCoverageAndCoreBranchBoundaries(t *testing.T) {
	for _, test := range []struct{ version, branch, coverage, status string }{
		{"11.4.1", "11.4.", "", "supported"},
		{"11.4.1", "11.4.", "0", "supported"},
		{"11.4.1", "11.40.", "1", "unsupported"},
	} {
		data := string(historyFixture("drupal", "published", test.branch, test.version, "published", ""))
		data = strings.Replace(data, `covered="1"`, `covered="`+test.coverage+`"`, 1)
		history, err := parseReleaseHistory([]byte(data), "drupal")
		if err != nil {
			t.Fatal(err)
		}
		item := projectupdates.Project{Current: test.version}
		if !assessRelease(&item, history) || item.Status != test.status {
			t.Fatalf("%+v", item)
		}
		if test.coverage == "" && item.SecurityCovered != nil || test.coverage == "0" && (item.SecurityCovered == nil || *item.SecurityCovered) {
			t.Fatalf("invented advisory coverage: %+v", item)
		}
	}
}

func releaseProject(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, data := range files {
		filename := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestReleaseInventoryCombinesComposerAndPackagedMetadata(t *testing.T) {
	root := releaseProject(t, map[string]string{
		"app/composer.json":                                          `{"require":{"drupal/token":"^1"},"require-dev":{"drupal/devel":"^5"}}`,
		"app/composer.lock":                                          `{"packages":[{"name":"drupal/core","version":"11.4.1"},{"name":"drupal/token","version":"1.2.0","type":"drupal-module"},{"name":"drupal/core-recommended","version":"11.4.1"}],"packages-dev":[{"name":"drupal/devel","version":"5.0.0","type":"drupal-module"}]}`,
		"app/web/core/lib/Drupal.php":                                `<?php class Drupal { const VERSION = '11.4.1'; }`,
		"app/web/modules/contrib/token/token.info.yml":               "project: token\nversion: 8.x-1.2\n",
		"app/web/modules/contrib/token/sub/sub.info.yml":             "project: token\nversion: 8.x-1.2\n",
		"app/web/modules/contrib/devel/devel.info.yml":               "project: devel\nversion: 5.0.0\n",
		"app/web/modules/manual/manual.info.yml":                     "project: manual\nversion: 2.0.0\n",
		"app/web/modules/custom/demo/demo.info.yml":                  "name: Demo\ncore_version_requirement: ^11\n",
		"app/web/modules/contrib/token/tests/fixtures/fake.info.yml": "project: do_not_query\nversion: 9.0.0\n",
		"app/web/sites/default/files/private.info.yml":               "project: do_not_query\nversion: 9.0.0\n",
	})
	items, before, notes, err := updateInventory(root, "app", nil)
	if err != nil || len(items) != 4 {
		t.Fatalf("items=%+v error=%v", items, err)
	}
	for _, item := range items {
		if item.ID == "token" && (!item.Direct || len(item.Paths) != 3 || item.Current != "1.2.0") {
			t.Fatalf("bad merge: %+v", item)
		}
		if item.ID == "devel" && (!item.Dev || !item.Direct) {
			t.Fatalf("lost dev relationship: %+v", item)
		}
		if item.ID == "manual" && (item.Origin != "packaged .info.yml" || item.Direct) {
			t.Fatalf("invented Composer relationship: %+v", item)
		}
	}
	if !strings.Contains(strings.Join(notes, " "), "1 .info.yml") {
		t.Fatal(notes)
	}
	filename := filepath.Join(root, "app/web/modules/manual/manual.info.yml")
	os.WriteFile(filename, []byte("project: manual\nversion: 2.1.0\n"), 0o644)
	_, after, _, err := updateInventory(root, "app", nil)
	if err != nil || before == after {
		t.Fatal("metadata changes are not fingerprinted", err)
	}
}

func TestReleaseInventoryRejectsUnsafeOrAmbiguousMetadata(t *testing.T) {
	for _, data := range []string{"project: ../../private\nversion: 1.0.0\n", "project: token\nproject: other\n", "project: [invalid]\n"} {
		root := releaseProject(t, map[string]string{"modules/demo/demo.info.yml": data})
		if _, _, _, err := updateInventory(root, ".", nil); err == nil {
			t.Fatal("accepted invalid metadata", data)
		}
	}
	root := releaseProject(t, map[string]string{"modules/demo/demo.info.yml": "project: token\nversion: 1.2.0\n"})
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "modules/demo/escape.info.yml")); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := updateInventory(root, ".", nil); err == nil {
		t.Fatal("accepted symlink metadata")
	}
}

func TestNonComposerUpdatesCacheFiltersAndFreshness(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	root := releaseProject(t, map[string]string{"modules/manual/manual.info.yml": "project: manual\nversion: 1.2.0\n"})
	fetches := 0
	fetch := func(context.Context, string) ([]byte, error) {
		fetches++
		return historyFixture("manual", "published", "8.x-2.", "8.x-1.2", "published", ""), nil
	}
	request := projectupdates.Request{
		Location: config.Location{Root: root}, ComposerRoot: ".", Branch: "8.3", Now: time.Now(),
		Settings: config.Updates{TimeoutSeconds: 10, MaxAgeHours: 24, MaxStaleHours: 168},
		Profile:  releaseAssessment(root, ".", nil, fetch),
		HTTP: func(_ context.Context, address string) ([]byte, error) {
			if strings.Contains(address, "supported-versions") {
				return []byte(`<table><tr><td>8.3</td><td>23 Nov 2023</td><td>age</td><td>31 Dec 2025</td><td>age</td><td>31 Dec 2027</td></tr></table>`), nil
			}
			return []byte(`{"8.3.36":{"date":"21 Oct 2026","tags":[]}}`), nil
		},
		Composer: func(context.Context, []byte, []byte) ([]byte, error) {
			t.Fatal("Composer must not run without manifests")
			return nil, nil
		},
	}
	result := projectupdates.Run(context.Background(), request)
	if !result.Complete || result.ExitCode != 0 || fetches != 1 || len(result.Projects) != 1 || result.Projects[0].Status != "unsupported" {
		t.Fatalf("%+v fetches=%d", result, fetches)
	}
	request.Settings.Offline = true
	request.Options.Security = true
	result = projectupdates.Run(context.Background(), request)
	if !result.Complete || fetches != 1 || len(result.Projects) != 1 {
		t.Fatalf("offline/security: %+v", result)
	}
	request.Options.Direct = true
	result = projectupdates.Run(context.Background(), request)
	if !result.Complete || len(result.Projects) != 0 {
		t.Fatalf("unknown direct relationship: %+v", result)
	}
	request.Options.Direct = false
	request.Now = request.Now.Add(48 * time.Hour)
	result = projectupdates.Run(context.Background(), request)
	if result.Complete || result.ExitCode != 3 || result.Sources[2].Status != "stale" {
		t.Fatalf("stale: %+v", result)
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	request.Settings.Offline = false
	request.Now = time.Now()
	request.Profile = releaseAssessment(root, ".", nil, func(ctx context.Context, id string) ([]byte, error) {
		os.WriteFile(filepath.Join(root, "modules/manual/manual.info.yml"), []byte("project: manual\nversion: 2.0.0\n"), 0o644)
		return fetch(ctx, id)
	})
	result = projectupdates.Run(context.Background(), request)
	if result.Complete || !strings.Contains(strings.Join(result.Notes, " "), "inventory changed") {
		t.Fatalf("drift: %+v", result)
	}
}

func TestLiveDrupalReleaseHistory(t *testing.T) {
	if os.Getenv("PHPWARDEN_UPDATES_LIVE") != "1" {
		t.Skip("explicit opt-in public Drupal metadata")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	data, err := fetchReleaseHistory(ctx, "token")
	if err != nil {
		t.Fatal(err)
	}
	history, err := parseReleaseHistory(data, "token")
	if err != nil {
		t.Fatal(err)
	}
	item := projectupdates.Project{Current: "1.12.0"}
	if !assessRelease(&item, history) || item.Status == "unknown" || item.ReleaseURL == "" {
		t.Fatalf("%+v", item)
	}
}
