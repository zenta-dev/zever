package orm

import (
	"database/sql/driver"
	"fmt"
)

// JSONText is a JSON document stored as TEXT: the Go counterpart of the
// .zen `json` scalar and what the zenorm backend emits for `json` columns.
//
// database/sql cannot scan into encoding/json.RawMessage on recent Go
// toolchains (it aliases a string-kind type the driver rejects), so
// generated code uses this named string type instead. It binds as TEXT
// (never BLOB), preserving sqlite json1 function behavior, and marshals
// as raw JSON -- exactly like json.RawMessage -- so API wire shapes are
// unchanged.
//
// Value/MarshalJSON stay on values so driver.Valuer and encoding/json find
// them on non-addressable values.
//
//nolint:recvcheck // Scan/UnmarshalJSON mutate (pointer receivers) while
type JSONText string

// String returns the raw JSON document text.
func (j JSONText) String() string { return string(j) }

// Bytes returns the raw JSON document bytes.
func (j JSONText) Bytes() []byte { return []byte(j) }

// Scan implements database/sql.Scanner, accepting the TEXT ([]byte/string)
// values sqlite drivers return. A NULL src is a schema violation for a
// non-optional column and errors; nullable columns read through
// Option[JSONText].
func (j *JSONText) Scan(src any) error {
	switch v := src.(type) {
	case string:
		*j = JSONText(v)

		return nil
	case []byte:
		*j = JSONText(v)

		return nil
	default:
		return fmt.Errorf("orm: cannot scan %T into JSONText", src)
	}
}

// Value implements database/sql/driver.Valuer, binding the document as TEXT.
func (j JSONText) Value() (driver.Value, error) { return string(j), nil }

// MarshalJSON emits the document raw, like json.RawMessage. Empty emits
// null, matching encoding/json's own empty-RawMessage rendering.
func (j JSONText) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}

	return []byte(j), nil
}

// UnmarshalJSON copies the raw document; JSON null reads as empty.
func (j *JSONText) UnmarshalJSON(b []byte) error {
	if string(b) == "null" {
		*j = ""

		return nil
	}

	*j = JSONText(append([]byte(nil), b...))

	return nil
}

// scanJSONText coerces src into JSONText for convertScan's named-type case.
func scanJSONText(src any) (JSONText, error) {
	var j JSONText
	if err := j.Scan(src); err != nil {
		return "", err
	}

	return j, nil
}
