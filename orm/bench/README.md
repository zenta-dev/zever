# orm/bench

Reproducible benchmark harness for a small fixed query set over sqlite
`:memory:`, comparing the zever ORM against hand-written SQL through the
same adapter (so the driver cost cancels out and the delta is ORM
overhead). No new dependencies: everything here uses the `orm` module's
existing requires.

## Query set

All benchmarks seed 1000 widgets (`w000000`..`w000999`) unless noted,
and report ns/op plus allocs/op (`-benchmem`):

- PointLookup: `WHERE id = ?` + `First` (one row).
- FilteredOrderLimit: `WHERE qty > ? ORDER BY qty DESC LIMIT 20` + `All`.
- Count: `WHERE qty > ?` + `Count`.
- Insert: single-row insert into an empty table (ids `new-<i>`).

The `Raw*` twins issue the same SQL text by hand over the same
`db.DB`, scanning into plain locals.

## Run

From the `orm` module directory:

```sh
go test ./bench/ -bench=. -benchmem -run='^$' -count=1
```

`results.txt` is the raw output of that command. Re-run it to reproduce;
expect noise of roughly +-10% run to run on shared hardware.

## Machine

- CPU: 11th Gen Intel i5-11400H @ 2.70GHz (12 threads, laptop)
- RAM: 16 GB, Linux amd64, go1.27.2

## Reading the numbers

On the recorded run the ORM tracks hand-written SQL within noise on
timing, at a small constant allocation overhead (roughly +6 allocs/op on
point lookups, +27 on the ordered page, +6 on count, +7 on insert) from
builder nodes and arg slices. No cross-tool claims are made here: GORM
and sqlc comparisons are explicitly unwired (see below).

## GORM / sqlc: TODO(verify)

- `compare_gorm_test.go` (`ormcompare` build tag): needs
  `gorm.io/gorm` plus a pure-Go sqlite driver added to `orm/go.mod`
  with a written justification, then one benchmark per shape above.
- `compare_sqlc_test.go` (`ormcompare` tag): needs `sqlc generate`
  output for the same four shapes checked in -- hand-writing
  "sqlc-equivalent" SQL would misrepresent the tool.

Run the tagged stubs with:

```sh
go test ./bench/ -tags ormcompare -run NotWired -v -count=1
```

Both skip until their dependencies land.
