package resolver

import (
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/diag"
	"github.com/zenta-dev/zever/dsl/ir"
)

// suggestFixtures builds the indexes hint lookups resolve against: one User
// entity (id, email), one Task entity, one CreateTask message, one TaskService
// with a GetTask rpc, and the implicit module they all belong to.
func suggestFixtures() (map[string]*ir.Entity, map[string]*ir.Message, map[string]*ir.Service, *ir.Module) {
	user := &ir.Entity{Name: "User", Fields: []*ir.Field{{Name: "id"}, {Name: "email"}}}
	task := &ir.Entity{Name: "Task", Fields: []*ir.Field{{Name: "id"}, {Name: "owner_id"}}}
	entities := map[string]*ir.Entity{"User": user, "Task": task}
	messages := map[string]*ir.Message{"CreateTaskInput": {Name: "CreateTaskInput"}}
	services := map[string]*ir.Service{"TaskService": {Name: "TaskService",
		Operations: []*ir.Operation{{Name: "GetTask"}}}}
	module := &ir.Module{}

	return entities, messages, services, module
}

func mustHint(t *testing.T, diags diag.List, target error, wantHint string) {
	t.Helper()

	for _, d := range diags {
		if errors.Is(d, target) {
			if !strings.Contains(d.Msg, wantHint) {
				t.Fatalf("diagnostic %q lacks hint %q", d.Msg, wantHint)
			}

			return
		}
	}

	t.Fatalf("no diagnostic wrapping %v in: %v", target, diags)
}

func TestDidYouMeanHints(t *testing.T) {
	entities, messages, services, module := suggestFixtures()
	pos := diag.Position{File: "a.zen", Line: 3, Col: 9}

	t.Run("unknown field type", func(t *testing.T) {
		_, d := resolveFieldType(&ast.TypeExpr{Name: "strnig", NamePos: pos}, nil)
		mustHint(t, diag.List{d}, ErrUnresolvedReference, `(did you mean "string"?)`)
	})

	t.Run("unknown field type keeps position", func(t *testing.T) {
		_, d := resolveFieldType(&ast.TypeExpr{Name: "strnig", NamePos: pos}, nil)
		if d.Pos != pos {
			t.Fatalf("Pos = %v, want %v", d.Pos, pos)
		}
	})

	t.Run("far field type has no hint", func(t *testing.T) {
		_, d := resolveFieldType(&ast.TypeExpr{Name: "zzz", NamePos: pos}, nil)
		if strings.Contains(d.Msg, "did you mean") {
			t.Fatalf("far input must not hint: %q", d.Msg)
		}
	})

	t.Run("unknown field attribute", func(t *testing.T) {
		fd := &ast.FieldDecl{Name: "n", Type: &ast.TypeExpr{Name: "string"},
			Attributes: []*ast.Attribute{{Name: "unqiue", Pos: pos}}}
		_, diags := resolveField(&ast.EntityDecl{Name: "User"}, fd, nil)
		mustHint(t, diags, ErrInvalidAttribute, `(did you mean "unique"?)`)
	})

	t.Run("unknown entity attribute", func(t *testing.T) {
		decl := &ast.EntityDecl{Name: "User", Attributes: []*ast.Attribute{{Name: "schem", Pos: pos}}}
		_, diags := resolveEntitySchema(decl, module)
		mustHint(t, diags, ErrInvalidAttribute, `(did you mean "schema"?)`)
	})

	t.Run("unknown message field attribute", func(t *testing.T) {
		fd := &ast.FieldDecl{Name: "n", Type: &ast.TypeExpr{Name: "string"},
			Attributes: []*ast.Attribute{{Name: "validat", Pos: pos}}}
		_, diags := resolveFieldForMessage(fd, nil, nil, nil)
		mustHint(t, diags, ErrInvalidAttribute, `(did you mean "validate"?)`)
	})

	t.Run("unknown message ref field attribute", func(t *testing.T) {
		fd := &ast.FieldDecl{Name: "n", Type: &ast.TypeExpr{Name: "User"},
			Attributes: []*ast.Attribute{{Name: "defalt", Pos: pos}}}
		_, diags := resolveFieldForMessage(fd, entities, messages, nil)
		mustHint(t, diags, ErrInvalidAttribute, `(did you mean "default"?)`)
	})

	t.Run("unknown param attribute", func(t *testing.T) {
		pd := &ast.ParamDecl{Name: "q", Type: &ast.TypeExpr{Name: "string"},
			Attributes: []*ast.Attribute{{Name: "validat", Pos: pos}}}
		_, diags := resolveParam(pd, nil, nil, nil)
		mustHint(t, diags, ErrInvalidAttribute, `(did you mean "validate"?)`)
	})

	t.Run("unknown rpc return type", func(t *testing.T) {
		rd := &ast.RPCDecl{Name: "Get", Returns: "Usr", ReturnsPos: pos}
		_, diags := resolveOperation("TaskService", rd, module, entities, messages, nil)
		mustHint(t, diags, ErrUnresolvedReference, `(did you mean "User"?)`)
	})

	t.Run("unknown error code", func(t *testing.T) {
		_, diags := resolveErrors([]ast.Value{&ast.IdentValue{Name: "not_fond", Pos: pos}})
		mustHint(t, diags, ErrInvalidOption, `(did you mean "not_found"?)`)
	})

	t.Run("unknown validate kind", func(t *testing.T) {
		attr := &ast.Attribute{Args: []*ast.Arg{{Name: "min_lne", Pos: pos, Value: &ast.IntLit{Value: 1}}}}
		_, diags := resolveValidateRules(attr, ir.TString)
		mustHint(t, diags, ErrInvalidAttribute, `(did you mean "min_len"?)`)
	})

	t.Run("unknown relation target", func(t *testing.T) {
		rd := &ast.RelationDecl{Kind: ast.HasMany, FieldName: "tasks", Target: "Taks", TargetPos: pos}
		_, diags := resolveRelation(entities["User"], rd, entities)
		mustHint(t, diags, ErrUnresolvedReference, `(did you mean "Task"?)`)
	})

	t.Run("unknown relation attribute", func(t *testing.T) {
		_, _, _, diags := resolveRelationAttributes(
			&ast.RelationDecl{Attributes: []*ast.Attribute{{Name: "foreign_ke", Pos: pos}}}, &ir.Relation{})
		mustHint(t, diags, ErrInvalidRelation, `(did you mean "foreign_key"?)`)
	})

	t.Run("unknown foreign key", func(t *testing.T) {
		_, d := resolveForeignKeyField(ir.HasMany, entities["User"], entities["Task"], "owner_i", pos)
		mustHint(t, diag.List{d}, ErrInvalidRelation, `(did you mean "owner_id"?)`)
	})

	t.Run("unknown index column", func(t *testing.T) {
		_, diags := resolveIndex(entities["User"],
			&ast.IndexDecl{Columns: []string{"emial"}, ColumnPos: []diag.Position{pos}})
		mustHint(t, diags, ErrUnresolvedReference, `(did you mean "email"?)`)
	})

	t.Run("unknown index attribute", func(t *testing.T) {
		_, diags := resolveIndex(entities["User"],
			&ast.IndexDecl{Columns: []string{"email"}, Attributes: []*ast.Attribute{{Name: "unqiue", Pos: pos}}})
		mustHint(t, diags, ErrInvalidAttribute, `(did you mean "unique"?)`)
	})

	t.Run("unknown permission resource", func(t *testing.T) {
		v := permCall(pos, "Usr", "owner_id")
		_, diags := resolvePermission(v, entities, module)
		mustHint(t, diags, ErrUnresolvedReference, `(did you mean "User"?)`)
	})

	t.Run("unknown permission owner field", func(t *testing.T) {
		v := permCall(pos, "User", "emial")
		_, diags := resolvePermission(v, entities, module)
		mustHint(t, diags, ErrUnresolvedReference, `(did you mean "email"?)`)
	})

	t.Run("unknown permission argument", func(t *testing.T) {
		v := &ast.CallValue{Name: "check", Pos: pos, Args: []*ast.Arg{
			{Value: &ast.StringLit{Value: "x"}},
			{Name: "resorce", Pos: pos, Value: &ast.IdentValue{Name: "User"}},
			{Name: "owner_field", Value: &ast.IdentValue{Name: "id"}},
		}}
		_, diags := resolvePermission(v, entities, module)
		mustHint(t, diags, ErrInvalidOption, `(did you mean "resource"?)`)
	})

	t.Run("unknown retry call", func(t *testing.T) {
		_, diags := resolveRetryPolicy([]ast.Value{&ast.CallValue{Name: "max_attempt", Pos: pos}})
		mustHint(t, diags, ErrInvalidOption, `(did you mean "max_attempts"?)`)
	})

	t.Run("unknown backoff argument", func(t *testing.T) {
		_, _, d := decodeBackoff(&ast.CallValue{Name: "backoff", Pos: pos, Args: []*ast.Arg{
			{Value: &ast.IdentValue{Name: "exponential"}},
			{Name: "bas", Pos: pos, Value: &ast.DurationLit{Valid: true}},
		}})
		mustHint(t, diag.List{d}, ErrInvalidOption, `(did you mean "base"?)`)
	})

	t.Run("unknown saga service", func(t *testing.T) {
		_, diags := resolveRPCRef(&ast.RPCRef{Service: "TaskServices", RPC: "GetTask", Pos: pos}, services)
		mustHint(t, diags, ErrUnresolvedReference, `(did you mean "TaskService"?)`)
	})

	t.Run("unknown saga rpc", func(t *testing.T) {
		_, diags := resolveRPCRef(&ast.RPCRef{Service: "TaskService", RPC: "GetTaks", Pos: pos}, services)
		mustHint(t, diags, ErrUnresolvedReference, `(did you mean "GetTask"?)`)
	})

	t.Run("unknown http placeholder", func(t *testing.T) {
		rd := &ast.RPCDecl{Name: "Get", Returns: "Task",
			Params: []*ast.ParamDecl{{Name: "task_id", Type: &ast.TypeExpr{Name: "string"}}},
			HTTP:   &ast.HTTPOption{Method: "GET", Path: "/t/{taskId}", PathPos: pos}}
		_, diags := resolveHTTP(rd)
		mustHint(t, diags, ErrUnresolvedReference, `(did you mean "task_id"?)`)
	})

	t.Run("unknown auth call", func(t *testing.T) {
		_, diags := resolveAuth(&ast.CallValue{Name: "require", Pos: pos})
		mustHint(t, diags, ErrInvalidOption, `(did you mean "required"?)`)
	})

	t.Run("unknown auth argument", func(t *testing.T) {
		_, diags := resolveAuthCall(&ast.CallValue{Name: "required", Pos: pos, Args: []*ast.Arg{
			{Name: "role", Pos: pos, Value: &ast.SetLit{Items: []string{"admin"}}},
		}})
		mustHint(t, diags, ErrInvalidOption, `(did you mean "roles"?)`)
	})

	t.Run("unknown default function", func(t *testing.T) {
		field := &ir.Field{Type: ir.FieldType{Scalar: ir.TTimestamp}}
		attr := &ast.Attribute{Args: []*ast.Arg{{Value: &ast.CallValue{Name: "noww", Pos: pos}}}}
		mustHint(t, resolveDefaultAttribute(attr, field), ErrInvalidAttribute, `(did you mean "now"?)`)
	})

	t.Run("unknown enum default value", func(t *testing.T) {
		field := &ir.Field{Type: ir.FieldType{Scalar: ir.TEnum, EnumValues: []string{"active", "archived"}}}
		attr := &ast.Attribute{Args: []*ast.Arg{{Value: &ast.IdentValue{Name: "activ", Pos: pos}}}}
		mustHint(t, resolveDefaultAttribute(attr, field), ErrUnresolvedReference, `(did you mean "active"?)`)
	})
}

// permCall builds a permission check() value with the given resource and
// owner field names.
func permCall(pos diag.Position, resource, owner string) ast.Value {
	return &ast.CallValue{Name: "check", Pos: pos, Args: []*ast.Arg{
		{Pos: pos, Value: &ast.StringLit{Value: "x"}},
		{Name: "resource", Pos: pos, Value: &ast.IdentValue{Name: resource}},
		{Name: "owner_field", Pos: pos, Value: &ast.IdentValue{Name: owner}},
	}}
}
