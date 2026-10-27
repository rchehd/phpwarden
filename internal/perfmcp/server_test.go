package perfmcp

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rchehd/phpwarden/internal/performance"
)

func fixtureServer(t *testing.T, dir string) (*Server, *mcp.ClientSession) {
	t.Helper()
	if dir == "" {
		dir = "../../tools/spx/fixtures"
	}
	server, err := New(performance.Options{Directory: dir, Producer: performance.Producer, Workspace: "../../tools/spx/fixtures", ContainerRoot: "/fixture"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	st, ct := mcp.NewInMemoryTransports()
	ss, err := server.server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "phpwarden-test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close(); ss.Close(); server.Close(); cancel() })
	return server, cs
}

func call(t *testing.T, client *mcp.ClientSession, name string, args map[string]any) (*mcp.CallToolResult, performance.QueryResult) {
	t.Helper()
	result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out performance.QueryResult
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%v: %s", err, data)
	}
	return result, out
}

func TestOfficialSDKDiscoveryAndAllFiveTools(t *testing.T) {
	_, client := fixtureServer(t, "")
	tools, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 5 {
		t.Fatalf("%+v", tools)
	}
	for _, tool := range tools.Tools {
		if tool.OutputSchema == nil || tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint {
			t.Fatalf("unsafe tool contract: %+v", tool)
		}
		data, _ := json.Marshal(tool.InputSchema)
		if !bytes.Contains(data, []byte(`"additionalProperties":false`)) {
			t.Fatalf("open input schema: %s", data)
		}
	}
	res, list := call(t, client, "find_reports", map[string]any{"limit": 1})
	if res.IsError || !list.Complete || list.Total != 2 || list.Returned != 1 || list.NextOffset == nil {
		t.Fatalf("%+v %+v", res, list)
	}
	res, meta := call(t, client, "get_report_metadata", map[string]any{"report_key": "repeated"})
	if res.IsError || len(meta.Snapshot) != 64 || meta.Profiles[0].Calls != 50 {
		t.Fatalf("%+v", meta)
	}
	res, flat := call(t, client, "get_flat_profile", map[string]any{"report_key": "repeated", "snapshot": meta.Snapshot, "limit": 1})
	if res.IsError || flat.Total != 4 || flat.Functions[0].Calls != 24 || flat.Functions[0].ID != 3 || !flat.Functions[0].Mapped {
		t.Fatalf("%+v", flat)
	}
	res, callers := call(t, client, "get_callers", map[string]any{"report_key": "repeated", "snapshot": meta.Snapshot, "function_id": 3})
	if res.IsError || callers.Callers[0].Name != "phpwarden_render_item" || callers.Callers[0].Calls != 24 {
		t.Fatalf("%+v", callers)
	}
	res, graph := call(t, client, "get_aggregated_call_graph", map[string]any{"report_key": "repeated", "function_id": 1, "depth": 2})
	if res.IsError || graph.Total != 2 || len(graph.Edges) != 2 || len(graph.Functions) != 3 {
		t.Fatalf("%+v", graph)
	}
	if len(res.Content) != 1 {
		t.Fatalf("missing text compatibility output: %+v", res)
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("missing JSON text fallback")
	}
	var fallback performance.QueryResult
	if json.Unmarshal([]byte(text.Text), &fallback) != nil || fallback.Snapshot != graph.Snapshot {
		t.Fatal("text fallback differs from structured content")
	}
}

func TestSchemasRejectPathsCommandsInvalidIDsAndOversizedQueries(t *testing.T) {
	_, client := fixtureServer(t, "")
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"get_report_metadata", map[string]any{"report_key": "../../etc/passwd"}},
		{"get_report_metadata", map[string]any{"report_key": "cached", "project": "/outside"}},
		{"get_flat_profile", map[string]any{"report_key": "cached", "command": "docker build"}},
		{"get_callers", map[string]any{"report_key": "cached"}},
		{"get_callers", map[string]any{"report_key": "cached", "function_id": -1}},
		{"get_flat_profile", map[string]any{"report_key": "cached", "limit": 0}},
		{"get_flat_profile", map[string]any{"report_key": "cached", "limit": 51}},
		{"get_flat_profile", map[string]any{"report_key": "cached", "function_contains": strings.Repeat("x", 129)}},
		{"get_aggregated_call_graph", map[string]any{"report_key": "cached", "depth": 6}},
	} {
		result, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: test.name, Arguments: test.args})
		if err != nil || !result.IsError {
			t.Fatalf("%s %v: %+v %v", test.name, test.args, result, err)
		}
	}
	for _, name := range []string{"setup", "record", "execute", "teardown"} {
		r, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
		if err == nil && (r == nil || !r.IsError) {
			t.Fatalf("exposed runtime mutation %s: %+v %v", name, r, err)
		}
	}
}

func TestIncompleteReportsAndBusyQueriesAreToolErrors(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"cached.json", "cached.txt.gz"} {
		data, err := os.ReadFile(filepath.Join("../../tools/spx/fixtures", name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{bad"), 0600); err != nil {
		t.Fatal(err)
	}
	server, client := fixtureServer(t, dir)
	res, list := call(t, client, "find_reports", map[string]any{})
	if !res.IsError || list.Complete || len(list.Profiles) != 1 || len(list.Diagnostics) != 1 {
		t.Fatalf("%+v %+v", res, list)
	}
	res, metadata := call(t, client, "get_report_metadata", map[string]any{"report_key": "cached"})
	if res.IsError || !metadata.Complete {
		t.Fatalf("%+v", metadata)
	}
	server.gate <- struct{}{}
	res, busy := call(t, client, "get_report_metadata", map[string]any{"report_key": "cached"})
	<-server.gate
	if !res.IsError || busy.Complete || busy.Diagnostics[0].Code != "busy" {
		t.Fatalf("%+v", busy)
	}
	res, stale := call(t, client, "get_flat_profile", map[string]any{"report_key": "cached", "snapshot": strings.Repeat("0", 64)})
	if !res.IsError || stale.Complete || stale.Diagnostics[0].Code != "input_changed" || len(stale.Functions) != 0 {
		t.Fatalf("%+v", stale)
	}
}

type discardCloser struct{ io.Writer }

func (discardCloser) Close() error { return nil }

func TestTransportFrameInputAndReplyLimits(t *testing.T) {
	input := &sessionInput{ReadCloser: io.NopCloser(strings.NewReader("12345")), left: 4}
	buf := make([]byte, 10)
	if n, err := input.Read(buf); err != nil || n != 4 {
		t.Fatalf("%d %v", n, err)
	}
	if _, err := input.Read(buf); err == nil {
		t.Fatal("input budget not enforced")
	}
	input = &sessionInput{ReadCloser: io.NopCloser(strings.NewReader("\n\n")), left: 10, frames: MaxSessionFrames - 1}
	if _, err := input.Read(buf); err == nil {
		t.Fatal("frame budget not enforced")
	}
	var result bytes.Buffer
	if n, err := (output{discardCloser{&result}}).Write(make([]byte, MaxReply+1)); err == nil || n != 0 || result.Len() != 0 {
		t.Fatal("oversized response was written")
	}
	server, err := New(performance.Options{Directory: "../../tools/spx/fixtures", Producer: performance.Producer})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = server.Serve(ctx, io.NopCloser(strings.NewReader(strings.Repeat("x", MaxFrame+1)+"\n")), discardCloser{io.Discard})
	if err == nil {
		t.Fatal("accepted an oversized JSON-RPC frame")
	}
}

func TestEmptyDirectoryCanStartAndCancellationClosesStdio(t *testing.T) {
	server, err := New(performance.Options{Directory: t.TempDir(), Producer: performance.Producer})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	input, writer := io.Pipe()
	defer writer.Close()
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, discardCloser{io.Discard}) }()
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled session returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("stdio cancellation hung")
	}
}
