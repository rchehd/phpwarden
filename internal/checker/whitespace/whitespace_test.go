package whitespace

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/scope"
)

func settings(t *testing.T) config.Whitespace {
	t.Helper()
	effective, err := config.Parse([]byte(`{"version":1,"profile":"drupal","php":{"runtime":"8.4","target":{"min":"8.4","max":"8.4"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	return effective.Config.Tools.Whitespace
}

func source(t *testing.T, root, filename, data string) {
	t.Helper()
	name := filepath.Join(root, filename)
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestTextRulesAndLineScope(t *testing.T) {
	root := t.TempDir()
	data := "old \nclean\r\n \t\r\n<<<<<<< HEAD\n|||||||| base\n=======\n>>>>>>>> branch\n" + strings.Repeat("x", 128*1024) + "\t\nlast \t"
	source(t, root, "demo.txt", data)
	selection := scope.Selection{Files: []string{"demo.txt"}, LineMap: map[string][]scope.Range{"demo.txt": {{Start: 3, End: 8}}}}
	result := Run(context.Background(), root, selection, settings(t))
	var positions []int
	for _, finding := range result.Findings {
		positions = append(positions, *finding.Line)
		if finding.Kind != "code" || *finding.Path != "demo.txt" || finding.Severity != "error" {
			t.Fatalf("incorrect finding: %+v", finding)
		}
	}
	if !reflect.DeepEqual(positions, []int{3, 4, 5, 6, 7, 8}) || *result.Findings[0].Column != 1 || *result.Findings[5].Column != 128*1024+1 {
		t.Fatalf("incorrect scoped positions: %+v", result)
	}
	selection.LineMap["demo.txt"] = nil
	result = Run(context.Background(), root, selection, settings(t))
	if len(result.Findings) != 8 || *result.Findings[7].Line != 9 || *result.Findings[7].Column != 5 {
		t.Fatalf("full/EOF findings lost: %+v", result)
	}
	selection.LineMap["demo.txt"] = []scope.Range{}
	if result := Run(context.Background(), root, selection, settings(t)); len(result.Findings) != 0 || result.Check.Status != "passed" {
		t.Fatalf("deletion-only diff must have no added-line findings: %+v", result)
	}
}

func TestRuleExclusionsAreIndependent(t *testing.T) {
	root := t.TempDir()
	selection := scope.Selection{Files: []string{"guide.md", "change.patch", "changes.diff", "src/demo.txt"}, LineMap: map[string][]scope.Range{}}
	for _, filename := range selection.Files {
		source(t, root, filename, "<<<<<<< HEAD \n")
		selection.LineMap[filename] = nil
	}
	cfg := settings(t)
	result := Run(context.Background(), root, selection, cfg)
	if len(result.Findings) != 5 {
		t.Fatalf("whitespace exceptions hid conflict markers: %+v", result)
	}
	cfg.ConflictMarkersExclude = []string{"**/*.md", "**/*.patch", "**/*.diff"}
	cfg.TrailingWhitespaceExclude = []string{}
	result = Run(context.Background(), root, selection, cfg)
	if len(result.Findings) != 5 || result.Findings[0].Rule != "whitespace.trailing" {
		t.Fatalf("marker exceptions hid whitespace: %+v", result)
	}
	cfg.TrailingWhitespace = false
	result = Run(context.Background(), root, selection, cfg)
	if len(result.Findings) != 1 || result.Findings[0].Rule != "conflict.marker" {
		t.Fatalf("independent toggle failed: %+v", result)
	}
	cfg.ConflictMarkers = false
	if result = Run(context.Background(), root, selection, cfg); result.Check.Status != "skipped" || *result.Check.Reason != "rules_disabled" {
		t.Fatalf("disabled rules did not skip: %+v", result)
	}
}

func TestBinaryAndMarkerLookalikes(t *testing.T) {
	root := t.TempDir()
	selection := scope.Selection{Files: []string{"binary.dat", "text.txt"}, LineMap: map[string][]scope.Range{"binary.dat": nil, "text.txt": nil}}
	source(t, root, "binary.dat", "<<<<<<< HEAD \n"+strings.Repeat("x", 128*1024)+"\x00")
	source(t, root, "text.txt", "<<<<<< short\n<<<<<<<identifier\n =======\n+<<<<<<< HEAD\ntext >>>>>>> branch\ntext\r\n")
	if result := Run(context.Background(), root, selection, settings(t)); len(result.Findings) != 0 || result.Check.Status != "passed" {
		t.Fatalf("binary or lookalike produced a finding: %+v", result)
	}
	source(t, root, "text.txt", "\xffvalue \n")
	if result := Run(context.Background(), root, selection, settings(t)); len(result.Findings) != 1 || result.Findings[0].Rule != "whitespace.trailing" {
		t.Fatalf("byte rules must also work on non-UTF-8 text: %+v", result)
	}
}

func TestFailurePoliciesKeepKnownFindings(t *testing.T) {
	root := t.TempDir()
	source(t, root, "first.txt", "bad \n")
	selection := scope.Selection{Files: []string{"first.txt", "missing.txt"}, LineMap: map[string][]scope.Range{"first.txt": nil, "missing.txt": nil}}
	for _, required := range []bool{true, false} {
		cfg := settings(t)
		cfg.Required = required
		outcome := Run(context.Background(), root, selection, cfg)
		result := report.New()
		result.Checks, result.Findings, result.Diagnostics = []report.Check{outcome.Check}, outcome.Findings, outcome.Diagnostics
		result.Finalize()
		want := 1
		if required {
			want = 3
		}
		if result.Complete || result.ExitCode != want || len(result.Findings) != 1 || outcome.Check.Status != "error" || *outcome.Check.Reason != "input_read" {
			t.Fatalf("read failure was misreported: %+v", result)
		}
	}
	selection.Files = []string{"first.txt"}
	cfg := settings(t)
	cfg.FailOn = "never"
	if result := Run(context.Background(), root, selection, cfg); result.Check.Status != "passed" || len(result.Findings) != 1 {
		t.Fatalf("nonblocking policy lost findings: %+v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := Run(ctx, root, selection, cfg); result.Check.Status != "error" || *result.Check.Reason != "checker_timeout" {
		t.Fatalf("cancellation passed: %+v", result)
	}
	cfg.Enabled = false
	if result := Run(ctx, root, selection, cfg); result.Check.Status != "skipped" || *result.Check.Reason != "disabled" {
		t.Fatalf("disabled checker ran: %+v", result)
	}
}
