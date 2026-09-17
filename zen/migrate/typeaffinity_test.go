package migrate

import (
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/backend/atlas"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

func strField(name string, scalar ir.ScalarType) *ir.Field {
	return &ir.Field{Name: name, Type: ir.FieldType{Scalar: scalar}}
}

func TestNormalizeType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dialect string
		in      string
		want    string
	}{
		{"pgSynonymVarchar", atlas.DialectPostgres, "character varying(255)", "varchar"},
		{"pgOwnSpelling", atlas.DialectPostgres, "VARCHAR(32)", "varchar"},
		{"pgTimestamptz", atlas.DialectPostgres, "timestamp with time zone", "timestamp"},
		{"pgAtlasTimestamptz", atlas.DialectPostgres, "timestamptz", "timestamp"},
		{"pgIntAlias", atlas.DialectPostgres, "int4", "integer"},
		{"pgUnknown", atlas.DialectPostgres, "citext", "citext"},
		{"sqliteIntAffinity", atlas.DialectSQLite, "BIGINT", "INTEGER"},
		{"sqliteTextAffinity", atlas.DialectSQLite, "VARCHAR(255)", "TEXT"},
		{"sqliteBlobAffinity", atlas.DialectSQLite, "BLOB", "BLOB"},
		{"sqliteRealAffinity", atlas.DialectSQLite, "DOUBLE PRECISION", "REAL"},
		{"sqliteNumericAffinity", atlas.DialectSQLite, "NUMERIC(10,2)", "NUMERIC"},
		{"mysqlUnsignedIgnored", atlas.DialectMySQL, "int unsigned", "int"},
		{"mysqlBoolIsTinyint", atlas.DialectMySQL, "BOOLEAN", "tinyint"},
		{"mysqlTinyint1", atlas.DialectMySQL, "tinyint(1)", "tinyint"},
		{"mysqlDoublePrecision", atlas.DialectMySQL, "DOUBLE PRECISION", "double"},
		{"mysqlEnumBase", atlas.DialectMySQL, "enum('a','b')", "enum"},
		{"mysqlVarchar", atlas.DialectMySQL, "VARCHAR(255)", "varchar"},
		{"unknownDialectLowercased", "oracle", "VARCHAR2(10)", "varchar2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := normalizeType(tt.dialect, tt.in); got != tt.want {
				t.Fatalf("normalizeType(%q, %q) = %q, want %q", tt.dialect, tt.in, got, tt.want)
			}
		})
	}
}

func TestStripTypeArgs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"varchar(255)", "varchar"},
		{"numeric(10,2)", "numeric"},
		{"integer", "integer"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			if got := stripTypeArgs(tt.in); got != tt.want {
				t.Fatalf("stripTypeArgs(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSQLiteAffinity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"INT", "INTEGER"},
		{"unsigned big int", "INTEGER"},
		{"VARCHAR(255)", "TEXT"},
		{"CLOB", "TEXT"},
		{"TEXT", "TEXT"},
		{"BLOB", "BLOB"},
		{"", "BLOB"},
		{"REAL", "REAL"},
		{"FLOAT", "REAL"},
		{"DOUBLE", "REAL"},
		{"NUMERIC", "NUMERIC"},
		{"BOOLEAN", "NUMERIC"},
		{"DATE", "NUMERIC"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			if got := sqliteAffinity(tt.in); got != tt.want {
				t.Fatalf("sqliteAffinity(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestMysqlNormalize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"int unsigned", "int"},
		{"bigint zerofill", "bigint"},
		{"int auto_increment", "int"},
		{"  int   unsigned  ", "int"},
		{"varchar", "varchar"},
		{"weirdtype", "weirdtype"},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			if got := mysqlNormalize(tt.in); got != tt.want {
				t.Fatalf("mysqlNormalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestTypeChanged(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dialect string
		field   *ir.Field
		live    liveColumn
		want    bool
		wantErr bool
	}{
		{
			name:    "pgSame",
			dialect: atlas.DialectPostgres,
			field:   strField("email", ir.TString),
			live:    liveColumn{Name: "email", RawType: "text"},
			want:    false,
		},
		{
			name:    "pgSameBounded",
			dialect: atlas.DialectPostgres,
			field: &ir.Field{Name: "email", Type: ir.FieldType{Scalar: ir.TString},
				Validate: []ir.Validation{{Kind: "max_len", Args: map[string]any{"value": int64(32)}}}},
			live: liveColumn{Name: "email", RawType: "character varying"},
			want: false,
		},
		{
			name:    "pgChanged",
			dialect: atlas.DialectPostgres,
			field:   strField("email", ir.TString),
			live:    liveColumn{Name: "email", RawType: "integer"},
			want:    true,
		},
		{
			name:    "sqliteSameAffinity",
			dialect: atlas.DialectSQLite,
			field:   strField("n", ir.TInt64),
			live:    liveColumn{Name: "n", RawType: "INTEGER"},
			want:    false,
		},
		{
			name:    "sqliteChangedAffinity",
			dialect: atlas.DialectSQLite,
			field:   strField("n", ir.TInt64),
			live:    liveColumn{Name: "n", RawType: "TEXT"},
			want:    true,
		},
		{
			name:    "mysqlSame",
			dialect: atlas.DialectMySQL,
			field:   strField("n", ir.TInt64),
			live:    liveColumn{Name: "n", RawType: "bigint"},
			want:    false,
		},
		{
			name:    "emptyLiveTypeIsNotChange",
			dialect: atlas.DialectPostgres,
			field:   strField("email", ir.TString),
			live:    liveColumn{Name: "email", RawType: "   "},
			want:    false,
		},
		{
			name:    "unsupportedDialectIsNotChange",
			dialect: "oracle",
			field:   strField("email", ir.TString),
			live:    liveColumn{Name: "email", RawType: "whatever"},
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := typeChanged(tt.dialect, tt.field, tt.live)
			if tt.wantErr {
				if err == nil {
					t.Fatal("typeChanged: want error, got nil")
				}

				return
			}

			if err != nil {
				t.Fatalf("typeChanged: %v", err)
			}

			if got != tt.want {
				t.Fatalf("typeChanged = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRenderAlterColumnType_oracleError(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "Order"}

	if _, err := renderAlterColumnType("oracle", e, strField("total", ir.TInt64), "integer"); err == nil {
		t.Fatal("renderAlterColumnType(oracle): want error, got nil")
	}
}

func TestRenderModifyColumn_oracleError(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "Order"}

	if _, err := renderModifyColumn("oracle", e, strField("total", ir.TInt64), false, kindAlterType, "int"); err == nil {
		t.Fatal("renderModifyColumn(oracle): want error, got nil")
	}
}

// NOTE: typeChanged's atlas.ColumnType error branch is defensive-only:
// ColumnType fails solely on an unknown dialect, which typeChanged gates
// before calling it. It is unreachable and left uncovered, as in upstream.

func TestRenderAlterColumnType(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "Order"}

	stmt, err := renderAlterColumnType(atlas.DialectPostgres, e, strField("total", ir.TInt64), "integer")
	if err != nil {
		t.Fatalf("renderAlterColumnType: %v", err)
	}

	if stmt.Meta.Kind != kindAlterType || stmt.Meta.PriorType != "integer" {
		t.Fatalf("meta = %+v, want kind alter_type with prior integer", stmt.Meta)
	}

	if stmt.SQL == "" {
		t.Fatal("SQL is empty")
	}

	if _, err := renderAlterColumnType(atlas.DialectSQLite, e, strField("total", ir.TInt64), "TEXT"); err == nil {
		t.Fatal("renderAlterColumnType(sqlite): want error, got nil")
	}

	if _, err := renderAlterColumnType(atlas.DialectMySQL, e, strField("total", ir.TInt64), "bigint"); err == nil {
		t.Fatal("renderAlterColumnType(mysql): want error, got nil")
	}
}

func TestMysqlColumnDefinition(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		col  liveColumn
		want string
	}{
		{"nullable", liveColumn{Name: "a", RawType: "bigint", Nullable: true}, "bigint"},
		{"notNull", liveColumn{Name: "a", RawType: "bigint", Nullable: false}, "bigint NOT NULL"},
		{"trimsSpace", liveColumn{Name: "a", RawType: "  varchar(10)  ", Nullable: true}, "varchar(10)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := mysqlColumnDefinition(tt.col); got != tt.want {
				t.Fatalf("mysqlColumnDefinition = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderModifyColumn(t *testing.T) {
	t.Parallel()

	e := &ir.Entity{Name: "Order"}

	stmt, err := renderModifyColumn(atlas.DialectMySQL, e, strField("total", ir.TInt64), false, kindAlterType, "int")
	if err != nil {
		t.Fatalf("renderModifyColumn: %v", err)
	}

	if stmt.Meta.Kind != kindAlterType || stmt.Meta.PriorType != "int" {
		t.Fatalf("meta = %+v", stmt.Meta)
	}

	if stmt.SQL == "" {
		t.Fatal("SQL is empty")
	}

	nullable, err := renderModifyColumn(atlas.DialectMySQL, e, strField("total", ir.TInt64), true, kindAlterNullability, "bigint NOT NULL")
	if err != nil {
		t.Fatalf("renderModifyColumn nullable: %v", err)
	}

	if nullable.Meta.Kind != kindAlterNullability {
		t.Fatalf("meta.Kind = %q, want alter_nullability", nullable.Meta.Kind)
	}

	if _, err := renderModifyColumn(atlas.DialectPostgres, e, strField("total", ir.TInt64), false, kindAlterType, "int"); err == nil {
		t.Fatal("renderModifyColumn(postgres): want error, got nil")
	}
}
