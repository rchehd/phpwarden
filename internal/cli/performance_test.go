package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/performance"
)

func TestPerformanceCLIReadsWithoutProjectConfigOrExternalTools(t *testing.T) {
	fixture, err := filepath.Abs("../../tools/spx/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	for _, format := range []string{"text", "json"} {
		t.Run(format, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := []string{"performance", "--reports", fixture, "--producer", performance.Producer, "--project", fixture, "--container-root", "/fixture", "--format", format, "--limit=1"}
			if code := Run(args, t.TempDir(), &stdout, &stderr); code != 0 || stderr.Len() != 0 {
				t.Fatalf("%d %s %s", code, &stdout, &stderr)
			}
			if format == "json" {
				var result performance.Report
				if json.Unmarshal(stdout.Bytes(), &result) != nil || !result.Complete || len(result.Profiles) != 2 || len(result.Profiles[0].Functions) != 1 {
					t.Fatalf("%s", &stdout)
				}
			} else if !strings.Contains(stdout.String(), "15.825") || !strings.Contains(stdout.String(), "scenario.php:2 (definition)") || !strings.Contains(stdout.String(), "from phpwarden_render_item × 24") {
				t.Fatalf("%s", &stdout)
			}
		})
	}
}

func TestPerformanceArgumentsAndHelp(t *testing.T) {
	fixture, err := filepath.Abs("../../tools/spx/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	base := []string{"performance", "--reports", fixture, "--producer", performance.Producer}
	for _, extra := range [][]string{{"--limit=0"}, {"--limit=-1"}, {"--limit=21"}, {"--limit=invalid"}, {"--project=missing"}, {"--container-root=/fixture"}, {"--report=../cached"}, {"--report=bad\nkey"}, {"--format=compact"}, {"--commits=2"}, {"--reports=elsewhere"}, {"--limit"}} {
		var stdout, stderr bytes.Buffer
		args := append(append([]string{}, base...), extra...)
		args = append(args, "--format=json")
		code := Run(args, t.TempDir(), &stdout, &stderr)
		var result performance.Report
		if code != 2 || json.Unmarshal(stdout.Bytes(), &result) != nil || result.Complete || result.ExitCode != 2 || stderr.Len() != 0 {
			t.Fatalf("%v: %d %s %s", extra, code, &stdout, &stderr)
		}
	}
	for _, args := range [][]string{{"performance", "--help"}, {"--help"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, t.TempDir(), &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "performance") {
			t.Fatalf("%d %s %s", code, &stdout, &stderr)
		}
	}
	for _, producer := range []string{"", "0.4.22", strings.Repeat("a", 40)} {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"performance", "--reports", fixture, "--producer=" + producer, "--format=json"}, t.TempDir(), &stdout, &stderr)
		if code != 2 {
			t.Fatalf("accepted producer %s: %d", producer, code)
		}
	}
}

func TestPerformanceBrokenReportIsIncompleteAndRelativeInput(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte(`{"secret":"never-echo-this"`), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Run([]string{"performance", "--reports", ".", "--producer", performance.Producer, "--format=json"}, dir, &stdout, &stderr)
	var result performance.Report
	if code != 3 || json.Unmarshal(stdout.Bytes(), &result) != nil || result.Complete || len(result.Profiles) != 0 || strings.Contains(stdout.String(), "never-echo-this") || stderr.Len() != 0 {
		t.Fatalf("%d %s %s", code, &stdout, &stderr)
	}
}
