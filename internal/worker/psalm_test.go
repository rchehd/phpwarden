package worker

import (
	"context"
	"os"
	"testing"

	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
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

func TestPsalmRegisteredWorkerCheckFailsClosedWithoutItsBundle(t *testing.T) {
	for _, runtime := range []string{"8.3", "8.4"} {
		for _, required := range []bool{false, true} {
			location, effective, _ := fixture(t)
			cfg := effective.Config
			cfg.PHP.Runtime = runtime
			cfg.PHP.Target.Min, cfg.PHP.Target.Max = runtime, runtime
			cfg.Tools.Psalm.Required = required
			selection := scope.Selection{Mode: "full", Files: []string{"src/demo.module"}, ProjectFiles: []string{}, LineMap: map[string][]scope.Range{"src/demo.module": nil}}
			result := analyze(context.Background(), location, cfg, nil, selection, []string{"psalm"}, nil, processFunc(func(context.Context, runner.Command) runner.Result {
				t.Fatal("missing bundle or unsupported runtime must not start PHP")
				return runner.Result{}
			}))
			wantReason := "dependencies_missing"
			if runtime == "8.4" {
				wantReason = "runtime_unsupported"
			}
			if result.Complete || required && result.ExitCode != 3 || !required && result.ExitCode != 0 || len(result.Findings) != 0 || len(result.Checks) != 1 || result.Checks[0].ID != "psalm" || result.Checks[0].Status != "unavailable" || result.Checks[0].Reason == nil || *result.Checks[0].Reason != wantReason {
				t.Fatalf("registered checker did not fail closed: %+v", result)
			}
		}
	}
}
