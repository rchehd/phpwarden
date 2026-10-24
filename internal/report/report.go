package report

import (
	"crypto/rand"
	"fmt"
	"sort"

	"github.com/rchehd/phpwarden/internal/scope"
)

type Report struct {
	Version     int              `json:"version"`
	RunID       string           `json:"run_id"`
	Verdict     string           `json:"verdict"`
	Complete    bool             `json:"complete"`
	ExitCode    int              `json:"exit_code"`
	Profile     *string          `json:"profile"`
	Scope       *scope.Selection `json:"scope"`
	Environment *Environment     `json:"environment"`
	Checks      []Check          `json:"checks"`
	Findings    []Finding        `json:"findings"`
	Diagnostics []Diagnostic     `json:"diagnostics"`
	FixedFiles  []string         `json:"fixed_files,omitempty"`
}

type Cache struct {
	Status string `json:"status"`
	Key    string `json:"key,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type Environment struct {
	Cache                *Cache      `json:"cache,omitempty"`
	FrameworkVersion     *string     `json:"framework_version,omitempty"`
	PHPRuntime           *string     `json:"php_runtime"`
	PHPVersion           *string     `json:"php_version,omitempty"`
	CompatibilityPending []string    `json:"compatibility_pending,omitempty"`
	DiscoveryWarnings    []string    `json:"discovery_warnings,omitempty"`
	Advisories           *Advisories `json:"advisories,omitempty"`
	RequestedRuntime     string      `json:"requested_runtime"`
	TargetMin            string      `json:"php_target_min"`
	TargetMax            string      `json:"php_target_max"`
	Image                *string     `json:"image"`
	Toolset              *string     `json:"toolset"`
	Protocol             int         `json:"protocol"`
}

// Advisories records where composer-audit took its security advisories from and when they were fetched.
type Advisories struct {
	Source    string `json:"source"`
	FetchedAt string `json:"fetched_at"`
}

type Check struct {
	ID       string  `json:"id"`
	Status   string  `json:"status"`
	Required bool    `json:"required"`
	FailOn   string  `json:"fail_on"`
	Reason   *string `json:"reason"`
}

type Finding struct {
	Fingerprint *string `json:"fingerprint,omitempty"`
	Checker     string  `json:"checker"`
	Rule        string  `json:"rule"`
	Kind        string  `json:"kind"`
	Severity    string  `json:"severity"`
	Message     string  `json:"message"`
	Path        *string `json:"path"`
	Line        *int    `json:"line"`
	Column      *int    `json:"column"`
}

type Diagnostic struct {
	Code    string  `json:"code"`
	Stage   string  `json:"stage"`
	Message string  `json:"message"`
	Checker *string `json:"checker"`
}

func New() Report {
	return Report{Version: 1, RunID: rand.Text(), Verdict: "pass", Complete: true,
		Checks: []Check{}, Findings: []Finding{}, Diagnostics: []Diagnostic{}}
}

func InputError(stage string, err error) Report {
	result := New()
	result.ExitCode, result.Verdict, result.Complete = 2, "error", false
	result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: stage + "_invalid", Stage: stage, Message: err.Error()})
	return result
}

func (result *Report) Finalize() {
	if result.ExitCode == 2 {
		return
	}
	result.Verdict, result.ExitCode, result.Complete = "pass", 0, true
	for _, check := range result.Checks {
		if check.Status == "error" || check.Status == "unavailable" {
			result.Complete = false
			if check.Required {
				result.ExitCode = 3
			}
		}
		if check.Status == "failed" && result.ExitCode == 0 {
			result.ExitCode = 1
		}
		if result.ExitCode == 0 && check.FailOn != "never" {
			for _, finding := range result.Findings {
				if finding.Checker == check.ID && (finding.Severity == "error" || check.FailOn == "warning" && finding.Severity == "warning") {
					result.ExitCode = 1
					break
				}
			}
		}
	}
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Checker == nil {
			result.ExitCode, result.Complete = 3, false
		}
	}
	if result.ExitCode == 1 {
		result.Verdict = "fail"
	} else if result.ExitCode != 0 {
		result.Verdict = "error"
	}
	sort.Slice(result.Checks, func(i, j int) bool { return result.Checks[i].ID < result.Checks[j].ID })
	sort.Slice(result.Findings, func(i, j int) bool {
		a, b := result.Findings[i], result.Findings[j]
		if value(a.Path) != value(b.Path) {
			return value(a.Path) < value(b.Path)
		}
		if number(a.Line) != number(b.Line) {
			return number(a.Line) < number(b.Line)
		}
		if number(a.Column) != number(b.Column) {
			return number(a.Column) < number(b.Column)
		}
		if a.Checker != b.Checker {
			return a.Checker < b.Checker
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Message < b.Message
	})
	unique := result.Findings[:0]
	seen := make(map[string]bool)
	for _, finding := range result.Findings {
		key := fmt.Sprintf("%q/%d/%d/%q/%q/%q/%q/%q", value(finding.Path), number(finding.Line), number(finding.Column), finding.Checker, finding.Rule, finding.Kind, finding.Severity, finding.Message)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, finding)
		}
	}
	result.Findings = unique
}

func value(input *string) string {
	if input == nil {
		return ""
	}
	return *input
}
func number(input *int) int {
	if input == nil {
		return 0
	}
	return *input
}
