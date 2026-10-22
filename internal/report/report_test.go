package report

import "testing"

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
