package dialect

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// baseDialect implements only the base Dialect contract, so it does not
// satisfy DistinctOnDialect at all.
type baseDialect struct{}

func (baseDialect) Name() string               { return "base" }
func (baseDialect) Placeholder(int) string     { return "?" }
func (baseDialect) QuoteIdent(s string) string { return s }

// TestCheckDistinctOnMatrix pins every branch of the single DISTINCT ON
// capability rule both query-build-time and render-time gates rely on:
// nil is unset, empty is a caller error, and a non-empty list fails only on
// a dialect that lacks the feature.
func TestCheckDistinctOnMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		dialect    Dialect
		distinctOn []string
		wantErrIs  error
		wantMsg    string
	}{
		{name: "nil is unset", dialect: sqlite.New(), distinctOn: nil},
		{name: "nil unset on postgres", dialect: postgres.New(), distinctOn: nil},
		{
			name: "empty is caller error", dialect: postgres.New(), distinctOn: []string{},
			wantMsg: "orm: DISTINCT ON requires at least one column",
		},
		{
			name: "sqlite lacks support", dialect: sqlite.New(), distinctOn: []string{"id"},
			wantErrIs: ErrUnsupportedByDialect,
		},
		{
			name: "base dialect lacks interface", dialect: baseDialect{}, distinctOn: []string{"id"},
			wantErrIs: ErrUnsupportedByDialect,
		},
		{name: "postgres supports", dialect: postgres.New(), distinctOn: []string{"id", "created_at"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := CheckDistinctOn(tt.dialect, tt.distinctOn)

			if tt.wantErrIs == nil && tt.wantMsg == "" {
				if err != nil {
					t.Fatalf("CheckDistinctOn() = %v, want nil", err)
				}

				return
			}

			if err == nil {
				t.Fatalf("CheckDistinctOn() = nil, want error")
			}

			if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
				t.Fatalf("CheckDistinctOn() = %v, want errors.Is %v", err, tt.wantErrIs)
			}

			if tt.wantMsg != "" && err.Error() != tt.wantMsg {
				t.Fatalf("CheckDistinctOn() = %q, want %q", err.Error(), tt.wantMsg)
			}
		})
	}
}

// BenchmarkCheckDistinctOn measures the render-time capability gate on its
// supported fast path.
func BenchmarkCheckDistinctOn(b *testing.B) {
	d := postgres.New()
	cols := []string{"id"}

	b.ReportAllocs()

	for b.Loop() {
		if err := CheckDistinctOn(d, cols); err != nil {
			b.Fatalf("CheckDistinctOn: %v", err)
		}
	}
}
