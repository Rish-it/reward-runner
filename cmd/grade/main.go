// Command grade runs hidden test files and writes a Harbor verifier reward.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Rish-it/reward-runner/internal/grade"
)

func main() {
	cfg, err := parseArgs(os.Args[1:])
	if errors.Is(err, flag.ErrHelp) {
		os.Exit(0)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "grade:", err)
		os.Exit(2)
	}
	if err := grade.Run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, "grade: error (no reward written):", err)
		os.Exit(1)
	}
}

func parseArgs(args []string) (grade.Config, error) {
	var cfg grade.Config
	fs := flag.NewFlagSet("grade", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), "usage: grade --runner bun [flags] [FILE[=WEIGHT]...]")
		fs.PrintDefaults()
	}
	fs.StringVar(&cfg.Runner, "runner", "", "test runner adapter: bun")
	fs.StringVar(&cfg.App, "app", "", "the agent's workspace (default: current directory)")
	fs.StringVar(&cfg.Tests, "tests", "", "directory holding the hidden tests (default: this binary's directory)")
	fs.StringVar(&cfg.Out, "out", "/logs/verifier", "where reward.json, metrics.json and junit/ go")
	fs.DurationVar(&cfg.Timeout, "timeout", 60*time.Second, "timeout per test file")
	fs.StringVar(&cfg.Mode, "mode", "binary", "binary (1 only if every test passes) or partial (weighted fraction)")

	// The flag package stops at the first positional argument; keep going so flags may follow files.
	for {
		if err := fs.Parse(args); err != nil {
			return cfg, err
		}
		if fs.NArg() == 0 {
			break
		}
		cfg.Files = append(cfg.Files, fs.Arg(0))
		args = fs.Args()[1:]
	}

	if cfg.App == "" {
		wd, err := os.Getwd()
		if err != nil {
			return cfg, err
		}
		cfg.App = wd
	}
	if cfg.Tests == "" {
		exe, err := os.Executable()
		if err != nil {
			return cfg, err
		}
		cfg.Tests = filepath.Dir(exe)
	}
	return cfg, nil
}
