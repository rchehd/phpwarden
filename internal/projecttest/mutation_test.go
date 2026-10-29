package projecttest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/runner"
	"github.com/rchehd/phpwarden/internal/scope"
)

func mutationFixture(t *testing.T, output string) {
	t.Helper()
	row := func(name string) Mutant {
		var result Mutant
		result.Mutator.Name, result.Mutator.File, result.Mutator.Line = name, "/workspace/src/Risk.php", 8
		result.Mutator.Original, result.Mutator.Mutated = "return 1;", "return 0;"
		result.Diff = "- return 1;\n+ return 0;"
		return result
	}
	report := MutationReport{Stats: MutationStats{Total: 3, Killed: 1, Escaped: 1, Uncovered: 1, MSI: 33.33, MutationCoverage: 66.67, CoveredMSI: 50}, Killed: []Mutant{row("IncrementInteger")}, Escaped: []Mutant{row("DecrementInteger")}, Uncovered: []Mutant{row("ReturnRemoval")}, Timeouts: []Mutant{}, StaticKilled: []Mutant{}, Errors: []Mutant{}, SyntaxErrors: []Mutant{}, Ignored: []Mutant{}}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	put(t, output, "infection.json", string(data))
	put(t, output, "infection.html", "<html>mutants</html>")
}

func mutationSelection() scope.Selection {
	return scope.Selection{Mode: "full", Files: []string{"src/Risk.php"}, LineMap: map[string][]scope.Range{"src/Risk.php": nil}}
}

func TestMutationReportKeepsDiffsAndRejectsInvalidCountsAndScope(t *testing.T) {
	for _, scenario := range []string{"valid", "count", "missing stat", "null stat", "missing group", "negative", "bad score", "score mismatch", "outside", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			output := t.TempDir()
			mutationFixture(t, output)
			data, _ := os.ReadFile(filepath.Join(output, "infection.json"))
			var document map[string]any
			json.Unmarshal(data, &document)
			stats := document["stats"].(map[string]any)
			switch scenario {
			case "count":
				stats["totalMutantsCount"] = 9
			case "missing stat":
				delete(stats, "escapedCount")
			case "null stat":
				stats["escapedCount"] = nil
			case "missing group":
				delete(document, "escaped")
			case "negative":
				stats["skippedCount"] = -1
			case "bad score":
				stats["msi"] = 101
			case "score mismatch":
				stats["msi"] = 100
			case "outside":
				document["escaped"].([]any)[0].(map[string]any)["mutator"].(map[string]any)["originalFilePath"] = "/workspace/other.php"
			}
			data, _ = json.Marshal(document)
			if scenario == "malformed" {
				data = []byte(`{"stats":`)
			}
			put(t, output, "infection.json", string(data))
			result, err := readMutation(output, MutationPlan{Scope: mutationSelection()})
			if scenario != "valid" {
				if err == nil {
					t.Fatalf("accepted %s", scenario)
				}
				return
			}
			if err != nil || len(result.Escaped) != 1 || result.Escaped[0].Diff == "" || result.Stats.Total != 3 {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

func TestMutationExecutionGatesAndFailurePaths(t *testing.T) {
	for _, scenario := range []string{"advisory", "threshold", "initial failure", "missing report", "zero", "timeout", "wrong version", "setup failure", "native failure"} {
		t.Run(scenario, func(t *testing.T) {
			root, effective := fixture(t)
			put(t, root, "src/Risk.php", "<?php return 1;")
			minimum := 80.0
			if scenario == "threshold" {
				effective.Config.Mutation.MinMSI = &minimum
			}
			plan, err := Build(context.Background(), root, effective, Options{Mutation: true, MutationScope: mutationSelection()})
			if err != nil {
				t.Fatal(err)
			}
			mutations := 0
			execute := func(ctx context.Context, checkout, output string, args []string) runner.Result {
				if args[0] == infectionExecutable {
					if slices.Contains(args, "--version") {
						version := InfectionVersion
						if scenario == "wrong version" {
							version = "0.0.0"
						}
						return runner.Result{Stdout: []byte("Infection " + version)}
					}
					mutations++
					if !slices.Contains(args, "--with-uncovered") || !slices.Contains(args, "--skip-initial-tests") {
						t.Fatalf("%v", args)
					}
					if scenario == "missing report" {
						return runner.Result{}
					}
					mutationFixture(t, output)
					if scenario == "zero" || scenario == "timeout" {
						data, _ := os.ReadFile(filepath.Join(output, "infection.json"))
						var report MutationReport
						json.Unmarshal(data, &report)
						if scenario == "zero" {
							report.Stats = MutationStats{}
							report.Killed, report.Escaped, report.Uncovered = []Mutant{}, []Mutant{}, []Mutant{}
						} else {
							report.Timeouts, report.Escaped = report.Escaped, []Mutant{}
							report.Stats.Timeouts, report.Stats.Escaped = 1, 0
						}
						data, _ = json.Marshal(report)
						put(t, output, "infection.json", string(data))
					}
					if scenario == "native failure" {
						return runner.Result{ExitCode: 1}
					}
					return runner.Result{}
				}
				if slices.Contains(args, "-r") {
					if strings.Contains(args[1], "require 'phar:") {
						if scenario == "setup failure" {
							return runner.Result{ExitCode: 255}
						}
						var value any
						if scenario == "threshold" {
							value = minimum
						}
						data, _ := json.Marshal(map[string]any{"config": "/workspace/.phpwarden-mutation-fixture", "min_msi": value})
						return runner.Result{Stdout: data}
					}
					put(t, checkout, "web/core/.phpwarden-coverage-fixture", "<phpunit/>")
					return runner.Result{Stdout: []byte("/workspace/web/core/.phpwarden-coverage-fixture")}
				}
				if !slices.Contains(args, "--version") {
					coverageFixture(t, output)
				}
				junit, exit := successJUnit, 0
				if scenario == "initial failure" {
					junit = `<testsuites><testsuite name="unit" tests="1" failures="1"><testcase name="bad"><failure>bad</failure></testcase></testsuite></testsuites>`
					exit = 1
				}
				return fakeExecute(t, junit, exit, nil)(ctx, checkout, output, args)
			}
			result := Run(context.Background(), plan, filepath.Join(t.TempDir(), "results"), execute)
			want := 3
			if scenario == "advisory" {
				want = 0
			}
			if scenario == "threshold" || scenario == "initial failure" {
				want = 1
			}
			if result.ExitCode != want || result.Command != "mutation" || result.Complete != (scenario == "advisory" || scenario == "threshold") {
				t.Fatalf("%+v", result)
			}
			if scenario == "initial failure" && mutations != 0 {
				t.Fatal("mutated despite initial test failure")
			}
		})
	}
}

func TestPrivateMutationGitDoesNotUseHostGitDirectory(t *testing.T) {
	checkout := t.TempDir()
	outside := t.TempDir()
	put(t, outside, "preserved", "keep")
	t.Setenv("GIT_DIR", outside)
	put(t, checkout, "src/Risk.php", "<?php return 2;\n")
	selection := mutationSelection()
	selection.Mode = "diff"
	plan := MutationPlan{Scope: selection, BaseContents: map[string][]byte{"src/Risk.php": []byte("<?php return 1;\n")}}
	if err := mutationGit(context.Background(), checkout, plan); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(checkout, ".git/HEAD")); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(checkout, "src/Risk.php"))
	if string(data) != "<?php return 2;\n" {
		t.Fatal("current source not restored")
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 1 || entries[0].Name() != "preserved" {
		t.Fatal("host Git directory changed")
	}
}
