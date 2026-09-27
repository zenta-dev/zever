// Coverage tests for the atlas backend's exported model helpers, dialect
// column-type branches, enum DDL/HCL paths, and dependency-ordering edges
// that the main golden tests do not exercise.
//
// Reachability notes (all branches below are covered; none are left silent):
//   - depsSatisfied's unknown-dependency skip is defensive: the resolver
//     forbids cross-module relations, so every dependency orderEntities
//     records is module-local and therefore known. It is covered by a direct
//     unit call, not via RenderSchemaDDL.
//   - The trailing "return text" defaults in postgresColumnType,
//     sqliteColumnType, mysqlColumnType, and atlasColumnType only fire for
//     ScalarType values outside the ir vocabulary. They are covered by direct
//     tables with an out-of-range ScalarType value.
package atlas

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

// TestSchemaOf_table covers the explicit-schema and empty (public default)
// branches, through both the unexported and exported entry points.
func TestSchemaOf_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		schema string
		want   string
	}{
		{name: "explicit", schema: "billing", want: "billing"},
		{name: "empty defaults to public", schema: "", want: "public"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := &ir.Entity{Name: "Widget", Schema: tt.schema}
			if got := schemaOf(e); got != tt.want {
				t.Fatalf("schemaOf(%q) = %q, want %q", tt.schema, got, tt.want)
			}

			if got := SchemaOf(e); got != tt.want {
				t.Fatalf("SchemaOf(%q) = %q, want %q", tt.schema, got, tt.want)
			}
		})
	}
}

// TestQuoteIdent_table proves double-quoting and embedded-quote escaping,
// through both the unexported and exported entry points.
func TestQuoteIdent_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		ident string
		want  string
	}{
		{name: "plain", ident: "users", want: `"users"`},
		{name: "embedded quote", ident: `we"ird`, want: `"we""ird"`},
		{name: "empty", ident: "", want: `""`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := quoteIdent(tt.ident); got != tt.want {
				t.Fatalf("quoteIdent(%q) = %q, want %q", tt.ident, got, tt.want)
			}

			if got := QuoteIdent(tt.ident); got != tt.want {
				t.Fatalf("QuoteIdent(%q) = %q, want %q", tt.ident, got, tt.want)
			}
		})
	}
}

// TestMaxLen_table covers the kind filter, the int64/positive guards, and
// the happy path.
func TestMaxLen_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		field     *ir.Field
		wantLen   int64
		wantFound bool
	}{
		{
			name:      "no validations",
			field:     &ir.Field{Name: "bio"},
			wantFound: false,
		},
		{
			name: "other kind first",
			field: &ir.Field{Name: "bio", Validate: []ir.Validation{
				{Kind: "min_len", Args: map[string]any{"value": int64(1)}},
				{Kind: "max_len", Args: map[string]any{"value": int64(64)}},
			}},
			wantLen:   64,
			wantFound: true,
		},
		{
			name: "non-int64 value",
			field: &ir.Field{Name: "bio", Validate: []ir.Validation{
				{Kind: "max_len", Args: map[string]any{"value": "many"}},
			}},
			wantFound: false,
		},
		{
			name: "zero bound",
			field: &ir.Field{Name: "bio", Validate: []ir.Validation{
				{Kind: "max_len", Args: map[string]any{"value": int64(0)}},
			}},
			wantFound: false,
		},
		{
			name: "negative bound",
			field: &ir.Field{Name: "bio", Validate: []ir.Validation{
				{Kind: "max_len", Args: map[string]any{"value": int64(-5)}},
			}},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotLen, gotFound := maxLen(tt.field)
			if gotLen != tt.wantLen || gotFound != tt.wantFound {
				t.Fatalf("maxLen(%v) = (%d, %t), want (%d, %t)",
					tt.field.Validate, gotLen, gotFound, tt.wantLen, tt.wantFound)
			}
		})
	}
}

// TestOnDeleteSQL_table covers every DSL on-delete value plus the empty and
// unknown fallthroughs.
func TestOnDeleteSQL_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value string
		want  string
	}{
		{value: "restrict", want: "RESTRICT"},
		{value: "cascade", want: "CASCADE"},
		{value: "set_null", want: "SET NULL"},
		{value: "", want: ""},
		{value: "bogus", want: ""},
	}

	for _, tt := range tests {
		t.Run("value="+tt.value, func(t *testing.T) {
			t.Parallel()

			if got := onDeleteSQL(tt.value); got != tt.want {
				t.Fatalf("onDeleteSQL(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

// TestRelationForeignKey_branches covers every derivation outcome: nil
// target, nil foreign key, each relation kind, a target without a primary
// key, and the belongs_to/has_one happy paths.
func TestRelationForeignKey_branches(t *testing.T) {
	t.Parallel()

	pk := &ir.Field{Name: "id", Primary: true}
	fkCol := &ir.Field{Name: "user_id"}

	newEntity := func(name string, withPK bool) *ir.Entity {
		e := &ir.Entity{Name: name, Schema: "public"}
		if withPK {
			e.Fields = []*ir.Field{pk}
		}

		return e
	}

	t.Run("nil target", func(t *testing.T) {
		t.Parallel()

		e := newEntity("Order", true)
		if _, ok := relationForeignKey(e, &ir.Relation{Kind: ir.BelongsTo, ForeignKey: fkCol}); ok {
			t.Fatal("relationForeignKey with nil target should report false")
		}
	})

	t.Run("nil foreign key", func(t *testing.T) {
		t.Parallel()

		e := newEntity("Order", true)
		target := newEntity("User", true)

		if _, ok := relationForeignKey(e, &ir.Relation{Kind: ir.BelongsTo, Target: target}); ok {
			t.Fatal("relationForeignKey with nil foreign key should report false")
		}
	})

	t.Run("has_many", func(t *testing.T) {
		t.Parallel()

		e := newEntity("User", true)
		target := newEntity("Order", true)

		if _, ok := relationForeignKey(e, &ir.Relation{
			Kind: ir.HasMany, Target: target, ForeignKey: fkCol,
		}); ok {
			t.Fatal("relationForeignKey with has_many should report false")
		}
	})

	t.Run("many_to_many", func(t *testing.T) {
		t.Parallel()

		e := newEntity("User", true)
		target := newEntity("Tag", true)

		if _, ok := relationForeignKey(e, &ir.Relation{
			Kind: ir.ManyToMany, Target: target, ForeignKey: fkCol,
		}); ok {
			t.Fatal("relationForeignKey with many_to_many should report false")
		}
	})

	t.Run("unknown kind", func(t *testing.T) {
		t.Parallel()

		e := newEntity("User", true)
		target := newEntity("Tag", true)

		if _, ok := relationForeignKey(e, &ir.Relation{
			Kind: ir.RelationKind(99), Target: target, ForeignKey: fkCol,
		}); ok {
			t.Fatal("relationForeignKey with unknown kind should report false")
		}
	})

	t.Run("target without primary key", func(t *testing.T) {
		t.Parallel()

		e := newEntity("Order", true)
		target := newEntity("User", false)

		if _, ok := relationForeignKey(e, &ir.Relation{
			Kind: ir.BelongsTo, Target: target, ForeignKey: fkCol, OnDelete: "cascade",
		}); ok {
			t.Fatal("relationForeignKey with PK-less target should report false")
		}
	})

	t.Run("belongs_to", func(t *testing.T) {
		t.Parallel()

		e := newEntity("Order", true)
		target := newEntity("User", true)

		fk, ok := relationForeignKey(e, &ir.Relation{
			Kind: ir.BelongsTo, Target: target, ForeignKey: fkCol, OnDelete: "restrict",
		})
		if !ok {
			t.Fatal("relationForeignKey with belongs_to should report true")
		}

		if fk.Table != "orders" || fk.Column != "user_id" || fk.RefTable != "users" ||
			fk.RefColumn != "id" || fk.RefSchema != "public" || fk.OnDelete != "restrict" {
			t.Fatalf("belongs_to foreignKey = %+v, unexpected attribution", fk)
		}

		if fk.Name != "orders_user_id_fkey" {
			t.Fatalf("belongs_to foreignKey name = %q, want orders_user_id_fkey", fk.Name)
		}
	})

	t.Run("has_one lands on target", func(t *testing.T) {
		t.Parallel()

		e := newEntity("User", true)
		target := newEntity("Profile", true)

		fk, ok := relationForeignKey(e, &ir.Relation{
			Kind: ir.HasOne, Target: target, ForeignKey: &ir.Field{Name: "user_id"},
		})
		if !ok {
			t.Fatal("relationForeignKey with has_one should report true")
		}

		if fk.Table != "profiles" || fk.RefTable != "users" {
			t.Fatalf("has_one foreignKey = %+v, want holder profiles referencing users", fk)
		}
	})
}

// TestCollectForeignKeys_dedup proves a belongs_to/has_one pair describing
// the same column collapses into one constraint, and that has_many is
// skipped.
func TestCollectForeignKeys_dedup(t *testing.T) {
	t.Parallel()

	pk := &ir.Field{Name: "id", Primary: true}
	userID := &ir.Field{Name: "user_id"}

	user := &ir.Entity{Name: "User", Schema: "public", Fields: []*ir.Field{pk}}
	order := &ir.Entity{
		Name:   "Order",
		Schema: "public",
		Fields: []*ir.Field{pk, userID},
	}
	profile := &ir.Entity{
		Name:   "Profile",
		Schema: "public",
		Fields: []*ir.Field{pk, {Name: "owner_id"}},
	}

	mod := &ir.Module{}
	user.Module, order.Module, profile.Module = mod, mod, mod
	user.Relations = []*ir.Relation{
		{Kind: ir.HasOne, Target: order, ForeignKey: userID, OnDelete: "restrict"},
		{Kind: ir.HasMany, Target: profile, ForeignKey: profile.Fields[1]},
	}
	order.Relations = []*ir.Relation{
		{Kind: ir.BelongsTo, Target: user, ForeignKey: userID, OnDelete: "cascade"},
	}
	mod.Entities = []*ir.Entity{user, order, profile}

	fks := collectForeignKeys(mod)

	if len(fks["orders"]) != 1 {
		t.Fatalf("expected 1 deduped FK on orders, got %v", fks["orders"])
	}

	// User's relations are walked first, so the has_one attribution wins
	// the dedup; the point is the pair collapses into one constraint.
	if fks["orders"][0].OnDelete != "restrict" {
		t.Fatalf("deduped FK = %+v, want the first-walked attribution", fks["orders"][0])
	}

	if len(fks) != 1 {
		t.Fatalf("has_many must not produce a constraint, got %v", fks)
	}
}

// TestColumnType_table proves the exported ColumnType mirrors the DDL
// renderer's per-dialect mapping and rejects unknown dialects.
func TestColumnType_table(t *testing.T) {
	t.Parallel()

	strField := func() *ir.Field { return &ir.Field{Name: "nickname"} }
	bounded := &ir.Field{Name: "nickname", Type: ir.FieldType{Scalar: ir.TString}, Validate: []ir.Validation{
		{Kind: "max_len", Args: map[string]any{"value": int64(32)}},
	}}

	tests := []struct {
		name    string
		dialect string
		field   *ir.Field
		want    string
		wantErr bool
	}{
		{name: "postgres uuid", dialect: DialectPostgres, field: &ir.Field{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}}, want: "uuid"},
		{name: "postgres bounded", dialect: DialectPostgres, field: bounded, want: "varchar(32)"},
		{name: "sqlite json", dialect: DialectSQLite, field: &ir.Field{Name: "m", Type: ir.FieldType{Scalar: ir.TJSON}}, want: "TEXT"},
		{name: "mysql bool", dialect: DialectMySQL, field: &ir.Field{Name: "a", Type: ir.FieldType{Scalar: ir.TBool}}, want: "BOOLEAN"},
		{name: "unknown dialect", dialect: "oracle", field: strField(), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ColumnType(tt.dialect, tt.field)
			if tt.wantErr {
				if err == nil {
					t.Fatal("ColumnType: expected an unknown-dialect error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("ColumnType: %v", err)
			}

			if got != tt.want {
				t.Fatalf("ColumnType(%q, %v) = %q, want %q", tt.dialect, tt.field.Type, got, tt.want)
			}
		})
	}
}

// TestQualifiedTableName_table proves postgres qualification, bare names
// elsewhere, and unknown-dialect rejection.
func TestQualifiedTableName_table(t *testing.T) {
	t.Parallel()

	entity := &ir.Entity{Name: "Order", Schema: "billing"}

	tests := []struct {
		name    string
		dialect string
		want    string
		wantErr bool
	}{
		{name: "postgres", dialect: DialectPostgres, want: `"billing"."orders"`},
		{name: "sqlite", dialect: DialectSQLite, want: `"orders"`},
		{name: "mysql", dialect: DialectMySQL, want: "`orders`"},
		{name: "unknown", dialect: "oracle", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := QualifiedTableName(tt.dialect, entity)
			if tt.wantErr {
				if err == nil {
					t.Fatal("QualifiedTableName: expected an unknown-dialect error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("QualifiedTableName: %v", err)
			}

			if got != tt.want {
				t.Fatalf("QualifiedTableName(%q) = %q, want %q", tt.dialect, got, tt.want)
			}
		})
	}
}

// TestDeclaredIndexes_table proves the unique suffix, the plain suffix, and
// the skip of blocks whose columns failed to resolve.
func TestDeclaredIndexes_table(t *testing.T) {
	t.Parallel()

	userID := &ir.Field{Name: "user_id"}
	status := &ir.Field{Name: "status"}

	entity := &ir.Entity{
		Name:   "Order",
		Schema: "public",
		Indexes: []*ir.Index{
			{Columns: []*ir.Field{userID}},
			{Columns: []*ir.Field{userID, status}, Unique: true},
			{Columns: []*ir.Field{nil}},
		},
	}

	specs := DeclaredIndexes(entity)
	if len(specs) != 2 {
		t.Fatalf("DeclaredIndexes = %v, want 2 specs (unresolved block skipped)", specs)
	}

	if specs[0].Name != "orders_user_id_idx" || specs[0].Unique {
		t.Fatalf("first spec = %+v, want plain idx", specs[0])
	}

	if specs[1].Name != "orders_user_id_status_key" || !specs[1].Unique {
		t.Fatalf("second spec = %+v, want unique key", specs[1])
	}
}

// TestEntityForeignKeys_table proves the nil guards and the happy path that
// reuses the module's belongs_to derivation.
func TestEntityForeignKeys_table(t *testing.T) {
	t.Parallel()

	if got := EntityForeignKeys(nil); got != nil {
		t.Fatalf("EntityForeignKeys(nil) = %v, want nil", got)
	}

	orphan := &ir.Entity{Name: "Order", Schema: "public"}
	if got := EntityForeignKeys(orphan); got != nil {
		t.Fatalf("EntityForeignKeys without module = %v, want nil", got)
	}

	schema := compileSchema(t, `entity User {
		id: uuid @primary
	}

	entity Order {
		id: uuid @primary
		user_id: uuid

		belongs_to user: User @foreign_key(user_id) @on_delete(cascade)
	}`)

	var order *ir.Entity

	for _, e := range schema.Modules[0].Entities {
		if e.Name == "Order" {
			order = e
		}
	}

	if order == nil {
		t.Fatal("test schema missing Order entity")
	}

	fks := EntityForeignKeys(order)
	if len(fks) != 1 {
		t.Fatalf("EntityForeignKeys(Order) = %v, want 1 FK", fks)
	}

	if fks[0].Name != "orders_user_id_fkey" || fks[0].RefTable != "users" {
		t.Fatalf("EntityForeignKeys(Order) = %+v, unexpected FK", fks)
	}

	var user *ir.Entity

	for _, e := range schema.Modules[0].Entities {
		if e.Name == "User" {
			user = e
		}
	}

	if got := EntityForeignKeys(user); len(got) != 0 {
		t.Fatalf("EntityForeignKeys(User) = %v, want no held FKs", got)
	}
}

// TestNullableColumns_table proves only set_null foreign keys force
// nullability.
func TestNullableColumns_table(t *testing.T) {
	t.Parallel()

	got := NullableColumns([]ForeignKey{
		{Column: "assignee_id", OnDelete: "set_null"},
		{Column: "user_id", OnDelete: "cascade"},
		{Column: "plain_id"},
	})

	if len(got) != 1 || !got["assignee_id"] {
		t.Fatalf("NullableColumns = %v, want only assignee_id", got)
	}

	if got := NullableColumns(nil); len(got) != 0 {
		t.Fatalf("NullableColumns(nil) = %v, want empty", got)
	}
}

// TestGenerateNamedEnum covers the shared-enum HCL block and the postgres
// CREATE TYPE bootstrap statement, plus the per-dialect column types for a
// named enum field.
func TestGenerateNamedEnum(t *testing.T) {
	t.Parallel()

	src := `enum Status { active, inactive }

	entity Task {
		id: uuid @primary
		status: Status
		inline: enum(x, y)
		name: string @validate(max_len: 32)
	}`

	schema := compileSchema(t, src)
	content := generateRoot(t, src)

	checkGolden(t, "named_enum", content)

	rendered := string(content)
	for _, want := range []string{
		`enum "status" {`,
		`values = ["active", "inactive"]`,
		`type = enum.status`,
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("generated HCL missing %q:\n%s", want, rendered)
		}
	}

	stmts, err := RenderSchemaDDL(DialectPostgres, schema)
	if err != nil {
		t.Fatalf("RenderSchemaDDL: %v", err)
	}

	joined := strings.Join(stmts, "\n")

	for _, want := range []string{
		`CREATE TYPE "status" AS ENUM ('active', 'inactive');`,
		`"status" "status" NOT NULL`,
		`"inline" text NOT NULL`,
		`"name" varchar(32) NOT NULL`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("postgres DDL missing %q:\n%s", want, joined)
		}
	}

	sqliteStmt, err := RenderCreateTableSQL(DialectSQLite, schema.Modules[0].Entities[0])
	if err != nil {
		t.Fatalf("RenderCreateTableSQL (sqlite): %v", err)
	}

	if !strings.Contains(sqliteStmt, `"status" TEXT NOT NULL`) {
		t.Fatalf("sqlite named enum should stay TEXT:\n%s", sqliteStmt)
	}

	mysqlStmt, err := RenderCreateTableSQL(DialectMySQL, schema.Modules[0].Entities[0])
	if err != nil {
		t.Fatalf("RenderCreateTableSQL (mysql): %v", err)
	}

	if !strings.Contains(mysqlStmt, "`status` ENUM('active', 'inactive') NOT NULL") {
		t.Fatalf("mysql named enum should inline ENUM values:\n%s", mysqlStmt)
	}
}

// TestPostgresColumnType_table maps every scalar, both string widths, both
// enum shapes, and an out-of-range scalar.
func TestPostgresColumnType_table(t *testing.T) {
	t.Parallel()

	scalarField := func(s ir.ScalarType) *ir.Field {
		return &ir.Field{Name: "c", Type: ir.FieldType{Scalar: s}}
	}

	tests := []struct {
		name  string
		field *ir.Field
		want  string
	}{
		{name: "uuid", field: scalarField(ir.TUUID), want: "uuid"},
		{name: "text", field: scalarField(ir.TString), want: "text"},
		{
			name: "varchar",
			field: &ir.Field{Name: "c", Type: ir.FieldType{Scalar: ir.TString},
				Validate: []ir.Validation{{Kind: "max_len", Args: map[string]any{"value": int64(16)}}}},
			want: "varchar(16)",
		},
		{name: "int32", field: scalarField(ir.TInt32), want: "integer"},
		{name: "int64", field: scalarField(ir.TInt64), want: "bigint"},
		{name: "float32", field: scalarField(ir.TFloat32), want: "real"},
		{name: "float64", field: scalarField(ir.TFloat64), want: "double precision"},
		{name: "bool", field: scalarField(ir.TBool), want: "boolean"},
		{name: "timestamp", field: scalarField(ir.TTimestamp), want: "timestamptz"},
		{name: "date", field: scalarField(ir.TDate), want: "date"},
		{name: "bytes", field: scalarField(ir.TBytes), want: "bytea"},
		{name: "json", field: scalarField(ir.TJSON), want: "jsonb"},
		{
			name:  "named enum",
			field: &ir.Field{Name: "c", Type: ir.FieldType{Scalar: ir.TEnum, EnumName: "TodoStatus", EnumValues: []string{"a"}}},
			want:  `"todo_status"`,
		},
		{
			name:  "inline enum",
			field: &ir.Field{Name: "c", Type: ir.FieldType{Scalar: ir.TEnum, EnumValues: []string{"a"}}},
			want:  "text",
		},
		{name: "invalid", field: scalarField(ir.ScalarType(99)), want: "text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := postgresColumnType(tt.field); got != tt.want {
				t.Fatalf("postgresColumnType(%v) = %q, want %q", tt.field.Type, got, tt.want)
			}
		})
	}
}

// TestSQLiteColumnType_table covers the max_len width branch, the plain
// branch, and the out-of-range default.
func TestSQLiteColumnType_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field *ir.Field
		want  string
	}{
		{name: "text", field: &ir.Field{Name: "c", Type: ir.FieldType{Scalar: ir.TString}}, want: "TEXT"},
		{
			name: "varchar",
			field: &ir.Field{Name: "c", Type: ir.FieldType{Scalar: ir.TString},
				Validate: []ir.Validation{{Kind: "max_len", Args: map[string]any{"value": int64(16)}}}},
			want: "VARCHAR(16)",
		},
		{name: "bool", field: &ir.Field{Name: "c", Type: ir.FieldType{Scalar: ir.TBool}}, want: "INTEGER"},
		{name: "invalid", field: &ir.Field{Name: "c", Type: ir.FieldType{Scalar: ir.ScalarType(99)}}, want: "TEXT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := sqliteColumnType(tt.field); got != tt.want {
				t.Fatalf("sqliteColumnType(%v) = %q, want %q", tt.field.Type, got, tt.want)
			}
		})
	}
}

// TestMySQLColumnType_table covers the int32 primary AUTO_INCREMENT branch
// and the out-of-range default; the remaining rows are locked by the MySQL
// golden tests in render_ddl_test.go.
func TestMySQLColumnType_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field *ir.Field
		want  string
	}{
		{
			name:  "int32 primary",
			field: &ir.Field{Name: "id", Primary: true, Type: ir.FieldType{Scalar: ir.TInt32}},
			want:  "INT AUTO_INCREMENT",
		},
		{
			name:  "int32 plain",
			field: &ir.Field{Name: "n", Type: ir.FieldType{Scalar: ir.TInt32}},
			want:  "INT",
		},
		{name: "invalid", field: &ir.Field{Name: "c", Type: ir.FieldType{Scalar: ir.ScalarType(99)}}, want: "TEXT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := mysqlColumnType(tt.field); got != tt.want {
				t.Fatalf("mysqlColumnType(%v) = %q, want %q", tt.field.Type, got, tt.want)
			}
		})
	}
}

// TestAtlasColumnType_table covers the named-enum reference, the inline-enum
// fallback, and the out-of-range default.
func TestAtlasColumnType_table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field *ir.Field
		want  string
	}{
		{name: "uuid", field: &ir.Field{Name: "c", Type: ir.FieldType{Scalar: ir.TUUID}}, want: "uuid"},
		{
			name:  "named enum",
			field: &ir.Field{Name: "c", Type: ir.FieldType{Scalar: ir.TEnum, EnumName: "TodoStatus", EnumValues: []string{"a"}}},
			want:  "enum.todo_status",
		},
		{
			name:  "inline enum",
			field: &ir.Field{Name: "c", Type: ir.FieldType{Scalar: ir.TEnum, EnumValues: []string{"a"}}},
			want:  "text",
		},
		{name: "invalid", field: &ir.Field{Name: "c", Type: ir.FieldType{Scalar: ir.ScalarType(99)}}, want: "text"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := atlasColumnType(tt.field); got != tt.want {
				t.Fatalf("atlasColumnType(%v) = %q, want %q", tt.field.Type, got, tt.want)
			}
		})
	}
}

// TestRenderSchemaDDLUniqueIndex proves a unique index(...) block renders as
// CREATE UNIQUE INDEX, alongside the plain form.
func TestRenderSchemaDDLUniqueIndex(t *testing.T) {
	t.Parallel()

	src := `entity Order {
		id: uuid @primary
		user_id: uuid
		status: enum(pending, paid)

		index(user_id)
		index(user_id, status) @unique
	}`

	stmts, err := RenderSchemaDDL(DialectPostgres, compileSchema(t, src))
	if err != nil {
		t.Fatalf("RenderSchemaDDL: %v", err)
	}

	joined := strings.Join(stmts, "\n")

	for _, want := range []string{
		`CREATE INDEX IF NOT EXISTS "orders_user_id_idx" ON "public"."orders" ("user_id");`,
		`CREATE UNIQUE INDEX IF NOT EXISTS "orders_user_id_status_key" ON "public"."orders" ("user_id", "status");`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("postgres DDL missing %q:\n%s", want, joined)
		}
	}
}

// TestRenderCreateTableSQLNonBelongsToRelations proves has_many is ignored
// and a declared has_one (whose constraint lands on the target table) is
// filtered out of the declaring entity's own statement.
func TestRenderCreateTableSQLNonBelongsToRelations(t *testing.T) {
	t.Parallel()

	src := `entity User {
		id: uuid @primary

		has_one profile: Profile @foreign_key(user_id)
		has_many orders: Order @foreign_key(user_id)
	}

	entity Profile {
		id: uuid @primary
		user_id: uuid @unique
	}

	entity Order {
		id: uuid @primary
		user_id: uuid

		belongs_to user: User @foreign_key(user_id)
	}`

	schema := compileSchema(t, src)

	var user *ir.Entity

	for _, e := range schema.Modules[0].Entities {
		if e.Name == "User" {
			user = e
		}
	}

	if user == nil {
		t.Fatal("test schema missing User entity")
	}

	stmt, err := RenderCreateTableSQL(DialectPostgres, user)
	if err != nil {
		t.Fatalf("RenderCreateTableSQL: %v", err)
	}

	if strings.Contains(stmt, "REFERENCES") {
		t.Fatalf("declaring entity must not inline has_one/has_many constraints:\n%s", stmt)
	}

	// A belongs_to relation on the entity itself does inline its REFERENCES
	// clause (the true branch of the relation loop).
	var order *ir.Entity

	for _, e := range schema.Modules[0].Entities {
		if e.Name == "Order" {
			order = e
		}
	}

	if order == nil {
		t.Fatal("test schema missing Order entity")
	}

	orderStmt, err := RenderCreateTableSQL(DialectPostgres, order)
	if err != nil {
		t.Fatalf("RenderCreateTableSQL (order): %v", err)
	}

	if !strings.Contains(orderStmt, `REFERENCES "public"."users" ("id")`) {
		t.Fatalf("belongs_to entity missing its inline REFERENCES clause:\n%s", orderStmt)
	}
}

// TestRenderSchemaDDLEmptyEntityError proves a field-less entity fails the
// whole-schema render rather than emitting invalid SQL. The resolver rejects
// field-less entities, so this uses hand-built IR.
func TestRenderSchemaDDLEmptyEntityError(t *testing.T) {
	t.Parallel()

	mod := &ir.Module{}
	empty := &ir.Entity{Name: "Empty", Schema: "public", Module: mod}
	mod.Entities = []*ir.Entity{empty}

	if _, err := RenderSchemaDDL(DialectPostgres, &ir.Schema{Modules: []*ir.Module{mod}}); err == nil {
		t.Fatal("RenderSchemaDDL: expected an error for a field-less entity, got nil")
	}
}

// TestOrderEntities_cycle proves entities in a reference cycle fall back to
// declaration order instead of stalling.
func TestOrderEntities_cycle(t *testing.T) {
	t.Parallel()

	a := &ir.Entity{Name: "A", Schema: "public"}
	b := &ir.Entity{Name: "B", Schema: "public"}
	a.Relations = []*ir.Relation{{
		Kind:       ir.BelongsTo,
		Target:     b,
		ForeignKey: &ir.Field{Name: "b_id"},
	}}
	b.Relations = []*ir.Relation{{
		Kind:       ir.BelongsTo,
		Target:     a,
		ForeignKey: &ir.Field{Name: "a_id"},
	}}

	mod := &ir.Module{Entities: []*ir.Entity{a, b}}

	ordered := orderEntities(mod)
	if len(ordered) != 2 || ordered[0] != a || ordered[1] != b {
		t.Fatalf("orderEntities(cycle) = %v, want declaration order [A B]", ordered)
	}
}

// TestDepsSatisfied_unknownDep proves a dependency naming an entity outside
// the module counts as satisfied (the defensive resolver-invariant
// fallback).
func TestDepsSatisfied_unknownDep(t *testing.T) {
	t.Parallel()

	byName := map[string]*ir.Entity{"A": {Name: "A"}}
	emitted := map[string]bool{}

	if !depsSatisfied([]string{"Elsewhere"}, emitted, byName) {
		t.Fatal("depsSatisfied with an unknown dependency should report true")
	}

	if depsSatisfied([]string{"A"}, emitted, byName) {
		t.Fatal("depsSatisfied with an unemitted known dependency should report false")
	}

	emitted["A"] = true

	if !depsSatisfied([]string{"A"}, emitted, byName) {
		t.Fatal("depsSatisfied with an emitted dependency should report true")
	}
}

// TestGenerateSchemaFallbacks proves hand-built IR with an empty Schema
// still renders against the public schema, and that an entity without a
// primary key renders with no primary_key block. The resolver always
// populates Schema and requires @primary, so both fallbacks need hand-built
// IR.
func TestGenerateSchemaFallbacks(t *testing.T) {
	t.Parallel()

	mod := &ir.Module{}
	entity := &ir.Entity{
		Name:   "Log",
		Module: mod,
		Fields: []*ir.Field{
			{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}},
			{Name: "msg", Type: ir.FieldType{Scalar: ir.TString}},
		},
	}
	mod.Entities = []*ir.Entity{entity}

	out, err := New().Generate(&ir.Schema{Modules: []*ir.Module{mod}})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.hcl"]
	if !ok {
		t.Fatalf("Generate output missing schema.hcl, got keys %v", keys(out))
	}

	rendered := string(content)
	for _, want := range []string{`schema "public" {`, `table "logs" {`, "schema = schema.public"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("generated HCL missing %q:\n%s", want, rendered)
		}
	}

	if strings.Contains(rendered, "primary_key") {
		t.Fatalf("entity without a primary key must not render one:\n%s", rendered)
	}

	if got := SchemaOf(entity); got != "public" {
		t.Fatalf("SchemaOf(entity without schema) = %q, want public", got)
	}
}
