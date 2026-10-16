In `/app/src/stats.ts`:

1. Make `mean(xs)` throw a `RangeError` when `xs` is empty.
2. Add and export `median(xs: number[]): number`. For an even number of values it returns the mean of the two
   middle values. It must not modify `xs`, and it throws a `RangeError` when `xs` is empty.
