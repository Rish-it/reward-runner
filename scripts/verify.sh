#!/bin/bash
# Runs the unit tests, builds the Linux binaries into examples/bun-basic/tests/, and runs the M1 verification
# matrix (docs/04-plan.md) inside the real Bun image, laid out the way Harbor runs a task:
# the app in /app, hidden tests in /tests, rewards in /logs/verifier.
#
#   scripts/verify.sh                  # needs Go and Docker
#   BUN_IMAGE=oven/bun:1.3.x scripts/verify.sh
set -euo pipefail

if [ "${1:-}" != "--in-container" ]; then
  cd "$(dirname "$0")/.."
  go vet ./...
  go test -count=1 ./...
  for arch in amd64:x86_64 arm64:aarch64; do
    CGO_ENABLED=0 GOOS=linux GOARCH="${arch%%:*}" go build -trimpath -ldflags='-s -w' \
      -o "examples/bun-basic/tests/grade-${arch##*:}" ./cmd/grade
  done
  exec docker run --rm -v "$PWD":/src:ro "${BUN_IMAGE:-oven/bun:1.3.14}" bash /src/scripts/verify.sh --in-container
fi

# ---- inside the container ----
EX=/src/examples/bun-basic
GRADE=/tests/grade-$(uname -m)
OUT=/logs/verifier
failed=0
echo "bun $(bun --version), $(uname -m)"

# fresh resets /app, /tests and /logs/verifier to the example task's starting state.
fresh() {
  rm -rf /app /tests "$OUT"
  cp -r "$EX/environment/repo" /app
  cp -r "$EX/tests" /tests
  mkdir -p "$OUT"
  cd /app
}

# check NAME JS: passes when the JS expression is true. It sees r (reward.json), m (metrics.json) and
# f (metrics.json files), or null when a file is missing.
check() {
  if NAME="$1" COND="$2" bun -e '
    const fs = require("fs");
    const read = (p) => (fs.existsSync(p) ? JSON.parse(fs.readFileSync(p, "utf8")) : null);
    const r = read("/logs/verifier/reward.json"), m = read("/logs/verifier/metrics.json"), f = m && m.files;
    if (!eval(process.env.COND)) {
      console.error("  condition: " + process.env.COND);
      console.error("  reward.json: " + JSON.stringify(r) + "\n  metrics.json: " + JSON.stringify(m));
      process.exit(1);
    }'; then
    echo "PASS $1"
  else
    echo "FAIL $1"
    failed=1
  fi
}

# authoring_error NAME CMD...: CMD must exit non-zero and leave no reward file. With STALE=1, a stale
# {"reward": 1} is planted first and must be gone too.
authoring_error() {
  local name=$1 code=0
  shift
  fresh
  if [ "${STALE:-}" = 1 ]; then echo '{"reward": 1}' >"$OUT/reward.json"; fi
  "$@" >/dev/null 2>&1 || code=$?
  if [ "$code" -ne 0 ] && [ ! -e "$OUT/reward.json" ] && [ ! -e "$OUT/reward.txt" ]; then
    echo "PASS $name (exit $code, no reward)"
  else
    echo "FAIL $name: exit $code, reward.json: $(cat "$OUT/reward.json" 2>/dev/null || echo none)"
    failed=1
  fi
}

# Partial agent: mean untouched (1/2), median right only for odd counts (1/4).
partial_agent() {
  cat >>/app/src/stats.ts <<'TS'

export function median(xs: number[]): number {
  xs.sort((a, b) => a - b);
  return xs[Math.floor(xs.length / 2)];
}
TS
}

fresh
bash /tests/test.sh >/dev/null 2>&1
check "null run: reward 0" 'r.reward === 0 && Object.keys(r).length === 1'
check "null run: missing export is load_failed" 'f["median.test.ts"].load_failed && f["median.test.ts"].tests === 0'
check "null run: counts" 'f["mean.test.ts"].tests === 2 && f["mean.test.ts"].passed === 1 && f["mean.test.ts"].failing[0] === "mean of empty throws"'
check "null run: __grader__ cleaned up" '!fs.existsSync("/app/__grader__")'
check "null run: runner recorded" 'm.runner.name === "bun" && m.runner.version === Bun.version && m.runner.sha256.length === 64'

fresh
bash "$EX/solution/solve.sh"
bash /tests/test.sh >/dev/null 2>&1
check "oracle run: reward 1" 'r.reward === 1 && Object.keys(r).length === 1 && m.weighted_fraction === 1'

fresh
partial_agent
"$GRADE" --runner bun mean.test.ts median.test.ts >/dev/null 2>&1
check "partial agent, binary: reward 0" 'r.reward === 0 && m.weighted_fraction === 0.375'
"$GRADE" --runner bun mean.test.ts=0.4 median.test.ts=0.6 --mode partial >/dev/null 2>&1
check "partial agent, partial: exact fraction" \
  'JSON.stringify(r) === JSON.stringify({"mean.test.ts": 0.5, "median.test.ts": 0.25, "reward": 0.35})'
check "partial agent, partial: counts" \
  'f["median.test.ts"].tests === 4 && f["median.test.ts"].passed === 1 && f["median.test.ts"].failed === 3 &&
   f["median.test.ts"].weight === 0.6 &&
   f["median.test.ts"].failing.join("|") === "median > even count|median > does not modify input|median > empty throws"'

fresh
partial_agent
"$GRADE" --runner bun --mode partial >/dev/null 2>&1
check "discovery, equal weights" 'r.reward === 0.375 && f["mean.test.ts"].weight === 0.5 && Object.keys(f).length === 2'

# Hang: median starts a background child, then loops forever. The fail-closed 0 must exist while it runs,
# the timeout must fire, the child must die with the process group, and a reward must still be written.
fresh
cat >>/app/src/stats.ts <<'TS'

export function median(xs: number[]): number {
  Bun.spawn(["sleep", "1234"]);
  while (true) {}
}
TS
start=$SECONDS
"$GRADE" --runner bun --timeout 3s median.test.ts mean.test.ts >/dev/null 2>&1 &
sleep 1.5
check "hang: fail-closed reward 0 while running" 'r.reward === 0 && m === null'
wait
elapsed=$((SECONDS - start))
check "hang: timed out file scores 0, reward written" \
  'r.reward === 0 && f["median.test.ts"].timed_out && f["median.test.ts"].seconds < 5 && f["mean.test.ts"].tests === 2'
check "hang: process group killed" \
  '!fs.readdirSync("/proc").some((p) => { try { return fs.readFileSync(`/proc/${p}/cmdline`, "utf8").includes("1234"); } catch { return false; } })'
if [ "$elapsed" -le 8 ]; then echo "PASS hang: whole run took ${elapsed}s"; else echo "FAIL hang: took ${elapsed}s"; failed=1; fi

# Zero tests: Bun 1.3.14 exits 0 and writes no XML for a file with no tests.
fresh
bash "$EX/solution/solve.sh"
printf 'import { mean } from "../src/stats.ts";\nmean([1]);\n' >/tests/empty.test.ts
"$GRADE" --runner bun >/dev/null 2>&1
check "zero-test file scores 0" \
  'r.reward === 0 && f["empty.test.ts"].tests === 0 && f["empty.test.ts"].exit_code === 0 && f["mean.test.ts"].passed === 2'

STALE=1 authoring_error "bad weights also remove a stale reward" "$GRADE" --runner bun mean.test.ts=0.9
authoring_error "bad weights (sum 1.1)" "$GRADE" --runner bun mean.test.ts=0.5 median.test.ts=0.6 --mode partial
authoring_error "weights on some files only" "$GRADE" --runner bun mean.test.ts=1 median.test.ts
authoring_error "missing test file" "$GRADE" --runner bun mean.test.ts nope.test.ts
authoring_error "missing runner" env PATH=/usr/bin:/bin "$GRADE" --runner bun mean.test.ts
authoring_error "unknown runner" "$GRADE" --runner jest mean.test.ts
authoring_error "unknown mode" "$GRADE" --runner bun --mode best mean.test.ts
authoring_error "unknown flag" "$GRADE" --runner bun --nope mean.test.ts

[ "$failed" -eq 0 ] && echo "ALL PASSED" || { echo "SOME FAILED"; exit 1; }
