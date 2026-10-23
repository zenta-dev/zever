# Benchmarks in zever

This document describes the benchmark infrastructure in zever: what has benchmarks, what doesn't (and why), and how to run and compare them locally with `benchstat`.

## What Has Benchmarks

Benchmarks exist for hot paths where performance matters and the cost of the operation is measurable in microseconds or less. Each benchmark targets a specific internal operation, not a full end-to-end flow.

| Package | Benchmark File | What It Measures |
|---------|---------------|------------------|
| `config` | `config_bench_test.go`, `paths_bench_test.go` | Env parsing, path resolution, validation |
| `container` | `container_bench_test.go`, `paths_bench_test.go` | Lazy service accessor overhead, pool lookup |
| `shared/lrucache` | `lrucache_bench_test.go` | Get/Put/Delete throughput under contention |
| `shared/registry` | `registry_bench_test.go` | Registry registration and lookup |
| `core/billing` | `billing_bench_test.go` | Registry registration + factory invocation |
| `core/flag` | `flag_bench_test.go` | Registry registration + factory invocation |
| `core/geo` | `geo_bench_test.go` | Registry registration + factory invocation |
| `orm` | `render_bench_test.go` | Query builder render paths |
| `dsl` | `compile_bench_test.go` | Schema compilation pipeline |

Run `grep -r "_bench_test.go" --include="*.go" | wc -l` to count benchmark files (currently ~15).

## What Does NOT Have Benchmarks (and Why)

| Area | Reason |
|------|--------|
| Network adapters (ai, db, cache/redis, etc.) | Require external services; benchmarks would measure network/DB, not our code. Integration tests cover these. |
| Adapter `Open`/`Register` | One-time startup cost; not a hot path. |
| CLI commands (`zever compile`, `zever doctor`) | Invoked by humans; latency dominated by process start/go build. |
| DSL parser (lexer/parser/resolver) | Already covered by `dsl/compile` benchmarks at the compiler level. |
| Error paths / validation | Cold paths; optimizing them doesn't improve throughput. |
| `container.Close` | Runs once at shutdown; not a throughput concern. |

If you need to measure an adapter's real-world latency, write an integration test against a local instance (e.g., `POSTGRES_DSN=... go test ./adapters/db/postgres/... -run=Benchmark`).

## Running Benchmarks Locally

### Single package

```sh
cd /path/to/package
go test -bench=. -benchmem -count=5 -benchtime=1s
```

- `-bench=.` runs all benchmarks in the package
- `-benchmem` includes allocation stats
- `-count=5` runs each benchmark 5 times (for variance)
- `-benchtime=1s` runs each iteration for at least 1 second

### Multiple packages (via go.work)

From repo root:

```sh
go test -bench=. -benchmem -count=5 ./config/... ./container/... ./shared/lrucache/...
```

Note: `go test ./...` doesn't work with go.work; list packages explicitly.

### Saving results for comparison

```sh
go test -bench=. -benchmem -count=5 ./config/... > before.txt
# make changes
go test -bench=. -benchmem -count=5 ./config/... > after.txt
benchstat before.txt after.txt
```

## Using benchstat for A/B Comparison

`benchstat` (install with `go install golang.org/x/perf/cmd/benchstat@latest`) computes statistical significance between two benchmark runs.

### Example workflow

```sh
# Baseline
go test -bench=BenchmarkConfigParse -benchmem -count=10 ./config/... > base.txt

# Make a change (e.g., optimize env parsing)
# ...

# After change
go test -bench=BenchmarkConfigParse -benchmem -count=10 ./config/... > changed.txt

# Compare
benchstat base.txt changed.txt
```

### Interpreting output

```
name              old time/op    new time/op    delta
Parse-8           12.3µs ± 2%    10.1µs ± 1%  -17.88%  (p=0.001 n=10+10)

name              old alloc/op   new alloc/op   delta
Parse-8           2.45kB ± 0%    2.10kB ± 0%  -14.29%  (p=0.001 n=10+10)

name              old allocs/op  new allocs/op  delta
Parse-8           32 ± 0%        28 ± 0%      -12.50%  (p=0.001 n=10+10)
```

- `delta` shows the percentage change (negative = faster/smaller)
- `p=0.001` means statistically significant (p < 0.05)
- `n=10+10` means 10 runs per version

**Rule of thumb**: Only trust deltas with `p < 0.05` and at least 5 runs per side. Noise in CI can be high; always run locally on a quiet machine for final numbers.

## Adding a New Benchmark

1. Create `*_bench_test.go` in the target package.
2. Name the function `BenchmarkXxx`.
3. Use `b.ResetTimer()` after setup.
4. Use `b.RunParallel` for contention benchmarks.
5. Document what the benchmark measures in a comment above the function.

```go
// BenchmarkGet measures single-threaded cache Get throughput.
// The cache is pre-populated with 1000 entries.
func BenchmarkGet(b *testing.B) {
	c := New(1024)
	for i := 0; i < 1000; i++ {
		c.Put(i, i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(i % 1000)
	}
}
```

6. Run it locally with `-count=5 -benchtime=1s` to verify stability before committing.

## CI Benchmarks

Benchmarks are **not** run in CI on every PR (they're too noisy and slow). They run:
- On `push` to `main` (scheduled)
- Manually via `make bench` (which runs `go test -bench=. -benchmem -count=1 ./...` across workspace modules)

Results are uploaded as artifacts but not gated. Performance regressions are caught by manual `benchstat` comparison during release prep or when a PR explicitly targets a hot path.

## Anti-Patterns to Avoid

| Anti-Pattern | Why It's Bad | Fix |
|--------------|--------------|-----|
| `b.N` in setup loop | Measures setup, not the operation | Use `b.ResetTimer()` after setup |
| Benchmarking `fmt.Println` | I/O dominates; not the code under test | Benchmark the pure function |
| Single run (`-count=1`) | No variance data; noise looks like signal | Always use `-count=5` minimum |
| Comparing different machines | Hardware differences swamp code changes | Run both versions on same machine, same load |
| Benchmarking cold paths | Optimizing startup/shutdown doesn't help throughput | Focus on hot loops (Get/Put, render, parse) |

## Related Commands

```sh
# Run all benchmarks once (smoke test)
make bench

# Run with memory stats for a specific package
go test -bench=. -benchmem -count=5 ./shared/lrucache/...

# Generate CPU profile
go test -bench=BenchmarkGet -cpuprofile=cpu.prof ./shared/lrucache/...
go tool pprof cpu.prof
```