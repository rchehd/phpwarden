package projecttest

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"github.com/rchehd/phpwarden/internal/progress"
)

//go:embed mutation-config.php
var mutationConfig string

type MutationStats struct {
	Total            int     `json:"totalMutantsCount"`
	Killed           int     `json:"killedCount"`
	StaticKilled     int     `json:"killedByStaticAnalysisCount"`
	Uncovered        int     `json:"notCoveredCount"`
	Escaped          int     `json:"escapedCount"`
	Errors           int     `json:"errorCount"`
	SyntaxErrors     int     `json:"syntaxErrorCount"`
	Skipped          int     `json:"skippedCount"`
	Ignored          int     `json:"ignoredCount"`
	Timeouts         int     `json:"timeOutCount"`
	MSI              float64 `json:"msi"`
	MutationCoverage float64 `json:"mutationCodeCoverage"`
	CoveredMSI       float64 `json:"coveredCodeMsi"`
}

type Mutant struct {
	Mutator struct {
		Name     string `json:"mutatorName"`
		Original string `json:"originalSourceCode"`
		Mutated  string `json:"mutatedSourceCode"`
		File     string `json:"originalFilePath"`
		Line     int    `json:"originalStartLine"`
	} `json:"mutator"`
	Diff   string `json:"diff"`
	Output string `json:"processOutput"`
}

type MutationReport struct {
	Version       string        `json:"version"`
	Stats         MutationStats `json:"stats"`
	Escaped       []Mutant      `json:"escaped"`
	Timeouts      []Mutant      `json:"timeouted"`
	Killed        []Mutant      `json:"killed"`
	StaticKilled  []Mutant      `json:"killedByStaticAnalysis"`
	Errors        []Mutant      `json:"errored"`
	SyntaxErrors  []Mutant      `json:"syntaxErrors"`
	Uncovered     []Mutant      `json:"uncovered"`
	Ignored       []Mutant      `json:"ignored"`
	MinMSI        *float64      `json:"min_msi,omitempty"`
	MinCoveredMSI *float64      `json:"min_covered_msi,omitempty"`
}

func readMutation(directory string, plan MutationPlan) (*MutationReport, error) {
	if err := regularFile(directory, "infection.json"); err != nil {
		return nil, err
	}
	filename := filepath.Join(directory, "infection.json")
	info, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	if info.Size() > 64<<20 {
		return nil, fmt.Errorf("oversized Infection report")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(data, &shape); err != nil {
		return nil, err
	}
	var statsShape map[string]json.RawMessage
	if err := json.Unmarshal(shape["stats"], &statsShape); err != nil {
		return nil, err
	}
	for _, name := range []string{"totalMutantsCount", "killedCount", "killedByStaticAnalysisCount", "notCoveredCount", "escapedCount", "errorCount", "syntaxErrorCount", "skippedCount", "ignoredCount", "timeOutCount", "msi", "mutationCodeCoverage", "coveredCodeMsi"} {
		if value, exists := statsShape[name]; !exists || string(value) == "null" {
			return nil, fmt.Errorf("missing Infection statistic %s", name)
		}
	}
	var report MutationReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	report.Version = InfectionVersion
	s := report.Stats
	groups := []struct {
		name  string
		rows  []Mutant
		count int
	}{
		{"escaped", report.Escaped, s.Escaped}, {"timeouted", report.Timeouts, s.Timeouts},
		{"killed", report.Killed, s.Killed}, {"killedByStaticAnalysis", report.StaticKilled, s.StaticKilled},
		{"errored", report.Errors, s.Errors}, {"syntaxErrors", report.SyntaxErrors, s.SyntaxErrors},
		{"uncovered", report.Uncovered, s.Uncovered}, {"ignored", report.Ignored, s.Ignored},
	}
	total := s.Skipped
	if s.Skipped < 0 || s.Total < 0 {
		return nil, fmt.Errorf("negative Infection counts")
	}
	for _, group := range groups {
		if value, exists := shape[group.name]; !exists || len(value) == 0 || value[0] != '[' || group.count < 0 || group.count != len(group.rows) {
			return nil, fmt.Errorf("Infection records disagree with %s count", group.name)
		}
		total += group.count
		for _, row := range group.rows {
			file := strings.TrimPrefix(row.Mutator.File, "/workspace/")
			if !strings.HasPrefix(row.Mutator.File, "/workspace/") || row.Mutator.Name == "" || row.Mutator.Line < 1 || !mutationContainsFile(plan, file) {
				return nil, fmt.Errorf("Infection mutant is outside the selected source scope")
			}
		}
	}
	if total != s.Total {
		return nil, fmt.Errorf("Infection totals disagree with mutant records")
	}
	for _, value := range []float64{s.MSI, s.MutationCoverage, s.CoveredMSI} {
		if !finite(value) || value < 0 || value > 100 {
			return nil, fmt.Errorf("invalid Infection score")
		}
	}
	if s.Errors+s.SyntaxErrors+s.Timeouts+s.Skipped == 0 {
		denominator := s.Total - s.Ignored
		expectedMSI, expectedCovered, expectedCoverage := 0.0, 0.0, 0.0
		if denominator > 0 {
			expectedMSI = 100 * float64(s.Killed+s.StaticKilled) / float64(denominator)
			expectedCoverage = 100 * float64(denominator-s.Uncovered) / float64(denominator)
		}
		if denominator-s.Uncovered > 0 {
			expectedCovered = 100 * float64(s.Killed+s.StaticKilled) / float64(denominator-s.Uncovered)
		}
		if math.Abs(expectedMSI-s.MSI) > 0.011 || math.Abs(expectedCovered-s.CoveredMSI) > 0.011 || math.Abs(expectedCoverage-s.MutationCoverage) > 0.011 {
			return nil, fmt.Errorf("Infection scores disagree with mutant counts")
		}
	}
	if err := regularFile(directory, "infection.html"); err != nil {
		return nil, err
	}
	return &report, nil
}

func runMutation(ctx context.Context, plan Plan, configuration, checkout, directory string, execute Execute, result *Report) {
	result.Complete, result.Status, result.ExitCode = false, "error", 3
	fail := func(err error) { result.Diagnostics = append(result.Diagnostics, err.Error()) }
	progress.Stage(ctx, "Preparing isolated mutation testing")
	if err := mutationGit(ctx, checkout, *plan.Mutation); err != nil {
		fail(err)
		return
	}
	data, err := json.Marshal(plan)
	if err != nil {
		fail(err)
		return
	}
	prepared := execute(ctx, checkout, directory, []string{"-r", strings.TrimPrefix(mutationConfig, "<?php\n"), string(data), "/workspace/" + configuration})
	if err := saveLog(directory, "infection-setup", prepared); err != nil {
		fail(err)
		return
	}
	var settings struct {
		Config        string   `json:"config"`
		MinMSI        *float64 `json:"min_msi"`
		MinCoveredMSI *float64 `json:"min_covered_msi"`
	}
	if prepared.ExitCode != 0 || prepared.Err != nil || json.Unmarshal(prepared.Stdout, &settings) != nil || !strings.HasPrefix(settings.Config, "/workspace/") {
		fail(fmt.Errorf("Infection setup failed; inspect infection-setup.stdout/stderr"))
		return
	}
	for _, threshold := range []*float64{settings.MinMSI, settings.MinCoveredMSI} {
		if threshold != nil && (!finite(*threshold) || *threshold < 0 || *threshold > 100) {
			fail(fmt.Errorf("mutation thresholds must be between 0 and 100"))
			return
		}
	}
	progress.Stage(ctx, "Running mutation tests")
	args := []string{infectionExecutable, "--configuration=" + settings.Config, "--no-interaction", "--no-progress", "--no-ansi", "--skip-initial-tests", "--coverage=/artifacts/infection-coverage", "--with-uncovered", "--logger-github=false"}
	if plan.Mutation.Scope.Mode == "diff" && !plan.Mutation.Scope.WholeFiles {
		args = append(args, "--git-diff-lines", "--git-diff-base=HEAD")
	} else {
		for _, name := range plan.Mutation.Scope.Files {
			args = append(args, "/workspace/"+name)
		}
	}
	execution := execute(ctx, checkout, directory, args)
	if err := saveLog(directory, "infection", execution); err != nil {
		fail(err)
		return
	}
	if ctx.Err() != nil {
		return
	}
	report, err := readMutation(directory, *plan.Mutation)
	if err != nil {
		fail(fmt.Errorf("invalid Infection report: %w; inspect infection.stdout/stderr", err))
		return
	}
	result.Mutation = report
	report.MinMSI, report.MinCoveredMSI = settings.MinMSI, settings.MinCoveredMSI
	if execution.ExitCode != 0 || execution.Err != nil {
		fail(fmt.Errorf("Infection did not complete successfully (exit %d); inspect infection.stdout/stderr", execution.ExitCode))
		return
	}
	s := report.Stats
	if s.Total == 0 || s.Total == s.Ignored+s.Skipped {
		result.Status = "not_run"
		fail(fmt.Errorf("Infection produced no assessed mutations"))
		return
	}
	if s.Errors+s.SyntaxErrors+s.Timeouts+s.Skipped > 0 {
		fail(fmt.Errorf("Mutation analysis is incomplete: %d errors, %d syntax errors, %d timeouts, %d skipped mutants", s.Errors, s.SyntaxErrors, s.Timeouts, s.Skipped))
		return
	}
	result.Complete, result.Status, result.ExitCode = true, "passed", 0
	if settings.MinMSI != nil && s.MSI < *settings.MinMSI {
		result.Status, result.ExitCode = "failed", 1
		fail(fmt.Errorf("MSI %.2f%% is below the configured minimum %.2f%%", s.MSI, *settings.MinMSI))
	}
	if settings.MinCoveredMSI != nil {
		if s.Total-s.Uncovered-s.Ignored-s.Skipped == 0 {
			result.Complete, result.Status, result.ExitCode = false, "error", 3
			fail(fmt.Errorf("Covered MSI threshold requires covered mutations"))
		} else if s.CoveredMSI < *settings.MinCoveredMSI {
			result.Status, result.ExitCode = "failed", 1
			fail(fmt.Errorf("Covered MSI %.2f%% is below the configured minimum %.2f%%", s.CoveredMSI, *settings.MinCoveredMSI))
		}
	}
}

func mutationContainsFile(plan MutationPlan, file string) bool {
	_, exists := plan.Scope.LineMap[file]
	return exists
}
