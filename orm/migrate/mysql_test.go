package migrate

import (
	"context"
	"errors"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
)

func TestQuoteIdent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dialect string
		in      string
		want    string
	}{
		{"mysqlBackticks", atlas.DialectMySQL, "orders", "`orders`"},
		{"mysqlEscapesBacktick", atlas.DialectMySQL, "we`ird", "`we``ird`"},
		{"postgresDoubleQuotes", atlas.DialectPostgres, "orders", `"orders"`},
		{"sqliteDoubleQuotes", atlas.DialectSQLite, "orders", `"orders"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := quoteIdent(tt.dialect, tt.in); got != tt.want {
				t.Fatalf("quoteIdent = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderMySQLDropIndex(t *testing.T) {
	t.Parallel()

	got := renderMySQLDropIndex("`orders`", "orders_user_id_idx")
	want := "DROP INDEX `orders_user_id_idx` ON `orders`;"

	if got != want {
		t.Fatalf("renderMySQLDropIndex = %q, want %q", got, want)
	}
}

func TestMysqlTableExists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("present", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{1}}}, nil
			},
		}

		got, err := mysqlTableExists(ctx, conn, "orders")
		if err != nil || !got {
			t.Fatalf("mysqlTableExists = (%v, %v), want (true, nil)", got, err)
		}
	})

	t.Run("absent", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{}, nil
			},
		}

		got, err := mysqlTableExists(ctx, conn, "orders")
		if err != nil || got {
			t.Fatalf("mysqlTableExists = (%v, %v), want (false, nil)", got, err)
		}
	})

	t.Run("queryError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		if _, err := mysqlTableExists(ctx, conn, "orders"); err == nil {
			t.Fatal("mysqlTableExists: want error, got nil")
		}
	})
}

func TestIntrospectMySQLColumns(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("rows", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{
					{"id", "bigint", "NO", nil},
					{"email", "varchar(255)", "YES", nil},
				}}, nil
			},
		}

		cols, err := introspectMySQLColumns(ctx, conn, "orders")
		if err != nil {
			t.Fatalf("introspectMySQLColumns: %v", err)
		}

		if len(cols) != 2 || cols[0].Name != "id" || cols[0].Nullable {
			t.Fatalf("cols = %+v", cols)
		}

		if cols[1].RawType != "varchar(255)" || !cols[1].Nullable {
			t.Fatalf("cols = %+v", cols)
		}
	})

	t.Run("queryError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		if _, err := introspectMySQLColumns(ctx, conn, "orders"); err == nil {
			t.Fatal("introspectMySQLColumns: want error, got nil")
		}
	})

	t.Run("scanError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"id"}}, scanErr: errors.New("boom")}, nil
			},
		}

		if _, err := introspectMySQLColumns(ctx, conn, "orders"); err == nil {
			t.Fatal("introspectMySQLColumns: want scan error, got nil")
		}
	})

	t.Run("iterateError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"id", "bigint", "NO", nil}}, iterErr: errors.New("boom")}, nil
			},
		}

		if _, err := introspectMySQLColumns(ctx, conn, "orders"); err == nil {
			t.Fatal("introspectMySQLColumns: want iterate error, got nil")
		}
	})
}

func TestIntrospectMySQLIndexes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("grouped", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{
					{"orders_user_id_idx", "user_id", 1},
					{"orders_email_key", "email", 0},
					{"orders_pair_idx", "a", 1},
					{"orders_pair_idx", "b", 1},
				}}, nil
			},
		}

		idx, err := introspectMySQLIndexes(ctx, conn, "orders")
		if err != nil {
			t.Fatalf("introspectMySQLIndexes: %v", err)
		}

		if len(idx) != 3 {
			t.Fatalf("indexes = %+v, want 3", idx)
		}

		if idx[0].Name != "orders_user_id_idx" || idx[0].Unique {
			t.Fatalf("idx[0] = %+v", idx[0])
		}

		if idx[1].Name != "orders_email_key" || !idx[1].Unique {
			t.Fatalf("idx[1] = %+v", idx[1])
		}

		if len(idx[2].Columns) != 2 || idx[2].Columns[0] != "a" || idx[2].Columns[1] != "b" {
			t.Fatalf("idx[2] = %+v", idx[2])
		}
	})

	t.Run("queryError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		if _, err := introspectMySQLIndexes(ctx, conn, "orders"); err == nil {
			t.Fatal("introspectMySQLIndexes: want error, got nil")
		}
	})

	t.Run("scanError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"x", "y", 1}}, scanErr: errors.New("boom")}, nil
			},
		}

		if _, err := introspectMySQLIndexes(ctx, conn, "orders"); err == nil {
			t.Fatal("introspectMySQLIndexes: want scan error, got nil")
		}
	})

	t.Run("iterateError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"x", "y", 1}}, iterErr: errors.New("boom")}, nil
			},
		}

		if _, err := introspectMySQLIndexes(ctx, conn, "orders"); err == nil {
			t.Fatal("introspectMySQLIndexes: want iterate error, got nil")
		}
	})
}

func TestIntrospectMySQLForeignKeys(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("rows", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"user_id", "users", "id"}}}, nil
			},
		}

		fks, err := introspectMySQLForeignKeys(ctx, conn, "orders")
		if err != nil {
			t.Fatalf("introspectMySQLForeignKeys: %v", err)
		}

		if len(fks) != 1 || fks[0].Column != "user_id" || fks[0].RefTable != "users" || fks[0].RefColumn != "id" {
			t.Fatalf("fks = %+v", fks)
		}
	})

	t.Run("queryError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		if _, err := introspectMySQLForeignKeys(ctx, conn, "orders"); err == nil {
			t.Fatal("introspectMySQLForeignKeys: want error, got nil")
		}
	})

	t.Run("scanError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"a", "b", "c"}}, scanErr: errors.New("boom")}, nil
			},
		}

		if _, err := introspectMySQLForeignKeys(ctx, conn, "orders"); err == nil {
			t.Fatal("introspectMySQLForeignKeys: want scan error, got nil")
		}
	})

	t.Run("iterateError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"a", "b", "c"}}, iterErr: errors.New("boom")}, nil
			},
		}

		if _, err := introspectMySQLForeignKeys(ctx, conn, "orders"); err == nil {
			t.Fatal("introspectMySQLForeignKeys: want iterate error, got nil")
		}
	})
}
