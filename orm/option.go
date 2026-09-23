package orm

import (
	"database/sql/driver"
	"fmt"
	"time"
)

// Option is the scan-side representation of a nullable column: unlike
// NullableColumn (predicate-building, bare-V-only), Option[T] is what a
// codegen'd Scan method reads a NULL-capable column into. The two concepts
// are deliberately split: NullableColumn.Eq must not accept an Option[V],
// so "compare to NULL" is only ever spellable as IsNull()/IsNotNull().
//
// The zero value of Option[T] is None (IsSome reports false).
type Option[T any] struct {
	v     T
	valid bool
}

// Some builds a present Option holding v.
func Some[T any](v T) Option[T] { return Option[T]{v: v, valid: true} }

// None builds an absent Option.
func None[T any]() Option[T] { return Option[T]{} }

// IsSome reports whether o holds a value.
func (o Option[T]) IsSome() bool { return o.valid }

// Get returns o's value and whether it was present. When ok is false, v is
// T's zero value. See ExampleOption_Get for a runnable example.
func (o Option[T]) Get() (T, bool) { return o.v, o.valid }

// GetOr returns o's value if present, else fallback.
func (o Option[T]) GetOr(fallback T) T {
	if o.valid {
		return o.v
	}

	return fallback
}

// Scan implements database/sql.Scanner. A nil src means SQL NULL: o becomes
// None. Otherwise src is coerced into T using the same small set of
// conversions database/sql itself performs for common driver value types
// (string, []byte, the fixed-width integer/float kinds, bool, time.Time) --
// see convertScan below. T must be one of that closed set of
// driver-compatible types (plus orm.JSONText for `json` columns); anything
// else is a programmer error caught at scan time with a descriptive error,
// never a silent zero value.
func (o *Option[T]) Scan(src any) error {
	if src == nil {
		*o = Option[T]{}

		return nil
	}

	v, err := convertScan[T](src)
	if err != nil {
		return fmt.Errorf("orm: Option.Scan: %w", err)
	}

	*o = Option[T]{v: v, valid: true}

	return nil
}

// Value implements database/sql/driver.Valuer: an absent Option encodes as
// SQL NULL, a present one as its underlying value.
func (o Option[T]) Value() (driver.Value, error) {
	if !o.valid {
		return nil, nil //nolint:nilnil // driver.Valuer's documented NULL encoding
	}

	return driver.Value(o.v), nil
}

// convertScan coerces a driver-returned src into T, covering exactly the
// concrete types a codegen'd NullableColumn's underlying Go type can be:
// string, []byte, int64 and its narrower int32 alias, float64/float32,
// bool, and time.Time. Anything else -- including a genuinely unsupported T
// -- returns a clear error rather than reaching for reflection.
func convertScan[T any](src any) (T, error) {
	var zero T

	switch any(zero).(type) {
	case string:
		s, err := scanString(src)
		if err != nil {
			return zero, err
		}

		return any(s).(T), nil //nolint:forcetypeassert // guarded by the outer type switch
	case []byte:
		b, err := scanBytes(src)
		if err != nil {
			return zero, err
		}

		return any(b).(T), nil //nolint:forcetypeassert // guarded by the outer type switch
	case int64:
		n, err := scanInt64(src)
		if err != nil {
			return zero, err
		}

		return any(n).(T), nil //nolint:forcetypeassert // guarded by the outer type switch
	case int32:
		n, err := scanInt64(src)
		if err != nil {
			return zero, err
		}

		return any(int32(n)).(T), nil //nolint:forcetypeassert,gosec // guarded by the outer type switch; narrowing matches the driver-returned column width
	case float64:
		f, err := scanFloat64(src)
		if err != nil {
			return zero, err
		}

		return any(f).(T), nil //nolint:forcetypeassert // guarded by the outer type switch
	case float32:
		f, err := scanFloat64(src)
		if err != nil {
			return zero, err
		}

		return any(float32(f)).(T), nil //nolint:forcetypeassert // guarded by the outer type switch
	case bool:
		bv, err := scanBool(src)
		if err != nil {
			return zero, err
		}

		return any(bv).(T), nil //nolint:forcetypeassert // guarded by the outer type switch
	case time.Time:
		t, err := scanTime(src)
		if err != nil {
			return zero, err
		}

		return any(t).(T), nil //nolint:forcetypeassert // guarded by the outer type switch
	case JSONText:
		j, err := scanJSONText(src)
		if err != nil {
			return zero, err
		}

		return any(j).(T), nil //nolint:forcetypeassert // guarded by the outer type switch
	default:
		return zero, fmt.Errorf("orm: Option[%T] is not a supported scan type", zero)
	}
}

func scanString(src any) (string, error) {
	switch v := src.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	case time.Time:
		// A driver (modernc sqlite for a TIMESTAMP column, the postgres
		// driver for timestamptz) returns a time.Time for a timestamp.
		// Mirror database/sql's own convertAssign *string case exactly:
		// RFC3339Nano text with fractional seconds preserved, so a
		// generated Scan (which parses time.RFC3339) round-trips the same
		// text a plain rows.Scan into *string would have produced.
		return v.Format(time.RFC3339Nano), nil
	default:
		return "", fmt.Errorf("cannot scan %T into string", src)
	}
}

func scanBytes(src any) ([]byte, error) {
	switch v := src.(type) {
	case []byte:
		out := make([]byte, len(v))
		copy(out, v)

		return out, nil
	case string:
		return []byte(v), nil
	case time.Time:
		// Mirrors database/sql's convertAssign *[]byte case for a
		// time.Time source: its RFC3339Nano bytes.
		return v.AppendFormat(make([]byte, 0, len(time.RFC3339Nano)), time.RFC3339Nano), nil
	default:
		return nil, fmt.Errorf("cannot scan %T into []byte", src)
	}
}

func scanInt64(src any) (int64, error) {
	switch v := src.(type) {
	case int64:
		return v, nil
	case int32:
		return int64(v), nil
	case int:
		return int64(v), nil
	case float64:
		return int64(v), nil
	case []byte:
		return parseInt64(string(v))
	case string:
		return parseInt64(v)
	default:
		return 0, fmt.Errorf("cannot scan %T into int64", src)
	}
}

func parseInt64(s string) (int64, error) {
	var n int64

	_, err := fmt.Sscanf(s, "%d", &n)
	if err != nil {
		return 0, fmt.Errorf("cannot scan %q into int64: %w", s, err)
	}

	return n, nil
}

func scanFloat64(src any) (float64, error) {
	switch v := src.(type) {
	case float64:
		return v, nil
	case float32:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case []byte:
		return parseFloat64(string(v))
	case string:
		return parseFloat64(v)
	default:
		return 0, fmt.Errorf("cannot scan %T into float64", src)
	}
}

func parseFloat64(s string) (float64, error) {
	var f float64

	_, err := fmt.Sscanf(s, "%g", &f)
	if err != nil {
		return 0, fmt.Errorf("cannot scan %q into float64: %w", s, err)
	}

	return f, nil
}

func scanBool(src any) (bool, error) {
	switch v := src.(type) {
	case bool:
		return v, nil
	case int64:
		return v != 0, nil
	case int:
		return v != 0, nil
	case []byte:
		s := string(v)

		return s != "" && s != "false" && s != "0", nil
	default:
		return false, fmt.Errorf("cannot scan %T into bool", src)
	}
}

func scanTime(src any) (time.Time, error) {
	switch v := src.(type) {
	case time.Time:
		return v, nil
	case string:
		return parseTime(v)
	case []byte:
		return parseTime(string(v))
	default:
		return time.Time{}, fmt.Errorf("cannot scan %T into time.Time", src)
	}
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("cannot scan %q into time.Time: %w", s, err)
	}

	return t, nil
}
