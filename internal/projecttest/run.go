package projecttest

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"time"

	"github.com/rchehd/phpwarden/internal/progress"
	"github.com/rchehd/phpwarden/internal/runner"
)

type Report struct {
	Version        int      `json:"version"`
	Command        string   `json:"command"`
	Status         string   `json:"status"`
	Complete       bool     `json:"complete"`
	ExitCode       int      `json:"exit_code"`
	Plan           *Plan    `json:"selection,omitempty"`
	PHPUnitVersion string   `json:"phpunit_version,omitempty"`
	Counts         Counts   `json:"counts"`
	Cases          []Case   `json:"cases"`
	Artifacts      string   `json:"artifacts,omitempty"`
	Diagnostics    []string `json:"diagnostics"`
}

type Execute func(context.Context, string, string, []string) runner.Result

func Invalid(message string, code int) Report {
	status := "not_run"
	if code == 130 {
		status = "interrupted"
	}
	return Report{Version: 1, Command: "test", Status: status, ExitCode: code, Cases: []Case{}, Diagnostics: []string{message}}
}

func Run(ctx context.Context, plan Plan, output string, execute Execute) (result Report) {
	result = Report{Version: 1, Command: "test", Status: "error", ExitCode: 3, Plan: &plan, Cases: []Case{}, Diagnostics: []string{}}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(plan.TimeoutSeconds)*time.Second)
	defer cancel()
	directory, err := artifacts(output)
	if err != nil {
		return Invalid(err.Error(), 2)
	}
	result.Artifacts = directory
	defer func() {
		if ctx.Err() != nil {
			result.Complete, result.Status, result.ExitCode = false, "error", 3
			if ctx.Err() == context.Canceled {
				result.ExitCode, result.Status = 130, "interrupted"
			}
			result.Diagnostics = append(result.Diagnostics, ctx.Err().Error())
		}
		if err := Save(result); err != nil {
			result.Complete, result.Status, result.ExitCode = false, "error", 3
			result.Diagnostics = append(result.Diagnostics, "Could not save test report: "+err.Error())
		}
	}()
	fail := func(err error) Report { result.Diagnostics = append(result.Diagnostics, err.Error()); return result }
	progress.Stage(ctx, "Preparing isolated test workspace")
	checkout, err := os.MkdirTemp("", "phpwarden-test-")
	if err != nil {
		return fail(err)
	}
	defer os.RemoveAll(checkout)
	if relative, err := filepath.Rel(plan.Root, checkout); err != nil || relative == "." || relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return fail(fmt.Errorf("temporary workspace must be outside the project; select a different TMPDIR"))
	}
	before, err := workspace(ctx, plan.Root, checkout, directory)
	if err != nil {
		return fail(err)
	}
	defer func() {
		verification, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		after, err := workspace(verification, plan.Root, "", directory)
		if err != nil || !reflect.DeepEqual(before, after) {
			result.Complete, result.Status, result.ExitCode = false, "error", 3
			result.Diagnostics = append(result.Diagnostics, "Project inputs changed during test execution; run test again.")
		}
	}()
	progress.Stage(ctx, "Checking project PHPUnit runtime")
	probe := execute(ctx, checkout, directory, []string{"/workspace/" + plan.Executable, "--version"})
	if err := saveLog(directory, "phpunit-version", probe); err != nil {
		return fail(err)
	}
	version := regexp.MustCompile(`^PHPUnit ([0-9]+\.[0-9]+\.[0-9]+)\b`).FindSubmatch(probe.Stdout)
	if probe.ExitCode != 0 || probe.Err != nil || version == nil {
		return fail(fmt.Errorf("project PHPUnit could not start with the selected PHP runtime; inspect phpunit-version.stdout/stderr and install compatible locked dev dependencies"))
	}
	result.PHPUnitVersion = string(version[1])
	paths := plan.Paths
	if len(paths) == 0 {
		paths = []string{""}
	}
	var suites []Suite
	for index, name := range paths {
		progress.Stage(ctx, fmt.Sprintf("Running project tests · %d/%d", index+1, len(paths)))
		prefix := fmt.Sprintf("phpunit-%03d", index+1)
		junit := prefix + ".xml"
		args := []string{"/workspace/" + plan.Executable, "--configuration", "/workspace/" + plan.Config, "--colors=never", "--do-not-cache-result", "--no-coverage", "--log-junit", "/artifacts/" + junit}
		if plan.Suite != "" {
			args = append(args, "--testsuite", plan.Suite)
		}
		if plan.Filter != "" {
			args = append(args, "--filter", plan.Filter)
		}
		if name != "" {
			args = append(args, "/workspace/"+name)
		}
		execution := execute(ctx, checkout, directory, args)
		if err := saveLog(directory, prefix, execution); err != nil {
			return fail(err)
		}
		if ctx.Err() != nil {
			return result
		}
		if execution.ExitCode < 0 || execution.ExitCode > 2 || execution.ExitCode == 0 && execution.Err != nil {
			return fail(fmt.Errorf("PHPUnit could not complete (exit %d); inspect %s.stdout/stderr", execution.ExitCode, prefix))
		}
		filename := filepath.Join(directory, junit)
		if err := regularFile(directory, junit); err != nil {
			return fail(fmt.Errorf("PHPUnit did not produce a valid JUnit report; inspect %s.stdout/stderr", prefix))
		}
		info, err := os.Lstat(filename)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 32<<20 {
			return fail(fmt.Errorf("invalid or oversized JUnit artifact"))
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return fail(err)
		}
		batch, cases, counts, err := readJUnit(data)
		if err != nil {
			return fail(fmt.Errorf("invalid PHPUnit report: %w", err))
		}
		if counts.Tests == 0 && plan.Filter == "" {
			return fail(fmt.Errorf("PHPUnit executed no tests; check paths, testsuite and filter"))
		}
		if execution.ExitCode == 0 && counts.Failures+counts.Errors > 0 {
			return fail(fmt.Errorf("PHPUnit exit code disagrees with failed testcase records"))
		}
		if execution.ExitCode != 0 && counts.Failures+counts.Errors == 0 {
			return fail(fmt.Errorf("PHPUnit reported issues without failed testcase records; inspect %s.stdout/stderr", prefix))
		}
		for i := range cases {
			cases[i].File = strings.TrimPrefix(cases[i].File, "/workspace/")
		}
		result.Cases = append(result.Cases, cases...)
		result.Counts.add(counts)
		suites = append(suites, batch...)
	}
	combined := struct {
		XMLName xml.Name `xml:"testsuites"`
		Suites  []Suite  `xml:"testsuite"`
	}{Suites: suites}
	data, err := xml.MarshalIndent(combined, "", "  ")
	if err != nil {
		return fail(err)
	}
	if err := writeArtifact(directory, "junit.xml", append([]byte(xml.Header), data...)); err != nil {
		return fail(err)
	}
	result.Complete, result.Status, result.ExitCode = true, "passed", 0
	if result.Counts.Failures+result.Counts.Errors > 0 {
		result.Status, result.ExitCode = "failed", 1
	}
	if result.Counts.Tests == result.Counts.Skipped {
		result.Complete, result.Status, result.ExitCode = false, "not_run", 3
		if result.Counts.Tests == 0 {
			result.Diagnostics = append(result.Diagnostics, "PHPUnit executed no tests; check paths, testsuite and filter.")
		} else {
			result.Diagnostics = append(result.Diagnostics, "All selected tests were skipped.")
		}
	}
	return result
}

func saveLog(directory, prefix string, result runner.Result) error {
	if err := writeArtifact(directory, prefix+".stdout", result.Stdout); err != nil {
		return err
	}
	return writeArtifact(directory, prefix+".stderr", result.Stderr)
}

func writeArtifact(directory, name string, data []byte) error {
	file, err := os.OpenFile(filepath.Join(directory, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func Save(result Report) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(result.Artifacts, ".report-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, err = file.Write(append(data, '\n'))
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), filepath.Join(result.Artifacts, "report.json"))
}
