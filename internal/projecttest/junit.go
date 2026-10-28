package projecttest

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
)

type Counts struct {
	Tests      int `json:"tests"`
	Assertions int `json:"assertions"`
	Failures   int `json:"failures"`
	Errors     int `json:"errors"`
	Skipped    int `json:"skipped"`
}

type Case struct {
	Name       string  `xml:"name,attr" json:"name"`
	Class      string  `xml:"classname,attr,omitempty" json:"class,omitempty"`
	File       string  `xml:"file,attr,omitempty" json:"file,omitempty"`
	Line       int     `xml:"line,attr,omitempty" json:"line,omitempty"`
	Assertions int     `xml:"assertions,attr,omitempty" json:"assertions"`
	Time       string  `xml:"time,attr,omitempty" json:"time,omitempty"`
	Failures   []Issue `xml:"failure,omitempty" json:"failures,omitempty"`
	Errors     []Issue `xml:"error,omitempty" json:"errors,omitempty"`
	Skipped    []Issue `xml:"skipped,omitempty" json:"skipped,omitempty"`
	Output     string  `xml:"system-out,omitempty" json:"output,omitempty"`
	Stderr     string  `xml:"system-err,omitempty" json:"stderr,omitempty"`
}

type Issue struct {
	Type    string `xml:"type,attr,omitempty" json:"type,omitempty"`
	Message string `xml:"message,attr,omitempty" json:"message,omitempty"`
	Detail  string `xml:",chardata" json:"detail,omitempty"`
}

type Suite struct {
	XMLName  xml.Name `xml:"testsuite"`
	Name     string   `xml:"name,attr"`
	Tests    *int     `xml:"tests,attr,omitempty"`
	Failures *int     `xml:"failures,attr,omitempty"`
	Errors   *int     `xml:"errors,attr,omitempty"`
	Skipped  *int     `xml:"skipped,attr,omitempty"`
	Suites   []Suite  `xml:"testsuite"`
	Cases    []Case   `xml:"testcase"`
}

func readJUnit(data []byte) ([]Suite, []Case, Counts, error) {
	if len(data) > 32<<20 {
		return nil, nil, Counts{}, fmt.Errorf("JUnit report exceeds 32 MiB")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var suites []Suite
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, nil, Counts{}, fmt.Errorf("invalid JUnit XML: %w", err)
		}
		if _, forbidden := token.(xml.Directive); forbidden {
			return nil, nil, Counts{}, fmt.Errorf("JUnit directives are not supported")
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "testsuites":
			var root struct {
				Suites []Suite `xml:"testsuite"`
			}
			if err := decoder.DecodeElement(&root, &start); err != nil {
				return nil, nil, Counts{}, err
			}
			suites = root.Suites
		case "testsuite":
			var suite Suite
			if err := decoder.DecodeElement(&suite, &start); err != nil {
				return nil, nil, Counts{}, err
			}
			suites = []Suite{suite}
		default:
			return nil, nil, Counts{}, fmt.Errorf("unexpected JUnit root %s", start.Name.Local)
		}
		break
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, Counts{}, err
		}
		if text, ok := token.(xml.CharData); ok && len(bytes.TrimSpace(text)) == 0 {
			continue
		}
		if _, ok := token.(xml.Comment); ok {
			continue
		}
		return nil, nil, Counts{}, fmt.Errorf("unexpected data after JUnit report")
	}
	var cases []Case
	var counts Counts
	for _, suite := range suites {
		rows, actual, err := flatten(suite)
		if err != nil {
			return nil, nil, Counts{}, err
		}
		cases = append(cases, rows...)
		counts.add(actual)
	}
	return suites, cases, counts, nil
}

func flatten(suite Suite) ([]Case, Counts, error) {
	rows := append([]Case{}, suite.Cases...)
	for _, child := range suite.Suites {
		nested, _, err := flatten(child)
		if err != nil {
			return nil, Counts{}, err
		}
		rows = append(rows, nested...)
	}
	counts := Counts{Tests: len(rows)}
	for _, row := range rows {
		if row.Name == "" || row.Assertions < 0 {
			return nil, counts, fmt.Errorf("invalid JUnit testcase")
		}
		counts.Assertions += row.Assertions
		counts.Failures += len(row.Failures)
		counts.Errors += len(row.Errors)
		counts.Skipped += len(row.Skipped)
		if row.Time != "" {
			if value, err := strconv.ParseFloat(row.Time, 64); err != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, counts, fmt.Errorf("invalid JUnit duration")
			}
		}
	}
	for _, count := range []struct {
		expected *int
		actual   int
	}{{suite.Tests, counts.Tests}, {suite.Failures, counts.Failures}, {suite.Errors, counts.Errors}, {suite.Skipped, counts.Skipped}} {
		if count.expected != nil && *count.expected != count.actual {
			return nil, counts, fmt.Errorf("JUnit totals do not match testcase records")
		}
	}
	return rows, counts, nil
}

func (counts *Counts) add(other Counts) {
	counts.Tests += other.Tests
	counts.Assertions += other.Assertions
	counts.Failures += other.Failures
	counts.Errors += other.Errors
	counts.Skipped += other.Skipped
}
