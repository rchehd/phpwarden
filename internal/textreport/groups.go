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
}

func (w *writer) grouped(result report.Report) {
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
		title := group.rule
		if value, ok := ruleTitles[group.rule]; ok {
			title = value
		}
		if common != "" && !strings.Contains(common, "\n") && utf8.RuneCountInString(common) <= 70 {
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
		w.wrapped("", strings.ToUpper(group.severity)+"  "+title+" — "+totals, group.severity)
		w.wrapped("  ", group.checker+" / "+group.rule, "")
		if environment := result.Environment; group.checker == "composer-audit" && environment != nil && environment.Advisories != nil {
			advisories := environment.Advisories
			w.wrapped("  ", fmt.Sprintf("advisories from %s, %s", advisories.Source, strings.Replace(strings.Replace(advisories.FetchedAt, "T", " ", 1), "Z", " UTC", 1)), "")
		}
		if common != "" && common != title {
			w.wrapped("  ", common, "")
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
			w.line("  %s", rows[start].file)
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
					w.wrapped("    ", strings.Join(positions, " · "), "")
				}
				if common == "" {
					w.wrapped("    ", message, "")
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
			w.wrapped("  ", note+"; use --all to show all", "")
		}
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
