package textreport

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/report"
)

func TestEveryCheckerHasCategory(t *testing.T) {
	for _, id := range checker.IDs() {
		if categoryName(id) == "Other checks" {
			t.Errorf("checker %s has no category", id)
		}
	}
}

func TestDefaultTextGroupsFailuresFindingsAndSummaryByCategory(t *testing.T) {
	result := base("demo.php", "composer.json")
	result.Checks = []report.Check{
		check("eslint", "error", true, "dependencies_missing"),
		check("phpcs", "passed", true, ""),
		check("stylelint", "skipped", true, "no_config"),
		check("composer-validate", "passed", true, ""),
		check("gitleaks", "passed", true, ""),
		check("semgrep", "skipped", true, "disabled"),
		check("deptrac", "skipped", true, "disabled"),
	}
	result.Diagnostics = []report.Diagnostic{
		{Checker: new("eslint"), Code: "dependencies_missing", Message: "Failed to load config airbnb-base."},
		{Checker: new("stylelint"), Code: "no_config", Message: "No stylesheet configuration for demo.css."},
	}
	result.Findings = []report.Finding{
		{Checker: "phpcs", Rule: "style.example", Severity: "warning", Message: "Use a single space", Path: new("demo.php"), Line: new(4)},
		{Checker: "composer-validate", Rule: "composer.validate.warning", Severity: "warning", Message: "Avoid exact version constraints", Path: new("composer.json")},
	}
	result.Finalize()
	before, _ := json.Marshal(result)
	var output bytes.Buffer
	if err := Write(&output, result, Options{}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, forbidden := range []string{"Skipped:", "eslint", "phpcs", "stylelint", "composer-validate", "gitleaks", "semgrep", "deptrac", "style.example", "composer.validate.warning"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("implementation detail %q leaked into default output:\n%s", forbidden, text)
		}
	}
	start, end := strings.Index(text, "\nCode standards\n"), strings.Index(text, "\nDependencies\n")
	if start < 0 || end <= start {
		t.Fatalf("missing ordered category sections:\n%s", text)
	}
	section := text[start:end]
	for _, want := range []string{"ERROR", "Failed to load config airbnb-base.", "WARNING", "No stylesheet configuration", "Use a single space", "demo.php"} {
		if !strings.Contains(section, want) {
			t.Errorf("category lost %q:\n%s", want, section)
		}
	}
	summary := text[strings.LastIndex(text, "\nChecks\n"):]
	for _, want := range []string{"✗ Code standards", "! Dependencies", "✓ Security", "- Architecture", "no config", "disabled"} {
		if !strings.Contains(summary, want) {
			t.Errorf("category summary lost %q:\n%s", want, summary)
		}
	}
	after, _ := json.Marshal(result)
	if !bytes.Equal(before, after) {
		t.Fatal("category presentation changed the underlying report")
	}
}

func TestCategorySummaryPreservesCoverageAndFailurePolicies(t *testing.T) {
	for _, tc := range []struct {
		name, status, reason, want string
		required                   bool
	}{
		{"required failure", "error", "checker_timeout", "error", true},
		{"optional failure", "unavailable", "dependencies_missing", "incomplete", false},
		{"missing configuration", "skipped", "no_config", "partial", true},
		{"no applicable files", "skipped", "no_files", "passed", true},
		{"disabled", "skipped", "disabled", "passed", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := base("demo.php")
			result.Checks = []report.Check{check("phpcs", "passed", true, ""), check("stylelint", tc.status, tc.required, tc.reason)}
			result.Finalize()
			var output bytes.Buffer
			if err := Write(&output, result, Options{}); err != nil {
				t.Fatal(err)
			}
			summary := output.String()[strings.LastIndex(output.String(), "\nChecks\n"):]
			if !strings.Contains(summary, tc.want) || tc.status == "skipped" && !strings.Contains(summary, strings.ReplaceAll(tc.reason, "_", " ")) {
				t.Fatalf("category hid coverage or policy: %s", summary)
			}
		})
	}
}

func TestCategoryFindingsRespectEachChecksFailurePolicy(t *testing.T) {
	for _, policy := range []string{"never", "error", "warning"} {
		result := base("demo.php")
		result.Checks = []report.Check{check("phpcs", "passed", true, ""), check("eslint", "passed", true, "")}
		result.Checks[0].FailOn = policy
		result.Checks[1].FailOn = "warning"
		result.Findings = []report.Finding{{Checker: "phpcs", Rule: "style.rule", Severity: "warning", Message: "A style suggestion", Path: new("demo.php")}}
		result.Finalize()
		var output bytes.Buffer
		if err := Write(&output, result, Options{}); err != nil {
			t.Fatal(err)
		}
		want := "! Code standards  warning"
		if policy == "warning" {
			want = "✗ Code standards  failed"
		}
		if !strings.Contains(output.String(), want) {
			t.Fatalf("category changed failure policy %s:\n%s", policy, output.String())
		}
	}
}
