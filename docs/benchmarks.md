# Benchmarks

Measured numbers for router, ORM, and queue hot paths, plus how to
re-run them. All benchmarks are deterministic (no network, no timing
assertions) and live in `*_bench_test.go` files next to the code they
measure. In-process backends (sqlite `:memory:`, miniredis) keep them
offline; their absolute numbers are dominated by the backend's own
parsing, so compare shapes (allocs/op) and relative deltas, not raw ns.

> **Staleness warning:** numbers below are a point-in-time snapshot, not a
> performance contract. They depend on machine, CPU load, and Go version.
> Re-run locally before drawing conclusions; treat committed numbers older
> than a few weeks as stale.

## Results (2026-10-03)

Router (`adapters/router/fiber`, sequential vs concurrent `ServeHTTP`).
`ServeHTTP` now runs the adaptor over a pooled `*httptest.ResponseRecorder`
(`recorderPool`); `BenchmarkServeHTTPAdaptor` isolates the adaptor wrapping
with a no-op handler. "before" is the pre-pool `httptest.NewRecorder()` path.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkServeHTTP before (cpu=1) | 5,973 | 6,638 | 60 |
| BenchmarkServeHTTP after (cpu=1) | 5,874 | 6,062 | 55 |
| BenchmarkServeHTTP after (cpu=4) | 4,855 | 6,079 | 56 |
| BenchmarkServeHTTPAdaptor (cpu=1) | 4,651 | 5,457 | 45 |
| BenchmarkServeHTTPAdaptor (cpu=4) | 3,622 | 5,461 | 45 |
| BenchmarkServeHTTPParallel (cpu=1) | 8,483 | 11,195 | 65 |
| BenchmarkServeHTTPParallel (cpu=4) | 4,001 | 11,219 | 65 |

The pool removes the recorder's struct/map/buffer allocations from the hot
path: `ServeHTTP` drops 5 allocs/op and ~576 B/op. `BenchmarkServeHTTPAdaptor`
shows the adaptor wrapping alone costs ~45 allocs/op (the pooled recorder is
3 of them).

ORM render (`orm`, sqlite `:memory:`). `escapeLike` and `flattenGroupTerms`
were hoisted to a package-level replacer / a lazy dedup (linear scan for few
leaves, map for many). "before" is the per-call `strings.NewReplacer` /
always-map path.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkEscapeLike before (cpu=1) | 1,361 | 6,784 | 8 |
| BenchmarkEscapeLike after (cpu=1) | 51 | 24 | 1 |
| BenchmarkEscapeLike after (cpu=4) | 44 | 24 | 1 |
| BenchmarkFlattenGroupTermsFew before (cpu=1) | 434 | 572 | 7 |
| BenchmarkFlattenGroupTermsFew after (cpu=1) | 354 | 560 | 3 |
| BenchmarkFlattenGroupTermsFew after (cpu=4) | 369 | 560 | 3 |
| BenchmarkFlattenGroupTermsRollup (cpu=1) | 499 | 560 | 3 |
| BenchmarkFlattenGroupTermsMany (cpu=1) | 7,729 | 21,272 | 75 |

The hoisted replacer cuts `escapeLike` from 8 allocs to 1 (the result
string). The lazy dedup cuts `flattenGroupTerms` for a few leaves from 7
allocs to 3 and is also faster (354 vs 434 ns); past the linear-scan
threshold (`linearDedupMaxLeaves` = 8) it falls back to the map
(`...Many`), which is the faster shape for many leaves.

Queue (`adapters/queue/db`, stale-claim sweep over sqlite). The sweep is one
SELECT plus one set-based UPDATE for the whole batch: the database computes
`attempt = attempt + 1` itself (new orm expression assignment,
`orm.SetExpr(col, orm.Add(col.Expr(), 1))`), under the same lease guard
(`claimed_by != '' AND claimed_until <= now AND id IN (...)`), so a row
settled between the SELECT and the UPDATE is skipped instead of
double-bumped. "before" is the old N+1 loop (one guarded UPDATE per row).

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkReclaimStale before (cpu=1) | 4,748,598 | 331,827 | 5,690 |
| BenchmarkReclaimStale after (cpu=1) | 645,270 | 109,471 | 3,208 |
| BenchmarkReclaimStale before (cpu=4) | 4,869,046 | 331,997 | 5,685 |
| BenchmarkReclaimStale after (cpu=4) | 631,722 | 109,574 | 3,210 |

The single set-based UPDATE cuts the sweep ~7.4x (4.75M ns/op to 645K ns/op
at cpu=1): the N per-row round trips and per-row commit overhead collapse
into one statement, and the attempt bump moves into the database. B/op
drops ~3x (331 KB to 109 KB) and allocs/op ~1.8x (5,690 to 3,208) with it.
A second run confirmed the after numbers within noise (646,554 / 637,319
ns/op).

Idempotency (`adapters/idempotency/redis`, miniredis). `Begin` and
`Complete` are now one Lua script each (1 RTT) instead of two client calls.
Absolute numbers are dominated by miniredis's in-process command parsing.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkBegin (cpu=1) | 152,587 | 190,137 | 786 |
| BenchmarkBegin (cpu=4) | 128,635 | 190,249 | 786 |
| BenchmarkComplete (cpu=1) | 162,142 | 198,666 | 793 |
| BenchmarkComplete (cpu=4) | 133,553 | 198,746 | 793 |

## Commands

Router and queue are fast, so they run 1s per bench; ORM runs 100
iterations per bench:

```sh
go test -run=NONE -bench=. -benchtime=1s -cpu=1,4 -benchmem ./adapters/router/fiber/
go test -run=NONE -bench=BenchmarkEscapeLike -benchtime=1s -cpu=1,4 -benchmem ./orm/
go test -run=NONE -bench=BenchmarkFlattenGroupTerms -benchtime=1s -cpu=1,4 -benchmem ./orm/render/
go test -run=NONE -bench=BenchmarkReclaimStale -benchtime=1s -cpu=1,4 -benchmem ./adapters/queue/db/
go test -run=NONE -bench=. -benchtime=1s -cpu=1,4 -benchmem ./adapters/idempotency/redis/
go test -run=NONE -bench=. -benchtime=100x -cpu=1,4 -benchmem ./orm/
```

Each path is its own Go module; run from the repo root with the `go.work`
workspace active (or `go -C <dir> test ...` from inside the module).
`-run=NONE` skips functional tests so only benchmarks execute.

## Machine

- CPU: 11th Gen Intel(R) Core(TM) i5-11400H @ 2.70GHz, 6 cores / 12 threads
- Caches: L1d 288 KiB, L1i 192 KiB, L2 7.5 MiB, L3 12 MiB
- RAM: 16 GiB (15 GiB usable)
- OS/Go: linux/amd64, `go version go1.27.0 linux/amd64`

## Repro

1. Check out the same commit and ensure `go.work` resolves (`go env GOWORK`).
2. Run the commands above; each prints `goos`/`goarch`/`cpu` lines
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

Adaptor-isolation coverage:

- Router: `BenchmarkServeHTTPAdaptor`
  (`adapters/router/fiber/fiber_bench_test.go`) -- no-op handler over the
  pooled recorder, isolating the fasthttpadaptor wrapping from handler logic.

Round-trip coverage (in-process backends):

- Queue: `BenchmarkReclaimStale` (`adapters/queue/db/queue_bench_test.go`) --
  seeds a batch of expired claims, reclaims them (1 SELECT + 1 set-based
  UPDATE computing attempt+1 in the database), re-stales them with one
  multi-row UPDATE, repeats.
- Idempotency: `BenchmarkBegin` / `BenchmarkComplete`
  (`adapters/idempotency/redis/redis_bench_test.go`) -- fresh key per
  iteration over one shared miniredis, so every call takes the
  reservation-miss / upsert path.

Render-coverage (ORM):

- `BenchmarkEscapeLike` (`orm/column_bench_test.go`) -- LIKE-pattern
  escaping with the hoisted replacer.
- `BenchmarkFlattenGroupTermsFew` / `...Rollup` / `...Many`
  (`orm/render/agg_bench_test.go`) -- the lazy dedup: linear scan for few
  leaves, map fallback past the threshold.

Benchmarks use `b.Context()`; helpers assert `t.Helper` where applicable.
