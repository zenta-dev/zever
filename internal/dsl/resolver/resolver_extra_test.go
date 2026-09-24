package resolver

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

func isExtraErr(d *diag.Diagnostic, target error) bool {
	return d != nil && errors.Is(d, target)
}

func TestExtraCheckCrossModuleNilSchema(t *testing.T) {
	if diags := CheckCrossModule(nil); len(diags) != 0 {
		t.Fatalf("CheckCrossModule(nil) = %v, want empty", diags)
	}
}

func TestExtraCheckCrossModuleNilTargetsSkipped(t *testing.T) {
	mod := &ir.Module{Name: "a"}

	owner := &ir.Entity{Name: "X", Module: mod, Relations: []*ir.Relation{
		{FieldName: "nil_target"},
		{FieldName: "nil_module", Target: &ir.Entity{Name: "Y"}},
	}}
	mod.Entities = []*ir.Entity{owner}

	if diags := CheckCrossModule(&ir.Schema{Modules: []*ir.Module{mod}}); len(diags) != 0 {
		t.Fatalf("diags = %v, want empty (nil targets are skipped)", diags)
	}
}

func TestExtraCheckCrossModuleParamAndModuleName(t *testing.T) {
	modA := &ir.Module{Name: "a"}
	modB := &ir.Module{Name: "b"}
	entB := &ir.Entity{Name: "E", Module: modB}
	modB.Entities = []*ir.Entity{entB}
	modA.Services = []*ir.Service{{
		Name:   "S",
		Module: modA,
		Operations: []*ir.Operation{{
			Name:   "Op",
			Params: []*ir.Param{{Name: "p", Ref: &ir.TypeRef{Entity: entB}}},
		}},
	}}

	got := CheckCrossModule(&ir.Schema{Modules: []*ir.Module{modA, modB}})
	found := false

	for _, d := range got {
		if isExtraErr(d, ErrCrossModule) {
			found = true
		}
	}

	if !found {
		t.Fatalf("expected ErrCrossModule for cross-module param, got %v", got)
	}

	if moduleName(nil) != "public" {
		t.Fatalf("moduleName(nil) = %q, want public", moduleName(nil))
	}

	if moduleName(&ir.Module{}) != "public" {
		t.Fatalf("moduleName(empty) = %q, want public", moduleName(&ir.Module{}))
	}

	if moduleName(&ir.Module{Name: "billing"}) != "billing" {
		t.Fatalf("moduleName(billing) = %q, want billing", moduleName(&ir.Module{Name: "billing"}))
	}
}

func TestExtraDefensiveNilModuleGuards(t *testing.T) {
	ent := entityWithIDField("Ghost")
	emptyModules := map[ast.Decl]*ir.Module{}

	if diags := resolveEntities([]*ast.EntityDecl{ent}, emptyModules, nil); len(diags) != 0 {
		t.Fatalf("resolveEntities without module = %v, want empty (skipped)", diags)
	}

	enumDecl := &ast.EnumDecl{Name: "GhostEnum", Values: []string{"a"}}
	if diags := resolveEnums([]*ast.EnumDecl{enumDecl}, emptyModules); len(diags) != 0 {
		t.Fatalf("resolveEnums without module = %v, want empty (skipped)", diags)
	}

	jobDecl := &ast.JobDecl{Name: "GhostJob"}
	if diags := resolveJobs([]*ast.JobDecl{jobDecl}, emptyModules, nil, nil, nil); len(diags) != 0 {
		t.Fatalf("resolveJobs without module = %v, want empty (skipped)", diags)
	}

	msgDecl := &ast.MessageDecl{Name: "GhostMsg"}
	declareMessages([]*ast.MessageDecl{msgDecl}, emptyModules)
	if n := len(emptyModules); n != 0 {
		t.Fatalf("declareMessages without module registered %d modules, want none", n)
	}

	if diags := resolveMessageFields([]*ast.MessageDecl{msgDecl}, emptyModules, nil, nil, nil); len(diags) != 0 {
		t.Fatalf("resolveMessageFields without module = %v, want empty (skipped)", diags)
	}

	mod := &ir.Module{Name: ""}
	if diags := resolveMessageFields(
		[]*ast.MessageDecl{msgDecl}, map[ast.Decl]*ir.Module{msgDecl: mod}, nil, map[string]*ir.Message{}, nil,
	); len(diags) != 0 {
		t.Fatalf("resolveMessageFields without index entry = %v, want empty (skipped)", diags)
	}

	svcDecl := &ast.ServiceDecl{Name: "GhostSvc"}
	if diags := resolveServices([]*ast.ServiceDecl{svcDecl}, emptyModules, nil, nil, nil); len(diags) != 0 {
		t.Fatalf("resolveServices without module = %v, want empty (skipped)", diags)
	}

	schedDecl := &ast.ScheduleDecl{Name: "GhostSched", Cron: "0 0 * * *"}
	if diags := resolveSchedules([]*ast.ScheduleDecl{schedDecl}, emptyModules, nil); len(diags) != 0 {
		t.Fatalf("resolveSchedules without module = %v, want empty (skipped)", diags)
	}

	if diags := resolveRelationsAndIndexes([]*ast.EntityDecl{ent}, map[string]*ir.Entity{}); len(diags) != 0 {
		t.Fatalf("resolveRelationsAndIndexes without owner = %v, want empty (skipped)", diags)
	}

	if _, diags := resolveFieldForMessage(
		&ast.FieldDecl{Name: "x", Type: &ast.TypeExpr{Name: "Nope"}}, nil, nil, nil,
	); len(diags) == 0 {
		t.Fatalf("expected unresolved-reference diagnostic for unknown message field type")
	}
}

func TestExtraEnumTypeFallbackPos(t *testing.T) {
	_, d := resolveEnumType(&ast.TypeExpr{Name: "enum", Args: []string{"a", "a"}})
	if d == nil {
		t.Fatalf("expected diagnostic for duplicate enum value without ArgPos")
	}

	pos := diag.Position{File: "a.zen", Line: 3, Col: 7}
	_, d = resolveEnumType(&ast.TypeExpr{
		Name: "enum", Args: []string{"a", "a"},
		ArgPos: []diag.Position{{File: "a.zen", Line: 3, Col: 1}, pos},
	})

	if d == nil {
		t.Fatalf("expected diagnostic for duplicate enum value with ArgPos")
	}

	if d.Pos != pos {
		t.Fatalf("diagnostic pos = %v, want dup arg pos %v", d.Pos, pos)
	}
}

func TestExtraNamedEnumWithArgsRejected(t *testing.T) {
	enumByName := map[string]*ir.Enum{"Status": {Name: "Status", Values: []string{"a"}}}

	_, d := resolveFieldType(&ast.TypeExpr{Name: "Status", Args: []string{"x"}}, enumByName)
	if d == nil {
		t.Fatalf("expected diagnostic for args on named enum reference")
	}
}

func TestExtraFieldAttributeShapes(t *testing.T) {
	strField := func(attrs ...*ast.Attribute) *ast.FieldDecl {
		return &ast.FieldDecl{Name: "f", Type: &ast.TypeExpr{Name: "string"}, Attributes: attrs}
	}

	t.Run("unknown field attribute", func(t *testing.T) {
		_, diags := resolveField(entityWithIDField("E"), strField(&ast.Attribute{Name: "bogus"}), nil)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("renamed_from arg count", func(t *testing.T) {
		_, diags := resolveField(entityWithIDField("E"), strField(&ast.Attribute{Name: "renamed_from"}), nil)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("default arg count", func(t *testing.T) {
		_, diags := resolveField(entityWithIDField("E"), strField(&ast.Attribute{Name: "default"}), nil)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("default unsupported value shape", func(t *testing.T) {
		_, diags := resolveField(entityWithIDField("E"), strField(&ast.Attribute{
			Name: "default", Args: []*ast.Arg{{Value: &ast.DurationLit{Valid: true}}},
		}), nil)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("default unknown function", func(t *testing.T) {
		_, diags := resolveField(entityWithIDField("E"), strField(&ast.Attribute{
			Name: "default", Args: []*ast.Arg{{Value: &ast.CallValue{Name: "yesterday"}}},
		}), nil)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("bool default unknown ident", func(t *testing.T) {
		fd := &ast.FieldDecl{Name: "b", Type: &ast.TypeExpr{Name: "bool"}, Attributes: []*ast.Attribute{{
			Name: "default", Args: []*ast.Arg{{Value: &ast.IdentValue{Name: "maybe"}}},
		}}}
		_, diags := resolveField(entityWithIDField("E"), fd, nil)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("ident default on scalar rejected", func(t *testing.T) {
		_, diags := resolveField(entityWithIDField("E"), strField(&ast.Attribute{
			Name: "default", Args: []*ast.Arg{{Value: &ast.IdentValue{Name: "pending"}}},
		}), nil)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("float default accepted", func(t *testing.T) {
		fd := &ast.FieldDecl{Name: "x", Type: &ast.TypeExpr{Name: "float64"}, Attributes: []*ast.Attribute{{
			Name: "default", Args: []*ast.Arg{{Value: &ast.FloatLit{Value: 1.5}}},
		}}}
		field, diags := resolveField(entityWithIDField("E"), fd, nil)
		mustNotHaveErrors(t, diags)

		if field.Default == nil {
			t.Fatalf("expected Default to be set for float literal")
		}
	})
}

func TestExtraEntitySchemaShapes(t *testing.T) {
	t.Run("decode error keeps going", func(t *testing.T) {
		decl := &ast.EntityDecl{Name: "E", Attributes: []*ast.Attribute{{Name: "schema"}}}
		if _, diags := resolveEntitySchema(decl, &ir.Module{}); len(diags) == 0 {
			t.Fatalf("expected diagnostic for bare @schema")
		}
	})

	t.Run("string form", func(t *testing.T) {
		decl := &ast.EntityDecl{Name: "E", Attributes: []*ast.Attribute{{
			Name: "schema", Args: []*ast.Arg{{Value: &ast.StringLit{Value: "billing"}}},
		}}}
		name, diags := resolveEntitySchema(decl, &ir.Module{})
		mustNotHaveErrors(t, diags)

		if name != "billing" {
			t.Fatalf("schema = %q, want billing", name)
		}
	})

	t.Run("non ident/string rejected", func(t *testing.T) {
		decl := &ast.EntityDecl{Name: "E", Attributes: []*ast.Attribute{{
			Name: "schema", Args: []*ast.Arg{{Value: &ast.IntLit{Value: 1}}},
		}}}
		if _, diags := resolveEntitySchema(decl, &ir.Module{}); len(diags) == 0 {
			t.Fatalf("expected diagnostic for non-ident @schema arg")
		}
	})
}

func TestExtraRetryShapes(t *testing.T) {
	dur := &ast.DurationLit{Value: 30 * time.Second, Valid: true}

	cases := []struct {
		name  string
		retry []ast.Value
	}{
		{"non-call entry", []ast.Value{&ast.StringLit{Value: "x"}}},
		{"max_attempts named arg", []ast.Value{&ast.CallValue{Name: "max_attempts", Args: []*ast.Arg{
			{Name: "n", Value: &ast.IntLit{Value: 1}},
		}}}},
		{"max_attempts string arg", []ast.Value{&ast.CallValue{Name: "max_attempts", Args: []*ast.Arg{
			{Value: &ast.StringLit{Value: "x"}},
		}}}},
		{"backoff second positional", []ast.Value{&ast.CallValue{Name: "backoff", Args: []*ast.Arg{
			{Value: &ast.IdentValue{Name: "exponential"}},
			{Value: &ast.IdentValue{Name: "extra"}},
		}}}},
		{"backoff unknown kind", []ast.Value{&ast.CallValue{Name: "backoff", Args: []*ast.Arg{
			{Value: &ast.IdentValue{Name: "linear"}},
			{Name: "base", Value: dur},
		}}}},
		{"backoff unknown named arg", []ast.Value{&ast.CallValue{Name: "backoff", Args: []*ast.Arg{
			{Value: &ast.IdentValue{Name: "exponential"}},
			{Name: "speed", Value: dur},
		}}}},
		{"backoff missing base", []ast.Value{&ast.CallValue{Name: "backoff", Args: []*ast.Arg{
			{Value: &ast.IdentValue{Name: "exponential"}},
		}}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			files := []*ast.File{{Name: "schema/app.zen", Decls: []ast.Decl{
				&ast.JobDecl{Name: "J", Retry: tc.retry},
			}}}

			_, diags := Resolve(files)
			mustHaveErr(t, diags, ErrInvalidOption)
		})
	}
}

func TestExtraMessageFieldAttributes(t *testing.T) {
	newMsg := func(fields ...*ast.FieldDecl) []*ast.File {
		return []*ast.File{{Name: "schema/app.zen", Decls: []ast.Decl{
			entityWithIDField("User"),
			&ast.MessageDecl{Name: "M", Fields: fields},
		}}}
	}

	t.Run("scalar default accepted", func(t *testing.T) {
		files := newMsg(&ast.FieldDecl{Name: "text", Type: &ast.TypeExpr{Name: "string"}, Attributes: []*ast.Attribute{{
			Name: "default", Args: []*ast.Arg{{Value: &ast.StringLit{Value: "hi"}}},
		}}})

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})

	t.Run("unknown scalar attribute", func(t *testing.T) {
		files := newMsg(&ast.FieldDecl{Name: "text", Type: &ast.TypeExpr{Name: "string"}, Attributes: []*ast.Attribute{{
			Name: "primary",
		}}})

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("unknown ref attribute", func(t *testing.T) {
		files := newMsg(&ast.FieldDecl{Name: "u", Type: &ast.TypeExpr{Name: "User"}, Attributes: []*ast.Attribute{{
			Name: "unique",
		}}})

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("unknown field type skipped", func(t *testing.T) {
		files := newMsg(&ast.FieldDecl{Name: "u", Type: &ast.TypeExpr{Name: "Nope"}})

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrUnresolvedReference)
	})
}

func TestExtraEmptySchemaDirDefaults(t *testing.T) {
	files := []*ast.File{{Name: "schema/app.zen", Decls: []ast.Decl{entityWithIDField("User")}}}

	schema, diags := ResolveWithSchemaDir(files, "")
	mustNotHaveErrors(t, diags)

	if len(schema.Modules) != 1 || schema.Modules[0].Name != "" {
		t.Fatalf("expected one public module, got %+v", schema.Modules)
	}
}

func TestExtraPathSegmentsNoMarker(t *testing.T) {
	if segs := pathSegmentsUnderSchemaDir("schema", "plain.zen"); segs != nil {
		t.Fatalf("segments = %v, want nil", segs)
	}

	// Marker present but nothing between it and the file name: the fallback
	// also yields nil.
	if segs := pathSegmentsUnderSchemaDir("schema", "/x/schema/app.zen"); segs != nil {
		t.Fatalf("segments = %v, want nil", segs)
	}
}

func TestExtraOpinionNilSchemas(t *testing.T) {
	if diags := validateEverythingIn(nil); len(diags) != 0 {
		t.Fatalf("validateEverythingIn(nil) = %v, want empty", diags)
	}

	if diags := secureEverything(nil); len(diags) != 0 {
		t.Fatalf("secureEverything(nil) = %v, want empty", diags)
	}

	if diags := versionPathDrift(nil); len(diags) != 0 {
		t.Fatalf("versionPathDrift(nil) = %v, want empty", diags)
	}
}

func TestExtraValidateEntityAndNestedEntityParams(t *testing.T) {
	addr := entityWithIDField("Addr", stringField("street"))
	msg := &ast.MessageDecl{Name: "M", Fields: []*ast.FieldDecl{
		{Name: "addr", Type: &ast.TypeExpr{Name: "Addr"}},
	}}
	svc := &ast.ServiceDecl{Name: "S", RPCs: []*ast.RPCDecl{{
		Name:    "Get",
		Returns: "Addr",
		Params: []*ast.ParamDecl{
			{Name: "a", Type: &ast.TypeExpr{Name: "Addr"}},
			{Name: "m", Type: &ast.TypeExpr{Name: "M"}},
		},
		Auth: &ast.IdentValue{Name: "required"},
	}}}

	files := []*ast.File{{Name: "schema/app.zen", Decls: []ast.Decl{addr, msg, svc}}}

	_, diags := Resolve(files)
	mustHaveErr(t, diags, ErrMissingValidation)
}

func TestExtraValidateParamFieldsEmptyRef(t *testing.T) {
	if diags := validateParamFields(nil, nil, &ir.Param{Name: "p", Ref: &ir.TypeRef{}}); len(diags) != 0 {
		t.Fatalf("validateParamFields with empty ref = %v, want nil", diags)
	}
}

func TestExtraRelationShapes(t *testing.T) {
	user := func() *ast.EntityDecl { return entityWithIDField("User") }
	order := func(rel *ast.RelationDecl) *ast.EntityDecl {
		return &ast.EntityDecl{
			Name:      "Order",
			Fields:    []*ast.FieldDecl{primaryUUIDField("id"), {Name: "user_id", Type: &ast.TypeExpr{Name: "uuid"}}},
			Relations: []*ast.RelationDecl{rel},
		}
	}
	fk := func(name string) *ast.Attribute {
		return &ast.Attribute{Name: "foreign_key", Args: []*ast.Arg{{Value: &ast.IdentValue{Name: name}}}}
	}
	resolve := func(decls ...ast.Decl) diag.List {
		files := []*ast.File{{Name: "schema/app.zen", Decls: decls}}
		_, diags := Resolve(files)

		return diags
	}

	t.Run("m2m rejects foreign_key", func(t *testing.T) {
		a := entityWithIDField("A")
		a.Relations = []*ast.RelationDecl{{
			Kind: ast.ManyToMany, FieldName: "tags", Target: "Tag",
			Join:       &ast.JoinBlock{Table: "a_tags"},
			Attributes: []*ast.Attribute{fk("user_id")},
		}}
		tag := entityWithIDField("Tag")
		tag.Relations = []*ast.RelationDecl{{
			Kind: ast.ManyToMany, FieldName: "as", Target: "A",
			Join: &ast.JoinBlock{Table: "a_tags"},
		}}

		mustHaveErr(t, resolve(a, tag), ErrInvalidRelation)
	})

	t.Run("has_one missing fk", func(t *testing.T) {
		o := order(&ast.RelationDecl{Kind: ast.HasOne, FieldName: "profile", Target: "User", Attributes: []*ast.Attribute{fk("nope")}})
		mustHaveErr(t, resolve(user(), o), ErrInvalidRelation)
	})

	t.Run("m2m missing join", func(t *testing.T) {
		o := entityWithIDField("Solo")
		o.Relations = []*ast.RelationDecl{{Kind: ast.ManyToMany, FieldName: "tags", Target: "User"}}
		mustHaveErr(t, resolve(user(), o), ErrInvalidRelation)
	})

	t.Run("join on has_many", func(t *testing.T) {
		o := order(&ast.RelationDecl{
			Kind: ast.HasMany, FieldName: "users", Target: "User",
			Join:       &ast.JoinBlock{Table: "t"},
			Attributes: []*ast.Attribute{fk("user_id")},
		})
		mustHaveErr(t, resolve(user(), o), ErrInvalidRelation)
	})

	t.Run("foreign_key arg count", func(t *testing.T) {
		o := order(&ast.RelationDecl{Kind: ast.BelongsTo, FieldName: "user", Target: "User", Attributes: []*ast.Attribute{{
			Name: "foreign_key", Args: []*ast.Arg{{Value: &ast.StringLit{Value: "a"}}, {Value: &ast.StringLit{Value: "b"}}},
		}}})
		mustHaveErr(t, resolve(user(), o), ErrInvalidRelation)
	})

	t.Run("unknown relation attribute", func(t *testing.T) {
		o := order(&ast.RelationDecl{Kind: ast.BelongsTo, FieldName: "user", Target: "User", Attributes: []*ast.Attribute{
			fk("user_id"), {Name: "bogus"},
		}})
		mustHaveErr(t, resolve(user(), o), ErrInvalidRelation)
	})

	t.Run("on_delete arg error", func(t *testing.T) {
		o := order(&ast.RelationDecl{Kind: ast.BelongsTo, FieldName: "user", Target: "User", Attributes: []*ast.Attribute{
			fk("user_id"),
			{Name: "on_delete", Args: []*ast.Arg{{Value: &ast.StringLit{Value: "a"}}, {Value: &ast.StringLit{Value: "b"}}}},
		}})
		mustHaveErr(t, resolve(user(), o), ErrInvalidRelation)
	})

	t.Run("foreign_key string form valid", func(t *testing.T) {
		o := order(&ast.RelationDecl{Kind: ast.BelongsTo, FieldName: "user", Target: "User", Attributes: []*ast.Attribute{{
			Name: "foreign_key", Args: []*ast.Arg{{Value: &ast.StringLit{Value: "user_id"}}},
		}}})
		mustNotHaveErrors(t, resolve(user(), o))
	})

	t.Run("foreign_key non-ident rejected", func(t *testing.T) {
		o := order(&ast.RelationDecl{Kind: ast.BelongsTo, FieldName: "user", Target: "User", Attributes: []*ast.Attribute{{
			Name: "foreign_key", Args: []*ast.Arg{{Value: &ast.IntLit{Value: 1}}},
		}}})
		mustHaveErr(t, resolve(user(), o), ErrInvalidRelation)
	})

	t.Run("index unknown attribute", func(t *testing.T) {
		u := user()
		u.Indexes = []*ast.IndexDecl{{Columns: []string{"id"}, Attributes: []*ast.Attribute{{Name: "bogus"}}}}
		mustHaveErr(t, resolve(u), ErrInvalidAttribute)
	})

	t.Run("interleaved pairs match cleanly", func(t *testing.T) {
		mk := func(name, target, table string) *ast.EntityDecl {
			e := entityWithIDField(name)
			e.Relations = []*ast.RelationDecl{{
				Kind: ast.ManyToMany, FieldName: "f", Target: target,
				Join: &ast.JoinBlock{Table: table},
			}}

			return e
		}

		mustNotHaveErrors(t, resolve(
			mk("E1", "E3", "t1"),
			mk("E2", "E4", "t2"),
			mk("E3", "E1", "t1"),
			mk("E4", "E2", "t2"),
		))
	})

	t.Run("unreachable kind helpers", func(t *testing.T) {
		if got := toIRRelationKind(ast.RelationKind(99)); got != ir.HasMany {
			t.Fatalf("toIRRelationKind(unknown) = %v, want HasMany", got)
		}

		if _, d := resolveForeignKeyField(ir.ManyToMany, nil, nil, "", diag.Position{}); d != nil {
			t.Fatalf("m2m fk branch = %v, want nil", d)
		}

		if _, d := resolveForeignKeyField(ir.RelationKind(99), nil, nil, "", diag.Position{}); d != nil {
			t.Fatalf("unknown kind fk branch = %v, want nil", d)
		}

		for kind, want := range map[ir.RelationKind]string{
			ir.HasMany: "has_many", ir.HasOne: "has_one", ir.BelongsTo: "belongs_to",
			ir.ManyToMany: "many_to_many", ir.RelationKind(99): "relation",
		} {
			if got := relationKindLabel(kind); got != want {
				t.Fatalf("relationKindLabel(%v) = %q, want %q", kind, got, want)
			}
		}
	})
}

func TestExtraDispatchArgShapes(t *testing.T) {
	dur := 30 * time.Second
	strs := []string{"a", "b"}

	params := make([]*ast.ParamDecl, 0, 7)
	for _, n := range []string{"p1", "p2", "p3", "p4", "p5", "p6", "p7"} {
		params = append(params, &ast.ParamDecl{Name: n, Type: &ast.TypeExpr{Name: "string"}})
	}

	job := &ast.JobDecl{Name: "J", Params: params}
	sched := &ast.ScheduleDecl{Name: "S", Cron: "0 0 * * *", Dispatch: &ast.CallValue{
		Name: "J",
		Args: []*ast.Arg{
			{Value: &ast.StringLit{Value: "s"}},
			{Value: &ast.IntLit{Value: 1}},
			{Value: &ast.FloatLit{Value: 1.5}},
			{Value: &ast.DurationLit{Value: dur, Valid: true}},
			{Value: &ast.IdentValue{Name: "somevar"}},
			{Value: &ast.SetLit{Items: strs}},
			{Value: &ast.StringLit{Value: "t2"}},
		},
	}}

	files := []*ast.File{{Name: "schema/app.zen", Decls: []ast.Decl{job, sched}}}

	schema, diags := Resolve(files)
	mustNotHaveErrors(t, diags)

	var found *ir.Schedule

	for _, m := range schema.Modules {
		for _, s := range m.Schedules {
			found = s
		}
	}

	if found == nil {
		t.Fatalf("schedule S not resolved")
	}

	want := []any{"s", int64(1), 1.5, dur, "somevar", strs, "t2"}
	if !reflect.DeepEqual(found.DispatchArgs, want) {
		t.Fatalf("DispatchArgs = %#v, want %#v", found.DispatchArgs, want)
	}
}

// TestExtraDispatchArgRejectsNestedCall covers resolveDispatch's rejection
// of a nested CallValue dispatch argument: previously dispatchArgValue
// silently collapsed it to its bare call name, discarding its own
// arguments with no diagnostic (e.g. dispatch: Job(build_payload(x, y))
// resolved DispatchArgs to ["build_payload"], quietly losing x and y).
func TestExtraDispatchArgRejectsNestedCall(t *testing.T) {
	call := &ast.CallValue{Name: "J", Args: []*ast.Arg{
		{Value: &ast.CallValue{Name: "build_payload", Args: []*ast.Arg{{Value: &ast.IdentValue{Name: "x"}}}}},
	}}

	_, _, diags := resolveDispatch(call, map[string]*ir.Job{"J": {Name: "J", Params: []*ir.Param{{Name: "p1"}}}})
	if len(diags) == 0 {
		t.Fatal("expected a diagnostic rejecting the nested call argument, got none")
	}

	if !strings.Contains(diags.Error(), "nested call") {
		t.Fatalf("diagnostics = %v, want one mentioning the nested call", diags)
	}
}

func TestExtraDispatchNonCall(t *testing.T) {
	if _, _, diags := resolveDispatch(&ast.IdentValue{Name: "X"}, map[string]*ir.Job{}); len(diags) == 0 {
		t.Fatalf("expected diagnostic for non-call dispatch")
	}

	// dispatchArgValue's default branch is only reachable with a nil Value:
	// every concrete ast.Value has its own case, and no foreign
	// implementation can exist outside package ast.
	if got := dispatchArgValue(nil); got != nil {
		t.Fatalf("dispatchArgValue(nil) = %#v, want nil", got)
	}
}

func TestExtraServiceReturnsMessage(t *testing.T) {
	files := []*ast.File{{Name: "schema/app.zen", Decls: []ast.Decl{
		entityWithIDField("User"),
		&ast.MessageDecl{Name: "M", Fields: []*ast.FieldDecl{
			{Name: "text", Type: &ast.TypeExpr{Name: "string"}},
		}},
		&ast.ServiceDecl{Name: "S", RPCs: []*ast.RPCDecl{{
			Name:    "Get",
			Returns: "M",
			Auth:    &ast.IdentValue{Name: "required"},
		}}},
	}}}

	schema, diags := Resolve(files)
	mustNotHaveErrors(t, diags)

	var op *ir.Operation

	for _, m := range schema.Modules {
		for _, svc := range m.Services {
			for _, o := range svc.Operations {
				op = o
			}
		}
	}

	if op == nil || op.Returns == nil || !op.Returns.IsMessage() {
		t.Fatalf("expected message return type, got %+v", op)
	}
}

func TestExtraEntityRefParam(t *testing.T) {
	ref := entityWithIDField("Ref")

	t.Run("entity param resolves", func(t *testing.T) {
		files := []*ast.File{{Name: "schema/app.zen", Decls: []ast.Decl{
			ref,
			&ast.ServiceDecl{Name: "S", RPCs: []*ast.RPCDecl{{
				Name:    "Get",
				Returns: "Ref",
				Params:  []*ast.ParamDecl{{Name: "r", Type: &ast.TypeExpr{Name: "Ref"}}},
				Auth:    &ast.IdentValue{Name: "required"},
			}}},
		}}}

		_, diags := Resolve(files)
		mustNotHaveErrors(t, diags)
	})

	t.Run("validate on ref param rejected", func(t *testing.T) {
		files := []*ast.File{{Name: "schema/app.zen", Decls: []ast.Decl{
			ref,
			&ast.ServiceDecl{Name: "S", RPCs: []*ast.RPCDecl{{
				Name:    "Get",
				Returns: "Ref",
				Params: []*ast.ParamDecl{{Name: "r", Type: &ast.TypeExpr{Name: "Ref"}, Attributes: []*ast.Attribute{{
					Name: "validate",
				}}}},
				Auth: &ast.IdentValue{Name: "required"},
			}}},
		}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("unknown ref param attribute", func(t *testing.T) {
		files := []*ast.File{{Name: "schema/app.zen", Decls: []ast.Decl{
			ref,
			&ast.ServiceDecl{Name: "S", RPCs: []*ast.RPCDecl{{
				Name:    "Get",
				Returns: "Ref",
				Params: []*ast.ParamDecl{{Name: "r", Type: &ast.TypeExpr{Name: "Ref"}, Attributes: []*ast.Attribute{{
					Name: "bogus",
				}}}},
				Auth: &ast.IdentValue{Name: "required"},
			}}},
		}}}

		_, diags := Resolve(files)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})
}

func TestExtraPathParamsShapes(t *testing.T) {
	if _, malformed := pathParams("/a/{b{c}"); !malformed {
		t.Fatalf("expected malformed for nested brace")
	}

	if _, malformed := pathParams("/a/b}"); !malformed {
		t.Fatalf("expected malformed for stray closing brace")
	}
}

func TestExtraAuthShapes(t *testing.T) {
	t.Run("invalid value shape", func(t *testing.T) {
		if _, diags := resolveAuth(&ast.StringLit{Value: "x"}); len(diags) == 0 {
			t.Fatalf("expected diagnostic for string auth value")
		}
	})

	t.Run("unknown call", func(t *testing.T) {
		if _, diags := resolveAuth(&ast.CallValue{Name: "maybe"}); len(diags) == 0 {
			t.Fatalf("expected diagnostic for unknown auth call")
		}
	})

	t.Run("unknown arg and non-set roles", func(t *testing.T) {
		_, diags := resolveAuth(&ast.CallValue{Name: "required", Args: []*ast.Arg{
			{Name: "bogus", Value: &ast.StringLit{Value: "x"}},
			{Name: "roles", Value: &ast.StringLit{Value: "admin"}},
		}})

		if len(diags) != 2 {
			t.Fatalf("diags = %v, want 2 (unknown arg + non-set roles)", diags)
		}
	})
}

func TestExtraPermissionShapes(t *testing.T) {
	ent := &ir.Entity{Name: "E", Fields: []*ir.Field{{Name: "owner"}}}
	idx := map[string]*ir.Entity{"E": ent}
	str := func() *ast.StringLit { return &ast.StringLit{Value: "x"} }
	ident := func(n string) *ast.IdentValue { return &ast.IdentValue{Name: n} }
	res := &ast.Arg{Name: "resource", Value: ident("E")}
	owner := &ast.Arg{Name: "owner_field", Value: ident("owner")}

	cases := []struct {
		name string
		args []*ast.Arg
	}{
		{"positional non-string", []*ast.Arg{{Value: ident("Admin")}, res, owner}},
		{"second positional", []*ast.Arg{{Value: str()}, {Value: str()}, res, owner}},
		{"resource non-ident", []*ast.Arg{{Value: str()}, {Name: "resource", Value: str()}, owner}},
		{"owner_field non-ident", []*ast.Arg{{Value: str()}, res, {Name: "owner_field", Value: str()}}},
		{"unknown arg", []*ast.Arg{{Value: str()}, res, owner, {Name: "bogus", Value: str()}}},
		{"missing positional", []*ast.Arg{res, owner}},
		{"missing resource", []*ast.Arg{{Value: str()}, owner}},
		{"missing owner_field", []*ast.Arg{{Value: str()}, res}},
		{"unknown owner field", []*ast.Arg{{Value: str()}, res, {Name: "owner_field", Value: ident("nope")}}},
		{"unknown resource", []*ast.Arg{{Value: str()}, {Name: "resource", Value: ident("Nope")}, owner}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, diags := resolvePermission(&ast.CallValue{Name: "check", Args: tc.args}, idx, nil); len(diags) == 0 {
				t.Fatalf("expected diagnostic for %s", tc.name)
			}
		})
	}

	t.Run("non-check value", func(t *testing.T) {
		if _, diags := resolvePermission(ident("x"), idx, nil); len(diags) == 0 {
			t.Fatalf("expected diagnostic for non-call permission")
		}
	})
}

func TestExtraValuePosShapes(t *testing.T) {
	if got := valuePos(&ast.IntLit{Value: 1}); got != (diag.Position{}) {
		_ = got
	}

	for _, v := range []ast.Value{
		&ast.IntLit{Value: 1},
		&ast.FloatLit{Value: 1.5},
		&ast.DurationLit{Valid: true},
		&ast.SetLit{},
		nil,
	} {
		_ = valuePos(v)
	}

	if valuePos(nil) != (diag.Position{}) {
		t.Fatalf("valuePos(nil) = %v, want zero", valuePos(nil))
	}
}

func TestExtraUnknownDeclKind(t *testing.T) {
	// A nil ast.Decl reaches declNameAndPos' default branch: every concrete
	// declaration kind has its own case.
	if _, _, ok := declNameAndPos(nil); ok {
		t.Fatalf("declNameAndPos(nil) ok = true, want false")
	}

	syms, diags := resolveSymbols([]ast.Decl{nil})
	if len(diags) != 0 {
		t.Fatalf("diags = %v, want empty (unknown decl skipped)", diags)
	}

	if len(syms.entityOrder) != 0 {
		t.Fatalf("expected empty symbol table, got %+v", syms)
	}
}

func TestExtraValidateDecodeShapes(t *testing.T) {
	t.Run("format non-string", func(t *testing.T) {
		_, diags := resolveValidateRules(&ast.Attribute{
			Name: "validate", Args: []*ast.Arg{{Name: "format", Value: &ast.IntLit{Value: 1}}},
		}, ir.TString)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})

	t.Run("numeric kind string value", func(t *testing.T) {
		_, diags := resolveValidateRules(&ast.Attribute{
			Name: "validate", Args: []*ast.Arg{{Name: "min_len", Value: &ast.StringLit{Value: "x"}}},
		}, ir.TString)
		mustHaveErr(t, diags, ErrInvalidAttribute)
	})
}

func TestExtraVersionDriftMatchAndNoHTTP(t *testing.T) {
	files := []*ast.File{{Name: "schema/v1/iam/user.zen", Decls: []ast.Decl{
		entityWithIDField("User"),
		&ast.ServiceDecl{Name: "S", RPCs: []*ast.RPCDecl{
			{
				Name:    "Get",
				Returns: "User",
				Auth:    &ast.IdentValue{Name: "required"},
				HTTP:    &ast.HTTPOption{Method: "GET", Path: "/v1/users"},
			},
			{
				Name:    "Ping",
				Returns: "User",
				Auth:    &ast.IdentValue{Name: "required"},
			},
		}},
	}}}

	_, diags := ResolveWithSchemaDir(files, "schema")

	for _, d := range diags {
		if isExtraErr(d, ErrVersionDrift) {
			t.Fatalf("unexpected version drift warning: %v", d)
		}
	}
}
