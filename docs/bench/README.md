# Benchmarks in zever

This document describes the benchmark infrastructure in zever: what has benchmarks, what doesn't (and why), and how to run and compare them locally with `benchstat`.

## What Has Benchmarks

Benchmarks exist for hot paths where performance matters: registry
`Register`/`Open` dispatch, per-request adapter operations (cache Get/Put,
revocation checks, rate-limit checks), codec encode/decode, ORM query
rendering, and the DSL compile pipeline. Each benchmark targets a specific
internal operation, not a full end-to-end flow.

Coverage spans every layer (`core/...`, `adapters/...`, `shared/...`,
`config`, `container`, `orm`, `dsl/...`, `serve`, `cmd/zever`,
`tools/...`). Both `*_bench_test.go` and `bench_test.go` file names are
used; either counts.

| Layer | Examples | What Is Measured |
|-------|----------|------------------|
| `config` | `config_bench_test.go`, `paths_bench_test.go` | Env parsing, path resolution, validation |
| `container` | `container_bench_test.go`, `paths_bench_test.go` | Lazy service accessor overhead, pool lookup |
| `shared/...` | `lrucache`, `registry`, `codec` | Get/Put/Delete throughput, registration/lookup, encode/decode |
| `core/...` | `cache`, `lock`, `auth/revocation/...` | Interface dispatch, store hot paths (hit/miss, round trip, parallel) |
| `adapters/...` | `cache/redis`, `lock/redis`, `idempotency/redis`, `db/postgres` | Adapter operations against in-process fakes, pure-function paths |
| `orm` | `render/*_bench_test.go` | Query builder render paths |
| `dsl/...` | `compile`, `lexer`, `parser`, `resolver`, backends | Tokenize/parse/resolve/generate stages individually |

Run `find . -name '*bench_test.go' | wc -l` to count benchmark files
(currently ~250 across ~220 packages).

Benchmarks that need a Redis speak to an in-process `miniredis` server on
loopback, never to external infrastructure: they stay deterministic and
offline. Anything requiring a live server (e.g. Postgres conformance) is a
`POSTGRES_DSN`-gated integration test, not a benchmark.

## What Does NOT Have Benchmarks (and Why)

| Area | Reason |
|------|--------|
| Interface-only packages (`core/auth/revocation`, `dsl/backend`) | No logic to measure; the implementations carry the benchmarks. |
| Conformance helpers (`core/*/…test`, `revocationtest`) | Shared test harnesses, not production code. |
| `main` packages and codegen CLIs (`cmd/zever/cmd/server`, `tools/gengrammar`) | Invoked by humans; latency dominated by process start. |
| Adapter `Open` against live infra | One-time startup cost; integration tests cover these. |
| Error paths / validation | Cold paths; optimizing them doesn't improve throughput. |
| `container.Close` | Runs once at shutdown; not a throughput concern. |
| Generated code (`examples/.../generated/...`) | Compiler output; benchmark the generator (`dsl/...`) instead. |

If you need to measure real-world latency against live infrastructure,
write a `POSTGRES_DSN`-gated integration test (e.g.
`POSTGRES_DSN='postgres://user:pass@localhost:5432/test?sslmode=disable' go test ./adapters/db/postgres/...`)
rather than a benchmark: benchmarks must pass offline and deterministically.

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

1. Create `*_bench_test.go` in the target package (`bench_test.go` also accepted).
2. Name the function `BenchmarkXxx`.
3. Call `b.ReportAllocs()` to track allocations.
4. Use `b.Context()` for the context under test.
5. Loop with `for b.Loop()` (not manual `b.N` loops).
6. Use `b.ResetTimer()` after setup, `b.RunParallel` for contention benchmarks.
7. Never register fixed names into package-global registries (use per-invocation unique names); never touch the network (use `miniredis` for Redis-backed stores); keep runs deterministic (no sleeps, no unseeded randomness).
8. Document what the benchmark measures in a comment above the function.

```go
// BenchmarkGet measures single-threaded cache Get throughput.
// The cache is pre-populated with 1000 entries.
func BenchmarkGet(b *testing.B) {
	c := New(1024)
	for i := 0; i < 1000; i++ {
		c.Put(i, i)
	}
	ctx := b.Context()
	b.ReportAllocs()
	b.ResetTimer()
	i := 0
	for b.Loop() {
		c.Get(ctx, i%1000)
		i++
	}
}
```

Run the new benchmark locally with `-count=5 -benchtime=1s` to verify
stability before committing.

## CI Benchmarks

Benchmarks never gate a PR: every CI job that runs them sets
`continue-on-error: true` and stays out of the required status checks.
Two mechanisms exist:

- `ci.yml` `bench` job (main/tags only, never PRs): a smoke run proving
  every benchmark still executes (`-benchtime=100x -count=1 -cpu=1`) and
  archiving the raw output for ad-hoc `benchstat` comparison.
- `.github/workflows/bench.yml`: triggers on pushes to `main`, on PRs
  touching `**/*_bench_test.go`, and daily at 06:00 UTC. Each run
  benchmarks one rotating module group (deterministic by day-of-week,
  `-count=6`) and uploads the raw output as an artifact; a
  `benchstat-compare` job then re-runs the same group against the base
  branch and emits a `::warning::` for any statistically significant
  regression above 5% (advisory only — shared runners are noisy, so follow
  up on repeated warnings, never on one sample).

`cmd/zever/benchworkflow_test.go` pins this contract: both jobs exist, both
set `continue-on-error`, and the workflow covers PRs. Update that test in
the same commit if you change the workflow's jobs.

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
# Run all benchmarks once (smoke test; shard with
# SHARD_TOTAL/SHARD_INDEX, scope with MODULES=./orm ./container)
make bench

# Full -count=10 baseline + A/B comparison
make bench-baseline
make bench-ab BASELINE=.bench/baseline.txt

# CPU+memory profiles for shared/codec, orm/render, container
make bench-profile

# Run with memory stats for a specific package
go test -bench=. -benchmem -count=5 ./shared/lrucache/...

# Generate CPU profile
go test -bench=BenchmarkGet -cpuprofile=cpu.prof ./shared/lrucache/...
go tool pprof cpu.prof
```