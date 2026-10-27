package performance

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/klauspost/compress/zstd"
)

type functionInfo struct {
	name, file string
	line       int
}
type statistics struct{ calls, self, inclusive, cpuSelf int64 }
type edge struct{ caller, callee int }
type frame struct {
	fn                                   int
	wall, cpu, childrenWall, childrenCPU int64
	outer                                bool
}
type event struct {
	fn     int
	values [22]int64
}
type parsed struct {
	functions []functionInfo
	stats     []statistics
	edges     map[edge]int64
	wall, cpu int64
	decoded   int64
}

func add(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, problem("invalid_body", "Numeric overflow in report events.")
	}
	return a + b, nil
}

func functionLine(line string) (functionInfo, error) {
	parts := strings.Split(line, ":")
	for i, part := range parts {
		if part == "" {
			continue
		}
		pure := true
		for _, c := range part {
			if c < '0' || c > '9' {
				pure = false
				break
			}
		}
		if !pure {
			continue
		}
		position, err := strconv.ParseUint(part, 10, 31)
		if err != nil || i == len(parts)-1 {
			break
		}
		file, name := strings.Join(parts[:i], ":"), strings.Join(parts[i+1:], ":")
		if name == "" || len(file) > 4096 || len(name) > 2048 {
			break
		}
		return functionInfo{name, file, int(position)}, nil
	}
	return functionInfo{}, problem("unsupported_format", "Function rows must contain the verified file:line:name format.")
}

func decodeEvent(line string, metrics int, history *[20]event, position, count int) (event, bool, error) {
	var next event
	parts := strings.Split(line, "|")
	start := !strings.HasPrefix(line, "-")
	offset := 0
	var identity string
	if start {
		if len(parts) != metrics+2 {
			return next, false, problem("invalid_body", "Invalid START event width.")
		}
		if parts[0] != "" {
			if _, err := strconv.ParseUint(parts[0], 16, 32); err != nil {
				return next, false, problem("invalid_body", "Invalid call-site position.")
			}
		}
		identity = parts[1]
		offset = 2
	} else {
		if len(parts) != metrics+1 {
			return next, false, problem("invalid_body", "Invalid END event width.")
		}
		identity = strings.TrimPrefix(parts[0], "-")
		offset = 1
	}
	if strings.HasPrefix(identity, "r") {
		n, err := strconv.Atoi(strings.TrimPrefix(identity, "r"))
		if err != nil || n < 1 || n > count {
			return next, false, problem("invalid_body", "Invalid event back-reference.")
		}
		next.fn = history[(position-n+20)%20].fn
	} else {
		n, err := strconv.ParseUint(identity, 10, 31)
		if err != nil {
			return next, false, problem("invalid_body", "Invalid function identifier.")
		}
		next.fn = int(n)
	}
	for i, text := range parts[offset:] {
		absolute := strings.HasPrefix(text, "a")
		if absolute {
			text = strings.TrimPrefix(text, "a")
		}
		if text == "" && !absolute {
			text = "0"
		}
		value, err := strconv.ParseInt(text, 10, 64)
		if err != nil {
			return next, false, problem("invalid_body", "Invalid event metric.")
		}
		if !absolute && count > 0 {
			value, err = add(value, history[(position-1+20)%20].values[i])
			if err != nil {
				return next, false, err
			}
		}
		next.values[i] = value
	}
	return next, start, nil
}

func parseBody(ctx context.Context, data []byte, compression string, meta header, cap limits, maxBytes int64) (result parsed, parseErr error) {
	var reader io.Reader
	var closeReader func()
	if compression == "gz" {
		decoded, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return result, problem("invalid_body", "Invalid gzip report body.")
		}
		reader = decoded
		closeReader = func() { _ = decoded.Close() }
	} else {
		decoded, err := zstd.NewReader(bytes.NewReader(data), zstd.WithDecoderConcurrency(1), zstd.WithDecoderLowmem(true), zstd.WithDecoderMaxMemory(16<<20), zstd.WithDecoderMaxWindow(8<<20), zstd.WithDecodeBuffersBelow(0))
		if err != nil {
			return result, problem("invalid_body", "Invalid Zstandard report body.")
		}
		reader = decoded
		closeReader = decoded.Close
	}
	defer closeReader()
	left := maxBytes + 1
	defer func() { result.decoded = maxBytes + 1 - left }()
	scanner := bufio.NewScanner(checkedReader{ctx, reader, &left})
	scanner.Buffer(make([]byte, min(4096, cap.line)), cap.line+1)
	result.stats = make([]statistics, meta.Functions)
	result.edges = map[edge]int64{}
	stack := make([]frame, 0, 32)
	active := make([]int, meta.Functions)
	var history [20]event
	position, count, events, section := 0, 0, 0, 0
	var calls int64
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if len(line) > cap.line {
			return result, problem("resource_limit", "Report line length limit exceeded.")
		}
		if line == "" {
			continue
		}
		if line == "[events]" {
			if section != 0 {
				return result, problem("invalid_body", "Duplicate or out-of-order report sections.")
			}
			section = 1
			continue
		}
		if line == "[functions]" {
			if section != 1 || len(stack) != 0 || calls == 0 {
				return result, problem("incomplete_body", "Event stream is incomplete or unbalanced.")
			}
			section = 2
			continue
		}
		if section == 2 {
			if len(result.functions) >= meta.Functions {
				return result, problem("invalid_body", "Function table exceeds declared counts.")
			}
			function, err := functionLine(line)
			if err != nil {
				return result, err
			}
			result.functions = append(result.functions, function)
			continue
		}
		if section != 1 {
			return result, problem("unsupported_format", "Unknown report section or format.")
		}
		events++
		if events > cap.events {
			return result, problem("resource_limit", "Report event count limit exceeded.")
		}
		next, start, err := decodeEvent(line, len(meta.Metrics), &history, position, count)
		if err != nil {
			return result, err
		}
		if next.fn >= meta.Functions {
			return result, problem("invalid_body", "Event refers to an unknown function.")
		}
		wall, cpu := next.values[meta.wt], int64(0)
		if meta.ct >= 0 {
			cpu = next.values[meta.ct]
		}
		if wall < result.wall || cpu < result.cpu {
			return result, problem("invalid_body", "Time counters must be monotonic.")
		}
		result.wall, result.cpu = wall, cpu
		history[position] = next
		position = (position + 1) % 20
		count = min(count+1, 20)
		if start {
			if len(stack) >= cap.depth {
				return result, problem("resource_limit", "Report stack depth limit exceeded.")
			}
			caller := -1
			if len(stack) > 0 {
				caller = stack[len(stack)-1].fn
			}
			identity := edge{caller, next.fn}
			if _, ok := result.edges[identity]; !ok && len(result.edges) >= cap.edges {
				return result, problem("resource_limit", "Report edge count limit exceeded.")
			}
			result.edges[identity]++
			result.stats[next.fn].calls++
			stack = append(stack, frame{fn: next.fn, wall: wall, cpu: cpu, outer: active[next.fn] == 0})
			active[next.fn]++
			calls++
		} else {
			if len(stack) == 0 || stack[len(stack)-1].fn != next.fn {
				return result, problem("incomplete_body", "END events must match the complete call stack.")
			}
			ended := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			active[next.fn]--
			duration, cpuDuration := wall-ended.wall, cpu-ended.cpu
			if duration < ended.childrenWall || cpuDuration < ended.childrenCPU {
				return result, problem("invalid_body", "Nested event timings are inconsistent.")
			}
			stats := &result.stats[next.fn]
			stats.self, err = add(stats.self, duration-ended.childrenWall)
			if err != nil {
				return result, err
			}
			stats.cpuSelf, err = add(stats.cpuSelf, cpuDuration-ended.childrenCPU)
			if err != nil {
				return result, err
			}
			if ended.outer {
				stats.inclusive, err = add(stats.inclusive, duration)
				if err != nil {
					return result, err
				}
			}
			if len(stack) > 0 {
				parent := &stack[len(stack)-1]
				parent.childrenWall, err = add(parent.childrenWall, duration)
				if err != nil {
					return result, err
				}
				parent.childrenCPU, err = add(parent.childrenCPU, cpuDuration)
				if err != nil {
					return result, err
				}
			}
		}
	}
	result.decoded = maxBytes + 1 - left
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if result.decoded > maxBytes || left == 0 {
		return result, problem("resource_limit", "Decoded report byte limit exceeded.")
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			return result, problem("resource_limit", "Report line length limit exceeded.")
		}
		return result, problem("invalid_body", "Report decoding failed.")
	}
	if section != 2 || len(stack) != 0 || len(result.functions) != meta.Functions || calls != meta.Recorded || events != int(calls*2) {
		return result, problem("incomplete_body", "Report sections, counts or call stack are incomplete.")
	}
	if result.wall/1000 != meta.Wall {
		return result, problem("inconsistent_metadata", "Report duration does not match the verified producer metadata units.")
	}
	for _, stats := range result.stats {
		if stats.calls == 0 {
			return result, problem("inconsistent_metadata", "Called-function counts include an unrecorded function.")
		}
	}
	return result, nil
}
