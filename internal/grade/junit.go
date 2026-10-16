package grade

import (
	"encoding/xml"
	"fmt"
	"os"
)

// report is what one JUnit file says: the root totals and the failing test names.
type report struct {
	tests, failures, errors, skipped int
	failing                          []string
}

type suite struct {
	XMLName  xml.Name
	Name     string  `xml:"name,attr"`
	Tests    int     `xml:"tests,attr"`
	Failures int     `xml:"failures,attr"`
	Errors   int     `xml:"errors,attr"`
	Skipped  int     `xml:"skipped,attr"`
	Suites   []suite `xml:"testsuite"`
	Cases    []struct {
		Name    string    `xml:"name,attr"`
		Failure *struct{} `xml:"failure"`
		Error   *struct{} `xml:"error"`
	} `xml:"testcase"`
}

// parseJUnit reads totals from the root element only: Bun nests a <testsuite> per file and per describe,
// each repeating a subset of the counts, so summing nested suites over-counts.
func parseJUnit(path string) (report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return report{}, err
	}
	var root suite
	if err := xml.Unmarshal(data, &root); err != nil {
		return report{}, fmt.Errorf("bad JUnit XML: %w", err)
	}
	if root.XMLName.Local != "testsuites" && root.XMLName.Local != "testsuite" {
		return report{}, fmt.Errorf("bad JUnit XML: root element <%s>", root.XMLName.Local)
	}
	r := report{tests: root.Tests, failures: root.Failures, errors: root.Errors, skipped: root.Skipped}
	if r.tests < 0 || r.failures < 0 || r.errors < 0 || r.skipped < 0 || r.failures+r.errors+r.skipped > r.tests {
		return report{}, fmt.Errorf("bad JUnit XML: inconsistent totals tests=%d failures=%d errors=%d skipped=%d",
			r.tests, r.failures, r.errors, r.skipped)
	}

	// Name failing cases by their describe path. Depth 0 is the root, depth 1 the per-file suite.
	var walk func(s suite, prefix string, depth int)
	walk = func(s suite, prefix string, depth int) {
		if depth >= 2 {
			prefix += s.Name + " > "
		}
		for _, c := range s.Cases {
			if c.Failure != nil || c.Error != nil {
				r.failing = append(r.failing, prefix+c.Name)
			}
		}
		for _, sub := range s.Suites {
			walk(sub, prefix, depth+1)
		}
	}
	walk(root, "", 0)
	return r, nil
}
