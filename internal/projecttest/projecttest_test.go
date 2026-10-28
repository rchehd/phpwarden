package projecttest

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/config"
	"github.com/rchehd/phpwarden/internal/profile"
	"github.com/rchehd/phpwarden/internal/runner"
)

func put(t *testing.T, root, name, value string) {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) (string, config.Effective) {
	t.Helper()
	root := t.TempDir()
	effective, err := config.Parse([]byte("version: 1\nprofile: drupal\nphp: {runtime: '8.3', target: {min: '8.3', max: '8.3'}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	web := "web"
	effective.Project = &profile.Facts{WebRoot: &web, SourceRoots: []string{"web/modules/custom"}}
	put(t, root, "composer.json", `{"config":{"vendor-dir":"dependencies"}}`)
	put(t, root, "dependencies/bin/phpunit", "<?php")
	put(t, root, "web/core/phpunit.xml.dist", `<phpunit bootstrap="tests/bootstrap.php"/>`)
	put(t, root, "web/modules/custom/a/tests/src/Unit/ExampleTest.php", "<?php")
	put(t, root, "web/modules/custom/b/tests/src/Kernel/ExampleTest.php", "<?php")
	return root, effective
}

func TestPlanUsesProfileUnitPathsProjectConfigAndCustomVendor(t *testing.T) {
	root, effective := fixture(t)
	plan, err := Build(context.Background(), root, effective, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Executable != "dependencies/bin/phpunit" || plan.Config != "web/core/phpunit.xml.dist" || !slices.Equal(plan.Paths, []string{"web/modules/custom/a/tests/src/Unit"}) {
		t.Fatalf("%+v", plan)
	}
	put(t, root, "phpunit.xml", `<phpunit/>`)
	put(t, root, "phpunit.xml.dist", `<phpunit/>`)
	plan, err = Build(context.Background(), root, effective, Options{Suite: "unit", Filter: "boundary"})
	if err != nil || plan.Config != "phpunit.xml" || len(plan.Paths) != 0 || plan.Suite != "unit" || plan.Filter != "boundary" {
		t.Fatalf("%+v %v", plan, err)
	}
	effective.Config.Tests.Paths = []string{"web/modules/custom/b/tests/src/Kernel"}
	plan, err = Build(context.Background(), root, effective, Options{Paths: []string{"web/modules/custom/a/tests/src/Unit/ExampleTest.php"}})
	if err != nil || len(plan.Paths) != 1 || !strings.HasSuffix(plan.Paths[0], "ExampleTest.php") {
		t.Fatalf("%+v %v", plan, err)
	}
	plan, err = Build(context.Background(), root, effective, Options{Paths: []string{"web/modules/custom/a/tests/src/Unit", "web/modules/custom/a/tests/src/Unit/ExampleTest.php"}})
	if err != nil || len(plan.Paths) != 1 || strings.HasSuffix(plan.Paths[0], ".php") {
		t.Fatalf("overlapping paths: %+v %v", plan, err)
	}
}

func TestPlanHonorsMonorepoAndRejectsMissingOrEscapingInputs(t *testing.T) {
	root, effective := fixture(t)
	effective.Config.ComposerRoot = "backend"
	put(t, root, "backend/composer.json", `{}`)
	put(t, root, "backend/vendor/bin/phpunit", "<?php")
	put(t, root, "backend/phpunit.xml", `<phpunit/>`)
	plan, err := Build(context.Background(), root, effective, Options{})
	if err != nil || plan.Executable != "backend/vendor/bin/phpunit" || plan.Config != "backend/phpunit.xml" {
		t.Fatalf("%+v %v", plan, err)
	}
	for _, options := range []Options{{Config: "missing.xml"}, {Paths: []string{"../outside"}}, {Paths: []string{"missing"}}} {
		if _, err := Build(context.Background(), root, effective, options); err == nil {
			t.Fatalf("accepted %+v", options)
		}
	}
	outside := t.TempDir()
	put(t, outside, "config.xml", "<phpunit/>")
	if err := os.Symlink(filepath.Join(outside, "config.xml"), filepath.Join(root, "escaped.xml")); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(context.Background(), root, effective, Options{Config: "escaped.xml"}); err == nil {
		t.Fatal("accepted escaping config")
	}
	if err := os.Remove(filepath.Join(root, "backend/vendor/bin/phpunit")); err != nil {
		t.Fatal(err)
	}
	if _, err := Build(context.Background(), root, effective, Options{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("%v", err)
	}
}

func TestPlanRejectsKnownDependencyMismatchBeforeRunningTests(t *testing.T) {
	root, effective := fixture(t)
	message := "installed Drupal conflicts with composer.lock"
	effective.Project.Installed = &profile.Installed{Status: "invalid", Error: &message}
	if _, err := Build(context.Background(), root, effective, Options{}); !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), message) {
		t.Fatalf("%v", err)
	}
}

const successJUnit = `<?xml version="1.0"?><testsuites><testsuite name="unit" tests="1" failures="0" errors="0" skipped="0"><testsuite name="ExampleTest" tests="1"><testcase name="testBoundary" classname="ExampleTest" assertions="2" file="/workspace/src/ExampleTest.php" line="7" time="0.01"/></testsuite></testsuite></testsuites>`

func TestJUnitValidatesActualCasesAndTotals(t *testing.T) {
	_, cases, counts, err := readJUnit([]byte(successJUnit))
	if err != nil || counts.Tests != 1 || counts.Assertions != 2 || len(cases) != 1 {
		t.Fatalf("%+v %v", counts, err)
	}
	for _, value := range []string{
		`<testsuites>`, `<wrong/>`, successJUnit + `<testsuites/>`,
		strings.Replace(successJUnit, `tests="1"`, `tests="9"`, 1),
		strings.Replace(successJUnit, `assertions="2"`, `assertions="-1"`, 1),
		strings.Replace(successJUnit, `time="0.01"`, `time="NaN"`, 1),
		strings.Replace(successJUnit, `failures="0"`, `failures="1"`, 1),
	} {
		if _, _, _, err := readJUnit([]byte(value)); err == nil {
			t.Fatalf("accepted invalid report %s", value)
		}
	}
}

func TestPHPUnit9RetainsMultipleErrorsForOneTestcase(t *testing.T) {
	data, err := os.ReadFile("testdata/phpunit9-multiple-errors.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, cases, counts, err := readJUnit(data)
	if err != nil || counts.Tests != 2 || counts.Errors != 2 || len(cases) != 2 || len(cases[0].Errors) != 2 {
		t.Fatalf("%+v %+v %v", counts, cases, err)
	}
	if cases[0].Errors[0].Type != "RuntimeException" || cases[0].Errors[1].Type != "PHPUnit\\Framework\\RiskyTestError" {
		t.Fatalf("lost error records: %+v", cases[0])
	}
}

func fakeExecute(t *testing.T, junit string, exit int, action func(string, string)) Execute {
	t.Helper()
	return func(ctx context.Context, checkout, output string, args []string) runner.Result {
		if slices.Contains(args, "--version") {
			return runner.Result{Stdout: []byte("PHPUnit 10.5.66 by Sebastian Bergmann and contributors.\n")}
		}
		if action != nil {
			action(checkout, output)
		}
		if junit != "" {
			index := slices.Index(args, "--log-junit")
			put(t, output, filepath.Base(args[index+1]), junit)
		}
		return runner.Result{ExitCode: exit, Stdout: []byte("fixture output"), Stderr: []byte("fixture stderr")}
	}
}

func TestRunPreservesSourceAndSavesConsistentReports(t *testing.T) {
	root, effective := fixture(t)
	plan, err := Build(context.Background(), root, effective, Options{})
	if err != nil {
		t.Fatal(err)
	}
	put(t, root, "source.php", "original")
	put(t, root, ".git/config", "private metadata")
	output := filepath.Join(t.TempDir(), "results")
	var isolated string
	result := Run(context.Background(), plan, output, fakeExecute(t, successJUnit, 0, func(checkout, output string) {
		isolated = checkout
		if _, err := os.Stat(filepath.Join(checkout, ".git")); !os.IsNotExist(err) {
			t.Fatal("copied Git metadata")
		}
		put(t, checkout, "source.php", "changed in copy")
	}))
	if !result.Complete || result.ExitCode != 0 || result.Counts.Tests != 1 || result.Cases[0].File != "src/ExampleTest.php" {
		t.Fatalf("%+v", result)
	}
	data, _ := os.ReadFile(filepath.Join(root, "source.php"))
	if string(data) != "original" {
		t.Fatal("source modified")
	}
	if _, err := os.Stat(isolated); !os.IsNotExist(err) {
		t.Fatal("workspace not removed")
	}
	data, err = os.ReadFile(filepath.Join(output, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var saved Report
	if err := json.Unmarshal(data, &saved); err != nil || saved.ExitCode != result.ExitCode || !saved.Complete {
		t.Fatalf("%s %v", data, err)
	}
	data, err = os.ReadFile(filepath.Join(output, "junit.xml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, counts, err := readJUnit(data); err != nil || counts.Tests != 1 {
		t.Fatalf("%+v %v", counts, err)
	}
	if repeat := Run(context.Background(), plan, output, fakeExecute(t, successJUnit, 0, nil)); repeat.ExitCode != 2 {
		t.Fatalf("overwrote existing artifacts: %+v", repeat)
	}
}

func TestFilterAcrossUnitDirectoriesRequiresAtLeastOneExecutedTest(t *testing.T) {
	for _, allEmpty := range []bool{false, true} {
		t.Run(map[bool]string{false: "one match", true: "no matches"}[allEmpty], func(t *testing.T) {
			root, effective := fixture(t)
			put(t, root, "web/modules/custom/b/tests/src/Unit/OtherTest.php", "<?php")
			plan, err := Build(context.Background(), root, effective, Options{Filter: "Boundary"})
			if err != nil {
				t.Fatal(err)
			}
			execute := func(ctx context.Context, checkout, output string, args []string) runner.Result {
				junit := successJUnit
				if allEmpty || strings.Contains(args[len(args)-1], "/custom/a/") {
					junit = "<testsuites/>"
				}
				return fakeExecute(t, junit, 0, nil)(ctx, checkout, output, args)
			}
			result := Run(context.Background(), plan, filepath.Join(t.TempDir(), "results"), execute)
			want := 0
			if allEmpty {
				want = 3
			}
			if result.ExitCode != want || result.Complete == allEmpty {
				t.Fatalf("%+v", result)
			}
		})
	}
}

func TestRunDistinguishesFailedTestsFromIncompleteExecution(t *testing.T) {
	failed := `<testsuites><testsuite name="unit" tests="1" failures="1"><testcase name="testBad" assertions="1"><failure type="AssertionFailedError">Expected 18, got 17</failure></testcase></testsuite></testsuites>`
	errord := `<testsuites><testsuite name="unit" tests="1" errors="1"><testcase name="testException"><error type="RuntimeException">fixture exception</error></testcase></testsuite></testsuites>`
	for _, item := range []struct {
		name, junit  string
		native, exit int
		complete     bool
	}{
		{"assertion", failed, 1, 1, true}, {"test exception", errord, 2, 1, true},
		{"missing report", "", 2, 3, false}, {"malformed", "<testsuites>", 0, 3, false},
		{"empty", "<testsuites/>", 0, 3, false}, {"wrong exit", failed, 0, 3, false},
		{"warnings", successJUnit, 1, 3, false}, {"crash", successJUnit, 139, 3, false},
		{"skipped", `<testsuites><testsuite name="unit" tests="1" skipped="1"><testcase name="testSkip"><skipped/></testcase></testsuite></testsuites>`, 0, 3, false},
	} {
		t.Run(item.name, func(t *testing.T) {
			root, effective := fixture(t)
			plan, err := Build(context.Background(), root, effective, Options{})
			if err != nil {
				t.Fatal(err)
			}
			result := Run(context.Background(), plan, filepath.Join(t.TempDir(), "results"), fakeExecute(t, item.junit, item.native, nil))
			if result.ExitCode != item.exit || result.Complete != item.complete {
				t.Fatalf("%+v", result)
			}
		})
	}
}

func TestRunDetectsStaleInputsCancellationAndArtifactSymlinks(t *testing.T) {
	for _, scenario := range []string{"stale", "cancel", "junit link", "log link", "report link"} {
		t.Run(scenario, func(t *testing.T) {
			root, effective := fixture(t)
			plan, err := Build(context.Background(), root, effective, Options{})
			if err != nil {
				t.Fatal(err)
			}
			outside := t.TempDir()
			put(t, outside, "preserved", "keep")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			junit := successJUnit
			if scenario == "junit link" {
				junit = ""
			}
			action := func(checkout, output string) {
				switch scenario {
				case "stale":
					put(t, root, "new-input.php", "changed")
				case "cancel":
					cancel()
				default:
					name := map[string]string{"junit link": "phpunit-001.xml", "log link": "phpunit-001.stdout", "report link": "report.json"}[scenario]
					if err := os.Symlink(filepath.Join(outside, "preserved"), filepath.Join(output, name)); err != nil {
						t.Fatal(err)
					}
				}
			}
			result := Run(ctx, plan, filepath.Join(t.TempDir(), "results"), fakeExecute(t, junit, 0, action))
			expected := 3
			if scenario == "cancel" {
				expected = 130
			}
			if scenario == "report link" {
				expected = 0
			}
			if result.ExitCode != expected || scenario != "report link" && result.Complete {
				t.Fatalf("%+v", result)
			}
			data, _ := os.ReadFile(filepath.Join(outside, "preserved"))
			if string(data) != "keep" {
				t.Fatal("artifact symlink changed external file")
			}
		})
	}
}

func TestWorkspaceRejectsExternalLinksAndPreservesInternalLinks(t *testing.T) {
	root := t.TempDir()
	put(t, root, "source.php", "safe")
	if err := os.Symlink(filepath.Join(root, "source.php"), filepath.Join(root, "internal")); err != nil {
		t.Fatal(err)
	}
	copy := t.TempDir()
	if _, err := workspace(context.Background(), root, copy, ""); err != nil {
		t.Fatal(err)
	}
	link, err := os.Readlink(filepath.Join(copy, "internal"))
	if err != nil || link != "/workspace/source.php" {
		t.Fatalf("%s %v", link, err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := workspace(context.Background(), root, "", ""); err == nil {
		t.Fatal("accepted external symlink")
	}
}
