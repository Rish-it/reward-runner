# 01 — Context: what Harbor verifiers are, and the problem

## Harbor in one minute

[Harbor](https://harborframework.com) ([GitHub](https://github.com/harbor-framework/harbor)) is a framework
from the Terminal-Bench team for running coding agents (Claude Code, Codex, ...) against tasks inside
sandboxes (Docker, Daytona, E2B, ...) and scoring the result. Companies use the Harbor task format for eval
datasets and for hiring take-homes ("build a task that frontier models can't reliably solve").

A task is a folder:

```
my-task/
├── task.toml              # metadata, timeouts, verifier settings
├── instruction.md         # the spec the agent receives
├── environment/
│   ├── Dockerfile         # the sandbox image (offline at grading time)
│   └── repo/              # the "before" state of the codebase
├── solution/solve.sh      # reference (oracle) solution
└── tests/
    └── test.sh            # verifier entrypoint
```

## How a run is graded

1. Harbor builds the image from `environment/` and starts the sandbox.
2. The agent reads `instruction.md` and edits the code (it never sees `tests/`).
3. After the agent finishes, Harbor uploads `tests/` to `/tests/` and runs `/tests/test.sh` in the task's
   working directory (usually the Dockerfile `WORKDIR`, e.g. `/app`).
4. `test.sh` must write a reward to `/logs/verifier/reward.json` or `/logs/verifier/reward.txt`.

Reward contract (from Harbor's [verifier docs](https://harborframework.com/docs/tasks/verifier) and
`src/harbor/verifier/verifier.py`):

| Fact | Detail |
|---|---|
| Files | `reward.txt` = a single number; `reward.json` = a key/value object of numbers |
| Precedence | If both exist, Harbor reads **only** `reward.json` |
| Validation | `reward.json` values must be finite numbers. A nested object (e.g. a per-file breakdown) is rejected |
| Pass | Harbor and Terminal-Bench treat a trial as passed only at `reward >= 1.0` |
| Diagnostics | Any other file in `/logs/verifier/` (e.g. `metrics.json`, `ctrf.json`) is ignored by Harbor and free-form |

Typical task-quality checks before publishing a task:

- **Oracle run**: apply `solution/solve.sh`, reward must be `1.0`.
- **Null / nop run**: untouched environment, reward must be `0`.
- **Rollouts**: N runs with a frontier model; pass@1 must land in a target band (e.g. above 0, at most 0.6).

Because "pass" means `reward >= 1.0`, a fractional reward never counts as partial credit downstream.
Terminal-Bench-Science enforces literal `0`/`1` rewards with an automated check (see
[02-research.md](02-research.md#official-stance-on-partial-credit)).

## The problem

Every task author rewrites the same glue in `test.sh`. For a Bun/TypeScript task it typically looks like:

1. Copy each hidden test file into the app (e.g. `<app>/__grader__/`) so relative imports like
   `../src/x.ts` resolve.
2. Run `bun test <file> --reporter=junit --reporter-outfile=/tmp/<file>.xml`.
3. Regex the totals (`tests`, `failures`, `skipped`) out of the XML.
4. Compute `passed / total` per file, weight the files, sum.
5. Write `reward.txt` / a JSON breakdown.

What goes wrong with hand-written glue:

- **Rewritten per task**, slightly differently each time, usually without tests of its own.
- **Fragile parsing**: Bun nests a `<testsuite>` per `describe`, each repeating counts, so summing every
  occurrence over-counts. Only the root `<testsuites>` line has the true totals.
- **Silent semantics**: a test file that fails to import (the agent didn't add the export) makes Bun write
  **no XML at all**. Glue must treat that as 0, not crash and not reuse a stale file.
- **No timeout**: an agent's infinite loop hangs the verifier, so the trial ends with no reward instead of 0.
- **Fractional rewards**: weighted partial scores look nice but are rejected or ignored by official benchmarks,
  and writing a nested breakdown to `reward.json` makes Harbor reject the reward entirely.
- **Reward hacking**: the agent's code runs inside the test process. It can plant a `bunfig.toml` preload,
  a `conftest.py`, a modified test config, or patched `node_modules`, or rewrite the report on exit.

Bun makes this worse than most runners: `bun test` has **no JSON reporter**
([oven-sh/bun#6250](https://github.com/oven-sh/bun/issues/6250), open since 2023) and no custom reporters
([oven-sh/bun#2984](https://github.com/oven-sh/bun/issues/2984), open). JUnit XML (since Bun 1.1.35) is the
only machine-readable output; the console summary (`N pass / M fail`) is plain text on stderr.

## The idea

One small tool you drop into `tests/`, so `test.sh` becomes a single line, with the counting, the
edge cases, the binary reward and the anti-gaming hardening done once and tested properly.
The plan is in [04-plan.md](04-plan.md).
