# 04 — Core plan

The *why* is in [01-context.md](01-context.md) and [03-decisions.md](03-decisions.md). This file is the *how*.

## Usage (target)

```bash
#!/bin/bash
# tests/test.sh: Harbor runs this in the task's working directory after the agent finishes
/tests/grade-$(uname -m) --runner bun div.test.ts mul.test.ts
```

```bash
/tests/grade-$(uname -m) --runner bun div.test.ts=0.4 mul.test.ts=0.6 --mode partial
/tests/grade-$(uname -m) --cmd 'mytool --junit {xml} {file}' a.test=1   # any JUnit-capable runner
```

The binaries are named by `uname -m` output (`grade-x86_64`, `grade-aarch64`) so `test.sh` needs no
`case` statement.

## CLI

```
grade (--runner NAME | --cmd TEMPLATE) [flags] FILE[=WEIGHT]...
```

| Flag | Default | Meaning |
|---|---|---|
| `--runner` | none | Built-in adapter: `bun` (M1); `pytest`, `vitest` later |
| `--cmd` | none | Command template with `{file}` (absolute path of the copied test) and `{xml}` (report path) |
| `--app` | cwd | The agent's workspace |
| `--tests` | the binary's own directory | Where the hidden tests live (normally `/tests`) |
| `--out` | `/logs/verifier` | Where `reward.json`, `metrics.json` and `junit/` go |
| `--timeout` | `60s` | Per test file |
| `--mode` | `binary` | `binary` = 1 only if everything passes; `partial` = weighted fraction |

`FILE` is relative to `--tests`. With no weights, files are weighted equally. With weights, every file needs
one and they must sum to 1 (±1e-6). With no files at all, every test file in `--tests` matching the runner's
naming pattern is used.

## Run sequence

1. **Validate** flags, files, weights and that the runner binary exists. On any error: print why, exit
   non-zero, write **no** reward (D7: authoring/infrastructure error, not a bad submission).
2. **Fail closed**: delete stale `reward.json`/`reward.txt`/`junit/`, then write `{"reward": 0}`. If the
   grader dies later, the trial still reads 0.
3. **Place tests**: copy `--tests` into `<app>/__grader__/`, so hidden tests import app code as
   `../src/x.ts`. (Hardening may change where things run; see below.)
4. **Run each file** with the adapter command in its own process group, with a clean environment and the
   timeout. On timeout, kill the whole group.
5. **Parse** `<out>/junit/<file>.xml` if it exists: root totals plus the names of failing test cases.
   A missing XML means load failure or timeout, so that file scores 0.
6. **Score**, write `metrics.json`, then overwrite `reward.json`. Print a short summary to stderr. Exit 0.
7. **Clean up** `<app>/__grader__/`.

## Scoring rules

- Per file: `passed = tests − failures − errors − skipped`, clamped at 0. Skipped and todo tests are not passes.
- `fraction = passed / tests`, or 0 if `tests == 0`. A file with zero tests never counts as passing.
- A file **fully passes** when `tests > 0` and `passed == tests`.
- `binary`: reward `1` only if every file fully passes, else `0`.
- `partial`: reward = Σ weight × fraction, rounded to 4 decimals.

## Outputs

`reward.json` (the only file Harbor reads; flat numbers only):

```json
{"reward": 0}
```

In `--mode partial`: `{"reward": 0.3, "div.test.ts": 0, "mul.test.ts": 0.5}`.

`metrics.json` (diagnostics, ignored by Harbor, free-form):

```json
{
  "mode": "binary",
  "reward": 0,
  "weighted_fraction": 0.3,
  "runner": {"name": "bun", "path": "/usr/local/bin/bun", "version": "1.3.11", "sha256": "..."},
  "files": {
    "div.test.ts": {"weight": 0.4, "tests": 0, "passed": 0, "failed": 0, "skipped": 0,
                    "load_failed": true, "timed_out": false, "seconds": 0.21, "failing": []},
    "mul.test.ts": {"weight": 0.6, "tests": 2, "passed": 1, "failed": 1, "skipped": 0,
                    "load_failed": false, "timed_out": false, "seconds": 0.34, "failing": ["3 * 3"]}
  }
}
```

`junit/<file>.xml`: each raw report, kept for humans.

## Runner adapters

| Runner | Command | Known quirks | Milestone |
|---|---|---|---|
| bun | `bun test {file} --reporter=junit --reporter-outfile={xml}` | No XML on import failure; read the root `<testsuites>` only (nested suites repeat counts); `skipped` includes todo; no `errors` attribute; `{file}` must be absolute | M1 |
| pytest | `python -m pytest {file} --junitxml={xml} -p no:cacheprovider` | Collection errors appear as `errors`, not missing XML (verify); conftest/ini hardening | M4 |
| vitest | `vitest run {file} --reporter=junit --outputFile={xml}` with a grader-owned config | Agent-controlled config; `node_modules` may be agent-modified | M4 |
| generic | `--cmd` template | Whatever the runner does; must write JUnit to `{xml}` | M4 |
| jest, go (gotestsum), rust (cargo-nextest), deno, node:test | n/a | Go runs packages, not files; Rust hidden tests must be integration tests | later |

Re-verify every quirk against the runner version in the task image before relying on it.

## Hardening plan (M2)

Assume the agent tries to raise its score. Each row needs a malicious fixture proving the mitigation works,
or an honest "not mitigated" line in the README's threat model.

| # | Threat | Planned mitigation (verify each empirically) |
|---|---|---|
| T1 | Agent-planted runner config in the workspace (`bunfig.toml` `[test] preload` patching `expect`; `vitest.config.*`; `conftest.py`, `pytest.ini`, `pyproject.toml`) | Force a grader-owned config: Bun's config flag (check `bun --help` for a config-path option), Vitest `--config`, pytest `--noconftest` / `-c` / `--rootdir`. Or run from a directory the agent doesn't control (`cd /tests` works for Bun because bare imports resolve from the test file's directory) |
| T2 | Agent-set environment (`NODE_OPTIONS=--require`, `BUN_OPTIONS`, `PYTHONPATH`, ...) | Run the runner with a minimal allowlisted environment |
| T3 | `package.json` scripts | Never run `npm test` / `bun run test`; call the runner binary directly |
| T4 | Hangs, fork bombs | Per-file timeout and process-group kill; document `ulimit` options |
| T5 | Forged or stale reports | Delete old XML first; write each report to a fresh path. In-process rewriting at exit can't be fully prevented; document it |
| T6 | Patched dependencies (`node_modules`, `site-packages`) or a replaced runner binary (the agent often has root) | Record the runner's path, version and sha256 in `metrics.json`; optional pin flag; support verifier-owned dependency dirs (as terminal-bench's nextjs-performance does) |
| T7 | Agent code touching the reward channel | Optionally run tests as an unprivileged user (as terminal-bench's ks-solver-cpp does) while the grader writes the reward as root |
| T8 | Everything above in a shared container | Recommend Harbor's separate verifier mode (`[verifier] environment_mode = "separate"`) in the README as the strongest option |

## Verification matrix

For each supported runner, inside its real Docker image (`oven/bun:1.3.x` for M1), with a small example
Harbor task in `examples/`:

| Case | Expected |
|---|---|
| Null run (untouched repo) | reward 0 |
| Oracle run (`solve.sh` applied) | reward 1 |
| Partial agent | binary: 0; partial: the exact expected fraction; `metrics.json` counts correct |
| Hanging test | that file 0 within the timeout; reward still written |
| Import/load failure | that file 0, `load_failed: true` |
| Test file with zero tests | that file 0 |
| Bad weights / missing file / missing runner | non-zero exit, no reward file |
| Malicious fixtures (T1–T3, T5, T6) | reward never wrongly becomes 1; unmitigated cases documented |
| Real Harbor: `harbor run -p examples/<task> -a oracle` (and the nop agent) | 1.0 (and 0) |

Run the full suite twice from a clean checkout; both runs must pass.

## Repo layout (target)

```
cmd/grade/main.go        # flag parsing and exit codes only
internal/grade/          # run.go (exec, timeout, process group), junit.go, score.go, runners.go
examples/bun-basic/      # a Harbor task: environment/, solution/, tests/
testdata/malicious/      # one fixture per threat
docs/                    # these files
```

Standard library only. Ask before adding any dependency.

## Milestones

| | Scope | Done when |
|---|---|---|
| M0 | Research, decisions, plan (this repo) | Done |
| M1 | Bun MVP: CLI, bun adapter, binary + partial modes, `metrics.json`, fail-loud validation, timeout, example task | Verification matrix passes in `oven/bun`, plus a real Harbor oracle/nop run |
| M2 | Bun hardening T1–T8 and a README threat model | Every malicious fixture behaves as documented |
| M3 | Release: binaries for linux amd64/arm64 (+ darwin for local dev), copy-paste `test.sh` snippets | Ask the owner before creating a release |
| M4 | pytest, vitest and generic `--cmd` (only when a task needs them) | Matrix passes in each runner's image |
| Later | jest, go, nextest, deno, node:test; CTRF input; Rewardkit criterion | As needed |

## Open questions

- Should the unprivileged-user mode (T7) be on by default, or opt-in?
- Is pinning the runner sha256 (T6) worth a flag, or is recording it in `metrics.json` enough?
- Decided: name `reward-runner`, license Apache-2.0.
