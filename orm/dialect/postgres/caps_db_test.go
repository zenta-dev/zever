package postgres

import "testing"

// TestDBCapabilities_matrix verifies the DB-backed-adapter capabilities:
// pgvector ops and tsvector are postgres-only, FTS5 is sqlite-only, and the
// row-lease CAS claim holds.
func TestDBCapabilities_matrix(t *testing.T) {
	t.Parallel()

	d := New()

	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{name: "SupportsVectorOps", got: d.SupportsVectorOps(), want: true},
		{name: "SupportsTSVector", got: d.SupportsTSVector(), want: true},
		{name: "SupportsFTS5", got: d.SupportsFTS5(), want: false},
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
