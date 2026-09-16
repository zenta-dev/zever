package dialect

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/postgres"
	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// TestErrUnsupportedByDialect_sentinel_stable verifies the sentinel keeps
// its shape and package prefix.
func TestErrUnsupportedByDialect_sentinel_stable(t *testing.T) {
	t.Parallel()

	if ErrUnsupportedByDialect == nil {
		t.Fatal("ErrUnsupportedByDialect is nil")
	}

	if got, want := ErrUnsupportedByDialect.Error(), "orm/dialect: unsupported by dialect"; got != want {
		t.Fatalf("ErrUnsupportedByDialect = %q, want %q", got, want)
	}
}

// TestDialects_implement_base_interface verifies both in-tree dialects
// satisfy the base contract through the registry.
func TestDialects_implement_base_interface(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "sqlite", want: "sqlite"},
		{name: "postgres", want: "postgres"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := For(tt.name)
			if err != nil {
				t.Fatalf("For(%q) error: %v", tt.name, err)
			}

			if d == nil {
				t.Fatalf("For(%q) returned nil dialect", tt.name)
			}

			if got := d.Name(); got != tt.want {
				t.Fatalf("Name() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestDialects_implement_capability_interfaces pins the type-safety
// contract: every in-tree dialect must satisfy the full capability method
// set so call sites use interface assertions, never stringly checks.
func TestDialects_implement_capability_interfaces(t *testing.T) {
	t.Parallel()

	sqliteDialect := sqlite.New()
	postgresDialect := postgres.New()

	sqliteIfaces := []any{
		CTEDialect(sqliteDialect),
		SetOpDialect(sqliteDialect),
		ReturningDialect(sqliteDialect),
		ExplainDialect(sqliteDialect),
		MergeDialect(sqliteDialect),
		JSONEachDialect(sqliteDialect),
		JSONSetReturningDialect(sqliteDialect),
		JoinCapabilities(sqliteDialect),
		MutateJoinDialect(sqliteDialect),
		MutateOrderDialect(sqliteDialect),
		ConflictWhereDialect(sqliteDialect),
		LockingDialect(sqliteDialect),
		NullsOrderDialect(sqliteDialect),
		DistinctOnDialect(sqliteDialect),
		ExtendedLockingDialect(sqliteDialect),
		TablesampleDialect(sqliteDialect),
		LateralJoinDialect(sqliteDialect),
		AggregateGroupingDialect(sqliteDialect),
		OrderedAggregateDialect(sqliteDialect),
		ArrayDialect(sqliteDialect),
		WindowFrameDialect(sqliteDialect),
		CTEMaterializationDialect(sqliteDialect),
		CTESearchCycleDialect(sqliteDialect),
	}

	postgresIfaces := []any{
		CTEDialect(postgresDialect),
		SetOpDialect(postgresDialect),
		ReturningDialect(postgresDialect),
		ExplainDialect(postgresDialect),
		MergeDialect(postgresDialect),
		JSONEachDialect(postgresDialect),
		JSONSetReturningDialect(postgresDialect),
		JoinCapabilities(postgresDialect),
		MutateJoinDialect(postgresDialect),
		MutateOrderDialect(postgresDialect),
		ConflictWhereDialect(postgresDialect),
		LockingDialect(postgresDialect),
		NullsOrderDialect(postgresDialect),
		DistinctOnDialect(postgresDialect),
		ExtendedLockingDialect(postgresDialect),
		TablesampleDialect(postgresDialect),
		LateralJoinDialect(postgresDialect),
		AggregateGroupingDialect(postgresDialect),
		OrderedAggregateDialect(postgresDialect),
		ArrayDialect(postgresDialect),
		WindowFrameDialect(postgresDialect),
		CTEMaterializationDialect(postgresDialect),
		CTESearchCycleDialect(postgresDialect),
	}

	if len(sqliteIfaces) == 0 || len(postgresIfaces) == 0 {
		t.Fatal("capability interface lists must not be empty")
	}
}

// TestFor_rejects_unknown_names verifies unknown and removed dialect names
// resolve to the typed unknown-dialect error, never a silent default.
func TestFor_rejects_unknown_names(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: ""},
		{name: "mysql"},
		{name: "MySQL"},
		{name: "sqlite3"},
		{name: "postgresql"},
		{name: "definitely-not-a-dialect"},
	}

	for _, tt := range tests {
		t.Run("name="+tt.name, func(t *testing.T) {
			t.Parallel()

			d, err := For(tt.name)
			if err == nil {
				t.Fatalf("For(%q) = nil error, want error", tt.name)
			}

			if d != nil {
				t.Fatalf("For(%q) returned non-nil dialect with error", tt.name)
			}

			if !errors.Is(err, ErrUnknownDialect) {
				t.Fatalf("For(%q) error = %v, want it to wrap ErrUnknownDialect", tt.name, err)
			}
		})
	}
}
