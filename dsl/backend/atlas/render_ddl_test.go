package atlas

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

func TestRenderSchemaDDLUnknownDialect(t *testing.T) {
	if _, err := RenderSchemaDDL("oracle", compileSchema(t, "")); err == nil {
		t.Fatalf("RenderSchemaDDL: want error for unknown dialect, got nil")
	}

	if _, err := RenderCreateTableSQL("oracle", nil); err == nil {
		t.Fatalf("RenderCreateTableSQL: want error for unknown dialect, got nil")
	}
}

func TestRenderSchemaDDLOptionalFieldIsNullable(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string
		nickname: string?
	}`

	stmts, err := RenderSchemaDDL(DialectPostgres, compileSchema(t, src))
	if err != nil {
		t.Fatalf("RenderSchemaDDL: %v", err)
	}

	joined := strings.Join(stmts, "\n")

	if !strings.Contains(joined, `"email" text NOT NULL`) {
		t.Fatalf("expected non-optional column to be NOT NULL:\n%s", joined)
	}

	if strings.Contains(joined, `"nickname" text NOT NULL`) {
		t.Fatalf("optional column rendered as NOT NULL:\n%s", joined)
	}

	if !strings.Contains(joined, `"nickname" text`) {
		t.Fatalf("expected nullable nickname column:\n%s", joined)
	}
}

func TestRenderSchemaDDLPostgres(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string @unique
		nickname: string @validate(max_len: 32)
		metadata: json
		active: bool
	}

	entity Order {
		id: uuid @primary
		user_id: uuid
		total_cents: int64

		belongs_to user: User @foreign_key(user_id) @on_delete(cascade)

		index(user_id)
	}`

	stmts, err := RenderSchemaDDL(DialectPostgres, compileSchema(t, src))
	if err != nil {
		t.Fatalf("RenderSchemaDDL: %v", err)
	}

	joined := strings.Join(stmts, "\n")

	for _, want := range []string{
		`CREATE SCHEMA IF NOT EXISTS "public";`,
		`CREATE TABLE IF NOT EXISTS "public"."users"`,
		`"id" uuid NOT NULL`,
		`"email" text NOT NULL UNIQUE`,
		`"nickname" varchar(32) NOT NULL`,
		`"metadata" jsonb NOT NULL`,
		`"active" boolean NOT NULL`,
		`PRIMARY KEY ("id")`,
		`"user_id" uuid NOT NULL REFERENCES "public"."users" ("id") ON DELETE CASCADE`,
		`CREATE INDEX IF NOT EXISTS "orders_user_id_idx" ON "public"."orders" ("user_id");`,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("postgres DDL missing %q:\n%s", want, joined)
		}
	}
}

func TestRenderSchemaDDLMySQL(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string @unique
		nickname: string @validate(max_len: 32)
		metadata: json
		active: bool
	}

	entity Order {
		id: uuid @primary
		user_id: uuid
		total_cents: int64

		belongs_to user: User @foreign_key(user_id) @on_delete(cascade)

		index(user_id)
	}`

	stmts, err := RenderSchemaDDL(DialectMySQL, compileSchema(t, src))
	if err != nil {
		t.Fatalf("RenderSchemaDDL: %v", err)
	}

	joined := strings.Join(stmts, "\n")

	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS `users`",
		"`id` CHAR(36) NOT NULL",
		"`email` TEXT NOT NULL UNIQUE",
		"`nickname` VARCHAR(32) NOT NULL",
		"`metadata` JSON NOT NULL",
		"`active` BOOLEAN NOT NULL",
		"PRIMARY KEY (`id`)",
		"CONSTRAINT `orders_user_id_fkey` FOREIGN KEY (`user_id`) REFERENCES `users` (`id`) ON DELETE CASCADE",
		"CREATE INDEX `orders_user_id_idx` ON `orders` (`user_id`);",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("mysql DDL missing %q:\n%s", want, joined)
		}
	}

	// MySQL parses but silently ignores inline column-level REFERENCES, so
	// the foreign key must be rendered as a table-level CONSTRAINT clause.
	// The `user_id` column line must therefore not carry an inline
	// REFERENCES clause.
	if strings.Contains(joined, "`user_id` CHAR(36) NOT NULL REFERENCES") {
		t.Fatalf("mysql foreign key rendered inline instead of as a table constraint:\n%s", joined)
	}

	if strings.Contains(joined, "CREATE SCHEMA") {
		t.Fatalf("mysql DDL should not emit CREATE SCHEMA:\n%s", joined)
	}
}

func TestRenderCreateTableSQLMySQL(t *testing.T) {
	src := `entity Widget {
		id: uuid @primary
		name: string @unique
		quantity: int32
		views: int64
		ratio: float32
		price: float64
		active: bool
		created_at: timestamp
		valid_on: date
		payload: bytes
		metadata: json
		status: enum(pending, active)
	}`

	schema := compileSchema(t, src)

	stmt, err := RenderCreateTableSQL(DialectMySQL, schema.Modules[0].Entities[0])
	if err != nil {
		t.Fatalf("RenderCreateTableSQL: %v", err)
	}

	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS `widgets` (",
		"`id` CHAR(36) NOT NULL",
		"`name` TEXT NOT NULL UNIQUE",
		"`quantity` INT NOT NULL",
		"`views` BIGINT NOT NULL",
		"`ratio` FLOAT NOT NULL",
		"`price` DOUBLE NOT NULL",
		"`active` BOOLEAN NOT NULL",
		"`created_at` DATETIME NOT NULL",
		"`valid_on` DATE NOT NULL",
		"`payload` BLOB NOT NULL",
		"`metadata` JSON NOT NULL",
		"`status` ENUM('pending', 'active') NOT NULL",
		"PRIMARY KEY (`id`)",
	} {
		if !strings.Contains(stmt, want) {
			t.Fatalf("mysql DDL missing %q:\n%s", want, stmt)
		}
	}

	if strings.Contains(stmt, "`public`.") {
		t.Fatalf("mysql DDL is schema-qualified:\n%s", stmt)
	}
}

// TestRenderCreateTableSQLMySQLAutoIncrement proves an integer primary key
// renders as an AUTO_INCREMENT column (MySQL's sequence idiom), while a
// non-primary integer and a non-integer primary key do not.
func TestRenderCreateTableSQLMySQLAutoIncrement(t *testing.T) {
	src := `entity Counter {
		id: int64 @primary
		hits: int32
	}

	entity Token {
		id: uuid @primary
	}`

	schema := compileSchema(t, src)

	counter, err := RenderCreateTableSQL(DialectMySQL, schema.Modules[0].Entities[0])
	if err != nil {
		t.Fatalf("RenderCreateTableSQL (counter): %v", err)
	}

	if !strings.Contains(counter, "`id` BIGINT AUTO_INCREMENT NOT NULL") {
		t.Fatalf("mysql integer primary key missing AUTO_INCREMENT:\n%s", counter)
	}

	if !strings.Contains(counter, "`hits` INT NOT NULL") {
		t.Fatalf("mysql non-primary integer should not be AUTO_INCREMENT:\n%s", counter)
	}

	token, err := RenderCreateTableSQL(DialectMySQL, schema.Modules[0].Entities[1])
	if err != nil {
		t.Fatalf("RenderCreateTableSQL (token): %v", err)
	}

	if strings.Contains(token, "AUTO_INCREMENT") {
		t.Fatalf("mysql uuid primary key should not be AUTO_INCREMENT:\n%s", token)
	}
}

// TestRenderSchemaDDLOrdersDependencies proves a referenced table is
// created before the table whose REFERENCES clause names it, regardless of
// declaration order.
func TestRenderSchemaDDLOrdersDependencies(t *testing.T) {
	src := `entity Order {
		id: uuid @primary
		user_id: uuid

		belongs_to user: User @foreign_key(user_id)
	}

	entity User {
		id: uuid @primary
	}`

	stmts, err := RenderSchemaDDL(DialectSQLite, compileSchema(t, src))
	if err != nil {
		t.Fatalf("RenderSchemaDDL: %v", err)
	}

	joined := strings.Join(stmts, "\n")

	users := strings.Index(joined, `CREATE TABLE IF NOT EXISTS "users"`)
	orders := strings.Index(joined, `CREATE TABLE IF NOT EXISTS "orders"`)

	if users < 0 || orders < 0 {
		t.Fatalf("missing a CREATE TABLE statement:\n%s", joined)
	}

	if users > orders {
		t.Fatalf("users table created after orders references it:\n%s", joined)
	}
}

// TestRenderSchemaDDLSelfReferenceDoesNotStall proves a self-referencing
// entity still emits (it depends on no other entity).
func TestRenderSchemaDDLSelfReferenceDoesNotStall(t *testing.T) {
	src := `entity Node {
		id: uuid @primary
		parent_id: uuid

		belongs_to parent: Node @foreign_key(parent_id)
	}`

	stmts, err := RenderSchemaDDL(DialectSQLite, compileSchema(t, src))
	if err != nil {
		t.Fatalf("RenderSchemaDDL: %v", err)
	}

	if len(stmts) != 1 || !strings.Contains(stmts[0], `CREATE TABLE IF NOT EXISTS "nodes"`) {
		t.Fatalf("unexpected statements: %v", stmts)
	}
}

func TestRenderCreateTableSQLSQLite(t *testing.T) {
	src := `entity Widget {
		id: uuid @primary
		name: string @unique
		quantity: int32
		views: int64
		ratio: float32
		price: float64
		active: bool
		created_at: timestamp
		valid_on: date
		payload: bytes
		metadata: json
		status: enum(pending, active)
	}`

	schema := compileSchema(t, src)

	stmt, err := RenderCreateTableSQL(DialectSQLite, schema.Modules[0].Entities[0])
	if err != nil {
		t.Fatalf("RenderCreateTableSQL: %v", err)
	}

	for _, want := range []string{
		`CREATE TABLE IF NOT EXISTS "widgets" (`,
		`"id" TEXT NOT NULL`,
		`"name" TEXT NOT NULL UNIQUE`,
		`"quantity" INTEGER NOT NULL`,
		`"views" INTEGER NOT NULL`,
		`"ratio" REAL NOT NULL`,
		`"price" REAL NOT NULL`,
		`"active" INTEGER NOT NULL`,
		`"created_at" TEXT NOT NULL`,
		`"valid_on" TEXT NOT NULL`,
		`"payload" BLOB NOT NULL`,
		`"metadata" TEXT NOT NULL`,
		`"status" TEXT NOT NULL`,
		`PRIMARY KEY ("id")`,
	} {
		if !strings.Contains(stmt, want) {
			t.Fatalf("sqlite DDL missing %q:\n%s", want, stmt)
		}
	}

	if strings.Contains(stmt, `"public".`) {
		t.Fatalf("sqlite DDL is schema-qualified:\n%s", stmt)
	}
}

// TestRenderCreateTableSQLNoFields proves an entity with zero columns is
// an error rather than invalid SQL.
func TestRenderCreateTableSQLNoFields(t *testing.T) {
	// The resolver rejects a field-less entity, so this hand-built IR is the
	// only way to reach the guard.
	e := &ir.Entity{Name: "Empty", Schema: "public"}

	if _, err := RenderCreateTableSQL(DialectPostgres, e); err == nil {
		t.Fatalf("RenderCreateTableSQL: want error for field-less entity, got nil")
	}
}

// TestRenderSchemaDDLHasOneForeignKeyLandsOnTarget proves a has_one
// foreign key is attributed to the table that physically holds the column.
func TestRenderSchemaDDLHasOneForeignKeyLandsOnTarget(t *testing.T) {
	src := `entity User {
		id: uuid @primary

		has_one profile: Profile @foreign_key(user_id)
	}

	entity Profile {
		id: uuid @primary
		user_id: uuid @unique
		bio: string
	}`

	stmts, err := RenderSchemaDDL(DialectSQLite, compileSchema(t, src))
	if err != nil {
		t.Fatalf("RenderSchemaDDL: %v", err)
	}

	joined := strings.Join(stmts, "\n")

	if !strings.Contains(joined, `"user_id" TEXT NOT NULL UNIQUE REFERENCES "users" ("id")`) {
		t.Fatalf("has_one foreign key not emitted on profiles table:\n%s", joined)
	}
}
