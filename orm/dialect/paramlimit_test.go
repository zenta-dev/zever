package dialect

import (
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestMaxParamsPostgres pins the Postgres bind-parameter budget: the
// wire protocol addresses parameters with a uint16, so 65535 is the hard
// per-statement ceiling an IN fan-out must stay under.
func TestMaxParamsPostgres(t *testing.T) {
	t.Parallel()

	if got := MaxParams(postgres.New()); got != 65535 {
		t.Fatalf("MaxParams(postgres) = %d, want 65535", got)
	}
}

// TestMaxParamsSQLiteVersionGated pins the SQLite budget to the library
// version: 999 before 3.32.0, 32766 after (SQLite raised
// SQLITE_MAX_VARIABLE_NUMBER in 3.32.0).
func TestMaxParamsSQLiteVersionGated(t *testing.T) {
	t.Parallel()

	cases := []struct {
		version string
		want    int
	}{
		{"3.8.0", 999},
		{"3.31.1", 999},
		{"3.32.0", 32766},
		{"3.46.0", 32766},
	}

	for _, tc := range cases {
		d, err := sqlite.NewWithVersion(tc.version)
		if err != nil {
			t.Fatalf("NewWithVersion(%q): %v", tc.version, err)
		}

		if got := MaxParams(d); got != tc.want {
			t.Errorf("MaxParams(sqlite %s) = %d, want %d", tc.version, got, tc.want)
		}
	}

	if got := MaxParams(sqlite.New()); got != 32766 {
		t.Errorf("MaxParams(sqlite default) = %d, want 32766", got)
	}
}

// TestMaxParamsFallback proves a dialect that does not implement
// ParamLimitDialect gets the conservative 999 budget, never zero or
// unbounded.
func TestMaxParamsFallback(t *testing.T) {
	t.Parallel()

	if got := MaxParams(plainDialect{}); got != 999 {
		t.Fatalf("MaxParams(plain) = %d, want fallback 999", got)
	}
}

type plainDialect struct{}

func (plainDialect) Name() string               { return "plain" }
func (plainDialect) Placeholder(int) string     { return "?" }
func (plainDialect) QuoteIdent(s string) string { return `"` + s + `"` }
