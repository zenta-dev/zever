# Error conventions

Canonical rules for error messages, sentinels, custom types, wrapping, and
branching in this repo. Enforced forward-looking by `make err-lint`
(`tools/errscan`); the guard is additive, not a cleanup of existing code.

## Message format

Every error message starts with `<package>: <message>` — a colon-space
prefix naming the package or adapter that emits it. No brackets, no trailing
period, no capitalization.

```go
// good
var ErrNotFound = errors.New("store: record not found")
return fmt.Errorf("store: get %q: %w", key, err)

// bad
var ErrNotFound = errors.New("record not found")         // unprefixed
var ErrNotFound = errors.New("[E404] record not found")  // bracket prefix
var ErrNotFound = errors.New("Store: record not found.") // capitalized, trailing period
```

## Sentinel errors

- Named `ErrXxx`, declared package-level, in `errors.go`.
- Doc comment required (exported identifier convention).
- Public API: renames and removals are breaking changes.

```go
// errors.go

// ErrNotFound is returned when a lookup matches no record.
var ErrNotFound = errors.New("store: record not found")
```

## Custom error types

- Named `FooError`; value receivers by default (error values stay comparable
  and safe to copy).
- Implement `Unwrap() error` when the type wraps another error.
- Carry structured data as fields; callers extract it with
  `errors.As`/`errors.AsType`.

```go
// ValidationError describes a single failed field check.
type ValidationError struct {
	Field  string
	Reason string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("validate: field %q: %s", e.Field, e.Reason)
}

func (e ValidationError) Unwrap() error { return ErrValidation }
```

## Wrapping

- Wrap with `%w` so `errors.Is`/`errors.As` keep working through the chain.
- Never put two `%w` verbs in one `fmt.Errorf` — combine a sentinel with a
  cause using `errors.Join` instead.
- Use `%v` only to deliberately break the chain at an abstraction boundary
  (the wrapped error is context, not something callers should match on).

```go
// good
return fmt.Errorf("store: get %q: %w", key, err)
return errors.Join(ErrNotFound, fmt.Errorf("store: get %q: %w", key, err))

// bad
return fmt.Errorf("store: %w: %w", ErrNotFound, err) // two %w verbs
```

## Multi-error

- `errors.Join` inside a single `Unwrap() error` is the default shape (e.g.
  `container.Close` joining per-service close errors).
- `Unwrap() []error` only when callers genuinely need element-wise access.

## Branching

- Branch with `errors.Is` and `errors.AsType` only. Never match on error
  strings, prefixes, or message content.

```go
if errors.Is(err, ErrNotFound) { ... }

var target *ValidationError
if errors.AsType[*ValidationError](err, &target) { ... }
```

Go 1.26+ `errors.AsType[E]` is preferred for new code; `errors.As` is not
deprecated and stays valid.

## Panics

- Panics are reserved for resolver-invariant guards in
  `dsl/backend/gogen/render_validate.go` (states the resolver makes
  unreachable). The parser itself never panics.
- Everything else returns an error.

## Boundaries and security

- Translate errors at package/service boundaries: map internal errors to
  safe, stable messages before they leave the process (HTTP responses, CLI
  output, RPC status).
- Never leak internal error text (DSNs, SQL, stack details, internal
  addresses) to external callers.
- Log once, structured, via `log/slog` or the repo logger; redact secrets
  (`config.Redact`, `redis.RedactAddr`) — never log raw option maps.

```go
func (s *Service) Handle(w http.ResponseWriter, r *http.Request) {
	if err := s.do(r.Context()); err != nil {
		s.log.ErrorContext(r.Context(), "service: handle", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
}
```

## Guard

`make err-lint` (`tools/errscan`) scans the repo and fails on:

- `unprefixed` — `errors.New`/`fmt.Errorf` literal with no `:` in its first
  token (skipped in `shared/apperror`, `_test.go` files).
- `bracket-prefix` — literal starting with `[...]` (outside
  `shared/apperror`, which emits `[CODE]` messages).
- `double-%w` — two or more `%w` in one `fmt.Errorf` format string.
- `panic` — `panic(...)` outside `dsl/backend/gogen/render_validate.go` and
  `_test.go` files.

Escape hatch: add an `// errscan:allow` comment on the offending line (or
anywhere in the file for a file-level allow).

## References

- [Go 1.13 errors: Working with Errors in Go](https://go.dev/blog/go1.13-errors)
- [errors package documentation](https://pkg.go.dev/errors)
- [Go Style Guide: Error handling](https://google.github.io/styleguide/go/decisions#error-handling)
