package apperror

import "testing"

// TestErrorCode_outOfRange covers codes outside the closed vocabulary.
func TestErrorCode_outOfRange(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		code       ErrorCode
		wantString string
		wantStatus int
	}{
		{"zero", ErrorCode(0), "UNKNOWN", 500},
		{"negative", ErrorCode(-1), "UNKNOWN", 500},
		{"beyond", ErrorCode(17), "UNKNOWN", 500},
		{"huge", ErrorCode(1 << 20), "UNKNOWN", 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.code.String(); got != tt.wantString {
				t.Errorf("String() = %q, want %q", got, tt.wantString)
			}

			if got := tt.code.HTTPStatus(); got != tt.wantStatus {
				t.Errorf("HTTPStatus() = %d, want %d", got, tt.wantStatus)
			}
		})
	}
}
