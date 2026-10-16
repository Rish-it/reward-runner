#!/bin/bash
cat > /app/src/stats.ts <<'TS'
export function mean(xs: number[]): number {
  if (xs.length === 0) throw new RangeError("mean of an empty array");
  let sum = 0;
  for (const x of xs) sum += x;
  return sum / xs.length;
}

export function median(xs: number[]): number {
  if (xs.length === 0) throw new RangeError("median of an empty array");
  const s = [...xs].sort((a, b) => a - b);
  const mid = Math.floor(s.length / 2);
  return s.length % 2 ? s[mid] : (s[mid - 1] + s[mid]) / 2;
}
TS
