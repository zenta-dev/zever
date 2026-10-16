# Good first issues (draft pool)

> Do NOT file these on GitHub. This is a curated draft pool for
> maintainers to pick from. Each item is small-scope, makes no public
> API change, and is verifiable with existing tests.

Label convention: `good first issue` means small scope, no public API
change, verifiable with existing tests
(`.github/CONTRIBUTING.md:114`).

## 1. Cache conformance: empty-key TTL table cases

- Context: `core/cache/cachetest/conformance.go:48`
  (`Conformance(t, factory)` entry point).
- Task: add table rows covering empty-key and zero/negative-TTL
  `Set`/`Get` behavior against the existing in-kit suite only; no
  interface or adapter change.
- Acceptance: `go test ./core/cache/...` green; new rows fail
  meaningfully if the behavior regresses (assert on documented
  behavior, not timing).
- Size: XS.

## 2. Queue conformance: delayed-visibility case

- Context: `core/queue/queuetest/conformance.go:47`
  (`Conformance(t, factory)` entry point).
- Task: extend the delayed-delivery section with one deterministic
  case (already-due delay delivers on next `Pop`; far-future delay
  stays invisible), using the kit's polling helpers, never
  `time.Sleep` for sync.
- Acceptance: `go test ./core/queue/...` green.
- Size: S.

## 3. Registry: SortedNames determinism test

- Context: `shared/registry/registry.go:105` (`SortedNames`).
- Task: add a test registering several adapter names in scrambled
  order (per-invocation unique names; never fixed names in the
  package-global registry) and asserting sorted output.
- Acceptance: `go test ./shared/registry/...` green, including
  `-count=2`.
- Size: XS.

## 4. Registry: duplicate-register error assertion

- Context: `shared/registry/registry.go:30` (`New` with
  duplicate/unknown callbacks).
- Task: assert the duplicate-registration path returns the expected
  `DuplicateError` shape via `errors.As` in the battery registry
  using it (for example cache `core/cache/cache.go:48`), without
  touching exported API.
- Acceptance: `go test ./shared/registry/... ./core/cache/...` green.
- Size: XS.

## 5. Parser: max-nesting-depth diagnostic test

- Context: `dsl/parser/parser_attribute.go:169`
  (`maxValueDepth = 200`, enforced at `:182`).
- Task: add a parse test with deeply nested call expressions
  asserting the `max nesting depth exceeded` diagnostic fires and
  `ParseFile` still returns a non-nil file (best-effort contract,
  `dsl/parser/parser.go:1`).
- Acceptance: `go test ./dsl/parser/...` green.
- Size: S.

## 6. Compile: multi-file filename-sorted determinism test

- Context: `dsl/compile/compile.go:40` (`Compile(files, backends...)`).
- Task: add a determinism test compiling the same schema from a map
  with shuffled insertion order (or two files split differently)
  asserting identical output; no backend change.
- Acceptance: `go test ./dsl/compile/...` green.
- Size: S.

## 7. ORM dialect: fail-closed table case

- Context: `orm/dialect/dialect.go:52` (capability gates return
  typed `ErrUnsupportedByDialect`, never invalid SQL).
- Task: add one table row per currently ungated-but-tested capability
  asserting the sqlite dialect returns `ErrUnsupportedByDialect`
  via `errors.Is`; docs/comments only if a gate description is
  ambiguous.
- Acceptance: `go test ./orm/...` green (sqlite `:memory:` default).
- Size: S.

## 8. Config: redact nested-secret case

- Context: `config/redact.go:257` (`Redact`, single recursive pass).
- Task: add test cases for nested maps/slices under sensitive keys
  (secret replaced, non-secret siblings preserved, input map not
  mutated).
- Acceptance: `go test ./config/...` green.
- Size: XS.

## 9. Container close: struct-value dedup case

- Context: `container/close.go:27` (`dedupKey`: struct values never
  dedupe).
- Task: add a close test with two struct values embedding the same
  pointer asserting both close (regression guard for the documented
  rule in the comment above `dedupKey`).
- Acceptance: `go test ./container/...` green.
- Size: S.

## 10. Container: New(nil) closes cleanly example test

- Context: `container/container.go:125`
  (`New(cfg)`; nil cfg valid, closes cleanly).
- Task: add/extend an example test constructing `New(nil)` and
  closing without resolving anything, asserting no error and no
  opened connections.
- Acceptance: `go test ./container/...` green with `-race`.
- Size: XS.

## 11. Cache Open: unknown-adapter error assertion

- Context: `core/cache/cache.go:58` (`Open` resolves via registry).
- Task: assert `Open` with an unregistered adapter name returns the
  `UnknownAdapterError` shape (naming the missing `Register` call)
  via `errors.As`; test-only, unique adapter name per run.
- Acceptance: `go test ./core/cache/...` green.
- Size: XS.

## 12. Queue Open: unknown-adapter error assertion

- Context: `core/queue/queue.go:94` (`Open` resolves via registry).
- Task: same as above for the queue battery: unregistered name
  yields the unknown-adapter error; no interface change.
- Acceptance: `go test ./core/queue/...` green.
- Size: XS.
