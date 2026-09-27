package ir

import "testing"

// TestMessageFieldByName mirrors TestEntityFieldByName for messages, closing
// the only uncovered method in ir_message.go.
func TestMessageFieldByName(t *testing.T) {
	tests := []struct {
		name          string
		message       *Message
		fieldName     string
		wantFound     bool
		wantFieldName string
	}{
		{
			name: "field found",
			message: &Message{
				Name: "Greeting",
				Fields: []*Field{
					{Name: "text", Type: FieldType{Scalar: TString}},
					{Name: "lang", Type: FieldType{Scalar: TString}},
				},
			},
			fieldName:     "lang",
			wantFound:     true,
			wantFieldName: "lang",
		},
		{
			name: "field not found",
			message: &Message{
				Name: "Greeting",
				Fields: []*Field{
					{Name: "text", Type: FieldType{Scalar: TString}},
				},
			},
			fieldName: "nonexistent",
			wantFound: false,
		},
		{
			name: "empty fields slice",
			message: &Message{
				Name:   "Empty",
				Fields: []*Field{},
			},
			fieldName: "anyfield",
			wantFound: false,
		},
		{
			name:      "nil message",
			message:   nil,
			fieldName: "anyfield",
			wantFound: false,
		},
		{
			name: "nil fields",
			message: &Message{
				Name:   "NoFields",
				Fields: nil,
			},
			fieldName: "anyfield",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.message.FieldByName(tt.fieldName)
			if !tt.wantFound {
				if got != nil {
					t.Fatalf("FieldByName(%q) = %v, want nil", tt.fieldName, got)
				}

				return
			}

			if got == nil {
				t.Fatalf("FieldByName(%q) = nil, want field named %q", tt.fieldName, tt.wantFieldName)
			}

			if got.Name != tt.wantFieldName {
				t.Fatalf("FieldByName(%q).Name = %q, want %q", tt.fieldName, got.Name, tt.wantFieldName)
			}
		})
	}
}

func TestParamIsRef(t *testing.T) {
	var nilParam *Param

	if nilParam.IsRef() {
		t.Fatalf("nil param IsRef() = true, want false")
	}

	scalarParam := Param{Type: FieldType{Scalar: TString}}
	if scalarParam.IsRef() {
		t.Fatalf("scalar param IsRef() = true, want false")
	}

	refParam := Param{Ref: &TypeRef{}}
	if !refParam.IsRef() {
		t.Fatalf("ref param IsRef() = false, want true")
	}
}

func TestTransportKind(t *testing.T) {
	if (HTTPTransport{}).Kind() != TransportHTTP {
		t.Fatalf("HTTPTransport.Kind() != TransportHTTP")
	}

	if (GRPCTransport{}).Kind() != TransportGRPC {
		t.Fatalf("GRPCTransport.Kind() != TransportGRPC")
	}
}

func TestTypeRefHelpers(t *testing.T) {
	mod := &Module{Name: "billing"}
	ent := &Entity{Name: "Invoice", Module: mod}
	msg := &Message{Name: "Receipt", Module: mod}

	entityRef := TypeRef{Entity: ent}
	messageRef := TypeRef{Message: msg}
	emptyRef := TypeRef{}

	if !entityRef.IsEntity() || entityRef.IsMessage() {
		t.Fatalf("entity ref IsEntity/IsMessage wrong: %+v", entityRef)
	}

	if !messageRef.IsMessage() || messageRef.IsEntity() {
		t.Fatalf("message ref IsEntity/IsMessage wrong: %+v", messageRef)
	}

	if emptyRef.IsEntity() || emptyRef.IsMessage() {
		t.Fatalf("empty ref reports a side: %+v", emptyRef)
	}

	if entityRef.Name() != "Invoice" {
		t.Fatalf("entity ref Name() = %q, want Invoice", entityRef.Name())
	}

	if messageRef.Name() != "Receipt" {
		t.Fatalf("message ref Name() = %q, want Receipt", messageRef.Name())
	}

	if emptyRef.Name() != "" {
		t.Fatalf("empty ref Name() = %q, want empty", emptyRef.Name())
	}

	if entityRef.Module() != mod {
		t.Fatalf("entity ref Module() wrong")
	}

	if messageRef.Module() != mod {
		t.Fatalf("message ref Module() wrong")
	}

	if emptyRef.Module() != nil {
		t.Fatalf("empty ref Module() = %v, want nil", emptyRef.Module())
	}
}

// TestErrorCodeDefaults covers the defensive default branches of HTTPStatus
// and GRPCName for values outside the 1-16 vocabulary.
func TestErrorCodeDefaults(t *testing.T) {
	for _, code := range []ErrorCode{0, -1, 99} {
		if got := code.HTTPStatus(); got != 500 {
			t.Errorf("ErrorCode(%d).HTTPStatus() = %d, want 500", int(code), got)
		}

		if got := code.GRPCName(); got != "UNKNOWN" {
			t.Errorf("ErrorCode(%d).GRPCName() = %q, want UNKNOWN", int(code), got)
		}
	}
}
