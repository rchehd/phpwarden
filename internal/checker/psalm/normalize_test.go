package psalm

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/psalmanalysis"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/scope"
	"github.com/rchehd/phpwarden/internal/textreport"
	"github.com/rchehd/phpwarden/schemas"
)

func captured(t *testing.T, name string) psalmanalysis.Result {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Roots            struct{ Project, Bundle, Generated string }
		Sources, Context []string
		Result           psalmanalysis.Result
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.Result.Roots = psalmanalysis.Roots(fixture.Roots)
	fixture.Result.SourceFiles, fixture.Result.ContextFiles = fixture.Sources, fixture.Context
	return fixture.Result
}

func selected(files ...string) scope.Selection {
	result := scope.Selection{Mode: "diff", Files: files, LineMap: map[string][]scope.Range{}, ProjectFiles: []string{}}
	for _, name := range files {
		result.LineMap[name] = []scope.Range{{Start: 999, End: 999}}
	}
	return result
}

func envelope(findings []report.Finding, check report.Check) report.Report {
	result := report.New()
	result.Checks, result.Findings = []report.Check{check}, findings
	result.Finalize()
	return result
}

func validate(t *testing.T, result report.Report) []byte {
	t.Helper()
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if err := schemas.ValidateReport(document); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCapturedDrupalReportsRetainEveryFlow(t *testing.T) {
	for _, name := range []string{"drupal10-automatic", "drupal11-automatic", "drupal10-monorepo", "drupal11-monorepo"} {
		t.Run(name, func(t *testing.T) {
			native := captured(t, name)
			outcome := Normalize(native, selected(native.SourceFiles...), Policy{Required: true, FailOn: "never"})
			if outcome.Check.Status != "passed" || len(outcome.Diagnostics) != 0 || len(outcome.Findings) != 7 {
				t.Fatalf("unexpected outcome: %+v", outcome)
			}
			for index, finding := range outcome.Findings {
				var row issue
				_ = json.Unmarshal(native.Findings[index], &row)
				if finding.Rule != "psalm."+row.Type || finding.Fingerprint == nil || len(finding.Trace) != len(row.Trace) || finding.Severity != row.Severity {
					t.Fatalf("native issue lost information: %+v", finding)
				}
				for i, step := range finding.Trace {
					if row.Trace[i].File == "" {
						if step.Origin != "symbol" || step.Path != nil || step.Line != nil || step.Label != row.Trace[i].Label {
							t.Fatalf("native symbol lost: %+v", step)
						}
						continue
					}
					if step.Label != row.Trace[i].Label || step.Snippet != row.Trace[i].Snippet || step.Line == nil || *step.Line != row.Trace[i].Line || step.EndColumn == nil || *step.EndColumn != row.Trace[i].EndColumn {
						t.Fatalf("native trace changed: %+v", step)
					}
				}
			}
			last := outcome.Findings[len(outcome.Findings)-1]
			if !strings.HasSuffix(*last.Path, "/Controller/ProbeController.php") || *last.Line != 28 || last.Trace[len(last.Trace)-1].Origin != "generated" {
				t.Fatalf("generated sink was not anchored to the project trace: %+v", last)
			}
			result := envelope(outcome.Findings, outcome.Check)
			data := validate(t, result)
			for _, forbidden := range []string{"/project/", "/generated/", "/tool-vendor/", "phpwarden-psalm-worker-"} {
				if bytes.Contains(data, []byte(forbidden)) {
					t.Fatalf("private location leaked: %s", forbidden)
				}
			}
			if !result.Complete || result.ExitCode != 0 || len(result.Findings) != 7 {
				t.Fatalf("advisory flows affected completeness or were deduplicated: %+v", result)
			}
		})
	}
}

func TestDiffMatchesSourceIntermediateSinkAndConsumedMetadata(t *testing.T) {
	native := captured(t, "drupal11-automatic")
	root := "web/modules/custom/catalog_review/"
	for _, tc := range []struct {
		name string
		file string
		meta bool
		want int
	}{
		{"helper body changed outside trace lines", root + "src/InputHelper.php", false, 4},
		{"sink controller", root + "src/Controller/ProbeController.php", false, 5},
		{"form", root + "src/Form/ProbeForm.php", false, 1},
		{"hook", root + "catalog_review.module", false, 1},
		{"route entrypoints changed", root + "catalog_review.routing.yml", false, 7},
		{"service metadata changed", root + "catalog_review.services.yml", false, 7},
		{"composer context changed", "composer.lock", true, 7},
		{"unconsumed metadata", "README.md", true, 0},
		{"unrelated source", "unrelated.php", false, 0},
		{"empty diff", "", false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			selection := selected()
			if tc.file != "" {
				selection = selected(tc.file)
			}
			if tc.meta {
				selection.ProjectFiles, selection.Files = []string{tc.file}, nil
			}
			outcome := Normalize(native, selection, Policy{FailOn: "never"})
			if outcome.Check.Status != "passed" || len(outcome.Findings) != tc.want {
				t.Fatalf("want %d flows, got %d; check: %+v; diagnostics: %+v", tc.want, len(outcome.Findings), outcome.Check, outcome.Diagnostics)
			}
		})
	}
}

func TestMalformedHiddenFindingsCannotBecomeClean(t *testing.T) {
	for _, variant := range []string{"path-escape", "outside", "relative", "no-line", "backward-range", "bad-column", "no-trace", "bad-trace", "foreign-anchor", "no-message", "no-type", "severity", "references", "null", "null-findings", "roots", "inventory", "diagnostic", "runtime"} {
		t.Run(variant, func(t *testing.T) {
			native := captured(t, "drupal11-automatic")
			var row map[string]any
			_ = json.Unmarshal(native.Findings[0], &row)
			switch variant {
			case "path-escape":
				row["file_path"] = "/project/../etc/passwd"
			case "outside":
				row["file_path"] = "/outside/source.php"
			case "relative":
				row["file_path"] = "src.php"
			case "no-line":
				row["line_from"] = 0
			case "backward-range":
				row["line_to"] = 1
			case "bad-column":
				row["column_from"] = -1
			case "no-trace":
				row["taint_trace"] = []any{}
			case "bad-trace":
				row["taint_trace"].([]any)[0].(map[string]any)["file_path"] = "/other/file.php"
			case "foreign-anchor":
				native.SourceFiles = []string{"other.php"}
			case "no-message":
				delete(row, "message")
			case "no-type":
				delete(row, "type")
			case "severity":
				row["severity"] = "notice"
			case "references":
				row["other_references"] = []string{"unmodeled source"}
			case "roots":
				native.Roots.Bundle = "/project/vendor/tools"
			case "inventory":
				native.ContextFiles = []string{"../composer.json"}
			case "diagnostic":
				native.Diagnostics = []psalmanalysis.Diagnostic{{Stage: "taint", Code: "stale", Message: "Changed"}}
			case "runtime":
				native.PHP = "8.3"
			}
			native.Findings[0], _ = json.Marshal(row)
			if variant == "null" {
				native.Findings[0] = json.RawMessage("null")
			}
			if variant == "null-findings" {
				native.Findings = nil
			}
			outcome := Normalize(native, selected("unrelated.php"), Policy{Required: true, FailOn: "never"})
			if outcome.Check.Status != "error" || len(outcome.Findings) != 0 || len(outcome.Diagnostics) == 0 {
				t.Fatalf("malformed hidden finding became clean: %+v", outcome)
			}
		})
	}
}

func TestFailuresRemainDiagnosticsUnderAdvisoryPolicy(t *testing.T) {
	for _, stage := range []string{"dependencies", "runtime", "syntax", "readiness", "taint", "freshness", "cleanup"} {
		for _, required := range []bool{true, false} {
			native := captured(t, "drupal11-automatic")
			native.Complete = false
			native.Diagnostics = []psalmanalysis.Diagnostic{{Stage: stage, Code: "incomplete", Message: "Native phase failed"}}
			outcome := Normalize(native, selected(), Policy{Required: required, FailOn: "never"})
			if len(outcome.Findings) != 0 || len(outcome.Diagnostics) != 1 || outcome.Diagnostics[0].Stage != stage {
				t.Fatalf("partial findings leaked: %+v", outcome)
			}
			result := envelope(outcome.Findings, outcome.Check)
			result.Diagnostics = outcome.Diagnostics
			validate(t, result)
			if result.Complete || required && result.ExitCode != 3 || !required && result.ExitCode != 0 {
				t.Fatalf("execution failure hidden by policy: %+v", result)
			}
		}
	}
}

func TestReadinessDetailsAreDiagnosticsEvenOutsideReportScope(t *testing.T) {
	native := captured(t, "drupal11-automatic")
	native.Complete = false
	native.Diagnostics = []psalmanalysis.Diagnostic{{Stage: "readiness", Code: "readiness_incomplete", Message: "Ordinary analysis must complete"}}
	row := json.RawMessage(`{"type":"UndefinedClass","message":"Class MissingType does not exist","file_path":"/project/src/Controller.php","line_from":12}`)
	for range 12 {
		native.Readiness = append(native.Readiness, row)
	}
	outcome := Normalize(native, selected("unrelated.php"), Policy{Required: true, FailOn: "never"})
	if len(outcome.Findings) != 0 || len(outcome.Diagnostics) != 12 || !strings.Contains(outcome.Diagnostics[1].Message, "src/Controller.php:12: UndefinedClass") || !strings.Contains(outcome.Diagnostics[11].Message, "2 additional") {
		t.Fatalf("readiness detail was lost or filtered: %+v", outcome)
	}
	if result := envelope(outcome.Findings, outcome.Check); result.Complete || result.ExitCode != 3 {
		t.Fatal("readiness failure became advisory success")
	}
}

func TestFingerprintsAndDistinctFlowsSurviveReportFinalization(t *testing.T) {
	native := captured(t, "drupal11-automatic")
	native.Findings = native.Findings[:1]
	var variant map[string]any
	_ = json.Unmarshal(native.Findings[0], &variant)
	variant["taint_trace"].([]any)[1].(map[string]any)["label"] = "second input path"
	other, _ := json.Marshal(variant)
	native.Findings = append(native.Findings, other, native.Findings[0])
	outcome := Normalize(native, selected(native.SourceFiles...), Policy{Required: true, FailOn: "error"})
	result := envelope(outcome.Findings, outcome.Check)
	if !result.Complete || result.ExitCode != 1 || len(result.Findings) != 2 || *result.Findings[0].Fingerprint == *result.Findings[1].Fingerprint {
		t.Fatalf("distinct flows at one sink lost: %+v", result)
	}
	for _, field := range []*string{&native.Roots.Project, &native.Roots.Bundle, &native.Roots.Generated} {
		previous := *field
		*field = "/relocated" + previous
		for index, raw := range native.Findings {
			native.Findings[index] = json.RawMessage(strings.ReplaceAll(string(raw), previous+"/", *field+"/"))
		}
	}
	again := Normalize(native, selected(native.SourceFiles...), Policy{Required: true, FailOn: "error"})
	if len(again.Findings) != len(outcome.Findings) || *again.Findings[0].Fingerprint != *outcome.Findings[0].Fingerprint {
		t.Fatalf("fingerprint depends on temporary location: %+v", again)
	}
}

func TestSecurityTextShowsBoundedTraceAndAllExpandsIt(t *testing.T) {
	native := captured(t, "drupal11-automatic")
	outcome := Normalize(native, selected(native.SourceFiles...), Policy{Required: true, FailOn: "never"})
	result := envelope(outcome.Findings, outcome.Check)
	result.Findings = result.Findings[:1]
	step := report.TraceStep{Label: "intermediate flow step", Origin: "symbol"}
	result.Findings[0].Trace = append(slices.Clone(result.Findings[0].Trace), step, step, step, step, step)
	for _, all := range []bool{false, true} {
		var output bytes.Buffer
		if err := textreport.Write(&output, result, textreport.Options{All: all}); err != nil {
			t.Fatal(err)
		}
		text := output.String()
		if !strings.Contains(text, "Security") || !strings.Contains(text, "Untrusted HTML reaches output") || !strings.Contains(text, "→") || strings.Contains(text, "Other checks") || strings.Contains(text, "psalm.Tainted") {
			t.Fatalf("unexpected presentation: %s", text)
		}
		if strings.Contains(text, "flow steps omitted") == all {
			t.Fatalf("trace expansion failed: %s", text)
		}
		if strings.Contains(text, "tool:") != all || strings.Contains(text, `Symfony\Component\HttpFoundation`) != all {
			t.Fatalf("implementation paths were not limited to expanded output: %s", text)
		}
	}
}
