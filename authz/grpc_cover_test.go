package authz

import (
	"errors"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestCoverGrpcStatusErrorInternalFallback(t *testing.T) {
	t.Parallel()

	err := grpcStatusError(errors.New("boom"))
	s, ok := status.FromError(err)
	if !ok {
		t.Fatalf("grpcStatusError() = %T (via status.FromError ok=%v), want *status.Status", err, ok)
	}
	if s.Code() != codes.Internal {
		t.Fatalf("Code() = %v, want Internal", s.Code())
	}
	if s.Message() != "internal error" {
		t.Fatalf("Message() = %q, want %q (redacted, no internals leaked)", s.Message(), "internal error")
	}
}
