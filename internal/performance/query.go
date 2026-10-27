package performance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
)

type Reader struct {
	mu      sync.RWMutex
	options Options
	closed  bool
}

func Open(options Options) (*Reader, error) {
	if options.Directory == "" || options.Producer != Producer {
		return nil, fmt.Errorf("supply a reports directory and the supported producer commit")
	}
	if options.Key != "" && !keyPattern.MatchString(options.Key) {
		return nil, fmt.Errorf("invalid report key")
	}
	if (options.Workspace == "") != (options.ContainerRoot == "") || options.ContainerRoot != "" && (!path.IsAbs(options.ContainerRoot) || strings.Contains(options.ContainerRoot, "\\")) {
		return nil, fmt.Errorf("source mapping needs a workspace and an absolute container root")
	}
	var err error
	options.Directory, err = filepath.Abs(options.Directory)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve the reports directory")
	}
	options.directoryRoot, err = os.OpenRoot(options.Directory)
	if err != nil {
		return nil, fmt.Errorf("cannot open the reports directory")
	}
	if options.Workspace != "" {
		options.Workspace, err = filepath.Abs(options.Workspace)
		if err == nil {
			options.workspaceRoot, err = os.OpenRoot(options.Workspace)
		}
		if err != nil {
			options.directoryRoot.Close()
			return nil, fmt.Errorf("cannot open the source workspace")
		}
		options.ContainerRoot = path.Clean(options.ContainerRoot)
	}
	options.Top = 1
	return &Reader{options: options}, nil
}

func (reader *Reader) Close() error {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if reader.closed {
		return nil
	}
	reader.closed = true
	err := reader.options.directoryRoot.Close()
	if reader.options.workspaceRoot != nil {
		if closeErr := reader.options.workspaceRoot.Close(); err == nil {
			err = closeErr
		}
	}
	return err
}

type Query struct {
	View       string
	Key        string
	Snapshot   string
	FunctionID *int
	Filter     string
	Sort       string
	Offset     int
	Limit      int
	Depth      int
}

type Metadata struct {
	Key              string   `json:"key"`
	RecordedAt       string   `json:"recorded_at"`
	Kind             string   `json:"kind"`
	Description      string   `json:"description"`
	Compression      string   `json:"compression"`
	WallMS           float64  `json:"wall_ms"`
	CPUMS            *float64 `json:"cpu_ms,omitempty"`
	CPUOmittedReason string   `json:"cpu_omitted_reason,omitempty"`
	PeakMemory       int64    `json:"peak_memory_bytes"`
	Calls            int64    `json:"calls"`
	Functions        int      `json:"total_functions"`
}

type FunctionSummary struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Calls       int64    `json:"calls"`
	SelfMS      float64  `json:"self_ms"`
	InclusiveMS float64  `json:"inclusive_ms"`
	CPUSelfMS   *float64 `json:"cpu_self_ms,omitempty"`
	Share       float64  `json:"self_percent"`
	File        string   `json:"file,omitempty"`
	Line        int      `json:"definition_line,omitempty"`
	Mapped      bool     `json:"source_mapped"`
}

type CallEdge struct {
	CallerID int   `json:"caller_id"`
	CalleeID int   `json:"callee_id"`
	Calls    int64 `json:"calls"`
}

type QueryResult struct {
	Version        int               `json:"version"`
	View           string            `json:"view"`
	Complete       bool              `json:"complete"`
	ExitCode       int               `json:"exit_code"`
	Scope          string            `json:"scope"`
	Producer       string            `json:"producer"`
	ProducerOrigin string            `json:"producer_origin"`
	Snapshot       string            `json:"snapshot,omitempty"`
	Profiles       []Metadata        `json:"profiles"`
	Functions      []FunctionSummary `json:"functions"`
	Callers        []Caller          `json:"callers"`
	Edges          []CallEdge        `json:"edges"`
	Total          int               `json:"total"`
	Offset         int               `json:"offset"`
	Returned       int               `json:"returned"`
	Omitted        int               `json:"omitted"`
	NextOffset     *int              `json:"next_offset,omitempty"`
	Diagnostics    []Diagnostic      `json:"diagnostics"`
	Notes          []string          `json:"notes"`
}

func QueryError(view, code, message string, exit int) QueryResult {
	return QueryResult{Version: 1, View: view, Complete: false, ExitCode: exit, Scope: "selected_report", Producer: Producer, ProducerOrigin: "explicit operator assertion", Profiles: []Metadata{}, Functions: []FunctionSummary{}, Callers: []Caller{}, Edges: []CallEdge{}, Diagnostics: []Diagnostic{{Code: code, Message: message}}, Notes: initial().Notes}
}

func queryArguments(query *Query) string {
	if !slices.Contains([]string{"reports", "metadata", "flat", "callers", "graph"}, query.View) {
		return "Unknown report view."
	}
	if query.View != "reports" && !keyPattern.MatchString(query.Key) {
		return "A valid report key is required."
	}
	if query.Offset < 0 || query.Offset > 32768 || query.Limit < 0 || query.Limit > 50 {
		return "Offset must be nonnegative and limit must be 1–50."
	}
	if query.Limit == 0 {
		query.Limit = 20
	}
	if query.FunctionID != nil && (*query.FunctionID < 0 || *query.FunctionID >= 8192) {
		return "Function ID must be a report-local ID from 0 to 8191."
	}
	if query.View == "callers" && query.FunctionID == nil {
		return "The callers view requires a function ID."
	}
	if query.Snapshot != "" && !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(query.Snapshot) {
		return "Invalid snapshot token."
	}
	if len(query.Filter) > 128 {
		return "Function filters are limited to 128 bytes."
	}
	if query.Sort == "" {
		query.Sort = "self_ms"
	}
	if !slices.Contains([]string{"self_ms", "inclusive_ms", "calls"}, query.Sort) {
		return "Unsupported function sort order."
	}
	if query.Depth == 0 {
		query.Depth = 2
	}
	if query.Depth < 1 || query.Depth > 5 {
		return "Graph depth must be 1–5."
	}
	return ""
}

func (reader *Reader) Query(ctx context.Context, query Query) QueryResult {
	if message := queryArguments(&query); message != "" {
		return QueryError(query.View, "arguments", message, 2)
	}
	reader.mu.RLock()
	defer reader.mu.RUnlock()
	if reader.closed {
		return QueryError(query.View, "input_unavailable", "Report reader is closed.", 3)
	}
	options := reader.options
	if options.Key != "" && query.Key != "" && options.Key != query.Key {
		return QueryError(query.View, "arguments", "The server is restricted to another report.", 2)
	}
	if query.View != "reports" {
		options.Key = query.Key
	}
	result := QueryResult{Version: 1, View: query.View, Profiles: []Metadata{}, Functions: []FunctionSummary{}, Callers: []Caller{}, Edges: []CallEdge{}, Offset: query.Offset}
	var viewErr error
	validation := run(ctx, options, func(ctx context.Context, capture captureView) {
		if query.View == "reports" {
			return
		}
		result.Snapshot = capture.snapshot
		if query.Snapshot != "" && query.Snapshot != capture.snapshot {
			viewErr = problem("input_changed", "The report changed between queries; request its metadata again.")
			return
		}
		if query.FunctionID != nil && *query.FunctionID >= len(capture.data.functions) {
			viewErr = problem("function_not_found", "The function ID is not present in this report.")
			return
		}
		if ctx.Err() != nil {
			viewErr = ctx.Err()
			return
		}
		switch query.View {
		case "flat":
			result.flat(capture, query)
		case "callers":
			result.callers(capture, query)
		case "graph":
			result.graph(capture, query)
		}
	})
	result.Complete = validation.Complete
	result.ExitCode = validation.ExitCode
	result.Scope = validation.Scope
	result.Producer = validation.Producer
	result.ProducerOrigin = validation.ProducerOrigin
	result.Diagnostics = validation.Diagnostics
	result.Notes = append(validation.Notes, result.Notes...)
	if !validation.Complete {
		result.Snapshot = ""
		result.Functions = []FunctionSummary{}
		result.Callers = []Caller{}
		result.Edges = []CallEdge{}
		result.Total = 0
		result.Returned = 0
		result.Omitted = 0
		result.NextOffset = nil
		if query.View != "reports" {
			return result
		}
	}
	if viewErr != nil {
		diagnostic := diagnosticFor(query.Key, viewErr)
		exit := 3
		if diagnostic.Code == "function_not_found" {
			exit = 2
		}
		if diagnostic.Code == "cancelled" {
			exit = 130
		}
		return QueryError(query.View, diagnostic.Code, diagnostic.Message, exit)
	}
	for _, profile := range validation.Profiles {
		result.Profiles = append(result.Profiles, Metadata{profile.Key, profile.RecordedAt, profile.Kind, profile.Description, profile.Compression, profile.WallMS, profile.CPUMS, profile.CPUOmittedReason, profile.PeakMemory, profile.Calls, profile.TotalFunctions})
	}
	if query.View == "reports" {
		slices.SortFunc(result.Profiles, func(a, b Metadata) int {
			if a.RecordedAt != b.RecordedAt {
				return strings.Compare(b.RecordedAt, a.RecordedAt)
			}
			return strings.Compare(a.Key, b.Key)
		})
		result.Total = len(result.Profiles)
		start := min(query.Offset, result.Total)
		end := min(start+query.Limit, result.Total)
		result.Profiles = result.Profiles[start:end]
		result.page(len(result.Profiles))
	}
	if query.View == "metadata" {
		result.Total = len(result.Profiles)
		result.page(len(result.Profiles))
	}
	return result
}

func (result *QueryResult) page(returned int) {
	result.Returned = returned
	result.Omitted = max(0, result.Total-returned)
	if next := result.Offset + returned; returned > 0 && next < result.Total {
		result.NextOffset = &next
	}
}

func functionSummary(capture captureView, id int) FunctionSummary {
	fn, stats := capture.data.functions[id], capture.data.stats[id]
	file, line, mapped := sourceLocation(capture.workspace, capture.container, fn.file, fn.line)
	row := FunctionSummary{ID: id, Name: functionName(fn), Calls: stats.calls, SelfMS: float64(stats.self) / 1e6, InclusiveMS: float64(stats.inclusive) / 1e6, File: file, Line: line, Mapped: mapped}
	if capture.meta.ct >= 0 && capture.data.cpuReliable {
		value := float64(stats.cpuSelf) / 1e6
		row.CPUSelfMS = &value
	}
	if capture.data.wall > 0 {
		row.Share = float64(stats.self) / float64(capture.data.wall) * 100
	}
	return row
}

func (result *QueryResult) flat(capture captureView, query Query) {
	ids := []int{}
	for id, fn := range capture.data.functions {
		if query.FunctionID != nil && id != *query.FunctionID || query.Filter != "" && !strings.Contains(functionName(fn), query.Filter) {
			continue
		}
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b int) int {
		x, y := capture.data.stats[a].self, capture.data.stats[b].self
		if query.Sort == "calls" {
			x, y = capture.data.stats[a].calls, capture.data.stats[b].calls
		}
		if query.Sort == "inclusive_ms" {
			x, y = capture.data.stats[a].inclusive, capture.data.stats[b].inclusive
		}
		if x > y {
			return -1
		}
		if x < y {
			return 1
		}
		return a - b
	})
	result.Total = len(ids)
	start := min(query.Offset, len(ids))
	budget := 64 << 10
	for _, id := range ids[start:min(start+query.Limit, len(ids))] {
		row := functionSummary(capture, id)
		encoded, _ := json.Marshal(row)
		if len(encoded) > budget {
			break
		}
		budget -= len(encoded)
		result.Functions = append(result.Functions, row)
	}
	result.page(len(result.Functions))
}

func (result *QueryResult) callers(capture captureView, query Query) {
	for pair, calls := range capture.data.edges {
		if pair.callee != *query.FunctionID {
			continue
		}
		name := "{entry}"
		if pair.caller >= 0 {
			name = functionName(capture.data.functions[pair.caller])
		}
		result.Callers = append(result.Callers, Caller{pair.caller, name, calls})
	}
	slices.SortFunc(result.Callers, func(a, b Caller) int {
		if a.Calls > b.Calls {
			return -1
		}
		if a.Calls < b.Calls {
			return 1
		}
		return a.ID - b.ID
	})
	result.Total = len(result.Callers)
	start := min(query.Offset, result.Total)
	result.Callers = result.Callers[start:min(start+query.Limit, result.Total)]
	result.Functions = append(result.Functions, functionSummary(capture, *query.FunctionID))
	result.Notes = append(result.Notes, "Caller counts are direct aggregated calls. Caller ID -1 denotes the request/command entry.")
	result.page(len(result.Callers))
}

func (result *QueryResult) graph(capture captureView, query Query) {
	root := -1
	if query.FunctionID != nil {
		root = *query.FunctionID
	}
	distances := map[int]int{root: 0}
	frontier := []int{root}
	adjacent := map[int][]int{}
	for pair := range capture.data.edges {
		adjacent[pair.caller] = append(adjacent[pair.caller], pair.callee)
	}
	for len(frontier) > 0 {
		current := frontier[0]
		frontier = frontier[1:]
		if distances[current] >= query.Depth {
			continue
		}
		for _, callee := range adjacent[current] {
			if _, seen := distances[callee]; !seen {
				distances[callee] = distances[current] + 1
				frontier = append(frontier, callee)
			}
		}
	}
	edges := []CallEdge{}
	for pair, calls := range capture.data.edges {
		if depth, ok := distances[pair.caller]; ok && depth < query.Depth {
			edges = append(edges, CallEdge{pair.caller, pair.callee, calls})
		}
	}
	slices.SortFunc(edges, func(a, b CallEdge) int {
		if a.Calls > b.Calls {
			return -1
		}
		if a.Calls < b.Calls {
			return 1
		}
		if a.CallerID != b.CallerID {
			return a.CallerID - b.CallerID
		}
		return a.CalleeID - b.CalleeID
	})
	result.Total = len(edges)
	start := min(query.Offset, len(edges))
	seen := map[int]bool{}
	budget := 64 << 10
	for _, edge := range edges[start:min(start+query.Limit, len(edges))] {
		rows := []FunctionSummary{}
		cost := 64
		for _, id := range []int{edge.CallerID, edge.CalleeID} {
			if id < 0 || seen[id] || edge.CallerID == edge.CalleeID && len(rows) > 0 {
				continue
			}
			row := functionSummary(capture, id)
			encoded, _ := json.Marshal(row)
			cost += len(encoded)
			rows = append(rows, row)
		}
		if cost > budget {
			break
		}
		budget -= cost
		for _, row := range rows {
			seen[row.ID] = true
			result.Functions = append(result.Functions, row)
		}
		result.Edges = append(result.Edges, edge)
	}
	if len(result.Edges) == 0 && root >= 0 {
		result.Functions = append(result.Functions, functionSummary(capture, root))
	}
	slices.SortFunc(result.Functions, func(a, b FunctionSummary) int { return a.ID - b.ID })
	result.Notes = append(result.Notes, "Graph edges aggregate direct calls across the capture; node timings are whole-function totals, not path timings. Cycles represent recursion. ID -1 denotes entry.")
	result.page(len(result.Edges))
}
