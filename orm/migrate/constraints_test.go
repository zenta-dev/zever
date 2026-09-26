package migrate

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/dsl/backend/atlas"
	"github.com/zenta-dev/zever/dsl/ir"
)

func TestLooksManaged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		index string
		table string
		want  bool
	}{
		{"plainIdx", "orders_user_id_idx", "orders", true},
		{"uniqueKey", "orders_email_key", "orders", true},
		{"dbaIndex", "dba_custom_idx2", "orders", false},
		{"sqliteAutoindex", "sqlite_autoindex_users_1", "users", false},
		{"otherTable", "users_email_idx", "orders", false},
		{"primaryKey", "users_pkey", "users", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := looksManaged(tt.index, tt.table); got != tt.want {
				t.Fatalf("looksManaged(%q, %q) = %v, want %v", tt.index, tt.table, got, tt.want)
			}
		})
	}
}

func TestUniqueIndexName(t *testing.T) {
	t.Parallel()

	if got := uniqueIndexName("orders", "email"); got != "orders_email_key" {
		t.Fatalf("uniqueIndexName = %q", got)
	}
}

func TestCreateIndexSQL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dialect string
		unique  bool
		want    string
		wantErr bool
	}{
		{
			name:    "pgPlain",
			dialect: atlas.DialectPostgres,
			want:    `CREATE INDEX IF NOT EXISTS "orders_user_id_idx" ON "public"."orders" ("user_id");`,
		},
		{
			name:    "pgUnique",
			dialect: atlas.DialectPostgres,
			unique:  true,
			want:    `CREATE UNIQUE INDEX IF NOT EXISTS "orders_user_id_idx" ON "public"."orders" ("user_id");`,
		},
		{
			name:    "mysqlNoIfNotExists",
			dialect: atlas.DialectMySQL,
			want:    "CREATE INDEX `orders_user_id_idx` ON `orders` (`user_id`);",
		},
		{
			name:    "badName",
			dialect: atlas.DialectPostgres,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			name := "orders_user_id_idx"
			cols := []string{"user_id"}

			if tt.wantErr {
				name = "not valid!"
			}

			table := `"public"."orders"`
			if tt.dialect == atlas.DialectMySQL {
				table = "`orders`"
			}

			got, err := createIndexSQL(tt.dialect, table, name, cols, tt.unique)
			if tt.wantErr {
				if err == nil {
					t.Fatal("createIndexSQL: want error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("createIndexSQL: %v", err)
			}

			if got != tt.want {
				t.Fatalf("createIndexSQL = %q, want %q", got, tt.want)
			}
		})
	}

	if _, err := createIndexSQL(atlas.DialectPostgres, "t", "good_idx", []string{"bad col!"}, false); err == nil {
		t.Fatal("createIndexSQL bad column: want error, got nil")
	}
}

func TestQualifiedIndexName(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "Order", Schema: "public"}

	if got := qualifiedIndexName(atlas.DialectPostgres, e, "orders_user_id_idx"); got != `"public"."orders_user_id_idx"` {
		t.Fatalf("pg qualified = %q", got)
	}

	if got := qualifiedIndexName(atlas.DialectSQLite, e, "orders_user_id_idx"); got != `"orders_user_id_idx"` {
		t.Fatalf("sqlite qualified = %q", got)
	}

	if got := qualifiedIndexName(atlas.DialectMySQL, e, "orders_user_id_idx"); got != "`orders_user_id_idx`" {
		t.Fatalf("mysql qualified = %q", got)
	}
}

func TestRenderCreateAndDropIndex(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "Order", Schema: "public"}
	spec := atlas.IndexSpec{Name: "orders_user_id_idx", Columns: []string{"user_id"}}

	created, err := renderCreateIndex(atlas.DialectSQLite, e, spec)
	if err != nil {
		t.Fatalf("renderCreateIndex: %v", err)
	}

	if created.Meta.Kind != kindCreateIndex || created.Meta.ObjectName == "" {
		t.Fatalf("meta = %+v", created.Meta)
	}

	if _, renderErr := renderCreateIndex("oracle", e, spec); renderErr == nil {
		t.Fatal("renderCreateIndex(oracle): want error, got nil")
	}

	dropped, err := renderDropIndex(atlas.DialectSQLite, e, liveIndex{Name: "orders_old_idx", Columns: []string{"user_id"}})
	if err != nil {
		t.Fatalf("renderDropIndex: %v", err)
	}

	if dropped.Meta.Kind != kindDropIndex || dropped.Meta.PriorSQL == "" {
		t.Fatalf("meta = %+v", dropped.Meta)
	}

	myDrop, err := renderDropIndex(atlas.DialectMySQL, e, liveIndex{Name: "orders_old_idx", Columns: []string{"user_id"}})
	if err != nil {
		t.Fatalf("renderDropIndex mysql: %v", err)
	}

	if !strings.Contains(myDrop.SQL, " ON ") {
		t.Fatalf("mysql drop = %q, want DROP INDEX ... ON", myDrop.SQL)
	}

	if _, err := renderDropIndex(atlas.DialectSQLite, e, liveIndex{Name: "bad!", Columns: []string{"user_id"}}); err == nil {
		t.Fatal("renderDropIndex bad name: want error, got nil")
	}
}

func TestIntrospectIndexes(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	e := &ir.Entity{Name: "Order"}

	t.Run("postgresFromState", func(t *testing.T) {
		t.Parallel()

		state := &liveSchemaState{indexes: map[string][]liveIndex{
			"orders": {{Name: "orders_user_id_idx", Columns: []string{"user_id"}}},
		}}

		got, err := introspectIndexes(ctx, &fakeDB{dialect: atlas.DialectPostgres}, atlas.DialectPostgres, e, state)
		if err != nil || len(got) != 1 {
			t.Fatalf("indexes = %+v, err = %v", got, err)
		}
	})

	t.Run("mysql", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"orders_user_id_idx", "user_id", 1}}}, nil
			},
		}

		got, err := introspectIndexes(ctx, conn, atlas.DialectMySQL, e, &liveSchemaState{})
		if err != nil || len(got) != 1 {
			t.Fatalf("indexes = %+v, err = %v", got, err)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		t.Parallel()

		if _, err := introspectIndexes(ctx, &fakeDB{dialect: "oracle"}, "oracle", e, &liveSchemaState{}); err == nil {
			t.Fatal("introspectIndexes(oracle): want error, got nil")
		}
	})
}

func TestIntrospectPostgresIndexes(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("groupedOrdered", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{
					{"orders", "orders_pair_idx", "a", false},
					{"orders", "orders_pair_idx", "b", false},
					{"orders", "orders_email_key", "email", true},
				}}, nil
			},
		}

		got, err := introspectPostgresIndexes(ctx, conn, "public", []string{"orders"})
		if err != nil {
			t.Fatalf("introspectPostgresIndexes: %v", err)
		}

		idx := got["orders"]
		if len(idx) != 2 || idx[0].Name != "orders_pair_idx" || len(idx[0].Columns) != 2 {
			t.Fatalf("indexes = %+v", idx)
		}

		if !idx[1].Unique {
			t.Fatalf("indexes = %+v", idx)
		}
	})

	t.Run("emptyTables", func(t *testing.T) {
		t.Parallel()

		got, err := introspectPostgresIndexes(ctx, &fakeDB{dialect: atlas.DialectPostgres}, "public", nil)
		if err != nil || len(got) != 0 {
			t.Fatalf("got = %+v, err = %v", got, err)
		}
	})

	t.Run("queryError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		if _, err := introspectPostgresIndexes(ctx, conn, "public", []string{"orders"}); err == nil {
			t.Fatal("want error, got nil")
		}
	})

	t.Run("scanError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"o", "i", "c", true}}, scanErr: errors.New("boom")}, nil
			},
		}

		if _, err := introspectPostgresIndexes(ctx, conn, "public", []string{"orders"}); err == nil {
			t.Fatal("want scan error, got nil")
		}
	})

	t.Run("iterateError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{
					vals:    [][]any{{"o", "i", "c", true}},
					iterErr: errors.New("boom"),
				}, nil
			},
		}

		if _, err := introspectPostgresIndexes(ctx, conn, "public", []string{"orders"}); err == nil {
			t.Fatal("want iterate error, got nil")
		}
	})
}

func TestIntrospectSQLiteIndexes_live(t *testing.T) {
	ctx := t.Context()
	conn := openTestSQLite(t, "idx.db")

	if _, err := conn.Exec(ctx, `CREATE TABLE "widgets" ("id" TEXT NOT NULL, "size" INTEGER);`); err != nil {
		t.Fatalf("seed table: %v", err)
	}

	if _, err := conn.Exec(ctx, `CREATE INDEX "widgets_size_idx" ON "widgets" ("size");`); err != nil {
		t.Fatalf("seed index: %v", err)
	}

	got, err := introspectSQLiteIndexes(ctx, conn, "widgets")
	if err != nil {
		t.Fatalf("introspectSQLiteIndexes: %v", err)
	}

	if len(got) != 1 || got[0].Name != "widgets_size_idx" || len(got[0].Columns) != 1 {
		t.Fatalf("indexes = %+v", got)
	}

	if _, err := introspectSQLiteIndexes(ctx, conn, "bad!"); err == nil {
		t.Fatal("bad table: want error, got nil")
	}

	if _, err := introspectSQLiteIndexColumns(ctx, conn, "bad!"); err == nil {
		t.Fatal("bad index: want error, got nil")
	}
}

func TestIndexStatementsFor_integration(t *testing.T) {
	ctx := t.Context()
	conn := openTestSQLite(t, "indexplan.db")

	v1 := compileSchema(t, applyTestSchema)

	plan, err := Plan(ctx, conn, v1.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if _, applyErr := Apply(ctx, conn, plan); applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}

	// Steady state: unique email index exists live, plan is empty.
	steady, err := Plan(ctx, conn, v1.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan steady: %v", err)
	}

	if len(steady.Statements()) != 0 {
		t.Fatalf("steady = %v, want empty", steady.Statements())
	}

	// A hand-added managed-looking index gets dropped automatically.
	if _, execErr := conn.Exec(ctx, `CREATE INDEX "users_nick_idx" ON "users" ("email");`); execErr != nil {
		t.Fatalf("seed stray index: %v", execErr)
	}

	drop, err := Plan(ctx, conn, v1.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan drop: %v", err)
	}

	found := false

	for _, s := range drop.Statements() {
		if strings.Contains(s, "DROP INDEX") && strings.Contains(s, "users_nick_idx") {
			found = true
		}
	}

	if !found {
		t.Fatalf("drop plan = %v, want DROP of stray managed index", drop.Statements())
	}

	// A hand-added foreign-looking index is left alone.
	if _, execErr := conn.Exec(ctx, `CREATE INDEX "dba_helper" ON "users" ("email");`); execErr != nil {
		t.Fatalf("seed dba index: %v", execErr)
	}

	// Remove the unique attribute: warns, never drops.
	v2 := compileSchema(t, `entity User {
	id: uuid @primary
	email: string
}
`)

	warn, err := Plan(ctx, conn, v2.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan unwarn: %v", err)
	}

	for _, s := range warn.Statements() {
		if strings.Contains(s, "DROP") && strings.Contains(s, "users_email_key") {
			t.Fatalf("unique constraint was dropped automatically: %v", warn.Statements())
		}
	}

	if len(warn.Warnings) == 0 {
		t.Fatal("want an undeclared-unique warning, got none")
	}
}

func TestRenderIndexErrors(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "Order", Schema: "public"}

	if _, err := renderCreateIndex(atlas.DialectSQLite, e, atlas.IndexSpec{Name: "bad!", Columns: []string{"a"}}); err == nil {
		t.Fatal("renderCreateIndex bad name: want error, got nil")
	}

	if _, err := renderDropIndex("oracle", e, liveIndex{Name: "orders_old_idx", Columns: []string{"a"}}); err == nil {
		t.Fatal("renderDropIndex(oracle): want error, got nil")
	}

	if _, err := renderDropIndex(atlas.DialectSQLite, e, liveIndex{Name: "orders_old_idx", Columns: []string{"bad col!"}}); err == nil {
		t.Fatal("renderDropIndex bad columns: want error, got nil")
	}
}

func TestIntrospectSQLiteIndexes_errors(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("queryError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		if _, err := introspectSQLiteIndexes(ctx, conn, "widgets"); err == nil {
			t.Fatal("want query error, got nil")
		}
	})

	t.Run("scanError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{0, "i", 1, "c", 0}}, scanErr: errors.New("boom")}, nil
			},
		}

		if _, err := introspectSQLiteIndexes(ctx, conn, "widgets"); err == nil {
			t.Fatal("want scan error, got nil")
		}
	})

	t.Run("closeError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{closeErr: errors.New("boom")}, nil
			},
		}

		if _, err := introspectSQLiteIndexes(ctx, conn, "widgets"); err == nil {
			t.Fatal("want close error, got nil")
		}
	})

	t.Run("columnsError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(q string, _ []any) (db.Rows, error) {
				if strings.Contains(q, "index_list") {
					return &fakeRows{vals: [][]any{{0, "widgets_size_idx", 0, "c", 0}}}, nil
				}

				return nil, errors.New("boom")
			},
		}

		if _, err := introspectSQLiteIndexes(ctx, conn, "widgets"); err == nil {
			t.Fatal("want columns error, got nil")
		}
	})

	t.Run("pkOriginSkipped", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(q string, _ []any) (db.Rows, error) {
				if strings.Contains(q, "index_list") {
					return &fakeRows{vals: [][]any{{0, "pk_idx", 1, "pk", 0}}}, nil
				}

				return &fakeRows{}, nil
			},
		}

		got, err := introspectSQLiteIndexes(ctx, conn, "widgets")
		if err != nil || len(got) != 0 {
			t.Fatalf("got = %+v, err = %v, want empty", got, err)
		}
	})
}

func TestIntrospectSQLiteIndexColumns_errors(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("queryError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		if _, err := introspectSQLiteIndexColumns(ctx, conn, "idx"); err == nil {
			t.Fatal("want query error, got nil")
		}
	})

	t.Run("scanError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{0, 1, "a"}}, scanErr: errors.New("boom")}, nil
			},
		}

		if _, err := introspectSQLiteIndexColumns(ctx, conn, "idx"); err == nil {
			t.Fatal("want scan error, got nil")
		}
	})

	t.Run("nonStringSkipped", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{0, 1, nil}, {1, 2, "b"}}}, nil
			},
		}

		got, err := introspectSQLiteIndexColumns(ctx, conn, "idx")
		if err != nil || len(got) != 1 || got[0] != "b" {
			t.Fatalf("got = %+v, err = %v", got, err)
		}
	})
}

func TestIndexDiffDirect(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "Order", Schema: "public"}

	t.Run("declaredUniqueSingleColRecorded", func(t *testing.T) {
		t.Parallel()

		col := &ir.Field{Name: "user_id"}
		de := &ir.Entity{
			Name:   "Order",
			Fields: []*ir.Field{{Name: "id"}, col},
			Indexes: []*ir.Index{
				{Columns: []*ir.Field{col}, Unique: true},
			},
		}

		d := newIndexDiff(de, nil)

		if !d.declaredUniqueCols["user_id"] {
			t.Fatalf("declaredUniqueCols = %v, want user_id", d.declaredUniqueCols)
		}

		if len(d.declaredByName) != 1 {
			t.Fatalf("declaredByName = %v, want 1 entry", d.declaredByName)
		}
	})

	t.Run("createDeclaredError", func(t *testing.T) {
		t.Parallel()

		d := &indexDiff{
			table: "orders",
			liveByName: map[string]liveIndex{
				"orders_keep_idx": {Name: "orders_keep_idx", Columns: []string{"a"}},
			},
			declaredByName: map[string]atlas.IndexSpec{
				"orders_keep_idx": {Name: "orders_keep_idx", Columns: []string{"a"}},
				"bad!":            {Name: "bad!", Columns: []string{"a"}},
			},
		}

		if _, err := d.createDeclaredIndexes(atlas.DialectSQLite, e); err == nil {
			t.Fatal("createDeclaredIndexes bad name: want error, got nil")
		}
	})

	t.Run("createDeclaredMissing", func(t *testing.T) {
		t.Parallel()

		d := &indexDiff{
			table:      "orders",
			liveByName: map[string]liveIndex{},
			declaredByName: map[string]atlas.IndexSpec{
				"orders_user_id_idx": {Name: "orders_user_id_idx", Columns: []string{"user_id"}},
			},
		}

		stmts, err := d.createDeclaredIndexes(atlas.DialectSQLite, e)
		if err != nil || len(stmts) != 1 {
			t.Fatalf("stmts = %+v, err = %v", stmts, err)
		}
	})

	t.Run("createUniqueMissing", func(t *testing.T) {
		t.Parallel()

		ue := &ir.Entity{Name: "User", Fields: []*ir.Field{
			{Name: "id", Primary: true},
			{Name: "email", Unique: true},
		}}
		d := newIndexDiff(ue, nil)

		stmts, err := d.createUniqueFieldIndexes(atlas.DialectSQLite, ue)
		if err != nil || len(stmts) != 1 {
			t.Fatalf("stmts = %+v, err = %v", stmts, err)
		}
	})

	t.Run("createUniqueError", func(t *testing.T) {
		t.Parallel()

		ue := &ir.Entity{Name: "User", Fields: []*ir.Field{
			{Name: "email", Unique: true},
		}}
		d := newIndexDiff(ue, nil)

		if _, err := d.createUniqueFieldIndexes("oracle", ue); err == nil {
			t.Fatal("createUniqueFieldIndexes(oracle): want error, got nil")
		}
	})

	t.Run("dropSkipsUniqueFieldAndRendersError", func(t *testing.T) {
		t.Parallel()

		ue := &ir.Entity{Name: "User", Fields: []*ir.Field{
			{Name: "email", Unique: true},
		}}
		d := newIndexDiff(ue, []liveIndex{
			{Name: "users_email_key", Columns: []string{"email"}, Unique: true},
			{Name: "users_stray_idx", Columns: []string{"bad col!"}},
		})

		// users_email_key is the unique field's own index: skipped, and the
		// bad-columns stray fails rendering.
		if _, err := d.dropManagedIndexes(atlas.DialectSQLite, ue); err == nil {
			t.Fatal("dropManagedIndexes bad columns: want error, got nil")
		}
	})
}

func TestRenderAddForeignKey_badIdents(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "Order", Schema: "public"}

	if _, err := renderAddForeignKey(atlas.DialectPostgres, e, atlas.ForeignKey{Name: "f", Column: "bad!"}); err == nil {
		t.Fatal("bad column: want error, got nil")
	}

	if _, err := renderAddForeignKey(atlas.DialectPostgres, e, atlas.ForeignKey{Name: "bad!", Column: "c"}); err == nil {
		t.Fatal("bad name: want error, got nil")
	}
}

func TestIntrospectSQLiteForeignKeys_errors(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("queryError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		if _, err := introspectSQLiteForeignKeys(ctx, conn, "orders"); err == nil {
			t.Fatal("want query error, got nil")
		}
	})

	t.Run("scanError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{0, 0, "u", "c", "i", nil, nil, nil}}, scanErr: errors.New("boom")}, nil
			},
		}

		if _, err := introspectSQLiteForeignKeys(ctx, conn, "orders"); err == nil {
			t.Fatal("want scan error, got nil")
		}
	})
}

func TestForeignKeyStatementsFor_postgres(t *testing.T) {
	const fkSchema = `entity User {
	id: uuid @primary
}

entity Order {
	id: uuid @primary
	user_id: uuid

	belongs_to user: User @foreign_key(user_id) @on_delete(cascade)
}
`

	t.Run("addsMissing", func(t *testing.T) {
		schema := compileSchema(t, fkSchema).Schema

		var order *ir.Entity

		for _, m := range schema.Modules {
			for _, e := range m.Entities {
				if e.Name == "Order" {
					order = e
				}
			}
		}

		if order == nil {
			t.Fatal("order entity not found")
		}

		w := &warnings{}
		stmts, err := foreignKeyStatementsFor(
			t.Context(), &fakeDB{dialect: atlas.DialectPostgres},
			atlas.DialectPostgres, order, &liveSchemaState{}, w,
		)
		if err != nil {
			t.Fatalf("foreignKeyStatementsFor: %v", err)
		}

		if len(stmts) != 1 || !strings.Contains(stmts[0].SQL, "ADD CONSTRAINT") {
			t.Fatalf("stmts = %+v", stmts)
		}
	})

	t.Run("warnsOnUndeclaredLive", func(t *testing.T) {
		schema := compileSchema(t, fkSchema).Schema

		var order *ir.Entity

		for _, m := range schema.Modules {
			for _, e := range m.Entities {
				if e.Name == "Order" {
					order = e
				}
			}
		}

		state := &liveSchemaState{foreignKeys: map[string][]liveForeignKey{
			"orders": {
				{Column: "user_id", RefTable: "users", RefColumn: "id"},
				{Column: "legacy_id", RefTable: "legacy", RefColumn: "id"},
			},
		}}

		w := &warnings{}
		_, err := foreignKeyStatementsFor(
			t.Context(), &fakeDB{dialect: atlas.DialectPostgres},
			atlas.DialectPostgres, order, state, w,
		)
		if err != nil {
			t.Fatalf("foreignKeyStatementsFor: %v", err)
		}

		if len(w.messages()) != 1 || !strings.Contains(w.messages()[0], "legacy_id") {
			t.Fatalf("warnings = %v", w.messages())
		}
	})

	t.Run("introspectError", func(t *testing.T) {
		schema := compileSchema(t, fkSchema).Schema

		var order *ir.Entity

		for _, m := range schema.Modules {
			for _, e := range m.Entities {
				if e.Name == "Order" {
					order = e
				}
			}
		}

		w := &warnings{}

		if _, err := foreignKeyStatementsFor(
			t.Context(), &fakeDB{dialect: "oracle"}, "oracle", order, &liveSchemaState{}, w,
		); err == nil {
			t.Fatal("want introspect error, got nil")
		}
	})
}

func TestCreateUniqueFieldIndexes_integration(t *testing.T) {
	ctx := t.Context()
	conn := openTestSQLite(t, "uniqueplan.db")

	// Seed the table WITHOUT the unique index the schema declares.
	if _, err := conn.Exec(ctx, `CREATE TABLE "users" ("id" TEXT NOT NULL, "email" TEXT);`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	v1 := compileSchema(t, applyTestSchema)

	plan, err := Plan(ctx, conn, v1.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	found := false

	for _, s := range plan.Statements() {
		if strings.Contains(s, "CREATE UNIQUE INDEX") && strings.Contains(s, "users_email_key") {
			found = true
		}
	}

	if !found {
		t.Fatalf("plan = %v, want CREATE UNIQUE INDEX for @unique email", plan.Statements())
	}
}

func TestIndexStatementsFor_declaredIndexSteady(t *testing.T) {
	ctx := t.Context()
	conn := openTestSQLite(t, "declaredsteady.db")

	v := compileSchema(t, `entity User {
	id: uuid @primary
	email: string @unique

	index(email)
}
`)

	plan, err := Plan(ctx, conn, v.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if _, applyErr := Apply(ctx, conn, plan); applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}

	steady, err := Plan(ctx, conn, v.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan steady: %v", err)
	}

	if len(steady.Statements()) != 0 {
		t.Fatalf("steady = %v, want empty (declared indexes skipped)", steady.Statements())
	}
}

func TestOnDeleteClause(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"restrict", " ON DELETE RESTRICT"},
		{"cascade", " ON DELETE CASCADE"},
		{"set_null", " ON DELETE SET NULL"},
		{"", ""},
		{"bogus", ""},
	}

	for _, tt := range tests {
		if got := onDeleteClause(tt.in); got != tt.want {
			t.Fatalf("onDeleteClause(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRenderAddForeignKey(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "Order", Schema: "public"}

	pg, err := renderAddForeignKey(atlas.DialectPostgres, e, atlas.ForeignKey{
		Name: "orders_user_id_fkey", Column: "user_id",
		RefTable: "users", RefSchema: "public", RefColumn: "id", OnDelete: "cascade",
	})
	if err != nil {
		t.Fatalf("renderAddForeignKey pg: %v", err)
	}

	if pg.Meta.Kind != kindAddForeignKey || !strings.Contains(pg.SQL, "ON DELETE CASCADE") {
		t.Fatalf("pg = %+v", pg)
	}

	my, err := renderAddForeignKey(atlas.DialectMySQL, e, atlas.ForeignKey{
		Name: "orders_user_id_fkey", Column: "user_id",
		RefTable: "users", RefColumn: "id",
	})
	if err != nil {
		t.Fatalf("renderAddForeignKey mysql: %v", err)
	}

	if !strings.Contains(my.SQL, "ADD CONSTRAINT") {
		t.Fatalf("mysql = %+v", my)
	}

	if _, err := renderAddForeignKey(atlas.DialectSQLite, e, atlas.ForeignKey{Name: "f", Column: "c"}); err == nil {
		t.Fatal("renderAddForeignKey(sqlite): want error, got nil")
	}

	if _, err := renderAddForeignKey("oracle", e, atlas.ForeignKey{Name: "f", Column: "c"}); err == nil {
		t.Fatal("renderAddForeignKey(oracle): want error, got nil")
	}
}

func TestIntrospectForeignKeys(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	e := &ir.Entity{Name: "Order"}

	t.Run("postgresFromState", func(t *testing.T) {
		t.Parallel()

		state := &liveSchemaState{foreignKeys: map[string][]liveForeignKey{
			"orders": {{Column: "user_id", RefTable: "users", RefColumn: "id"}},
		}}

		got, err := introspectForeignKeys(ctx, &fakeDB{dialect: atlas.DialectPostgres}, atlas.DialectPostgres, e, state)
		if err != nil || len(got) != 1 {
			t.Fatalf("fks = %+v, err = %v", got, err)
		}
	})

	t.Run("mysql", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"user_id", "users", "id"}}}, nil
			},
		}

		got, err := introspectForeignKeys(ctx, conn, atlas.DialectMySQL, e, &liveSchemaState{})
		if err != nil || len(got) != 1 {
			t.Fatalf("fks = %+v, err = %v", got, err)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		t.Parallel()

		if _, err := introspectForeignKeys(ctx, &fakeDB{dialect: "oracle"}, "oracle", e, &liveSchemaState{}); err == nil {
			t.Fatal("introspectForeignKeys(oracle): want error, got nil")
		}
	})
}

func TestIntrospectPostgresForeignKeys(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("rows", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"orders", "user_id", "users", "id"}}}, nil
			},
		}

		got, err := introspectPostgresForeignKeys(ctx, conn, "public", []string{"orders"})
		if err != nil {
			t.Fatalf("introspectPostgresForeignKeys: %v", err)
		}

		if len(got["orders"]) != 1 || got["orders"][0].Column != "user_id" {
			t.Fatalf("fks = %+v", got)
		}
	})

	t.Run("emptyTables", func(t *testing.T) {
		t.Parallel()

		got, err := introspectPostgresForeignKeys(ctx, &fakeDB{dialect: atlas.DialectPostgres}, "public", nil)
		if err != nil || len(got) != 0 {
			t.Fatalf("got = %+v, err = %v", got, err)
		}
	})

	t.Run("queryError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		if _, err := introspectPostgresForeignKeys(ctx, conn, "public", []string{"orders"}); err == nil {
			t.Fatal("want error, got nil")
		}
	})

	t.Run("scanError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"o", "c", "r", "rc"}}, scanErr: errors.New("boom")}, nil
			},
		}

		if _, err := introspectPostgresForeignKeys(ctx, conn, "public", []string{"orders"}); err == nil {
			t.Fatal("want scan error, got nil")
		}
	})
}

func TestIntrospectSQLiteForeignKeys_live(t *testing.T) {
	ctx := t.Context()
	conn := openTestSQLite(t, "fk.db")

	if _, err := conn.Exec(ctx, `CREATE TABLE "users" ("id" TEXT NOT NULL PRIMARY KEY);`); err != nil {
		t.Fatalf("seed users: %v", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE "orders" ("id" TEXT NOT NULL, "user_id" TEXT REFERENCES "users" ("id"));`); err != nil {
		t.Fatalf("seed orders: %v", err)
	}

	got, err := introspectSQLiteForeignKeys(ctx, conn, "orders")
	if err != nil {
		t.Fatalf("introspectSQLiteForeignKeys: %v", err)
	}

	if len(got) != 1 || got[0].Column != "user_id" || got[0].RefTable != "users" {
		t.Fatalf("fks = %+v", got)
	}

	if _, err := introspectSQLiteForeignKeys(ctx, conn, "bad!"); err == nil {
		t.Fatal("bad table: want error, got nil")
	}
}

func TestForeignKeyStatementsFor_sqliteWarns(t *testing.T) {
	ctx := t.Context()
	conn := openTestSQLite(t, "fkwarn.db")

	// Seed tables WITHOUT foreign keys so the declared FK is missing live.
	if _, err := conn.Exec(ctx, `CREATE TABLE "users" ("id" TEXT NOT NULL PRIMARY KEY);`); err != nil {
		t.Fatalf("seed users: %v", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE "orders" ("id" TEXT NOT NULL, "user_id" TEXT);`); err != nil {
		t.Fatalf("seed orders: %v", err)
	}

	v := compileSchema(t, `entity User {
	id: uuid @primary
}

entity Order {
	id: uuid @primary
	user_id: uuid

	belongs_to user: User @foreign_key(user_id) @on_delete(cascade)
}
`)

	plan, err := Plan(ctx, conn, v.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	// SQLite cannot add FK constraints to existing tables: warned, skipped.
	if len(plan.Warnings) == 0 {
		t.Fatal("want an FK warning on sqlite, got none")
	}

	for _, s := range plan.Statements() {
		if strings.Contains(s, "ADD CONSTRAINT") {
			t.Fatalf("sqlite plan adds an FK constraint: %v", plan.Statements())
		}
	}
}

func TestNullabilityStatementFor(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "User"}
	f := &ir.Field{Name: "note", Optional: true}

	t.Run("matchIsNoop", func(t *testing.T) {
		t.Parallel()

		w := &warnings{}
		stmt, err := nullabilityStatementFor(atlas.DialectPostgres, e, f, liveColumn{Name: "note", Nullable: true}, false, w)
		if err != nil || stmt.SQL != "" {
			t.Fatalf("stmt = %+v, err = %v", stmt, err)
		}
	})

	t.Run("postgres", func(t *testing.T) {
		t.Parallel()

		w := &warnings{}
		stmt, err := nullabilityStatementFor(atlas.DialectPostgres, e, f, liveColumn{Name: "note", Nullable: false}, false, w)
		if err != nil || !strings.Contains(stmt.SQL, "DROP NOT NULL") {
			t.Fatalf("stmt = %+v, err = %v", stmt, err)
		}

		if stmt.Meta.PriorType != "NOT NULL" {
			t.Fatalf("meta = %+v", stmt.Meta)
		}
	})

	t.Run("mysql", func(t *testing.T) {
		t.Parallel()

		w := &warnings{}
		mf := &ir.Field{Name: "note", Optional: false, Type: strField("note", ir.TString).Type}
		stmt, err := nullabilityStatementFor(atlas.DialectMySQL, e, mf, liveColumn{Name: "note", RawType: "text", Nullable: true}, false, w)
		if err != nil || !strings.Contains(stmt.SQL, "MODIFY COLUMN") {
			t.Fatalf("stmt = %+v, err = %v", stmt, err)
		}
	})

	t.Run("sqliteWarns", func(t *testing.T) {
		t.Parallel()

		w := &warnings{}
		stmt, err := nullabilityStatementFor(atlas.DialectSQLite, e, f, liveColumn{Name: "note", Nullable: false}, false, w)
		if err != nil || stmt.SQL != "" {
			t.Fatalf("stmt = %+v, err = %v", stmt, err)
		}

		if len(w.messages()) != 1 {
			t.Fatalf("warnings = %v, want 1", w.messages())
		}
	})

	t.Run("forcedNullableExcluded", func(t *testing.T) {
		t.Parallel()

		w := &warnings{}
		// Declared non-optional but forced nullable by set_null FK: matches
		// live nullable, so no statement.
		nf := &ir.Field{Name: "user_id"}
		stmt, err := nullabilityStatementFor(atlas.DialectPostgres, e, nf, liveColumn{Name: "user_id", Nullable: true}, true, w)
		if err != nil || stmt.SQL != "" {
			t.Fatalf("stmt = %+v, err = %v", stmt, err)
		}
	})
}

func TestRenderAlterNullability(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "User"}

	set, err := renderAlterNullability(atlas.DialectPostgres, e, &ir.Field{Name: "note"}, false)
	if err != nil {
		t.Fatalf("renderAlterNullability: %v", err)
	}

	if !strings.Contains(set.SQL, "SET NOT NULL") || set.Meta.PriorType != "NULL" {
		t.Fatalf("set = %+v", set)
	}

	drop, err := renderAlterNullability(atlas.DialectPostgres, e, &ir.Field{Name: "note"}, true)
	if err != nil {
		t.Fatalf("renderAlterNullability drop: %v", err)
	}

	if !strings.Contains(drop.SQL, "DROP NOT NULL") || drop.Meta.PriorType != "NOT NULL" {
		t.Fatalf("drop = %+v", drop)
	}

	if _, err := renderAlterNullability("oracle", e, &ir.Field{Name: "note"}, false); err == nil {
		t.Fatal("renderAlterNullability(oracle): want error, got nil")
	}
}
