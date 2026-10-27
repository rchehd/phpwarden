package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	buildinfo "github.com/rchehd/phpwarden"
)

func TestPerformanceClientHelpAndStrictArguments(t *testing.T) {
	for _, action := range []string{"connect", "disconnect"} {
		var stdout, stderr bytes.Buffer
		if code := Run([]string{"performance", action, "--help"}, t.TempDir(), &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "--client") || stderr.Len() != 0 {
			t.Fatalf("%d %s %s", code, &stdout, &stderr)
		}
	}
	for _, options := range [][]string{nil, {"--client=none"}, {"--client=unknown"}, {"--client=codex,codex"}, {"--client=codex", "--client=cursor"}, {"--client"}, {"--client=all", "--apply"}, {"--client=all", "--reports=/tmp"}, {"--project=bad\npath"}} {
		var stdout, stderr bytes.Buffer
		args := append([]string{"performance", "connect"}, options...)
		args = append(args, "--format=json")
		code := Run(args, t.TempDir(), &stdout, &stderr)
		var result struct {
			ExitCode int `json:"exit_code"`
		}
		if code != 2 || json.Unmarshal(stdout.Bytes(), &result) != nil || result.ExitCode != 2 || stderr.Len() != 0 {
			t.Fatalf("%v: %d %s %s", options, code, &stdout, &stderr)
		}
	}
}

func TestPerformanceClientsRequireHostAndSavedSetup(t *testing.T) {
	for _, distribution := range []string{"standalone", "container"} {
		t.Run(distribution, func(t *testing.T) {
			prior := buildinfo.Distribution
			buildinfo.Distribution = distribution
			defer func() { buildinfo.Distribution = prior }()
			var stdout, stderr bytes.Buffer
			code := Run([]string{"performance", "connect", "--client=all", "--state-dir", t.TempDir(), "--format=json"}, t.TempDir(), &stdout, &stderr)
			message := "setup first"
			if distribution == "container" {
				message = "host CLI"
			}
			if code != 2 || !strings.Contains(stdout.String(), message) || stderr.Len() != 0 {
				t.Fatalf("%d %s %s", code, &stdout, &stderr)
			}
		})
	}
}
