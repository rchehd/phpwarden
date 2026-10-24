package launcher

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rchehd/phpwarden/internal/cache"
	"github.com/rchehd/phpwarden/internal/checker/composeraudit"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/runrequest"
	"github.com/rchehd/phpwarden/internal/scope"
	"github.com/rchehd/phpwarden/internal/toolset"
	"github.com/rchehd/phpwarden/internal/worker"
)

func cachedFixture(t *testing.T) (config.Location, config.Effective, scope.Options) {
	t.Helper()
	location, _, options := fixture(t)
	data, _ := os.ReadFile(location.Path)
	data = []byte(strings.Replace(string(data), `"version":1`, `"version":1,"runner":{"result_cache":true}`, 1))
	if err := os.WriteFile(location.Path, data, 0600); err != nil {
		t.Fatal(err)
	}
	effective, err := config.Load(location)
	if err != nil {
		t.Fatal(err)
	}
	return location, effective, options
}

func TestResultCacheReusesOnlyCurrentCompleteResults(t *testing.T) {
	location, effective, options := cachedFixture(t)
	var calls []runner.Command
	process := dockerDouble(t, location, &calls)
	run := func(fresh bool) report.Report {
		if fresh {
			return CheckFresh(context.Background(), location, effective, options, []string{"debug"}, process)
		}
		return Check(context.Background(), location, effective, options, []string{"debug"}, process)
	}
	first, second := run(false), run(false)
	if !first.Complete || first.Environment.Cache.Status != "miss" || second.Environment.Cache.Status != "hit" || first.RunID == second.RunID || second.ExitCode != 1 || len(second.Findings) != 1 {
		t.Fatalf("cache changed report identity or verdict: %+v / %+v", first, second)
	}
	runs := func() int {
		n := 0
		for _, c := range calls {
			if c.Args[0] == "run" {
				n++
			}
		}
		return n
	}
	if runs() != 1 {
		t.Fatal("cache hit launched a worker")
	}
	if result := run(true); result.Environment.Cache != nil || runs() != 2 {
		t.Fatal("fresh check used result cache")
	}
	filename := filepath.Join(location.Root, "src/demo.php")
	info, _ := os.Stat(filename)
	if err := os.WriteFile(filename, []byte("<?php\nvar_dump(2);\n"), 0644); err != nil {
		t.Fatal(err)
	}
	os.Chtimes(filename, info.ModTime(), info.ModTime())
	if result := run(false); result.Environment.Cache.Status != "miss" || runs() != 3 {
		t.Fatal("same-size same-mtime content change reused cache")
	}
	directory, _ := cache.Directory("results")
	if err := os.WriteFile(filepath.Join(directory, resultKeyForReport(t, location, effective, options)+".json"), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if result := run(false); result.Environment.Cache.Status != "miss" || runs() != 4 {
		t.Fatal("corrupt entry returned success")
	}
}

func resultKeyForReport(t *testing.T, location config.Location, effective config.Effective, options scope.Options) string {
	t.Helper()
	selection, err := scope.Build(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	request, err := runrequest.Build(location, effective, selection, []string{"debug"}, testImage, "unused")
	if err != nil {
		t.Fatal(err)
	}
	selected, err := toolset.Resolve(effective)
	if err != nil {
		t.Fatal(err)
	}
	request.Toolset, request.RuntimeVersion = selected.ID, selected.PHPVersion
	return resultKey(location.Root, request)
}

func TestResultCacheConcurrentChecksAndCancelledWait(t *testing.T) {
	location, effective, options := cachedFixture(t)
	var runs atomic.Int32
	entered, unblock := make(chan struct{}), make(chan struct{})
	process := processFunc(func(ctx context.Context, cmd runner.Command) runner.Result {
		if cmd.Args[0] == "run" {
			if runs.Add(1) == 1 {
				close(entered)
				<-unblock
			}
		}
		var calls []runner.Command
		return dockerDouble(t, location, &calls)(ctx, cmd)
	})
	done := make(chan report.Report, 1)
	go func() { done <- Check(context.Background(), location, effective, options, []string{"debug"}, process) }()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	interrupted := Check(ctx, location, effective, options, []string{"debug"}, process)
	cancel()
	if interrupted.Complete || interrupted.ExitCode != 3 || runs.Load() != 1 {
		t.Fatalf("cancelled cache waiter ran worker: %+v", interrupted)
	}
	close(unblock)
	first := <-done
	second := Check(context.Background(), location, effective, options, []string{"debug"}, process)
	if !first.Complete || second.Environment.Cache.Status != "hit" || runs.Load() != 1 {
		t.Fatal("shared key did not coalesce worker runs")
	}
}

func TestResultCacheRejectsExpiredAndIncompleteEntries(t *testing.T) {
	location, effective, options := cachedFixture(t)
	selection, _ := scope.Build(context.Background(), options)
	request, err := runrequest.Build(location, effective, selection, []string{"debug"}, testImage, toolset.DevelopmentID("8.4"))
	if err != nil {
		t.Fatal(err)
	}
	state, err := openResultCache(context.Background(), location, request, true)
	if err != nil {
		t.Fatal(err)
	}
	defer state.Close()
	result := report.New()
	result.Complete = false
	state.Write(result, time.Now())
	if _, err := state.store.Read(state.info.Key); !os.IsNotExist(err) {
		t.Fatal("cached incomplete report")
	}
	var calls []runner.Command
	execution := dockerDouble(t, location, &calls)(context.Background(), runner.Command{Name: "docker", Args: []string{"run", "--read-only", "--network=none", "--cap-drop=ALL", "--security-opt=no-new-privileges:true", "--pull=never", testImage, "--mount", `type=bind,"source=` + location.Root + `",target=/workspace,readonly`}, Input: mustJSON(t, request)})
	if err := json.Unmarshal(execution.Stdout, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Complete {
		t.Fatalf("expected complete fixture: %+v", result)
	}
	now := time.Now()
	state.Write(result, now.Add(-25*time.Hour))
	if _, hit := state.Read(request, now); hit {
		t.Fatal("accepted expired cache")
	}
	state.Write(result, now.Add(time.Hour))
	if _, hit := state.Read(request, now); hit {
		t.Fatal("accepted future cache")
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestResultKeyIncludesContextAndBypassesLiveCheckers(t *testing.T) {
	location, effective, options := cachedFixture(t)
	selection, _ := scope.Build(context.Background(), options)
	request, err := runrequest.Build(location, effective, selection, []string{"debug"}, testImage, toolset.DevelopmentID("8.4"))
	if err != nil {
		t.Fatal(err)
	}
	key := resultKey(location.Root, request)
	for name, mutate := range map[string]func(*runrequest.Request){
		"run":     func(r *runrequest.Request) { r.RunID = "another" },
		"content": func(r *runrequest.Request) { r.Fingerprint["file:src/demo.php"] = strings.Repeat("b", 64) },
		"config":  func(r *runrequest.Request) { r.Config.Tools.Debug.FailOn = "never" },
		"scope":   func(r *runrequest.Request) { r.Selection.Mode = "paths" },
		"image":   func(r *runrequest.Request) { r.Image = "sha256:" + strings.Repeat("b", 64) },
		"toolset": func(r *runrequest.Request) { r.Toolset = "other" },
		"checker": func(r *runrequest.Request) { r.Only = []string{"whitespace"} },
	} {
		var changed runrequest.Request
		json.Unmarshal(mustJSON(t, request), &changed)
		mutate(&changed)
		if (resultKey(location.Root, changed) == key) != (name == "run") {
			t.Errorf("wrong key dependency: %s", name)
		}
	}
	if resultKey(location.Root+"other", request) == key {
		t.Fatal("cache crosses project roots")
	}
	request.Only = []string{"phpstan"}
	if cacheEligibility(request) != "live_checker:phpstan" {
		t.Fatal("cached executable project context")
	}
	request.Only = []string{"debug", "composer-audit"}
	if cacheEligibility(request) != "" {
		t.Fatal("delegated audit should refresh after result cache lookup")
	}
}

func TestFixLockCancellationDoesNotLaunchOrChangeFiles(t *testing.T) {
	location, effective, options := fixture(t)
	release, err := cache.LockProject(context.Background(), location.Root)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	before, _ := os.ReadFile(filepath.Join(location.Root, "src/demo.php"))
	for _, fix := range []func(context.Context, config.Location, config.Effective, scope.Options, []string, runner.Process) report.Report{Fix, worker.Fix} {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		result := fix(ctx, location, effective, options, []string{"phpcs"}, processFunc(func(context.Context, runner.Command) runner.Result {
			t.Error("locked fix launched process")
			return runner.Result{}
		}))
		cancel()
		if result.Complete || result.ExitCode != 3 || len(result.FixedFiles) > 0 || result.Diagnostics[0].Code != "fix_lock" {
			t.Fatalf("fix lock not respected: %+v", result)
		}
	}
	after, _ := os.ReadFile(filepath.Join(location.Root, "src/demo.php"))
	if string(before) != string(after) {
		t.Fatal("cancelled fix changed source")
	}
}

func TestCachedWorkerStillRefreshesDelegatedAudit(t *testing.T) {
	location, effective, options := cachedFixture(t)
	os.WriteFile(filepath.Join(location.Root, "composer.json"), []byte(`{"require":{}}`), 0600)
	os.WriteFile(filepath.Join(location.Root, "composer.lock"), []byte(`{"packages":[],"packages-dev":[]}`), 0600)
	var calls []runner.Command
	base := dockerDouble(t, location, &calls)
	audits, workers := 0, 0
	process := processFunc(func(ctx context.Context, cmd runner.Command) runner.Result {
		if cmd.Args[0] == "run" {
			if slices.Contains(cmd.Args, "audit") {
				audits++
				return runner.Result{Stdout: []byte(`{"advisories":[],"abandoned":[]}`)}
			}
			workers++
		}
		return base(ctx, cmd)
	})
	only := []string{"debug", "composer-audit"}
	first := Check(context.Background(), location, effective, options, only, process)
	if !first.Complete || first.Environment.Advisories == nil {
		t.Fatalf("initial audit failed: %+v", first)
	}
	directory, _ := composeraudit.CacheDir()
	entries, _ := os.ReadDir(directory)
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			os.Remove(filepath.Join(directory, entry.Name()))
		}
	}
	second := Check(context.Background(), location, effective, options, only, process)
	if !second.Complete || second.Environment.Cache.Status != "hit" || audits != 2 || workers != 1 {
		t.Fatalf("cached worker hid advisory refresh: %+v audits=%d workers=%d", second, audits, workers)
	}
}
