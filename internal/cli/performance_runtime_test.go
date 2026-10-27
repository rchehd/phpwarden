package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	buildinfo "github.com/rchehd/phpwarden"
	"github.com/rchehd/phpwarden/internal/perfruntime"
)

func TestPerformanceLifecycleHelpAndStrictOptions(t *testing.T) {
	for _, action := range []string{"setup", "status", "teardown"} {
		var stdout, stderr bytes.Buffer
		if code := Run([]string{"performance", action, "--help"}, t.TempDir(), &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "--apply") || stderr.Len() != 0 {
			t.Fatalf("%d %s %s", code, &stdout, &stderr)
		}
	}
	for _, args := range [][]string{{"status", "--apply"}, {"status", "--http"}, {"teardown", "--php=8.3"}, {"setup", "--apply=true"}, {"setup", "--service"}, {"setup", "--project=one", "--project=two"}, {"setup", "--format=compact"}, {"setup", "--only=phpstan"}, {"setup", "--http-allow=bad\nvalue"}} {
		var stdout, stderr bytes.Buffer
		input := append([]string{"performance"}, args...)
		input = append(input, "--format=json")
		code := Run(input, t.TempDir(), &stdout, &stderr)
		var r perfruntime.Report
		if code != 2 || json.Unmarshal(stdout.Bytes(), &r) != nil || r.ExitCode != 2 || r.Loaded || stderr.Len() != 0 {
			t.Fatalf("%v: %d %s %s", args, code, &stdout, &stderr)
		}
	}
}

func TestPerformanceAbsentStatusNeedsNoToolsOrConfig(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	for _, action := range []string{"status", "teardown"} {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"performance", action, "--state-dir", t.TempDir(), "--format=json"}, t.TempDir(), &stdout, &stderr)
		var r perfruntime.Report
		if code != 0 || json.Unmarshal(stdout.Bytes(), &r) != nil || r.State != "absent" || r.Prepared || r.Loaded || r.RuntimeChecked || stderr.Len() != 0 {
			t.Fatalf("%d %s %s", code, &stdout, &stderr)
		}
	}
}

func TestPerformanceContainerDistributionRefusesRuntimeManagement(t *testing.T) {
	previous := buildinfo.Distribution
	buildinfo.Distribution = "container"
	defer func() { buildinfo.Distribution = previous }()
	var stdout, stderr bytes.Buffer
	code := Run([]string{"performance", "setup", "--apply", "--format=json"}, t.TempDir(), &stdout, &stderr)
	var r perfruntime.Report
	if code != 2 || json.Unmarshal(stdout.Bytes(), &r) != nil || r.ExitCode != 2 || !strings.Contains(stdout.String(), "host CLI") {
		t.Fatalf("%d %s %s", code, &stdout, &stderr)
	}
}
