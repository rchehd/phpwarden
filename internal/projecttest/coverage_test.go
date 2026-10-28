package projecttest

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/rchehd/phpwarden/internal/runner"
)

func coverageFixture(t *testing.T, directory string) {
	t.Helper()
	put(t, directory, "coverage/clover.xml", `<coverage><project><metrics statements="4" coveredstatements="2"/></project></coverage>`)
	put(t, directory, "coverage/xml/index.xml", `<phpunit><build><driver name="pcov" version="1.0.12"/></build><project source="/workspace/src"/></phpunit>`)
	put(t, directory, "coverage/xml/Risk.php.xml", `<phpunit><file name="Risk.php" path="."><totals><lines executable="4" executed="2"/></totals><class name="Risk"><namespace name="App"/><method name="branch" start="8" crap="12.5" coverage="50"/></class></file></phpunit>`)
	put(t, directory, "coverage/crap4j.xml", `<crap_result><methods><method><className>App\Risk</className><methodName>branch</methodName><complexity>4</complexity><coverage>50</coverage><crap>12.5</crap></method></methods></crap_result>`)
	put(t, directory, "coverage/html/index.html", "<html>coverage</html>")
}

func TestCoverageImportsNativeMetricsAndRejectsIncompleteArtifacts(t *testing.T) {
	for _, scenario := range []string{"valid", "zero", "counts", "missing", "mismatch", "NaN", "trailing", "escape", "symlink"} {
		t.Run(scenario, func(t *testing.T) {
			directory := t.TempDir()
			coverageFixture(t, directory)
			switch scenario {
			case "zero":
				put(t, directory, "coverage/clover.xml", `<coverage><project><metrics statements="0" coveredstatements="0"/></project></coverage>`)
			case "counts":
				put(t, directory, "coverage/clover.xml", `<coverage><project><metrics statements="4" coveredstatements="5"/></project></coverage>`)
			case "missing":
				os.Remove(filepath.Join(directory, "coverage/xml/Risk.php.xml"))
			case "mismatch":
				put(t, directory, "coverage/crap4j.xml", `<crap_result/>`)
			case "NaN":
				put(t, directory, "coverage/xml/Risk.php.xml", `<phpunit><file name="Risk.php" path="."><totals><lines executable="4" executed="2"/></totals><class name="Risk"><method name="branch" start="8" crap="NaN" coverage="50"/></class></file></phpunit>`)
			case "trailing":
				put(t, directory, "coverage/clover.xml", `<coverage><project><metrics statements="4" coveredstatements="2"/></project></coverage><extra/>`)
			case "escape":
				put(t, directory, "coverage/xml/index.xml", `<phpunit><project source="/tmp/private"/></phpunit>`)
			case "symlink":
				os.Remove(filepath.Join(directory, "coverage/clover.xml"))
				if err := os.Symlink(filepath.Join(t.TempDir(), "outside"), filepath.Join(directory, "coverage/clover.xml")); err != nil {
					t.Fatal(err)
				}
			}
			result, err := readCoverage(directory)
			if scenario != "valid" {
				if err == nil {
					t.Fatalf("accepted %s: %+v", scenario, result)
				}
				return
			}
			if err != nil || result.LinePercent != 50 || result.MaxCRAP != 12.5 || len(result.Methods) != 1 || result.Methods[0].File != "src/Risk.php" || result.Methods[0].Complexity != 4 {
				t.Fatalf("%+v %v", result, err)
			}
		})
	}
}

func TestCoverageRunsAllPathsOnceAndSeparatesThresholdsFromExecutionErrors(t *testing.T) {
	for _, scenario := range []string{"advisory", "line gate", "crap gate", "missing report", "missing driver"} {
		t.Run(scenario, func(t *testing.T) {
			root, effective := fixture(t)
			put(t, root, "web/modules/custom/b/tests/src/Unit/OtherTest.php", "<?php")
			plan, err := Build(context.Background(), root, effective, Options{Coverage: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Paths) != 2 || len(plan.Coverage.Include) == 0 {
				t.Fatalf("%+v", plan)
			}
			minimum, maximum := 75.0, 10.0
			if scenario == "line gate" {
				plan.Coverage.MinLineCoverage = &minimum
			}
			if scenario == "crap gate" {
				plan.Coverage.MaxCRAP = &maximum
			}
			runs := 0
			execute := func(ctx context.Context, checkout, output string, args []string) runner.Result {
				if slices.Contains(args, "-r") {
					if scenario == "missing driver" {
						return runner.Result{ExitCode: 255}
					}
					put(t, checkout, "web/core/.phpwarden-coverage-fixture", "<phpunit/>")
					return runner.Result{Stdout: []byte("/workspace/web/core/.phpwarden-coverage-fixture")}
				}
				if !slices.Contains(args, "--version") {
					runs++
					if slices.Contains(args, "--no-coverage") || !slices.Contains(args, "pcov.enabled=1") || !strings.HasPrefix(args[len(args)-1], "/artifacts/") {
						t.Fatalf("wrong coverage args: %v", args)
					}
					if scenario != "missing report" {
						coverageFixture(t, output)
					}
				}
				return fakeExecute(t, successJUnit, 0, nil)(ctx, checkout, output, args)
			}
			result := Run(context.Background(), plan, filepath.Join(t.TempDir(), "results"), execute)
			want := 0
			if strings.HasSuffix(scenario, "gate") {
				want = 1
			}
			if strings.HasPrefix(scenario, "missing") {
				want = 3
			}
			if result.ExitCode != want || result.Complete != (want != 3) || runs > 1 {
				t.Fatalf("%+v runs=%d", result, runs)
			}
		})
	}
}
