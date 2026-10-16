import { expect, test } from "bun:test";
import { mean } from "../src/stats.ts";

test("mean of values", () => expect(mean([1, 2, 3, 6])).toBe(3));
test("mean of empty throws", () => expect(() => mean([])).toThrow(RangeError));
