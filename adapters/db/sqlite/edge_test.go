package sqlite

import "testing"

// TestEdgeExec_zeroRowsAffected covers the zero-rows boundary: an UPDATE that
// matches nothing must report 0 affected rows without error.
func TestEdgeExec_zeroRowsAffected(t *testing.T) {
	t.Parallel()

	d := newMemoryDB(t)
	ctx := t.Context()

	if _, err := d.Exec(ctx, "CREATE TABLE t (id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		t.Fatalf("create table: %v", err)
	}

	n, err := d.Exec(ctx, "UPDATE t SET v = ? WHERE id = ?", "x", -1)
	if err != nil {
		t.Fatalf("Exec no-match = %v, want nil", err)
	}

	if n != 0 {
		t.Fatalf("RowsAffected = %d, want 0", n)
	}
}
