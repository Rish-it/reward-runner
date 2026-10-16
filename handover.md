# Handover

Current state and next steps for whoever picks this up next (you, Claude Code or Codex). Last updated
2026-10-16. Update this file at the end of every working session.

## Where things stand

| Milestone | State |
|---|---|
| M0: research, decisions, plan | Done (on `main`) |
| M1: Bun MVP | Done (merged into `main` via PR #1) |
| M2: Bun hardening T1–T8 + README threat model | **Next.** Not started |
| M3: release | Later; needs the owner's OK |

Remote: https://github.com/Rish-it/reward-runner. `main` holds all M1 work (PR #1 merged branch `m1-bun-mvp`):

| Commit | What |
|---|---|
| `6e8e64d` | Research, decisions and core plan |
| `dc47847` | M1: `grade` CLI, bun adapter, example task, verification matrix |
| `d063a99` | M1 marked done after the real Harbor run |
| `d9c8371` | M1 results and Harbor facts written into the docs |
| `109eea8` | This handover file |
| `fc8a9cf` (`main`) | Merge of PR #1 |

## What happened so far

1. **Research and decisions (M0).** No public tool turns `bun test` results into a Harbor reward; every task
   hand-writes its `test.sh`. Decided: a single static Go binary (D3), JUnit XML as input (D4), binary reward
   by default (D2), fail loudly on authoring errors (D7), hardening as the core value (D6). See
   `docs/02-research.md` and `docs/03-decisions.md`.
2. **M1 built.** `grade --runner bun FILE[=WEIGHT]...` places hidden tests in `<app>/__grader__/`, runs each
   file with Bun's JUnit reporter in its own process group with a timeout, reads the root `<testsuites>`
   totals, and writes `reward.json` (0/1, or a weighted fraction with `--mode partial`), `metrics.json` and
   `junit/`. Stale outputs are removed first, a `{"reward": 0}` is written before any test runs (fail closed),
   and authoring/infrastructure errors exit non-zero with no reward. About 700 lines, Go standard library only.
3. **M1 verified.** `scripts/verify.sh` (go vet, go test, Linux builds, a 23-check matrix inside
   `oven/bun:1.3.14`) passed on arm64 and amd64, and twice more from clean clones. A mutation check proved the
   process-group kill test catches a leader-only kill. New Bun 1.3.14 quirks are in `docs/02-research.md`.
4. **First real Harbor run.** Harbor 0.24.0 (`uv tool install harbor`), docker environment, arm64 Mac:
   `harbor run -p examples/bun-basic -a oracle` gave reward 1, `-a nop` gave 0. No fixes were needed. Details
   in `docs/04-plan.md` ("M1 results") and `docs/02-research.md` ("Harbor 0.24.0, real run").

## Repo map

```
cmd/grade/main.go           flags and exit codes (2 = bad flags, 1 = authoring/infra error, 0 = reward written)
internal/grade/grade.go     validate, fail closed, place tests, bun adapter, runner path/version/sha256
internal/grade/run.go       process group, timeout, SIGKILL of the whole group
internal/grade/junit.go     root totals, failing test names by describe path
internal/grade/score.go     binary/partial scoring, metrics.json, atomic writes
internal/grade/grade_test.go + testdata/   unit tests on a captured Bun 1.3.14 report
examples/bun-basic/         Harbor task (oven/bun:1.3.14, mean/median); tests/grade-* are built, gitignored
scripts/verify.sh           the whole M1 check: unit tests, builds, Docker matrix
docs/01..04                 context, research, decisions, plan (read in order)
AGENTS.md                   working rules for coding agents (CLAUDE.md imports it)
```

## Set up and check

Needs Go (go.mod says 1.23; tested with 1.26.4), Docker, and for the Harbor check, Harbor
(`uv tool install harbor`).

```bash
scripts/verify.sh                               # must end with ALL PASSED; also builds tests/grade-*
harbor run -p examples/bun-basic -a oracle      # expect reward 1
harbor run -p examples/bun-basic -a nop         # expect reward 0
```

Harbor writes results to `./jobs/` (gitignored); it uploads nothing unless you pass `--upload`.

## Next: M2 hardening

Goal (from `docs/04-plan.md`, "Hardening plan" and "Milestones"): every threat T1–T8 either has a malicious
fixture under `testdata/malicious/` proving the mitigation works, or a plain "not mitigated" line in a new
README threat-model section. Done when every fixture behaves as documented.

Work each threat the same way:

1. Write the fixture first: an agent submission that tries the attack against `examples/bun-basic`.
2. Show it wins today (reward wrongly 1) inside `oven/bun:1.3.14`. If it doesn't, record why in
   `docs/02-research.md` and keep the fixture as a regression check.
3. Add the smallest mitigation, then a check for it in `scripts/verify.sh`.
4. Re-verify any Bun behaviour you rely on in the Docker image and record it in `docs/02-research.md`.

What M1 already covers, so these need fixtures more than code:

- T3: the bun binary is called directly, never `package.json` scripts.
- T4: per-file timeout and process-group kill (a fork bomb and `ulimit` still need checking).
- T5: stale `reward.json`/`metrics.json`/`junit/` are removed before anything runs; each report goes to a
  fresh path. In-process report rewriting is not prevented.
- T6: the runner's path, version and sha256 are recorded in `metrics.json` (no pin flag yet).

Open, roughly in order of risk:

- **T1, planted config/preload.** `bunfig.toml` with `[test] preload` in `/app` can patch `expect`. Today tests
  run from `<app>/__grader__/`, inside the agent-writable app. Options in the plan: a grader-owned Bun config
  (check what `bun test --help` offers in 1.3.14), or running from a directory the agent doesn't control.
- **T2, environment injection.** The environment is inherited in M1; switch the runner to a minimal allowlist.
- **T7, agent code touching the reward channel.** Optional unprivileged test user while the grader writes the
  reward as root.
- **T8.** Recommend Harbor's separate verifier mode in the README.

Also open:

- Run a partial-mode variant under real Harbor: `reward.json` then has per-file keys next to `reward`
  (only binary mode has been run under Harbor).
- Whether Harbor keeps the exec bit on `tests/grade-*` (`test.sh` runs `chmod +x`, so it wasn't tested).

## Rules that bite

From `AGENTS.md` (Codex reads it directly; Claude Code reads it through `CLAUDE.md`):

- Go standard library only; ask the owner before adding any dependency.
- Ask the owner before releases, tags, publishing, license or name changes.
- Binary reward by default; `reward.json` holds flat numbers only.
- Wrong reward is worse than no reward: authoring/infra errors exit non-zero with no reward.
- Every behaviour has a check; run the whole suite twice from clean before calling a milestone done.
- When a decision changes, update `docs/03-decisions.md` in the same commit.

## Pending for the owner

- Nothing. M1 is merged into `main`; branch M2 from `main`, e.g. `m2-hardening`.

## Starting prompt for the next session

```
Read handover.md, then AGENTS.md and docs/01–04 in order. Start M2 (Bun hardening) on a new branch
m2-hardening from main. Begin with T1 (planted bunfig.toml preload): write the malicious fixture
under testdata/malicious/, show whether it raises the reward in oven/bun:1.3.14, then mitigate and add a
check to scripts/verify.sh. Follow AGENTS.md; ask before adding dependencies. Update handover.md before
you stop.
```
