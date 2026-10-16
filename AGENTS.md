# AGENTS.md

Instructions for coding agents (Claude Code, Codex, Cursor, ...) working in this repo.

## Start here

Read, in order: `handover.md` (current state and next steps), `README.md`, `docs/01-context.md`,
`docs/02-research.md`, `docs/03-decisions.md`, `docs/04-plan.md`. The current milestone is the first one in
`docs/04-plan.md` that isn't marked done. Update `handover.md` before you stop.

## Rules

- **Follow the decisions** in `docs/03-decisions.md`. If you think one is wrong, say so and ask before
  deviating. When a decision changes, update that file in the same commit.
- **Verify, don't assume.** Runner behaviour changes between versions. Re-check every quirk you rely on
  against the runner version in the target Docker image, and record the version in `docs/02-research.md`.
- **Smallest code that works.** Go standard library only. No speculative abstractions, no config files the
  CLI doesn't need. Ask before adding any dependency.
- **Binary by default.** Never make a fractional `reward.json` the default (D2). `reward.json` holds flat
  numbers only; details go in `metrics.json`.
- **Wrong reward is worse than no reward.** Authoring and infrastructure errors exit non-zero and write no
  reward; bad submissions score 0 (D7).
- **Every behaviour has a check.** Each row of the verification matrix in `docs/04-plan.md` needs an
  automated test or a scripted Docker run. Run the whole suite twice from clean before calling a milestone done.
- **Hardening claims need proof.** Every mitigation gets a malicious fixture under `testdata/malicious/`;
  anything not mitigated goes in the README's threat-model section, stated plainly.

## Ask the owner before

Creating releases or tags, publishing anything, changing the license, renaming the project, or adding
dependencies.
