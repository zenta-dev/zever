package orm

import (
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
	"time"
)

// CursorKeyValue is the closed set of value types a keyset cursor can pack
// into an opaque page token: exactly the driver-compatible scalar types
// orm's own scan machinery (Option.Scan/convertScan in orm/option.go)
// supports, so encode and decode are always total. The union uses
// underlying types (`~`), so a codegen'd enum column (a defined string
// type) and similar named scalar types satisfy it; `time.Time` is exact
// because a defined struct wrapping it has no stable text encoding here.
type CursorKeyValue interface {
	~string | ~[]byte | ~int64 | ~int32 | ~float64 | ~float32 | ~bool | time.Time
}

// CursorKey is one keyset position: the value of the last row of the
// previous page on a single ordering column, together with that column and
// the direction of travel. V is the ordering column's own value type
// (Column[T, V]), so a cursor can only ever be built from -- and decoded
// back into -- a matching column, and a mismatched value type never
// compiles.
//
// The columns/direction live in the key rather than being inferred from a
// Query's ORDER BY so a token is self-describing: Encode/DecodeCursor
// round-trip a complete position, and a token issued against one column
// cannot be silently replayed against another (DecodeCursor validates the
// encoded column against the one the caller passes).
//
// CursorKey is single-column. Multi-column keysets -- the common
// (sort_key, id) tie-breaker shape -- compose the same way
// examples/pagination hand-rolls them: build the tuple predicate with
// AfterTuple (or by hand from typed columns) and AND it into the Query
// alongside the single-column CursorKey when the sort key has duplicates,
// or drive the whole page with AfterTuple alone.
type CursorKey[T any, V CursorKeyValue] struct {
	column Column[T, V]
	value  V
	desc   bool
}

// NewCursorKey builds an ascending cursor over col positioned at v. Flip
// it with Desc for a descending sort; the matching ORDER BY term is
// OrderTerm, so the cursor and its Query's ordering can never diverge.
func NewCursorKey[T any, V CursorKeyValue](col Column[T, V], v V) CursorKey[T, V] {
	return CursorKey[T, V]{column: col, value: v}
}

// Desc flips k to descending (the page after k is "column < value"), the
// mirror of an ascending cursor's "column > value". It returns a new
// cursor, leaving k unchanged.
func (k CursorKey[T, V]) Desc() CursorKey[T, V] {
	k.desc = true

	return k
}

// Descending reports whether k travels in descending order.
func (k CursorKey[T, V]) Descending() bool { return k.desc }

// Value returns the cursor's position value.
func (k CursorKey[T, V]) Value() V { return k.value }

// OrderTerm returns the ORDER BY term matching k's column and direction
// (col.Asc() or col.Desc()), so the page Query's ordering always lines up
// with the keyset predicate NextPage applies.
func (k CursorKey[T, V]) OrderTerm() OrderTerm[T] {
	if k.desc {
		return k.column.Desc()
	}

	return k.column.Asc()
}

// Predicate builds the "strictly after k" WHERE clause for k's column and
// direction: column > value ascending, column < value descending. Compose
// it into a Query with .Where, or pass k straight to NextPage.
func (k CursorKey[T, V]) Predicate() Predicate[T] {
	if k.desc {
		return k.column.Lt(k.value)
	}

	return k.column.Gt(k.value)
}

// NextPage returns q restricted to the page strictly after k: k's keyset
// predicate is AND-ed into q's existing WHERE clause (if any), and q's
// ordering, LIMIT and OFFSET are left exactly as the caller set them --
// set ORDER BY via k.OrderTerm() and LIMIT yourself, e.g. through
// OffsetPage or q.Limit. For the first page, run q unchanged (no cursor
// yet). NextPage never mutates q.
func NextPage[T any, PT ptrScanner[T], V CursorKeyValue](q Query[T, PT], k CursorKey[T, V]) Query[T, PT] {
	return q.Where(k.Predicate())
}

// OffsetPage applies (page, pageSize) to q as LIMIT pageSize OFFSET
// (page-1)*pageSize, the classic number-based pagination shape, returning a
// new Query. page is 1-based and pageSize must be >= 1; anything else
// returns a "orm:" error rather than silently clamping.
func OffsetPage[T any, PT ptrScanner[T]](q Query[T, PT], page, pageSize int) (Query[T, PT], error) {
	if page < 1 {
		return q, fmt.Errorf("orm: OffsetPage: page %d out of range (must be >= 1)", page)
	}

	if pageSize < 1 {
		return q, fmt.Errorf("orm: OffsetPage: page size %d out of range (must be >= 1)", pageSize)
	}

	return q.Limit(pageSize).Offset((page - 1) * pageSize), nil
}

// AfterTuple builds the row-value keyset predicate "strictly after
// (cols[i] = values[i]) in a uniform-desc-or-asc ORDER BY over cols", for
// multi-column keysets whose ordering terms line up with cols. The
// predicate is the standard keyset OR-expansion -- for ascending,
// `c1 > v1 OR (c1 = v1 AND c2 > v2) OR (c1 = v1 AND c2 = v2 AND c3 > v3)
// ...` -- portable to every dialect orm supports, and composes with Query
// via .Where. values are bound positionally as SQL arguments; they must
// line up with cols and with the ORDER BY terms (all same direction).
//
// cols are type-erased AnyColumn[T] refs because a tuple genuinely mixes
// value types; each entry must still be a schema-derived column, never a
// caller string. The predicate renders unqualified, so it is meant for
// single-table queries (the keyset-pagination case).
func AfterTuple[T any](cols []AnyColumn[T], values []any, desc bool) (Predicate[T], error) {
	if len(cols) == 0 {
		return Predicate[T]{}, errors.New("orm: AfterTuple: no columns")
	}

	if len(cols) != len(values) {
		return Predicate[T]{}, fmt.Errorf("orm: AfterTuple: %d columns but %d values", len(cols), len(values))
	}

	relOp := Gt
	if desc {
		relOp = Lt
	}

	terms := make([]Predicate[T], 0, len(cols))

	for i := range cols {
		prefix := make([]Predicate[T], 0, i+1)

		for j := 0; j < i; j++ {
			prefix = append(prefix, Predicate[T]{n: Node{Kind: NBinary, Column: cols[j].Name(), Op: Eq, Value: values[j]}})
		}

		prefix = append(prefix, Predicate[T]{n: Node{Kind: NBinary, Column: cols[i].Name(), Op: relOp, Value: values[i]}})

		terms = append(terms, And(prefix...))
	}

	return Or(terms...), nil
}

// cursor token encoding: a binary, base64url (unpadded) page token.
//
//	[0] version byte
//	[1] flags (bit 0 = descending)
//	[2..] table name (uvarint length + bytes)
//	[..] column name (uvarint length + bytes)
//	[..] value (tag byte + payload, see encodeCursorValue)
//
// Versioning the layout means a future change can bump the version byte
// and decode old tokens with a clear error instead of garbage.
const (
	cursorVersion   byte = 1
	cursorTagString byte = 's'
	cursorTagBytes  byte = 'b'
	cursorTagInt64  byte = 'i'
	cursorTagInt32  byte = '4'
	cursorTagFloat  byte = 'd'
	cursorTagF32    byte = 'f'
	cursorTagBool   byte = 'o'
	cursorTagTime   byte = 't'
)

// Encode packs k into an opaque, URL-safe page token. The token carries
// the column and direction too, so it is self-describing: decoding it back
// (DecodeCursor) yields an identical cursor, and replaying it against a
// different column fails. Encode never panics and never returns an error
// for a CursorKeyValue-constrained value.
func (k CursorKey[T, V]) Encode() (string, error) {
	b := []byte{cursorVersion}

	if k.desc {
		b = append(b, 1)
	} else {
		b = append(b, 0)
	}

	b = appendCursorString(b, k.column.table)
	b = appendCursorString(b, k.column.name)

	payload, err := encodeCursorValue(any(k.value))
	if err != nil {
		return "", err
	}

	b = append(b, payload...)

	return base64.RawURLEncoding.EncodeToString(b), nil
}

// DecodeCursor unpacks a token produced by CursorKey.Encode back into a
// CursorKey[T, V] positioned at the same value, restricted to col: the
// token's encoded table+column must match col's, and its value tag must
// match V, or a "orm:" error is returned -- a token can never be replayed
// against the wrong column or reinterpreted as another value type. An
// empty token means "no cursor yet" (the first page) and returns ok=false.
func DecodeCursor[T any, V CursorKeyValue](token string, col Column[T, V]) (CursorKey[T, V], bool, error) {
	if token == "" {
		return CursorKey[T, V]{}, false, nil
	}

	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return CursorKey[T, V]{}, false, fmt.Errorf("orm: CursorKey.Decode: invalid token: %w", err)
	}

	if len(raw) < 2 || raw[0] != cursorVersion {
		return CursorKey[T, V]{}, false, fmt.Errorf("orm: CursorKey.Decode: unsupported token version %d", cursorVersionAt(raw))
	}

	rest := raw[2:]

	table, rest, err := readCursorString(rest)
	if err != nil {
		return CursorKey[T, V]{}, false, fmt.Errorf("orm: CursorKey.Decode: table name: %w", err)
	}

	name, rest, err := readCursorString(rest)
	if err != nil {
		return CursorKey[T, V]{}, false, fmt.Errorf("orm: CursorKey.Decode: column name: %w", err)
	}

	if table != col.table || name != col.name {
		return CursorKey[T, V]{}, false, fmt.Errorf(
			"orm: CursorKey.Decode: token is for column %q of table %q, not %q of table %q",
			name, table, col.name, col.table,
		)
	}

	if len(rest) < 1 {
		return CursorKey[T, V]{}, false, errors.New("orm: CursorKey.Decode: truncated token (no value)")
	}

	vType := reflect.TypeOf((*V)(nil)).Elem()

	want, err := cursorTagFor(vType)
	if err != nil {
		return CursorKey[T, V]{}, false, err
	}

	if rest[0] != want {
		return CursorKey[T, V]{}, false, fmt.Errorf(
			"orm: CursorKey.Decode: token value type %q does not match %v", rest[0], vType,
		)
	}

	val, err := decodeCursorValue(rest[0], rest[1:], vType)
	if err != nil {
		return CursorKey[T, V]{}, false, err
	}

	return CursorKey[T, V]{column: col, value: val.(V), desc: raw[1]&1 == 1}, true, nil //nolint:forcetypeassert // value tag was validated against V
}

func cursorVersionAt(raw []byte) byte {
	if len(raw) > 0 {
		return raw[0]
	}

	return 0
}

func appendCursorString(b []byte, s string) []byte {
	b = binary.AppendUvarint(b, uint64(len(s)))

	return append(b, s...)
}

func readCursorString(b []byte) (string, []byte, error) {
	n, sz := binary.Uvarint(b)
	if sz <= 0 {
		return "", nil, errors.New("bad length prefix")
	}

	b = b[sz:]

	if n > uint64(len(b)) {
		return "", nil, errors.New("truncated")
	}

	return string(b[:n]), b[n:], nil
}

// encodeCursorValue serializes v into its tag byte plus payload. v is one
// of the CursorKeyValue members; the kind dispatch uses reflection so a
// defined (enum) type's underlying kind is seen through. Exact time.Time is
// encoded as RFC3339Nano UTC text, the same text orm's sqlite path binds
// timestamps as. Tokens written before the Nano switch (plain RFC3339, no
// fraction) still decode, since RFC3339Nano parsing accepts them.
func encodeCursorValue(v any) ([]byte, error) {
	if t, ok := v.(time.Time); ok {
		s := t.UTC().Format(time.RFC3339Nano)
		b := []byte{cursorTagTime}
		b = appendCursorString(b, s)

		return b, nil
	}

	rv := reflect.ValueOf(v)

	switch rv.Kind() { //nolint:exhaustive // every other kind is rejected in default
	case reflect.String:
		s := rv.String()
		b := []byte{cursorTagString}
		b = appendCursorString(b, s)

		return b, nil
	case reflect.Slice:
		if rv.Type().Elem().Kind() != reflect.Uint8 {
			return nil, fmt.Errorf("orm: CursorKey.Encode: unsupported slice value type %T", v)
		}

		raw := rv.Bytes()
		cp := make([]byte, len(raw))
		copy(cp, raw)

		b := []byte{cursorTagBytes}
		b = appendCursorString(b, string(cp))

		return b, nil
	case reflect.Int64:
		b := []byte{cursorTagInt64}

		return binary.AppendVarint(b, rv.Int()), nil
	case reflect.Int32:
		b := []byte{cursorTagInt32}

		return binary.AppendVarint(b, rv.Int()), nil
	case reflect.Float64:
		b := []byte{cursorTagFloat}

		return binary.AppendUvarint(b, math.Float64bits(rv.Float())), nil
	case reflect.Float32:
		b := []byte{cursorTagF32}

		return binary.AppendUvarint(b, uint64(math.Float32bits(float32(rv.Float())))), nil //nolint:gosec // narrowing matches the type switch's own Float32 case
	case reflect.Bool:
		if rv.Bool() {
			return []byte{cursorTagBool, 1}, nil
		}

		return []byte{cursorTagBool, 0}, nil
	default:
		return nil, fmt.Errorf("orm: CursorKey.Encode: unsupported value type %T", v)
	}
}

// cursorTagFor maps V's Go type to the token value tag it encodes with,
// mirroring encodeCursorValue's kind dispatch. It is what makes the
// value-tag validation in DecodeCursor total: a caller decoding with the
// wrong V (e.g. string instead of int64) gets a clear error, never a
// reflect conversion panic.
func cursorTagFor(vType reflect.Type) (byte, error) {
	switch vType.Kind() { //nolint:exhaustive // every other kind is rejected in default
	case reflect.String:
		return cursorTagString, nil
	case reflect.Slice:
		if vType.Elem().Kind() == reflect.Uint8 {
			return cursorTagBytes, nil
		}
	case reflect.Int64:
		return cursorTagInt64, nil
	case reflect.Int32:
		return cursorTagInt32, nil
	case reflect.Float64:
		return cursorTagFloat, nil
	case reflect.Float32:
		return cursorTagF32, nil
	case reflect.Bool:
		return cursorTagBool, nil
	case reflect.Struct:
		if vType == reflect.TypeOf(time.Time{}) {
			return cursorTagTime, nil
		}
	}

	return 0, fmt.Errorf("orm: CursorKey.Decode: unsupported value type %v", vType)
}

// decodeCursorValue parses tag's payload into a value of vType. vType is
// guaranteed to match tag by the caller's cursorTagFor check, so the
// reflect conversions here cannot panic.
func decodeCursorValue(tag byte, payload []byte, vType reflect.Type) (any, error) {
	switch tag { //nolint:exhaustive // unknown tags rejected in default
	case cursorTagString, cursorTagBytes, cursorTagTime:
		return decodeCursorText(tag, payload, vType)
	case cursorTagInt64:
		n, sz := binary.Varint(payload)
		if sz <= 0 {
			return nil, errors.New("orm: CursorKey.Decode: corrupt int64 value")
		}

		return reflect.ValueOf(n).Convert(vType).Interface(), nil
	case cursorTagInt32:
		n, sz := binary.Varint(payload)
		if sz <= 0 {
			return nil, errors.New("orm: CursorKey.Decode: corrupt int32 value")
		}

		return reflect.ValueOf(int32(n)).Convert(vType).Interface(), nil //nolint:gosec // tag was validated as int32
	case cursorTagFloat:
		bits, sz := binary.Uvarint(payload)
		if sz <= 0 {
			return nil, errors.New("orm: CursorKey.Decode: corrupt float64 value")
		}

		return reflect.ValueOf(math.Float64frombits(bits)).Convert(vType).Interface(), nil
	case cursorTagF32:
		bits, sz := binary.Uvarint(payload)
		if sz <= 0 {
			return nil, errors.New("orm: CursorKey.Decode: corrupt float32 value")
		}

		return reflect.ValueOf(math.Float32frombits(uint32(bits))).Convert(vType).Interface(), nil //nolint:gosec // tag was validated as float32
	case cursorTagBool:
		if len(payload) != 1 {
			return nil, errors.New("orm: CursorKey.Decode: corrupt bool value")
		}

		return reflect.ValueOf(payload[0] == 1).Convert(vType).Interface(), nil
	default:
		return nil, fmt.Errorf("orm: CursorKey.Decode: unknown value tag %q", tag)
	}
}

// decodeCursorText parses the three text-encoded tags (string, []byte and
// time.Time), which all share a uvarint-length-prefixed payload.
func decodeCursorText(tag byte, payload []byte, vType reflect.Type) (any, error) {
	s, rest, err := readCursorString(payload)
	if err != nil {
		return nil, fmt.Errorf("orm: CursorKey.Decode: %q value: %w", tag, err)
	}

	if len(rest) != 0 {
		return nil, fmt.Errorf("orm: CursorKey.Decode: trailing bytes after %q value", tag)
	}

	switch tag {
	case cursorTagBytes:
		return reflect.ValueOf([]byte(s)).Convert(vType).Interface(), nil
	case cursorTagTime:
		t, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return nil, fmt.Errorf("orm: CursorKey.Decode: time value %q: %w", s, err)
		}

		return reflect.ValueOf(t).Convert(vType).Interface(), nil
	default:
		return reflect.ValueOf(s).Convert(vType).Interface(), nil
	}
}
