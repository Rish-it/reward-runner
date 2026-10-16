package grade

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestParseJUnitBunNested(t *testing.T) {
	// Captured from `bun test` 1.3.14 in oven/bun:1.3.14: nested describes, one failure, a skip and a todo.
	r, err := parseJUnit("testdata/bun-1.3.14-nested.xml")
	if err != nil {
		t.Fatal(err)
	}
	if r.tests != 6 || r.failures != 1 || r.errors != 0 || r.skipped != 2 {
		t.Errorf("totals = %+v, want tests=6 failures=1 skipped=2 (root only, nested suites not summed)", r)
	}
	if want := []string{"outer > inner > mul 3*3"}; !reflect.DeepEqual(r.failing, want) {
		t.Errorf("failing = %q, want %q", r.failing, want)
	}
}

func TestParseJUnitRejects(t *testing.T) {
	dir := t.TempDir()
	if _, err := parseJUnit(filepath.Join(dir, "missing.xml")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("missing file: err = %v, want ErrNotExist", err)
	}
	for name, body := range map[string]string{
		"truncated":   `<testsuites tests="2" failures="0">`,
		"wrong root":  `<html tests="1"></html>`,
		"negative":    `<testsuites tests="2" failures="-1"></testsuites>`,
		"too many":    `<testsuites tests="2" failures="1" skipped="2"></testsuites>`,
		"not numbers": `<testsuites tests="two"></testsuites>`,
	} {
		p := filepath.Join(dir, "r.xml")
		os.WriteFile(p, []byte(body), 0o644)
		if _, err := parseJUnit(p); err == nil {
			t.Errorf("%s: parsed without error", name)
		}
	}
}

func TestScore(t *testing.T) {
	pass := func(w float64, n int) *fileResult { return &fileResult{Weight: w, Tests: n, Passed: n} }
	part := func(w float64, passed, n int) *fileResult { return &fileResult{Weight: w, Tests: n, Passed: passed} }
	cases := []struct {
		name   string
		mode   string
		files  map[string]*fileResult
		reward float64
	}{
		{"all pass", "binary", map[string]*fileResult{"a": pass(0.5, 2), "b": pass(0.5, 1)}, 1},
		{"one failure", "binary", map[string]*fileResult{"a": pass(0.5, 2), "b": part(0.5, 0, 1)}, 0},
		{"zero tests never passes", "binary", map[string]*fileResult{"a": pass(1, 0)}, 0},
		{"no report", "binary", map[string]*fileResult{"a": {Weight: 1, LoadFailed: true}}, 0},
		{"partial fraction", "partial", map[string]*fileResult{"a": part(0.4, 0, 0), "b": part(0.6, 1, 2)}, 0.3},
		{"partial all pass", "partial", map[string]*fileResult{"a": pass(0.4, 3), "b": pass(0.6, 2)}, 1},
		{"partial never rounds up to a pass", "partial", map[string]*fileResult{"a": part(1, 19999, 20000)}, 0.9999},
	}
	for _, c := range cases {
		if got := score(c.mode, runnerInfo{}, c.files).Reward; got != c.reward {
			t.Errorf("%s: reward = %v, want %v", c.name, got, c.reward)
		}
	}
	m := score("partial", runnerInfo{}, map[string]*fileResult{"a.test.ts": part(0.4, 0, 0), "b.test.ts": part(0.6, 1, 2)})
	if got, want := rewardFile(m), map[string]float64{"reward": 0.3, "a.test.ts": 0, "b.test.ts": 0.5}; !reflect.DeepEqual(got, want) {
		t.Errorf("partial reward.json = %v, want %v", got, want)
	}
	m.Mode = "binary"
	if got := rewardFile(m); len(got) != 1 {
		t.Errorf("binary reward.json = %v, want only the reward key", got)
	}
}

func TestParseFiles(t *testing.T) {
	dir := t.TempDir()
	for _, f := range []string{"a.test.ts", "b_spec.tsx", "helper.ts", "sub/c.test.js", "node_modules/x/y.test.ts"} {
		os.MkdirAll(filepath.Dir(filepath.Join(dir, f)), 0o755)
		os.WriteFile(filepath.Join(dir, f), nil, 0o644)
	}
	files, err := parseFiles(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []testFile{{"a.test.ts", 1.0 / 3}, {"b_spec.tsx", 1.0 / 3}, {"sub/c.test.js", 1.0 / 3}}; !reflect.DeepEqual(files, want) {
		t.Errorf("discovered %v, want %v", files, want)
	}
	files, err = parseFiles(dir, []string{"a.test.ts=0.4", "sub/c.test.js=0.6"})
	if err != nil || files[0].weight != 0.4 || files[1].weight != 0.6 {
		t.Errorf("weighted: %v, %v", files, err)
	}
	for _, bad := range [][]string{
		{"a.test.ts=0.5", "b_spec.tsx=0.6"}, // sum != 1
		{"a.test.ts=1", "b_spec.tsx"},       // some weighted, some not
		{"a.test.ts=-1", "b_spec.tsx=2"},    // negative
		{"a.test.ts=NaN"},                   // not finite
		{"a.test.ts=x"},                     // not a number
		{"missing.test.ts"},                 // not found
		{"sub"},                             // a directory
		{"a.test.ts", "./a.test.ts"},        // listed twice
		{"../etc/passwd"},                   // escapes --tests
		{"/etc/passwd"},                     // absolute
	} {
		if _, err := parseFiles(dir, bad); err == nil {
			t.Errorf("%q: accepted", bad)
		}
	}
	if _, err := parseFiles(t.TempDir(), nil); err == nil {
		t.Error("empty tests dir: accepted")
	}
}

func TestRunFileTimeoutKillsProcessGroup(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	start := time.Now()
	_, timedOut, err := runFile(t.TempDir(), 500*time.Millisecond, "sh", "-c", "sleep 30 & echo $! > "+pidFile+"; wait")
	if err != nil || !timedOut {
		t.Fatalf("timedOut=%v err=%v, want a timeout", timedOut, err)
	}
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %v to time out", d)
	}
	data, _ := os.ReadFile(pidFile)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; syscall.Kill(pid, 0) == nil; i++ {
		if i == 50 {
			t.Fatalf("background child %d survived the timeout", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}

	code, timedOut, err := runFile(t.TempDir(), 5*time.Second, "sh", "-c", "exit 3")
	if code != 3 || timedOut || err != nil {
		t.Errorf("exit 3: code=%d timedOut=%v err=%v", code, timedOut, err)
	}
	if _, _, err := runFile(t.TempDir(), time.Second, "/nonexistent/runner"); err == nil {
		t.Error("missing binary: no error")
	}
}
