package performance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"slices"
	"strings"
	"time"
)

func InputError(message string) Report {
	result := initial()
	result.Complete, result.ExitCode = false, 2
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "arguments", Message: message})
	return result
}

func initial() Report {
	return Report{Version: 1, Command: "performance", ProducerOrigin: "explicit operator assertion", Scope: "directory", Complete: true, Profiles: []Profile{}, Diagnostics: []Diagnostic{}, Notes: []string{
		"Only the verified producer commit and complete unsampled SPX v0.5 event format are supported. Producer identity is explicitly supplied, not authenticated by the report.",
		"Times are milliseconds. Memory is the capture's overall peak, not per-function allocations or proof of a leak.",
		"CLI arguments, host/user metadata, custom metadata and HTTP query/fragment/userinfo are omitted. Unmapped absolute source paths are omitted.",
		"Source locations refer to function definitions. Path mapping does not establish that the current code matches the captured code. Measurements describe this capture only.",
	}}
}

func Run(ctx context.Context, options Options) Report {
	result := initial()
	if options.Directory == "" {
		return InputError("--reports requires a reports directory.")
	}
	if options.Producer != Producer {
		return InputError("Supply the verified producer commit with --producer; unknown producers are not inferred from the extension version.")
	}
	if options.Top == 0 {
		options.Top = 10
	}
	if options.Top < 1 || options.Top > 20 {
		return InputError("--limit must be between 1 and 20.")
	}
	if options.Key != "" && !keyPattern.MatchString(options.Key) {
		return InputError("Invalid report key.")
	}
	if (options.Workspace == "") != (options.ContainerRoot == "") {
		return InputError("Source mapping requires both --project and --container-root.")
	}
	if options.ContainerRoot != "" && (!path.IsAbs(options.ContainerRoot) || strings.Contains(options.ContainerRoot, "\\")) {
		return InputError("--container-root must be an absolute POSIX path.")
	}
	cap := defaults()
	if options.limits != nil {
		cap = *options.limits
	}
	ctx, cancel := context.WithTimeout(ctx, cap.timeout)
	defer cancel()
	result.Producer = Producer
	if options.Key != "" {
		result.Scope = "selected_report"
	}
	record := func(key string, err error) {
		result.Complete = false
		result.Diagnostics = append(result.Diagnostics, diagnosticFor(key, err))
	}
	if ctx.Err() != nil {
		record("", ctx.Err())
		finish(&result, ctx)
		return result
	}
	root, err := os.OpenRoot(options.Directory)
	if err != nil {
		record("", problem("input_unavailable", "Cannot open the reports directory."))
		finish(&result, ctx)
		return result
	}
	defer root.Close()
	rootInfo, err := root.Stat(".")
	if err != nil {
		record("", problem("input_unavailable", "Cannot inspect the reports directory."))
		finish(&result, ctx)
		return result
	}
	var workspace *os.Root
	if options.Workspace != "" {
		workspace, err = os.OpenRoot(options.Workspace)
		if err != nil {
			return InputError("Cannot open the source workspace.")
		}
		defer workspace.Close()
		options.ContainerRoot = path.Clean(options.ContainerRoot)
	}
	keys, err := reportKeys(root, options.Key, cap.reports)
	if err != nil {
		record("", err)
		finish(&result, ctx)
		return result
	}
	type captured struct {
		key            string
		metadata, body snapshot
		compression    string
	}
	captures := []captured{}
	inputLeft, decodedLeft := cap.batch, cap.batch
	outputLeft := 1 << 20
	for _, key := range keys {
		if ctx.Err() != nil {
			record(key, ctx.Err())
			break
		}
		if inputLeft < 1 || decodedLeft < 1 {
			record(key, problem("resource_limit", "Analysis byte budget exhausted; select one report."))
			break
		}
		metaFile, err := readFile(ctx, root, key+".json", min(cap.metadata, inputLeft))
		inputLeft -= int64(len(metaFile.data))
		if err != nil {
			record(key, err)
			continue
		}
		meta, err := decodeHeader(metaFile.data, key, cap)
		if err != nil {
			record(key, err)
			continue
		}
		name, compression, err := bodyName(root, key)
		if err != nil {
			record(key, err)
			continue
		}
		body, err := readFile(ctx, root, name, min(cap.compressed, inputLeft))
		inputLeft -= int64(len(body.data))
		if err != nil {
			record(key, err)
			continue
		}
		parsed, err := parseBody(ctx, body.data, compression, meta, cap, min(cap.decoded, decodedLeft))
		decodedLeft -= parsed.decoded
		if err != nil {
			record(key, err)
			continue
		}
		result.Profiles = append(result.Profiles, summarize(key, compression, meta, parsed, workspace, options.ContainerRoot, options.Top, &outputLeft))
		metaFile.data, body.data = nil, nil
		captures = append(captures, captured{key, metaFile, body, compression})
	}
	invalid := map[string]bool{}
	for _, capture := range captures {
		for _, input := range []snapshot{capture.metadata, capture.body} {
			if inputLeft < input.info.Size() {
				record(capture.key, problem("resource_limit", "Input verification byte budget exhausted; select one report."))
				invalid[capture.key] = true
				break
			}
			inputLeft -= input.info.Size()
			if err := unchanged(ctx, root, input, input.info.Size()); err != nil {
				record(capture.key, err)
				invalid[capture.key] = true
				break
			}
		}
		_, compression, err := bodyName(root, capture.key)
		if err != nil || compression != capture.compression {
			record(capture.key, problem("input_changed", "Report body selection changed during analysis."))
			invalid[capture.key] = true
		}
	}
	result.Profiles = slices.DeleteFunc(result.Profiles, func(profile Profile) bool { return invalid[profile.Key] })
	current, err := reportKeys(root, options.Key, cap.reports)
	if err != nil || !slices.Equal(current, keys) {
		record("", problem("input_changed", "Reports directory changed during analysis; rerun it."))
	}
	currentRoot, err := os.Stat(options.Directory)
	if err != nil || !os.SameFile(rootInfo, currentRoot) {
		record("", problem("input_changed", "Reports directory identity changed during analysis."))
		result.Profiles = []Profile{}
	}
	if ctx.Err() != nil {
		record("", ctx.Err())
	}
	finish(&result, ctx)
	return result
}

func finish(result *Report, ctx context.Context) {
	if !result.Complete {
		result.ExitCode = 3
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		result.Complete = false
		result.ExitCode = 130
	}
}

func functionName(function functionInfo) string {
	if strings.Contains(function.name, "/") || function.name == function.file {
		return "{main}"
	}
	return display(function.name, 256)
}

func summarize(key, compression string, meta header, data parsed, workspace *os.Root, container string, top int, outputLeft *int) Profile {
	kind, description := description(meta)
	profile := Profile{Key: key, RecordedAt: time.Unix(meta.Timestamp, 0).UTC().Format(time.RFC3339), Kind: kind, Description: description,
		Compression: compression, WallMS: float64(data.wall) / 1e6, PeakMemory: meta.Memory, Calls: meta.Recorded,
		TotalFunctions: len(data.functions), Functions: []Function{}}
	if meta.ct >= 0 {
		value := float64(data.cpu) / 1e6
		profile.CPUMS = &value
	}
	ids := make([]int, len(data.functions))
	for i := range ids {
		ids[i] = i
	}
	slices.SortFunc(ids, func(a, b int) int {
		if data.stats[a].self != data.stats[b].self {
			if data.stats[a].self > data.stats[b].self {
				return -1
			}
			return 1
		}
		return a - b
	})
	for _, id := range ids[:min(top, len(ids))] {
		function, stats := data.functions[id], data.stats[id]
		file, line, mapped := sourceLocation(workspace, container, function.file, function.line)
		row := Function{ID: id, Name: functionName(function), Calls: stats.calls, SelfMS: float64(stats.self) / 1e6,
			InclusiveMS: float64(stats.inclusive) / 1e6, File: file, Line: line, Mapped: mapped, Callers: []Caller{}}
		if meta.ct >= 0 {
			value := float64(stats.cpuSelf) / 1e6
			row.CPUSelfMS = &value
		}
		if data.wall > 0 {
			row.Share = float64(stats.self) / float64(data.wall) * 100
		}
		for pair, calls := range data.edges {
			if pair.callee != id {
				continue
			}
			name := "{entry}"
			if pair.caller >= 0 {
				name = functionName(data.functions[pair.caller])
			}
			row.Callers = append(row.Callers, Caller{pair.caller, name, calls})
		}
		slices.SortFunc(row.Callers, func(a, b Caller) int {
			if a.Calls != b.Calls {
				if a.Calls > b.Calls {
					return -1
				}
				return 1
			}
			return a.ID - b.ID
		})
		row.OmittedCallers = max(0, len(row.Callers)-3)
		row.Callers = row.Callers[:min(3, len(row.Callers))]
		encoded, _ := json.Marshal(row)
		if len(encoded) > *outputLeft {
			break
		}
		*outputLeft -= len(encoded)
		profile.Functions = append(profile.Functions, row)
	}
	profile.OmittedFunctions = profile.TotalFunctions - len(profile.Functions)
	return profile
}
