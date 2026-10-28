package projecttest

import (
	"bytes"
	"cmp"
	"context"
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rchehd/phpwarden/internal/projectpath"
)

//go:embed coverage-config.php
var coverageConfig string

type CoverageReport struct {
	Driver          string           `json:"driver"`
	DriverVersion   string           `json:"driver_version"`
	ExecutableLines int              `json:"executable_lines"`
	CoveredLines    int              `json:"covered_lines"`
	LinePercent     float64          `json:"line_percent"`
	MaxCRAP         float64          `json:"max_crap"`
	Methods         []CoverageMethod `json:"methods"`
}

type CoverageMethod struct {
	Class       string  `json:"class"`
	Name        string  `json:"name"`
	File        string  `json:"file"`
	Line        int     `json:"line"`
	CRAP        float64 `json:"crap"`
	LinePercent float64 `json:"line_percent"`
	Complexity  int     `json:"complexity"`
}

func prepareCoverage(ctx context.Context, plan Plan, checkout, artifacts, version string, execute Execute) (string, error) {
	data, err := json.Marshal(plan)
	if err != nil {
		return "", err
	}
	major, _, _ := strings.Cut(version, ".")
	probe := execute(ctx, checkout, artifacts, []string{"-d", "pcov.enabled=1", "-r", strings.TrimPrefix(coverageConfig, "<?php\n"), string(data), major})
	if err := saveLog(artifacts, "coverage-setup", probe); err != nil {
		return "", err
	}
	name := strings.TrimSpace(string(probe.Stdout))
	relative := strings.TrimPrefix(name, "/workspace/")
	if probe.ExitCode != 0 || probe.Err != nil || !strings.HasPrefix(name, "/workspace/") || path.Dir(relative) != path.Dir(plan.Config) || !strings.HasPrefix(path.Base(relative), ".phpwarden-coverage-") {
		return "", fmt.Errorf("coverage setup failed; inspect coverage-setup.stdout/stderr (PCOV and PHPUnit 9–12 are required)")
	}
	if err := regularFile(checkout, relative); err != nil {
		return "", fmt.Errorf("invalid generated coverage configuration: %w", err)
	}
	return relative, nil
}

func coverageArtifact(directory, name string, target any) error {
	if err := regularFile(directory, name); err != nil {
		return err
	}
	info, err := os.Stat(filepath.Join(directory, name))
	if err != nil {
		return err
	}
	if info.Size() > 32<<20 {
		return fmt.Errorf("oversized coverage artifact: %s", name)
	}
	data, err := os.ReadFile(filepath.Join(directory, name))
	if err != nil {
		return err
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(target); err != nil {
		return err
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if chars, ok := token.(xml.CharData); !ok || strings.TrimSpace(string(chars)) != "" {
			return fmt.Errorf("trailing content in coverage artifact")
		}
	}
}

func readCoverage(directory string) (*CoverageReport, error) {
	var clover struct {
		XMLName xml.Name `xml:"coverage"`
		Metrics struct {
			Lines   *int `xml:"statements,attr"`
			Covered *int `xml:"coveredstatements,attr"`
		} `xml:"project>metrics"`
	}
	if err := coverageArtifact(directory, "coverage/clover.xml", &clover); err != nil {
		return nil, err
	}
	lines, covered := clover.Metrics.Lines, clover.Metrics.Covered
	if lines == nil || covered == nil || *lines <= 0 || *covered < 0 || *covered > *lines {
		return nil, fmt.Errorf("coverage contains no executable lines or invalid totals")
	}
	var index struct {
		XMLName xml.Name `xml:"phpunit"`
		Driver  struct {
			Name    string `xml:"name,attr"`
			Version string `xml:"version,attr"`
		} `xml:"build>driver"`
		Project struct {
			Source string `xml:"source,attr"`
		} `xml:"project"`
	}
	if err := coverageArtifact(directory, "coverage/xml/index.xml", &index); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(index.Project.Source+"/", "/workspace/") {
		return nil, fmt.Errorf("invalid coverage source root")
	}
	if index.Driver.Name != "pcov" || index.Driver.Version == "" {
		return nil, fmt.Errorf("coverage did not record a PCOV driver")
	}
	result := &CoverageReport{Driver: index.Driver.Name, DriverVersion: index.Driver.Version, ExecutableLines: *lines, CoveredLines: *covered, LinePercent: 100 * float64(*covered) / float64(*lines), Methods: []CoverageMethod{}}
	type method struct {
		Name     string   `xml:"name,attr"`
		Line     *int     `xml:"start,attr"`
		CRAP     *float64 `xml:"crap,attr"`
		Coverage *float64 `xml:"coverage,attr"`
	}
	type unit struct {
		Name      string   `xml:"name,attr"`
		Methods   []method `xml:"method"`
		Namespace struct {
			Name string `xml:"name,attr"`
		} `xml:"namespace"`
	}
	seen := map[string]bool{}
	var size int64
	files := 0
	var xmlLines, xmlCovered int
	err := filepath.WalkDir(filepath.Join(directory, "coverage/xml"), func(filename string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in coverage artifacts")
		}
		if !strings.HasSuffix(filename, ".xml") || entry.Name() == "index.xml" {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files++
		size += info.Size()
		if files > 10000 || size > 256<<20 {
			return fmt.Errorf("coverage artifact limits exceeded")
		}
		var report struct {
			XMLName xml.Name `xml:"phpunit"`
			File    struct {
				Name    string `xml:"name,attr"`
				Path    string `xml:"path,attr"`
				Classes []unit `xml:"class"`
				Traits  []unit `xml:"trait"`
				Totals  struct {
					Lines struct {
						Executable *int `xml:"executable,attr"`
						Executed   *int `xml:"executed,attr"`
					} `xml:"lines"`
				} `xml:"totals"`
			} `xml:"file"`
		}
		relative, _ := filepath.Rel(directory, filename)
		if err := coverageArtifact(directory, relative, &report); err != nil {
			return err
		}
		if report.File.Name == "" || report.File.Totals.Lines.Executable == nil || report.File.Totals.Lines.Executed == nil || *report.File.Totals.Lines.Executable < 0 || *report.File.Totals.Lines.Executed < 0 || *report.File.Totals.Lines.Executed > *report.File.Totals.Lines.Executable {
			return fmt.Errorf("invalid coverage file totals")
		}
		xmlLines += *report.File.Totals.Lines.Executable
		xmlCovered += *report.File.Totals.Lines.Executed
		file := strings.TrimPrefix(path.Join(index.Project.Source, report.File.Path, report.File.Name), "/workspace/")
		if err := projectpath.Validate(file); err != nil {
			return err
		}
		for _, class := range append(report.File.Classes, report.File.Traits...) {
			if namespace := class.Namespace.Name; namespace != "" && namespace != "global" && !strings.HasPrefix(class.Name, namespace+"\\") {
				class.Name = namespace + "\\" + class.Name
			}
			for _, item := range class.Methods {
				key := file + "\x00" + class.Name + "\x00" + item.Name
				if class.Name == "" || item.Name == "" || item.Line == nil || *item.Line < 1 || item.CRAP == nil || !finite(*item.CRAP) || *item.CRAP < 0 || item.Coverage == nil || !finite(*item.Coverage) || *item.Coverage < 0 || *item.Coverage > 100 || seen[key] {
					return fmt.Errorf("invalid or duplicate coverage method")
				}
				seen[key] = true
				result.Methods = append(result.Methods, CoverageMethod{Class: class.Name, Name: item.Name, File: file, Line: *item.Line, CRAP: *item.CRAP, LinePercent: *item.Coverage})
				result.MaxCRAP = math.Max(result.MaxCRAP, *item.CRAP)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if files == 0 {
		return nil, fmt.Errorf("coverage XML contains no files")
	}
	if xmlLines != *lines || xmlCovered != *covered {
		return nil, fmt.Errorf("Clover and coverage XML totals disagree")
	}
	if err := regularFile(directory, "coverage/html/index.html"); err != nil {
		return nil, err
	}
	var crap4j struct {
		XMLName xml.Name `xml:"crap_result"`
		Methods []struct {
			Class      string   `xml:"className"`
			Name       string   `xml:"methodName"`
			CRAP       *float64 `xml:"crap"`
			Coverage   *float64 `xml:"coverage"`
			Complexity *int     `xml:"complexity"`
		} `xml:"methods>method"`
	}
	if err := coverageArtifact(directory, "coverage/crap4j.xml", &crap4j); err != nil {
		return nil, err
	}
	byMethod := map[string]int{}
	for i, method := range result.Methods {
		byMethod[method.Class+"::"+method.Name] = i
	}
	matched := map[string]bool{}
	for _, method := range crap4j.Methods {
		key := method.Class + "::" + method.Name
		i, ok := byMethod[key]
		if !ok || matched[key] || method.CRAP == nil || method.Coverage == nil || method.Complexity == nil || *method.Complexity < 0 || !finite(*method.CRAP) || !finite(*method.Coverage) || math.Abs(*method.CRAP-result.Methods[i].CRAP) > 0.011 || math.Abs(*method.Coverage-result.Methods[i].LinePercent) > 0.011 {
			return nil, fmt.Errorf("Crap4J and coverage XML method records disagree: %s", key)
		}
		matched[key] = true
		result.Methods[i].Complexity = *method.Complexity
	}
	if len(matched) != len(result.Methods) {
		return nil, fmt.Errorf("incomplete Crap4J method records")
	}
	slices.SortFunc(result.Methods, func(a, b CoverageMethod) int {
		if a.CRAP != b.CRAP {
			if a.CRAP > b.CRAP {
				return -1
			}
			return 1
		}
		if order := strings.Compare(a.File, b.File); order != 0 {
			return order
		}
		if order := cmp.Compare(a.Line, b.Line); order != 0 {
			return order
		}
		return strings.Compare(a.Class+"::"+a.Name, b.Class+"::"+b.Name)
	})
	return result, nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func coverageThresholds(plan Plan, result *Report) {
	if result.Coverage == nil {
		return
	}
	if minimum := plan.Coverage.MinLineCoverage; minimum != nil && result.Coverage.LinePercent < *minimum {
		result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("Line coverage %.2f%% is below the configured minimum %.2f%%.", result.Coverage.LinePercent, *minimum))
		result.Status, result.ExitCode = "failed", 1
	}
	if maximum := plan.Coverage.MaxCRAP; maximum != nil {
		if !slices.ContainsFunc(result.Coverage.Methods, func(method CoverageMethod) bool { return method.Complexity > 0 }) {
			result.Complete, result.Status, result.ExitCode = false, "error", 3
			result.Diagnostics = append(result.Diagnostics, "CRAP threshold requires measurable class or trait methods.")
		} else if result.Coverage.MaxCRAP > *maximum {
			result.Diagnostics = append(result.Diagnostics, fmt.Sprintf("Maximum method CRAP %.2f exceeds the configured maximum %.2f.", result.Coverage.MaxCRAP, *maximum))
			result.Status, result.ExitCode = "failed", 1
		}
	}
}
