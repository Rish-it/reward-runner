import { describe, expect, test } from "bun:test";
import { median } from "../src/stats.ts";

describe("median", () => {
  test("odd count", () => expect(median([3, 1, 2])).toBe(2));
  test("even count", () => expect(median([4, 1, 3, 2])).toBe(2.5));
  test("does not modify input", () => {
    const xs = [3, 1, 2];
    median(xs);
    expect(xs).toEqual([3, 1, 2]);
  });
  test("empty throws", () => expect(() => median([])).toThrow(RangeError));
});
