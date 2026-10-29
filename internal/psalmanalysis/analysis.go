package psalmanalysis

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/phpconstraint"
	"github.com/rchehd/phpwarden/internal/runner"
)

//go:embed syntax.php
var syntax []byte

type Diagnostic struct {
	Stage   string `json:"stage"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Phase struct {
	Stage    string `json:"stage"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

type Result struct {
	Roots        Roots             `json:"-"`
	SourceFiles  []string          `json:"-"`
	ContextFiles []string          `json:"-"`
	Complete     bool              `json:"complete"`
	PHP          string            `json:"php_version,omitempty"`
	Findings     []json.RawMessage `json:"findings"`
	Readiness    []json.RawMessage `json:"readiness"`
	Diagnostics  []Diagnostic      `json:"diagnostics"`
	Phases       []Phase           `json:"phases"`
}

type Roots struct {
	Project, Bundle, Generated string
}

type Prepared struct {
	Config   string
	Launcher string
	Files    []string
	Inputs   map[string]string
}

type Request struct {
	Root         string
	ComposerRoot string
	Bundle       string
	PHP          string
	Runtime      string
	Pins         map[string]Pin
	Timeout      time.Duration
	MemoryMB     int
	Prepare      func(context.Context, string, runner.Process) (Prepared, error)
}

func decode(data []byte, target any) error {
	if len(data) > 32<<20 {
		return fmt.Errorf("native report is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing native report content")
	}
	return nil
}

func Environment(directory, bundle string) []string {
	environment := []string{"HOME=" + directory, "TMPDIR=" + directory, "LANG=C.UTF-8", "PHPWARDEN_PSALM_BUNDLE=" + bundle}
	if value, ok := os.LookupEnv("PATH"); ok {
		environment = append(environment, "PATH="+value)
	}
	return environment
}

func PHPArgs(memory int) []string {
	return []string{"-d", "auto_prepend_file=", "-d", "auto_append_file=", "-d", "display_errors=stderr", "-d", "log_errors=0", "-d", fmt.Sprintf("memory_limit=%dM", memory)}
}

func Run(ctx context.Context, request Request, process runner.Process) (result Result) {
	result.Findings, result.Readiness, result.Diagnostics, result.Phases = []json.RawMessage{}, []json.RawMessage{}, []Diagnostic{}, []Phase{}
	fail := func(stage, code, message string) Result {
		result.Complete, result.Findings = false, []json.RawMessage{}
		result.Diagnostics = append(result.Diagnostics, Diagnostic{stage, code, message})
		return result
	}
	if request.Timeout <= 0 || request.MemoryMB < 128 || request.Prepare == nil || request.PHP == "" || request.Runtime == "" {
		return fail("inputs", "invalid_request", "Psalm requires explicit runtime, limits and profile preparation")
	}
	ctx, cancel := context.WithTimeout(ctx, request.Timeout)
	defer cancel()
	if ctx.Err() != nil {
		return fail("inputs", "cancelled", ctx.Err().Error())
	}
	if err := Dependencies(request.Root, request.ComposerRoot, request.Bundle, request.Pins); err != nil {
		return fail("dependencies", "dependencies_missing", err.Error())
	}
	before, err := Snapshot(request.Root)
	if err != nil {
		return fail("inputs", "input_read", err.Error())
	}
	root, err := filepath.EvalSymlinks(request.Root)
	if err != nil {
		return fail("inputs", "input_read", err.Error())
	}
	bundleRoot, err := filepath.EvalSymlinks(request.Bundle)
	if err != nil {
		return fail("inputs", "input_read", err.Error())
	}
	bundleRoot, err = filepath.Abs(bundleRoot)
	if err != nil {
		return fail("inputs", "input_read", err.Error())
	}
	tempRoot, err := filepath.EvalSymlinks(os.TempDir())
	if err != nil || inside(root, tempRoot) {
		return fail("inputs", "workspace_invalid", "Private workspace must be outside the project")
	}
	temporary, err := os.MkdirTemp(tempRoot, "phpwarden-psalm-worker-")
	if err != nil {
		return fail("inputs", "workspace_invalid", err.Error())
	}
	defer func() {
		if err := os.RemoveAll(temporary); err != nil {
			result = fail("cleanup", "cleanup_failed", err.Error())
		}
	}()
	result.Roots = Roots{Project: root, Bundle: bundleRoot, Generated: temporary}
	fresh := func(inputs map[string]string) error {
		current, err := Snapshot(request.Root)
		if err != nil || current != before {
			return fmt.Errorf("project context changed during Psalm analysis")
		}
		if inputs != nil {
			return HashesUnchanged(request.Root, inputs)
		}
		return nil
	}
	run := func(stage string, args []string, input []byte) (runner.Result, bool) {
		execution := process.Run(ctx, runner.Command{Name: request.PHP, Args: append(PHPArgs(request.MemoryMB), args...), Input: input, Directory: temporary, Env: Environment(temporary, request.Bundle)})
		result.Phases = append(result.Phases, Phase{stage, execution.ExitCode, string(execution.Stdout), string(execution.Stderr)})
		if execution.TimedOut || ctx.Err() != nil {
			result = fail(stage, "cancelled", "Psalm execution did not complete within its context")
			return execution, false
		}
		unexpectedError := false
		if execution.Err != nil {
			var exited *exec.ExitError
			unexpectedError = !errors.As(execution.Err, &exited) || exited.ExitCode() != execution.ExitCode
		}
		if execution.Unavailable || execution.ExitCode < 0 || unexpectedError || len(execution.Stderr) > 0 {
			result = fail(stage, "process_failed", "Native Psalm phase failed; see phase output")
			return execution, false
		}
		return execution, true
	}
	identity, ok := run("runtime", []string{"-r", "echo json_encode(['php'=>PHP_VERSION,'extensions'=>get_loaded_extensions()]);"}, nil)
	if !ok {
		return result
	}
	var runtime struct {
		PHP        string   `json:"php"`
		Extensions []string `json:"extensions"`
	}
	constraint, _ := phpconstraint.Parse("~8.3.16")
	decodeErr := decode(identity.Stdout, &runtime)
	allowed, versionErr := constraint.Allows(runtime.PHP)
	if decodeErr != nil || identity.ExitCode != 0 || !allowed || versionErr != nil || !strings.HasPrefix(runtime.PHP, request.Runtime+".") {
		return fail("runtime", "runtime_unsupported", "The pinned integration currently requires PHP 8.3.16 or later in the 8.3 series")
	}
	for _, extension := range []string{"dom", "libxml", "mbstring", "SimpleXML", "tokenizer", "xml"} {
		if !slices.Contains(runtime.Extensions, extension) {
			return fail("runtime", "extension_missing", "Required PHP extension is unavailable: "+extension)
		}
	}
	result.PHP = runtime.PHP
	prepared, err := request.Prepare(ctx, temporary, processFunc(func(ctx context.Context, command runner.Command) runner.Result {
		execution := process.Run(ctx, command)
		result.Phases = append(result.Phases, Phase{"preparation", execution.ExitCode, string(execution.Stdout), string(execution.Stderr)})
		return execution
	}))
	if err != nil {
		if ctx.Err() != nil {
			return fail("preparation", "cancelled", ctx.Err().Error())
		}
		return fail("preparation", "preparation_incomplete", err.Error())
	}
	if err := fresh(prepared.Inputs); err != nil {
		return fail("preparation", "stale", err.Error())
	}
	if len(prepared.Inputs) == 0 || len(prepared.Files) == 0 || !inside(temporary, prepared.Config) || !inside(temporary, prepared.Launcher) {
		return fail("preparation", "invalid_report", "Profile returned invalid analysis inputs")
	}
	result.SourceFiles = slices.Clone(prepared.Files)
	for name := range prepared.Inputs {
		name = filepath.ToSlash(filepath.Clean(name))
		if !slices.Contains(prepared.Files, name) {
			result.ContextFiles = append(result.ContextFiles, name)
		}
	}
	slices.Sort(result.ContextFiles)
	result.ContextFiles = slices.Compact(result.ContextFiles)
	files := []string{}
	for _, name := range prepared.Files {
		if _, err := Read(root, name); err != nil {
			return fail("preparation", "invalid_report", err.Error())
		}
		files = append(files, filepath.Join(root, filepath.FromSlash(name)))
	}
	generated := map[string][]byte{}
	err = filepath.WalkDir(temporary, func(filename string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(filename)
		if err != nil || !entry.Type().IsRegular() {
			return fmt.Errorf("invalid prepared file")
		}
		generated[filename] = data
		if filepath.Ext(filename) == ".php" {
			files = append(files, filename)
		}
		return nil
	})
	if err != nil {
		return fail("preparation", "invalid_report", err.Error())
	}
	unchanged := func() error {
		if err := fresh(prepared.Inputs); err != nil {
			return err
		}
		for filename, expected := range generated {
			actual, err := os.ReadFile(filename)
			if err != nil || !bytes.Equal(actual, expected) {
				return fmt.Errorf("prepared analysis input changed")
			}
		}
		return nil
	}
	slices.Sort(files)
	files = slices.Compact(files)
	input, _ := json.Marshal(files)
	script := filepath.Join(temporary, "syntax.php")
	if err := os.WriteFile(script, syntax, 0o600); err != nil {
		return fail("syntax", "workspace_invalid", err.Error())
	}
	checked, ok := run("syntax", []string{script}, input)
	if !ok {
		return result
	}
	var syntaxReport struct {
		Version int               `json:"version"`
		Checked int               `json:"checked"`
		Errors  []json.RawMessage `json:"errors"`
	}
	if decode(checked.Stdout, &syntaxReport) != nil || syntaxReport.Version != 1 || syntaxReport.Checked != len(files) || syntaxReport.Errors == nil || checked.ExitCode != 0 || len(syntaxReport.Errors) != 0 {
		return fail("syntax", "syntax_incomplete", "PHP source or generated syntax could not be verified")
	}
	if err := unchanged(); err != nil {
		return fail("syntax", "stale", err.Error())
	}
	arguments := []string{prepared.Launcher, "--root=" + filepath.Join(root, filepath.FromSlash(request.ComposerRoot)), "--config=" + prepared.Config, "--no-cache", "--no-diff", "--no-progress", "--threads=1", "--scan-threads=1", fmt.Sprintf("--memory-limit=%dM", request.MemoryMB), "--output-format=json"}
	readiness, ok := run("readiness", arguments, nil)
	if !ok {
		return result
	}
	if decode(readiness.Stdout, &result.Readiness) != nil || result.Readiness == nil || readiness.ExitCode != 0 || len(result.Readiness) != 0 {
		return fail("readiness", "readiness_incomplete", "Ordinary Psalm diagnostics must be resolved before taint analysis")
	}
	if err := unchanged(); err != nil {
		return fail("readiness", "stale", err.Error())
	}
	taint, ok := run("taint", append(arguments, "--taint-analysis"), nil)
	if !ok {
		return result
	}
	if decode(taint.Stdout, &result.Findings) != nil || result.Findings == nil || (taint.ExitCode != 0 && taint.ExitCode != 2) || (taint.ExitCode == 0) != (len(result.Findings) == 0) {
		return fail("taint", "invalid_report", "Invalid native taint report or exit code")
	}
	for _, issue := range result.Findings {
		var row struct {
			Type    string            `json:"type"`
			Message string            `json:"message"`
			File    string            `json:"file_path"`
			Line    int               `json:"line_from"`
			Trace   []json.RawMessage `json:"taint_trace"`
		}
		if json.Unmarshal(issue, &row) != nil || !strings.HasPrefix(row.Type, "Tainted") || row.Message == "" || len(row.Trace) == 0 || (!inside(root, row.File) && !inside(temporary, row.File)) || !validLocation(row.File, row.Line, root, temporary) {
			return fail("taint", "invalid_report", "Incomplete taint finding or unsupported source location")
		}
		for _, node := range row.Trace {
			var location struct {
				Label string `json:"label"`
				File  string `json:"file_path"`
				Line  int    `json:"line_from"`
			}
			if json.Unmarshal(node, &location) != nil || location.Label == "" || location.File != "" && !validLocation(location.File, location.Line, root, temporary, request.Bundle) {
				return fail("taint", "invalid_report", "Invalid native taint trace")
			}
		}
	}
	if err := unchanged(); err != nil {
		return fail("taint", "stale", err.Error())
	}
	if ctx.Err() != nil {
		return fail("taint", "cancelled", ctx.Err().Error())
	}
	result.Complete = true
	return result
}

func validLocation(filename string, line int, roots ...string) bool {
	resolved, err := filepath.EvalSymlinks(filename)
	if err != nil || line < 1 || !slices.ContainsFunc(roots, func(root string) bool { return inside(root, filename) && inside(root, resolved) }) {
		return false
	}
	info, err := os.Stat(filename)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	data, err := os.ReadFile(filename)
	return err == nil && line <= bytes.Count(data, []byte{'\n'})+1
}

func inside(root, filename string) bool {
	if !filepath.IsAbs(filename) || filepath.Clean(filename) != filename {
		return false
	}
	relative, err := filepath.Rel(root, filename)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

type processFunc func(context.Context, runner.Command) runner.Result

func (function processFunc) Run(ctx context.Context, command runner.Command) runner.Result {
	return function(ctx, command)
}
