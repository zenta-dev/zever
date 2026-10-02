package sqlite

import "testing"

// TestDBCapabilities_matrix verifies the DB-backed-adapter capabilities:
// SQLite has no pgvector bundle and no tsvector, while FTS5 and the
// row-lease CAS claim hold.
func TestDBCapabilities_matrix(t *testing.T) {
	t.Parallel()

	d := New()

	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{name: "SupportsVectorOps", got: d.SupportsVectorOps(), want: false},
		{name: "SupportsTSVector", got: d.SupportsTSVector(), want: false},
		{name: "SupportsFTS5", got: d.SupportsFTS5(), want: true},
		{name: "SupportsLeaseClaim", got: d.SupportsLeaseClaim(), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if tt.got != tt.want {
				t.Fatalf("%s = %v, want %v", tt.name, tt.got, tt.want)
			}
		})
	}
}
