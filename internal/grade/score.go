package grade

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
)

type runnerInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type fileResult struct {
	Weight     float64  `json:"weight"`
	Tests      int      `json:"tests"`
	Passed     int      `json:"passed"`
	Failed     int      `json:"failed"`
	Skipped    int      `json:"skipped"`
	LoadFailed bool     `json:"load_failed"`
	TimedOut   bool     `json:"timed_out"`
	ExitCode   int      `json:"exit_code"`
	Seconds    float64  `json:"seconds"`
	Failing    []string `json:"failing"`
	Error      string   `json:"error,omitempty"`
}

type metrics struct {
	Mode             string                 `json:"mode"`
	Reward           float64                `json:"reward"`
	WeightedFraction float64                `json:"weighted_fraction"`
	Runner           runnerInfo             `json:"runner"`
	Files            map[string]*fileResult `json:"files"`
}

// apply fills the counts from a report. Skipped and todo tests are not passes.
func (f *fileResult) apply(r report) {
	f.Tests, f.Failed, f.Skipped = r.tests, r.failures+r.errors, r.skipped
	f.Passed = max(f.Tests-f.Failed-f.Skipped, 0)
	if r.failing != nil {
		f.Failing = r.failing
	}
}

// fraction is passed/tests; a file with zero tests (or no report) scores 0.
func (f *fileResult) fraction() float64 {
	if f.Tests == 0 {
		return 0
	}
	return float64(f.Passed) / float64(f.Tests)
}

func (f *fileResult) fullyPassed() bool { return f.Tests > 0 && f.Passed == f.Tests }

func round4(x float64) float64 { return math.Round(x*1e4) / 1e4 }

func score(mode string, runner runnerInfo, files map[string]*fileResult) metrics {
	all := true
	weighted := 0.0
	for _, f := range files {
		weighted += f.Weight * f.fraction()
		all = all && f.fullyPassed()
	}
	m := metrics{Mode: mode, WeightedFraction: round4(weighted), Runner: runner, Files: files}
	switch {
	case all:
		m.Reward = 1
	case mode == "partial":
		// Harbor passes a trial at reward >= 1, so rounding must never turn a failure into a pass.
		m.Reward = min(m.WeightedFraction, 0.9999)
	}
	return m
}

// rewardFile is the flat object Harbor reads. Partial mode adds each file's fraction.
func rewardFile(m metrics) map[string]float64 {
	r := map[string]float64{"reward": m.Reward}
	if m.Mode == "partial" {
		for name, f := range m.Files {
			r[name] = round4(f.fraction())
		}
	}
	return r
}

// writeJSON writes via a temp file and rename, so a reader never sees half a file.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func printSummary(m metrics, files []testFile) {
	for _, tf := range files {
		f := m.Files[tf.name]
		status := fmt.Sprintf("%d/%d passed", f.Passed, f.Tests)
		switch {
		case f.TimedOut:
			status = "timed out"
		case f.LoadFailed:
			status = "no report (failed to load, crashed, exited early, or has no tests)"
		}
		if len(f.Failing) > 0 {
			status += ", failing: " + strings.Join(f.Failing, "; ")
		}
		fmt.Fprintf(os.Stderr, "grade: %s: %s\n", tf.name, status)
	}
	fmt.Fprintf(os.Stderr, "grade: reward %g (%s mode, weighted fraction %g)\n", m.Reward, m.Mode, m.WeightedFraction)
}
