package log

import "time"

// FieldType identifies the kind of value carried by a Field.
type FieldType uint8

const (
	// StringType marks a string field.
	StringType FieldType = iota
	// IntType marks an int field stored as int64.
	IntType
	// Int64Type marks an int64 field.
	Int64Type
	// Float64Type marks a float64 field.
	Float64Type
	// BoolType marks a bool field.
	BoolType
	// DurationType marks a time.Duration field.
	DurationType
	// TimeType marks a time.Time field.
	TimeType
	// ErrorType marks an error field.
	ErrorType
	// AnyType marks an arbitrary value field.
	AnyType
)

// Field is a single structured key-value pair attached to an event or context.
type Field struct {
	// Key is the field name.
	Key string
	// Type identifies the kind of value carried.
	Type FieldType
	// String holds the string value when Type is StringType.
	String string
	// Int64 holds the int or int64 value when Type is IntType or Int64Type.
	Int64 int64
	// Float64 holds the float64 value when Type is Float64Type.
	Float64 float64
	// Bool holds the bool value when Type is BoolType.
	Bool bool
	// Duration holds the duration value when Type is DurationType.
	Duration time.Duration
	// Time holds the time value when Type is TimeType.
	Time time.Time
	// Err holds the error value when Type is ErrorType.
	Err error
	// Any holds the arbitrary value when Type is AnyType.
	Any any
}

// String returns a string-valued field.
func String(key, val string) Field {
	return Field{Key: key, Type: StringType, String: val}
}

// Int returns an int-valued field.
func Int(key string, val int) Field {
	return Field{Key: key, Type: IntType, Int64: int64(val)}
}

// Int64 returns an int64-valued field.
func Int64(key string, val int64) Field {
	return Field{Key: key, Type: Int64Type, Int64: val}
}

// Float64 returns a float64-valued field.
func Float64(key string, val float64) Field {
	return Field{Key: key, Type: Float64Type, Float64: val}
}

// Bool returns a bool-valued field.
func Bool(key string, val bool) Field {
	return Field{Key: key, Type: BoolType, Bool: val}
}

// Duration returns a duration-valued field.
func Duration(key string, val time.Duration) Field {
	return Field{Key: key, Type: DurationType, Duration: val}
}

// Time returns a time-valued field.
func Time(key string, val time.Time) Field {
	return Field{Key: key, Type: TimeType, Time: val}
}

// Err returns an error-valued field under the conventional "error" key.
func Err(val error) Field {
	return Field{Key: "error", Type: ErrorType, Err: val}
}

// ErrKey returns an error-valued field under a custom key.
func ErrKey(key string, val error) Field {
	return Field{Key: key, Type: ErrorType, Err: val}
}

// Any returns a field carrying an arbitrary value.
func Any(key string, val any) Field {
	return Field{Key: key, Type: AnyType, Any: val}
}
