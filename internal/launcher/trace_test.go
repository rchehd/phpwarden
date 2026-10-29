package launcher

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/runner"
)

func TestWorkerTraceCannotEscapeDeclaredOrigin(t *testing.T) {
	for _, variant := range []string{"valid", "absolute", "traversal", "range", "symbol"} {
		t.Run(variant, func(t *testing.T) {
			location, effective, options := fixture(t)
			var calls []runner.Command
			base := dockerDouble(t, location, &calls)
			process := processFunc(func(ctx context.Context, command runner.Command) runner.Result {
				result := base(ctx, command)
				if command.Args[0] != "run" {
					return result
				}
				var document report.Report
				if err := json.Unmarshal(result.Stdout, &document); err != nil || len(document.Findings) != 1 {
					t.Fatalf("invalid transport fixture: %v", err)
				}
				step := report.TraceStep{Label: "source", Origin: "project", Path: new("src/demo.php"), Line: new(2)}
				switch variant {
				case "absolute":
					step.Path = new("/tmp/private/source.php")
				case "traversal":
					step.Path = new("../source.php")
				case "range":
					step.EndLine = new(1)
				case "symbol":
					step.Origin = "symbol"
				}
				document.Findings[0].Trace = []report.TraceStep{step}
				result.Stdout, _ = json.Marshal(document)
				return result
			})
			result := Check(context.Background(), location, effective, options, []string{"debug"}, process)
			if variant == "valid" {
				if !result.Complete || result.ExitCode != 1 || len(result.Findings) != 1 || len(result.Findings[0].Trace) != 1 {
					t.Fatalf("valid trace was lost: %+v", result)
				}
			} else if result.Complete || result.ExitCode != 3 || len(result.Findings) != 0 {
				t.Fatalf("invalid worker trace accepted: %+v", result)
			}
		})
	}
}
