# reward-runner

Turn hidden test results from any test runner into a [Harbor](https://harborframework.com) verifier
reward, safely.

> **Status: planning.** No code yet. This repo holds the research, decisions and plan, so a contributor or a
> coding agent can pick it up cold.

## The problem

A Harbor task grades an agent by running `tests/test.sh`, which must write a reward to
`/logs/verifier/reward.json` or `reward.txt`. Every task author rewrites the same glue: run the test runner,
parse its output, decide pass or fail, write the file, and defend against an agent that tries to game the
tests. For `bun test` there isn't even a JSON reporter, so authors regex JUnit XML by hand. Research found
no public tool that does this ([docs/02-research.md](docs/02-research.md)).

## What it will do

A single static binary you drop into `tests/`:

```bash
/tests/grade-$(uname -m) --runner bun div.test.ts mul.test.ts
```

- Runs each hidden test file with the runner's JUnit reporter, offline, with a per-file timeout.
- Writes `reward.json` as a literal `0` or `1` by default, matching Harbor's pass rule and Terminal-Bench's
  binary-reward requirement. A weighted `--mode partial` is optional.
- Writes `metrics.json` with per-file passed/failed/skipped counts, weighted fractions and failing test names.
- Treats a test file that fails to load or times out as 0, and fails loudly (no reward) on authoring or
  infrastructure errors.
- Hardens the run against common reward hacks: agent-planted configs and preloads, environment injection,
  patched dependencies, forged reports.

Bun first; pytest, Vitest and a generic JUnit `--cmd` mode follow when needed.

## Read in this order

1. [docs/01-context.md](docs/01-context.md): what Harbor verifiers are, and the problem
2. [docs/02-research.md](docs/02-research.md): what exists, the evidence, and why the gap is real
3. [docs/03-decisions.md](docs/03-decisions.md): what was decided, and why
4. [docs/04-plan.md](docs/04-plan.md): the core plan (CLI, scoring, hardening, verification, milestones)
5. [AGENTS.md](AGENTS.md): working rules for coding agents

## License

[Apache-2.0](LICENSE)
