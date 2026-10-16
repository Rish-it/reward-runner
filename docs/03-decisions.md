# 03 — Decisions: what we chose and why

Each decision lists the reason and what would make us revisit it. When a decision changes, update this file
in the same commit as the code.

## D1. Build it, small, without overthinking

**Why:** the gap is real (see [02-research.md](02-research.md)), the cost is only time, and it removes
rewritten glue from every task the author writes. It also makes a good portfolio piece, because verifier
hardening is the part people find hard.
**Expectations:** a personal tool first. Adoption is a bonus, not a goal. The audience is niche, and Harbor
could absorb the idea into Rewardkit.
**Revisit when:** Harbor ships an equivalent, or the tool stops saving time on real tasks.

## D2. Binary reward by default; details go elsewhere

**Why:** Harbor passes a trial only at `reward >= 1.0`, Terminal-Bench-Science rejects anything but `0`/`1`,
and pass@1-style specs count only full passes. So by default:

- `reward.json` = `{"reward": 1}` only if every test in every file passes, else `{"reward": 0}`.
- Counts, per-file fractions, weights and failing test names go to `metrics.json` (ignored by Harbor).

`--mode partial` stays available for authors who explicitly want a weighted fraction as the reward.
**Revisit when:** official benchmarks start accepting partial credit.

## D3. A single static Go binary

**Why:** the grader runs inside the task's offline image, and the image decides which runtimes exist. A
Python grader fails in `oven/bun`; a Bun/TypeScript grader fails in a Python image. A static binary needs
nothing installed. Go gives easy cross-compilation and `encoding/xml` in the standard library, with zero
third-party dependencies.
**Cost:** ship two Linux builds (amd64 and arm64, since Docker on Apple Silicon runs arm64), picked by
`uname -m` in `test.sh`.
**Considered:** a TypeScript file for Bun only (simplest MVP, but it ties the grader to one ecosystem);
Python (matches Rewardkit, but needs Python in every image).
**Revisit when:** a contribution to Rewardkit becomes the main delivery path.

## D4. JUnit XML as the common input

**Why:** it's the one format nearly every runner can write offline: Bun, pytest, Vitest, Deno, PHPUnit,
Maven and node:test natively; Jest via `jest-junit`; Go via `gotestsum`; Rust via `cargo-nextest`.
**Considered:** CTRF JSON (Harbor's default pytest template emits it, but fewer runners write it natively).
A CTRF input can be added later as a second parser.
**Rule:** read totals from the root element only, and list failing test cases. Don't sum nested suites.

## D5. Bun first, other runners only when needed

**Why:** Bun is the motivating case and the worst-served runner (no JSON reporter). Add pytest and Vitest when
a real task needs them, then a generic `--cmd` template for any JUnit-capable runner.
**Revisit when:** a task needs another runner.

## D6. Hardening is the core value, not counting

**Why:** counting is about 20 lines. What authors get wrong is gaming: agent-planted configs and preloads,
patched dependencies, forged or rewritten reports, hangs. Every mitigation must be proven with a malicious
fixture, and the remaining gaps documented honestly in a threat-model section.

## D7. Fail loudly on authoring and infrastructure errors

**Why:** a wrong reward is worse than no reward. Two outcomes are kept apart:

- **Bad submission** (tests fail, a test file fails to load because an export is missing, a timeout):
  reward `0`, exit `0`.
- **Authoring or infrastructure error** (weights don't sum to 1, a test file doesn't exist, the runner
  binary isn't installed, an invalid flag): no reward file, non-zero exit, so Harbor flags the trial instead
  of recording a fake `0`.

## D8. Out of scope

LLM judges, trajectory checks, file-content checks (Rewardkit and `graded` cover these), multi-step tasks,
any network access at grading time, and a long-running server or daemon.

## D9. Possible later: contribute upstream

A "JUnit/CTRF tests pass" criterion in Rewardkit would reach more Harbor users (in images that have
Python). Ask the maintainers first. This tool stays useful either way for images without Python.
