package performance

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
)

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,191}$`)

type failure struct{ code, message string }

func (f failure) Error() string          { return f.message }
func problem(code, message string) error { return failure{code, message} }

type snapshot struct {
	name string
	data []byte
	info os.FileInfo
	hash [32]byte
}

type checkedReader struct {
	ctx   context.Context
	input io.Reader
	left  *int64
}

func (r checkedReader) Read(data []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if *r.left <= 0 {
		return 0, problem("resource_limit", "Decoded report byte limit exceeded.")
	}
	if int64(len(data)) > *r.left {
		data = data[:*r.left]
	}
	n, err := r.input.Read(data)
	*r.left -= int64(n)
	return n, err
}

func readFile(ctx context.Context, root *os.Root, name string, maxBytes int64) (snapshot, error) {
	var result snapshot
	if err := ctx.Err(); err != nil {
		return result, err
	}
	before, err := root.Lstat(name)
	if err != nil {
		return result, problem("input_unavailable", "A report input is missing or unreadable.")
	}
	if !before.Mode().IsRegular() {
		return result, problem("unsafe_input", "Report inputs must be regular files; symlinks are rejected.")
	}
	if before.Size() > maxBytes {
		return result, problem("resource_limit", "Report input exceeds its byte limit.")
	}
	file, err := openInput(root, name)
	if err != nil {
		return result, problem("input_unavailable", "A report input could not be opened safely.")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return result, problem("input_changed", "Report input changed while opening it.")
	}
	left := maxBytes + 1
	data, err := io.ReadAll(checkedReader{ctx, io.LimitReader(file, maxBytes+1), &left})
	result.data = data
	if err != nil {
		return result, err
	}
	if int64(len(data)) > maxBytes {
		return result, problem("resource_limit", "Report input exceeds its byte limit.")
	}
	after, err := root.Lstat(name)
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
		return result, problem("input_changed", "Report input changed while reading it.")
	}
	return snapshot{name, data, after, sha256.Sum256(data)}, nil
}

func unchanged(ctx context.Context, root *os.Root, old snapshot, maxBytes int64) error {
	current, err := readFile(ctx, root, old.name, maxBytes)
	if err != nil {
		return err
	}
	if current.hash != old.hash || !os.SameFile(current.info, old.info) {
		return problem("input_changed", "Report input changed during analysis; rerun it.")
	}
	return nil
}

func reportKeys(root *os.Root, selected string, maxReports int) ([]string, error) {
	if selected != "" {
		if !keyPattern.MatchString(selected) {
			return nil, problem("arguments", "Invalid report key.")
		}
		return []string{selected}, nil
	}
	file, err := root.Open(".")
	if err != nil {
		return nil, problem("input_unavailable", "Cannot read the reports directory.")
	}
	defer file.Close()
	entries, err := file.ReadDir(maxReports*3 + 1)
	if err != nil && err != io.EOF {
		return nil, problem("input_unavailable", "Cannot list the reports directory.")
	}
	if len(entries) > maxReports*3 {
		return nil, problem("resource_limit", "Directory entry limit exceeded; select one report with --report.")
	}
	keys := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		for _, suffix := range []string{".json", ".txt.gz", ".txt.zst"} {
			if strings.HasSuffix(name, suffix) {
				key := strings.TrimSuffix(name, suffix)
				if !keyPattern.MatchString(key) {
					return nil, problem("unsafe_input", "An input has an invalid report key.")
				}
				keys[key] = true
				break
			}
		}
	}
	if len(keys) > maxReports {
		return nil, problem("resource_limit", "Report count limit exceeded; select one report with --report.")
	}
	result := make([]string, 0, len(keys))
	for key := range keys {
		result = append(result, key)
	}
	slices.Sort(result)
	if len(result) == 0 {
		return nil, problem("input_unavailable", "No SPX report inputs were found.")
	}
	return result, nil
}

func bodyName(root *os.Root, key string) (string, string, error) {
	name, compression := "", ""
	for _, suffix := range []string{"gz", "zst"} {
		candidate := key + ".txt." + suffix
		_, err := root.Lstat(candidate)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return "", "", problem("input_unavailable", "Cannot inspect the report body.")
		}
		if name != "" {
			return "", "", problem("ambiguous_input", "Both gzip and Zstandard bodies exist for the same report.")
		}
		name, compression = candidate, suffix
	}
	if name == "" {
		return "", "", problem("input_unavailable", "Report body is missing.")
	}
	return name, compression, nil
}

func diagnosticFor(key string, err error) Diagnostic {
	var fault failure
	if errors.As(err, &fault) {
		return Diagnostic{fault.code, key, fault.message}
	}
	if errors.Is(err, context.Canceled) {
		return Diagnostic{"cancelled", key, "Performance analysis was cancelled."}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return Diagnostic{"timeout", key, "Performance analysis exceeded its time limit."}
	}
	return Diagnostic{"invalid_report", key, "Report decoding or validation failed."}
}

func sourceLocation(root *os.Root, container, filename string, line int) (string, int, bool) {
	if root == nil || filename == "" || line <= 0 {
		return "", 0, false
	}
	filename = path.Clean(filename)
	var relative string
	if container == "/" {
		relative = strings.TrimPrefix(filename, "/")
	} else {
		var found bool
		relative, found = strings.CutPrefix(filename, container+"/")
		if !found {
			return "", 0, false
		}
	}
	if relative == "" || len(relative) > 512 || !path.IsAbs(filename) || display(relative, 512) != relative {
		return "", 0, false
	}
	parts := strings.Split(relative, "/")
	for i := range parts {
		info, err := root.Lstat(strings.Join(parts[:i+1], "/"))
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return "", 0, false
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return "", 0, false
		}
	}
	return relative, line, true
}
