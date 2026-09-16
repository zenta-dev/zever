package atlas

import (
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/ir"
	dslparser "github.com/zenta-dev/zever/internal/dsl/parser"
	"github.com/zenta-dev/zever/internal/dsl/resolver"
)

var update = flag.Bool("update", false, "update golden files")

// compileSchema runs a .zen source string through the parser and resolver,
// failing the test on any diagnostic from either stage.
func compileSchema(t *testing.T, src string) *ir.Schema {
	t.Helper()

	p := dslparser.New("test.zen", []byte(src))

	file, diags := p.ParseFile()
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}

	schema, diags := resolver.Resolve([]*ast.File{file})
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	return schema
}

func compileMultiModule(t *testing.T, files map[string]string) *ir.Schema {
	t.Helper()

	astFiles := make([]*ast.File, 0, len(files))
	for name, src := range files {
		p := dslparser.New(name, []byte(src))
		f, diags := p.ParseFile()
		if diags.HasErrors() {
			t.Fatalf("parse errors in %s: %v", name, diags)
		}
		astFiles = append(astFiles, f)
	}
	schema, diags := resolver.ResolveWithSchemaDir(astFiles, "schema")
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}
	return schema
}

// checkGolden compares got against testdata/golden/<name>.hcl.golden,
// rewriting the golden file instead when -update is passed.
func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()

	path := filepath.Join("testdata", "golden", name+".hcl.golden")

	if *update {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}

		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}

	if string(got) != string(want) {
		t.Fatalf("golden mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// keys returns a sorted slice of a map's keys, for deterministic test
// failure messages.
func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}

// generateRoot generates a schema and returns the implicit module's
// "schema.hcl" output.
func generateRoot(t *testing.T, src string) []byte {
	t.Helper()

	out, err := New().Generate(compileSchema(t, src))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	content, ok := out["schema.hcl"]
	if !ok {
		t.Fatalf("Generate output missing %q, got keys %v", "schema.hcl", keys(out))
	}

	return content
}

func TestName(t *testing.T) {
	if got := New().Name(); got != "atlas" {
		t.Fatalf("Name() = %q, want %q", got, "atlas")
	}
}

// TestGenerateSimpleEntity covers a single entity with a primary key, a
// nullable-free column set, and every scalar type mapping.
func TestGenerateSimpleEntity(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string
		nickname: string @validate(max_len: 64)
		quantity: int32
		views: int64
		ratio: float32
		price: float64
		active: bool
		created_at: timestamp
		valid_on: date
		payload: bytes
		metadata: json
		status: enum(pending, active, archived)
	}`

	content := generateRoot(t, src)

	checkGolden(t, "simple_entity", content)

	for _, want := range []string{
		`schema "public" {`,
		`table "users" {`,
		`schema = schema.public`,
		`type = uuid`,
		`type = varchar(64)`,
		`primary_key {`,
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("generated HCL missing %q:\n%s", want, content)
		}
	}
}

// TestGenerateBelongsToForeignKey covers a belongs_to relation with an
// @on_delete action, proving a foreign_key block lands on the owning
// entity's table and that a set_null foreign key column becomes nullable.
func TestGenerateBelongsToForeignKey(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string @unique
	}

	entity Order {
		id: uuid @primary
		user_id: uuid
		assignee_id: uuid
		total_cents: int64

		belongs_to user: User @foreign_key(user_id) @on_delete(cascade)
		belongs_to assignee: User @foreign_key(assignee_id) @on_delete(set_null)
	}`

	content := generateRoot(t, src)

	checkGolden(t, "belongs_to_foreign_key", content)

	for _, want := range []string{
		`foreign_key "orders_user_id_fkey" {`,
		`ref_columns = [table.users.column.id]`,
		`on_delete   = CASCADE`,
		`on_delete   = SET_NULL`,
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("generated HCL missing %q:\n%s", want, content)
		}
	}

	if !strings.Contains(string(content), "column \"assignee_id\" {\n    null = true\n") {
		t.Fatalf("set_null foreign key column is not nullable:\n%s", content)
	}
}

// TestGenerateIndexes covers a @unique field index, a single-column
// index(...) block, and a compound index(...) block.
func TestGenerateIndexes(t *testing.T) {
	src := `entity Order {
		id: uuid @primary
		reference: string @unique
		user_id: uuid
		status: enum(pending, paid)

		index(user_id)
		index(user_id, status) @unique
	}`

	content := generateRoot(t, src)

	checkGolden(t, "indexes", content)

	for _, want := range []string{
		`index "orders_reference_key" {`,
		`index "orders_user_id_idx" {`,
		`index "orders_user_id_status_key" {`,
		`columns = [column.user_id, column.status]`,
	} {
		if !strings.Contains(string(content), want) {
			t.Fatalf("generated HCL missing %q:\n%s", want, content)
		}
	}
}

// TestGenerateEmptySchema covers a schema with zero entities: one file for
// the implicit module holding just the default schema block.
func TestGenerateEmptySchema(t *testing.T) {
	content := generateRoot(t, "")

	if !strings.Contains(string(content), `schema "public" {`) {
		t.Fatalf("empty schema output missing default schema block:\n%s", content)
	}

	if strings.Contains(string(content), "table ") {
		t.Fatalf("empty schema output contains a table block:\n%s", content)
	}
}

// TestGenerateModulesAndSchemas proves one output file is produced per
// module and that an entity's @schema(...) selects its schema block.
func TestGenerateModulesAndSchemas(t *testing.T) {
	schema := compileMultiModule(t, map[string]string{
		"schema/billing/invoice.zen": `entity Invoice @schema(billing_archive) {
			id: uuid @primary
			amount_cents: int64
		}`,
		"schema/shipping/shipment.zen": `entity Shipment {
			id: uuid @primary
			tracking: string @unique
		}`,
	})

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	wantKeys := []string{"billing/schema.hcl", "shipping/schema.hcl"}
	if len(out) != len(wantKeys) {
		t.Fatalf("Generate output keys = %v, want exactly %v", keys(out), wantKeys)
	}

	for _, k := range wantKeys {
		if _, ok := out[k]; !ok {
			t.Fatalf("Generate output missing %q, got keys %v", k, keys(out))
		}
	}

	billing := string(out["billing/schema.hcl"])
	if !strings.Contains(billing, `schema "billing_archive" {`) {
		t.Fatalf("billing/schema.hcl missing billing_archive schema block:\n%s", billing)
	}

	if !strings.Contains(billing, "schema = schema.billing_archive") {
		t.Fatalf("billing/schema.hcl invoice table not attached to billing_archive:\n%s", billing)
	}

	shipping := string(out["shipping/schema.hcl"])
	if !strings.Contains(shipping, `schema "shipping" {`) {
		t.Fatalf("shipping/schema.hcl missing shipping schema block:\n%s", shipping)
	}
}

// TestGenerateTableNameCollision proves two entities that pluralize to the
// same table name are reported as an error rather than silently
// overwriting one another.
func TestGenerateTableNameCollision(t *testing.T) {
	schema := &ir.Schema{Modules: []*ir.Module{{
		Entities: []*ir.Entity{
			{Name: "Bus", Schema: "public", Fields: []*ir.Field{{Name: "id", Primary: true}}},
			{Name: "Buse", Schema: "public", Fields: []*ir.Field{{Name: "id", Primary: true}}},
		},
	}}}

	if _, err := New().Generate(schema); err == nil {
		t.Fatalf("Generate: want table-name collision error, got nil")
	}
}
