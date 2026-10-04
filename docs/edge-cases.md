# Edge-case test checklist

Manual per-module review uses this checklist. For each module, audit the
existing `*_test.go` files against every applicable row and add table-driven
cases (using the repo's `stub*`/`must*` helpers, `t.Parallel` where safe,
`t.Context()`, `errors.Is`/`As` assertions) for any gap.

## Categories

### Error paths
- Malformed input (bad encoding, invalid syntax, wrong types)
- Invalid options (zero/negative where disallowed, unknown enum values)
- `Open` / constructor failure (bad DSN, unreachable host, bad credentials)
- Unsupported dialect / feature (`dialect.ErrUnsupportedByDialect`)
- Use of a closed resource (after `Close`/`Stop`/`Shutdown`)
- Context cancellation / timeout mid-operation

### Boundaries
- Empty string, empty slice/map, nil slice/map
- Zero value, negative value, max int, overflow
- Empty collection, single element, duplicate keys
- Pagination: empty result, partial page, full page, page past end
- String length: empty, 1, very long (e.g. 1 MB)

### Concurrency
- Parallel access from multiple goroutines (`t.Parallel` + `b.RunParallel`)
- Race detector clean (`go test -race`)
- Cleanup/teardown (no leaked goroutines — `goleak` where enforced)
- Context cancel while operation in flight
- Double Close, Close before Open

### Lifecycle
- Open -> Close -> re-Open
- Double Close (idempotent or error, per contract)
- Use-after-Close returns error, not panic
- Idempotency (same operation twice)
- Retry exhaustion (all attempts fail)

### State
- Empty store, populated store
- Duplicate key insert (upsert vs error)
- TTL expiry (entry disappears after expiry)
- Concurrent writers, read-your-writes

### Determinism
- No `time.Sleep` for synchronization
- No network calls
- No unseeded randomness
- No timing assertions

## How to audit

1. Read the module's public interface (core battery or adapter options).
2. Read the existing `*_test.go` files.
3. For each checklist row above, mark **covered** or **gap**.
4. For each gap, add a focused table-driven test.
5. Run `go test -race ./...` in the module; confirm green.
6. Run `gofmt`, `go vet`.

## Priority

Batteries whose failure is security- or data-loss-relevant (auth, crypto,
payment, db, queue, storage, secrets) get the full checklist. Batteries that
are pure helpers (log, noop, static) get error-path + boundary rows only.
