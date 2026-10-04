package ir

import "testing"

func TestEntityFieldByNameNilReceiver(t *testing.T) {
	t.Parallel()

	var ent *Entity
	if f := ent.FieldByName("x"); f != nil {
		t.Fatalf("nil Entity.FieldByName = %v, want nil", f)
	}
}

func TestEntityFieldByNameEmptyFields(t *testing.T) {
	t.Parallel()

	ent := &Entity{Name: "Empty"}
	if f := ent.FieldByName("x"); f != nil {
		t.Fatalf("FieldByName on fieldless entity = %v, want nil", f)
	}
}

func TestMessageFieldByNameNilReceiver(t *testing.T) {
	t.Parallel()

	var msg *Message
	if f := msg.FieldByName("x"); f != nil {
		t.Fatalf("nil Message.FieldByName = %v, want nil", f)
	}
}

func TestErrorCodeZeroValueDefaults(t *testing.T) {
	t.Parallel()

	var c ErrorCode
	if got := c.HTTPStatus(); got != 500 {
		t.Fatalf("ErrorCode(0).HTTPStatus() = %d, want 500", got)
	}

	if got := c.GRPCName(); got != "UNKNOWN" {
		t.Fatalf("ErrorCode(0).GRPCName() = %q, want \"UNKNOWN\"", got)
	}
}

func TestErrorCodeFullVocabulary(t *testing.T) {
	t.Parallel()

	// Every defined code must map to a distinct, non-empty gRPC name and
	// a plausible HTTP status.
	seen := map[string]bool{}
	for c := ECancelled; c <= EUnauthenticated; c++ {
		if c.GRPCName() == "" {
			t.Fatalf("ErrorCode(%d).GRPCName() empty", int(c))
		}

		if seen[c.GRPCName()] {
			t.Fatalf("duplicate gRPC name %q", c.GRPCName())
		}

		seen[c.GRPCName()] = true

		if status := c.HTTPStatus(); status < 400 || status > 599 {
			t.Fatalf("ErrorCode(%d).HTTPStatus() = %d, want 4xx/5xx", int(c), status)
		}
	}
}

func TestTypeRefZeroValue(t *testing.T) {
	t.Parallel()

	var ref TypeRef
	if ref.IsEntity() || ref.IsMessage() {
		t.Fatal("zero TypeRef reports a type, want neither")
	}

	if got := ref.Name(); got != "" {
		t.Fatalf("zero TypeRef.Name() = %q, want \"\"", got)
	}

	if got := ref.Module(); got != nil {
		t.Fatalf("zero TypeRef.Module() = %v, want nil", got)
	}
}

func TestTypeRefMessageVariant(t *testing.T) {
	t.Parallel()

	msg := &Message{Name: "CreateUserRequest"}
	ref := TypeRef{Message: msg}

	if !ref.IsMessage() || ref.IsEntity() {
		t.Fatal("TypeRef{Message} should report IsMessage only")
	}

	if got := ref.Name(); got != "CreateUserRequest" {
		t.Fatalf("Name() = %q, want %q", got, "CreateUserRequest")
	}
}

func TestParamIsRefNilReceiver(t *testing.T) {
	t.Parallel()

	var p *Param
	if p.IsRef() {
		t.Fatal("nil Param.IsRef() = true, want false")
	}
}

func TestFieldTypeZeroValue(t *testing.T) {
	t.Parallel()

	var ft FieldType
	if ft.Scalar != TUUID {
		t.Fatalf("zero FieldType.Scalar = %d, want TUUID (%d)", ft.Scalar, TUUID)
	}

	if ft.EnumValues != nil || ft.EnumName != "" {
		t.Fatalf("zero FieldType enum parts = %v/%q, want nil/\"\"", ft.EnumValues, ft.EnumName)
	}
}

func TestValidationAndDefaultZeroValues(t *testing.T) {
	t.Parallel()

	var v Validation
	if v.Kind != "" || v.Args != nil {
		t.Fatalf("zero Validation = %+v, want empty kind and nil args", v)
	}

	var d DefaultValue
	if d.Kind != "" || d.Lit != nil {
		t.Fatalf("zero DefaultValue = %+v, want empty kind and nil lit", d)
	}
}

func TestTransportKinds(t *testing.T) {
	t.Parallel()

	var http HTTPTransport
	var grpc GRPCTransport

	if http.Kind() != TransportHTTP || grpc.Kind() != TransportGRPC {
		t.Fatalf("transport kinds = %d/%d, want TransportHTTP/TransportGRPC", http.Kind(), grpc.Kind())
	}
}

func TestScalarTypeZeroValueIsTUUID(t *testing.T) {
	t.Parallel()

	var s ScalarType
	if s != TUUID {
		t.Fatalf("zero ScalarType = %d, want TUUID (%d)", s, TUUID)
	}
}
