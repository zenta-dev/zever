package authz

import (
	"context"
	"strings"

	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/permission"
)

// Policy declares the authentication and permission requirements for a route.
type Policy struct {
	// AuthRequired enforces bearer token verification when true.
	AuthRequired bool
	// Roles carries the candidate roles evaluated by the permission checker.
	// Ignored when AuthRequired is false: an unauthenticated caller has no
	// verified identity, so it is never evaluated as holding any role.
	// An anonymous PermissionCheck therefore denies in the shipped
	// checkers (rbac requires a non-empty subject ID with at least one
	// role even for wildcard-role rules; casbin groupings never match an
	// empty subject): public routes must set AuthRequired false with an
	// empty PermissionCheck, never rely on a wildcard rule to allow
	// anonymous callers.
	Roles []string
	// PermissionCheck names the action passed to the checker; empty skips checks.
	PermissionCheck string
	// ResourceType names the resource type passed to the checker.
	ResourceType string
	// OwnerField names the resource's owner field (declared snake_case schema
	// name, e.g. "user_id") when the permission check is an ownership check.
	// Empty for non-ownership checks. Opt-in wiring: when non-empty,
	// Authorize populates the permission.Resource Attributes with
	// {OwnerField: resourceID}, so ownership-scoped (OwnedOnly) rules can
	// match. This is correct only when the owner identity equals the
	// resource ID (e.g. /users/{id}); resources whose owner differs from
	// their ID need the owner resolved server-side before the check --
	// passing a foreign ID here would misattribute ownership, so that
	// resolution is a behavior change left for maintainer review (see
	// UnaryServerInterceptor). Empty (default) preserves the previous
	// fail-closed behavior: Attributes stays empty and OwnedOnly rules deny.
	OwnerField string
}

// Authorize verifies the token with a and enforces pol with p, returning the verified claims.
// Fail-closed on nil backends: a nil Auth with AuthRequired denies as
// unauthenticated, and a nil Checker with a PermissionCheck denies,
// instead of panicking.
func Authorize(ctx context.Context, a auth.Auth, p permission.Checker, pol Policy, token, resourceID string) (auth.Claims, error) {
	var claims auth.Claims

	if !pol.AuthRequired && pol.PermissionCheck == "" {
		return claims, nil
	}

	if pol.AuthRequired {
		if token == "" {
			return claims, &UnauthenticatedError{Reason: "missing bearer token"}
		}
		if a == nil {
			return claims, &UnauthenticatedError{Reason: "missing authenticator"}
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

	if p == nil {
		return claims, &PermissionDeniedError{Reason: "permission check failed"}
	}

	var roles []string
	if pol.AuthRequired {
		roles = append([]string(nil), pol.Roles...)
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
	if pol.OwnerField != "" {
		resource.Attributes = map[string]string{pol.OwnerField: resourceID}
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
	// Clone so handlers holding ClaimsFromContext results cannot mutate
	// the stored copy (or each other's) through the shared Custom map.
	return context.WithValue(ctx, claimsKey{}, claims.Clone())
}

// ClaimsFromContext returns the claims stored by Middleware or UnaryServerInterceptor.
// The returned Custom map is a copy; mutating it never affects later lookups.
func ClaimsFromContext(ctx context.Context) (auth.Claims, bool) {
	claims, ok := ctx.Value(claimsKey{}).(auth.Claims)
	if !ok {
		return auth.Claims{}, false
	}
	return claims.Clone(), true
}
