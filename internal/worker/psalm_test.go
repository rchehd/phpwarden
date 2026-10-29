package worker

import (
	"context"
	"os"
	"testing"

	"github.com/rchehd/phpwarden/internal/runner"
)

func TestPsalmRejectsRawConfigurationChangesBeforeExecution(t *testing.T) {
	location, effective, _ := fixture(t)
	data, err := os.ReadFile(location.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(location.Path, append(data, []byte("\n# changed\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	result := AnalyzePsalm(context.Background(), location, effective, PsalmOptions{}, processFunc(func(context.Context, runner.Command) runner.Result {
		t.Fatal("stale configuration reached a native process")
		return runner.Result{}
	}))
	if result.Complete || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "stale" {
		t.Fatalf("stale configuration accepted: %+v", result)
	}
}
