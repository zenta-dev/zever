package postgres

import "testing"

// TestJSONCapabilities_truth_table pins the set-returning family answers.
func TestJSONCapabilities_truth_table(t *testing.T) {
	t.Parallel()

	d := New()

	tests := []struct {
		name string
		got  bool
		want bool
	}{
		{name: "SupportsJSONEach", got: d.SupportsJSONEach(), want: false},
		{name: "SupportsJSONSetReturning", got: d.SupportsJSONSetReturning(), want: true},
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
