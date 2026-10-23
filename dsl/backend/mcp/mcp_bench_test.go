package mcp

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

// BenchmarkRenderService measures the service rendering hot path.
func BenchmarkRenderService(b *testing.B) {
	m := &ir.Module{Name: "test"}
	svc := &ir.Service{
		Name: "TestService",
		Operations: []*ir.Operation{
			{Name: "Get", Params: []*ir.Param{{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}}}, Returns: &ir.TypeRef{}},
			{Name: "List", Params: []*ir.Param{{Name: "limit", Type: ir.FieldType{Scalar: ir.TInt32}}}, Returns: &ir.TypeRef{}},
			{Name: "Create", Params: []*ir.Param{{Name: "data", Type: ir.FieldType{Scalar: ir.TJSON}}}, Returns: &ir.TypeRef{}},
			{Name: "Update", Params: []*ir.Param{{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}}, {Name: "data", Type: ir.FieldType{Scalar: ir.TJSON}}}, Returns: &ir.TypeRef{}},
			{Name: "Delete", Params: []*ir.Param{{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}}}, Returns: nil},
		},
	}
	enums := map[string][]string{}
	enumOwners := map[string]string{}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = renderService(m, svc, enums, enumOwners)
	}
}

// BenchmarkRenderOperation measures the operation rendering hot path.
func BenchmarkRenderOperation(b *testing.B) {
	m := &ir.Module{Name: "test"}
	svc := &ir.Service{Name: "TestService"}
	op := &ir.Operation{
		Name: "Get",
		Params: []*ir.Param{
			{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}},
			{Name: "include", Type: ir.FieldType{Scalar: ir.TString}},
		},
		Returns:   &ir.TypeRef{Entity: &ir.Entity{Name: "User", Fields: []*ir.Field{{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}}, {Name: "name", Type: ir.FieldType{Scalar: ir.TString}}}}},
		Paginated: false,
	}
	r := &renderer{
		enums:      map[string][]string{},
		enumOwners: map[string]string{},
		defs:       map[string]map[string]any{},
		seen:       map[string]bool{},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = r.renderOperation(m, svc, op)
	}
}

// BenchmarkParamSchema measures the parameter schema rendering.
func BenchmarkParamSchema(b *testing.B) {
	r := &renderer{
		enums:      map[string][]string{},
		enumOwners: map[string]string{},
		defs:       map[string]map[string]any{},
		seen:       map[string]bool{},
	}
	m := &ir.Module{Name: "test"}
	p := &ir.Param{
		Name: "filter",
		Ref:  &ir.TypeRef{Message: &ir.Message{Name: "Filter", Fields: []*ir.Field{{Name: "name", Type: ir.FieldType{Scalar: ir.TString}}, {Name: "age", Type: ir.FieldType{Scalar: ir.TInt32}}}}},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = r.paramSchema(m, p)
	}
}

// BenchmarkFieldSchema measures the field schema rendering for scalars.
func BenchmarkFieldSchema(b *testing.B) {
	r := &renderer{
		enums:      map[string][]string{},
		enumOwners: map[string]string{},
		defs:       map[string]map[string]any{},
		seen:       map[string]bool{},
	}
	m := &ir.Module{Name: "test"}
	scalars := []ir.ScalarType{ir.TUUID, ir.TString, ir.TInt32, ir.TInt64, ir.TFloat32, ir.TFloat64, ir.TBool, ir.TTimestamp, ir.TDate, ir.TBytes}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = r.fieldSchema(m, ir.FieldType{Scalar: scalars[i%len(scalars)]})
		i++
	}
}

// BenchmarkFieldSchema_Enum measures the enum field schema rendering.
func BenchmarkFieldSchema_Enum(b *testing.B) {
	r := &renderer{
		enums:      map[string][]string{"Status": {"active", "inactive", "pending"}},
		enumOwners: map[string]string{"Status": "test"},
		defs:       map[string]map[string]any{},
		seen:       map[string]bool{},
	}
	m := &ir.Module{Name: "test"}
	ft := ir.FieldType{Scalar: ir.TEnum, EnumName: "Status"}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = r.fieldSchema(m, ft)
	}
}

// BenchmarkTypeRefSchema_Entity measures the entity type reference rendering.
func BenchmarkTypeRefSchema_Entity(b *testing.B) {
	r := &renderer{
		enums:      map[string][]string{},
		enumOwners: map[string]string{},
		defs:       map[string]map[string]any{},
		seen:       map[string]bool{},
	}
	m := &ir.Module{Name: "test"}
	ref := &ir.TypeRef{
		Entity: &ir.Entity{
			Name: "User",
			Fields: []*ir.Field{
				{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}, Primary: true},
				{Name: "name", Type: ir.FieldType{Scalar: ir.TString}},
				{Name: "email", Type: ir.FieldType{Scalar: ir.TString}},
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = r.typeRefSchema(m, ref)
	}
}

// BenchmarkTypeRefSchema_Message measures the message type reference rendering.
func BenchmarkTypeRefSchema_Message(b *testing.B) {
	r := &renderer{
		enums:      map[string][]string{},
		enumOwners: map[string]string{},
		defs:       map[string]map[string]any{},
		seen:       map[string]bool{},
	}
	m := &ir.Module{Name: "test"}
	ref := &ir.TypeRef{
		Message: &ir.Message{
			Name: "CreateUser",
			Fields: []*ir.Field{
				{Name: "name", Type: ir.FieldType{Scalar: ir.TString}},
				{Name: "email", Type: ir.FieldType{Scalar: ir.TString}},
			},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = r.typeRefSchema(m, ref)
	}
}

// BenchmarkEntitySchema measures the entity schema rendering.
func BenchmarkEntitySchema(b *testing.B) {
	r := &renderer{
		enums:      map[string][]string{},
		enumOwners: map[string]string{},
		defs:       map[string]map[string]any{},
		seen:       map[string]bool{},
	}
	m := &ir.Module{Name: "test"}
	e := &ir.Entity{
		Name: "User",
		Fields: []*ir.Field{
			{Name: "id", Type: ir.FieldType{Scalar: ir.TUUID}, Primary: true},
			{Name: "name", Type: ir.FieldType{Scalar: ir.TString}},
			{Name: "email", Type: ir.FieldType{Scalar: ir.TString}},
			{Name: "created_at", Type: ir.FieldType{Scalar: ir.TTimestamp}},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = r.entitySchema(m, e)
	}
}

// BenchmarkMessageSchema measures the message schema rendering.
func BenchmarkMessageSchema(b *testing.B) {
	r := &renderer{
		enums:      map[string][]string{},
		enumOwners: map[string]string{},
		defs:       map[string]map[string]any{},
		seen:       map[string]bool{},
	}
	m := &ir.Module{Name: "test"}
	msg := &ir.Message{
		Name: "CreateUser",
		Fields: []*ir.Field{
			{Name: "name", Type: ir.FieldType{Scalar: ir.TString}},
			{Name: "email", Type: ir.FieldType{Scalar: ir.TString}},
		},
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = r.messageSchema(m, msg)
	}
}

// BenchmarkQualify measures the definition name qualification.
func BenchmarkQualify(b *testing.B) {
	m := &ir.Module{Name: "user"}
	names := []string{"User", "CreateUser", "UpdateUser", "DeleteUser", "ListUsers"}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = qualify(m, names[i%len(names)])
		i++
	}
}

// BenchmarkScalarJSONType measures the scalar to JSON type mapping.
func BenchmarkScalarJSONType(b *testing.B) {
	scalars := []ir.ScalarType{ir.TUUID, ir.TString, ir.TInt32, ir.TInt64, ir.TFloat32, ir.TFloat64, ir.TBool, ir.TTimestamp, ir.TDate, ir.TBytes, ir.TEnum, ir.TJSON}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = scalarJSONType(scalars[i%len(scalars)])
		i++
	}
}

// BenchmarkModuleLabel measures the module label generation.
func BenchmarkModuleLabel(b *testing.B) {
	modules := []*ir.Module{
		{Name: "user"},
		{Name: "order"},
		{Name: "payment"},
		{Name: "inventory"},
		{Name: "notification"},
	}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = moduleLabel(modules[i%len(modules)])
		i++
	}
}

// BenchmarkOwnerModule measures the owner module resolution.
func BenchmarkOwnerModule(b *testing.B) {
	m := &ir.Module{Name: "test"}
	refs := []*ir.TypeRef{
		{Entity: &ir.Entity{Name: "User", Module: &ir.Module{Name: "user"}}},
		{Message: &ir.Message{Name: "CreateUser", Module: &ir.Module{Name: "user"}}},
		{Entity: &ir.Entity{Name: "Order"}},
		{Message: &ir.Message{Name: "UpdateOrder"}},
	}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = ownerModule(m, refs[i%len(refs)])
		i++
	}
}
