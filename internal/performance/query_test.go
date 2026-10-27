package performance

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func openFixture(t *testing.T, options Options) *Reader {
	t.Helper()
	reader, err := Open(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reader.Close() })
	return reader
}

func TestQueriesExposeValidatedMetadataFunctionsCallersAndGraph(t *testing.T) {
	reader := openFixture(t, Options{Directory: "../../tools/spx/fixtures", Producer: Producer, Workspace: "../../tools/spx/fixtures", ContainerRoot: "/fixture"})
	ctx := context.Background()
	list := reader.Query(ctx, Query{View: "reports", Limit: 1})
	if !list.Complete || list.Total != 2 || list.Returned != 1 || list.Profiles[0].Key != "cached" || list.NextOffset == nil || *list.NextOffset != 1 {
		t.Fatalf("%+v", list)
	}
	page := reader.Query(ctx, Query{View: "reports", Offset: *list.NextOffset, Limit: 1})
	if !page.Complete || page.Profiles[0].Key != "repeated" || page.NextOffset != nil {
		t.Fatalf("%+v", page)
	}
	meta := reader.Query(ctx, Query{View: "metadata", Key: "repeated"})
	if !meta.Complete || len(meta.Snapshot) != 64 || meta.Returned != 1 || meta.Profiles[0].Calls != 50 || meta.Profiles[0].WallMS != 398.357845 {
		t.Fatalf("%+v", meta)
	}
	flat := reader.Query(ctx, Query{View: "flat", Key: "repeated", Snapshot: meta.Snapshot, Limit: 1})
	if !flat.Complete || flat.Total != 4 || flat.Returned != 1 || flat.Functions[0].Name != "phpwarden_expensive_lookup" || flat.Functions[0].ID != 3 || flat.Functions[0].Calls != 24 || flat.Functions[0].File != "scenario.php" || flat.Functions[0].Line != 2 || flat.NextOffset == nil {
		t.Fatalf("%+v", flat)
	}
	id := flat.Functions[0].ID
	callers := reader.Query(ctx, Query{View: "callers", Key: "repeated", Snapshot: meta.Snapshot, FunctionID: &id})
	if !callers.Complete || callers.Total != 1 || callers.Callers[0].Name != "phpwarden_render_item" || callers.Callers[0].Calls != 24 || !strings.Contains(strings.Join(callers.Notes, " "), "direct aggregated") {
		t.Fatalf("%+v", callers)
	}
	root := 1
	graph := reader.Query(ctx, Query{View: "graph", Key: "repeated", Snapshot: meta.Snapshot, FunctionID: &root, Depth: 2, Limit: 1})
	if !graph.Complete || graph.Total != 2 || graph.Returned != 1 || graph.NextOffset == nil || len(graph.Functions) != 2 || graph.Edges[0].CallerID != 1 || graph.Edges[0].CalleeID != 2 || graph.Edges[0].Calls != 24 || !strings.Contains(strings.Join(graph.Notes, " "), "not path timings") {
		t.Fatalf("%+v", graph)
	}
}

func TestQueryFindsFunctionsOutsideCLITopSummary(t *testing.T) {
	dir, options := pair(t)
	var body strings.Builder
	body.WriteString("[events]\n|0|a0\n")
	var clock int64
	for id := 1; id <= 30; id++ {
		clock += 1000
		fmt.Fprintf(&body, "|%d|a%d\n", id, clock)
		clock += int64(id) * 1000
		fmt.Fprintf(&body, "-%d|a%d\n", id, clock)
	}
	clock += 1000
	fmt.Fprintf(&body, "-0|a%d\n[functions]\n", clock)
	for id := 0; id <= 30; id++ {
		fmt.Fprintf(&body, "/fixture/scenario.php:2:function_%02d\n", id)
	}
	put(t, dir, "cached.txt.gz", zipped(t, []byte(body.String())))
	put(t, dir, "cached.json", changedHeader(t, map[string]any{"call_count": 31, "recorded_call_count": 31, "called_function_count": 31, "wall_time_ms": clock / 1000, "enabled_metrics": []string{"wt"}}))
	summary := Run(context.Background(), options)
	if !summary.Complete {
		t.Fatalf("%+v", summary)
	}
	for _, fn := range summary.Profiles[0].Functions {
		if fn.ID == 1 {
			t.Fatal("low-cost function unexpectedly in top summary")
		}
	}
	reader := openFixture(t, options)
	id := 1
	result := reader.Query(context.Background(), Query{View: "flat", Key: "cached", FunctionID: &id})
	if !result.Complete || result.Total != 1 || result.Functions[0].ID != 1 || result.Functions[0].Calls != 1 {
		t.Fatalf("%+v", result)
	}
	result = reader.Query(context.Background(), Query{View: "flat", Key: "cached", Offset: 29, Limit: 1, Sort: "calls"})
	if !result.Complete || result.Total != 31 || result.NextOffset == nil || *result.NextOffset != 30 {
		t.Fatalf("%+v", result)
	}
}

func TestQueryRejectsStaleSnapshotsAndUnbalancedFocusedCaptures(t *testing.T) {
	dir, options := pair(t)
	reader := openFixture(t, options)
	meta := reader.Query(context.Background(), Query{View: "metadata", Key: "cached"})
	put(t, dir, "cached.json", changedHeader(t, map[string]any{"exec_ts": 1793093430}))
	result := reader.Query(context.Background(), Query{View: "flat", Key: "cached", Snapshot: meta.Snapshot})
	if result.Complete || result.ExitCode != 3 || len(result.Functions) != 0 || result.Diagnostics[0].Code != "input_changed" {
		t.Fatalf("%+v", result)
	}
	put(t, dir, "cached.json", fixture(t, "cached.json"))
	put(t, dir, "cached.txt.gz", zipped(t, []byte(strings.Replace(string(raw(t)), "-0|29411|29351|-344\n", "", 1))))
	id := 3
	for _, view := range []string{"flat", "callers", "graph", "metadata"} {
		result = reader.Query(context.Background(), Query{View: view, Key: "cached", FunctionID: &id})
		if result.Complete || result.ExitCode != 3 || result.Snapshot != "" || len(result.Functions)+len(result.Callers)+len(result.Edges)+len(result.Profiles) != 0 {
			t.Fatalf("%s: %+v", view, result)
		}
	}
}

func TestReaderPinsDirectoryAndRejectsSymlinkedMetadata(t *testing.T) {
	dir, options := pair(t)
	reader := openFixture(t, options)
	moved := dir + "-old"
	if err := os.Rename(dir, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(moved) })
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "cached.json", []byte(`{"private":"outside data"}`))
	result := reader.Query(context.Background(), Query{View: "metadata", Key: "cached"})
	if result.Complete || len(result.Profiles) > 0 || result.Diagnostics[0].Code != "input_changed" {
		t.Fatalf("%+v", result)
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte("outside data")) {
		t.Fatal("directory swap escaped the pinned root")
	}
	dir, options = pair(t)
	reader = openFixture(t, options)
	if err := os.Remove(filepath.Join(dir, "cached.json")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.json")
	put(t, filepath.Dir(outside), filepath.Base(outside), []byte("private-token"))
	if err := os.Symlink(outside, filepath.Join(dir, "cached.json")); err != nil {
		t.Fatal(err)
	}
	result = reader.Query(context.Background(), Query{View: "metadata", Key: "cached"})
	if result.Complete || result.Diagnostics[0].Code != "unsafe_input" {
		t.Fatalf("%+v", result)
	}
}

func TestReaderPinsWorkspaceAndRetainsNoQueryRowsAfterReplacement(t *testing.T) {
	_, options := pair(t)
	source := t.TempDir()
	put(t, source, "scenario.php", []byte("<?php"))
	options.Workspace = source
	options.ContainerRoot = "/fixture"
	reader := openFixture(t, options)
	moved := source + "-old"
	if err := os.Rename(source, moved); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(moved) })
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	put(t, source, "scenario.php", []byte("private replacement"))
	result := reader.Query(context.Background(), Query{View: "flat", Key: "cached"})
	if result.Complete || result.ExitCode != 3 || len(result.Functions) != 0 || len(result.Profiles) != 0 || result.Diagnostics[0].Code != "input_changed" {
		t.Fatalf("%+v", result)
	}
}

func TestQueryValidationPartialListingAndClosedReader(t *testing.T) {
	dir, options := pair(t)
	reader := openFixture(t, options)
	for _, q := range []Query{{View: "unknown"}, {View: "metadata", Key: "../secret"}, {View: "flat", Key: "cached", Limit: 51}, {View: "flat", Key: "cached", Offset: -1}, {View: "flat", Key: "cached", Sort: "unknown"}, {View: "graph", Key: "cached", Depth: 6}, {View: "callers", Key: "cached"}, {View: "metadata", Key: "cached", Snapshot: "bad"}, {View: "flat", Key: "cached", Filter: strings.Repeat("x", 129)}} {
		result := reader.Query(context.Background(), q)
		if result.Complete || result.ExitCode != 2 {
			t.Fatalf("%+v: %+v", q, result)
		}
	}
	id := 88
	if result := reader.Query(context.Background(), Query{View: "flat", Key: "cached", FunctionID: &id}); result.Complete || result.ExitCode != 2 || result.Diagnostics[0].Code != "function_not_found" {
		t.Fatalf("%+v", result)
	}
	put(t, dir, "broken.json", []byte("{bad"))
	list := reader.Query(context.Background(), Query{View: "reports"})
	if list.Complete || list.ExitCode != 3 || len(list.Profiles) != 1 || len(list.Diagnostics) != 1 {
		t.Fatalf("%+v", list)
	}
	if selected := reader.Query(context.Background(), Query{View: "metadata", Key: "cached"}); !selected.Complete || selected.Scope != "selected_report" {
		t.Fatalf("%+v", selected)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if result := reader.Query(context.Background(), Query{View: "reports"}); result.Complete || result.ExitCode != 3 {
		t.Fatalf("%+v", result)
	}
}

func TestGraphRecursionAndRestrictedReader(t *testing.T) {
	_, options := pair(t)
	options.Key = "cached"
	reader := openFixture(t, options)
	if r := reader.Query(context.Background(), Query{View: "metadata", Key: "repeated"}); r.Complete || r.ExitCode != 2 {
		t.Fatalf("%+v", r)
	}
	list := reader.Query(context.Background(), Query{View: "reports"})
	if !list.Complete || list.Total != 1 || list.Scope != "selected_report" {
		t.Fatalf("%+v", list)
	}
	id := 0
	capture := captureView{data: parsed{functions: []functionInfo{{name: "recursive"}}, stats: []statistics{{calls: 3, self: 100, inclusive: 100}}, edges: map[edge]int64{{-1, 0}: 1, {0, 0}: 2}, wall: 100}}
	result := QueryResult{Functions: []FunctionSummary{}, Edges: []CallEdge{}}
	result.graph(capture, Query{FunctionID: &id, Depth: 2, Limit: 20})
	if result.Total != 1 || len(result.Edges) != 1 || result.Edges[0].Calls != 2 || len(result.Functions) != 1 || result.Functions[0].Calls != 3 {
		t.Fatalf("%+v", result)
	}
}
