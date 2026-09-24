package orm

import (
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/db/sqlite"
)

func TestJSONTextScan(t *testing.T) {
	t.Parallel()

	var j JSONText
	if err := j.Scan(`{"a":1}`); err != nil {
		t.Fatalf("Scan string: %v", err)
	}

	if string(j) != `{"a":1}` {
		t.Fatalf("Scan string = %q", j)
	}

	if err := j.Scan([]byte(`[1,2]`)); err != nil {
		t.Fatalf("Scan bytes: %v", err)
	}

	if string(j) != `[1,2]` {
		t.Fatalf("Scan bytes = %q", j)
	}

	if err := j.Scan(nil); err == nil {
		t.Fatal("Scan NULL: want error")
	}

	if err := j.Scan(42); err == nil {
		t.Fatal("Scan int: want error")
	}
}

func TestJSONTextValue(t *testing.T) {
	t.Parallel()

	v, err := JSONText(`{"a":1}`).Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}

	s, ok := v.(string)
	if !ok || s != `{"a":1}` {
		t.Fatalf("Value = %#v, want TEXT string", v)
	}

	var _ driver.Valuer = JSONText("")
}

func TestJSONTextJSONRoundTrip(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(JSONText(`{"a":1}`))
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	if string(raw) != `{"a":1}` {
		t.Fatalf("Marshal = %s, want raw document", raw)
	}

	var j JSONText
	if err = json.Unmarshal(raw, &j); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if string(j) != `{"a":1}` {
		t.Fatalf("Unmarshal = %q", j)
	}

	empty, err := json.Marshal(JSONText(""))
	if err != nil {
		t.Fatalf("Marshal empty: %v", err)
	}

	if string(empty) != "null" {
		t.Fatalf("Marshal empty = %s, want null", empty)
	}

	var cleared JSONText
	if err = json.Unmarshal([]byte("null"), &cleared); err != nil {
		t.Fatalf("Unmarshal null: %v", err)
	}

	if cleared != "" {
		t.Fatalf("Unmarshal null = %q, want empty", cleared)
	}
}

func TestJSONTextOptionAndSQLite(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	conn, err := sqlite.New(db.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	t.Cleanup(func() { _ = conn.Close(ctx) })

	if _, err = conn.Exec(ctx, `CREATE TABLE docs (id TEXT, data TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}

	docs := NewTable[jsonDoc]("docs", []string{"id", "data"})
	byID := NewColumn[jsonDoc, string]("docs", "id")
	payload := NewColumn[jsonDoc, JSONText]("docs", "data")

	want := JSONText(`{"author":{"name":"Grace"},"tags":["go"]}`)
	if err = InsertInto(docs).Values(
		Set(byID, "d1"),
		Set(payload, want),
	).Exec(ctx, conn); err != nil {
		t.Fatalf("insert: %v", err)
	}

	got, err := From(docs).Where(byID.Eq("d1")).All(ctx, conn)
	if err != nil {
		t.Fatalf("All: %v", err)
	}

	if len(got) != 1 || got[0].Data != want {
		t.Fatalf("All = %+v, want data %s", got, want)
	}

	var opt Option[JSONText]
	if err := opt.Scan(`{"a":1}`); err != nil {
		t.Fatalf("Option Scan: %v", err)
	}

	v, ok := opt.Get()
	if !ok || v != `{"a":1}` {
		t.Fatalf("Option = (%q, %v)", v, ok)
	}

	var none Option[JSONText]
	if err := none.Scan(nil); err != nil {
		t.Fatalf("Option NULL: %v", err)
	}

	if none.IsSome() {
		t.Fatal("Option NULL: want None")
	}
}

type jsonDoc struct {
	ID   string
	Data JSONText
}

func (d *jsonDoc) Scan(row Row) error {
	return row.Scan(&d.ID, &d.Data)
}
