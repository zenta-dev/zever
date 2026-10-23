## Type Safety + DX (Track D)

### Type Safety

- **Eliminated all `forcetypeassert` linter suppressions in non-test code** (28 sites across 6 files):
  - `shared/lrucache` (15): strengthened nolint comments to document container invariants
  - `orm/option.go` (9): replaced forced assertions with safe comma-ok checks + `errors.New` for internal errors
  - `orm/inchunks.go` (2): replaced forced assertions with safe comma-ok checks + `errors.New`
  - `adapters/crypto/kms` + `local` (2): strengthened nolint comments to cite stdlib guarantee
- **Added compile-time interface satisfaction asserts** (`var _ Interface = (*Type)(nil)`) to 52 adapters that were missing them, catching interface drift at build time.

### DX Hardening

- **Verified error suggestion consistency**: `ParseAdapter` (permissive for custom adapters) + `config.ValidateAdapters` (registry-backed with did-you-mean) work as designed; no asymmetries found.
- **Confirmed doctor/CLI hint texts** cover all 36 batteries via `serviceEnvFields` (derived from `config.DefaultOptions`).
- **Added `docs/bench/README.md`** documenting the benchmark story: what has benchmarks, what doesn't (and why), how to run `benchstat` A/B comparisons locally, and anti-patterns to avoid.

### Linter Status

- `forcetypeassert`: clean (0 issues in non-test code)
- `perfsprint`: fixed 4 issues in `orm` package by using `errors.New` for internal errors