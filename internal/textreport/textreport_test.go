package textreport

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/scope"
)

var update = flag.Bool("update", false, "rewrite golden files")

func TestPublishedImageFailureDoesNotSuggestLocalBuild(t *testing.T) {
	result := report.New()
	result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "published_image_unavailable", Stage: "launcher", Message: "worker image download failed"})
	result.Finalize()
	var output bytes.Buffer
	if err := Write(&output, result, Options{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "docker buildx") || !strings.Contains(output.String(), "runner.image_pull") {
		t.Fatalf("wrong distribution guidance: %s", &output)
	}
}

func base(files ...string) report.Report {
	result := report.New()
	result.Scope = &scope.Selection{Mode: "diff", BaseRef: new("origin/main"), Files: files}
	result.Environment = &report.Environment{FrameworkVersion: new("11.3.16"), PHPVersion: new("8.4.26"), RequestedRuntime: "8.4"}
	return result
}

func check(id, status string, required bool, reason string) report.Check {
	result := report.Check{ID: id, Status: status, Required: required, FailOn: "error"}
	if reason != "" {
		result.Reason = &reason
	}
	return result
}

func scenarios() map[string]report.Report {
	clean := base("web/modules/custom/demo/demo.module")
	clean.Checks = []report.Check{check("debug", "passed", true, ""), check("phpcs", "passed", true, ""), check("yaml", "skipped", true, "no_files"), check("twig-cs-fixer", "skipped", true, "no_files"), check("composer-audit", "skipped", false, "disabled")}

	findings := base("web/modules/custom/demo/demo.module", "web/modules/custom/demo/new \nfile.module", "web/themes/custom/demo/card.html.twig")
	findings.Environment.DiscoveryWarnings = []string{`composer.json: duplicate JSON key "extra.patches"; the last value is used, as in Composer`}
	findings.Environment.Advisories = &report.Advisories{Source: "network", FetchedAt: "2026-10-23T10:01:45Z"}
	findings.Checks = []report.Check{check("composer-audit", "passed", false, ""), check("composer-validate", "passed", true, ""), check("debug", "failed", true, ""), check("phpstan", "failed", true, ""),
		check("twig-cs-fixer", "failed", true, ""), check("yaml", "passed", true, "")}
	findings.Findings = []report.Finding{
		{Checker: "phpstan", Rule: "return.type", Kind: "code", Severity: "error", Message: "Function demo() should return int but returns string.", Path: new("web/modules/custom/demo/demo.module"), Line: new(12)},
		{Checker: "debug", Rule: "debug.function", Kind: "code", Severity: "error", Message: "Debug function var_dump() is called", Path: new("web/modules/custom/demo/new \nfile.module"), Line: new(3), Column: new(1)},
		{Checker: "debug", Rule: "debug.function", Kind: "code", Severity: "error", Message: "Debug function dpm() is called", Path: new("web/modules/custom/demo/demo.module"), Line: new(108), Column: new(5)},
		{Checker: "phpstan", Rule: "function.deprecated", Kind: "code", Severity: "error", Message: "Call to deprecated function demo_old().", Path: new("web/modules/custom/demo/demo.module"), Line: new(4)},
		{Checker: "twig-cs-fixer", Rule: "File.Invalid", Kind: "global", Severity: "error", Message: `Twig template "web/themes/custom/demo/card.html.twig": Unable to tokenize file: The template is invalid.`},
		{Checker: "composer-validate", Rule: "composer.validate.warning", Kind: "global", Severity: "warning", Message: "Composer file \"composer.json\": \"composer.json\" does not contain valid JSON\nParse error on line 1:\n{\"name\":"},
		{Checker: "composer-audit", Rule: "SA-CORE-2026-016", Kind: "global", Severity: "warning", Message: `Composer file "composer.lock": drupal/core 11.3.16: Drupal core - Cross-site scripting (CVE-2026-108514) https://www.drupal.org/sa-core-2026-016`},
		{Checker: "composer-audit", Rule: "composer.abandoned", Kind: "global", Severity: "info", Message: `Composer file "composer.lock": acme/old is abandoned; use acme/new instead`},
	}
	findings.Diagnostics = []report.Diagnostic{{Code: "advisories_stale", Stage: "checker", Checker: new("composer-audit"), Message: "Using Composer advisories cached at 2026-10-20T09:00:00Z: network down"}}

	partial := base("composer.lock")
	partial.Checks = []report.Check{check("composer-audit", "unavailable", false, "advisories_unavailable"), check("debug", "skipped", true, "no_files")}
	partial.Diagnostics = []report.Diagnostic{{Code: "advisories_unavailable", Stage: "checker", Checker: new("composer-audit"), Message: "Composer advisories are unavailable and no cached result is recent enough: offline mode is enabled"}}

	timeout := base("web/modules/custom/demo/demo.module")
	timeout.Checks = []report.Check{check("phpstan", "error", true, "checker_timeout"), check("debug", "passed", true, "")}
	timeout.Diagnostics = []report.Diagnostic{{Code: "checker_timeout", Stage: "checker", Checker: new("phpstan"), Message: "PHPStan did not complete within its execution context"}}

	stale := base("web/modules/custom/demo/demo.module")
	stale.Checks = []report.Check{check("debug", "passed", true, "")}
	stale.Diagnostics = []report.Diagnostic{{Code: "stale", Stage: "launcher", Message: "Git scope changed during the check"}}

	input := report.InputError("compatibility", fmt.Errorf("no compatible entry in manifest \"development\" for profile=drupal framework=11.3.16 PHP runtime=8.3"))

	full := base("web/modules/custom/demo/demo.module")
	full.Scope.Mode, full.Scope.BaseRef, full.Environment.FrameworkVersion = "full", nil, nil
	full.Checks = []report.Check{check("debug", "passed", true, "")}

	result := map[string]report.Report{"clean": clean, "findings": findings, "partial": partial, "timeout": timeout, "stale": stale, "input-error": input, "full-scan": full}
	for name, value := range result {
		value.Finalize()
		result[name] = value
	}
	return result
}

func TestGoldenOutput(t *testing.T) {
	for name, result := range scenarios() {
		for _, compact := range []bool{false, true} {
			variant := name
			if compact {
				variant += "-compact"
			}
			t.Run(variant, func(t *testing.T) {
				var output bytes.Buffer
				if err := Write(&output, result, Options{Compact: compact}); err != nil {
					t.Fatal(err)
				}
				golden := filepath.Join("testdata", variant+".txt")
				if *update {
					if err := os.WriteFile(golden, output.Bytes(), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				want, err := os.ReadFile(golden)
				if err != nil {
					t.Fatal(err)
				}
				if output.String() != string(want) {
					t.Fatalf("output differs from %s:\n%s", golden, output.String())
				}
				if strings.Contains(output.String(), "\x1b[") || strings.Contains(output.String(), " \n") {
					t.Fatal("plain output contains ANSI codes or trailing spaces")
				}
			})
		}
	}
}

func TestColorOnlyWhenRequested(t *testing.T) {
	result := scenarios()["findings"]
	var output bytes.Buffer
	if err := Write(&output, result, Options{Color: true}); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"\x1b[31mERROR ", "\x1b[33mWARNING ", "\x1b[36mINFO ", "\x1b[31;1mFAIL\x1b[0m"} {
		if !strings.Contains(output.String(), code) {
			t.Fatalf("colored output misses %q:\n%s", code, output.String())
		}
	}
}

func TestLastCommitsHeader(t *testing.T) {
	for ref, want := range map[string]string{"HEAD~1": "changes in the last commit ·", "HEAD~3": "changes in the last 3 commits ·", "HEAD~x": "changes against HEAD~x ·"} {
		result := base("web/modules/custom/demo/demo.module")
		result.Scope.BaseRef = new(ref)
		result.Checks = []report.Check{check("debug", "passed", true, "")}
		result.Finalize()
		var output bytes.Buffer
		if err := Write(&output, result, Options{}); err != nil || !strings.Contains(output.String(), want) {
			t.Fatalf("header for %s misses %q: %s %v", ref, want, &output, err)
		}
	}
}

func TestRuleGroupsCollapseRepeatedMessagesAndKeepDistinctDetails(t *testing.T) {
	result := base("a.php", "b.php")
	result.Checks = []report.Check{check("phpstan", "failed", true, ""), check("other", "passed", true, "")}
	result.Findings = []report.Finding{
		{Checker: "phpstan", Rule: "method.notFound", Severity: "error", Message: "Call to undefined method A::run().", Path: new("a.php"), Line: new(12)},
		{Checker: "phpstan", Rule: "method.notFound", Severity: "error", Message: "Call to undefined method A::run().", Path: new("a.php"), Line: new(24)},
		{Checker: "phpstan", Rule: "method.notFound", Severity: "error", Message: "Call to undefined method B::stop().", Path: new("b.php"), Line: new(7)},
		{Checker: "other", Rule: "method.notFound", Severity: "warning", Message: "Advisory only.", Path: new("a.php"), Line: new(3)},
	}
	result.Finalize()
	var output bytes.Buffer
	if err := Write(&output, result, Options{}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{"2 categories", "\nCorrectness\n", "\nOther checks\n", "Undefined methods", "12 · 24", "B::stop()"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q:\n%s", want, text)
		}
	}
	if strings.Count(text, "Call to undefined method A::run().") != 1 {
		t.Fatalf("repeated explanation was not collapsed:\n%s", text)
	}
	if strings.Index(text, "Result: FAIL") > strings.Index(text, "\nCorrectness\n") || strings.Index(text, "\nCorrectness\n") > strings.Index(text, "\nOther checks\n") {
		t.Fatalf("verdict and errors must precede advisory groups:\n%s", text)
	}
}

func TestLargeRuleGroupShowsOmissionsWithoutChangingReport(t *testing.T) {
	result := base("a.php", "b.php")
	result.Checks = []report.Check{check("whitespace", "failed", true, "")}
	for line := 1; line <= 15; line++ {
		name := "a.php"
		if line > 10 {
			name = "b.php"
		}
		result.Findings = append(result.Findings, report.Finding{Checker: "whitespace", Rule: "whitespace.trailing", Severity: "error", Message: "Trailing spaces or tabs", Path: &name, Line: new(line)})
	}
	result.Finalize()
	before, _ := json.Marshal(result)
	var output bytes.Buffer
	if err := Write(&output, result, Options{}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, want := range []string{"15 findings · 2 files", "5 more findings in 1 file", "--all", "15 errors"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing full totals or omission notice %q:\n%s", want, text)
		}
	}
	after, _ := json.Marshal(result)
	if strings.Contains(text, "    11") || len(result.Findings) != 15 || result.ExitCode != 1 || !bytes.Equal(before, after) {
		t.Fatalf("display limit affected report or did not hide extra locations:\n%s", text)
	}
	output.Reset()
	if err := Write(&output, result, Options{All: true}); err != nil || strings.Contains(output.String(), "more findings") || !strings.Contains(output.String(), "11 · 12 · 13 · 14 · 15") {
		t.Fatalf("--all must show the hidden positions: %s %v", &output, err)
	}
}

func TestCoverageAndExecutionFailuresStayInTheirCategory(t *testing.T) {
	result := scenarios()["findings"]
	result.Checks = append(result.Checks, check("eslint", "error", true, "dependencies_missing"))
	result.Checks = append(result.Checks, check("deptrac", "skipped", true, "no_config"))
	result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "dependencies_missing", Stage: "checker", Checker: new("eslint"), Message: "Install the locked project dependencies."})
	result.Finalize()
	var output bytes.Buffer
	if err := Write(&output, result, Options{}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	start, end := strings.Index(text, "\nCode standards\n"), strings.Index(text, "\nCorrectness\n")
	if !strings.Contains(text, "Analysis: incomplete") || !strings.Contains(text, "Architecture") || !strings.Contains(text, "no config") || start < 0 || end <= start || !strings.Contains(text[start:end], "Install the locked") || !strings.Contains(text, "Using Composer advisories") {
		t.Fatalf("category output lost coverage or execution details:\n%s", text)
	}
}

func TestRepeatedMessageWrappingPreservesWordsAndLinks(t *testing.T) {
	message := strings.Repeat("Деталь перевірки ", 20) + "https://example.test/rule"
	result := base("demo.php")
	result.Checks = []report.Check{check("demo", "failed", true, "")}
	result.Findings = []report.Finding{{Checker: "demo", Rule: "demo.rule", Kind: "code", Severity: "error", Message: message, Path: new("demo.php"), Line: new(12)}}
	result.Finalize()
	var output bytes.Buffer
	if err := Write(&output, result, Options{}); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, line := range strings.Split(text, "\n") {
		if utf8.RuneCountInString(line) > 100 {
			t.Fatalf("message was not wrapped: %q", line)
		}
	}
	if strings.Count(text, "Деталь") != 20 || !strings.Contains(text, "https://example.test/rule") {
		t.Fatalf("wrapping lost details: %s", text)
	}
}
