// Package grade runs hidden test files with a test runner and turns the results into a Harbor reward.
package grade

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Config is the parsed command line.
type Config struct {
	Runner  string
	App     string // the agent's workspace; tests run with this as the working directory
	Tests   string // where the hidden tests live
	Out     string // where reward.json, metrics.json and junit/ go
	Timeout time.Duration
	Mode    string   // "binary" or "partial"
	Files   []string // FILE or FILE=WEIGHT, relative to Tests; empty means discover
}

// graderDir is where the hidden tests are copied inside the app, so "../src/x.ts" imports resolve.
const graderDir = "__grader__"

// Bun's default test file naming (bun.com/docs/test/discovery).
var bunTestFile = regexp.MustCompile(`[._](test|spec)\.(js|jsx|ts|tsx)$`)

// bunArgs builds the bun command. The file must be absolute: a bare name is treated as a filter.
func bunArgs(file, xml string) []string {
	return []string{"test", file, "--reporter=junit", "--reporter-outfile=" + xml}
}

type testFile struct {
	name   string
	weight float64
}

// Run grades the app. A returned error means an authoring or infrastructure problem: Run then leaves
// no reward file behind, so Harbor flags the trial instead of recording a fake 0.
func Run(cfg Config) (err error) {
	// Remove stale outputs first, so even a validation error can't leave an old reward behind.
	for _, name := range []string{"reward.json", "reward.txt", "metrics.json", "junit"} {
		if err := os.RemoveAll(filepath.Join(cfg.Out, name)); err != nil {
			return err
		}
	}
	files, runner, err := validate(&cfg)
	if err != nil {
		return err
	}

	// Fail closed: from here on, a grader that dies leaves a 0.
	if err := os.MkdirAll(cfg.Out, 0o755); err != nil {
		return err
	}
	rewardPath := filepath.Join(cfg.Out, "reward.json")
	if err := writeJSON(rewardPath, map[string]float64{"reward": 0}); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(rewardPath)
		}
	}()

	placed := filepath.Join(cfg.App, graderDir)
	if err := os.RemoveAll(placed); err != nil {
		return err
	}
	defer os.RemoveAll(placed)
	if err := os.CopyFS(placed, os.DirFS(cfg.Tests)); err != nil {
		return fmt.Errorf("copying tests into the app: %w", err)
	}

	results := make(map[string]*fileResult, len(files))
	for _, f := range files {
		r := &fileResult{Weight: f.weight, Failing: []string{}}
		results[f.name] = r
		xml := filepath.Join(cfg.Out, "junit", f.name+".xml")
		if err := os.MkdirAll(filepath.Dir(xml), 0o755); err != nil {
			return err
		}
		start := time.Now()
		r.ExitCode, r.TimedOut, err = runFile(cfg.App, cfg.Timeout, runner.Path, bunArgs(filepath.Join(placed, f.name), xml)...)
		r.Seconds = math.Round(time.Since(start).Seconds()*100) / 100
		if err != nil {
			return fmt.Errorf("running %s: %w", f.name, err)
		}
		if r.TimedOut {
			continue
		}
		rep, perr := parseJUnit(xml)
		if perr != nil {
			// No report (import failure, crash, process.exit) or an unusable one: the file scores 0.
			r.LoadFailed = true
			if !errors.Is(perr, fs.ErrNotExist) {
				r.Error = perr.Error()
			}
			continue
		}
		r.apply(rep)
	}

	m := score(cfg.Mode, runner, results)
	if err := writeJSON(filepath.Join(cfg.Out, "metrics.json"), m); err != nil {
		return err
	}
	if err := writeJSON(rewardPath, rewardFile(m)); err != nil {
		return err
	}
	printSummary(m, files)
	return nil
}

// validate checks everything that would make a reward meaningless, before anything is written.
func validate(cfg *Config) ([]testFile, runnerInfo, error) {
	var runner runnerInfo
	if cfg.Mode != "binary" && cfg.Mode != "partial" {
		return nil, runner, fmt.Errorf("--mode must be binary or partial, got %q", cfg.Mode)
	}
	if cfg.Runner != "bun" {
		return nil, runner, fmt.Errorf("--runner must be bun, got %q", cfg.Runner)
	}
	if cfg.Timeout <= 0 {
		return nil, runner, fmt.Errorf("--timeout must be positive")
	}
	for _, p := range []*string{&cfg.App, &cfg.Tests, &cfg.Out} {
		abs, err := filepath.Abs(*p)
		if err != nil {
			return nil, runner, err
		}
		*p = abs
	}
	for _, p := range []*string{&cfg.App, &cfg.Tests} {
		real, err := filepath.EvalSymlinks(*p)
		if err != nil {
			return nil, runner, err
		}
		if fi, err := os.Stat(real); err != nil || !fi.IsDir() {
			return nil, runner, fmt.Errorf("%s is not a directory", *p)
		}
		*p = real
	}
	if rel, err := filepath.Rel(cfg.Tests, cfg.App); err == nil && filepath.IsLocal(rel) {
		return nil, runner, fmt.Errorf("--app %s must not be inside --tests %s", cfg.App, cfg.Tests)
	}

	files, err := parseFiles(cfg.Tests, cfg.Files)
	if err != nil {
		return nil, runner, err
	}

	path, err := exec.LookPath("bun")
	if err != nil {
		return nil, runner, fmt.Errorf("runner not found: %w", err)
	}
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		return nil, runner, fmt.Errorf("%s --version: %w", path, err)
	}
	sum, err := sha256File(path)
	if err != nil {
		return nil, runner, err
	}
	return files, runnerInfo{Name: "bun", Path: path, Version: strings.TrimSpace(string(out)), SHA256: sum}, nil
}

// parseFiles resolves FILE[=WEIGHT] arguments, or discovers test files when there are none.
func parseFiles(testsDir string, args []string) ([]testFile, error) {
	if len(args) == 0 {
		err := filepath.WalkDir(testsDir, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			if !d.IsDir() && bunTestFile.MatchString(d.Name()) {
				rel, _ := filepath.Rel(testsDir, p)
				args = append(args, rel)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		if len(args) == 0 {
			return nil, fmt.Errorf("no test files found in %s", testsDir)
		}
	}

	var files []testFile
	seen := map[string]bool{}
	weighted := 0
	sum := 0.0
	for _, arg := range args {
		name, w, hasWeight := arg, 0.0, false
		if i := strings.LastIndex(arg, "="); i >= 0 {
			var err error
			name, hasWeight = arg[:i], true
			w, err = strconv.ParseFloat(arg[i+1:], 64)
			if err != nil || math.IsNaN(w) || math.IsInf(w, 0) || w < 0 {
				return nil, fmt.Errorf("bad weight in %q", arg)
			}
		}
		name = filepath.Clean(name)
		if !filepath.IsLocal(name) {
			return nil, fmt.Errorf("test file %q must be a path inside --tests", name)
		}
		if seen[name] {
			return nil, fmt.Errorf("test file %q listed twice", name)
		}
		seen[name] = true
		if fi, err := os.Stat(filepath.Join(testsDir, name)); err != nil || !fi.Mode().IsRegular() {
			return nil, fmt.Errorf("test file %q not found in %s", name, testsDir)
		}
		if hasWeight {
			weighted++
			sum += w
		}
		files = append(files, testFile{name: name, weight: w})
	}
	switch {
	case weighted == 0:
		for i := range files {
			files[i].weight = 1 / float64(len(files))
		}
	case weighted != len(files):
		return nil, fmt.Errorf("give every test file a weight, or none")
	case math.Abs(sum-1) > 1e-6:
		return nil, fmt.Errorf("weights must sum to 1, got %g", sum)
	}
	return files, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
