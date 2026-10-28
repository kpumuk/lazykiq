# Optimization evidence

Every performance or memory optimization requires measured results demonstrating the claim. A pure refactor or dependency bump without such a claim does not require benchmarks. This requirement is independent of visual evidence.

## Compare equivalent work

Choose a workload that exercises the affected path at a representative size. Prefer a focused existing Go benchmark. If none exists, add a meaningful benchmark to the change and run the identical harness on both revisions; copy only the harness into an isolated baseline checkout, never the optimized production code. Record any harness-only adaptation. Do not compare different algorithms called "before" and "after" solely inside the proposed revision and present them as measurements of two actual revisions.

Pin and report baseline/proposed SHAs. Keep machine, Go toolchain, build flags, `GOMAXPROCS`, dataset, and Redis/Sidekiq configuration equivalent. Exclude fixture generation from timed work unless setup itself is the optimization target. Prevent compiler elimination and validate the workload's result outside the timed path where practical. A miniredis microbenchmark measures that workload, not real Redis network throughput.

Run repeated trials, normally at least ten per revision, in a quiet environment. For example, run this exact command in each revision's checkout, with its output saved separately outside the repository:

```bash
go test ./internal/ui/components/table -run '^$' \
  -bench '^BenchmarkWideSearchRedraw$' -benchmem -count=10 -benchtime=1s
```

This repository example measures wide-table redraw/navigation. Select the actual package and benchmark for the proposed optimization; do not use it as generic proof for unrelated changes. Keep the full command identical between runs. Alternate baseline/proposed runs when machine drift could affect the conclusion. Retain raw output until the PR figures are checked.

## Measure the actual claim

Report `ns/op` or a relevant latency/throughput metric for speed claims. Use `B/op` and `allocs/op` for allocation reductions. Retained-memory, leak, or peak-memory claims need a repeated workload and the corresponding heap/RSS measurement over a stated duration; allocation counts alone do not demonstrate lower retained memory. Include memory alongside time when either may trade off against the other.

Use `benchstat` when available for distribution/significance analysis; otherwise calculate medians from the captured samples and state the method. Do not install tooling unnecessarily or present a single favorable trial as representative. For lower-is-better metrics, report signed change as `(proposed / baseline - 1) * 100%`, so a reduction is negative. If the baseline is zero, report the absolute difference instead of an undefined percentage.

If noise, an unrepresentative workload, or contradictory results prevent demonstrating the claimed optimization, investigate and adjust the claim or implementation. Do not publish an optimization PR as complete with missing, neutral, or regressing evidence for its central claim. State narrowly what the benchmark supports and any material regressions or tradeoffs.

## PR section

Include `## Benchmarks` with the exact command, workload size and setup, environment, revision IDs, and trial count. Follow it with a compact table of actual baseline/proposed medians and signed percentage deltas, using explicit units. Include the variation/significance result when available and limitations that affect interpretation. Link or attach raw results when useful, but keep the essential comparison in the PR body. Never invent example measurements or treat a screenshot of a benchmark as a substitute for reproducible results.
