package performance

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

func fixture(t testing.TB, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../../tools/spx/fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func put(t testing.TB, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func pair(t testing.TB) (string, Options) {
	t.Helper()
	dir := t.TempDir()
	for _, suffix := range []string{".json", ".txt.gz"} {
		put(t, dir, "cached"+suffix, fixture(t, "cached"+suffix))
	}
	return dir, Options{Directory: dir, Producer: Producer}
}

func raw(t testing.TB) []byte {
	t.Helper()
	reader, err := gzip.NewReader(bytes.NewReader(fixture(t, "cached.txt.gz")))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func zipped(t testing.TB, data []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func changedHeader(t testing.TB, changes map[string]any) []byte {
	t.Helper()
	var fields map[string]any
	if err := json.Unmarshal(fixture(t, "cached.json"), &fields); err != nil {
		t.Fatal(err)
	}
	for key, value := range changes {
		if value == nil {
			delete(fields, key)
		} else {
			fields[key] = value
		}
	}
	data, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func failed(t *testing.T, result Report, code string) {
	t.Helper()
	if result.Complete || result.ExitCode != 3 || len(result.Profiles) != 0 {
		t.Fatalf("unexpected success/partial summary: %+v", result)
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == code {
			return
		}
	}
	t.Fatalf("missing %s: %+v", code, result.Diagnostics)
}

func TestRealProfilesUnitsAndSourceMapping(t *testing.T) {
	result := Run(context.Background(), Options{Directory: "../../tools/spx/fixtures", Producer: Producer, Workspace: "../../tools/spx/fixtures", ContainerRoot: "/fixture"})
	if !result.Complete || result.ExitCode != 0 || len(result.Profiles) != 2 {
		t.Fatalf("%+v", result)
	}
	for i, expected := range []struct {
		calls int64
		wall  float64
		hot   int64
	}{{4, 15.949095, 1}, {50, 398.357845, 24}} {
		profile := result.Profiles[i]
		if profile.Calls != expected.calls || math.Abs(profile.WallMS-expected.wall) > 1e-8 || profile.CPUMS == nil {
			t.Fatalf("bad measurements: %+v", profile)
		}
		hot := profile.Functions[0]
		if hot.Name != "phpwarden_expensive_lookup" || hot.Calls != expected.hot || !hot.Mapped || hot.File != "scenario.php" || hot.Line != 2 || len(hot.Callers) != 1 || hot.Callers[0].Name != "phpwarden_render_item" || hot.Callers[0].Calls != expected.hot {
			t.Fatalf("bad hotspot: %+v", hot)
		}
		var self float64
		for _, fn := range profile.Functions {
			self += fn.SelfMS
		}
		if math.Abs(self-profile.WallMS) > 1e-8 {
			t.Fatal("self times do not partition the root duration")
		}
	}
	var source struct {
		SPX struct {
			Commit string `json:"commit"`
		} `json:"spx"`
	}
	data, err := os.ReadFile("../../tools/spx/sources.json")
	if err != nil || json.Unmarshal(data, &source) != nil || source.SPX.Commit != Producer {
		t.Fatal("producer pin drift")
	}
}

func TestZstandardAndCorruptCompression(t *testing.T) {
	dir, options := pair(t)
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		t.Fatal(err)
	}
	encoded := encoder.EncodeAll(raw(t), nil)
	encoder.Close()
	if err := os.Remove(filepath.Join(dir, "cached.txt.gz")); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "cached.txt.zst", encoded)
	result := Run(context.Background(), options)
	if !result.Complete || result.Profiles[0].Compression != "zst" || result.Profiles[0].WallMS != 15.949095 {
		t.Fatalf("%+v", result)
	}
	for _, suffix := range []string{"zst", "gz"} {
		t.Run(suffix, func(t *testing.T) {
			dir, options := pair(t)
			if suffix == "zst" {
				if err := os.Remove(filepath.Join(dir, "cached.txt.gz")); err != nil {
					t.Fatal(err)
				}
			}
			data := append([]byte(nil), encoded...)
			if suffix == "gz" {
				data = fixture(t, "cached.txt.gz")
			}
			for _, broken := range [][]byte{data[:len(data)-5], append(append([]byte(nil), data...), []byte("junk")...)} {
				put(t, dir, "cached.txt."+suffix, broken)
				failed(t, Run(context.Background(), options), "invalid_body")
			}
		})
	}
}

func TestMalformedEventsNeverYieldPartialFunctions(t *testing.T) {
	body := string(raw(t))
	cases := map[string]string{
		"unclosed":         strings.Replace(body, "-0|29411|29351|-344\n", "", 1),
		"wrong-end":        strings.Replace(body, "-3|", "-2|", 1),
		"backref":          strings.Replace(body, "|0|a0", "|r1|a0", 1),
		"metric":           strings.Replace(body, "|a0|a0|a0", "|secret|a0|a0", 1),
		"width":            strings.Replace(body, "|0|a0|a0|a0", "|0|a0|a0", 1),
		"unknown-function": strings.Replace(body, "|0|a0", "|99|a0", 1),
		"overflow":         strings.Replace(body, "|45717|40886|376", "|9223372036854775807|40886|376", 1),
		"callsite":         strings.Replace(body, "19|1|", "zz|1|", 1),
		"sections":         strings.Replace(body, "[functions]", "[events]", 1),
		"table":            strings.Replace(body, "/fixture/scenario.php:2:phpwarden_expensive_lookup", "legacy_function", 1),
		"missing-table":    strings.Split(body, "[functions]")[0],
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			dir, options := pair(t)
			put(t, dir, "cached.txt.gz", zipped(t, []byte(contents)))
			result := Run(context.Background(), options)
			if result.Complete || result.ExitCode != 3 || len(result.Profiles) != 0 {
				t.Fatalf("%+v", result)
			}
		})
	}
}

func TestMetadataValidation(t *testing.T) {
	cases := []map[string]any{
		{"key": "different"}, {"enabled_metrics": []string{"ct"}}, {"enabled_metrics": []string{"wt", "wt"}}, {"enabled_metrics": []string{"wt", "unknown"}},
		{"sampling_period_us": 1}, {"recorded_call_count": 3}, {"call_count": nil}, {"called_function_count": 0}, {"wall_time_ms": 15950}, {"cli": 2},
	}
	for i, changes := range cases {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			dir, options := pair(t)
			put(t, dir, "cached.json", changedHeader(t, changes))
			result := Run(context.Background(), options)
			if result.Complete || len(result.Profiles) != 0 {
				t.Fatalf("%+v", result)
			}
		})
	}
	for _, data := range [][]byte{[]byte(`{"key":"cached","key":"cached"}`), []byte(`{"key":"cached","KEY":"cached"}`), append(fixture(t, "cached.json"), []byte(`{}`)...), []byte(`null`)} {
		if _, err := decodeHeader(data, "cached", defaults()); err == nil {
			t.Fatal("accepted malformed metadata")
		}
	}
}

func TestNoSilentSiblingAndExplicitSelection(t *testing.T) {
	dir, options := pair(t)
	put(t, dir, "broken.json", []byte("{bad"))
	result := Run(context.Background(), options)
	if result.Complete || result.ExitCode != 3 || len(result.Profiles) != 1 || len(result.Diagnostics) != 1 {
		t.Fatalf("%+v", result)
	}
	options.Key = "cached"
	result = Run(context.Background(), options)
	if !result.Complete || result.Scope != "selected_report" || len(result.Profiles) != 1 {
		t.Fatalf("%+v", result)
	}
}

func TestMissingOrAmbiguousReportPairs(t *testing.T) {
	for _, name := range []string{"missing-body", "orphan-body", "ambiguous", "empty"} {
		t.Run(name, func(t *testing.T) {
			dir, options := pair(t)
			switch name {
			case "missing-body":
				if err := os.Remove(filepath.Join(dir, "cached.txt.gz")); err != nil {
					t.Fatal(err)
				}
			case "orphan-body":
				if err := os.Remove(filepath.Join(dir, "cached.json")); err != nil {
					t.Fatal(err)
				}
			case "ambiguous":
				put(t, dir, "cached.txt.zst", []byte("unused"))
			case "empty":
				options.Directory = t.TempDir()
			}
			result := Run(context.Background(), options)
			if result.Complete || result.ExitCode != 3 || len(result.Profiles) != 0 || len(result.Diagnostics) == 0 {
				t.Fatalf("%+v", result)
			}
		})
	}
}

func TestBoundedCallerSummaryHasOmissionCount(t *testing.T) {
	data := parsed{functions: []functionInfo{{name: "hot"}, {name: "a"}, {name: "b"}, {name: "c"}, {name: "d"}}, stats: []statistics{{calls: 20, self: 100, inclusive: 100}, {}, {}, {}, {}}, edges: map[edge]int64{{1, 0}: 8, {2, 0}: 6, {3, 0}: 4, {4, 0}: 2}, wall: 100}
	budget := 10000
	profile := summarize("key", "gz", header{CLI: 1, Recorded: 20, ct: -1}, data, nil, "", 1, &budget)
	fn := profile.Functions[0]
	if len(fn.Callers) != 3 || fn.OmittedCallers != 1 || fn.Callers[0].Calls != 8 || profile.OmittedFunctions != 4 {
		t.Fatalf("%+v", profile)
	}
	budget = 1
	profile = summarize("key", "gz", header{CLI: 1, ct: -1}, data, nil, "", 1, &budget)
	if len(profile.Functions) != 0 || profile.OmittedFunctions != 5 {
		t.Fatalf("%+v", profile)
	}
}

func TestConfinementAndChangedInput(t *testing.T) {
	for _, suffix := range []string{".json", ".txt.gz"} {
		t.Run(suffix, func(t *testing.T) {
			dir, options := pair(t)
			outside := t.TempDir()
			put(t, outside, "secret", []byte("private-token"))
			name := filepath.Join(dir, "cached"+suffix)
			if err := os.Remove(name); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(outside, "secret"), name); err != nil {
				t.Fatal(err)
			}
			result := Run(context.Background(), options)
			failed(t, result, "unsafe_input")
			encoded, _ := json.Marshal(result)
			if bytes.Contains(encoded, []byte("private-token")) {
				t.Fatal("leaked symlink contents")
			}
		})
	}
	dir, _ := pair(t)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	old, err := readFile(context.Background(), root, "cached.json", 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	put(t, dir, "cached.json", []byte("{}"))
	if unchanged(context.Background(), root, old, 64<<10) == nil {
		t.Fatal("accepted changed input")
	}
	workspace := t.TempDir()
	put(t, workspace, "safe.php", []byte("<?php"))
	outside := t.TempDir()
	put(t, outside, "secret.php", []byte("<?php"))
	if err := os.Symlink(outside, filepath.Join(workspace, "linked")); err != nil {
		t.Fatal(err)
	}
	source, err := os.OpenRoot(workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	for _, name := range []string{"/project/../secret.php", "/project/linked/secret.php", "/elsewhere/safe.php", "/project/missing.php"} {
		if _, _, mapped := sourceLocation(source, "/project", name, 1); mapped {
			t.Fatalf("mapped unsafe path %s", name)
		}
	}
	if file, line, mapped := sourceLocation(source, "/project", "/project/safe.php", 4); !mapped || file != "safe.php" || line != 4 {
		t.Fatal("valid source mapping failed")
	}
}

func TestRedactionAndTerminalSafety(t *testing.T) {
	dir, options := pair(t)
	changes := map[string]any{"cli": 0, "http_method": "GET", "http_request_uri": "https://private-user:private-password@example.invalid/public%20path?private-query#private-fragment", "cli_command_line": "private-cli", "host_name": "private-host", "custom_metadata_str": "private-custom", "process_pwd": "private-cwd"}
	put(t, dir, "cached.json", changedHeader(t, changes))
	result := Run(context.Background(), options)
	if !result.Complete || result.Profiles[0].Description != "GET /public%20path" {
		t.Fatalf("%+v", result)
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte("private-")) || bytes.Contains(encoded, []byte("/fixture")) {
		t.Fatalf("leaked metadata: %s", encoded)
	}
	if display("name\x1b[31m\u202eevil", 256) != "name?[31m?evil" {
		t.Fatal("terminal controls not removed")
	}
	changes["cli"] = 1
	put(t, dir, "cached.json", changedHeader(t, changes))
	result = Run(context.Background(), options)
	if result.Profiles[0].Description != "CLI execution (arguments redacted)" {
		t.Fatal("CLI arguments exposed")
	}
}

func TestResourceLimitsCancellationAndOutput(t *testing.T) {
	for _, name := range []string{"metadata", "compressed", "decoded", "batch", "events", "functions", "edges", "depth", "line", "reports"} {
		t.Run(name, func(t *testing.T) {
			_, options := pair(t)
			cap := defaults()
			switch name {
			case "metadata":
				cap.metadata = 4
			case "compressed":
				cap.compressed = 4
			case "decoded":
				cap.decoded = 40
			case "batch":
				cap.batch = 100
			case "events":
				cap.events = 2
			case "functions":
				cap.functions = 2
			case "edges":
				cap.edges = 1
			case "depth":
				cap.depth = 1
			case "line":
				cap.line = 16
			case "reports":
				cap.reports = 0
			}
			options.limits = &cap
			failed(t, Run(context.Background(), options), "resource_limit")
		})
	}
	_, options := pair(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := Run(ctx, options); result.ExitCode != 130 || result.Complete {
		t.Fatalf("%+v", result)
	}
	cap := defaults()
	cap.timeout = time.Nanosecond
	options.limits = &cap
	result := Run(context.Background(), options)
	if result.ExitCode != 3 || result.Complete {
		t.Fatalf("%+v", result)
	}
	options.limits = nil
	options.Top = 1
	result = Run(context.Background(), options)
	encoded, _ := json.Marshal(result)
	if !result.Complete || len(encoded) > 2<<20 || len(result.Profiles[0].Functions) != 1 || result.Profiles[0].OmittedFunctions != 3 {
		t.Fatalf("%+v", result)
	}
}

func TestRecursiveInclusiveAndBackReferences(t *testing.T) {
	meta := header{Functions: 2, Calls: 3, Recorded: 3, Metrics: []string{"wt", "ct"}, wt: 0, ct: 1, Wall: 10000}
	body := "[events]\n|0|a0|a0\n|1|1000000|500000\n|r1|1000000|500000\n-r1|2000000|1000000\n-r1|3000000|1500000\n-0|3000000|1500000\n[functions]\n/project/main.php:1:main\n/project/main.php:2:recursive\n"
	result, err := parseBody(context.Background(), zipped(t, []byte(body)), "gz", meta, defaults(), 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if result.stats[1].calls != 2 || result.stats[1].self != 6000000 || result.stats[1].inclusive != 6000000 || result.stats[0].self != 4000000 || result.edges[edge{1, 1}] != 1 {
		t.Fatalf("%+v", result)
	}
}

func TestDeclaredCalledFunctionsMustBeRecorded(t *testing.T) {
	meta, err := decodeHeader(fixture(t, "cached.json"), "cached", defaults())
	if err != nil {
		t.Fatal(err)
	}
	meta.Functions++
	_, err = parseBody(context.Background(), zipped(t, append(raw(t), []byte("/fixture/other.php:1:uncalled\n")...)), "gz", meta, defaults(), 1<<20)
	if err == nil || diagnosticFor("cached", err).Code != "inconsistent_metadata" {
		t.Fatalf("%v", err)
	}
}

func FuzzDecodeEvents(f *testing.F) {
	f.Add(string(raw(f)))
	f.Add("[events]\n|r20|a0\n[functions]\n")
	meta, err := decodeHeader(fixture(f, "cached.json"), "cached", defaults())
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 32<<10 {
			t.Skip()
		}
		cap := defaults()
		cap.events = 100
		cap.depth = 16
		cap.edges = 32
		cap.line = 4096
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_, _ = parseBody(ctx, zipped(t, []byte(body)), "gz", meta, cap, 64<<10)
	})
}
