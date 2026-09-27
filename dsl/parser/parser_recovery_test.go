package parser

import (
	"net/http"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
)

// TestRecoveryMalformedFieldThenValidRelationAndEntity is recovery
// guarantee (a): a malformed field followed by a valid relation, followed
// by a valid second entity, produces exactly one diagnostic and leaves the
// rest of the tree intact.
func TestRecoveryMalformedFieldThenValidRelationAndEntity(t *testing.T) {
	src := `entity Bad {
		: uuid
		has_many orders: Order
	}

	entity Good {
		id: uuid
	}`

	file, msgs := parseSrc(t, src)

	if len(msgs) != 1 {
		t.Fatalf("expected exactly 1 diagnostic, got %d: %v", len(msgs), msgs)
	}

	if len(file.Decls) != 2 {
		t.Fatalf("expected 2 decls, got %d", len(file.Decls))
	}

	bad, ok := file.Decls[0].(*ast.EntityDecl)
	if !ok || bad.Name != "Bad" {
		t.Fatalf("decls[0] = %+v, want EntityDecl{Bad}", file.Decls[0])
	}

	if len(bad.Fields) != 0 {
		t.Fatalf("expected Bad to have 0 fields (malformed field dropped), got %d", len(bad.Fields))
	}

	if len(bad.Relations) != 1 || bad.Relations[0].FieldName != "orders" || bad.Relations[0].Target != "Order" {
		t.Fatalf("expected Bad to still have the valid has_many relation, got %+v", bad.Relations)
	}

	good, ok := file.Decls[1].(*ast.EntityDecl)
	if !ok || good.Name != "Good" {
		t.Fatalf("decls[1] = %+v, want EntityDecl{Good}", file.Decls[1])
	}

	if len(good.Fields) != 1 || good.Fields[0].Name != "id" || good.Fields[0].Type.Name != "uuid" {
		t.Fatalf("expected Good's field intact, got %+v", good.Fields)
	}
}

// TestRecoveryGarbageEntityBodyThenGoodEntity is recovery guarantee (b),
// the single most important test in this task: a completely garbage entity
// body must never prevent the next entity from parsing fully correctly.
func TestRecoveryGarbageEntityBodyThenGoodEntity(t *testing.T) {
	src := `entity Bad { !!! }

	entity Good {
		id: uuid
		name: string
	}`

	file, msgs := parseSrc(t, src)

	if len(msgs) == 0 {
		t.Fatalf("expected at least one diagnostic for the garbage entity body")
	}

	if len(file.Decls) != 2 {
		t.Fatalf("expected 2 decls (Bad and Good both present), got %d: %+v", len(file.Decls), file.Decls)
	}

	bad, ok := file.Decls[0].(*ast.EntityDecl)
	if !ok || bad.Name != "Bad" {
		t.Fatalf("decls[0] = %+v, want EntityDecl{Bad}", file.Decls[0])
	}

	if len(bad.Fields) != 0 || len(bad.Relations) != 0 || len(bad.Indexes) != 0 {
		t.Fatalf("expected Bad to have zero members, got fields=%d relations=%d indexes=%d",
			len(bad.Fields), len(bad.Relations), len(bad.Indexes))
	}

	good, ok := file.Decls[1].(*ast.EntityDecl)
	if !ok || good.Name != "Good" {
		t.Fatalf("decls[1] = %+v, want EntityDecl{Good}", file.Decls[1])
	}

	if len(good.Fields) != 2 {
		t.Fatalf("expected Good to have exactly 2 fields, got %d: %+v", len(good.Fields), good.Fields)
	}

	if good.Fields[0].Name != "id" || good.Fields[0].Type.Name != "uuid" {
		t.Fatalf("Good.Fields[0] = %+v, want id:uuid", good.Fields[0])
	}

	if good.Fields[1].Name != "name" || good.Fields[1].Type.Name != "string" {
		t.Fatalf("Good.Fields[1] = %+v, want name:string", good.Fields[1])
	}
}

// TestRecoveryMissingClosingBraceAtEOF is recovery guarantee (c): a file
// truncated mid-construct, with no closing brace ever appearing, produces
// exactly one diagnostic and ParseFile still returns — the test simply
// completing (rather than the test runner timing out) is what proves there
// is no infinite loop.
func TestRecoveryMissingClosingBraceAtEOF(t *testing.T) {
	src := `entity Foo {
		id: uuid`

	file, msgs := parseSrc(t, src)

	if len(msgs) != 1 {
		t.Fatalf("expected exactly 1 diagnostic, got %d: %v", len(msgs), msgs)
	}

	if len(file.Decls) != 0 {
		t.Fatalf("expected the truncated entity to be dropped entirely, got %d decls: %+v", len(file.Decls), file.Decls)
	}
}

// TestRecoveryBogusAttributeParsesClean is recovery guarantee (d): an
// attribute with an unrecognized name still parses with zero diagnostics,
// pinning the parser/resolver boundary — the parser is permissive about
// identifier content, strict only about shape.
func TestRecoveryBogusAttributeParsesClean(t *testing.T) {
	src := `entity Foo {
		id: uuid @bogus(1, 2, 3)
	}`

	file := mustParseClean(t, src)
	e := firstEntity(t, file)

	if len(e.Fields[0].Attributes) != 1 || e.Fields[0].Attributes[0].Name != "bogus" {
		t.Fatalf("attributes = %+v, want [bogus(1,2,3)]", e.Fields[0].Attributes)
	}

	if len(e.Fields[0].Attributes[0].Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(e.Fields[0].Attributes[0].Args))
	}
}

// TestRecoveryMalformedRPCOptionThenValidOption is recovery guarantee (e):
// a malformed RPC option doesn't prevent a subsequent valid option in the
// same rpc body from parsing.
func TestRecoveryMalformedRPCOptionThenValidOption(t *testing.T) {
	src := `service S {
		rpc Get(id: uuid) -> Thing {
			!!!
			http: GET "/things/{id}"
		}
	}`

	file, msgs := parseSrc(t, src)

	if len(msgs) == 0 {
		t.Fatalf("expected at least one diagnostic for the garbage rpc option")
	}

	var svc *ast.ServiceDecl

	for _, d := range file.Decls {
		if s, ok := d.(*ast.ServiceDecl); ok {
			svc = s
		}
	}

	if svc == nil || len(svc.RPCs) != 1 {
		t.Fatalf("expected the rpc decl to still be present, got %+v", file.Decls)
	}

	rpc := svc.RPCs[0]
	if rpc.HTTP == nil {
		t.Fatalf("expected http option to still parse despite preceding garbage")
	}

	if rpc.HTTP.Method != http.MethodGet || rpc.HTTP.Path != "/things/{id}" {
		t.Fatalf("http option = %+v, want http.MethodGet /things/{id}", rpc.HTTP)
	}
}

// TestRecoveryMalformedErrorsOptionThenValidService proves a malformed
// errors: set (missing closing brace, trailing comma with no following
// item) is recovered from without preventing a subsequent, entirely valid
// service+rpc from parsing correctly.
func TestRecoveryMalformedErrorsOptionThenValidService(t *testing.T) {
	src := `service S {
		rpc Get(id: uuid) -> Thing {
			errors: { not_found,
		}
	}

	service T {
		rpc Ping(x: uuid) -> Thing {
			http: GET "/ping"
		}
	}`

	file, msgs := parseSrc(t, src)

	if len(msgs) == 0 {
		t.Fatalf("expected at least one diagnostic for the malformed errors: set")
	}

	var services []*ast.ServiceDecl

	for _, d := range file.Decls {
		if s, ok := d.(*ast.ServiceDecl); ok {
			services = append(services, s)
		}
	}

	if len(services) != 2 {
		t.Fatalf("expected both services to be present, got %d: %+v", len(services), file.Decls)
	}

	bad := services[0]
	if len(bad.RPCs) != 1 || len(bad.RPCs[0].Errors) != 0 {
		t.Fatalf("expected S.Get's malformed errors: set to be dropped, got %+v", bad.RPCs)
	}

	good := services[1]
	if len(good.RPCs) != 1 || good.RPCs[0].HTTP == nil || good.RPCs[0].HTTP.Path != "/ping" {
		t.Fatalf("expected T.Ping to parse cleanly despite S's malformed errors: set, got %+v", good.RPCs)
	}
}

// TestRecoveryNestedModuleIsRejected previously checked module-specific recovery.
// Module syntax was removed in v-next (dir-derived modules). This test now verifies
// that a message with a nested declaration-like block is recovered without swallowing subsequent entities.
func TestRecoveryNestedModuleIsRejected(t *testing.T) {
	src := `message Bad {
		id: uuid
		garbage
	}

	entity Good {
		id: uuid
	}`

	file, msgs := parseSrc(t, src)

	if len(msgs) == 0 {
		t.Fatalf("expected at least one diagnostic for the malformed message")
	}

	if len(file.Decls) != 2 {
		t.Fatalf("expected 2 top-level decls (Bad message and Good entity), got %d", len(file.Decls))
	}

	bad, ok := file.Decls[0].(*ast.MessageDecl)
	if !ok || bad.Name != "Bad" {
		t.Fatalf("decls[0] = %+v, want MessageDecl{Bad}", file.Decls[0])
	}

	good, ok := file.Decls[1].(*ast.EntityDecl)
	if !ok || good.Name != "Good" {
		t.Fatalf("decls[1] = %+v, want EntityDecl{Good}", file.Decls[1])
	}
}

// TestRecoveryMalformedEntityInModuleThenTopLevelEntity previously verified module recovery.
// Now verifies that a malformed entity body still recovers to the next top-level entity/message.
func TestRecoveryMalformedEntityInModuleThenTopLevelEntity(t *testing.T) {
	src := `entity Bad {
			!!!
		}

	entity Good {
		id: uuid @primary
	}`

	file, msgs := parseSrc(t, src)

	if len(msgs) == 0 {
		t.Fatalf("expected at least one diagnostic for the malformed entity body")
	}

	if len(file.Decls) != 2 {
		t.Fatalf("expected 2 top-level decls (Bad and Good), got %d: %+v", len(file.Decls), file.Decls)
	}

	badEntity, ok := file.Decls[0].(*ast.EntityDecl)
	if !ok || badEntity.Name != "Bad" {
		t.Fatalf("decls[0] = %+v, want EntityDecl{Bad}", file.Decls[0])
	}

	if len(badEntity.Fields) != 0 {
		t.Fatalf("expected Bad entity to have 0 fields (malformed field was dropped), got %d", len(badEntity.Fields))
	}

	goodEntity, ok := file.Decls[1].(*ast.EntityDecl)
	if !ok || goodEntity.Name != "Good" {
		t.Fatalf("decls[1] = %+v, want EntityDecl{Good}", file.Decls[1])
	}

	if len(goodEntity.Fields) != 1 || goodEntity.Fields[0].Name != "id" || goodEntity.Fields[0].Type.Name != "uuid" {
		t.Fatalf("expected Good to have id:uuid field intact, got %+v", goodEntity.Fields)
	}

	for _, msg := range msgs {
		if strings.Contains(msg, "nested module") {
			t.Fatalf("found unexpected nested module diagnostic: %q", msg)
		}
	}
}

// TestRecoveryTwoModulesWhereFirstMalformed previously tested two module blocks.
// Now verifies that two entities where the first is malformed still parse both.
func TestRecoveryTwoModulesWhereFirstMalformed(t *testing.T) {
	src := `entity A {
		garbage
	}

	entity X {
			id: uuid
		}`

	file, msgs := parseSrc(t, src)

	if len(msgs) == 0 {
		t.Fatalf("expected at least one diagnostic for the garbage in entity A")
	}

	if len(file.Decls) != 2 {
		t.Fatalf("expected 2 top-level decls (both entities), got %d: %+v", len(file.Decls), file.Decls)
	}

	entA, ok := file.Decls[0].(*ast.EntityDecl)
	if !ok || entA.Name != "A" {
		t.Fatalf("decls[0] = %+v, want EntityDecl{A}", file.Decls[0])
	}

	if len(entA.Fields) != 0 {
		t.Fatalf("expected entity A to have 0 fields (garbage was dropped), got %d: %+v", len(entA.Fields), entA.Fields)
	}

	entX, ok := file.Decls[1].(*ast.EntityDecl)
	if !ok || entX.Name != "X" {
		t.Fatalf("decls[1] = %+v, want EntityDecl{X}", file.Decls[1])
	}

	if len(entX.Fields) != 1 || entX.Fields[0].Name != "id" || entX.Fields[0].Type.Name != "uuid" {
		t.Fatalf("expected X to have id:uuid field intact, got %+v", entX.Fields)
	}

	for _, msg := range msgs {
		if strings.Contains(msg, "nested module") {
			t.Fatalf("found false nested module diagnostic: %q", msg)
		}
	}
}

// TestParseNeverPanicsOrHangs sweeps a table of fuzzed, garbage, and
// truncated inputs — including mid-construct EOF at every nesting level —
// and asserts ParseFile always returns a non-nil *ast.File without
// panicking. Each subtest completing at all (rather than the test binary
// timing out) is itself part of the guarantee being tested.
func TestParseNeverPanicsOrHangs(t *testing.T) {
	inputs := []string{
		"",
		"entity",
		"entity Foo",
		"entity Foo {",
		"entity Foo { id",
		"entity Foo { id:",
		"entity Foo { id: uuid @",
		"entity Foo { id: uuid @foo(",
		"entity Foo { id: uuid @foo(1,",
		"entity Foo @",
		"entity Foo @schema(",
		"entity Foo { has_many",
		"entity Foo { has_many x",
		"entity Foo { has_many x:",
		"entity Foo { has_many x: Y {",
		"entity Foo { has_many x: Y { join_table",
		"entity Foo { index",
		"entity Foo { index(",
		"entity Foo { index(a,",
		"service",
		"service S",
		"service S {",
		"service S { rpc",
		"service S { rpc G",
		"service S { rpc G(",
		"service S { rpc G() ->",
		"service S { rpc G() -> T {",
		"service S { rpc G() -> T { http",
		"service S { rpc G() -> T { http:",
		"service S { rpc G() -> T { http: GET",
		"job",
		"job J",
		"job J(",
		"job J() {",
		"job J() { retry:",
		"job J() { retry: max_attempts(",
		"schedule",
		"schedule Sc",
		"schedule Sc {",
		"schedule Sc { cron",
		"schedule Sc { cron:",
		"schedule Sc { dispatch:",
		"message",
		"message M",
		"message M {",
		"message M { id:",
		"message M { id: uuid",
		"message M { id: string }",
		"@@@@@@@@@@",
		"!!!!!!!!!!",
		"{{{{{{{{{{",
		"}}}}}}}}}}",
		"----------",
		"::::::::::",
		",,,,,,,,,,",
		"entity 123 {}",
		"entity Foo { 123: uuid }",
		"entity Foo { id: 123 }",
		"entity Foo { id: uuid @default(-) }",
		"entity Foo { id: uuid @default(- ) }",
		"entity Foo { id: uuid @default({a,) }",
		"entity Foo { id: uuid @default({) }",
		"\x00\x01\x02\x03",
		"entity \xff\xfe {",
		"message M { bad !!! } entity Good { id: uuid }",
		"entity Bad !!! entity Good { id: uuid }",
	}

	for _, src := range inputs {
		t.Run(src, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("ParseFile panicked on input %q: %v", src, r)
				}
			}()

			p := New("fuzz.zen", []byte(src))

			file, _ := p.ParseFile()
			if file == nil {
				t.Fatalf("ParseFile returned nil *ast.File for input %q", src)
			}
		})
	}
}
