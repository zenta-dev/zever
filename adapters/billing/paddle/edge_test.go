package paddle

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/core/billing"
)

// TestEdgeParsePaddleTotal_boundaries covers whitespace padding and int64
// overflow around the decimal-string parser.
func TestEdgeParsePaddleTotal_boundaries(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		s        string
		currency string
		want     int64
		wantErr  error
	}{
		{"whitespace padded", " 10.00 ", "usd", 1000, nil},
		{"whitespace only", "   ", "usd", 0, billing.ErrEmptyAmount},
		{"int64 overflow", "9223372036854775808", "usd", 0, billing.ErrMalformedAmount},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := parsePaddleTotal(tc.s, tc.currency)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("err = %v", err)
			}

			if got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}
