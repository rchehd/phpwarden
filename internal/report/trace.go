package report

import (
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/projectpath"
)

func ValidateTrace(trace []TraceStep) error {
	for _, step := range trace {
		invalid := fmt.Errorf("invalid finding trace location")
		if strings.TrimSpace(step.Label) == "" || !slices.Contains([]string{"project", "tool", "generated", "symbol"}, step.Origin) {
			return invalid
		}
		if step.Origin == "symbol" {
			if step.Path != nil || step.Line != nil || step.Column != nil || step.EndLine != nil || step.EndColumn != nil {
				return invalid
			}
			continue
		}
		if step.Path == nil || projectpath.Validate(*step.Path) != nil || *step.Path == "." || path.Clean(*step.Path) != *step.Path || step.Line == nil || *step.Line < 1 || step.Column != nil && *step.Column < 1 {
			return invalid
		}
		if step.EndLine != nil && *step.EndLine < *step.Line || step.EndColumn != nil && (step.Column == nil || step.EndLine == nil || *step.EndColumn < 1 || *step.EndLine == *step.Line && *step.EndColumn < *step.Column) {
			return invalid
		}
	}
	return nil
}
