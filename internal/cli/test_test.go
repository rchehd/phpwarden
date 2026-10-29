package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/projecttest"
)

func TestProjectTestHelpAndInvalidArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunContext(context.Background(), []string{"test", "--help"}, t.TempDir(), strings.NewReader(""), &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "--phpunit-config") {
		t.Fatalf("%d %s %s", code, &stdout, &stderr)
	}
	for _, args := range [][]string{
		{"test", "--format=json", "--format=json"},
		{"test", "--format=json", "--coverage=false"},
		{"test", "--format=json", "--coverage", "--coverage"},
		{"test", "--format=json", "--unknown=value"},
		{"test", "--format=json", "--testsuite=unit", "tests"},
		{"test", "--format=json", "--project=/missing"},
	} {
		stdout.Reset()
		stderr.Reset()
		code := RunContext(context.Background(), args, t.TempDir(), strings.NewReader(""), &stdout, &stderr)
		var result projecttest.Report
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || code != 2 || result.Complete || result.ExitCode != 2 || len(result.Diagnostics) == 0 {
			t.Fatalf("%v: %d %s %v", args, code, &stdout, err)
		}
	}
}

func TestMutationHelpAndInvalidArguments(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := RunContext(context.Background(), []string{"mutation", "--help"}, t.TempDir(), strings.NewReader(""), &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "--infection-config") {
		t.Fatalf("%d %s", code, stdout.String())
	}
	for _, args := range [][]string{
		{"mutation", "--format=json", "--mode=unknown"},
		{"mutation", "--format=json", "--commits=0"},
		{"mutation", "--format=json", "--whole-files=true"},
		{"mutation", "--format=json", "--project=/missing"},
	} {
		stdout.Reset()
		stderr.Reset()
		code := RunContext(context.Background(), args, t.TempDir(), strings.NewReader(""), &stdout, &stderr)
		var result projecttest.Report
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || code != 2 || result.Command != "mutation" || result.Complete {
			t.Fatalf("%v: %d %s", args, code, stdout.String())
		}
	}
}
