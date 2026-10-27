package performance

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

type header struct {
	Key       string   `json:"key"`
	Timestamp int64    `json:"exec_ts"`
	CLI       int      `json:"cli"`
	Method    string   `json:"http_method"`
	URI       string   `json:"http_request_uri"`
	Wall      int64    `json:"wall_time_ms"`
	Memory    int64    `json:"peak_memory_usage"`
	Functions int      `json:"called_function_count"`
	Calls     int64    `json:"call_count"`
	Recorded  int64    `json:"recorded_call_count"`
	Sampling  int64    `json:"sampling_period_us"`
	Metrics   []string `json:"enabled_metrics"`
	wt, ct    int
}

func decodeHeader(data []byte, key string, cap limits) (header, error) {
	var result header
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return result, problem("invalid_metadata", "Report metadata must be one JSON object.")
	}
	fields := map[string]json.RawMessage{}
	names := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok {
			return result, problem("invalid_metadata", "Invalid report metadata keys.")
		}
		folded := strings.ToLower(name)
		if names[folded] {
			return result, problem("invalid_metadata", "Duplicate report metadata keys are rejected.")
		}
		names[folded] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil {
			return result, problem("invalid_metadata", "Malformed report metadata.")
		}
		fields[name] = value
	}
	if _, err := decoder.Token(); err != nil {
		return result, problem("invalid_metadata", "Malformed report metadata.")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return result, problem("invalid_metadata", "Trailing report metadata is rejected.")
	}
	for _, name := range []string{"key", "exec_ts", "cli", "wall_time_ms", "peak_memory_usage", "called_function_count", "call_count", "recorded_call_count", "sampling_period_us", "enabled_metrics"} {
		value, ok := fields[name]
		if !ok || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return result, problem("invalid_metadata", "Required report metadata is missing.")
		}
	}
	if json.Unmarshal(data, &result) != nil || result.Key != key || result.CLI < 0 || result.CLI > 1 || result.Timestamp < 0 || result.Timestamp > 253402300799 || result.Wall < 0 || result.Memory < 0 || result.Functions < 1 || result.Calls < 1 {
		return result, problem("invalid_metadata", "Invalid report identity, counts or measurements.")
	}
	if result.Functions > cap.functions || result.Calls > int64(cap.events/2) {
		return result, problem("resource_limit", "Report function or event count limit exceeded.")
	}
	if result.Sampling != 0 || result.Recorded != result.Calls {
		return result, problem("partial_recording", "Only complete unsampled reports are supported; filtered recordings cannot establish full coverage.")
	}
	result.wt, result.ct = -1, -1
	known := map[string]bool{}
	for _, metric := range []string{"wt", "ct", "it", "zm", "zmac", "zmab", "zmfc", "zmfb", "zgr", "zgb", "zgc", "zif", "zil", "zuc", "zuf", "zuo", "zo", "ze", "mor", "io", "ior", "iow"} {
		known[metric] = true
	}
	seen := map[string]bool{}
	if len(result.Metrics) == 0 || len(result.Metrics) > len(known) {
		return result, problem("invalid_metadata", "Invalid enabled metric list.")
	}
	for i, metric := range result.Metrics {
		if !known[metric] || seen[metric] {
			return result, problem("invalid_metadata", "Unknown or duplicate metrics are rejected.")
		}
		seen[metric] = true
		if metric == "wt" {
			result.wt = i
		}
		if metric == "ct" {
			result.ct = i
		}
	}
	if result.wt < 0 {
		return result, problem("unsupported_metrics", "The report must record wall time.")
	}
	return result, nil
}

func display(value string, maxBytes int) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return '?'
		}
		return r
	}, strings.ToValidUTF8(value, "?"))
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes-3]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value + "..."
}

func description(meta header) (string, string) {
	if meta.CLI == 1 {
		return "cli", "CLI execution (arguments redacted)"
	}
	method := meta.Method
	if method == "" || len(method) > 16 || strings.IndexFunc(method, func(r rune) bool { return r < 'A' || r > 'Z' }) >= 0 {
		method = "HTTP"
	}
	uri, err := url.ParseRequestURI(meta.URI)
	if err != nil || !strings.HasPrefix(uri.Path, "/") {
		return "http", method + " request (URI redacted)"
	}
	return "http", display(method+" "+uri.EscapedPath(), 256)
}
