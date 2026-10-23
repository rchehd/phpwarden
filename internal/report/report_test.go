package report

import (
	"encoding/json"
	"testing"

	"github.com/rchehd/phpwarden/schemas"
)

func TestFailurePrecedenceAndOptionalCoverage(t *testing.T) {
	for _, testcase := range []struct {
		name     string
		checks   []Check
		code     int
		complete bool
	}{
		{"clean", []Check{{ID: "debug", Status: "passed", Required: true}}, 0, true},
		{"findings", []Check{{ID: "debug", Status: "failed", Required: true}}, 1, true},
		{"optional findings still block", []Check{{ID: "debug", Status: "failed", Required: false}}, 1, true},
		{"optional unavailable", []Check{{ID: "debug", Status: "unavailable", Required: false}}, 0, false},
		{"required unavailable", []Check{{ID: "debug", Status: "unavailable", Required: true}}, 3, false},
		{"failure then error", []Check{{ID: "first", Status: "failed"}, {ID: "second", Status: "error", Required: true}}, 3, false},
		{"error then failure", []Check{{ID: "first", Status: "error", Required: true}, {ID: "second", Status: "failed"}}, 3, false},
	} {
		t.Run(testcase.name, func(t *testing.T) {
			result := New()
			result.Checks = testcase.checks
			result.Finalize()
			if result.ExitCode != testcase.code || result.Complete != testcase.complete {
				t.Fatalf("bad aggregate: %+v", result)
			}
		})
	}
}

func TestEarlierFindingsStillBlockWhenOptionalCheckerLaterFails(t *testing.T) {
	for _, policy := range []string{"error", "never"} {
		result := New()
		result.Checks = []Check{{ID: "syntax", Status: "error", Required: false, FailOn: policy, Reason: new("process_failed")}}
		result.Findings = []Finding{{Checker: "syntax", Rule: "php.syntax", Kind: "syntax", Severity: "error", Message: "Syntax error", Path: new("demo.php"), Line: new(2)}}
		result.Finalize()
		want := 1
		if policy == "never" {
			want = 0
		}
		if result.Complete || result.ExitCode != want {
			t.Fatalf("partial findings lost their policy: %+v", result)
		}
		data, _ := json.Marshal(result)
		var document any
		if err := json.Unmarshal(data, &document); err != nil {
			t.Fatal(err)
		}
		if err := schemas.ValidateReport(document); err != nil {
			t.Fatal(err)
		}
	}
}
