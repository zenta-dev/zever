package migrate

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
)

func TestComputeRollback_unsupportedDialect(t *testing.T) {
	t.Parallel()

	conn := &fakeDB{dialect: "oracle"}

	if _, err := ComputeRollback(context.Background(), conn, 1); !errors.Is(err, ErrUnsupportedDialect) {
		t.Fatalf("ComputeRollback = %v, want ErrUnsupportedDialect", err)
	}
}

func TestComputeRollback_readError(t *testing.T) {
	t.Parallel()

	conn := &fakeDB{
		dialect: atlas.DialectSQLite,
		onQuery: func(_ string, _ []any) (db.Rows, error) {
			return nil, errors.New("boom")
		},
	}

	if _, err := ComputeRollback(context.Background(), conn, 1); err == nil {
		t.Fatal("ComputeRollback: want error, got nil")
	}
}

func TestComputeRollback_empty(t *testing.T) {
	t.Parallel()

	conn := &fakeDB{
		dialect: atlas.DialectSQLite,
		onQuery: func(_ string, _ []any) (db.Rows, error) {
			return &fakeRows{}, nil
		},
	}

	plan, err := ComputeRollback(context.Background(), conn, 5)
	if err != nil {
		t.Fatalf("ComputeRollback: %v", err)
	}

	if len(plan.Statements) != 0 || len(plan.Warnings) != 0 {
		t.Fatalf("plan = %+v, want empty", plan)
	}

	if plan.Dialect != atlas.DialectSQLite {
		t.Fatalf("Dialect = %q", plan.Dialect)
	}
}

func TestComputeRollback_invertsAndSkips(t *testing.T) {
	t.Parallel()

	conn := &fakeDB{
		dialect: atlas.DialectPostgres,
		onQuery: func(_ string, _ []any) (db.Rows, error) {
			return &fakeRows{vals: [][]any{
				{int64(2), "c2", "add_column", `"public"."orders"`, "note", "stmt", nil, nil, nil, nil},
				{int64(1), "c1", "create_table", nil, nil, nil, nil, nil, nil, nil},
			}}, nil
		},
	}

	plan, err := ComputeRollback(context.Background(), conn, 2)
	if err != nil {
		t.Fatalf("ComputeRollback: %v", err)
	}

	// One invertible row (2 statements) + one skipped create_table (1 warning).
	if len(plan.Statements) != 2 {
		t.Fatalf("statements = %v", plan.Statements)
	}

	if len(plan.Warnings) != 2 {
		t.Fatalf("warnings = %v", plan.Warnings)
	}

	if !strings.Contains(plan.Warnings[0], "DROPS") {
		t.Fatalf("warnings[0] = %q, want data-loss notice", plan.Warnings[0])
	}

	if !strings.Contains(plan.Warnings[1], "out of scope") {
		t.Fatalf("warnings[1] = %q, want skip notice", plan.Warnings[1])
	}
}

func TestInverseStatement_delegatesConstraints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		row  migrationRow
		ok   bool
	}{
		{"createIndex", migrationRow{Kind: kindCreateIndex, Table: "t", ObjectName: "i"}, true},
		{"dropIndex", migrationRow{Kind: kindDropIndex, Table: "t", PriorSQL: "CREATE INDEX x;"}, true},
		{"addFK", migrationRow{Kind: kindAddForeignKey, Table: "t", ObjectName: "f"}, true},
		{"nullability", migrationRow{Kind: kindAlterNullability, Table: "t", Column: "c", PriorType: "NULL"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, ok := inverseStatement(atlas.DialectPostgres, tt.row); ok != tt.ok {
				t.Fatalf("inverseStatement ok = %v, want %v", ok, tt.ok)
			}
		})
	}
}

func TestInverseStatement_sqliteAlterTypeRefused(t *testing.T) {
	t.Parallel()

	row := migrationRow{Kind: kindAlterType, Table: `"widgets"`, Column: "size", PriorType: "TEXT"}

	if _, ok := inverseStatement(atlas.DialectSQLite, row); ok {
		t.Fatal("inverseStatement sqlite alter_type: want false, got true")
	}
}

func TestSkipReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		row  migrationRow
		want string
	}{
		{"preTracking", migrationRow{ID: 3}, "predates rollback tracking"},
		{"createTable", migrationRow{ID: 4, Kind: kindCreateTable}, "out of scope"},
		{"unknown", migrationRow{ID: 5, Kind: "mystery"}, "no synthesizable inverse"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := skipReason(tt.row); !strings.Contains(got, tt.want) {
				t.Fatalf("skipReason = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLossyRollbackWarning(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		row   migrationRow
		want  string
		empty bool
	}{
		{"addColumn", migrationRow{Kind: kindAddColumn, Table: "t", Column: "c"}, "DROPS", false},
		{"dropColumn", migrationRow{Kind: kindDropColumn, Table: "t", Column: "c", PriorType: "TEXT"}, "UNRECOVERABLE", false},
		{"alterType", migrationRow{Kind: kindAlterType, Table: "t", Column: "c", PriorType: "TEXT"}, "not restored", false},
		{"nullToNotNull", migrationRow{Kind: kindAlterNullability, Table: "t", Column: "c", PriorType: "NOT NULL"}, "FAILS", false},
		{"nullToNull", migrationRow{Kind: kindAlterNullability, Table: "t", Column: "c", PriorType: "NULL"}, "", true},
		{"createIndex", migrationRow{Kind: kindCreateIndex}, "", true},
		{"dropIndex", migrationRow{Kind: kindDropIndex}, "", true},
		{"fk", migrationRow{Kind: kindAddForeignKey}, "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := lossyRollbackWarning(tt.row)
			if tt.empty {
				if got != "" {
					t.Fatalf("warning = %q, want empty", got)
				}

				return
			}

			if !strings.Contains(got, tt.want) {
				t.Fatalf("warning = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInverseStatement_columns(t *testing.T) {
	t.Parallel()

	pg := atlas.DialectPostgres

	tests := []struct {
		name    string
		dialect string
		row     migrationRow
		want    string
		ok      bool
	}{
		{
			name:    "addColumn",
			dialect: pg,
			row:     migrationRow{Kind: kindAddColumn, Table: `"public"."orders"`, Column: "note"},
			want:    `ALTER TABLE "public"."orders" DROP COLUMN IF EXISTS "note";`,
			ok:      true,
		},
		{
			name:    "addColumnSqlite",
			dialect: atlas.DialectSQLite,
			row:     migrationRow{Kind: kindAddColumn, Table: `"orders"`, Column: "note"},
			want:    `ALTER TABLE "orders" DROP COLUMN "note";`,
			ok:      true,
		},
		{
			name:    "dropColumn",
			dialect: pg,
			row:     migrationRow{Kind: kindDropColumn, Table: `"public"."orders"`, Column: "note", PriorType: "TEXT"},
			want:    `ALTER TABLE "public"."orders" ADD COLUMN IF NOT EXISTS "note" TEXT;`,
			ok:      true,
		},
		{
			name:    "dropColumnBadType",
			dialect: pg,
			row:     migrationRow{Kind: kindDropColumn, Table: `"public"."orders"`, Column: "note", PriorType: "TEXT; DROP"},
			ok:      false,
		},
		{
			name:    "renameColumn",
			dialect: pg,
			row:     migrationRow{Kind: kindRenameColumn, Table: `"public"."orders"`, Column: "shipped", PriorName: "sent"},
			want:    `ALTER TABLE "public"."orders" RENAME COLUMN "shipped" TO "sent";`,
			ok:      true,
		},
		{
			name:    "renameColumnBadPrior",
			dialect: pg,
			row:     migrationRow{Kind: kindRenameColumn, Table: `"public"."orders"`, Column: "shipped", PriorName: "not valid!"},
			ok:      false,
		},
		{
			name:    "alterType",
			dialect: pg,
			row:     migrationRow{Kind: kindAlterType, Table: `"public"."orders"`, Column: "total", PriorType: "integer"},
			want:    `ALTER TABLE "public"."orders" ALTER COLUMN "total" TYPE integer USING "total"::integer;`,
			ok:      true,
		},
		{
			name:    "alterTypeMysql",
			dialect: atlas.DialectMySQL,
			row:     migrationRow{Kind: kindAlterType, Table: "`orders`", Column: "total", PriorType: "int NOT NULL"},
			want:    "ALTER TABLE `orders` MODIFY COLUMN `total` int NOT NULL;",
			ok:      true,
		},
		{
			name:    "alterTypeBadPrior",
			dialect: atlas.DialectPostgres,
			row:     migrationRow{Kind: kindAlterType, Table: `"public"."orders"`, Column: "total", PriorType: "integer; DROP TABLE"},
			ok:      false,
		},
		{
			name:    "alterTypeSqliteRefused",
			dialect: atlas.DialectSQLite,
			row:     migrationRow{Kind: kindAlterType, Table: `"orders"`, Column: "total", PriorType: "INTEGER"},
			ok:      false,
		},
		{
			name:    "createTableNoInverse",
			dialect: pg,
			row:     migrationRow{Kind: kindCreateTable, Table: `"public"."orders"`, Column: "id"},
			ok:      false,
		},
		{
			name:    "emptyTable",
			dialect: pg,
			row:     migrationRow{Kind: kindAddColumn, Column: "note"},
			ok:      false,
		},
		{
			name:    "emptyColumn",
			dialect: pg,
			row:     migrationRow{Kind: kindAddColumn, Table: `"public"."orders"`},
			ok:      false,
		},
		{
			name:    "badColumn",
			dialect: pg,
			row:     migrationRow{Kind: kindAddColumn, Table: `"public"."orders"`, Column: "not valid!"},
			ok:      false,
		},
		{
			name:    "unknownKind",
			dialect: pg,
			row:     migrationRow{Kind: "mystery", Table: "t", Column: "c"},
			ok:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := inverseStatement(tt.dialect, tt.row)
			if ok != tt.ok {
				t.Fatalf("inverseStatement ok = %v, want %v (got %q)", ok, tt.ok, got)
			}

			if tt.ok && got != tt.want {
				t.Fatalf("inverseStatement = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInverseStatement_constraints(t *testing.T) {
	t.Parallel()

	pg := atlas.DialectPostgres

	tests := []struct {
		name    string
		dialect string
		row     migrationRow
		want    string
		ok      bool
	}{
		{
			name:    "createIndex",
			dialect: pg,
			row:     migrationRow{Kind: kindCreateIndex, Table: `"public"."orders"`, ObjectName: `"public"."orders_user_id_idx"`},
			want:    `DROP INDEX IF EXISTS "public"."orders_user_id_idx";`,
			ok:      true,
		},
		{
			name:    "createIndexMysql",
			dialect: atlas.DialectMySQL,
			row:     migrationRow{Kind: kindCreateIndex, Table: "`orders`", ObjectName: "`orders_user_id_idx`"},
			want:    "DROP INDEX `orders_user_id_idx` ON `orders`;",
			ok:      true,
		},
		{
			name:    "createIndexNoObject",
			dialect: pg,
			row:     migrationRow{Kind: kindCreateIndex, Table: "t"},
			ok:      false,
		},
		{
			name:    "dropIndex",
			dialect: pg,
			row:     migrationRow{Kind: kindDropIndex, Table: "t", PriorSQL: "CREATE INDEX IF NOT EXISTS \"x\" ON t (\"a\");"},
			want:    "CREATE INDEX IF NOT EXISTS \"x\" ON t (\"a\");",
			ok:      true,
		},
		{
			name:    "dropIndexEmpty",
			dialect: pg,
			row:     migrationRow{Kind: kindDropIndex, Table: "t"},
			ok:      false,
		},
		{
			name:    "dropIndexArbitrarySQLRefused",
			dialect: pg,
			row:     migrationRow{Kind: kindDropIndex, Table: "t", PriorSQL: "DROP TABLE users;"},
			ok:      false,
		},
		{
			name:    "addFK",
			dialect: pg,
			row:     migrationRow{Kind: kindAddForeignKey, Table: `"public"."orders"`, ObjectName: `"orders_user_id_fkey"`},
			want:    `ALTER TABLE "public"."orders" DROP CONSTRAINT IF EXISTS "orders_user_id_fkey";`,
			ok:      true,
		},
		{
			name:    "addFKMysql",
			dialect: atlas.DialectMySQL,
			row:     migrationRow{Kind: kindAddForeignKey, Table: "`orders`", ObjectName: "`orders_user_id_fkey`"},
			want:    "ALTER TABLE `orders` DROP FOREIGN KEY `orders_user_id_fkey`;",
			ok:      true,
		},
		{
			name:    "addFKNoObject",
			dialect: pg,
			row:     migrationRow{Kind: kindAddForeignKey, Table: "t"},
			ok:      false,
		},
		{
			name:    "nullabilityToNull",
			dialect: pg,
			row:     migrationRow{Kind: kindAlterNullability, Table: `"public"."orders"`, Column: "note", PriorType: "NULL"},
			want:    `ALTER TABLE "public"."orders" ALTER COLUMN "note" DROP NOT NULL;`,
			ok:      true,
		},
		{
			name:    "nullabilityToNotNull",
			dialect: pg,
			row:     migrationRow{Kind: kindAlterNullability, Table: `"public"."orders"`, Column: "note", PriorType: "NOT NULL"},
			want:    `ALTER TABLE "public"."orders" ALTER COLUMN "note" SET NOT NULL;`,
			ok:      true,
		},
		{
			name:    "nullabilityBadPrior",
			dialect: pg,
			row:     migrationRow{Kind: kindAlterNullability, Table: "t", Column: "c", PriorType: "MAYBE"},
			ok:      false,
		},
		{
			name:    "nullabilityMysql",
			dialect: atlas.DialectMySQL,
			row:     migrationRow{Kind: kindAlterNullability, Table: "`orders`", Column: "note", PriorType: "varchar(10) NULL"},
			want:    "ALTER TABLE `orders` MODIFY COLUMN `note` varchar(10) NULL;",
			ok:      true,
		},
		{
			name:    "nullabilityMysqlBadPrior",
			dialect: atlas.DialectMySQL,
			row:     migrationRow{Kind: kindAlterNullability, Table: "`orders`", Column: "note", PriorType: "nope;"},
			ok:      false,
		},
		{
			name:    "unknownConstraintKind",
			dialect: pg,
			row:     migrationRow{Kind: "mystery", Table: "t"},
			ok:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := inverseConstraintStatement(tt.dialect, tt.row)
			if ok != tt.ok {
				t.Fatalf("inverseConstraintStatement ok = %v, want %v (got %q)", ok, tt.ok, got)
			}

			if tt.ok && got != tt.want {
				t.Fatalf("inverseConstraintStatement = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReadRecentMigrations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("rowsAndNulls", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{
					{int64(1), "abc", "add_column", "t", "c", "stmt", "TEXT", nil, []byte("obj"), 7},
				}}, nil
			},
		}

		rows, err := readRecentMigrations(ctx, conn, 1)
		if err != nil {
			t.Fatalf("readRecentMigrations: %v", err)
		}

		if len(rows) != 1 {
			t.Fatalf("rows = %+v", rows)
		}

		r := rows[0]
		if r.ID != 1 || r.Checksum != "abc" || r.Kind != "add_column" || r.PriorName != "" || r.ObjectName != "obj" || r.PriorSQL != "7" {
			t.Fatalf("row = %+v", r)
		}
	})

	t.Run("queryError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		if _, err := readRecentMigrations(ctx, conn, 1); err == nil {
			t.Fatal("readRecentMigrations: want error, got nil")
		}
	})

	t.Run("scanError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{int64(1)}}, scanErr: errors.New("boom")}, nil
			},
		}

		if _, err := readRecentMigrations(ctx, conn, 1); err == nil {
			t.Fatal("readRecentMigrations: want scan error, got nil")
		}
	})

	t.Run("iterateError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{
					vals:    [][]any{{int64(1), "c", "k", "t", "c", "s", nil, nil, nil, nil}},
					iterErr: errors.New("boom"),
				}, nil
			},
		}

		if _, err := readRecentMigrations(ctx, conn, 1); err == nil {
			t.Fatal("readRecentMigrations: want iterate error, got nil")
		}
	})
}

func TestNullableText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   any
		want string
	}{
		{"nil", nil, ""},
		{"string", "x", "x"},
		{"bytes", []byte("y"), "y"},
		{"int", 7, "7"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := nullableText(tt.in); got != tt.want {
				t.Fatalf("nullableText = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestApplyRollback(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("nilPlan", func(t *testing.T) {
		t.Parallel()

		if _, err := ApplyRollback(ctx, &fakeDB{dialect: atlas.DialectSQLite}, nil); err == nil {
			t.Fatal("ApplyRollback(nil): want error, got nil")
		}
	})

	t.Run("executesInOrder", func(t *testing.T) {
		t.Parallel()

		var got []string

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onExec: func(q string, _ []any) (int64, error) {
				got = append(got, q)
				return 1, nil
			},
		}

		n, err := ApplyRollback(ctx, conn, &RollbackPlan{
			Dialect:    atlas.DialectSQLite,
			Statements: []string{"DROP INDEX a;", "DELETE FROM schema_migrations WHERE id = 1;"},
		})
		if err != nil || n != 2 {
			t.Fatalf("ApplyRollback = (%d, %v), want (2, nil)", n, err)
		}

		if len(got) != 2 || got[0] != "DROP INDEX a;" {
			t.Fatalf("executed = %v", got)
		}
	})

	t.Run("stopsAtFirstFailure", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onExec: func(q string, _ []any) (int64, error) {
				if strings.Contains(q, "BAD") {
					return 0, errors.New("boom")
				}

				return 1, nil
			},
		}

		n, err := ApplyRollback(ctx, conn, &RollbackPlan{
			Dialect:    atlas.DialectSQLite,
			Statements: []string{"BAD;", "DELETE FROM schema_migrations WHERE id = 1;"},
		})
		if err == nil || n != 0 {
			t.Fatalf("ApplyRollback = (%d, %v), want (0, err)", n, err)
		}
	})
}
