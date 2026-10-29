package textreport

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/rchehd/phpwarden/internal/report"
)

type ruleKey struct {
	severity, checker, rule string
}

type ruleGroup struct {
	ruleKey
	rows []located
}

func ruleGroups(findings []report.Finding) []ruleGroup {
	indexes := map[ruleKey]int{}
	groups := []ruleGroup{}
	for _, finding := range findings {
		key := ruleKey{finding.Severity, finding.Checker, finding.Rule}
		index, exists := indexes[key]
		if !exists {
			index = len(groups)
			indexes[key] = index
			groups = append(groups, ruleGroup{ruleKey: key})
		}
		row := locate(finding)
		if finding.Checker == "phpcs" && finding.Rule == "SlevomatCodingStandard.Whitespaces.DuplicateSpaces.DuplicateSpaces" && row.column > 0 && row.message == fmt.Sprintf("Duplicate spaces at position %d.", row.column) {
			row.message = "Duplicate spaces"
		}
		groups[index].rows = append(groups[index].rows, row)
	}
	rank := map[string]int{"error": 0, "warning": 1, "info": 2}
	slices.SortFunc(groups, func(a, b ruleGroup) int {
		return cmp.Or(cmp.Compare(rank[a.severity], rank[b.severity]), strings.Compare(a.checker, b.checker), strings.Compare(a.rule, b.rule))
	})
	return groups
}

var ruleTitles = map[string]string{
	"debug.function":              "Debug calls",
	"debug.js_call":               "Debug calls left in JavaScript",
	"drupal.config.forbidden_key": "Forbidden configuration keys",
	"classExtendsInternalClass.classExtendsInternalClass":    "Extending internal classes",
	"globalDrupalDependencyInjection.useDependencyInjection": "Use dependency injection",
	"method.childReturnType":                                 "Incompatible method return types",
	"jscpd.duplicate":                                        "Duplicate code",
	"method.notFound":                                        "Undefined methods",
	"class.notFound":                                         "Unknown classes",
	"property.notFound":                                      "Undefined properties",
	"rector.upgrade":                                         "Upgrade suggestions",
	"UnusedLocalVariable":                                    "Unused local variables",
	"psalm.TaintedHtml":                                      "Untrusted HTML reaches output",
	"psalm.TaintedTextWithQuotes":                            "Untrusted text reaches HTML attributes",
	"psalm.TaintedSql":                                       "Untrusted input reaches a database query",
}

func (w *writer) grouped(result report.Report, indent string) {
	for _, group := range ruleGroups(result.Findings) {
		rows := group.rows
		sortRows(rows)
		common := rows[0].message
		for _, row := range rows[1:] {
			if row.message != common {
				common = ""
				break
			}
		}
		title := map[string]string{
			"Security": "Security issue", "Code standards": "Code standard violation", "Correctness": "Code correctness issue",
			"Dependencies": "Package configuration", "Maintainability": "Maintainability issue", "Drupal quality": "Drupal quality issue",
			"Architecture": "Dependency rule violation", "Upgrades": "Upgrade suggestion",
		}[categoryName(group.checker)]
		if title == "" {
			title = "Code issue"
		}
		if value, ok := ruleTitles[group.rule]; ok {
			title = value
		}
		if !strings.HasPrefix(group.rule, "psalm.") && common != "" && !strings.Contains(common, "\n") && utf8.RuneCountInString(common) <= 70 {
			title = common
		}
		files := map[string]bool{}
		for _, row := range rows {
			if row.file != "(project)" {
				files[row.file] = true
			}
		}
		w.line("")
		totals := plural(len(rows), "finding")
		if len(files) > 0 {
			totals += " · " + plural(len(files), "file")
		}
		w.wrapped(indent, strings.ToUpper(group.severity)+"  "+title+" — "+totals, group.severity)
		if environment := result.Environment; group.checker == "composer-audit" && environment != nil && environment.Advisories != nil {
			advisories := environment.Advisories
			w.wrapped(indent+"  ", fmt.Sprintf("advisories from %s, %s", advisories.Source, strings.Replace(strings.Replace(advisories.FetchedAt, "T", " ", 1), "Z", " UTC", 1)), "")
		}
		if common != "" && common != title {
			w.wrapped(indent+"  ", common, "")
		}
		limit := len(rows)
		if !w.options.All {
			limit = min(limit, 10)
		}
		for start := 0; start < limit; {
			end := start + 1
			for end < limit && rows[end].file == rows[start].file {
				end++
			}
			w.line("")
			w.line("%s%s", indent+"  ", rows[start].file)
			if slices.ContainsFunc(rows[start:end], func(row located) bool { return len(row.finding.Trace) > 0 }) {
				for _, row := range rows[start:end] {
					w.wrapped(indent+"    ", position(row), "")
					if common == "" {
						w.wrapped(indent+"    ", row.message, "")
					}
					w.trace(row.finding, indent+"      ")
				}
				start = end
				continue
			}
			messages := map[string][]string{}
			order := []string{}
			for _, row := range rows[start:end] {
				if _, exists := messages[row.message]; !exists {
					order = append(order, row.message)
				}
				messages[row.message] = append(messages[row.message], position(row))
			}
			for _, message := range order {
				positions := slices.DeleteFunc(messages[message], func(value string) bool { return value == "" })
				if len(positions) > 0 {
					w.wrapped(indent+"    ", strings.Join(positions, " · "), "")
				}
				if common == "" {
					w.wrapped(indent+"    ", message, "")
				}
			}
			start = end
		}
		if limit < len(rows) {
			hiddenFiles := map[string]bool{}
			for _, row := range rows[limit:] {
				if row.file != "(project)" {
					hiddenFiles[row.file] = true
				}
			}
			note := "… " + plural(len(rows)-limit, "more finding")
			if len(hiddenFiles) > 0 {
				note += " in " + plural(len(hiddenFiles), "file")
			}
			w.wrapped(indent+"  ", note+"; use --all to show all", "")
		}
	}
}

func (w *writer) trace(finding report.Finding, indent string) {
	steps := finding.Trace
	for index, step := range steps {
		if !w.options.All && len(steps) > 6 && index >= 3 && index < len(steps)-2 {
			if index == 3 {
				w.wrapped(indent, "… "+plural(len(steps)-5, "flow step")+" omitted; use --all to show all", "")
			}
			continue
		}
		label := display(step.Label)
		if !w.options.All && strings.Contains(label, `\`) {
			prefix := ""
			if strings.HasPrefix(label, "call to ") {
				prefix = "call to "
			}
			label = prefix + label[strings.LastIndex(label, `\`)+1:]
		}
		if step.Path != nil && (step.Origin == "project" || w.options.All) {
			prefix := ""
			if step.Origin != "project" {
				prefix = step.Origin + ":"
			}
			location := prefix + display(*step.Path)
			if !w.options.All && finding.Path != nil && *step.Path == *finding.Path {
				location = "line"
			}
			if step.Line != nil {
				location += fmt.Sprintf(":%d", *step.Line)
			}
			if step.Column != nil {
				location += fmt.Sprintf(":%d", *step.Column)
			}
			label += " · " + location
		}
		w.wrapped(indent, "→ "+label, "")
	}
}

func (w *writer) wrapped(indent, text, severity string) {
	color := map[string]string{"error": "31", "warning": "33", "info": "36"}[severity]
	for _, paragraph := range strings.Split(text, "\n") {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			if line != "" && utf8.RuneCountInString(indent+line+" "+word) > 100 {
				w.line("%s%s", indent, w.paint(color, line))
				line = ""
			}
			if line != "" {
				line += " "
			}
			line += word
		}
		w.line("%s%s", indent, w.paint(color, line))
	}
}
