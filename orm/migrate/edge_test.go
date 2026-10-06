package migrate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/db"
)

// permissiveFakeDB accepts any Query/Exec and returns an empty result set,
// for edge tests that only assert on the dialect gate or the nil-plan guard
// before any statement runs.
type permissiveFakeDB struct {
	dialect string
}

func (f *permissiveFakeDB) Query(context.Context, string, ...any) (db.Rows, error) {
	return &emptyRows{}, nil
}

func (f *permissiveFakeDB) Exec(context.Context, string, ...any) (int64, error) { return 0, nil }
func (f *permissiveFakeDB) Ping(context.Context) error                          { return nil }
func (f *permissiveFakeDB) Close(context.Context) error                         { return nil }
func (f *permissiveFakeDB) Dialect() string                                     { return f.dialect }

type emptyRows struct{}

func (emptyRows) Next() bool                 { return false }
func (emptyRows) Scan(...any) error          { return nil }
func (emptyRows) Close() error               { return nil }
func (emptyRows) Err() error                 { return nil }
func (emptyRows) Columns() ([]string, error) { return nil, nil }

func TestChecksumOfEmpty(t *testing.T) {
	got := ChecksumOf("")
	if got == "" {
		t.Fatal("ChecksumOf(\"\") = \"\", want non-empty digest")
	}
}

func TestChecksumOfDeterministic(t *testing.T) {
	a := ChecksumOf("CREATE TABLE t (id text);")
	b := ChecksumOf("CREATE TABLE t (id text);")

	if a != b {
		t.Fatalf("ChecksumOf not deterministic: %q != %q", a, b)
	}
}

func TestChecksumOfDistinctInputs(t *testing.T) {
	a := ChecksumOf("CREATE TABLE a (id text);")
	b := ChecksumOf("CREATE TABLE b (id text);")

	if a == b {
		t.Fatal("ChecksumOf collided for different statements")
	}
}

func TestChecksumOfUnicode(t *testing.T) {
	got := ChecksumOf("CREATE TABLE café (nom text);")
	if got == "" {
		t.Fatal("ChecksumOf unicode: empty digest")
	}
}

func TestApplyNilPlan(t *testing.T) {
	_, err := Apply(t.Context(), &permissiveFakeDB{dialect: "sqlite"}, nil)
	if err == nil {
		t.Fatal("Apply nil plan: want error, got nil")
	}

	if !strings.Contains(err.Error(), "nil plan") {
		t.Fatalf("err = %v, want nil plan message", err)
	}
}

func TestApplyRollbackNilPlan(t *testing.T) {
	_, err := ApplyRollback(t.Context(), &permissiveFakeDB{dialect: "sqlite"}, nil)
	if err == nil {
		t.Fatal("ApplyRollback nil plan: want error, got nil")
	}

	if !strings.Contains(err.Error(), "nil plan") {
		t.Fatalf("err = %v, want nil plan message", err)
	}
}

func TestApplyEmptyPlan(t *testing.T) {
	got, err := Apply(t.Context(), &permissiveFakeDB{dialect: "sqlite"}, &MigrationPlan{Dialect: "sqlite"})
	if err != nil {
		t.Fatalf("Apply empty plan: %v", err)
	}

	if got != 0 {
		t.Fatalf("applied = %d, want 0", got)
	}
}

func TestApplyUnsupportedDialect(t *testing.T) {
	_, err := Apply(t.Context(), &permissiveFakeDB{dialect: "oracle"}, &MigrationPlan{Dialect: "oracle"})
	if err == nil {
		t.Fatal("Apply unsupported dialect: want error, got nil")
	}

	if !errors.Is(err, ErrUnsupportedDialect) {
		t.Fatalf("err = %v, want ErrUnsupportedDialect", err)
	}
}

func TestComputeRollbackZeroN(t *testing.T) {
	plan, err := ComputeRollback(t.Context(), &permissiveFakeDB{dialect: "sqlite"}, 0)
	if err != nil {
		t.Fatalf("ComputeRollback(0): %v", err)
	}

	if plan == nil || len(plan.Statements) != 0 {
		t.Fatalf("ComputeRollback(0) = %v, want empty plan", plan)
	}
}

func TestComputeRollbackNegativeN(t *testing.T) {
	// Negative LIMIT is passed through to the database; the fake returns
	// no rows, so the plan is empty. The function must not panic.
	plan, err := ComputeRollback(t.Context(), &permissiveFakeDB{dialect: "sqlite"}, -1)
	if err != nil {
		t.Fatalf("ComputeRollback(-1): %v", err)
	}

	if plan == nil || len(plan.Statements) != 0 {
		t.Fatalf("ComputeRollback(-1) = %v, want empty plan", plan)
	}
}

func TestComputeRollbackUnsupportedDialect(t *testing.T) {
	_, err := ComputeRollback(t.Context(), &permissiveFakeDB{dialect: "oracle"}, 5)
	if err == nil {
		t.Fatal("ComputeRollback unsupported dialect: want error, got nil")
	}

	if !errors.Is(err, ErrUnsupportedDialect) {
		t.Fatalf("err = %v, want ErrUnsupportedDialect", err)
	}
}

func TestDropFTS5VirtualTableQuotesInjection(t *testing.T) {
	got := DropFTS5VirtualTable(`docs"; DROP TABLE users; --`)
	want := `DROP TABLE IF EXISTS "docs""; DROP TABLE users; --";`
	if got != want {
		t.Fatalf("got = %q, want %q", got, want)
	}
}

func TestCreateFTS5VirtualTableUnicodeIdent(t *testing.T) {
	if _, err := CreateFTS5VirtualTable("café", []string{"body"}); err == nil {
		t.Fatal("CreateFTS5VirtualTable unicode table: want error, got nil")
	}
}

func TestCreateFTS5VirtualTableLeadingDigit(t *testing.T) {
	if _, err := CreateFTS5VirtualTable("1docs", []string{"body"}); err == nil {
		t.Fatal("CreateFTS5VirtualTable leading-digit table: want error, got nil")
	}
}

func TestCreateFTS5VirtualTableQuotedKeywordIdent(t *testing.T) {
	// A SQL keyword is a legal identifier once quoted.
	got, err := CreateFTS5VirtualTable("select", []string{"from"})
	if err != nil {
		t.Fatalf("CreateFTS5VirtualTable keyword idents: %v", err)
	}

	want := `CREATE VIRTUAL TABLE IF NOT EXISTS "select" USING fts5(from);`
	if got != want {
		t.Fatalf("got = %q, want %q", got, want)
	}
}
