package observability

import "strings"

// Limits for attribute keys, values, and counts.
const (
	// MaxKeyLen is the maximum allowed attribute key length.
	MaxKeyLen = 256
	// MaxValueLen is the default maximum allowed string attribute value length.
	MaxValueLen = 4096
	// MaxAttrs is the maximum number of attributes kept by normalizeAttrs.
	MaxAttrs = 32
)

// AttributeValue is a sealed attribute value type.
type AttributeValue interface {
	isAttributeValue()
}

// StringValue carries a string attribute value.
type StringValue struct {
	Value string
}

// Int64Value carries an integer attribute value.
type Int64Value struct {
	Value int64
}

// Float64Value carries a float attribute value.
type Float64Value struct {
	Value float64
}

// BoolValue carries a boolean attribute value.
type BoolValue struct {
	Value bool
}

func (StringValue) isAttributeValue()  {}
func (Int64Value) isAttributeValue()   {}
func (Float64Value) isAttributeValue() {}
func (BoolValue) isAttributeValue()    {}

// StringAttr returns a string attribute value.
func StringAttr(val string) StringValue { return StringValue{Value: val} }

// IntAttr returns an integer attribute value.
func IntAttr(val int) Int64Value { return Int64Value{Value: int64(val)} }

// Int64Attr returns an int64 attribute value.
func Int64Attr(val int64) Int64Value { return Int64Value{Value: val} }

// Float64Attr returns a float64 attribute value.
func Float64Attr(val float64) Float64Value { return Float64Value{Value: val} }

// BoolAttr returns a boolean attribute value.
func BoolAttr(val bool) BoolValue { return BoolValue{Value: val} }

// Attr is a single key-value pair attached to spans and metrics.
type Attr struct {
	Key   string
	Value AttributeValue
}

// String returns a string-valued attribute.
func String(key, val string) Attr {
	return Attr{Key: key, Value: StringAttr(val)}
}

// Int returns an int-valued attribute.
func Int(key string, val int) Attr {
	return Attr{Key: key, Value: IntAttr(val)}
}

// Int64 returns an int64-valued attribute.
func Int64(key string, val int64) Attr {
	return Attr{Key: key, Value: Int64Attr(val)}
}

// Float64 returns a float64-valued attribute.
func Float64(key string, val float64) Attr {
	return Attr{Key: key, Value: Float64Attr(val)}
}

// Bool returns a bool-valued attribute.
func Bool(key string, val bool) Attr {
	return Attr{Key: key, Value: BoolAttr(val)}
}

// redact reports whether key carries sensitive data needing redaction.
func redact(key string) bool {
	lowered := strings.ToLower(key)
	switch {
	case strings.Contains(lowered, "password"),
		strings.Contains(lowered, "secret"),
		strings.Contains(lowered, "token"),
		strings.Contains(lowered, "api_key"),
		strings.Contains(lowered, "apikey"),
		strings.Contains(lowered, "auth"),
		strings.Contains(lowered, "cookie"),
		strings.Contains(lowered, "session"),
		strings.Contains(lowered, "private_key"),
		strings.Contains(lowered, "privatekey"):
		return true
	default:
		return false
	}
}

// normalizeAttrs caps attribute count, truncates overlong strings, and redacts secrets.
func normalizeAttrs(attrs []Attr, limit int) []Attr {
	effective := limit
	if effective <= 0 {
		effective = MaxValueLen
	}

	n := len(attrs)
	if n > MaxAttrs {
		n = MaxAttrs
	}
	out := make([]Attr, 0, n)
	for i := 0; i < n; i++ {
		a := attrs[i]
		if len(a.Key) > MaxKeyLen {
			a.Key = a.Key[:MaxKeyLen]
		}
		if redact(a.Key) {
			a.Value = StringAttr("[redacted]")
			out = append(out, a)
			continue
		}
		if v, ok := a.Value.(StringValue); ok {
			if len(v.Value) > effective {
				a.Value = StringAttr(v.Value[:effective])
			}
		}
		out = append(out, a)
	}
	return out
}
