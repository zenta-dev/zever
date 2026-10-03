package orm

// Nullable is the arbitrary-type maybe-wrapper: it has the same present/
// absent API as Option (NullableSome/NullableNone plus IsSome/Get/GetOr) but
// is NEVER scanned, so T is unconstrained. Use Nullable for struct/entity
// values produced by outer joins (e.g. Row2[A, Nullable[B]]), where a row
// side is present or absent but its columns are read by the entity's own
// Scan, not by the wrapper. Use Option for a NULL-capable scan column, whose
// T must be a ScanValue.
//
// The zero value of Nullable[T] is absent (IsSome reports false).
type Nullable[T any] struct {
	v     T
	valid bool
}

// NullableSome builds a present Nullable holding v.
func NullableSome[T any](v T) Nullable[T] { return Nullable[T]{v: v, valid: true} }

// NullableNone builds an absent Nullable.
func NullableNone[T any]() Nullable[T] { return Nullable[T]{} }

// IsSome reports whether n holds a value.
func (n Nullable[T]) IsSome() bool { return n.valid }

// Get returns n's value and whether it was present. When ok is false, v is
// T's zero value.
func (n Nullable[T]) Get() (T, bool) { return n.v, n.valid }

// GetOr returns n's value if present, else fallback.
func (n Nullable[T]) GetOr(fallback T) T {
	if n.valid {
		return n.v
	}

	return fallback
}
