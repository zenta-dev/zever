package breaking

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

func TestCompareIdenticalSchemasProduceNoChanges(t *testing.T) {
	t.Parallel()

	old := testSchema(testModule("app"))
	newSchema := testSchema(testModule("app"))

	if changes := Compare(old, newSchema); len(changes) != 0 {
		t.Fatalf("changes = %v, want none for identical schemas", changes)
	}
}

func TestCompareEmptySchemasProduceNoChanges(t *testing.T) {
	t.Parallel()

	if changes := Compare(&ir.Schema{}, &ir.Schema{}); len(changes) != 0 {
		t.Fatalf("changes = %v, want none", changes)
	}
}

func TestCompareNilSchemaProducesNoChanges(t *testing.T) {
	t.Parallel()

	if changes := Compare(nil, nil); len(changes) != 0 {
		t.Fatalf("changes = %v, want none", changes)
	}

	if changes := Compare(nil, testSchema(testModule("app"))); len(changes) != 1 {
		t.Fatalf("changes = %v, want exactly one module_added", changes)
	}
}

func TestCompareModuleAddedIsNonBreaking(t *testing.T) {
	t.Parallel()

	old := &ir.Schema{}
	newSchema := testSchema(testModule("billing"))

	changes := Compare(old, newSchema)
	if len(changes) != 1 {
		t.Fatalf("changes = %v, want exactly 1", changes)
	}

	if changes[0].Kind != KindModuleAdded || changes[0].Breaking {
		t.Fatalf("change = %+v, want non-breaking KindModuleAdded", changes[0])
	}

	if HasBreaking(changes) {
		t.Fatal("HasBreaking = true, want false for a pure addition")
	}
}

func TestCompareWholeModuleRemovedIsBreaking(t *testing.T) {
	t.Parallel()

	old := testSchema(testModule("billing"))
	newSchema := &ir.Schema{}

	changes := Compare(old, newSchema)
	if len(changes) != 1 {
		t.Fatalf("changes = %v, want exactly 1", changes)
	}

	if changes[0].Kind != KindModuleRemoved || !changes[0].Breaking {
		t.Fatalf("change = %+v, want breaking KindModuleRemoved", changes[0])
	}
}

func TestCompareEntityAddedIsNonBreaking(t *testing.T) {
	t.Parallel()

	old := testSchema(testModule("app"))
	newSchema := testSchema(testModule("app"))
	newSchema.Modules[0].Entities = append(newSchema.Modules[0].Entities, &ir.Entity{Name: "Order"})

	changes := Compare(old, newSchema)
	if len(changes) != 1 {
		t.Fatalf("changes = %v, want exactly 1", changes)
	}

	if changes[0].Kind != KindEntityAdded || changes[0].Breaking {
		t.Fatalf("change = %+v, want non-breaking KindEntityAdded", changes[0])
	}
}

func TestCompareFieldRenamedIsNotRemoveAddPair(t *testing.T) {
	t.Parallel()

	oldEnt := &ir.Entity{Name: "User", Fields: []*ir.Field{{Name: "email"}}}
	newEnt := &ir.Entity{Name: "User", Fields: []*ir.Field{{
		Name:        "login",
		RenamedFrom: strPtr("email"),
	}}}

	old := &ir.Schema{Modules: []*ir.Module{{Name: "app", Entities: []*ir.Entity{oldEnt}}}}
	newSchema := &ir.Schema{Modules: []*ir.Module{{Name: "app", Entities: []*ir.Entity{newEnt}}}}

	changes := Compare(old, newSchema)
	if len(changes) != 1 {
		t.Fatalf("changes = %v, want exactly 1 (rename only)", changes)
	}

	if changes[0].Kind != KindFieldRenamed {
		t.Fatalf("change = %+v, want KindFieldRenamed", changes[0])
	}

	if changes[0].Breaking {
		t.Fatal("rename reported as breaking, want non-breaking")
	}
}

func TestCompareRenamedFieldStillDetectsTypeChange(t *testing.T) {
	t.Parallel()

	oldEnt := &ir.Entity{Name: "User", Fields: []*ir.Field{{Name: "email", Type: ir.FieldType{Scalar: ir.TString}}}}
	newEnt := &ir.Entity{Name: "User", Fields: []*ir.Field{{
		Name:        "login",
		Type:        ir.FieldType{Scalar: ir.TInt64},
		RenamedFrom: strPtr("email"),
	}}}

	old := &ir.Schema{Modules: []*ir.Module{{Name: "app", Entities: []*ir.Entity{oldEnt}}}}
	newSchema := &ir.Schema{Modules: []*ir.Module{{Name: "app", Entities: []*ir.Entity{newEnt}}}}

	changes := Compare(old, newSchema)

	var renamed, typeChanged bool
	for _, c := range changes {
		if c.Kind == KindFieldRenamed {
			renamed = true
		}

		if c.Kind == KindFieldTypeChanged {
			typeChanged = true
		}
	}

	if !renamed || !typeChanged {
		t.Fatalf("changes = %v, want both KindFieldRenamed and KindFieldTypeChanged", changes)
	}
}

func TestCompareOptionalToRequiredIsBreaking(t *testing.T) {
	t.Parallel()

	oldEnt := &ir.Entity{Name: "User", Fields: []*ir.Field{{Name: "nickname", Optional: true}}}
	newEnt := &ir.Entity{Name: "User", Fields: []*ir.Field{{Name: "nickname"}}}

	old := &ir.Schema{Modules: []*ir.Module{{Name: "app", Entities: []*ir.Entity{oldEnt}}}}
	newSchema := &ir.Schema{Modules: []*ir.Module{{Name: "app", Entities: []*ir.Entity{newEnt}}}}

	changes := Compare(old, newSchema)
	if len(changes) != 1 || changes[0].Kind != KindFieldOptionalityChanged || !changes[0].Breaking {
		t.Fatalf("changes = %v, want breaking KindFieldOptionalityChanged", changes)
	}
}

func TestCompareValidateAddedIsNonBreaking(t *testing.T) {
	t.Parallel()

	oldEnt := &ir.Entity{Name: "User", Fields: []*ir.Field{{Name: "email"}}}
	newEnt := &ir.Entity{Name: "User", Fields: []*ir.Field{{
		Name:     "email",
		Validate: []ir.Validation{{Kind: ir.ValidationFormat}},
	}}}

	old := &ir.Schema{Modules: []*ir.Module{{Name: "app", Entities: []*ir.Entity{oldEnt}}}}
	newSchema := &ir.Schema{Modules: []*ir.Module{{Name: "app", Entities: []*ir.Entity{newEnt}}}}

	changes := Compare(old, newSchema)
	if len(changes) != 1 || changes[0].Kind != KindValidateAdded || changes[0].Breaking {
		t.Fatalf("changes = %v, want non-breaking KindValidateAdded", changes)
	}
}

func TestCompareOperationHTTPRemovedIsBreaking(t *testing.T) {
	t.Parallel()

	oldOp := &ir.Operation{
		Name:       "GetUser",
		Transports: []ir.Transport{ir.HTTPTransport{Method: "GET", Path: "/v1/users/{id}"}},
	}
	newOp := &ir.Operation{Name: "GetUser"}

	oldSvc := &ir.Service{Name: "UserService", Operations: []*ir.Operation{oldOp}}
	newSvc := &ir.Service{Name: "UserService", Operations: []*ir.Operation{newOp}}

	old := &ir.Schema{Modules: []*ir.Module{{Name: "app", Services: []*ir.Service{oldSvc}}}}
	newSchema := &ir.Schema{Modules: []*ir.Module{{Name: "app", Services: []*ir.Service{newSvc}}}}

	changes := Compare(old, newSchema)
	if len(changes) != 1 || changes[0].Kind != KindOperationHTTPChanged || !changes[0].Breaking {
		t.Fatalf("changes = %v, want breaking KindOperationHTTPChanged", changes)
	}
}

func TestCompareDeterministicAcrossCalls(t *testing.T) {
	t.Parallel()

	old := testSchema(testModule("app"))
	newSchema := testSchema(testModule("billing"))

	first := Compare(old, newSchema)
	second := Compare(old, newSchema)

	if len(first) != len(second) {
		t.Fatalf("change count differs: %d vs %d", len(first), len(second))
	}

	for i := range first {
		if first[i].Kind != second[i].Kind || first[i].Message != second[i].Message {
			t.Fatalf("change %d differs: %+v vs %+v", i, first[i], second[i])
		}
	}
}

func TestHasBreakingEmptyList(t *testing.T) {
	t.Parallel()

	if HasBreaking(nil) {
		t.Fatal("HasBreaking(nil) = true, want false")
	}
}

func strPtr(s string) *string { return &s }
