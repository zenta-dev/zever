# transactions — savepoint-nested transactions with orm

A tiny double-entry money movement app (`schema/transactions.zen`): three
wallets and a transfers table, moved with `orm.WithNestedTx` — a
transparent savepoint wrapper around `db.WithTx`.

## What it demonstrates

| Concern | Where |
| --- | --- |
| Schema-first DSL, two entities in one module | [`schema/transactions.zen`](schema/transactions.zen) |
| `zever compile --backend=zenorm` — generated orm query builders, committed in-place | [`generated/zenorm/orm/gen/app/app.go`](generated/zenorm/orm/gen/app/app.go) |
| One transfer as a `WithNestedTx`: reads both balances in the tx, `UpdateTable(...).Set(...).Where(...)` debit, `Insert` of the transfer row, credit — then Commit/Rollback | [`cmd/app/main.go`](cmd/app/main.go) |
| Savepoint nesting: a `WithNestedTx` called inside another degrades to a SAVEPOINT (`sp_1`, `sp_2`, …) instead of `db.WithTx`'s "nested transaction not supported" error | [`cmd/app/main.go`](cmd/app/main.go) |
| Savepoint rollback undoing only the failed step's writes (debit + transfer row + credit all removed) while the outer transaction commits the successful sibling | [`cmd/app/main.go`](cmd/app/main.go) |
| Whole-transaction rollback on a genuinely failing top-level transfer (insufficient funds, an app-level error) | [`cmd/app/main.go`](cmd/app/main.go) |

`WithNestedTx` is transparent about which case it is in: with no transaction
in the context it behaves exactly like `db.WithTx` (Begin, `fn`, Commit on
success / Rollback on error), and nested under another `WithNestedTx` it
issues a SAVEPOINT whose `RELEASE`/`RollbackTo` never touches the outer
transaction. Any transfer function therefore works both on its own and as a
unit of a larger batch.

## Running it

```bash
go run ./examples/transactions/cmd/app
```

It opens an in-memory SQLite database, seeds three wallets, and prints:

```
after one top-level transfer (Ada -> Grace 1500):
  w-ada           Ada    8500 cents
  w-grace       Grace   21500 cents
  w-margaret Margaret   30000 cents
inner transfer failed and rolled back to its savepoint: simulated failure after debit, transfer row and credit
after nested batch (Grace -> Margaret 2500 committed, Margaret -> Grace 1000 rolled back):
  w-ada           Ada    8500 cents
  w-grace       Grace   19000 cents
  w-margaret Margaret   32500 cents
transfer rows: 2 total, 0 with note "simulated-failure" (the savepoint rollback removed the failed one)
top-level transfer of 999999 correctly failed: insufficient funds: Ada has 8500 cents, needs 999999
after the failed top-level transfer (unchanged):
  w-ada           Ada    8500 cents
  w-grace       Grace   19000 cents
  w-margaret Margaret   32500 cents
```

## Regenerating code from the schema

From the repo root:

```bash
go run ./cmd/zever compile examples/transactions/schema/transactions.zen --backend=zenorm --out examples/transactions/generated
```

`generated/` is committed CLI output — never hand-edit it.

## Tests

```bash
go build ./examples/transactions/...
go test -count=1 ./examples/transactions/...
```
