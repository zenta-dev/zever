package authz

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/permission"
)

// BearerTokenFromMD extracts the bearer token from gRPC metadata, returning empty on any malformed input.
func BearerTokenFromMD(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}

	values := md.Get("authorization")
	if len(values) == 0 {
		return ""
	}

	return parseBearerToken(values[0])
}

// ResourceIDer is implemented by request messages carrying the target
// resource id in an Id field -- the protogogen convention for
// Get/Delete-by-id requests. UnaryServerInterceptor uses it to populate
// the permission check's resource id; requests without it evaluate with
// an empty id, exactly as before.
type ResourceIDer interface {
	GetId() string
}

// UnaryServerInterceptor enforces per-method policies, passing requests without a policy straight to the handler.
// For a method with a PermissionCheck, the resource id is read from the
// request when it implements ResourceIDer, and is "" otherwise.
func UnaryServerInterceptor(a auth.Auth, p permission.Checker, policies map[string]Policy) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		pol, ok := policies[info.FullMethod]
		if !ok {
			return handler(ctx, req)
		}

		token := BearerTokenFromMD(ctx)

		var resourceID string
		if pol.PermissionCheck != "" {
			if r, ok := req.(ResourceIDer); ok {
				resourceID = r.GetId()
			}
		}

		claims, err := Authorize(ctx, a, p, pol, token, resourceID)
		if err != nil {
			return nil, grpcStatusError(err)
		}

		return handler(withClaims(ctx, claims), req)
	}
}

func grpcStatusError(err error) error {
	var unauthenticated *UnauthenticatedError
	var denied *PermissionDeniedError

	switch {
	case errors.As(err, &unauthenticated):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.As(err, &denied):
		return status.Error(codes.PermissionDenied, err.Error())
	default:
		// Defensive: Authorize only returns typed errors, so this
		// branch is unreachable today. Kept per always-check-errors;
		// the message is redacted to avoid leaking internals.
		return status.Error(codes.Internal, "internal error")
	}
}
