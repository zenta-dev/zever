package ir

import "testing"

// TestErrorCodeHTTPStatusAndGRPCName is an exhaustive table test covering
// all sixteen ErrorCode values, so a future addition to the vocabulary
// fails this test loudly (unhandled default) rather than silently falling
// through to a wrong HTTPStatus/GRPCName.
func TestErrorCodeHTTPStatusAndGRPCName(t *testing.T) {
	tests := []struct {
		code       ErrorCode
		wantStatus int
		wantName   string
	}{
		{ECancelled, 499, "CANCELLED"},
		{EUnknown, 500, "UNKNOWN"},
		{EInvalidArgument, 400, "INVALID_ARGUMENT"},
		{EDeadlineExceeded, 504, "DEADLINE_EXCEEDED"},
		{ENotFound, 404, "NOT_FOUND"},
		{EAlreadyExists, 409, "ALREADY_EXISTS"},
		{EPermissionDenied, 403, "PERMISSION_DENIED"},
		{EUnauthenticated, 401, "UNAUTHENTICATED"},
		{EResourceExhausted, 429, "RESOURCE_EXHAUSTED"},
		{EFailedPrecondition, 400, "FAILED_PRECONDITION"},
		{EAborted, 409, "ABORTED"},
		{EOutOfRange, 400, "OUT_OF_RANGE"},
		{EUnimplemented, 501, "UNIMPLEMENTED"},
		{EInternal, 500, "INTERNAL"},
		{EUnavailable, 503, "UNAVAILABLE"},
		{EDataLoss, 500, "DATA_LOSS"},
	}

	if len(tests) != 16 {
		t.Fatalf("expected exactly 16 table rows (the full gRPC-canonical vocabulary), got %d", len(tests))
	}

	seen := make(map[ErrorCode]bool, len(tests))

	for _, tt := range tests {
		t.Run(tt.wantName, func(t *testing.T) {
			if got := tt.code.HTTPStatus(); got != tt.wantStatus {
				t.Errorf("HTTPStatus() = %d, want %d", got, tt.wantStatus)
			}

			if got := tt.code.GRPCName(); got != tt.wantName {
				t.Errorf("GRPCName() = %q, want %q", got, tt.wantName)
			}

			if seen[tt.code] {
				t.Fatalf("ErrorCode %d (%s) duplicated in table", tt.code, tt.wantName)
			}

			seen[tt.code] = true
		})
	}
}

func TestEntityFieldByName(t *testing.T) {
	tests := []struct {
		name          string
		entity        *Entity
		fieldName     string
		wantFound     bool
		wantFieldName string
	}{
		{
			name: "field found",
			entity: &Entity{
				Name: "User",
				Fields: []*Field{
					{Name: "id", Type: FieldType{Scalar: TUUID}},
					{Name: "email", Type: FieldType{Scalar: TString}},
					{Name: "age", Type: FieldType{Scalar: TInt32}},
				},
			},
			fieldName:     "email",
			wantFound:     true,
			wantFieldName: "email",
		},
		{
			name: "field not found",
			entity: &Entity{
				Name: "User",
				Fields: []*Field{
					{Name: "id", Type: FieldType{Scalar: TUUID}},
					{Name: "email", Type: FieldType{Scalar: TString}},
				},
			},
			fieldName: "nonexistent",
			wantFound: false,
		},
		{
			name: "empty fields slice",
			entity: &Entity{
				Name:   "Empty",
				Fields: []*Field{},
			},
			fieldName: "anyfield",
			wantFound: false,
		},
		{
			name:      "nil entity",
			entity:    nil,
			fieldName: "anyfield",
			wantFound: false,
		},
		{
			name: "nil fields",
			entity: &Entity{
				Name:   "NoFields",
				Fields: nil,
			},
			fieldName: "anyfield",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.entity.FieldByName(tt.fieldName)
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
