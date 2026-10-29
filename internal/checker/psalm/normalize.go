package psalm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/checker"
	"github.com/rchehd/phpwarden/internal/phpconstraint"
	"github.com/rchehd/phpwarden/internal/projectpath"
	"github.com/rchehd/phpwarden/internal/psalmanalysis"
	"github.com/rchehd/phpwarden/internal/report"
	"github.com/rchehd/phpwarden/internal/scope"
)

type Policy struct {
	Required bool
	FailOn   string
}

type location struct {
	File      string `json:"file_path"`
	Line      int    `json:"line_from"`
	EndLine   int    `json:"line_to"`
	Column    int    `json:"column_from"`
	EndColumn int    `json:"column_to"`
	Snippet   string `json:"snippet"`
}

type node struct {
	location
	Label string `json:"label"`
}

type issue struct {
	location
	Type       string          `json:"type"`
	Severity   string          `json:"severity"`
	Message    string          `json:"message"`
	Trace      []node          `json:"taint_trace"`
	References json.RawMessage `json:"other_references"`
}

func Normalize(native psalmanalysis.Result, selection scope.Selection, policy Policy) checker.Outcome {
	result := checker.Outcome{Check: report.Check{ID: "psalm", Status: "passed", Required: policy.Required, FailOn: policy.FailOn}, Findings: []report.Finding{}, Diagnostics: []report.Diagnostic{}}
	fail := func(message string) checker.Outcome {
		result.Check.Status, result.Check.Reason = "error", new("invalid_report")
		result.Findings = []report.Finding{}
		result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: "invalid_report", Stage: "normalization", Checker: new("psalm"), Message: message})
		return result
	}
	if !slices.Contains([]string{"never", "warning", "error"}, policy.FailOn) {
		return fail("Invalid Psalm finding policy")
	}
	if native.PHP != "" {
		version, err := phpconstraint.StableVersion(native.PHP)
		if err != nil || version != native.PHP {
			return fail("Invalid Psalm runtime identity")
		}
		result.PHPVersion, result.Runtime = new(version), new(version[:strings.LastIndex(version, ".")])
	}
	if !native.Complete {
		if len(native.Diagnostics) == 0 {
			return fail("Incomplete Psalm analysis has no execution diagnostic")
		}
		result.Check.Status, result.Check.Reason = "error", new(native.Diagnostics[0].Code)
		if native.Diagnostics[0].Stage == "dependencies" || native.Diagnostics[0].Stage == "runtime" {
			result.Check.Status = "unavailable"
		}
		for _, diagnostic := range native.Diagnostics {
			if diagnostic.Code == "" || diagnostic.Stage == "" || diagnostic.Message == "" {
				return fail("Invalid Psalm execution diagnostic")
			}
			result.Diagnostics = append(result.Diagnostics, report.Diagnostic{Code: diagnostic.Code, Stage: diagnostic.Stage, Message: diagnostic.Message, Checker: new("psalm")})
		}
		return result
	}
	if len(native.Diagnostics) > 0 || native.Findings == nil || result.PHPVersion == nil || len(native.SourceFiles) == 0 || !validRoots(native.Roots) {
		return fail("Complete Psalm analysis lacks verified inputs or contains execution diagnostics")
	}
	for _, name := range append(slices.Clone(native.SourceFiles), native.ContextFiles...) {
		if name == "." || projectpath.Validate(name) != nil || filepath.ToSlash(filepath.Clean(name)) != name {
			return fail("Invalid Psalm input inventory")
		}
	}
	for _, raw := range native.Findings {
		var row issue
		if json.Unmarshal(raw, &row) != nil || !strings.HasPrefix(row.Type, "Tainted") || len(row.Type) <= len("Tainted") || row.Message == "" || !slices.Contains([]string{"error", "warning", "info"}, row.Severity) || len(row.Trace) == 0 || len(row.References) > 0 && string(row.References) != "null" && string(row.References) != "[]" {
			return fail("Invalid or unsupported Psalm taint finding")
		}
		primary, err := normalizeLocation(row.location, native.Roots)
		if err != nil || primary.Origin == "symbol" || primary.Origin == "tool" {
			return fail("Invalid Psalm primary location")
		}
		finding := report.Finding{Checker: "psalm", Rule: "psalm." + row.Type, Kind: "code", Severity: row.Severity, Message: row.Message}
		var anchor *report.TraceStep
		for _, step := range row.Trace {
			point, err := normalizeLocation(step.location, native.Roots)
			if err != nil || strings.TrimSpace(step.Label) == "" {
				return fail("Invalid Psalm taint trace")
			}
			point.Label = step.Label
			finding.Trace = append(finding.Trace, point)
			if point.Origin == "project" && slices.Contains(native.SourceFiles, *point.Path) {
				anchor = &point
			}
		}
		if primary.Origin == "project" && slices.Contains(native.SourceFiles, *primary.Path) {
			anchor = &primary
		}
		if anchor == nil {
			return fail("Psalm taint flow has no analyzed project source location")
		}
		finding.Path, finding.Line, finding.Column = anchor.Path, anchor.Line, anchor.Column
		finding.Fingerprint = new(fingerprint(finding))
		if Relevant(finding, selection, native.ContextFiles) {
			result.Findings = append(result.Findings, finding)
		}
	}
	for _, finding := range result.Findings {
		if policy.FailOn != "never" && (finding.Severity == "error" || policy.FailOn == "warning" && finding.Severity == "warning") {
			result.Check.Status = "failed"
		}
	}
	return result
}

// Native traces describe signatures and call sites, so selection covers whole files along the flow.
func Relevant(finding report.Finding, selection scope.Selection, contextFiles []string) bool {
	for _, name := range append(slices.Clone(selection.Files), selection.ProjectFiles...) {
		if slices.Contains(contextFiles, name) {
			return true
		}
	}
	if finding.Path != nil && slices.Contains(selection.Files, *finding.Path) {
		return true
	}
	return slices.ContainsFunc(finding.Trace, func(step report.TraceStep) bool {
		return step.Origin == "project" && step.Path != nil && slices.Contains(selection.Files, *step.Path)
	})
}

func validRoots(roots psalmanalysis.Roots) bool {
	names := []string{roots.Project, roots.Bundle, roots.Generated}
	for i, name := range names {
		if !filepath.IsAbs(name) || filepath.Clean(name) != name {
			return false
		}
		for _, other := range names[i+1:] {
			if within(name, other) != "" || within(other, name) != "" || name == other {
				return false
			}
		}
	}
	return true
}

func normalizeLocation(input location, roots psalmanalysis.Roots) (report.TraceStep, error) {
	point := report.TraceStep{Origin: "symbol", Snippet: input.Snippet}
	invalid := fmt.Errorf("invalid native location")
	if input.File == "" {
		if input.Line != 0 || input.EndLine != 0 || input.Column != 0 || input.EndColumn != 0 {
			return point, invalid
		}
		return point, nil
	}
	if input.Line < 1 || input.Column < 0 || input.EndColumn < 0 || input.EndLine != 0 && input.EndLine < input.Line || input.EndColumn > 0 && (input.Column == 0 || input.EndLine == 0) || input.EndLine == input.Line && input.EndColumn > 0 && input.EndColumn < input.Column {
		return point, invalid
	}
	for _, candidate := range []struct{ origin, root string }{{"project", roots.Project}, {"tool", roots.Bundle}, {"generated", roots.Generated}} {
		if name := within(candidate.root, input.File); name != "" {
			point.Origin, point.Path, point.Line = candidate.origin, new(name), new(input.Line)
			if input.Column > 0 {
				point.Column = new(input.Column)
			}
			if input.EndLine > 0 {
				point.EndLine = new(input.EndLine)
			}
			if input.EndColumn > 0 {
				point.EndColumn = new(input.EndColumn)
			}
			return point, nil
		}
	}
	return point, invalid
}

func within(root, filename string) string {
	if !filepath.IsAbs(filename) || filepath.Clean(filename) != filename {
		return ""
	}
	relative, err := filepath.Rel(root, filename)
	if err != nil || relative == "." || projectpath.Validate(filepath.ToSlash(relative)) != nil {
		return ""
	}
	return filepath.ToSlash(relative)
}

func fingerprint(finding report.Finding) string {
	finding.Trace = slices.Clone(finding.Trace)
	for index := range finding.Trace {
		finding.Trace[index].Snippet = ""
	}
	data, _ := json.Marshal(finding)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
