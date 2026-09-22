package zenorm

import (
	"flag"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
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

func checkGolden(t *testing.T, name string, got []byte) {
	t.Helper()

	path := filepath.Join("testdata", "golden", name+".go.golden")

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

// validateGoSyntax proves content parses as syntactically valid Go and
// round-trips through go/format.Source without changing further.
func validateGoSyntax(t *testing.T, filename string, content []byte) {
	t.Helper()

	fset := token.NewFileSet()

	if _, err := parser.ParseFile(fset, filename, content, parser.AllErrors); err != nil {
		t.Fatalf("go/parser failed to parse generated file %s: %v\n--- content ---\n%s", filename, err, content)
	}

	formatted, err := format.Source(content)
	if err != nil {
		t.Fatalf("go/format.Source failed on generated file %s: %v", filename, err)
	}

	if string(formatted) != string(content) {
		t.Fatalf("generated file %s is not gofmt-formatted:\n--- got ---\n%s\n--- formatted ---\n%s",
			filename, content, formatted)
	}
}

func TestGenerateSimpleEntity(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string @unique
		created_at: timestamp @default(now())
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	path := "orm/gen/app/app.go"

	content, ok := out[path]
	if !ok {
		t.Fatalf("Generate output missing %q, got keys %v", path, keys(out))
	}

	checkGolden(t, "simple_entity", content)
	validateGoSyntax(t, path, content)
}

// TestGenerateNullableFieldEmitsOptionAndNullableColumn proves an optional
// column generates an orm.Option[GoType] struct field and an
// orm.NullableColumn Cols entry, never a "*GoType" pointer field.
func TestGenerateNullableFieldEmitsOptionAndNullableColumn(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string
		bio: string?
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	path := "orm/gen/app/app.go"

	content, ok := out[path]
	if !ok {
		t.Fatalf("Generate output missing %q, got keys %v", path, keys(out))
	}

	checkGolden(t, "nullable_field", content)
	validateGoSyntax(t, path, content)

	got := string(content)

	if !regexp.MustCompile(`Bio\s+orm\.Option\[string\]`).MatchString(got) {
		t.Fatalf("expected optional field to generate an orm.Option[string] struct field, got:\n%s", got)
	}

	if !regexp.MustCompile(`Bio\s+orm\.NullableColumn\[User,\s*string\]`).MatchString(got) {
		t.Fatalf("expected optional field's Cols entry to be orm.NullableColumn, got:\n%s", got)
	}

	if !regexp.MustCompile(`Email\s+string\s`).MatchString(got) {
		t.Fatalf("expected non-optional field to keep a bare string type, got:\n%s", got)
	}

	if strings.Contains(got, "Bio *string") {
		t.Fatalf("optional field must not generate a pointer type:\n%s", got)
	}
}

// TestGenerateOptionalPrimaryKeyRejected proves an entity whose @primary
// field is also marked optional fails to generate with a clear error.
func TestGenerateOptionalPrimaryKeyRejected(t *testing.T) {
	src := `entity Widget {
		id: uuid @primary
	}`

	schema := compileSchema(t, src)

	for _, m := range schema.Modules {
		for _, e := range m.Entities {
			for _, f := range e.Fields {
				if f.Primary {
					f.Optional = true
				}
			}
		}
	}

	_, err := New().Generate(schema)
	if err == nil {
		t.Fatal("Generate: expected an error for an optional primary key, got nil")
	}

	if !strings.Contains(err.Error(), "must not be optional") {
		t.Fatalf("Generate error = %v, want it to mention the primary key must not be optional", err)
	}
}

// TestGenerateScalarMapping hits every row of the field-type mapping table
// on one entity.
func TestGenerateScalarMapping(t *testing.T) {
	src := `entity Widget {
		id: uuid @primary
		name: string
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

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	path := "orm/gen/app/app.go"

	content, ok := out[path]
	if !ok {
		t.Fatalf("Generate output missing %q, got keys %v", path, keys(out))
	}

	checkGolden(t, "scalar_mapping", content)
	validateGoSyntax(t, path, content)
}

// TestGenerateMultipleEntities proves every entity of one module lands in
// that module's single output file, not one file per entity.
func TestGenerateMultipleEntities(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string @unique
	}

	entity OrderItem {
		id: uuid @primary
		sku: string
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	path := "orm/gen/app/app.go"

	if len(out) != 1 {
		t.Fatalf("Generate output keys = %v, want exactly [%s]", keys(out), path)
	}

	content, ok := out[path]
	if !ok {
		t.Fatalf("Generate output missing %q, got keys %v", path, keys(out))
	}

	validateGoSyntax(t, path, content)

	got := string(content)
	for _, want := range []string{
		"type User struct", "type OrderItem struct",
		"func (e *User) Scan(", "func (e *OrderItem) Scan(",
		"var Users = orm.NewTable[User]", "var OrderItems = orm.NewTable[OrderItem]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated %s missing %q:\n%s", path, want, got)
		}
	}

	checkGolden(t, "multi_entity", content)
}

// TestGenerateEmptySchema covers a schema with zero entities: Generate
// should return an empty, non-nil output map.
func TestGenerateEmptySchema(t *testing.T) {
	schema := compileSchema(t, "")

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if out == nil {
		t.Fatalf("Generate output is nil, want non-nil empty map")
	}

	if len(out) != 0 {
		t.Fatalf("Generate output = %v, want empty", keys(out))
	}
}

// TestGenerateSkipsMessagesAndServices proves a schema with services and
// messages alongside an entity still only emits the entity's table -- this
// backend never reads m.Messages/m.Services at all.
func TestGenerateSkipsMessagesAndServices(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string
	}

	service UserService {
		rpc GetUser(id: uuid) -> User {
			http: GET "/v1/users/{id}"
			auth: required
		}
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if len(out) != 1 {
		t.Fatalf("Generate output = %v, want exactly one file", keys(out))
	}
}

// TestGenerateEmitsRelationCodegen proves a has_many/belongs_to relation
// pair emits an orm.Relation var plus a Join<Entity><Relation>/
// LeftJoin<Entity><Relation> helper pair for EACH declared relation
// direction.
func TestGenerateEmitsRelationCodegen(t *testing.T) {
	src := `entity User {
		id: uuid @primary
		email: string @unique

		has_many orders: Order @foreign_key(user_id)
	}

	entity Order {
		id: uuid @primary
		user_id: uuid
		amount_cents: int64

		belongs_to user: User @foreign_key(user_id)
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	path := "orm/gen/app/app.go"

	content, ok := out[path]
	if !ok {
		t.Fatalf("Generate output missing %q, got keys %v", path, keys(out))
	}

	validateGoSyntax(t, path, content)
	checkGolden(t, "relation_has_many_belongs_to", content)

	got := string(content)
	for _, want := range []string{
		`UserOrdersRel = orm.NewRelation[User, Order]("id", "user_id", Orders)`,
		`OrderUserRel = orm.NewRelation[Order, User]("user_id", "id", Users)`,
		"func JoinUserOrders(left orm.Query[User, *User], joinType orm.JoinType) orm.Join2[User, *User, Order, *Order]",
		"func LeftJoinUserOrders(left orm.Query[User, *User]) orm.LeftJoin2[User, *User, Order, *Order]",
		"func JoinOrderUser(left orm.Query[Order, *Order], joinType orm.JoinType) orm.Join2[Order, *Order, User, *User]",
		"func LeftJoinOrderUser(left orm.Query[Order, *Order]) orm.LeftJoin2[Order, *Order, User, *User]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated %s missing %q:\n%s", path, want, got)
		}
	}
}

// TestGenerateSkipsUnresolvedRelationsWithoutError proves a schema with no
// resolvable relations still generates without error: relations this
// backend cannot express (many_to_many, missing ForeignKey/Target) are
// silently skipped rather than emitted broken or erroring the whole
// Generate call.
func TestGenerateSkipsUnresolvedRelationsWithoutError(t *testing.T) {
	src := `entity Widget {
		id: uuid @primary
		name: string
	}`

	schema := compileSchema(t, src)

	if _, err := New().Generate(schema); err != nil {
		t.Fatalf("Generate: %v", err)
	}
}

// TestGenerateNamedEnumEmitsTypeAndConsts proves a field referencing a
// named enum generates a string-kind Go type plus typed constants --
// without these, the Cols struct and entity struct would reference a Go
// type no file defines and the generated package would not compile.
func TestGenerateNamedEnumEmitsTypeAndConsts(t *testing.T) {
	src := `enum Role {
		admin, member
	}

	entity User {
		id: uuid @primary
		email: string @unique
		role: Role @default(member)
	}`

	schema := compileSchema(t, src)

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	path := "orm/gen/app/app.go"

	content, ok := out[path]
	if !ok {
		t.Fatalf("Generate output missing %q, got keys %v", path, keys(out))
	}

	checkGolden(t, "named_enum", content)
	validateGoSyntax(t, path, content)

	got := string(content)
	for _, want := range []string{
		"type Role string",
		`RoleAdmin  Role = "admin"`,
		`RoleMember Role = "member"`,
		"Role  orm.Column[User, Role]",
		"Role  Role   `json:\"role\"`",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated %s missing %q:\n%s", path, want, got)
		}
	}
}

// TestGenerateCrossModuleEnumRefEmitsValues proves a module referencing a
// named enum declared in another module still gets that enum's type and
// values (named enums are global): the emitting module resolves through
// the schema-wide index, not just its own declarations.
func TestGenerateCrossModuleEnumRefEmitsValues(t *testing.T) {
	shop := parseNamed(t, "schema/shop/shop.zen", `entity Order {
		id: uuid @primary
		status: OrderStatus @default(pending)
	}`)
	billing := parseNamed(t, "schema/billing/billing.zen", `enum OrderStatus {
		pending, paid
	}`)

	schema, diags := resolver.ResolveWithSchemaDir([]*ast.File{shop, billing}, "schema")
	if diags.HasErrors() {
		t.Fatalf("resolve errors: %v", diags)
	}

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	path := "orm/gen/shop/shop.go"

	content, ok := out[path]
	if !ok {
		t.Fatalf("Generate output missing %q, got keys %v", path, keys(out))
	}

	validateGoSyntax(t, path, content)

	got := string(content)
	for _, want := range []string{
		"type OrderStatus string",
		`OrderStatusPending OrderStatus = "pending"`,
		`OrderStatusPaid    OrderStatus = "paid"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("generated %s missing %q:\n%s", path, want, got)
		}
	}
}

// parseNamed parses one named source string, failing the test on any
// diagnostic.
func parseNamed(t *testing.T, name, src string) *ast.File {
	t.Helper()

	p := dslparser.New(name, []byte(src))

	file, diags := p.ParseFile()
	if diags.HasErrors() {
		t.Fatalf("parse errors in %s: %v", name, diags)
	}

	return file
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	sort.Strings(out)

	return out
}
