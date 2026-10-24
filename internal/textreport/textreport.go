package textreport

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/rchehd/phpwarden/internal/report"
)

type Options struct {
	Compact bool
	Color   bool
	All     bool
}

// Global findings name their file in a message prefix because the report schema keeps their path null.
var globalFile = regexp.MustCompile(`^(?:YAML file|Twig template|Composer file|Composer package|Config sync) ("(?:[^"\\]|\\.)*"): `)

type hint struct {
	text, known string
}

var hints = map[string]hint{
	"compatibility_invalid":  {"Check php.runtime and php.target in .phpwarden.yaml; development images support runtime 8.1–8.4 and targets from 8.1 up to the runtime.", ""},
	"config_invalid":         {"Fix the configuration; phpwarden config validate shows the details.", ""},
	"discovery_invalid":      {"Fix the project metadata named above; phpwarden config show lists what discovery found.", ""},
	"docker_unavailable":     {"Start the local Docker daemon or check DOCKER_HOST.", ""},
	"image_unavailable":      {"Build the development images: docker buildx bake", "docker buildx bake"},
	"stale":                  {"Run the check again once the files stop changing.", ""},
	"checker_timeout":        {"Raise the checker's timeout_seconds or runner.timeout_seconds.", ""},
	"advisories_unavailable": {"Connect to the network, or raise tools.composer-audit.max_stale_hours to accept an older cache.", ""},
	"dependencies_missing":   {"Rebuild the Docker image together with the CLI.", "rebuild"},
	"scope_invalid":          {"Check the base branch and paths; phpwarden scope shows what would be checked.", "--base"},
}

type writer struct {
	out     io.Writer
	options Options
	err     error
}

func (w *writer) line(format string, args ...any) {
	if w.err == nil {
		_, w.err = fmt.Fprintln(w.out, strings.TrimRight(fmt.Sprintf(format, args...), " "))
	}
}

func (w *writer) paint(code, text string) string {
	if !w.options.Color || text == "" || code == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (w *writer) severity(value string) string {
	label := map[string]string{"error": "error", "warning": "warn ", "info": "info "}[value]
	return w.paint(map[string]string{"error": "31", "warning": "33", "info": "36"}[value], label)
}

func Write(out io.Writer, result report.Report, options Options) error {
	w := &writer{out: out, options: options}
	if len(result.FixedFiles) > 0 {
		names := make([]string, len(result.FixedFiles))
		for i, name := range result.FixedFiles {
			names[i] = display(name)
		}
		w.line("Fixed %d file(s): %s", len(result.FixedFiles), strings.Join(names, ", "))
		w.line("")
	}
	if len(result.Checks) == 0 {
		w.line("%s", w.paint("1", "PHPWarden did not run any checks."))
		w.line("")
		w.problems(result.Diagnostics)
		w.line("")
		w.line("Result: %s · exit %d (%s)", w.paint("31;1", "NOT RUN"), result.ExitCode, meaning(result))
		return w.err
	}
	w.header(result)
	if !options.Compact {
		w.result(result)
		complete := "complete"
		if !result.Complete {
			complete = "incomplete"
		}
		w.line("Analysis: %s · %s", complete, plural(len(ruleGroups(result.Findings)), "rule group"))
		skipped := []string{}
		for _, check := range result.Checks {
			if check.Status == "skipped" {
				name := check.ID
				if check.Reason != nil {
					name += " (" + strings.ReplaceAll(*check.Reason, "_", " ") + ")"
				}
				skipped = append(skipped, name)
			}
		}
		if len(skipped) > 0 {
			w.wrapped("", "Skipped: "+strings.Join(skipped, "; "), "")
		}
	}
	if result.Environment != nil && (len(result.Environment.DiscoveryWarnings) > 0 || len(result.Environment.CompatibilityPending) > 0) {
		w.line("")
		w.line("%s", w.paint("1", "Project warnings"))
		for _, warning := range result.Environment.DiscoveryWarnings {
			w.line("  %s %s", w.paint("33", "!"), warning)
		}
		if pending := result.Environment.CompatibilityPending; len(pending) > 0 {
			w.line("  %s compatibility pending: %s", w.paint("33", "!"), strings.Join(pending, ", "))
		}
	}
	if options.Compact {
		w.compact(result)
		w.diagnostics(result)
	} else {
		w.diagnostics(result)
		w.grouped(result)
	}
	w.checks(result)
	if options.Compact {
		w.line("")
		w.result(result)
	}
	return w.err
}

func (w *writer) diagnostics(result report.Report) {
	status := map[string]string{}
	for _, check := range result.Checks {
		status[check.ID] = check.Status
	}
	problems, notes := []report.Diagnostic{}, []report.Diagnostic{}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Checker != nil && (status[*diagnostic.Checker] == "passed" || status[*diagnostic.Checker] == "failed") {
			notes = append(notes, diagnostic)
		} else {
			problems = append(problems, diagnostic)
		}
	}
	if len(problems) > 0 {
		w.line("")
		w.line("%s", w.paint("1", "Problems"))
		w.problems(problems, result.Checks...)
	}
	if len(notes) > 0 {
		w.line("")
		w.line("%s", w.paint("1", "Notes"))
		for _, note := range notes {
			w.line("  %s %s: %s", w.paint("33", "!"), *note.Checker, note.Message)
		}
	}
}

func (w *writer) result(result report.Report) {
	label := strings.ToUpper(result.Verdict)
	color := map[string]string{"pass": "32;1", "fail": "31;1", "error": "31;1"}[result.Verdict]
	if !result.Complete && result.ExitCode == 0 {
		label, color = "PARTIAL", "33;1"
	}
	total := counts(result.Findings)
	if total == "" {
		total = "no findings"
	}
	w.line("Result: %s · %s · exit %d (%s)", w.paint(color, label), total, result.ExitCode, meaning(result))
}

func against(base string) string {
	if count, err := strconv.Atoi(strings.TrimPrefix(base, "HEAD~")); strings.HasPrefix(base, "HEAD~") && err == nil && count > 0 {
		if count == 1 {
			return "in the last commit"
		}
		return "in the last " + plural(count, "commit")
	}
	return "against " + base
}

func (w *writer) header(result report.Report) {
	parts := []string{"PHPWarden"}
	if scope := result.Scope; scope != nil {
		switch {
		case scope.Mode == "full":
			parts = append(parts, "full scan")
		case scope.Mode == "paths":
			parts = append(parts, "selected paths")
		case scope.BaseRef != nil && scope.WholeFiles:
			parts = append(parts, "whole changed files "+against(*scope.BaseRef))
		case scope.BaseRef != nil:
			parts = append(parts, "changes "+against(*scope.BaseRef))
		}
		parts = append(parts, plural(len(scope.Files), "file"))
	}
	if environment := result.Environment; environment != nil {
		if environment.Cache != nil && environment.Cache.Status != "bypass" {
			parts = append(parts, "cache "+environment.Cache.Status)
		}
		if environment.FrameworkVersion != nil && *environment.FrameworkVersion != "auto" {
			parts = append(parts, "Drupal "+*environment.FrameworkVersion)
		}
		if environment.PHPVersion != nil {
			parts = append(parts, "PHP "+*environment.PHPVersion)
		} else if environment.RequestedRuntime != "" {
			parts = append(parts, "PHP "+environment.RequestedRuntime)
		}
	}
	w.line("%s", w.paint("1", strings.Join(parts, " · ")))
}

type located struct {
	file         string
	line, column int
	message      string
	finding      report.Finding
}

func locate(finding report.Finding) located {
	row := located{file: "(project)", message: finding.Message, finding: finding}
	if finding.Path != nil {
		row.file = display(*finding.Path)
	} else if match := globalFile.FindStringSubmatchIndex(finding.Message); match != nil {
		if name, err := strconv.Unquote(finding.Message[match[2]:match[3]]); err == nil {
			row.file, row.message = display(name), finding.Message[match[1]:]
		}
	}
	if finding.Line != nil {
		row.line = *finding.Line
	}
	if finding.Column != nil {
		row.column = *finding.Column
	}
	return row
}

func sortRows(rows []located) {
	slices.SortStableFunc(rows, func(a, b located) int {
		if a.file != b.file {
			return strings.Compare(a.file, b.file)
		}
		if a.line != b.line {
			return a.line - b.line
		}
		return a.column - b.column
	})
}

func (w *writer) compact(result report.Report) {
	rows := make([]located, len(result.Findings))
	for i, finding := range result.Findings {
		rows[i] = locate(finding)
	}
	sortRows(rows)
	if len(rows) > 0 {
		w.line("")
	}
	for _, row := range rows {
		location := row.file
		if row.line > 0 {
			location += ":" + strconv.Itoa(row.line) + ":" + strconv.Itoa(max(row.column, 1))
		}
		message := strings.Join(strings.Fields(row.message), " ")
		w.line("%s  %s  %s  %s", location, w.severity(row.finding.Severity), message, w.paint("2", "["+row.finding.Checker+": "+row.finding.Rule+"]"))
	}
}

func (w *writer) problems(diagnostics []report.Diagnostic, checks ...report.Check) {
	shown := map[int]bool{}
	for _, check := range checks {
		if check.Status != "error" && check.Status != "unavailable" {
			continue
		}
		policy := "optional"
		if check.Required {
			policy = "required"
		}
		w.line("  %s %s  %s (%s)", w.paint("31", "✗"), check.ID, check.Status, policy)
		for i, diagnostic := range diagnostics {
			if diagnostic.Checker != nil && *diagnostic.Checker == check.ID {
				shown[i] = true
				w.detail(diagnostic)
			}
		}
	}
	for i, diagnostic := range diagnostics {
		if shown[i] {
			continue
		}
		name := diagnostic.Stage
		if diagnostic.Checker != nil {
			name = *diagnostic.Checker
		}
		lines := strings.Split(diagnostic.Message, "\n")
		w.line("  %s %s: %s", w.paint("31", "✗"), name, lines[0])
		for _, line := range lines[1:] {
			w.line("    %s", line)
		}
		w.hint(diagnostic, "    ")
	}
}

func (w *writer) detail(diagnostic report.Diagnostic) {
	for _, line := range strings.Split(diagnostic.Message, "\n") {
		w.line("      %s", line)
	}
	w.hint(diagnostic, "      ")
}

func (w *writer) hint(diagnostic report.Diagnostic, indent string) {
	if advice, ok := hints[diagnostic.Code]; ok && (advice.known == "" || !strings.Contains(diagnostic.Message, advice.known)) {
		w.line("%s%s %s", indent, w.paint("1", "Fix:"), advice.text)
	}
}

func (w *writer) checks(result report.Report) {
	byChecker := map[string][]report.Finding{}
	for _, finding := range result.Findings {
		byChecker[finding.Checker] = append(byChecker[finding.Checker], finding)
	}
	rank := map[string]int{"failed": 0, "error": 1, "unavailable": 1, "passed": 2, "skipped": 3}
	checks := slices.Clone(result.Checks)
	slices.SortStableFunc(checks, func(a, b report.Check) int {
		if rank[a.Status] != rank[b.Status] {
			return rank[a.Status] - rank[b.Status]
		}
		return strings.Compare(a.ID, b.ID)
	})
	type row struct{ names, status, note string }
	rows := []row{}
	for _, check := range checks {
		note := counts(byChecker[check.ID])
		if note == "" && check.Reason != nil {
			note = strings.ReplaceAll(*check.Reason, "_", " ")
		}
		last := len(rows) - 1
		if (check.Status == "passed" || check.Status == "skipped") && last >= 0 && rows[last].status == check.Status && rows[last].note == note {
			rows[last].names += ", " + check.ID
			continue
		}
		rows = append(rows, row{check.ID, check.Status, note})
	}
	width := 0
	for _, row := range rows {
		width = max(width, len(row.names))
	}
	w.line("")
	w.line("%s", w.paint("1", "Checks"))
	for _, row := range rows {
		mark := map[string]string{"failed": w.paint("31", "✗"), "error": w.paint("31", "✗"), "unavailable": w.paint("31", "✗"), "passed": w.paint("32", "✓"), "skipped": w.paint("2", "-")}[row.status]
		name := row.names
		if len(name) < width {
			name += strings.Repeat(" ", width-len(name))
		}
		w.line("  %s %s  %-11s  %s", mark, name, row.status, row.note)
	}
}

func position(row located) string {
	if row.line == 0 {
		return ""
	}
	if row.column == 0 {
		return strconv.Itoa(row.line)
	}
	return strconv.Itoa(row.line) + ":" + strconv.Itoa(row.column)
}

func counts(findings []report.Finding) string {
	total := map[string]int{}
	for _, finding := range findings {
		total[finding.Severity]++
	}
	parts := []string{}
	for _, severity := range []string{"error", "warning", "info"} {
		if total[severity] > 0 {
			parts = append(parts, plural(total[severity], severity))
		}
	}
	return strings.Join(parts, ", ")
}

func plural(count int, word string) string {
	if count == 1 {
		return "1 " + word
	}
	return strconv.Itoa(count) + " " + word + "s"
}

func meaning(result report.Report) string {
	switch result.ExitCode {
	case 0:
		if !result.Complete {
			return "an optional check could not run"
		}
		return "no blocking findings"
	case 1:
		return "blocking findings"
	case 2:
		return "configuration, arguments, or project metadata"
	case 130:
		return "interrupted"
	}
	for _, check := range result.Checks {
		if check.Required && (check.Status == "error" || check.Status == "unavailable") {
			return "a required check could not run"
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "stale" {
			return "the project changed during the check"
		}
	}
	return "the check could not complete"
}

// display keeps ordinary paths readable and quotes only those a terminal would garble.
func display(name string) string {
	if name == "" || strings.TrimSpace(name) != name || strings.ContainsFunc(name, func(r rune) bool { return unicode.IsControl(r) || r == '"' }) {
		return strconv.Quote(name)
	}
	return name
}
