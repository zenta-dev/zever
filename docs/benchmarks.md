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

## Conventions

Every benchmark lives in a `*_bench_test.go` file next to the code it measures
and follows these rules:

- **Deterministic:** no network, no `time.Sleep` sync, no unseeded randomness,
  no timing assertions. In-process backends only (sqlite `:memory:`).
- **Allocation visibility:** `b.ReportAllocs()` on every benchmark.
- **Setup excluded:** `b.ResetTimer()` after warmup/setup; `b.Cleanup` for
  teardown (Open/Close).
- **Context:** `b.Context()` (or `b.Parallel`'s implicit context), never a
  stored context.
- **Concurrent types:** a `b.RunParallel` variant alongside the sequential one,
  with balanced work per iteration so memory stays flat.
- **Naming:** `Benchmark<Thing>` for the hot path, `Benchmark<Thing>Parallel`
  for the concurrent variant, `Benchmark<Thing>Before`/`After` only when
  documenting an optimization.
- **Target:** the operation the module exists to perform (e.g. `cache` Get/Set,
  `queue` Push/Pop, `codec` encode/decode, `orm` query build, `dsl` lex/parse).

Run all benchmarks with `make bench` (`go test -bench=. -benchmem ./...`) or
per module with `go -C <dir> test -run=NONE -bench=. -benchtime=1s -cpu=1,4 -benchmem`.

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

Queue (`adapters/queue/db`, stale-claim sweep over sqlite, second pass). The
sweep collapsed from two statements to one: the batch is now selected and
released by a single set-based UPDATE whose `IN` subquery projects the same
stale-claim set the standalone SELECT read (same lease guard, same id order,
same batch LIMIT), so the ids slice, the per-row `msgRow` scan, and the
SELECT-to-UPDATE window are gone. "before" is the two-statement sweep at
commit 8a7f3d9; "after" is the single-statement form. Medians of 10 runs
(`-count=10`), benchstat p=0.000.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkReclaimStale before (cpu=12) | 898,500 | 109,534 | 3,204 |
| BenchmarkReclaimStale after (cpu=12) | 405,700 | 10,834 | 130 |

The single set-based UPDATE cuts the sweep a further ~2.2x (898.5µs to
405.7µs): the SELECT's per-row driver work (cursor allocation, text
conversion, the 10-cell positional scan) and the ids slice disappear, and
the subquery plus outer guard evaluate atomically, so a row settled mid-sweep
is still skipped rather than double-bumped. B/op drops ~10x (107 KB to
10.6 KB) and allocs/op ~24x (3,204 to 130) with it. One tradeoff: an idle
sweep now issues a 0-row UPDATE instead of skipping the write entirely
(~52µs per sweep at the 250ms gate, ~0.2% CPU).

Rate limiter (`adapters/ratelimit/memory`, striped lock table). The bucket
map was sharded into 64 stripes (`DefaultStripeCount`) keyed by an FNV-1a
hash of the limiter key, each with its own `sync.RWMutex`. The global
`MaxEntries` bound is preserved exactly: new-key admission takes a short
`admitMu` to reserve a slot under the bound (evicting idle-first, then
least-recently-used, across stripes) before inserting into the stripe, so
existing-key calls never touch `admitMu`. "before" is the single
`sync.RWMutex` table at commit 9e3a0d0; "after" is the sharded table.
Medians of 3 runs, `-benchtime 2s`.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkAllowParallel before (cpu=1) | 86.57 | 0 | 0 |
| BenchmarkAllowParallel after (cpu=1) | 88.08 | 0 | 0 |
| BenchmarkAllowParallel before (cpu=4) | 122.2 | 0 | 0 |
| BenchmarkAllowParallel after (cpu=4) | 40.70 | 0 | 0 |
| BenchmarkAllowParallelHotKey before (cpu=1) | 75.00 | 0 | 0 |
| BenchmarkAllowParallelHotKey after (cpu=1) | 76.21 | 0 | 0 |
| BenchmarkAllowParallelHotKey before (cpu=4) | 112.4 | 0 | 0 |
| BenchmarkAllowParallelHotKey after (cpu=4) | 114.6 | 0 | 0 |

Sharding cuts the many-keys parallel path ~3x at cpu=4 (122.2 to 40.70
ns/op): before, every `Allow` serialized on one lock and cpu=4 was slower
than cpu=1; after, different keys proceed in parallel. The single-key
benchmark is unchanged (75.00 to 76.21 ns/op at cpu=1) because one key
still serializes on its stripe -- sharding helps key parallelism, not
same-key throughput. Allocations stay at 0 B/op, 0 allocs/op on every row.

Reproduce:

```
go test -run '^$' -bench 'BenchmarkAllowParallel' -benchtime 2s -cpu 1,4 -count 3 ./adapters/ratelimit/memory/
```

Machine: 11th Gen Intel Core i5-11400H @ 2.70GHz (6C/12T), 16 GB RAM,
go1.27.0 linux/amd64.

Idempotency (`adapters/idempotency/redis`, miniredis). `Begin` and
`Complete` are now one Lua script each (1 RTT) instead of two client calls.
Absolute numbers are dominated by miniredis's in-process command parsing.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkBegin (cpu=1) | 152,587 | 190,137 | 786 |
| BenchmarkBegin (cpu=4) | 128,635 | 190,249 | 786 |
| BenchmarkComplete (cpu=1) | 162,142 | 198,666 | 793 |
| BenchmarkComplete (cpu=4) | 133,553 | 198,746 | 793 |

Shared Redis registry (`shared/redisclient`, offline, 2026-10-04). `Shared`
now returns a refcounted client keyed by the resolved addr+DB+TLS/password
identity, so every Redis-backed battery on the same Redis shares one
client/pool. `BenchmarkSharedAcquireRelease` is a warm registry hit
(`toRedisOptions` + key + refcount + release) against a pre-seeded entry;
`BenchmarkPrivateClientBuild` is the per-battery `goredis.NewClient` the
registry avoids on every borrower after the first (idle-conn warming
disabled to stay offline). Medians of 3 runs, `-benchtime 1s`.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkSharedAcquireRelease (cpu=1) | 343 | 656 | 5 |
| BenchmarkSharedAcquireRelease (cpu=4) | 306 | 656 | 5 |
| BenchmarkPrivateClientBuild (cpu=1) | 10,155 | 8,786 | 71 |
| BenchmarkPrivateClientBuild (cpu=4) | 9,853 | 18,215 | 71 |

A shared hit is ~30x faster and ~14x fewer allocs than building a private
client (343 vs 10,155 ns/op, 5 vs 71 allocs/op at cpu=1): later borrowers
skip the client/pool construction entirely. The shared hit still allocates
its own `*goredis.Options` and key string per call; those are caller-local
and dwarfed by the pool build they replace.

Job worker (`core/job`, empty-poll backoff). `runLoop` used to call
`time.After(wait)` on every empty poll, allocating a fresh timer each time;
it now creates one `*time.Timer` for the run and `Reset`s it per poll,
stopping it on exit. `BenchmarkWorkerEmptyPoll` measures the first empty
poll in a run (attempt 0, wait 0). "before" is the `time.After` path.
Medians of 5 runs.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkWorkerEmptyPoll before (cpu=1) | 379.0 | 248 | 3 |
| BenchmarkWorkerEmptyPoll after (cpu=1) | 241.7 | 0 | 0 |
| BenchmarkWorkerEmptyPoll before (cpu=4) | 335.0 | 248 | 3 |
| BenchmarkWorkerEmptyPoll after (cpu=4) | 237.8 | 0 | 0 |

Reusing the timer removes the 3 allocs/op (timer, channel, runtime timer)
and 248 B/op from every idle poll, and cuts the wait path ~1.6x at cpu=1
(379.0 to 241.7 ns/op). Backoff values are unchanged: the first poll in a
run still waits 0, then 1ms, 2ms, 4ms, ..., capped at `DefaultMaxPollWait`.

ORM render group dedup (`orm/render`, sqlite). `dedupGroupLeaves`'s map
fallback keyed on a formatted `string` per leaf; it now keys on a comparable
`groupLeafKey{fn, col}` struct, removing the per-leaf key allocation. Medians
of 3 runs, cpu=12.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkFlattenGroupTermsMany before | 6,164 | 21,272 | 75 |
| BenchmarkFlattenGroupTermsMany after | 5,598 | 22,168 | 11 |

The struct key drops 64 allocs/op (one per leaf) on a 64-leaf GROUP BY and
cuts ns/op ~9%; B/op rises slightly (the keys are stored inline rather than
pointing at many tiny strings) but total allocation count — and thus GC
pressure — falls sharply.

Container resolve/close (`container`, sqlite shared-pool open+close). The
shutdown-critical subset (`DB`, `Cache`, `Queue`, `Scheduler`, `Job`) is
resolved from a cold container and closed. `Close` used to allocate a fresh
`chan error` and a capturing goroutine closure per service, and deduped with
a `sync.Map`; it now reuses one buffered result channel across successful
closes (reallocating only after a timeout abandons a goroutine), runs the
shutdown body as a top-level `closeAsync` (no closure per `go`), and dedups
with a local plain map created lazily. `logOpen` also drops its intermediate
`[...]` string concatenation. Medians of 5 runs, cpu=12.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkContainer_ResolveSubset before | 11,090 | 11,157 | 57 |
| BenchmarkContainer_ResolveSubset after | 10,884 | 10,289 | 37 |

The close-path rework drops 20 allocs/op (~35%): the per-service result
channel and goroutine closure disappear, `sync.Map`'s per-key entry nodes are
replaced by one small map, and the pool log line skips a string concat. ns/op
is dominated by `config.Default()` and the fresh `Container`, so the delta is
modest; B/op falls ~870 bytes/op.

SMTP `buildMIME` (`adapters/mailer/smtp`). The header block was written
with `fmt.Fprintf` (one printer + arg slice per header) and the buffer grew
unbounded; it now writes precomputed strings through `buf.Write`/`WriteString`
and grows once via `estimateSize`. `writeBase64` used to allocate
`encoded[i:end]+"\r\n"` per 76-byte line; it now encodes into a preallocated
`[]byte` and writes the line and CRLF separately. `joinAddresses` builds into
a `strings.Builder` (single-address fast path) instead of a `[]string` +
`strings.Join`. The generated MIME bytes are unchanged. Medians of 10 runs,
cpu=12.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkBuildMIME before | 8,206 | 5,892 | 102 |
| BenchmarkBuildMIME after | 6,711 | 6,421 | 90 |
| BenchmarkBuildMIMEAttachments before | 11,812 | 9,799 | 155 |
| BenchmarkBuildMIMEAttachments after | 9,332 | 6,933 | 135 |

The rework cuts allocs/op ~12% (102 to 90, 155 to 135) and ns/op ~19-21%.
B/op rises ~530 bytes/op on `BenchmarkBuildMIME` (the `estimateSize` hint
over-reserves header slack, still a single buffer allocation) and falls
~2.9 KB/op on `BenchmarkBuildMIMEAttachments` (the old growth path reallocated
the buffer ~7 times; the encoded attachment now lives in one preallocated
slice).

Mailer log (`adapters/mailer/log`, `io.Discard`, 2026-10-05). `Send` reuses
one `logMessage` (with its address/attachment slices) across calls under the
existing mutex, encodes into a reused `bytes.Buffer` via `json.MarshalWrite`
(passing the struct by pointer so `encoding/json/v2` skips its `reflect.New`
shallow copy and its `bytes.Clone` result copy), and writes the buffer plus
the newline byte in a single `Write`. `mailer.Address.Validate` was rewritten
to split on `@` and scan domain labels with `strings.IndexByte` instead of
`strings.SplitN`/`strings.Split`, dropping its per-call slices. The checker
constructor now allocates the reusable scratch state once (288 B, up from
48 B); the per-send path no longer allocates at all. "before" is the
`json.Marshal` + `append(data, '\n')` path. Medians of 10 runs.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkSend before (cpu=1) | 2,034 | 1,088 | 12 |
| BenchmarkSend after (cpu=1) | 1,462 | 0 | 0 |
| BenchmarkSendParallel before (cpu=1) | 2,280 | 1,090 | 12 |
| BenchmarkSendParallel after (cpu=1) | 1,599 | 0 | 0 |

`Send` drops 12 allocs/op to 0 and ~1 KiB/op to 0, and is ~28% faster at
cpu=1 (2,034 to 1,462 ns/op); the concurrent variant tracks it (2,280 to
1,599 ns/op). The per-call work was six allocs from `Address.Validate`
(`SplitN`+`Split` per recipient), two address slices, one attachment slice,
and the codec's `reflect.New` + `bytes.Clone` + `append` growth. All are now
reused or eliminated; output bytes are unchanged (verified against
`json.Marshal` + newline).

Reproduce:

```
go test -run '^$' -bench=. -benchmem -count=10 ./adapters/mailer/log/
```

TUI closest matcher (`cmd/zever`, 2026-10-05). `closest` used to allocate a
full `(la+1)x(lb+1)` DP matrix per candidate (21 top-level commands), and
`damerauLevenshtein` allocated the same matrix per call. The kernel now keeps
three rolling rows in one scratch buffer (`damerauLevenshteinScratch`) that
`closest` sizes to the longest candidate and reuses across the whole scan;
`damerauLevenshtein` delegates with a nil scratch. Distances, the chosen
match, and ordering are unchanged. Medians of 10 runs, cpu=12.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkClosest before | 8,611 | 14,008 | 176 |
| BenchmarkClosest after | 5,429 | 416 | 1 |
| BenchmarkDamerauLevenshtein before | 597 | 832 | 9 |
| BenchmarkDamerauLevenshtein after | 413 | 240 | 1 |

The scratch rows cut `BenchmarkClosest` from 176 to 1 alloc/op (-99.4%) and
14,008 to 416 B/op (-97.0%), and ~37% faster (8.6µs to 5.4µs); the standalone
kernel drops 9 to 1 allocs/op (832 to 240 B/op), ns/op within noise. The
remaining alloc is the single scratch slice per `closest` call.
Search (`adapters/search/db`, sqlite file-backed, 2026-10-05). `IndexBatch`
now writes the whole batch as one multi-row
`INSERT ... ON CONFLICT (id, idx) DO UPDATE SET content = excluded.content, metadata = excluded.metadata`
-- both dialects expose the proposed row as `excluded` in the DO UPDATE
clause, so one shared SET list carries every row's own replacement values --
instead of one typed upsert per document, and the write goes through the
`db.Preparer` probe so repeated batch shapes reuse the driver-side cached
statement. `Search` preallocates the hits slice at `limit` capacity and
aliases the count query's arg slice to the hits args' prefix (one backing
array, no realloc). "before" is the per-document orm loop / append-grown
hits path. Medians of 10 runs.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkIndexBatch before | 630,000 | 28,688 | 553 |
| BenchmarkIndexBatch after | 132,800 | 8,951 | 182 |
| BenchmarkSearch before | 379,700 | 11,170 | 253 |
| BenchmarkSearch after | 372,400 | 10,378 | 244 |

The multi-row upsert cuts `IndexBatch` ~4.7x (630 to 133 ns/op): the eight
per-document orm chains (Values/OnConflict/DoUpdate allocations, render,
arg encoding, per-statement exec) collapse into one statement, and
allocs/op drop 67% (553 to 182) with B/op down 69% (28.0 to 8.7 KiB). The
remaining per-document work is the metadata map clone plus JSON encode,
which is irreducible without changing the stored shape. `Search` drops
9 allocs/op (253 to 244) and ~790 B/op; its ns/op is dominated by the two
FTS5 queries and the per-hit JSON metadata decode, so the time delta is
within noise.

Vectorstore brute-force query (`adapters/vectorstore/db`, sqlite leg, 2026-10-05).
256-row scan with topK=10. `decodeRow` used to JSON-decode metadata for every
scanned row, but only the topK survivors leave the bounded heap (~246 wasted
decodes per query). The heap item now carries the raw metadata blob,
`heapToSorted` decodes metadata for the topK survivors only, and the heap is
preallocated with a bounded constant capacity. "before" is the per-row decode
path. Medians
of 10 runs, cpu=12.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkQuery before | 506,500 | 181,030 | 4,919 |
| BenchmarkQuery after | 260,500 | 84,930 | 3,439 |

Lazy decode cuts allocs/op ~30% (4,919 to 3,439) and ns/op ~49% (506.5µs to
260.5µs): each skipped decode was a `map[string]any` plus a JSON parse, and
the decode dominated the scan. B/op falls ~53% (176.8 KiB to 82.9 KiB).
Semantic note: corrupt metadata in a row that does not survive topK no longer
fails the query (`TestEdgeQuery_corruptMetadataBelowTopK` documents this); a
surviving row with corrupt metadata still fails.
Config redaction + env overlay (`config`, 2026-10-05). Three changes, all
semantics-preserving. (1) `isSensitiveKey` no longer allocates a
`strings.ToLower` copy per key: already-lowercase ASCII keys (the common case
for JSON-tagged option maps) match directly, mixed-case ASCII keys fold
in place (`asciiContainsFold`/`asciiEqualFold`), and only non-ASCII keys fall
back to `strings.ToLower`. (2) `Redact` merges the deep copy and the
reclamation walk into a single pass (`redactCopyMap`), so values under
sensitive keys are replaced instead of copied-then-dropped; the public
contract (fresh copy, caller map never mutated) is unchanged.
(3) `RedactedServices` redacts the freshly built `serviceToMap` output in
place via `redactMap` — the maps are owned by the result, so the deep copy
`Redact` used to perform was pure overhead. Separately, `applyEnv` used to
call `strings.ToLower` on the head and rest of every env var (218 vars in
the test environment, ~370 allocs/op); it now matches the head
case-insensitively against the 34 known service names without allocation
(`matchService`, first-byte candidate groups) and lowers the rest only on a
match, and `findField` compares normalized field names without allocation
(`normEqualFold`). Redaction output is byte-identical to the old
implementation (verified by the full config suite plus an equivalence check
over mixed-case keys, nested maps/slices, header lists, empty/nil values).
Medians of 10 runs, cpu=12.

| Benchmark | ns/op before | ns/op after | allocs/op before | allocs/op after |
| --- | --- | --- | --- | --- |
| BenchmarkRedactedServices | 112,600 | 74,060 | 597 | 539 |
| BenchmarkLoadFile | 64,380 | 39,170 | 572 | 202 |
| BenchmarkApplyEnv | 27,335 | 5,419 | 374 | 2 |
| BenchmarkRedact | 1,783 | 1,239 | 12 | 10 |

`ApplyEnv` drops 374 of its 374 allocs/op to 2 (the `os.Environ` slice and
the one lowered field name) and is ~5x faster; `LoadFile` inherits most of
that (572 to 202 allocs/op, ~39% faster) since the env overlay runs on every
load. `RedactedServices` is ~34% faster; its remaining allocs/op are the
JSON marshal/unmarshal bridge in `serviceToMap`, which stays marshal-based
on purpose to preserve each package's json-tag semantics. `Redact` drops
2 allocs/op (mixed-case header keys no longer allocate a lowered copy) and
~31% ns/op from the merged pass.
DSL lexer (`dsl/lexer`, 2026-10-05). `scanIllegal` builds the diagnostic
message and the ILLEGAL token literal into one stack buffer and materializes
them as a single string (a prior change, #355, already halved this path). The
rune → `(msg, lit)` string is a pure function, so it is now memoized in a
per-Lexer cache (`illegalCache`, 8 entries): repeated illegal runes share one
backing array, cutting the per-rune string alloc to zero on hits. The
`diag.Diagnostic` alloc is unavoidable (each is retained in `Lexer.errs`).
Other token literals -- ident, number, comment text -- are at floor: each needs
its own `string` and cannot be avoided without `unsafe`, which the lexer does
not use. Medians of 10 runs, `-count=10`.

| Benchmark | ns/op | B/op | allocs/op |
| --- | --- | --- | --- |
| BenchmarkLexIllegal before (cpu=12) | 1,885,509 | 1,471,879 | 18,019 |
| BenchmarkLexIllegal after (cpu=12) | 733,141 | 1,183,961 | 9,022 |
| BenchmarkLexAstral before (cpu=12) | 84,848 | 75,405 | 1,015 |
| BenchmarkLexAstral after (cpu=12) | 38,682 | 59,437 | 516 |

The memoization cuts `BenchmarkLexIllegal` ~2.6x (1.89M to 733K ns/op) and
~50% of allocs/op (18,019 to 9,022 -- the remaining 9,000 are the retained
`Diagnostic` structs); `BenchmarkLexAstral` tracks it (1,015 to 516 allocs/op,
~55% faster). Token output is byte-identical: same tokens, diagnostics, and
positions (golden fixtures under `dsl/compile/testdata` unchanged).

Reproduce:

```
go test -run '^$' -bench='BenchmarkClosest|BenchmarkDamerauLevenshtein' -benchmem -count=10 ./cmd/zever/
go test -run '^$' -bench='BenchmarkQuery$' -benchmem -count=10 ./adapters/vectorstore/db/
go test -run '^$' -bench=. -benchmem -count=10 ./adapters/search/db/
go test -run '^$' -bench=. -benchmem -count=10 ./config/
go test -run '^$' -bench='BenchmarkLexIllegal|BenchmarkLexAstral' -benchmem -count=10 ./dsl/lexer/
```

## Commands

Router and queue are fast, so they run 1s per bench; ORM runs 100
iterations per bench:

```sh
go test -run=NONE -bench=. -benchtime=1s -cpu=1,4 -benchmem ./adapters/router/fiber/
go test -run=NONE -bench=BenchmarkEscapeLike -benchtime=1s -cpu=1,4 -benchmem ./orm/
go test -run=NONE -bench=BenchmarkFlattenGroupTerms -benchtime=1s -cpu=1,4 -benchmem ./orm/render/
go test -run=NONE -bench=BenchmarkReclaimStale -benchtime=1s -cpu=1,4 -benchmem ./adapters/queue/db/
go test -run=NONE -bench=. -benchtime=1s -cpu=1,4 -benchmem ./adapters/idempotency/redis/
go test -run=NONE -bench='BenchmarkSharedAcquireRelease|BenchmarkPrivateClientBuild' -benchtime=1s -cpu=1,4 -benchmem ./shared/redisclient/
go test -run=NONE -bench=BenchmarkWorkerEmptyPoll -benchtime=1s -cpu=1,4 -benchmem ./core/job/
go test -run=NONE -bench=BenchmarkContainer_ResolveSubset -benchtime=200ms -count=5 -benchmem ./container/
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
- Rate limiter: `BenchmarkAllowParallel` / `BenchmarkAllowParallelHotKey`
  (`adapters/ratelimit/memory/memory_bench_test.go`) -- 256 precomputed
  keys cycled per iteration over one shared limiter (many-keys), and one
  shared key (hot-key, lock-contention shape).
- Mailer log: `BenchmarkSendParallel`
  (`adapters/mailer/log/log_bench_test.go`) -- one shared checker over
  `io.Discard`, exercising the mutex-guarded encode/write path.

Adaptor-isolation coverage:

- Router: `BenchmarkServeHTTPAdaptor`
  (`adapters/router/fiber/fiber_bench_test.go`) -- no-op handler over the
  pooled recorder, isolating the fasthttpadaptor wrapping from handler logic.

Round-trip coverage (in-process backends):

- Queue: `BenchmarkReclaimStale` (`adapters/queue/db/queue_bench_test.go`) --
  seeds a batch of expired claims, reclaims them (one set-based UPDATE whose
  IN subquery selects the batch, computing attempt+1 in the database),
  re-stales them with one multi-row UPDATE, repeats.
- Idempotency: `BenchmarkBegin` / `BenchmarkComplete`
  (`adapters/idempotency/redis/redis_bench_test.go`) -- fresh key per
  iteration over one shared miniredis, so every call takes the
  reservation-miss / upsert path.
- Shared Redis registry: `BenchmarkSharedAcquireRelease` /
  `BenchmarkPrivateClientBuild` (`shared/redisclient/shared_test.go`) --
  warm refcounted acquire/release vs a fresh per-battery client build,
  both offline.
- Job worker: `BenchmarkWorkerEmptyPoll` (`core/job/worker_bench_test.go`)
  -- the first empty poll in a run (attempt 0, wait 0) with a reused timer,
  so the timer allocation shows up directly in allocs/op.
- Vectorstore: `BenchmarkQuery` (`adapters/vectorstore/db/pgvector_bench_test.go`)
  -- 256-row brute-force sqlite scan through a bounded top-K heap, metadata
  decoded for survivors only.

Render-coverage (ORM):

- `BenchmarkEscapeLike` (`orm/column_bench_test.go`) -- LIKE-pattern
  escaping with the hoisted replacer.
- `BenchmarkFlattenGroupTermsFew` / `...Rollup` / `...Many`
  (`orm/render/agg_bench_test.go`) -- the lazy dedup: linear scan for few
  leaves, map fallback past the threshold.

Benchmarks use `b.Context()`; helpers assert `t.Helper` where applicable.
