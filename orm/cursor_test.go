package orm

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/zenta-dev/zever/db"
)

// cursorRow is a fixture entity with one column per CursorKeyValue member,
// so encode/decode round-trips can be exercised for every supported type.
type cursorRow struct {
	ID    string
	Num64 int64
	Num32 int32
	F64   float64
	F32   float32
	OK    bool
	At    time.Time
	Blob  []byte
}

var (
	cursorRowID     = NewColumn[cursorRow, string]("cursor_rows", "id")
	cursorRowNameTC = NewColumn[cursorRow, string]("cursor_rows", "name")
	cursorRowNum64  = NewColumn[cursorRow, int64]("cursor_rows", "num64")
	cursorRowNum32  = NewColumn[cursorRow, int32]("cursor_rows", "num32")
	cursorRowF64    = NewColumn[cursorRow, float64]("cursor_rows", "f64")
	cursorRowF32    = NewColumn[cursorRow, float32]("cursor_rows", "f32")
	cursorRowOK     = NewColumn[cursorRow, bool]("cursor_rows", "ok")
	cursorRowAt     = NewColumn[cursorRow, time.Time]("cursor_rows", "at")
	cursorRowBlob   = NewColumn[cursorRow, []byte]("cursor_rows", "blob")
	enumRowID       = NewColumn[cursorRow, enumID]("cursor_rows", "id")
)

// enumID is a defined string type, proving the ~underlying union in
// CursorKeyValue accepts codegen'd enum columns.
type enumID string

// seedCursorWidgets returns newWidgetsDB plus three extra widgets (w4..w6),
// giving six ordered rows for pagination walks.
func seedCursorWidgets(t *testing.T) (context.Context, db.DB) {
	t.Helper()

	ctx, conn := newWidgetsDB(t)

	extra := []struct {
		id, name string
		qty      int64
	}{
		{"w4", "Delta", 40},
		{"w5", "Epsilon", 50},
		{"w6", "Zeta", 60},
	}

	for _, r := range extra {
		if _, err := conn.Exec(ctx, `INSERT INTO widgets (id, name, quantity, bio) VALUES (?, ?, ?, ?)`, r.id, r.name, r.qty, nil); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}

	return ctx, conn
}

func assertWidgetIDs(t *testing.T, rows []*widget, want ...string) {
	t.Helper()

	if len(rows) != len(want) {
		t.Fatalf("got %d rows (%v), want %d (%v)", len(rows), rowIDs(rows), len(want), want)
	}

	got := make([]string, 0, len(rows))
	for _, w := range rows {
		got = append(got, w.ID)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("rows = %q, want %q", got, want)
	}
}

func rowIDs(rows []*widget) []string {
	out := make([]string, len(rows))
	for i, w := range rows {
		out[i] = w.ID
	}

	return out
}

func assertCursorRoundTrip[V CursorKeyValue](t *testing.T, col Column[cursorRow, V], val V) {
	t.Helper()

	k := NewCursorKey(col, val)
	token, err := k.Encode()
	if err != nil {
		t.Fatalf("Encode(%v): %v", val, err)
	}

	got, ok, err := DecodeCursor(token, col)
	if err != nil {
		t.Fatalf("DecodeCursor(%q): %v", token, err)
	}

	if !ok {
		t.Fatalf("DecodeCursor(%q): ok=false, want true", token)
	}

	if !reflect.DeepEqual(got.Value(), val) {
		t.Fatalf("round-trip value = %v (%T), want %v (%T)", got.Value(), got.Value(), val, val)
	}
}

func TestCursorKeyEncodeDecodeRoundTrip(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 30, 0, 0, time.UTC)

	assertCursorRoundTrip(t, cursorRowID, "hello")
	assertCursorRoundTrip(t, cursorRowNum64, int64(-42))
	assertCursorRoundTrip(t, cursorRowNum32, int32(7))
	assertCursorRoundTrip(t, cursorRowF64, 3.5)
	assertCursorRoundTrip(t, cursorRowF32, float32(1.25))
	assertCursorRoundTrip(t, cursorRowOK, true)
	assertCursorRoundTrip(t, cursorRowAt, at)
	assertCursorRoundTrip(t, cursorRowBlob, []byte{0x00, 0x01, 0xff})
}

func TestCursorKeyDefinedValueTypeRoundTrip(t *testing.T) {
	assertCursorRoundTrip(t, enumRowID, enumID("hello"))
}

func TestCursorKeyDescRoundTrip(t *testing.T) {
	k := NewCursorKey(cursorRowID, "hello").Desc()

	token, err := k.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	got, ok, err := DecodeCursor(token, cursorRowID)
	if err != nil {
		t.Fatalf("DecodeCursor: %v", err)
	}

	if !ok {
		t.Fatalf("DecodeCursor: ok=false, want true")
	}

	if !got.Descending() {
		t.Fatalf("round-trip Descending = false, want true")
	}

	if got.Value() != "hello" {
		t.Fatalf("round-trip value = %q, want %q", got.Value(), "hello")
	}
}

func TestDecodeCursorEmptyTokenIsFirstPage(t *testing.T) {
	_, ok, err := DecodeCursor("", cursorRowID)
	if err != nil {
		t.Fatalf("DecodeCursor(\"\"): %v", err)
	}

	if ok {
		t.Fatalf("DecodeCursor(\"\"): ok=true, want false (first page)")
	}
}

func TestDecodeCursorRejectsCorruptToken(t *testing.T) {
	if _, _, err := DecodeCursor("!!! not base64 !!!", cursorRowID); err == nil {
		t.Fatalf("DecodeCursor(corrupt): nil error, want error")
	}
}

func TestDecodeCursorRejectsWrongColumn(t *testing.T) {
	token, err := NewCursorKey(cursorRowID, "hello").Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	if _, _, err := DecodeCursor(token, cursorRowNameTC); err == nil {
		t.Fatalf("DecodeCursor with wrong column: nil error, want error")
	}
}

func TestDecodeCursorRejectsWrongValueType(t *testing.T) {
	token, err := NewCursorKey(cursorRowID, "hello").Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	if _, _, err := DecodeCursor(token, cursorRowNum64); err == nil {
		t.Fatalf("DecodeCursor with wrong value type: nil error, want error")
	}
}

func TestCursorKeyPredicateDirections(t *testing.T) {
	asc := NewCursorKey(widgetID, "w5").Predicate()
	if !asc.IsSet() || asc.n.Op != Gt {
		t.Fatalf("ascending predicate op = %v, want Gt", asc.n.Op)
	}

	desc := NewCursorKey(widgetID, "w5").Desc().Predicate()
	if !desc.IsSet() || desc.n.Op != Lt {
		t.Fatalf("descending predicate op = %v, want Lt", desc.n.Op)
	}
}

func TestCursorKeyOrderTerm(t *testing.T) {
	k := NewCursorKey(widgetID, "w5")
	ot := k.OrderTerm()

	if ot.Desc || ot.Column.Name() != "id" {
		t.Fatalf("ascending OrderTerm = %+v, want ascending id", ot)
	}

	if otd := k.Desc().OrderTerm(); !otd.Desc || otd.Column.Name() != "id" {
		t.Fatalf("descending OrderTerm = %+v, want descending id", otd)
	}
}

func TestCursorNextPageAscending(t *testing.T) {
	ctx, conn := seedCursorWidgets(t)

	base := From(widgets).OrderBy(widgetID.Asc()).Limit(2)

	page1, err := base.All(ctx, conn)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	assertWidgetIDs(t, page1, "w1", "w2")

	next := NewCursorKey(widgetID, page1[len(page1)-1].ID)

	page2, err := NextPage(base, next).All(ctx, conn)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	assertWidgetIDs(t, page2, "w3", "w4")
}

func TestCursorNextPageDescending(t *testing.T) {
	ctx, conn := seedCursorWidgets(t)

	base := From(widgets).OrderBy(widgetID.Desc()).Limit(2)

	page1, err := base.All(ctx, conn)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	assertWidgetIDs(t, page1, "w6", "w5")

	next := NewCursorKey(widgetID, page1[len(page1)-1].ID).Desc()

	page2, err := NextPage(base, next).All(ctx, conn)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	assertWidgetIDs(t, page2, "w4", "w3")
}

// TestCursorWalkAllPages mirrors the pagination example's keyset walk: page
// after page through an opaque cursor until a short page ends the loop, with
// every row seen exactly once.
func TestCursorWalkAllPages(t *testing.T) {
	ctx, conn := seedCursorWidgets(t)

	var (
		seen  []string
		after Option[string]
	)

	for {
		q := From(widgets).OrderBy(widgetID.Asc()).Limit(2)
		if v, ok := after.Get(); ok {
			q = NextPage(q, NewCursorKey(widgetID, v))
		}

		page, err := q.All(ctx, conn)
		if err != nil {
			t.Fatalf("page: %v", err)
		}

		if len(page) == 0 {
			break
		}

		for _, w := range page {
			seen = append(seen, w.ID)
		}

		after = Some(page[len(page)-1].ID)

		if len(page) < 2 {
			break
		}
	}

	if !reflect.DeepEqual(seen, []string{"w1", "w2", "w3", "w4", "w5", "w6"}) {
		t.Fatalf("keyset walk saw %v, want w1..w6 exactly once each", seen)
	}
}

func TestCursorNextPageWithExistingWhere(t *testing.T) {
	ctx, conn := seedCursorWidgets(t)

	base := From(widgets).Where(widgetQty.Gte(int64(20))).OrderBy(widgetID.Asc()).Limit(1)

	page, err := NextPage(base, NewCursorKey(widgetID, "w1")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	assertWidgetIDs(t, page, "w2")
}

func TestOffsetPage(t *testing.T) {
	ctx, conn := seedCursorWidgets(t)

	page2, err := OffsetPage(From(widgets).OrderBy(widgetID.Asc()), 2, 2)
	if err != nil {
		t.Fatalf("OffsetPage: %v", err)
	}

	rows, err := page2.All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	assertWidgetIDs(t, rows, "w3", "w4")
}

func TestOffsetPageRejectsInvalidBounds(t *testing.T) {
	if _, err := OffsetPage(From(widgets), 0, 5); err == nil {
		t.Fatalf("OffsetPage(page=0): nil error, want error")
	}

	if _, err := OffsetPage(From(widgets), 1, 0); err == nil {
		t.Fatalf("OffsetPage(pageSize=0): nil error, want error")
	}

	if _, err := OffsetPage(From(widgets), -1, 5); err == nil {
		t.Fatalf("OffsetPage(page=-1): nil error, want error")
	}
}

func TestAfterTupleMultiColumnKeyset(t *testing.T) {
	ctx, conn := newWidgetsDB(t)

	for _, r := range []struct {
		id  string
		qty int64
	}{
		{"w4", 20},
		{"w5", 20},
	} {
		if _, err := conn.Exec(ctx, `INSERT INTO widgets (id, name, quantity, bio) VALUES (?, ?, ?, ?)`, r.id, "x", r.qty, nil); err != nil {
			t.Fatalf("insert %s: %v", r.id, err)
		}
	}

	order := []OrderTerm[widget]{widgetQty.Asc(), widgetID.Asc()}

	page1, err := From(widgets).OrderBy(order...).Limit(2).All(ctx, conn)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	assertWidgetIDs(t, page1, "w1", "w2")

	p, err := AfterTuple(
		[]AnyColumn[widget]{widgetQty.Col(), widgetID.Col()},
		[]any{int64(20), "w2"},
		false,
	)
	if err != nil {
		t.Fatalf("AfterTuple: %v", err)
	}

	page2, err := From(widgets).Where(p).OrderBy(order...).Limit(2).All(ctx, conn)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	assertWidgetIDs(t, page2, "w4", "w5")
}

func TestAfterTupleDescending(t *testing.T) {
	ctx, conn := seedCursorWidgets(t)

	order := []OrderTerm[widget]{widgetQty.Desc(), widgetID.Desc()}

	page1, err := From(widgets).OrderBy(order...).Limit(2).All(ctx, conn)
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	assertWidgetIDs(t, page1, "w6", "w5")

	// (quantity 50, id w5) going down: quantity < 50 OR (quantity = 50 AND id < w5).
	p, err := AfterTuple(
		[]AnyColumn[widget]{widgetQty.Col(), widgetID.Col()},
		[]any{int64(50), "w5"},
		true,
	)
	if err != nil {
		t.Fatalf("AfterTuple: %v", err)
	}

	page2, err := From(widgets).Where(p).OrderBy(order...).Limit(2).All(ctx, conn)
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	assertWidgetIDs(t, page2, "w4", "w3")
}

func TestAfterTupleRejectsInvalidArgs(t *testing.T) {
	if _, err := AfterTuple[widget](nil, nil, false); err == nil {
		t.Fatalf("AfterTuple(nil): nil error, want error")
	}

	if _, err := AfterTuple[widget](
		[]AnyColumn[widget]{widgetQty.Col()},
		[]any{int64(1), "extra"},
		false,
	); err == nil {
		t.Fatalf("AfterTuple(length mismatch): nil error, want error")
	}
}

func TestCursorValueAccessor(t *testing.T) {
	k := NewCursorKey(widgetID, "w5")

	if k.Value() != "w5" {
		t.Fatalf("Value() = %q, want w5", k.Value())
	}

	if k.Descending() {
		t.Fatalf("fresh cursor Descending = true, want false")
	}
}

// TestCursorBlobRoundTripGuardsSliceAliasing ensures Encode snapshots the
// value at encode time and DecodeCursor returns a fresh copy: mutating the
// caller's []byte backing array after Encode must not corrupt the decoded
// value.
func TestCursorBlobRoundTripGuardsSliceAliasing(t *testing.T) {
	b := []byte{1, 2, 3}
	k := NewCursorKey(cursorRowBlob, b)

	token, err := k.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	b[0] = 99 // mutate the original backing array after the token was produced

	got, ok, err := DecodeCursor(token, cursorRowBlob)
	if err != nil || !ok {
		t.Fatalf("DecodeCursor: ok=%v err=%v", ok, err)
	}

	if !bytes.Equal(got.Value(), []byte{1, 2, 3}) {
		t.Fatalf("blob = %v, want [1 2 3] (must not alias caller's slice)", got.Value())
	}

	got.Value()[1] = 0 // mutating the decoded copy must not touch the token's bytes

	again, ok, err := DecodeCursor(token, cursorRowBlob)
	if err != nil || !ok {
		t.Fatalf("DecodeCursor: ok=%v err=%v", ok, err)
	}

	if !bytes.Equal(again.Value(), []byte{1, 2, 3}) {
		t.Fatalf("re-decoded blob = %v, want [1 2 3]", again.Value())
	}
}

func TestCursorTokenFormatIsStableAndOpaque(t *testing.T) {
	k := NewCursorKey(widgetID, "w5")
	token, err := k.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	if token == "" {
		t.Fatalf("token is empty")
	}

	if token == "w5" {
		t.Fatalf("token %q leaks the raw value", token)
	}
}

// TestDecodeCursorMalformedTokens proves every malformed-token path fails
// closed with an error rather than a panic or a misdecoded cursor.
func TestDecodeCursorMalformedTokens(t *testing.T) {
	enc := func(raw []byte) string {
		return base64.RawURLEncoding.EncodeToString(raw)
	}

	withStr := func(s string) []byte {
		b := binary.AppendUvarint(nil, uint64(len(s)))
		return append(b, s...)
	}

	head := func(table, col string) []byte {
		b := []byte{cursorVersion, 0}
		b = append(b, withStr(table)...)
		return append(b, withStr(col)...)
	}

	widgetsQty := head("widgets", "quantity")
	rowsID := head("cursor_rows", "id")

	badVer := []byte{cursorVersion + 1}

	cases := []struct {
		name  string
		token string
	}{
		// Column-independent failures, decoded against widgetQty.
		{"bad base64", "!!!"},
		{"bad version", enc(badVer)},
		{"truncated table", enc([]byte{cursorVersion, 0})},
		{"no value", enc(widgetsQty)},
		{"corrupt int64", enc(append(append(widgetsQty, cursorTagInt64), 0xFF))},
		{"tag mismatch", enc(append(append(widgetsQty, cursorTagString), withStr("x")...))},
		{"truncated column", enc(append(append([]byte{cursorVersion, 0}, withStr("widgets")...), binary.AppendUvarint(nil, 25)...))},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := DecodeCursor(tc.token, widgetQty); err == nil {
				t.Fatalf("DecodeCursor(%q) succeeded, want an error", tc.token)
			}
		})
	}

	// Type-matched corrupt payloads, one per value tag.
	typeCases := []struct {
		name  string
		token string
		run   func(string) error
	}{
		{"corrupt int32", enc(append(append(head("cursor_rows", "num32"), cursorTagInt32), 0xFF)), func(tok string) error {
			_, _, err := DecodeCursor(tok, cursorRowNum32)
			return err
		}},
		{"corrupt float64", enc(append(head("cursor_rows", "f64"), cursorTagFloat)), func(tok string) error {
			_, _, err := DecodeCursor(tok, cursorRowF64)
			return err
		}},
		{"corrupt float32", enc(append(head("cursor_rows", "f32"), cursorTagF32)), func(tok string) error {
			_, _, err := DecodeCursor(tok, cursorRowF32)
			return err
		}},
		{"corrupt bool", enc(append(append(head("cursor_rows", "ok"), cursorTagBool), 1, 2)), func(tok string) error {
			_, _, err := DecodeCursor(tok, cursorRowOK)
			return err
		}},
		{"text bad prefix", enc(append(append(rowsID, cursorTagString), 0xFF)), func(tok string) error {
			_, _, err := DecodeCursor(tok, cursorRowID)
			return err
		}},
		{"text trailing bytes", enc(append(append(append(rowsID, cursorTagString), withStr("x")...), 0x00)), func(tok string) error {
			_, _, err := DecodeCursor(tok, cursorRowID)
			return err
		}},
		{"bad time", enc(append(append(head("cursor_rows", "at"), cursorTagTime), withStr("not-a-time")...)), func(tok string) error {
			_, _, err := DecodeCursor(tok, cursorRowAt)
			return err
		}},
	}

	for _, tc := range typeCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(tc.token); err == nil {
				t.Fatalf("DecodeCursor(%q) succeeded, want an error", tc.token)
			}
		})
	}

	valid, err := NewCursorKey(widgetQty, int64(7)).Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	if _, _, err := DecodeCursor(valid, widgetName); err == nil {
		t.Fatal("DecodeCursor with the wrong column succeeded, want a column-mismatch error")
	}
}

// TestCursorValueHelpersUnit pins the defensive branches unreachable
// through Encode/DecodeCursor: unsupported kinds, unknown tags, empty
// version probes and truncated length prefixes.
func TestCursorValueHelpersUnit(t *testing.T) {
	if _, err := encodeCursorValue([]int32{1}); err == nil {
		t.Fatal("encodeCursorValue([]int32) succeeded, want an error")
	}

	if _, err := encodeCursorValue(42); err == nil {
		t.Fatal("encodeCursorValue(int) succeeded, want an error")
	}

	if _, err := cursorTagFor(reflect.TypeOf(struct{}{})); err == nil {
		t.Fatal("cursorTagFor(struct{}) succeeded, want an error")
	}

	if _, err := cursorTagFor(reflect.TypeOf([]int32{})); err == nil {
		t.Fatal("cursorTagFor([]int32) succeeded, want an error")
	}

	if got := cursorVersionAt(nil); got != 0 {
		t.Fatalf("cursorVersionAt(nil) = %d, want 0", got)
	}

	if _, _, err := readCursorString(nil); err == nil {
		t.Fatal("readCursorString(nil) succeeded, want an error")
	}

	if _, _, err := readCursorString(append(binary.AppendUvarint(nil, 7), "x"...)); err == nil {
		t.Fatal("readCursorString(truncated) succeeded, want an error")
	}

	if _, err := decodeCursorValue('z', nil, reflect.TypeOf(int64(0))); err == nil {
		t.Fatal("decodeCursorValue(unknown tag) succeeded, want an error")
	}

	if _, err := decodeCursorText(cursorTagTime, withStrHelper("x"), reflect.TypeOf(time.Time{})); err == nil {
		t.Fatal("decodeCursorText(bad time) succeeded, want an error")
	}
}

func withStrHelper(s string) []byte {
	b := binary.AppendUvarint(nil, uint64(len(s)))
	return append(b, s...)
}

// TestCursorBoolFalseRoundTrip pins the false-bool encoding (the zero
// payload byte) through a full token round trip.
func TestCursorBoolFalseRoundTrip(t *testing.T) {
	k := NewCursorKey(cursorRowOK, false)

	token, err := k.Encode()
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}

	again, ok, err := DecodeCursor(token, cursorRowOK)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if !ok {
		t.Fatal("Decode ok = false, want true")
	}

	if again.Value() {
		t.Fatalf("re-decoded bool = true, want false")
	}
}
