package apperror

import (
	"fmt"
	"testing"
)

func TestErrorCode_String(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code ErrorCode
		want string
	}{
		{"Cancelled", Cancelled, "CANCELLED"},
		{"Unknown", Unknown, "UNKNOWN"},
		{"InvalidArgument", InvalidArgument, "INVALID_ARGUMENT"},
		{"DeadlineExceeded", DeadlineExceeded, "DEADLINE_EXCEEDED"},
		{"NotFound", NotFound, "NOT_FOUND"},
		{"AlreadyExists", AlreadyExists, "ALREADY_EXISTS"},
		{"PermissionDenied", PermissionDenied, "PERMISSION_DENIED"},
		{"ResourceExhausted", ResourceExhausted, "RESOURCE_EXHAUSTED"},
		{"FailedPrecondition", FailedPrecondition, "FAILED_PRECONDITION"},
		{"Aborted", Aborted, "ABORTED"},
		{"OutOfRange", OutOfRange, "OUT_OF_RANGE"},
		{"Unimplemented", Unimplemented, "UNIMPLEMENTED"},
		{"Internal", Internal, "INTERNAL"},
		{"Unavailable", Unavailable, "UNAVAILABLE"},
		{"DataLoss", DataLoss, "DATA_LOSS"},
		{"Unauthenticated", Unauthenticated, "UNAUTHENTICATED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.code.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestErrorCode_String_unknownDefaults(t *testing.T) {
	t.Parallel()

	for _, code := range []ErrorCode{0, 99, -1} {
		code := code
		t.Run(fmt.Sprintf("code_%d", int(code)), func(t *testing.T) {
			t.Parallel()
			if got := code.String(); got != "UNKNOWN" {
				t.Errorf("String() = %q, want %q", got, "UNKNOWN")
			}
		})
	}
}

func TestErrorCode_HTTPStatus(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code ErrorCode
		want int
	}{
		{"Cancelled", Cancelled, 499},
		{"Unknown", Unknown, 500},
		{"InvalidArgument", InvalidArgument, 400},
		{"DeadlineExceeded", DeadlineExceeded, 504},
		{"NotFound", NotFound, 404},
		{"AlreadyExists", AlreadyExists, 409},
		{"PermissionDenied", PermissionDenied, 403},
		{"ResourceExhausted", ResourceExhausted, 429},
		{"FailedPrecondition", FailedPrecondition, 400},
		{"Aborted", Aborted, 409},
		{"OutOfRange", OutOfRange, 400},
		{"Unimplemented", Unimplemented, 501},
		{"Internal", Internal, 500},
		{"Unavailable", Unavailable, 503},
		{"DataLoss", DataLoss, 500},
		{"Unauthenticated", Unauthenticated, 401},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.code.HTTPStatus(); got != tt.want {
				t.Errorf("HTTPStatus() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestErrorCode_HTTPStatus_unknownDefaults500(t *testing.T) {
	t.Parallel()

	for _, code := range []ErrorCode{0, 99, -1} {
		code := code
		t.Run(fmt.Sprintf("code_%d", int(code)), func(t *testing.T) {
			t.Parallel()
			if got := code.HTTPStatus(); got != 500 {
				t.Errorf("HTTPStatus() = %d, want %d", got, 500)
			}
		})
	}
}

func TestErrorCode_ordinals(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code ErrorCode
		want int
	}{
		{"Cancelled", Cancelled, 1},
		{"Unknown", Unknown, 2},
		{"InvalidArgument", InvalidArgument, 3},
		{"DeadlineExceeded", DeadlineExceeded, 4},
		{"NotFound", NotFound, 5},
		{"AlreadyExists", AlreadyExists, 6},
		{"PermissionDenied", PermissionDenied, 7},
		{"ResourceExhausted", ResourceExhausted, 8},
		{"FailedPrecondition", FailedPrecondition, 9},
		{"Aborted", Aborted, 10},
		{"OutOfRange", OutOfRange, 11},
		{"Unimplemented", Unimplemented, 12},
		{"Internal", Internal, 13},
		{"Unavailable", Unavailable, 14},
		{"DataLoss", DataLoss, 15},
		{"Unauthenticated", Unauthenticated, 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := int(tt.code); got != tt.want {
				t.Errorf("ordinal = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestErrorCode_HTTPStatus_sharingGroups(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		codes []ErrorCode
		want  int
	}{
		{"BadRequest", []ErrorCode{InvalidArgument, FailedPrecondition, OutOfRange}, 400},
		{"Conflict", []ErrorCode{AlreadyExists, Aborted}, 409},
		{"InternalServerError", []ErrorCode{Unknown, Internal, DataLoss}, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			for _, code := range tt.codes {
				if got := code.HTTPStatus(); got != tt.want {
					t.Errorf("%v.HTTPStatus() = %d, want %d", code, got, tt.want)
				}
			}
		})
	}
}
