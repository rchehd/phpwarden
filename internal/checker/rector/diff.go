package rector

import (
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var hunkHeader = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(?:.*)$`)

func changedLines(diff string, original []byte) ([]int, error) {
	failure := func() ([]int, error) { return nil, fmt.Errorf("invalid Rector diff") }
	rows := strings.Split(strings.ReplaceAll(diff, "\r\n", "\n"), "\n")
	source := strings.Split(strings.ReplaceAll(string(original), "\r\n", "\n"), "\n")
	if len(rows) < 3 || !strings.HasPrefix(rows[0], "--- ") || !strings.HasPrefix(rows[1], "+++ ") {
		return failure()
	}
	result := []int{}
	oldLine, oldRemaining, newRemaining, previousEnd := 0, 0, 0, 0
	inHunk, changed, removed := false, false, false
	for index, row := range rows[2:] {
		if match := hunkHeader.FindStringSubmatch(row); match != nil {
			if inHunk && (oldRemaining != 0 || newRemaining != 0) {
				return failure()
			}
			oldLine, _ = strconv.Atoi(match[1])
			oldRemaining, newRemaining = 1, 1
			if match[2] != "" {
				oldRemaining, _ = strconv.Atoi(match[2])
			}
			if match[4] != "" {
				newRemaining, _ = strconv.Atoi(match[4])
			}
			if oldLine < previousEnd || oldLine < 0 || oldLine > len(source) {
				return failure()
			}
			previousEnd, inHunk, removed = oldLine+oldRemaining, true, false
			continue
		}
		if row == "" && index == len(rows)-3 {
			continue
		}
		if row == "\\ No newline at end of file" {
			continue
		}
		if !inHunk || len(row) == 0 {
			return failure()
		}
		switch row[0] {
		case ' ', '-':
			if oldRemaining <= 0 || oldLine < 1 || oldLine > len(source) || row[1:] != source[oldLine-1] {
				return failure()
			}
			if row[0] == '-' {
				result = append(result, oldLine)
				changed = true
				removed = true
			} else {
				newRemaining--
				removed = false
			}
			oldLine++
			oldRemaining--
		case '+':
			newRemaining--
			if !removed {
				result = append(result, max(1, min(oldLine, len(source))))
			}
			changed = true
		default:
			return failure()
		}
		if oldRemaining < 0 || newRemaining < 0 {
			return failure()
		}
	}
	if !inHunk || !changed || oldRemaining != 0 || newRemaining != 0 {
		return failure()
	}
	slices.Sort(result)
	return slices.Compact(result), nil
}
