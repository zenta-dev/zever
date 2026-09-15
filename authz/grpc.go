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

		claims, err := Authorize(ctx, a, p, pol, token, "")
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
