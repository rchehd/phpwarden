package report

import (
	"encoding/json"
	"testing"

	"github.com/rchehd/phpwarden/schemas"
)

func TestTraceLocationValidation(t *testing.T) {
	for _, variant := range []string{"valid", "symbol", "absolute", "traversal", "empty", "dot", "unclean", "backslash", "line", "column", "range", "end-column", "symbol-location", "origin", "label"} {
		t.Run(variant, func(t *testing.T) {
			step := TraceStep{Label: "source", Origin: "project", Path: new("src/input.php"), Line: new(2), Column: new(4), EndLine: new(2), EndColumn: new(8)}
			switch variant {
			case "symbol":
				step = TraceStep{Label: "source", Origin: "symbol"}
			case "absolute":
				step.Path = new("/tmp/input.php")
			case "traversal":
				step.Path = new("../input.php")
			case "empty":
				step.Path = new("")
			case "dot":
				step.Path = new(".")
			case "unclean":
				step.Path = new("src/./input.php")
			case "backslash":
				step.Path = new(`src\input.php`)
			case "line":
				step.Line = new(0)
			case "column":
				step.Column = new(0)
			case "range":
				step.EndLine = new(1)
			case "end-column":
				step.EndColumn = new(2)
			case "symbol-location":
				step.Origin = "symbol"
			case "origin":
				step.Origin = "outside"
			case "label":
				step.Label = " "
			}
			valid := variant == "valid" || variant == "symbol"
			if (ValidateTrace([]TraceStep{step}) == nil) != valid {
				t.Fatalf("unexpected validation: %+v", step)
			}
			if valid {
				result := New()
				result.Findings = []Finding{{Checker: "demo", Rule: "taint", Kind: "code", Severity: "warning", Message: "Flow", Path: new("src/input.php"), Line: new(2), Trace: []TraceStep{step}}}
				data, _ := json.Marshal(result)
				var document any
				_ = json.Unmarshal(data, &document)
				if err := schemas.ValidateReport(document); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
