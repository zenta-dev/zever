package zenorm

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

func TestGoName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"id", "ID"},
		{"email", "Email"},
		{"created_at", "CreatedAt"},
		{"user_id", "UserID"},
		{"amount_cents", "AmountCents"},
		{"url", "URL"},
		{"uuid", "UUID"},
		{"ip_address", "IPAddress"},
		{"metadata", "Metadata"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := goName(tt.input); got != tt.want {
				t.Fatalf("goName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGoScalar(t *testing.T) {
	tests := []struct {
		name       string
		ft         ir.FieldType
		wantType   string
		wantImport string
	}{
		{"uuid", ir.FieldType{Scalar: ir.TUUID}, "string", ""},
		{"string", ir.FieldType{Scalar: ir.TString}, "string", ""},
		{"int32", ir.FieldType{Scalar: ir.TInt32}, "int32", ""},
		{"int64", ir.FieldType{Scalar: ir.TInt64}, "int64", ""},
		{"float32", ir.FieldType{Scalar: ir.TFloat32}, "float32", ""},
		{"float64", ir.FieldType{Scalar: ir.TFloat64}, "float64", ""},
		{"bool", ir.FieldType{Scalar: ir.TBool}, "bool", ""},
		{"timestamp", ir.FieldType{Scalar: ir.TTimestamp}, "time.Time", "time"},
		{"date", ir.FieldType{Scalar: ir.TDate}, "time.Time", "time"},
		{"bytes", ir.FieldType{Scalar: ir.TBytes}, "[]byte", ""},
		{"json", ir.FieldType{Scalar: ir.TJSON}, "orm.JSONText", ""},
		{"inline enum", ir.FieldType{Scalar: ir.TEnum}, "string", ""},
		{"named enum", ir.FieldType{Scalar: ir.TEnum, EnumName: "status"}, "Status", ""},
		{"unknown", ir.FieldType{Scalar: ir.ScalarType(99)}, "any", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotType, gotImport := goScalar(tt.ft)
			if gotType != tt.wantType || gotImport != tt.wantImport {
				t.Fatalf("goScalar(%v) = (%q, %q), want (%q, %q)",
					tt.ft, gotType, gotImport, tt.wantType, tt.wantImport)
			}
		})
	}
}

func TestPrimaryField(t *testing.T) {
	if got := primaryField(nil); got != nil {
		t.Fatalf("primaryField(nil) = %v, want nil", got)
	}

	withoutPK := &ir.Entity{Name: "Widget", Fields: []*ir.Field{{Name: "name"}}}
	if got := primaryField(withoutPK); got != nil {
		t.Fatalf("primaryField without @primary = %v, want nil", got)
	}

	withPK := &ir.Entity{Name: "Widget", Fields: []*ir.Field{
		{Name: "name"},
		{Name: "id", Primary: true},
	}}
	if got := primaryField(withPK); got == nil || got.Name != "id" {
		t.Fatalf("primaryField with @primary = %v, want the id field", got)
	}
}

// relationFixture builds an owner/target entity pair sharing one module,
// with single @primary id fields on both sides.
func relationFixture() (owner, target *ir.Entity) {
	mod := &ir.Module{Name: "app"}

	owner = &ir.Entity{Name: "User", Module: mod, Fields: []*ir.Field{{Name: "id", Primary: true}}}
	target = &ir.Entity{Name: "Order", Module: mod, Fields: []*ir.Field{
		{Name: "id", Primary: true},
		{Name: "user_id"},
	}}

	return owner, target
}

func TestNewRelationModelSkips(t *testing.T) {
	owner, target := relationFixture()
	fk := target.Fields[1]

	newRel := func(kind ir.RelationKind, relTarget *ir.Entity, relFK *ir.Field, field string) *ir.Relation {
		return &ir.Relation{Kind: kind, FieldName: field, Target: relTarget, ForeignKey: relFK}
	}

	t.Run("has_one resolves keys", func(t *testing.T) {
		rm, ok := newRelationModel(owner, newRel(ir.HasOne, target, fk, "order"))
		if !ok {
			t.Fatal("newRelationModel(has_one) = false, want true")
		}

		if rm.ParentKeyCol != "id" || rm.ChildKeyCol != "user_id" {
			t.Fatalf("has_one keys = %q/%q, want id/user_id", rm.ParentKeyCol, rm.ChildKeyCol)
		}

		if rm.GoName != "Order" || rm.TargetType != "Order" || rm.TargetTableVar != "Orders" {
			t.Fatalf("has_one naming = %+v, want Order/Order/Orders", rm)
		}
	})

	t.Run("many_to_many skipped", func(t *testing.T) {
		if _, ok := newRelationModel(owner, newRel(ir.ManyToMany, target, fk, "tags")); ok {
			t.Fatal("newRelationModel(many_to_many) = true, want false")
		}
	})

	t.Run("nil target skipped", func(t *testing.T) {
		if _, ok := newRelationModel(owner, newRel(ir.HasMany, nil, fk, "orders")); ok {
			t.Fatal("newRelationModel(nil target) = true, want false")
		}
	})

	t.Run("nil foreign key skipped", func(t *testing.T) {
		if _, ok := newRelationModel(owner, newRel(ir.HasMany, target, nil, "orders")); ok {
			t.Fatal("newRelationModel(nil foreign key) = true, want false")
		}
	})

	t.Run("cross-module skipped", func(t *testing.T) {
		other := &ir.Module{Name: "billing"}
		foreign := &ir.Entity{Name: "Invoice", Module: other, Fields: []*ir.Field{{Name: "id", Primary: true}}}

		if _, ok := newRelationModel(owner, newRel(ir.HasMany, foreign, fk, "invoices")); ok {
			t.Fatal("newRelationModel(cross-module) = true, want false")
		}
	})

	t.Run("has_many without owner pk skipped", func(t *testing.T) {
		pkless := &ir.Entity{Name: "Log", Module: owner.Module, Fields: []*ir.Field{{Name: "msg"}}}

		if _, ok := newRelationModel(pkless, newRel(ir.HasMany, target, fk, "orders")); ok {
			t.Fatal("newRelationModel(has_many, owner without pk) = true, want false")
		}
	})

	t.Run("belongs_to without target pk skipped", func(t *testing.T) {
		pkless := &ir.Entity{Name: "Log", Module: owner.Module, Fields: []*ir.Field{{Name: "msg"}}}

		if _, ok := newRelationModel(owner, newRel(ir.BelongsTo, pkless, owner.Fields[0], "log")); ok {
			t.Fatal("newRelationModel(belongs_to, target without pk) = true, want false")
		}
	})

	t.Run("unknown kind skipped", func(t *testing.T) {
		if _, ok := newRelationModel(owner, newRel(ir.RelationKind(99), target, fk, "orders")); ok {
			t.Fatal("newRelationModel(unknown kind) = true, want false")
		}
	})
}

func TestFillRelationKeysRejectsManyToMany(t *testing.T) {
	owner, _ := relationFixture()

	rm := relationModel{}

	if fillRelationKeys(owner, &ir.Relation{Kind: ir.ManyToMany}, &rm) {
		t.Fatal("fillRelationKeys(many_to_many) = true, want false")
	}

	if fillRelationKeys(owner, &ir.Relation{Kind: ir.RelationKind(99)}, &rm) {
		t.Fatal("fillRelationKeys(unknown kind) = true, want false")
	}
}
