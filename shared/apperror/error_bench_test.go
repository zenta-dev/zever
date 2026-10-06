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

// BenchmarkErrorAccessors measures the Code/Message/Cause accessor path.
func BenchmarkErrorAccessors(b *testing.B) {
	cause := errors.New("boom")
	e := Wrap(Internal, "something broke", cause)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = e.Code()
		_ = e.Message()
		_ = e.Cause()
	}
}

// BenchmarkErrorCodeString measures mapping an ErrorCode to its gRPC name.
func BenchmarkErrorCodeString(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		_ = benchErrorCodes[i%len(benchErrorCodes)].String()
		i++
	}
}

// BenchmarkErrorCodeHTTPStatus measures mapping an ErrorCode to its HTTP status.
func BenchmarkErrorCodeHTTPStatus(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()

	i := 0
	for b.Loop() {
		_ = benchErrorCodes[i%len(benchErrorCodes)].HTTPStatus()
		i++
	}
}
