package psalmanalysis

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/rchehd/phpwarden/internal/runner"
)

type fixture struct {
	request Request
	process processFunc
	stages  []string
	private string
	mutate  func(string, *runner.Result)
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	base := t.TempDir()
	root, bundle := filepath.Join(base, "project"), filepath.Join(base, "bundle")
	write := func(name, text string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	packages := `{"packages":[{"name":"fixture/pkg","version":"1.0.0"}]}`
	write(filepath.Join(root, "composer.json"), `{}`)
	write(filepath.Join(root, "composer.lock"), packages)
	write(filepath.Join(root, "vendor/composer/installed.json"), packages)
	write(filepath.Join(root, "vendor/autoload.php"), "<?php\n")
	write(filepath.Join(root, "src.php"), "<?php\necho 'a';\n")
	write(filepath.Join(bundle, "autoload.php"), "<?php\n")
	write(filepath.Join(bundle, "bin/psalm"), "<?php\n")
	write(filepath.Join(bundle, "composer/installed.json"), fmt.Sprintf(`{"packages":[{"name":"vimeo/psalm","version":"6.20.3"},{"name":"psalm/plugin-symfony","version":"5.3.0"},{"name":"symfony/yaml","version":"v7.4.21"},{"name":"fixture/plugin","version":"dev-master","source":{"reference":%q}}]}`, "fixture-reference"))
	f := &fixture{}
	f.request = Request{Root: root, ComposerRoot: ".", Bundle: bundle, PHP: "php", Runtime: "8.3", Timeout: time.Minute, MemoryMB: 512, Pins: map[string]Pin{"fixture/plugin": {Reference: "fixture-reference"}}}
	f.request.Prepare = func(ctx context.Context, temporary string, process runner.Process) (Prepared, error) {
		f.private = temporary
		write(filepath.Join(temporary, "psalm.xml"), "<psalm/>\n")
		write(filepath.Join(temporary, "launcher.php"), "<?php\n")
		process.Run(ctx, runner.Command{Args: []string{"preparation"}})
		data, _ := os.ReadFile(filepath.Join(root, "src.php"))
		return Prepared{Config: filepath.Join(temporary, "psalm.xml"), Launcher: filepath.Join(temporary, "launcher.php"), Files: []string{"src.php"}, Inputs: map[string]string{"src.php": fmt.Sprintf("%x", sha256.Sum256(data))}}, nil
	}
	f.process = func(ctx context.Context, command runner.Command) runner.Result {
		stage := "readiness"
		result := runner.Result{Stdout: []byte(`[]`)}
		switch {
		case slices.Contains(command.Args, "-r"):
			stage = "runtime"
			result.Stdout = []byte(`{"php":"8.3.36","extensions":["dom","libxml","mbstring","SimpleXML","tokenizer","xml"]}`)
		case slices.Contains(command.Args, "preparation"):
			stage = "preparation"
		case strings.HasSuffix(command.Args[len(command.Args)-1], "syntax.php"):
			stage = "syntax"
			var files []string
			if json.Unmarshal(command.Input, &files) != nil {
				t.Fatal("missing syntax inventory")
			}
			result.Stdout = []byte(fmt.Sprintf(`{"version":1,"checked":%d,"errors":[]}`, len(files)))
		case slices.Contains(command.Args, "--taint-analysis"):
			stage = "taint"
			result.ExitCode = 2
			result.Stdout = []byte(fmt.Sprintf(`[{"type":"TaintedHtml","message":"Unsafe flow","file_path":%q,"line_from":2,"taint_trace":[{"label":"source","file_path":%q,"line_from":2}]}]`, filepath.Join(root, "src.php"), filepath.Join(root, "src.php")))
		}
		f.stages = append(f.stages, stage)
		if f.mutate != nil {
			f.mutate(stage, &result)
		}
		return result
	}
	return f
}

func (f *fixture) run(t *testing.T) Result {
	t.Helper()
	result := Run(context.Background(), f.request, f.process)
	if f.private != "" {
		if _, err := os.Stat(f.private); !os.IsNotExist(err) {
			t.Fatal("private workspace survived execution")
		}
	}
	return result
}

func TestCompletePipelinePreservesNativeFlow(t *testing.T) {
	f := newFixture(t)
	result := f.run(t)
	if !result.Complete || result.PHP != "8.3.36" || len(result.Findings) != 1 || len(result.Phases) != 5 || !slices.Equal(f.stages, []string{"runtime", "preparation", "syntax", "readiness", "taint"}) {
		t.Fatalf("unexpected pipeline: %+v", result)
	}
	if !strings.Contains(string(result.Findings[0]), "taint_trace") {
		t.Fatal("native trace lost")
	}
}

func TestReadinessAndSyntaxFailuresNeverRunTaint(t *testing.T) {
	for _, test := range []struct {
		stage, output string
		exit          int
	}{
		{"readiness", `[{"type":"ParseError"}]`, 2},
		{"readiness", `null`, 0},
		{"readiness", `[] trailing`, 0},
		{"readiness", `[]`, 1},
		{"syntax", `{"version":1,"checked":0,"errors":[]}`, 0},
		{"syntax", `{"version":1,"checked":3,"errors":[{}]}`, 3},
	} {
		t.Run(test.stage+test.output, func(t *testing.T) {
			f := newFixture(t)
			f.mutate = func(stage string, result *runner.Result) {
				if stage == test.stage {
					result.Stdout, result.ExitCode = []byte(test.output), test.exit
				}
			}
			result := f.run(t)
			if result.Complete || len(result.Findings) != 0 || len(result.Diagnostics) == 0 || slices.Contains(f.stages, "taint") {
				t.Fatalf("partial analysis accepted: %+v", result)
			}
		})
	}
}

func TestInvalidTaintReportsAndProcessFailures(t *testing.T) {
	for _, variant := range []string{"empty-exit2", "findings-exit0", "mixed", "missing-file", "bad-line", "bad-trace", "stderr", "crash", "process-error", "timeout"} {
		t.Run(variant, func(t *testing.T) {
			f := newFixture(t)
			f.mutate = func(stage string, result *runner.Result) {
				if stage != "taint" {
					return
				}
				switch variant {
				case "empty-exit2":
					result.Stdout = []byte(`[]`)
				case "findings-exit0":
					result.ExitCode = 0
				case "mixed":
					result.Stdout = bytesReplace(result.Stdout, "TaintedHtml", "ParseError")
				case "missing-file":
					result.Stdout = bytesReplace(result.Stdout, "src.php", "missing.php")
				case "bad-line":
					result.Stdout = bytesReplace(result.Stdout, `"line_from":2`, `"line_from":999`)
				case "bad-trace":
					result.Stdout = bytesReplace(result.Stdout, `"label":"source"`, `"label":""`)
				case "stderr":
					result.Stderr = []byte("warning")
				case "crash":
					result.ExitCode = 255
				case "process-error":
					result.Err = errors.New("process output failed")
				case "timeout":
					result.TimedOut = true
				}
			}
			result := f.run(t)
			if result.Complete || len(result.Findings) != 0 || len(result.Diagnostics) != 1 {
				t.Fatalf("invalid run accepted: %+v", result)
			}
		})
	}
}

func TestMissingConsumedInputHashesStopsBeforeSyntax(t *testing.T) {
	f := newFixture(t)
	prepare := f.request.Prepare
	f.request.Prepare = func(ctx context.Context, directory string, process runner.Process) (Prepared, error) {
		prepared, err := prepare(ctx, directory, process)
		prepared.Inputs = nil
		return prepared, err
	}
	result := f.run(t)
	if result.Complete || slices.Contains(f.stages, "syntax") || result.Diagnostics[0].Code != "invalid_report" {
		t.Fatalf("unfingerprinted inputs accepted: %+v", result)
	}
}

func bytesReplace(data []byte, before, after string) []byte {
	return []byte(strings.ReplaceAll(string(data), before, after))
}

func TestChangesInvalidateNativeFindings(t *testing.T) {
	for _, variant := range []string{"source", "source-same-stat", "new-file", "generated"} {
		t.Run(variant, func(t *testing.T) {
			f := newFixture(t)
			f.mutate = func(stage string, result *runner.Result) {
				if stage != "taint" {
					return
				}
				filename := filepath.Join(f.request.Root, "src.php")
				info, _ := os.Stat(filename)
				switch variant {
				case "source", "source-same-stat":
					os.WriteFile(filename, []byte("<?php\necho 'b';\n"), 0o600)
					if variant == "source-same-stat" {
						os.Chtimes(filename, info.ModTime(), info.ModTime())
					}
				case "new-file":
					os.WriteFile(filepath.Join(f.request.Root, "new.php"), []byte("<?php"), 0o600)
				case "generated":
					os.WriteFile(filepath.Join(f.private, "psalm.xml"), []byte("changed"), 0o600)
				}
			}
			result := f.run(t)
			if result.Complete || len(result.Findings) != 0 || result.Diagnostics[0].Code != "stale" {
				t.Fatalf("stale findings accepted: %+v", result)
			}
		})
	}
}

func TestMissingDependenciesAndRuntimeDoNotRunAnalysis(t *testing.T) {
	for _, variant := range []string{"tool-version", "missing-autoload", "installed-mismatch", "runtime", "extensions", "cancelled"} {
		t.Run(variant, func(t *testing.T) {
			f := newFixture(t)
			switch variant {
			case "tool-version":
				filename := filepath.Join(f.request.Bundle, "composer/installed.json")
				data, _ := os.ReadFile(filename)
				os.WriteFile(filename, bytesReplace(data, "6.20.3", "6.20.4"), 0o600)
			case "missing-autoload":
				os.Remove(filepath.Join(f.request.Root, "vendor/autoload.php"))
			case "installed-mismatch":
				os.WriteFile(filepath.Join(f.request.Root, "vendor/composer/installed.json"), []byte(`{"packages":[]}`), 0o600)
			case "runtime", "extensions":
				f.mutate = func(stage string, result *runner.Result) {
					if stage == "runtime" {
						if variant == "runtime" {
							result.Stdout = bytesReplace(result.Stdout, "8.3.36", "8.4.1")
						} else {
							result.Stdout = bytesReplace(result.Stdout, "mbstring", "missing")
						}
					}
				}
			case "cancelled":
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				result := Run(ctx, f.request, f.process)
				if result.Complete || len(f.stages) != 0 {
					t.Fatal("cancelled run executed")
				}
				return
			}
			result := f.run(t)
			if result.Complete || slices.Contains(f.stages, "preparation") || len(result.Diagnostics) == 0 {
				t.Fatalf("invalid prerequisites accepted: %+v", result)
			}
		})
	}
}
