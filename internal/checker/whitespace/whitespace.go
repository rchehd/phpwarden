package whitespace

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/scope"
)

func Run(ctx context.Context, root string, selection scope.Selection, cfg config.Whitespace) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "whitespace", Status: "passed", Required: cfg.Required, FailOn: cfg.FailOn},
		Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	trailing := selection.Filter(nil, cfg.TrailingWhitespaceExclude, func(string) bool { return cfg.TrailingWhitespace })
	conflicts := selection.Filter(nil, cfg.ConflictMarkersExclude, func(string) bool { return cfg.ConflictMarkers })
	if !cfg.Enabled || len(trailing.Files)+len(conflicts.Files) == 0 {
		reason := "no_files"
		if !cfg.Enabled {
			reason = "disabled"
		} else if !cfg.TrailingWhitespace && !cfg.ConflictMarkers {
			reason = "rules_disabled"
		}
		result.Check.Status, result.Check.Reason = "skipped", &reason
		return result
	}
	fail := func(code, message string) checker.Outcome {
		result.Check.Status, result.Check.Reason = "error", &code
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: code, Stage: "checker", Message: message, Checker: new("whitespace")})
		return result
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()
	for _, filename := range selection.Files {
		_, checkTrailing := trailing.LineMap[filename]
		_, checkConflicts := conflicts.LineMap[filename]
		if !checkTrailing && !checkConflicts {
			continue
		}
		if ctx.Err() != nil {
			return fail("checker_timeout", "Whitespace checker did not complete within its execution context")
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(filename)))
		if err != nil {
			return fail("input_read", fmt.Sprintf("Cannot read %q: %v", filename, err))
		}
		if bytes.IndexByte(data, 0) >= 0 {
			continue
		}
		for line := 1; len(data) > 0; line++ {
			if ctx.Err() != nil {
				return fail("checker_timeout", "Whitespace checker did not complete within its execution context")
			}
			content, rest, newline := bytes.Cut(data, []byte{'\n'})
			data = rest
			if newline && len(content) > 0 && content[len(content)-1] == '\r' {
				content = content[:len(content)-1]
			}
			if !selection.Contains(filename, line) {
				continue
			}
			add := func(rule, message string, column int) {
				result.Findings = append(result.Findings, report.Finding{Checker: "whitespace", Rule: rule, Kind: "code", Severity: "error",
					Message: message, Path: new(filename), Line: new(line), Column: &column})
			}
			if checkTrailing {
				end := len(bytes.TrimRight(content, " \t"))
				if end < len(content) {
					add("whitespace.trailing", "Trailing spaces or tabs", end+1)
				}
			}
			if checkConflicts && conflictMarker(content) {
				add("conflict.marker", "Unresolved merge conflict marker", 1)
			}
		}
	}
	if ctx.Err() != nil {
		return fail("checker_timeout", "Whitespace checker did not complete within its execution context")
	}
	if len(result.Findings) > 0 && cfg.FailOn != "never" {
		result.Check.Status = "failed"
	}
	return result
}

func conflictMarker(line []byte) bool {
	if len(line) < 7 || !bytes.ContainsAny(line[:1], "<|=>") {
		return false
	}
	end := 1
	for end < len(line) && line[end] == line[0] {
		end++
	}
	return end >= 7 && (end == len(line) || line[end] == ' ' || line[end] == '\t')
}
