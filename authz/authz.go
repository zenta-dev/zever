package authz

import (
	"context"
	"strings"

	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/permission"
)

// Policy declares the authentication and permission requirements for a route.
type Policy struct {
	// AuthRequired enforces bearer token verification when true.
	AuthRequired bool
	// Roles carries the candidate roles evaluated by the permission checker.
	// Ignored when AuthRequired is false: an unauthenticated caller has no
	// verified identity, so it is never evaluated as holding any role --
	// only rules that apply regardless of role (e.g. a public wildcard
	// rule) can allow an anonymous PermissionCheck.
	Roles []string
	// PermissionCheck names the action passed to the checker; empty skips checks.
	PermissionCheck string
	// ResourceType names the resource type passed to the checker.
	ResourceType string
	// OwnerField names the resource's owner field (declared snake_case schema
	// name, e.g. "user_id") when the permission check is an ownership check.
	// Empty for non-ownership checks. Carried for checkers that enforce
	// ownership themselves; see UnaryServerInterceptor.
	OwnerField string
}

// Authorize verifies the token with a and enforces pol with p, returning the verified claims.
func Authorize(ctx context.Context, a auth.Auth, p permission.Checker, pol Policy, token, resourceID string) (auth.Claims, error) {
	var claims auth.Claims

	if !pol.AuthRequired && pol.PermissionCheck == "" {
		return claims, nil
	}

	if pol.AuthRequired {
		if token == "" {
			return claims, &UnauthenticatedError{Reason: "missing bearer token"}
		}

		verified, err := a.Verify(ctx, token)
		if err != nil {
			return claims, &UnauthenticatedError{Reason: "invalid or expired token"}
		}

		claims = verified
	}

	if pol.PermissionCheck == "" {
		return claims, nil
	}

	var roles []string
	if pol.AuthRequired {
		roles = pol.Roles
	}

	subject := permission.Subject{
		ID:         claims.Subject,
		Roles:      roles,
		Attributes: stringAttrs(claims.Custom),
	}

	resource := permission.Resource{
		Type: pol.ResourceType,
		ID:   resourceID,
	}

	decision, err := p.Can(ctx, subject, pol.PermissionCheck, resource)
	if err != nil {
		return claims, &PermissionDeniedError{Reason: "permission check failed"}
	}

	if !decision.Allowed {
		return claims, &PermissionDeniedError{Reason: "permission denied"}
	}

	return claims, nil
}

func stringAttrs(m map[string]any) map[string]string {
	if len(m) == 0 {
		return nil
	}

	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}

	if len(out) == 0 {
		return nil
	}

	return out
}

func parseBearerToken(header string) string {
	const prefix = "Bearer "

	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}

	if len(header) > len(prefix) && strings.EqualFold(header[:len(prefix)], prefix) {
		token := strings.TrimSpace(header[len(prefix):])
		// Tokens never contain whitespace; "Bearer  a  b" must not
		// parse as "a  b" (RFC 6750 b64token).
		if token == "" || strings.ContainsAny(token, " \t") {
			return ""
		}
		return token
	}

	return ""
}

type claimsKey struct{}

func withClaims(ctx context.Context, claims auth.Claims) context.Context {
	return context.WithValue(ctx, claimsKey{}, claims)
}

// ClaimsFromContext returns the claims stored by Middleware or UnaryServerInterceptor.
func ClaimsFromContext(ctx context.Context) (auth.Claims, bool) {
	claims, ok := ctx.Value(claimsKey{}).(auth.Claims)
	return claims, ok
}
