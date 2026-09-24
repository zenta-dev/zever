package migrate

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

func renamed(s string) *string { return &s }

func TestPlan_unsupportedDialect(t *testing.T) {
	t.Parallel()

	conn := &fakeDB{dialect: "oracle"}

	if _, err := Plan(t.Context(), conn, &ir.Schema{}, PlanOptions{}); !errors.Is(err, ErrUnsupportedDialect) {
		t.Fatalf("Plan = %v, want ErrUnsupportedDialect", err)
	}
}

func TestMigrationPlan_Statements(t *testing.T) {
	t.Parallel()

	var nilPlan *MigrationPlan
	if got := nilPlan.Statements(); got != nil {
		t.Fatalf("nil Statements() = %v, want nil", got)
	}

	plan := &MigrationPlan{statements: []plannedStatement{{SQL: "a;"}, {SQL: "b;"}}}
	got := plan.Statements()

	if len(got) != 2 || got[0] != "a;" || got[1] != "b;" {
		t.Fatalf("Statements() = %v", got)
	}
}

func TestValidateIdent(t *testing.T) {
	t.Parallel()

	for _, ok := range []string{"users", "_x", "a1", "Camel_Case9"} {
		if err := validateIdent(ok); err != nil {
			t.Fatalf("validateIdent(%q): %v", ok, err)
		}
	}

	for _, bad := range []string{"", "0abc", "has space", "semi;colon", "quote\"q", "back`tick"} {
		if err := validateIdent(bad); err == nil {
			t.Fatalf("validateIdent(%q): want error, got nil", bad)
		}
	}
}

func TestPlan_bootstrapAndNoop(t *testing.T) {
	ctx := t.Context()
	conn := openTestSQLite(t, "plan.db")

	v1 := compileSchema(t, applyTestSchema)

	plan, err := Plan(ctx, conn, v1.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if plan.Dialect != atlas.DialectSQLite {
		t.Fatalf("Dialect = %q", plan.Dialect)
	}

	if len(plan.Statements()) == 0 {
		t.Fatal("bootstrap plan is empty")
	}

	if _, applyErr := Apply(ctx, conn, plan); applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}

	steady, err := Plan(ctx, conn, v1.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan steady: %v", err)
	}

	if len(steady.Statements()) != 0 {
		t.Fatalf("steady plan = %v, want empty", steady.Statements())
	}
}

func TestPlan_addAndDropColumn(t *testing.T) {
	ctx := t.Context()
	conn := openTestSQLite(t, "adddrop.db")

	v1 := compileSchema(t, applyTestSchema)

	plan, err := Plan(ctx, conn, v1.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if _, applyErr := Apply(ctx, conn, plan); applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}

	v2 := compileSchema(t, `entity User {
	id: uuid @primary
	email: string @unique
	note: string
}
`)

	added, err := Plan(ctx, conn, v2.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan v2: %v", err)
	}

	if len(added.Statements()) != 1 || !strings.Contains(added.Statements()[0], "ADD COLUMN") {
		t.Fatalf("add plan = %v, want one ADD COLUMN", added.Statements())
	}

	// Dropping without the opt-in flag plans nothing.
	dropped, err := Plan(ctx, conn, v1.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan v1: %v", err)
	}

	if _, applyErr := Apply(ctx, conn, added); applyErr != nil {
		t.Fatalf("Apply v2: %v", applyErr)
	}

	_ = dropped

	shrink, err := Plan(ctx, conn, v1.Schema, PlanOptions{DropColumns: true})
	if err != nil {
		t.Fatalf("Plan shrink: %v", err)
	}

	if len(shrink.Statements()) != 1 || !strings.Contains(shrink.Statements()[0], "DROP COLUMN") {
		t.Fatalf("shrink plan = %v, want one DROP COLUMN", shrink.Statements())
	}

	steadyDrop, err := Plan(ctx, conn, v1.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan no-drop: %v", err)
	}

	for _, s := range steadyDrop.Statements() {
		if strings.Contains(s, "DROP COLUMN") {
			t.Fatalf("plan without DropColumns contains a drop: %v", steadyDrop.Statements())
		}
	}
}

func TestTableExists(t *testing.T) {
	ctx := t.Context()
	conn := openTestSQLite(t, "exists.db")

	e := &ir.Entity{Name: "User"}

	exists, err := tableExists(ctx, conn, atlas.DialectSQLite, e)
	if err != nil || exists {
		t.Fatalf("tableExists = (%v, %v), want (false, nil)", exists, err)
	}

	v1 := compileSchema(t, applyTestSchema)

	plan, err := Plan(ctx, conn, v1.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if _, applyErr := Apply(ctx, conn, plan); applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}

	exists, err = tableExists(ctx, conn, atlas.DialectSQLite, e)
	if err != nil || !exists {
		t.Fatalf("tableExists = (%v, %v), want (true, nil)", exists, err)
	}

	if _, err := tableExists(ctx, conn, "oracle", e); err == nil {
		t.Fatal("tableExists(oracle): want error, got nil")
	}

	broken := &fakeDB{
		dialect: atlas.DialectSQLite,
		onQuery: func(_ string, _ []any) (db.Rows, error) {
			return nil, errors.New("boom")
		},
	}

	if _, err := tableExists(ctx, broken, atlas.DialectSQLite, e); err == nil {
		t.Fatal("tableExists: want query error, got nil")
	}
}

func TestTableExists_postgresAndMysql(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	e := &ir.Entity{Name: "User", Schema: "public"}

	pg := &fakeDB{
		dialect: atlas.DialectPostgres,
		onQuery: func(_ string, _ []any) (db.Rows, error) {
			return &fakeRows{vals: [][]any{{1}}}, nil
		},
	}

	if ok, err := tableExists(ctx, pg, atlas.DialectPostgres, e); err != nil || !ok {
		t.Fatalf("pg tableExists = (%v, %v)", ok, err)
	}

	my := &fakeDB{
		dialect: atlas.DialectMySQL,
		onQuery: func(_ string, _ []any) (db.Rows, error) {
			return &fakeRows{}, nil
		},
	}

	if ok, err := tableExists(ctx, my, atlas.DialectMySQL, e); err != nil || ok {
		t.Fatalf("mysql tableExists = (%v, %v)", ok, err)
	}
}

func TestIntrospectColumns(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	e := &ir.Entity{Name: "User"}

	t.Run("postgresFromState", func(t *testing.T) {
		t.Parallel()

		state := &liveSchemaState{columns: map[string][]liveColumn{
			"users": {{Name: "id", RawType: "uuid"}},
		}}

		cols, err := introspectColumns(ctx, &fakeDB{dialect: atlas.DialectPostgres}, atlas.DialectPostgres, e, state)
		if err != nil || len(cols) != 1 || cols[0].Name != "id" {
			t.Fatalf("cols = %+v, err = %v", cols, err)
		}
	})

	t.Run("mysql", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"id", "bigint", "NO", nil}}}, nil
			},
		}

		cols, err := introspectColumns(ctx, conn, atlas.DialectMySQL, e, &liveSchemaState{})
		if err != nil || len(cols) != 1 {
			t.Fatalf("cols = %+v, err = %v", cols, err)
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		t.Parallel()

		if _, err := introspectColumns(ctx, &fakeDB{dialect: "oracle"}, "oracle", e, &liveSchemaState{}); err == nil {
			t.Fatal("introspectColumns(oracle): want error, got nil")
		}
	})
}

func TestIntrospectPostgresColumns(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("grouped", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{
					{"users", "id", "uuid", "NO", nil},
					{"users", "email", "text", "YES", "def"},
				}}, nil
			},
		}

		cols, err := introspectPostgresColumns(ctx, conn, "public", []string{"users"})
		if err != nil {
			t.Fatalf("introspectPostgresColumns: %v", err)
		}

		got := cols["users"]
		if len(got) != 2 || got[0].Name != "id" || got[0].Nullable || !got[0].HasDefault == false {
			t.Fatalf("cols = %+v", got)
		}

		if !got[1].Nullable || !got[1].HasDefault {
			t.Fatalf("cols = %+v", got)
		}
	})

	t.Run("emptyTables", func(t *testing.T) {
		t.Parallel()

		cols, err := introspectPostgresColumns(ctx, &fakeDB{dialect: atlas.DialectPostgres}, "public", nil)
		if err != nil || len(cols) != 0 {
			t.Fatalf("cols = %+v, err = %v", cols, err)
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

		if _, err := introspectPostgresColumns(ctx, conn, "public", []string{"users"}); err == nil {
			t.Fatal("want error, got nil")
		}
	})

	t.Run("scanError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{vals: [][]any{{"u", "i", "t", "YES", nil}}, scanErr: errors.New("boom")}, nil
			},
		}

		if _, err := introspectPostgresColumns(ctx, conn, "public", []string{"users"}); err == nil {
			t.Fatal("want scan error, got nil")
		}
	})

	t.Run("iterateError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return &fakeRows{
					vals:    [][]any{{"u", "i", "t", "YES", nil}},
					iterErr: errors.New("boom"),
				}, nil
			},
		}

		if _, err := introspectPostgresColumns(ctx, conn, "public", []string{"users"}); err == nil {
			t.Fatal("want iterate error, got nil")
		}
	})
}

func TestIntrospectSQLiteColumns(t *testing.T) {
	ctx := t.Context()
	conn := openTestSQLite(t, "cols.db")

	v1 := compileSchema(t, applyTestSchema)

	plan, err := Plan(ctx, conn, v1.Schema, PlanOptions{})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if _, applyErr := Apply(ctx, conn, plan); applyErr != nil {
		t.Fatalf("Apply: %v", applyErr)
	}

	cols, err := introspectSQLiteColumns(ctx, conn, "users")
	if err != nil || len(cols) == 0 {
		t.Fatalf("cols = %+v, err = %v", cols, err)
	}

	empty, err := introspectSQLiteColumns(ctx, conn, "missing")
	if err != nil || len(empty) != 0 {
		t.Fatalf("missing cols = %+v, err = %v", empty, err)
	}

	if _, err := introspectSQLiteColumns(ctx, conn, "not valid!"); err == nil {
		t.Fatal("bad ident: want error, got nil")
	}
}

func TestRenderColumnStatements(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "User"}

	add, err := renderAddColumn(atlas.DialectPostgres, e, strField("note", ir.TString))
	if err != nil {
		t.Fatalf("renderAddColumn: %v", err)
	}

	if !strings.Contains(add.SQL, "IF NOT EXISTS") || add.Meta.Kind != kindAddColumn {
		t.Fatalf("add = %+v", add)
	}

	addLite, err := renderAddColumn(atlas.DialectSQLite, e, strField("note", ir.TString))
	if err != nil {
		t.Fatalf("renderAddColumn sqlite: %v", err)
	}

	if strings.Contains(addLite.SQL, "IF NOT EXISTS") {
		t.Fatalf("sqlite add carries IF NOT EXISTS: %q", addLite.SQL)
	}

	if _, renderErr := renderAddColumn("oracle", e, strField("note", ir.TString)); renderErr == nil {
		t.Fatal("renderAddColumn(oracle): want error, got nil")
	}

	drop, err := renderDropColumn(atlas.DialectPostgres, e, "note", "TEXT")
	if err != nil {
		t.Fatalf("renderDropColumn: %v", err)
	}

	if !strings.Contains(drop.SQL, "IF EXISTS") || drop.Meta.PriorType != "TEXT" {
		t.Fatalf("drop = %+v", drop)
	}

	if _, renderErr := renderDropColumn(atlas.DialectPostgres, e, "not valid!", "TEXT"); renderErr == nil {
		t.Fatal("renderDropColumn bad ident: want error, got nil")
	}

	rename, err := renderRenameColumn(atlas.DialectSQLite, e, "sent", "shipped")
	if err != nil {
		t.Fatalf("renderRenameColumn: %v", err)
	}

	if rename.Meta.Kind != kindRenameColumn || rename.Meta.PriorName != "sent" {
		t.Fatalf("rename = %+v", rename)
	}

	if _, err := renderRenameColumn(atlas.DialectSQLite, e, "bad!", "shipped"); err == nil {
		t.Fatal("renderRenameColumn bad old: want error, got nil")
	}

	if _, err := renderRenameColumn(atlas.DialectSQLite, e, "sent", "bad!"); err == nil {
		t.Fatal("renderRenameColumn bad new: want error, got nil")
	}
}

func TestRenameStatementsFor(t *testing.T) {
	t.Parallel()

	newEntity := func() *ir.Entity {
		return &ir.Entity{Name: "Order", Fields: []*ir.Field{
			{Name: "id"},
			{Name: "shipped", RenamedFrom: renamed("sent")},
		}}
	}

	t.Run("emitsRenameAndRewritesLive", func(t *testing.T) {
		t.Parallel()

		e := newEntity()
		live := map[string]liveColumn{"id": {Name: "id"}, "sent": {Name: "sent"}}
		w := &warnings{}

		stmts, err := renameStatementsFor(atlas.DialectSQLite, e, live, w)
		if err != nil || len(stmts) != 1 {
			t.Fatalf("stmts = %+v, err = %v", stmts, err)
		}

		if _, ok := live["sent"]; ok {
			t.Fatal("old name still live after rewrite")
		}

		if _, ok := live["shipped"]; !ok {
			t.Fatal("new name not live after rewrite")
		}
	})

	t.Run("skipsWhenOldNotLive", func(t *testing.T) {
		t.Parallel()

		e := newEntity()
		live := map[string]liveColumn{"id": {Name: "id"}, "shipped": {Name: "shipped"}}
		w := &warnings{}

		stmts, err := renameStatementsFor(atlas.DialectSQLite, e, live, w)
		if err != nil || len(stmts) != 0 {
			t.Fatalf("stmts = %+v, err = %v", stmts, err)
		}
	})

	t.Run("warnsWhenBothLive", func(t *testing.T) {
		t.Parallel()

		e := newEntity()
		live := map[string]liveColumn{"sent": {Name: "sent"}, "shipped": {Name: "shipped"}}
		w := &warnings{}

		stmts, err := renameStatementsFor(atlas.DialectSQLite, e, live, w)
		if err != nil || len(stmts) != 0 {
			t.Fatalf("stmts = %+v, err = %v", stmts, err)
		}

		if len(w.messages()) != 1 {
			t.Fatalf("warnings = %v, want 1", w.messages())
		}
	})

	t.Run("noRenames", func(t *testing.T) {
		t.Parallel()

		e := &ir.Entity{Name: "Order", Fields: []*ir.Field{{Name: "id"}}}
		w := &warnings{}

		stmts, err := renameStatementsFor(atlas.DialectSQLite, e, map[string]liveColumn{}, w)
		if err != nil || len(stmts) != 0 {
			t.Fatalf("stmts = %+v, err = %v", stmts, err)
		}
	})
}

func TestDropStatementsFor(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "User", Fields: []*ir.Field{{Name: "id"}}}
	live := map[string]liveColumn{
		"id":    {Name: "id"},
		"zeta":  {Name: "zeta", RawType: "TEXT"},
		"alpha": {Name: "alpha", RawType: "TEXT"},
	}

	stmts, err := dropStatementsFor(atlas.DialectSQLite, e, live)
	if err != nil {
		t.Fatalf("dropStatementsFor: %v", err)
	}

	if len(stmts) != 2 {
		t.Fatalf("stmts = %+v, want 2", stmts)
	}

	// Sorted by column name for stable plans.
	if !strings.Contains(stmts[0].SQL, `"alpha"`) || !strings.Contains(stmts[1].SQL, `"zeta"`) {
		t.Fatalf("stmts not sorted: %v", stmts)
	}
}

func TestLoadLiveSchemaState(t *testing.T) {
	t.Parallel()

	ctx := t.Context()

	t.Run("nonPostgresEmpty", func(t *testing.T) {
		t.Parallel()

		schema := compileSchema(t, applyTestSchema).Schema

		state, err := loadLiveSchemaState(ctx, &fakeDB{dialect: atlas.DialectSQLite}, atlas.DialectSQLite, schema)
		if err != nil {
			t.Fatalf("loadLiveSchemaState: %v", err)
		}

		if len(state.columns) != 0 {
			t.Fatalf("state = %+v, want empty", state)
		}
	})

	t.Run("postgresBatched", func(t *testing.T) {
		t.Parallel()

		schema := compileSchema(t, applyTestSchema).Schema

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(q string, _ []any) (db.Rows, error) {
				switch {
				case strings.Contains(q, "information_schema.columns"):
					return &fakeRows{vals: [][]any{{"users", "id", "uuid", "NO", nil}}}, nil
				case strings.Contains(q, "pg_class"):
					return &fakeRows{}, nil
				case strings.Contains(q, "table_constraints"):
					return &fakeRows{}, nil
				default:
					return nil, errors.New("unexpected query: " + q)
				}
			},
		}

		state, err := loadLiveSchemaState(ctx, conn, atlas.DialectPostgres, schema)
		if err != nil {
			t.Fatalf("loadLiveSchemaState: %v", err)
		}

		if len(state.columns["users"]) != 1 {
			t.Fatalf("state = %+v", state)
		}
	})

	t.Run("postgresError", func(t *testing.T) {
		t.Parallel()

		schema := compileSchema(t, applyTestSchema).Schema

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		if _, err := loadLiveSchemaState(ctx, conn, atlas.DialectPostgres, schema); err == nil {
			t.Fatal("loadLiveSchemaState: want error, got nil")
		}
	})
}

func TestTypeChangeStatementFor_sqliteWarns(t *testing.T) {
	ctx := t.Context()
	conn := openTestSQLite(t, "typewarn.db")

	if _, err := conn.Exec(ctx, `CREATE TABLE "widgets" ("id" TEXT NOT NULL, "size" TEXT);`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	v := compileSchema(t, `entity Widget {
	id: string @primary
	size: int64
}
`)

	plan, err := Plan(ctx, conn, v.Schema, PlanOptions{DetectTypeChanges: true})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	if len(plan.Statements()) != 0 {
		t.Fatalf("statements = %v, want none (sqlite warns and skips)", plan.Statements())
	}

	if len(plan.Warnings) == 0 {
		t.Fatal("want a type-change warning, got none")
	}
}

func TestPlan_computeError(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, applyTestSchema).Schema

	conn := &fakeDB{
		dialect: atlas.DialectPostgres,
		onQuery: func(_ string, _ []any) (db.Rows, error) {
			return nil, errors.New("boom")
		},
	}

	if _, err := Plan(t.Context(), conn, schema, PlanOptions{}); err == nil {
		t.Fatal("Plan: want introspection error, got nil")
	}
}

func TestPlan_renderSchemaError(t *testing.T) {
	t.Parallel()

	schema := &ir.Schema{Modules: []*ir.Module{{
		Name:     "m",
		Entities: []*ir.Entity{{Name: "Empty", Schema: "public"}},
	}}}

	conn := &fakeDB{
		dialect: atlas.DialectPostgres,
		onQuery: func(_ string, _ []any) (db.Rows, error) {
			return &fakeRows{}, nil
		},
	}

	if _, err := Plan(t.Context(), conn, schema, PlanOptions{}); err == nil {
		t.Fatal("Plan: want render error for fieldless entity, got nil")
	}
}

func TestPlan_postgresTypeChangeAndNullability(t *testing.T) {
	schema := compileSchema(t, applyTestSchema).Schema

	conn := &fakeDB{
		dialect: atlas.DialectPostgres,
		onQuery: func(q string, _ []any) (db.Rows, error) {
			switch {
			case strings.Contains(q, "information_schema.tables"):
				return &fakeRows{vals: [][]any{{1}}}, nil
			case strings.Contains(q, "information_schema.columns"):
				// email declared text NOT NULL, live integer NULL:
				// both a type change and a nullability change.
				return &fakeRows{vals: [][]any{
					{"users", "id", "uuid", "NO", nil},
					{"users", "email", "integer", "YES", nil},
				}}, nil
			case strings.Contains(q, "pg_class"):
				return &fakeRows{vals: [][]any{
					{"users", "users_email_key", "email", true},
				}}, nil
			case strings.Contains(q, "table_constraints"):
				return &fakeRows{}, nil
			default:
				return nil, errors.New("unexpected query: " + q)
			}
		},
	}

	plan, err := Plan(t.Context(), conn, schema, PlanOptions{DetectTypeChanges: true})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	var sawType, sawNull bool

	for _, s := range plan.Statements() {
		if strings.Contains(s, "TYPE") {
			sawType = true
		}

		if strings.Contains(s, "DROP NOT NULL") || strings.Contains(s, "SET NOT NULL") {
			sawNull = true
		}
	}

	if !sawType || !sawNull {
		t.Fatalf("plan = %v, want ALTER TYPE and DROP NOT NULL", plan.Statements())
	}
}

func TestPlan_mysqlTypeChangeAndDrop(t *testing.T) {
	schema := compileSchema(t, applyTestSchema).Schema

	conn := &fakeDB{
		dialect: atlas.DialectMySQL,
		onQuery: func(q string, _ []any) (db.Rows, error) {
			switch {
			case strings.Contains(q, "information_schema.tables"):
				return &fakeRows{vals: [][]any{{1}}}, nil
			case strings.Contains(q, "information_schema.columns"):
				return &fakeRows{vals: [][]any{
					{"id", "binary(16)", "NO", nil},
					{"email", "int", "NO", nil},
					{"legacy", "varchar(10)", "YES", nil},
				}}, nil
			case strings.Contains(q, "information_schema.statistics"):
				return &fakeRows{vals: [][]any{
					{"users_email_key", "email", 0},
				}}, nil
			case strings.Contains(q, "key_column_usage"):
				return &fakeRows{}, nil
			default:
				return nil, errors.New("unexpected query: " + q)
			}
		},
	}

	plan, err := Plan(t.Context(), conn, schema, PlanOptions{DetectTypeChanges: true, DropColumns: true})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	var sawModify, sawDrop bool

	for _, s := range plan.Statements() {
		if strings.Contains(s, "MODIFY COLUMN") {
			sawModify = true
		}

		if strings.Contains(s, "DROP COLUMN") {
			sawDrop = true
		}
	}

	if !sawModify || !sawDrop {
		t.Fatalf("plan = %v, want MODIFY COLUMN and DROP COLUMN", plan.Statements())
	}
}

func TestAlterStatementsFor_errors(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	schema := compileSchema(t, applyTestSchema).Schema

	var user *ir.Entity

	for _, m := range schema.Modules {
		for _, e := range m.Entities {
			if e.Name == "User" {
				user = e
			}
		}
	}

	t.Run("columnsError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(_ string, _ []any) (db.Rows, error) {
				return nil, errors.New("boom")
			},
		}

		if _, err := alterStatementsFor(ctx, conn, atlas.DialectMySQL, user, PlanOptions{}, &liveSchemaState{}, &warnings{}); err == nil {
			t.Fatal("want columns error, got nil")
		}
	})

	t.Run("indexesError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(q string, _ []any) (db.Rows, error) {
				if strings.Contains(q, "information_schema.statistics") {
					return nil, errors.New("boom")
				}

				if strings.Contains(q, "information_schema.columns") {
					return &fakeRows{vals: [][]any{
						{"id", "binary(16)", "NO", nil},
						{"email", "text", "YES", nil},
					}}, nil
				}

				return &fakeRows{}, nil
			},
		}

		if _, err := alterStatementsFor(ctx, conn, atlas.DialectMySQL, user, PlanOptions{}, &liveSchemaState{}, &warnings{}); err == nil {
			t.Fatal("want indexes error, got nil")
		}
	})

	t.Run("fkError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(q string, _ []any) (db.Rows, error) {
				switch {
				case strings.Contains(q, "key_column_usage"):
					return nil, errors.New("boom")
				case strings.Contains(q, "information_schema.statistics"):
					return &fakeRows{}, nil
				default:
					return &fakeRows{vals: [][]any{
						{"id", "binary(16)", "NO", nil},
						{"email", "text", "YES", nil},
					}}, nil
				}
			},
		}

		if _, err := alterStatementsFor(ctx, conn, atlas.DialectMySQL, user, PlanOptions{}, &liveSchemaState{}, &warnings{}); err == nil {
			t.Fatal("want fk error, got nil")
		}
	})

	t.Run("dropErrorOnBadLiveIdent", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectMySQL,
			onQuery: func(q string, _ []any) (db.Rows, error) {
				switch {
				case strings.Contains(q, "information_schema.statistics"),
					strings.Contains(q, "key_column_usage"):
					return &fakeRows{}, nil
				default:
					return &fakeRows{vals: [][]any{
						{"id", "binary(16)", "NO", nil},
						{"email", "text", "YES", nil},
						{"bad col!", "text", "YES", nil},
					}}, nil
				}
			},
		}

		opts := PlanOptions{DropColumns: true}

		if _, err := alterStatementsFor(ctx, conn, atlas.DialectMySQL, user, opts, &liveSchemaState{}, &warnings{}); err == nil {
			t.Fatal("want drop error, got nil")
		}
	})

	t.Run("renameErrorOnBadLiveIdent", func(t *testing.T) {
		t.Parallel()

		entity := &ir.Entity{Name: "Thing", Fields: []*ir.Field{
			{Name: "id"},
			{Name: "shipped", RenamedFrom: renamed("bad!")},
		}}

		conn := &fakeDB{
			dialect: atlas.DialectSQLite,
			onQuery: func(q string, _ []any) (db.Rows, error) {
				if strings.Contains(q, "PRAGMA table_info") {
					return &fakeRows{vals: [][]any{
						{0, "id", "TEXT", 1, nil, 0},
						{1, "bad!", "TEXT", 0, nil, 0},
					}}, nil
				}

				return &fakeRows{}, nil
			},
		}

		if _, err := alterStatementsFor(ctx, conn, atlas.DialectSQLite, entity, PlanOptions{}, &liveSchemaState{}, &warnings{}); err == nil {
			t.Fatal("want rename error, got nil")
		}
	})
}

func TestPlan_tableExistsError(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, applyTestSchema).Schema

	conn := &fakeDB{
		dialect: atlas.DialectPostgres,
		onQuery: func(q string, _ []any) (db.Rows, error) {
			// Prefetch succeeds; the per-table existence check fails.
			if strings.Contains(q, "information_schema.tables") {
				return nil, errors.New("boom")
			}

			return &fakeRows{}, nil
		},
	}

	if _, err := Plan(t.Context(), conn, schema, PlanOptions{}); err == nil {
		t.Fatal("Plan: want table-exists error, got nil")
	}
}

func TestPlan_mysqlIndexesError(t *testing.T) {
	t.Parallel()

	schema := compileSchema(t, applyTestSchema).Schema

	conn := &fakeDB{
		dialect: atlas.DialectMySQL,
		onQuery: func(q string, _ []any) (db.Rows, error) {
			switch {
			case strings.Contains(q, "information_schema.tables"):
				return &fakeRows{vals: [][]any{{1}}}, nil
			case strings.Contains(q, "information_schema.columns"):
				return &fakeRows{vals: [][]any{
					{"id", "binary(16)", "NO", nil},
					{"email", "text", "YES", nil},
				}}, nil
			case strings.Contains(q, "information_schema.statistics"):
				return nil, errors.New("boom")
			default:
				return &fakeRows{}, nil
			}
		},
	}

	if _, err := Plan(t.Context(), conn, schema, PlanOptions{}); err == nil {
		t.Fatal("Plan: want indexes error, got nil")
	}
}

func TestRenderColumnOracleErrors(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "User"}

	if _, err := renderDropColumn("oracle", e, "note", "TEXT"); err == nil {
		t.Fatal("renderDropColumn(oracle): want error, got nil")
	}

	if _, err := renderRenameColumn("oracle", e, "a", "b"); err == nil {
		t.Fatal("renderRenameColumn(oracle): want error, got nil")
	}
}

func TestTableExists_badIdent(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "0bad"}

	if _, err := tableExists(t.Context(), &fakeDB{dialect: atlas.DialectSQLite}, atlas.DialectSQLite, e); err == nil {
		t.Fatal("tableExists bad ident: want error, got nil")
	}
}

func TestIntrospectSQLiteColumns_scanError(t *testing.T) {
	t.Parallel()

	conn := &fakeDB{
		dialect: atlas.DialectSQLite,
		onQuery: func(_ string, _ []any) (db.Rows, error) {
			return &fakeRows{vals: [][]any{{0, "id", "TEXT", 1, nil, 0}}, scanErr: errors.New("boom")}, nil
		},
	}

	if _, err := introspectSQLiteColumns(t.Context(), conn, "users"); err == nil {
		t.Fatal("want scan error, got nil")
	}
}

func TestLoadLiveSchemaState_indexAndFkError(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	schema := compileSchema(t, applyTestSchema).Schema

	t.Run("indexError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(q string, _ []any) (db.Rows, error) {
				if strings.Contains(q, "pg_class") {
					return nil, errors.New("boom")
				}

				return &fakeRows{}, nil
			},
		}

		if _, err := loadLiveSchemaState(ctx, conn, atlas.DialectPostgres, schema); err == nil {
			t.Fatal("want index error, got nil")
		}
	})

	t.Run("fkError", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(q string, _ []any) (db.Rows, error) {
				if strings.Contains(q, "table_constraints") {
					return nil, errors.New("boom")
				}

				return &fakeRows{}, nil
			},
		}

		if _, err := loadLiveSchemaState(ctx, conn, atlas.DialectPostgres, schema); err == nil {
			t.Fatal("want fk error, got nil")
		}
	})

	t.Run("populatesAll", func(t *testing.T) {
		t.Parallel()

		conn := &fakeDB{
			dialect: atlas.DialectPostgres,
			onQuery: func(q string, _ []any) (db.Rows, error) {
				switch {
				case strings.Contains(q, "information_schema.columns"):
					return &fakeRows{vals: [][]any{{"users", "id", "uuid", "NO", nil}}}, nil
				case strings.Contains(q, "pg_class"):
					return &fakeRows{vals: [][]any{{"users", "users_email_key", "email", true}}}, nil
				case strings.Contains(q, "table_constraints"):
					return &fakeRows{vals: [][]any{{"orders", "user_id", "users", "id"}}}, nil
				default:
					return nil, errors.New("unexpected query: " + q)
				}
			},
		}

		state, err := loadLiveSchemaState(ctx, conn, atlas.DialectPostgres, schema)
		if err != nil {
			t.Fatalf("loadLiveSchemaState: %v", err)
		}

		if len(state.indexes["users"]) != 1 || len(state.foreignKeys["orders"]) != 1 {
			t.Fatalf("state = %+v", state)
		}
	})
}

func TestWarnings(t *testing.T) {
	t.Parallel()

	w := &warnings{}
	if got := w.messages(); len(got) != 0 {
		t.Fatalf("messages = %v, want empty", got)
	}

	w.add("hello %s", "world")

	got := w.messages()
	if len(got) != 1 || got[0] != "hello world" {
		t.Fatalf("messages = %v", got)
	}
}

func TestStatementTexts(t *testing.T) {
	t.Parallel()

	got := statementTexts([]plannedStatement{{SQL: "a;"}, {SQL: "b;"}})
	if len(got) != 2 || got[1] != "b;" {
		t.Fatalf("statementTexts = %v", got)
	}

	if got := statementTexts(nil); len(got) != 0 {
		t.Fatalf("statementTexts(nil) = %v", got)
	}
}
