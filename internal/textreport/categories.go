package textreport

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/report"
)

var categoryOrder = []string{"Security", "Code standards", "Correctness", "Dependencies", "Maintainability", "Drupal quality", "Architecture", "Upgrades", "Environment", "Other checks"}

var checkerCategories = map[string]string{
	"composer-audit": "Security", "gitleaks": "Security", "semgrep": "Security",
	"phpcs": "Code standards", "eslint": "Code standards", "stylelint": "Code standards", "twig-cs-fixer": "Code standards", "whitespace": "Code standards",
	"phpstan": "Correctness", "syntax": "Correctness", "yaml": "Correctness", "debug": "Correctness",
	"composer-validate": "Dependencies", "composer-require-checker": "Dependencies", "composer-unused": "Dependencies",
	"phpmd": "Maintainability", "dead-code": "Maintainability", "jscpd": "Maintainability",
	"eventhorizon": "Drupal quality", "drupal-config": "Drupal quality",
	"deptrac": "Architecture", "rector": "Upgrades",
}

func categoryName(id string) string {
	if id == "" {
		return "Environment"
	}
	if name, ok := checkerCategories[id]; ok {
		return name
	}
	return "Other checks"
}

type categoryIssue struct {
	severity, title string
	diagnostic      report.Diagnostic
}

type category struct {
	name   string
	result report.Report
	issues []categoryIssue
}

func reportCategories(result report.Report) []category {
	groups := []category{}
	indexes := map[string]int{}
	get := func(id string) *category {
		name := categoryName(id)
		index, exists := indexes[name]
		if !exists {
			index = len(groups)
			indexes[name] = index
			groups = append(groups, category{name: name, result: report.Report{Environment: result.Environment}})
		}
		return &groups[index]
	}
	checks := map[string]report.Check{}
	for _, check := range result.Checks {
		checks[check.ID] = check
		group := get(check.ID)
		group.result.Checks = append(group.result.Checks, check)
	}
	for _, finding := range result.Findings {
		group := get(finding.Checker)
		group.result.Findings = append(group.result.Findings, finding)
	}
	shown := map[string]bool{}
	for _, diagnostic := range result.Diagnostics {
		id := ""
		if diagnostic.Checker != nil {
			id = *diagnostic.Checker
			shown[id] = true
		}
		group := get(id)
		group.issues = append(group.issues, diagnosticIssue(diagnostic, checks[id]))
	}
	for _, check := range result.Checks {
		if shown[check.ID] || check.Status != "error" && check.Status != "unavailable" {
			continue
		}
		diagnostic := report.Diagnostic{Checker: new(check.ID), Message: "The check could not complete."}
		if check.Reason != nil {
			diagnostic.Code = *check.Reason
		}
		group := get(check.ID)
		group.issues = append(group.issues, diagnosticIssue(diagnostic, check))
	}
	if environment := result.Environment; environment != nil {
		warnings := slices.Clone(environment.DiscoveryWarnings)
		if len(environment.CompatibilityPending) > 0 {
			warnings = append(warnings, "Compatibility pending: "+strings.Join(environment.CompatibilityPending, ", "))
		}
		for _, warning := range warnings {
			group := get("")
			group.issues = append(group.issues, categoryIssue{severity: "warning", title: "Project metadata", diagnostic: report.Diagnostic{Message: warning}})
		}
	}
	ranks := map[string]int{"error": 0, "failed": 1, "incomplete": 2, "partial": 3, "warning": 4, "passed": 5, "skipped": 6}
	slices.SortStableFunc(groups, func(a, b category) int {
		aStatus, _ := a.summary()
		bStatus, _ := b.summary()
		return cmp.Or(cmp.Compare(ranks[aStatus], ranks[bStatus]), cmp.Compare(slices.Index(categoryOrder, a.name), slices.Index(categoryOrder, b.name)))
	})
	return groups
}

func diagnosticIssue(diagnostic report.Diagnostic, check report.Check) categoryIssue {
	issue := categoryIssue{severity: "error", title: "Analysis could not complete", diagnostic: diagnostic}
	if diagnostic.Checker != nil && check.Status != "error" && check.Status != "unavailable" {
		issue.severity, issue.title = "warning", "Analysis note"
	}
	if title, ok := map[string]string{
		"no_config": "Configuration missing", "dependencies_missing": "Dependencies unavailable",
		"checker_timeout": "Analysis timed out", "advisories_unavailable": "Security advisories unavailable",
		"config_invalid": "Invalid configuration", "stale": "Project changed during analysis",
	}[diagnostic.Code]; ok {
		issue.title = title
	}
	if diagnostic.Code == "dependencies_missing" && diagnostic.Checker != nil {
		switch *diagnostic.Checker {
		case "eslint":
			issue.title = "JavaScript dependencies unavailable"
		case "stylelint":
			issue.title = "Stylesheet dependencies unavailable"
		}
	}
	if check.Status == "error" || check.Status == "unavailable" {
		if check.Required {
			issue.title += " (required)"
		} else {
			issue.title += " (optional)"
		}
	}
	return issue
}

func (group category) summary() (string, string) {
	active, failures := 0, 0
	requiredFailure, blocked, gap := false, false, false
	skipped := map[string]int{}
	for _, check := range group.result.Checks {
		if check.Status == "skipped" {
			reason := "not applicable"
			if check.Reason != nil {
				reason = strings.ReplaceAll(*check.Reason, "_", " ")
			}
			skipped[reason]++
			gap = gap || reason != "disabled" && reason != "no files" && reason != "not selected" && reason != "not applicable"
			continue
		}
		active++
		if check.Status == "error" || check.Status == "unavailable" {
			failures++
			requiredFailure = requiredFailure || check.Required
		}
		blocked = blocked || check.Status == "failed"
		for _, finding := range group.result.Findings {
			if finding.Checker == check.ID && check.FailOn != "never" && (finding.Severity == "error" || check.FailOn == "warning" && finding.Severity == "warning") {
				blocked = true
			}
		}
	}
	status := "passed"
	switch {
	case requiredFailure:
		status = "error"
	case blocked:
		status = "failed"
	case failures > 0:
		status = "incomplete"
	case active == 0 && len(group.issues) == 0 && len(group.result.Findings) == 0:
		status = "skipped"
	case gap:
		status = "partial"
	case len(group.issues) > 0 || hasWarnings(group.result.Findings):
		status = "warning"
	}
	for _, issue := range group.issues {
		if issue.severity == "error" && issue.diagnostic.Checker == nil {
			status = "error"
		}
	}
	parts := []string{}
	if total := counts(group.result.Findings); total != "" {
		parts = append(parts, total)
	}
	if failures > 0 {
		parts = append(parts, plural(failures, "check")+" could not run")
	}
	reasons := []string{}
	for reason, count := range skipped {
		if active > 0 {
			reason = fmt.Sprintf("%d skipped: %s", count, reason)
		}
		reasons = append(reasons, reason)
	}
	slices.Sort(reasons)
	parts = append(parts, reasons...)
	return status, strings.Join(parts, " · ")
}

func hasWarnings(findings []report.Finding) bool {
	return slices.ContainsFunc(findings, func(finding report.Finding) bool { return finding.Severity == "warning" || finding.Severity == "error" })
}

func (w *writer) categoryDetails(groups []category) {
	for _, group := range groups {
		if len(group.issues) == 0 && len(group.result.Findings) == 0 {
			continue
		}
		w.line("")
		w.line("%s", w.paint("1", group.name))
		for _, severity := range []string{"error", "warning", "info"} {
			for _, issue := range group.issues {
				if issue.severity != severity {
					continue
				}
				w.line("")
				w.wrapped("  ", strings.ToUpper(severity)+" "+issue.title, severity)
				w.wrapped("    ", issue.diagnostic.Message, "")
				w.hint(issue.diagnostic, "    ")
			}
			filtered := report.Report{Environment: group.result.Environment}
			for _, finding := range group.result.Findings {
				if finding.Severity == severity {
					filtered.Findings = append(filtered.Findings, finding)
				}
			}
			w.grouped(filtered, "  ")
		}
	}
}

func (w *writer) categoryChecks(groups []category) {
	w.line("")
	w.line("%s", w.paint("1", "Checks"))
	width := 0
	for _, group := range groups {
		width = max(width, len(group.name))
	}
	for _, group := range groups {
		status, note := group.summary()
		mark, color := "✓", "32"
		switch status {
		case "error", "failed":
			mark, color = "✗", "31"
		case "incomplete", "partial", "warning":
			mark, color = "!", "33"
		case "skipped":
			mark, color = "-", "2"
		}
		w.line("  %s %-*s  %-11s  %s", w.paint(color, mark), width, group.name, status, note)
	}
}
