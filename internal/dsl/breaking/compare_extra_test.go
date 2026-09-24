package breaking

import (
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

func pos(line int) diag.Position { return diag.Position{File: "x.zen", Line: line, Col: 1} }

func testModule(name string) *ir.Module { return &ir.Module{Name: name} }

func testSchema(mods ...*ir.Module) *ir.Schema { return &ir.Schema{Modules: mods} }

func TestCompareNilSchemasProduceNoChanges(t *testing.T) {
	t.Parallel()

	if changes := Compare(nil, nil); len(changes) != 0 {
		t.Fatalf("Compare(nil, nil) = %+v, want no changes", changes)
	}

	// Compare(nil, schema) is a whole-module addition, not "no changes":
	// nil behaves as an empty schema, and every module in the new schema is
	// new relative to it.
	changes := Compare(nil, testSchema(testModule("m")))
	if len(changes) != 1 || changes[0].Kind != KindModuleAdded || changes[0].Breaking {
		t.Fatalf("Compare(nil, schema) = %+v, want one non-breaking KindModuleAdded", changes)
	}
}

// TestCompareModuleRemovedIsBreaking covers the fix for a module present
// only in the old schema: previously Compare silently skipped it (a whole
// module -- every entity/service/RPC it declared -- deleted with zero
// reported Change, so HasBreaking wrongly said "not breaking"). It must now
// report exactly one breaking KindModuleRemoved.
func TestCompareModuleRemovedIsBreaking(t *testing.T) {
	t.Parallel()

	oldMod := testModule("gone")
	oldMod.Entities = []*ir.Entity{{Name: "User", Module: oldMod, Pos: pos(1)}}
	newMod := testModule("fresh")
	newMod.Entities = []*ir.Entity{{Name: "Order", Module: newMod, Pos: pos(1)}}

	changes := Compare(testSchema(oldMod), testSchema(newMod))

	var sawRemoved, sawAdded bool

	for _, c := range changes {
		switch c.Kind {
		case KindModuleRemoved:
			sawRemoved = true

			if !c.Breaking {
				t.Fatalf("KindModuleRemoved change is not marked Breaking: %+v", c)
			}
		case KindModuleAdded:
			sawAdded = true

			if c.Breaking {
				t.Fatalf("KindModuleAdded change is marked Breaking: %+v", c)
			}
		case KindEntityRemoved, KindEntityAdded, KindFieldRemoved, KindFieldAdded,
			KindFieldRenamed, KindFieldTypeChanged, KindFieldOptionalityChanged,
			KindMessageRemoved, KindMessageAdded, KindServiceRemoved, KindServiceAdded,
			KindOperationRemoved, KindOperationAdded, KindOperationParamsChanged,
			KindOperationReturnsChanged, KindOperationHTTPChanged, KindValidateAdded:
			// Other change kinds are not relevant to this test.
		}
	}

	if !sawRemoved {
		t.Fatalf("expected a KindModuleRemoved change, got %+v", changes)
	}

	if !sawAdded {
		t.Fatalf("expected a KindModuleAdded change, got %+v", changes)
	}

	if !HasBreaking(changes) {
		t.Fatal("HasBreaking(changes) = false, want true (a module was removed)")
	}
}

func TestCompareServiceAddedIsNonBreaking(t *testing.T) {
	t.Parallel()

	oldMod := testModule("m")
	newMod := testModule("m")
	newMod.Services = []*ir.Service{{Name: "UserService", Module: newMod, Pos: pos(1)}}

	changes := Compare(testSchema(oldMod), testSchema(newMod))
	assertHasNonBreaking(t, changes, KindServiceAdded)
}

func TestCompareEnumValuesChangedIsBreaking(t *testing.T) {
	t.Parallel()

	mkField := func(values []string) *ir.Field {
		return &ir.Field{Name: "role", Type: ir.FieldType{Scalar: ir.TEnum, EnumValues: values}, Pos: pos(1)}
	}
	mkSchema := func(values []string) *ir.Schema {
		m := testModule("m")
		m.Entities = []*ir.Entity{{Name: "User", Module: m, Fields: []*ir.Field{mkField(values)}, Pos: pos(1)}}
		return testSchema(m)
	}

	changes := Compare(mkSchema([]string{"admin", "member"}), mkSchema([]string{"admin", "owner"}))
	assertHasBreaking(t, changes, KindFieldTypeChanged)

	// Same values in the same order: no type change, no changes at all.
	if dup := Compare(mkSchema([]string{"admin"}), mkSchema([]string{"admin"})); len(dup) != 0 {
		t.Fatalf("identical enum values produced %+v, want no changes", dup)
	}

	// Different counts: also a type change (covers the length-mismatch path).
	changes = Compare(mkSchema([]string{"admin"}), mkSchema([]string{"admin", "member"}))
	assertHasBreaking(t, changes, KindFieldTypeChanged)
}

func TestCompareRefFieldTypeChanges(t *testing.T) {
	t.Parallel()

	mkMsg := func(m *ir.Module, ref *ir.TypeRef) *ir.Message {
		return &ir.Message{Name: "Greeting", Module: m, Fields: []*ir.Field{
			{Name: "target", Ref: ref, Pos: pos(2)},
		}, Pos: pos(1)}
	}

	userEnt := &ir.Entity{Name: "User", Pos: pos(1)}
	userMsg := &ir.Message{Name: "User", Pos: pos(1)}

	cases := []struct {
		name string
		old  *ir.TypeRef
		new  *ir.TypeRef
	}{
		{"ref added", nil, &ir.TypeRef{Entity: userEnt}},
		{"ref removed", &ir.TypeRef{Entity: userEnt}, nil},
		{"entity vs message", &ir.TypeRef{Entity: userEnt}, &ir.TypeRef{Message: userMsg}},
		{"different name", &ir.TypeRef{Entity: userEnt}, &ir.TypeRef{Entity: &ir.Entity{Name: "Order"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			oldMod, newMod := testModule("m"), testModule("m")
			oldMod.Messages = []*ir.Message{mkMsg(oldMod, tc.old)}
			newMod.Messages = []*ir.Message{mkMsg(newMod, tc.new)}

			changes := Compare(testSchema(oldMod), testSchema(newMod))
			assertHasBreaking(t, changes, KindFieldTypeChanged)
		})
	}

	t.Run("same ref no change", func(t *testing.T) {
		t.Parallel()

		oldMod, newMod := testModule("m"), testModule("m")
		ref := func() *ir.TypeRef { return &ir.TypeRef{Entity: &ir.Entity{Name: "User"}} }
		oldMod.Messages = []*ir.Message{mkMsg(oldMod, ref())}
		newMod.Messages = []*ir.Message{mkMsg(newMod, ref())}

		if changes := Compare(testSchema(oldMod), testSchema(newMod)); len(changes) != 0 {
			t.Fatalf("identical ref fields produced %+v, want no changes", changes)
		}
	})

	t.Run("scalar mismatch", func(t *testing.T) {
		t.Parallel()

		oldMod, newMod := testModule("m"), testModule("m")
		oldMod.Entities = []*ir.Entity{{Name: "User", Module: oldMod,
			Fields: []*ir.Field{{Name: "age", Type: ir.FieldType{Scalar: ir.TInt32}, Pos: pos(2)}}, Pos: pos(1)}}
		newMod.Entities = []*ir.Entity{{Name: "User", Module: newMod,
			Fields: []*ir.Field{{Name: "age", Type: ir.FieldType{Scalar: ir.TString}, Pos: pos(2)}}, Pos: pos(1)}}

		changes := Compare(testSchema(oldMod), testSchema(newMod))
		assertHasBreaking(t, changes, KindFieldTypeChanged)
	})
}

func TestCompareOperationSignatureRefParams(t *testing.T) {
	t.Parallel()

	mkSvc := func(m *ir.Module, params []*ir.Param) *ir.Service {
		return &ir.Service{Name: "S", Module: m, Operations: []*ir.Operation{
			{Name: "Op", Params: params, Pos: pos(3)},
		}, Pos: pos(1)}
	}
	scalar := func(name string) *ir.Param { return &ir.Param{Name: name, Pos: pos(3)} }
	ref := func(name string) *ir.Param {
		return &ir.Param{Name: name, Ref: &ir.TypeRef{Entity: &ir.Entity{Name: "User"}}, Pos: pos(3)}
	}
	refTo := func(name, target string) *ir.Param {
		return &ir.Param{Name: name, Ref: &ir.TypeRef{Entity: &ir.Entity{Name: target}}, Pos: pos(3)}
	}
	refMsg := func(name, target string) *ir.Param {
		return &ir.Param{Name: name, Ref: &ir.TypeRef{Message: &ir.Message{Name: target}}, Pos: pos(3)}
	}
	scalarTyped := func(name string, s ir.ScalarType) *ir.Param {
		return &ir.Param{Name: name, Type: ir.FieldType{Scalar: s}, Pos: pos(3)}
	}

	cases := []struct {
		name string
		old  []*ir.Param
		new  []*ir.Param
	}{
		{"scalar vs ref", []*ir.Param{scalar("in")}, []*ir.Param{ref("in")}},
		{"ref vs scalar", []*ir.Param{ref("in")}, []*ir.Param{scalar("in")}},
		{"ref retargeted", []*ir.Param{ref("in")}, []*ir.Param{refTo("in", "Order")}},
		{"ref entity vs message", []*ir.Param{ref("in")}, []*ir.Param{refMsg("in", "User")}},
		{"scalar retyped", []*ir.Param{scalarTyped("in", ir.TInt32)}, []*ir.Param{scalarTyped("in", ir.TInt64)}},
		{"renamed", []*ir.Param{scalar("a")}, []*ir.Param{scalar("b")}},
		{"same ref params", []*ir.Param{ref("in")}, []*ir.Param{ref("in")}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			oldMod, newMod := testModule("m"), testModule("m")
			oldMod.Services = []*ir.Service{mkSvc(oldMod, tc.old)}
			newMod.Services = []*ir.Service{mkSvc(newMod, tc.new)}

			changes := Compare(testSchema(oldMod), testSchema(newMod))
			if tc.name == "same ref params" {
				for _, c := range changes {
					if c.Kind == KindOperationParamsChanged {
						t.Fatalf("identical ref params reported as changed: %+v", changes)
					}
				}

				return
			}
			assertHasBreaking(t, changes, KindOperationParamsChanged)
		})
	}
}

func TestCompareOperationReturnsShapes(t *testing.T) {
	t.Parallel()

	mkSvc := func(m *ir.Module, op *ir.Operation) *ir.Service {
		op.Name = "Op"
		return &ir.Service{Name: "S", Module: m, Operations: []*ir.Operation{op}, Pos: pos(1)}
	}
	userRef := func() *ir.TypeRef { return &ir.TypeRef{Entity: &ir.Entity{Name: "User"}} }
	orderRef := func() *ir.TypeRef { return &ir.TypeRef{Entity: &ir.Entity{Name: "Order"}} }
	userMsgRef := func() *ir.TypeRef { return &ir.TypeRef{Message: &ir.Message{Name: "User"}} }
	op := func(returns *ir.TypeRef, paginated bool) *ir.Operation {
		return &ir.Operation{Returns: returns, Paginated: paginated, Pos: pos(3)}
	}

	cases := []struct {
		name     string
		old      *ir.Operation
		new      *ir.Operation
		breaking bool
	}{
		{"paginated toggled", op(userRef(), false), op(userRef(), true), true},
		{"returns added", op(nil, false), op(userRef(), false), true},
		{"returns removed", op(userRef(), false), op(nil, false), true},
		{"returns retargeted", op(userRef(), false), op(orderRef(), false), true},
		{"entity vs message", op(userRef(), false), op(userMsgRef(), false), true},
		{"both nil returns", op(nil, false), op(nil, false), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			oldMod, newMod := testModule("m"), testModule("m")
			oldMod.Services = []*ir.Service{mkSvc(oldMod, tc.old)}
			newMod.Services = []*ir.Service{mkSvc(newMod, tc.new)}

			changes := Compare(testSchema(oldMod), testSchema(newMod))
			if tc.breaking {
				assertHasBreaking(t, changes, KindOperationReturnsChanged)
			} else if len(changes) != 0 {
				t.Fatalf("expected no changes, got %+v", changes)
			}
		})
	}
}

func TestCompareOperationHTTPShapes(t *testing.T) {
	t.Parallel()

	mkSvc := func(m *ir.Module, transports []ir.Transport) *ir.Service {
		return &ir.Service{Name: "S", Module: m, Operations: []*ir.Operation{
			{Name: "Op", Transports: transports, Pos: pos(3)},
		}, Pos: pos(1)}
	}
	http := func(method, path string) []ir.Transport {
		return []ir.Transport{ir.GRPCTransport{}, ir.HTTPTransport{Method: method, Path: path, Pos: pos(3)}}
	}
	grpcOnly := []ir.Transport{ir.GRPCTransport{}}

	t.Run("http removed is breaking", func(t *testing.T) {
		t.Parallel()

		oldMod, newMod := testModule("m"), testModule("m")
		oldMod.Services = []*ir.Service{mkSvc(oldMod, http("GET", "/v1/users"))}
		newMod.Services = []*ir.Service{mkSvc(newMod, grpcOnly)}

		changes := Compare(testSchema(oldMod), testSchema(newMod))
		assertHasBreaking(t, changes, KindOperationHTTPChanged)
	})

	t.Run("http path changed is breaking", func(t *testing.T) {
		t.Parallel()

		oldMod, newMod := testModule("m"), testModule("m")
		oldMod.Services = []*ir.Service{mkSvc(oldMod, http("GET", "/v1/users"))}
		newMod.Services = []*ir.Service{mkSvc(newMod, http("GET", "/v2/users"))}

		changes := Compare(testSchema(oldMod), testSchema(newMod))
		assertHasBreaking(t, changes, KindOperationHTTPChanged)
	})

	t.Run("no http on either side is unchanged", func(t *testing.T) {
		t.Parallel()

		oldMod, newMod := testModule("m"), testModule("m")
		oldMod.Services = []*ir.Service{mkSvc(oldMod, grpcOnly)}
		newMod.Services = []*ir.Service{mkSvc(newMod, grpcOnly)}

		if changes := Compare(testSchema(oldMod), testSchema(newMod)); len(changes) != 0 {
			t.Fatalf("expected no changes, got %+v", changes)
		}
	})
}
