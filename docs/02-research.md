# 02 — Research: what exists, and why the gap is real

Research done 2026-10-16. Question: *does an open-source tool already turn test-runner results
(especially `bun test`) into pass/fail counts and a Harbor reward?*

**Answer: no public tool does this.** The pieces exist separately; everyone writes their own glue.
The gap exists partly because official benchmarks deliberately score `0`/`1`, which shrinks what such a
tool needs to do. See [03-decisions.md](03-decisions.md) for what that means for the design.

## Method

- Pass 1 (Claude, via the agent-reach toolset): Exa web search, GitHub (repos, code, issues via `gh`),
  npm, PyPI, Reddit (OpenCLI), Harbor source and docs, plus local probes of `bun test` (Bun 1.3.14).
- Pass 2 (Grok): independent verification of each claim. Its findings were then spot-checked against
  primary sources; two of its citations were wrong (see [Corrections](#corrections-log)).
- Not covered: Twitter/X (no credentials), Harbor's Discord (login required), and exhaustive GitHub code
  search (rate-limited at about 10 requests a minute).

## Existing tools

| Tool | What it does | Why it doesn't fill the gap |
|---|---|---|
| Bun `--reporter=junit` ([docs](https://bun.com/docs/test/reporters)) | JUnit XML per run (since 1.1.35) | Only the raw report. No JSON reporter ([#6250](https://github.com/oven-sh/bun/issues/6250)), no custom reporters ([#2984](https://github.com/oven-sh/bun/issues/2984)) |
| [Harbor Rewardkit](https://github.com/harbor-framework/harbor/tree/main/packages/rewardkit) (official, Python) | Weighted criteria (file, command, JSON, HTTP, image, trajectory checks, LLM judges) that write `reward.json` | `command_succeeds` is exit-code only; no per-test counts; no test-runner integration in any release; needs Python in the image |
| [`graded`](https://pypi.org/project/graded/) (PyPI, v1.0.5, June 2026) | "Defensive verifier framework" for Harbor: weighted criteria with fractional scores, `reward.json` + `reward.txt`, LLM judge tracing | Python; you still write the "run tests and count" function; heavy deps (google-genai, instructor, pydantic); no public repo or license listed |
| [test-results-parser](https://github.com/test-results-reporter/parser), [junit2json](https://github.com/Kesin11/ts-junit2json), [junit-to-ctrf](https://github.com/ctrf-io/junit-to-ctrf), [ctrf-cli](https://github.com/ctrf-io/ctrf-cli), [zestia/junit-results-toolkit](https://github.com/zestia/junit-results-toolkit) | Parse or convert JUnit XML into totals / JSON / CTRF | Generic reporting: no reward file, no weights, no hidden-test placement, no timeouts, no hardening |
| pytest-json-ctrf (used by Harbor's default pytest template) | CTRF JSON diagnostics from pytest | Diagnostics only; the template still derives the reward from the exit code |
| [SWE-bench log parsers](https://github.com/SWE-bench/SWE-bench/blob/main/swebench/harness/log_parsers/javascript.py) | Per-test status from jest, vitest, karma, TAP logs | Harness-specific; no Bun parser |
| [neoromantic/evals](https://github.com/neoromantic/evals) | Runs LLM-feature evals as Bun tests with scorers | Different problem (scoring LLM outputs), not grading agent code changes |

## How real Harbor tasks do it today

Every sampled task hand-writes its verifier. None uses a shared helper.

| Task | Tests run with | Reward |
|---|---|---|
| [askableorg/terminal-task-template · hello-world-ts](https://github.com/askableorg/terminal-task-template/blob/HEAD/tasks/hello-world-ts/tests/test.sh) | `bun test`, run after `cd /tests` so an agent-planted `bunfig.toml` preload can't load | `0`/`1` from exit code |
| [bear-ai-dev/specific-sample-envs-xai · task 03](https://github.com/bear-ai-dev/specific-sample-envs-xai/blob/HEAD/tasks/03-restaurant-arena-engine-correction-scope-probe/tests/test.sh) | `bun test --reporter=junit`, grep of the root `<testsuites>` line, workspace copied to a root-owned dir | `0`/`1` (all tests pass and none error) |
| [root-daemon/lavalamp-rewritten · trial-score-aggregation](https://github.com/root-daemon/lavalamp-rewritten/blob/HEAD/benchmarks/lavalamp-quality/trial-score-aggregation/tests/test.sh) | `bun test /tests/verify.ts` | `0`/`1` from exit code |
| [Togetherbench/SWE-Together · agent-swarm-task](https://github.com/Togetherbench/SWE-Together/blob/HEAD/tasks/agent-swarm-task-ea4bd8/tests/test.sh) | Custom Python structural checks (no test runner) | Weighted fraction |
| [findalexli/agentsmd-rl · bun-glob-scan-double-visit](https://github.com/findalexli/agentsmd-rl/blob/HEAD/markdown_following/bun-glob-scan-double-visit/tests/test.sh) | pytest wrapper with `--ctrf` | `0`/`1` from exit code |
| [terminal-bench · nextjs-performance](https://github.com/harbor-framework/terminal-bench/blob/HEAD/tasks/nextjs-performance/tests/test.sh) | Playwright from a verifier-owned deps dir, JSON report converted to CTRF | `0`/`1`; CTRF is diagnostics only |
| [terminal-bench · react-lead-form](https://github.com/harbor-framework/terminal-bench/blob/HEAD/tasks/react-lead-form/tests/test.sh) | Vitest with a verifier-pinned config copied in by the verifier | `0` written first (fail-closed), then `1` on success |
| [terminal-bench · ks-solver-cpp](https://github.com/harbor-framework/terminal-bench/blob/HEAD/tasks/ks-solver-cpp/tests/test.sh) | Python checker; the agent's binary runs as `nobody` so it can't touch the reward channel | `0`/`1` |
| [terminal-bench · cargo-flight-dispatch](https://github.com/harbor-framework/terminal-bench/blob/HEAD/tasks/cargo-flight-dispatch/tests/test.sh), [bun-sourcemap-leak](https://github.com/harbor-framework/terminal-bench/blob/HEAD/tasks/bun-sourcemap-leak/tests/test.sh) | pytest with `--ctrf` (even for Rust/Bun tasks) | `0`/`1` from exit code |

Patterns worth reusing (they feed the hardening plan): run from a directory the agent doesn't control,
use verifier-owned dependencies and configs, run agent code as an unprivileged user, write a fail-closed `0`
first, keep rich diagnostics in a separate file.

## Official stance on partial credit

- **Terminal-Bench-Science** requires a literal `0` or `1`. Its automated
  [`check-binary-reward`](https://github.com/harbor-framework/terminal-bench-science/blob/HEAD/TASK_REVIEW_AUTOMATION.md#check-binary-reward)
  fails any task that writes another value to `reward.txt` or `reward.json`, and it is re-checked at runtime.
  The rationale: Harbor passes a trial only at `reward >= 1.0`, so partial structure is discarded by every
  consumer (the `/run` pass rate, the leaderboard, `harbor analyze`). Their advice is to score however you like
  internally, collapse to `0`/`1` at the write site, and put the breakdown in another file
  ([CONTRIBUTING](https://github.com/harbor-framework/terminal-bench-science/blob/HEAD/CONTRIBUTING.md)).
- **Terminal-Bench** (main): all sampled tasks are binary; its CONTRIBUTING requires the oracle to reach 1.0.
  An explicit binary rule like TB-Science's was not found there.
- [terminal-bench-science#345](https://github.com/harbor-framework/terminal-bench-science/issues/345) asked for
  partial credit; it was closed in a tracker reset, not adopted.
- Harbor itself accepts float rewards, and some adapters use them (e.g. scicode sub-step fractions).

## Demand signals (real but small)

- terminal-bench-science#345 (partial credit request, above).
- oven-sh/bun [#6250](https://github.com/oven-sh/bun/issues/6250) (JSON test output; a 2026 comment from someone
  building AI-agent hooks who regex-parses stderr), [#2984](https://github.com/oven-sh/bun/issues/2984)
  (custom reporters), [#1809](https://github.com/oven-sh/bun/issues/1809) (closed when JUnit shipped).
- Reported but not verified: a Long-Horizon Terminal-Bench write-up and an X post on partial-credit experiments.

## Audience size (2026-10-16)

| Signal | Number |
|---|---|
| harbor-framework/harbor | 6,146 stars, 2,007 forks |
| harbor-framework/terminal-bench | 909 stars, 585 forks |
| harbor-framework/terminal-bench-science | 719 stars, 475 forks |
| Harbor Hub datasets (per Grok) | tens to ~120 tasks each; low thousands of public tasks overall |

Interpretation: thousands of task authors, not millions. Niche, but growing.

## Verified technical facts

First probed locally with Bun 1.3.14, then **re-verified 2026-10-16 inside the `oven/bun:1.3.14` Docker image**
(Debian 13, `bun --version` = 1.3.14, both linux/arm64 and linux/amd64). Each fact below held there:

- `bun test /abs/path/x.test.ts --reporter=junit --reporter-outfile=out.xml` writes a root
  `<testsuites name="bun test" tests=".." failures=".." skipped="..">`. Nested `<testsuite>` elements (one per
  file, one per `describe`) repeat subsets of the same counts. Read the root only.
- `skipped` counts both `test.skip` (`<skipped />`) and `test.todo` (`<skipped message="TODO" />`). No `errors`
  attribute is emitted.
- A test file that fails to import (missing export) produces **no XML file** and exit code 1. So does a file
  that throws at the top level after registering tests.
- A failing `<testcase>` contains `<failure type="AssertionError" />`. That's enough to list failing test names.
  The `classname` attribute is not usable: it is double-escaped and reversed (`inner &amp;gt; outer` for
  `describe("outer") > describe("inner")`), so failing names are built from the nested suite names instead.
- A bare file name is treated as a filter by `bun test`; pass an absolute or `./`-prefixed path. An explicit
  absolute path runs even if the name doesn't match Bun's `*.test.*` / `*.spec.*` pattern.
- Killing a hung `bun test` with a timeout works (SIGTERM, and SIGKILL to the process group); the run leaves
  no XML.
- Copying hidden tests into `<app>/__grader__/` makes `import ... from "../src/x.ts"` resolve against the app.

New in the Docker re-check:

- A test file with **zero tests** exits **0** and writes **no XML** (same output as a load failure, but exit 0).
- `process.exit(0)` inside a test exits 0 and writes **no XML**, so it can't fake a partial pass.
- An unhandled promise rejection during a test is reported as a failure in the XML.
- An async test that never resolves fails after Bun's own 5 s per-test timeout, and the XML is written. A
  synchronous infinite loop blocks Bun's timer and needs the grader's per-file kill.
- A `setTimeout` that throws after the last test did not change the result (exit 0, all passed).

The first pass was a throwaway TypeScript prototype (deleted); the Docker re-check is scripted in
`scripts/verify.sh` (matrix) plus one-off probes. Harbor facts come from `src/harbor/verifier/verifier.py`
and the Harbor docs.

## Corrections log

| Claim | Correction |
|---|---|
| Pass 2 said `graded` isn't on PyPI | Wrong: [pypi.org/project/graded](https://pypi.org/project/graded/) exists (v1.0.0–1.0.5, June 11–12 2026) |
| Pass 2 cited harbor-framework/harbor#1382 as "CTRF is for diagnostics" | Wrong: #1382 is "Add Copilot CLI as a first-class agent" |
| Pass 2 said both TB and TB-Science enforce binary rewards | Verified for TB-Science only; TB tasks are binary by practice |

## Still open

- Search Harbor's Discord for: `rewardkit`, `partial credit`, `pass rate`, `junit`, `CTRF`, `test runner`,
  `weighted reward`, `eval-helpers`.
- Twitter/X discussion.
- Whether Rewardkit maintainers would accept a JUnit/CTRF "tests pass" criterion (ask before building
  that version).
