package apperror

import (
	"errors"
	"testing"
)

var benchErrorCodes = []ErrorCode{
	Cancelled, Unknown, InvalidArgument, DeadlineExceeded,
	NotFound, AlreadyExists, PermissionDenied, ResourceExhausted,
	FailedPrecondition, Aborted, OutOfRange, Unimplemented,
	Internal, Unavailable, DataLoss, Unauthenticated,
}

// BenchmarkNew measures constructing an *Error and formatting it without a cause.
func BenchmarkNew(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = New(NotFound, "no such task").Error()
	}
}

// BenchmarkWrap measures constructing an *Error around a cause and formatting it.
func BenchmarkWrap(b *testing.B) {
	cause := errors.New("boom")

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = Wrap(Internal, "something broke", cause).Error()
	}
}

// BenchmarkErrorCodeString measures mapping an ErrorCode to its gRPC name.
func BenchmarkErrorCodeString(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		_ = benchErrorCodes[i%len(benchErrorCodes)].String()
	}
}

// BenchmarkErrorCodeHTTPStatus measures mapping an ErrorCode to its HTTP status.
func BenchmarkErrorCodeHTTPStatus(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	for i := range b.N {
		_ = benchErrorCodes[i%len(benchErrorCodes)].HTTPStatus()
	}
}
