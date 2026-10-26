package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	buildinfo "github.com/rchehd/phpwarden"
	"github.com/rchehd/phpwarden/internal/selfupdate"
)

func TestUpdateSourceBuildRefusesMutationWithoutProjectOrNetwork(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Run([]string{"update", "--format=json"}, t.TempDir(), &stdout, &stderr)
	var result selfupdate.Report
	if code != 3 || json.Unmarshal(stdout.Bytes(), &result) != nil || !strings.Contains(result.Message, "source installation") || stderr.Len() != 0 {
		t.Fatalf("%d %s %s", code, &stdout, &stderr)
	}
}

func TestUpdateUsageAndBuildMetadata(t *testing.T) {
	for _, args := range [][]string{{"update", "--to"}, {"update", "--project=unknown"}, {"update", "--check=yes"}, {"update", "--format=compact"}, {"update", "--check", "--check"}, {"update", "--to=bad", "--format=json"}, {"update", "--to", "--check"}, {"--build-info", "--version"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, t.TempDir(), &stdout, &stderr); code != 2 {
			t.Fatalf("%v: %d %s %s", args, code, &stdout, &stderr)
		}
	}
	for _, args := range [][]string{{"--build-info"}, {"update", "--help"}} {
		var stdout, stderr bytes.Buffer
		if code := Run(args, t.TempDir(), &stdout, &stderr); code != 0 || stderr.Len() != 0 {
			t.Fatalf("%v: %d %s %s", args, code, &stdout, &stderr)
		}
		if args[0] == "--build-info" {
			var info buildinfo.Info
			if json.Unmarshal(stdout.Bytes(), &info) != nil || info.Distribution != "source" || info.Version != "dev" {
				t.Fatalf("bad build identity %s", &stdout)
			}
		}
	}
}
