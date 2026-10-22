package scope

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

type Range struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

var hunkHeader = regexp.MustCompile(`^@@ -[0-9]+(?:,[0-9]+)? \+([0-9]+)(?:,([0-9]+))? @@`)

func (repo repository) changedLines(base, filename string) ([]Range, error) {
	output, err := repo.git("diff", "--unified=0", "--inter-hunk-context=0", "--no-renames", "--no-ext-diff", "--no-textconv", "--no-color", "--ignore-submodules=all", base, "--", ":(top,literal)"+repo.prefix+filename)
	if err != nil {
		return nil, err
	}
	return parseLines(output)
}

func parseLines(patch []byte) ([]Range, error) {
	ranges := []Range{}
	for _, line := range bytes.Split(patch, []byte{'\n'}) {
		if bytes.HasPrefix(line, []byte("Binary files ")) || bytes.Equal(line, []byte("GIT binary patch")) {
			return nil, nil
		}
		if !bytes.HasPrefix(line, []byte("@@")) {
			continue
		}
		match := hunkHeader.FindSubmatch(line)
		if match == nil {
			return nil, fmt.Errorf("unsupported Git hunk header %q", line)
		}
		start, err := strconv.Atoi(string(match[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid Git line number: %w", err)
		}
		count := 1
		if len(match[2]) > 0 {
			count, err = strconv.Atoi(string(match[2]))
			if err != nil {
				return nil, fmt.Errorf("invalid Git line count: %w", err)
			}
		}
		if count > 0 {
			end := start + count - 1
			if start < 1 || end < start {
				return nil, fmt.Errorf("invalid Git line range")
			}
			ranges = append(ranges, Range{Start: start, End: end})
		}
	}
	return ranges, nil
}
