# Benchmarks

Measured baseline numbers for router, ORM, and queue hot paths, plus how to
re-run them. All benchmarks are deterministic (no network, no timing
assertions) and live in `*_bench_test.go` files next to the code they measure.

> **Staleness warning:** numbers below are a point-in-time snapshot, not a
> performance contract. They depend on machine, CPU load, and Go version.
> Re-run locally before drawing conclusions; treat committed numbers older
> than a few weeks as stale.

## Results (2026-10-02)

Router (`adapters/router/fiber`, sequential vs concurrent `ServeHTTP`):

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkServeHTTP (cpu=1) | 5,737 | 6,640 | 60 |
| BenchmarkServeHTTP (cpu=4) | 4,430 | 6,653 | 61 |
| BenchmarkServeHTTPParallel (cpu=1) | 6,650 | 11,771 | 70 |
| BenchmarkServeHTTPParallel (cpu=4) | 3,122 | 11,791 | 70 |

ORM (`orm`, sqlite `:memory:`, Preload of 200 authors / 5,000 above-chunk):

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkPreloadBelowChunkSize (cpu=1) | 440,028 | 114,916 | 3,284 |
| BenchmarkPreloadBelowChunkSize (cpu=4) | 441,927 | 114,910 | 3,284 |
| BenchmarkPreloadAboveChunkSize (cpu=1) | 19,334,665 | 3,047,867 | 80,317 |
| BenchmarkPreloadAboveChunkSize (cpu=4) | 17,449,881 | 3,047,925 | 80,317 |
| BenchmarkPreloadConcurrent (cpu=1) | 449,707 | 117,204 | 3,301 |
| BenchmarkPreloadConcurrent (cpu=4) | 450,125 | 116,336 | 3,309 |

Note: `PreloadConcurrent` matches the sequential time because the sqlite
`:memory:` adapter pins to a single pooled connection, so workers serialize
inside `database/sql`. The bench measures contention cost, not speedup.

Queue (`adapters/queue/memory`, Push/Pop/Ack round trip on one topic):

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkMemoryRoundTrip (cpu=1) | 1,784 | 2,440 | 18 |
| BenchmarkMemoryRoundTrip (cpu=4) | 1,568 | 2,440 | 18 |
| BenchmarkMemoryRoundTripParallel (cpu=1) | 1,597 | 2,440 | 18 |
| BenchmarkMemoryRoundTripParallel (cpu=4) | 1,612 | 2,234 | 16 |

## Commands

Router and queue are fast, so they run 1s per bench; ORM runs 100
iterations per bench:

```sh
go test -run=NONE -bench=. -benchtime=1s -cpu=1,4 -benchmem ./adapters/router/fiber/
go test -run=NONE -bench=. -benchtime=100x -cpu=1,4 -benchmem ./orm/
go test -run=NONE -bench=. -benchtime=1s -cpu=1,4 -benchmem ./adapters/queue/memory/
```

Each path is its own Go module; run from the repo root with the `go.work`
workspace active (or `go -C <dir> test ...` from inside the module).
`-run=NONE` skips functional tests so only benchmarks execute. Total
runtime is well under a minute on the machine below.

## Machine

- CPU: 11th Gen Intel(R) Core(TM) i5-11400H @ 2.70GHz, 6 cores / 12 threads
- Caches: L1d 288 KiB, L1i 192 KiB, L2 7.5 MiB, L3 12 MiB
- RAM: 16 GiB (15 GiB usable)
- OS/Go: linux/amd64, `go version go1.27.0 linux/amd64`

## Repro

1. Check out the same commit and ensure `go.work` resolves (`go env GOWORK`).
2. Run the three commands above; each prints `goos`/`goarch`/`cpu` lines
   identifying the machine.
3. Compare with `benchstat` across runs (`-count=5`) before claiming a
   regression or win; single-run ns/op differences under ~5% are noise.

## Bench inventory

Concurrent-load (`b.RunParallel`) coverage:

- Router: `BenchmarkServeHTTPParallel`
  (`adapters/router/fiber/fiber_bench_test.go`) -- per-worker
  request/recorder pairs against one shared route.
- ORM: `BenchmarkPreloadConcurrent` (`orm/preload_bench_test.go`) --
  read-only Preload shared over one seeded `:memory:` database.
- Queue: `BenchmarkMemoryRoundTripParallel`
  (`adapters/queue/memory/memory_bench_test.go`) -- balanced Push/Pop/Ack
  per iteration on one shared topic, flat memory regardless of benchtime.

Benchmarks use `b.Context()`; helpers assert `t.Helper` where applicable.
