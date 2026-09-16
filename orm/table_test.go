package orm

import "testing"

type tableT struct{}

func TestTableNameColumns(t *testing.T) {
	tbl := NewTable[tableT]("widgets", []string{"id", "name"})

	if got := tbl.Name(); got != "widgets" {
		t.Fatalf("Name() = %q, want %q", got, "widgets")
	}

	cols := tbl.Columns()
	if len(cols) != 2 || cols[0] != "id" || cols[1] != "name" {
		t.Fatalf("Columns() = %v", cols)
	}
}

// TestTableColumnsIsACopy proves mutating a slice returned by Columns()
// cannot corrupt the Table's own backing array (or a second caller's
// copy) -- Table's exported surface never leaks its internal slice by
// reference.
func TestTableColumnsIsACopy(t *testing.T) {
	tbl := NewTable[tableT]("widgets", []string{"id", "name"})

	cols := tbl.Columns()
	cols[0] = "mutated"

	again := tbl.Columns()
	if again[0] != "id" {
		t.Fatalf("Columns() leaked its backing array: got %v after external mutation", again)
	}
}

// TestNewTableCopiesInput proves NewTable copies its columns argument, so
// a caller mutating the slice it passed in afterward cannot affect the
// constructed Table.
func TestNewTableCopiesInput(t *testing.T) {
	cols := []string{"id", "name"}
	tbl := NewTable[tableT]("widgets", cols)

	cols[0] = "mutated"

	if got := tbl.Columns(); got[0] != "id" {
		t.Fatalf("NewTable did not copy its input: got %v", got)
	}
}
