package authz

import (
	"errors"
	"net/http"

	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/codec"
	"github.com/zenta-dev/zever/permission"
)

// authzErrorBody is the JSON error body written by writeAuthzError.
type authzErrorBody struct {
	Error string `json:"error"`
}

var authzErrorCodec = codec.JSONCodec[authzErrorBody]{}

// BearerToken extracts the bearer token from r, returning empty on any malformed input.
func BearerToken(r *http.Request) string {
	if r == nil {
		return ""
	}

	// Multiple Authorization headers fail closed: Header.Get would
	// silently return only the first (RFC 6750 forbids more than one
	// auth method per request).
	if len(r.Header.Values("Authorization")) != 1 {
		return ""
	}

	return parseBearerToken(r.Header.Get("Authorization"))
}

// Middleware wraps next with Bearer authentication and permission
// enforcement. resourceIDFromRequest may be nil.
func Middleware(a auth.Auth, p permission.Checker, pol Policy, resourceIDFromRequest func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := BearerToken(r)

			resourceID := ""
			if resourceIDFromRequest != nil {
				resourceID = resourceIDFromRequest(r)
			}

			ctx := r.Context()

			claims, err := Authorize(ctx, a, p, pol, token, resourceID)
			if err != nil {
				writeAuthzError(w, err, len(r.Header.Values("Authorization")) == 0)
				return
			}

			next.ServeHTTP(w, r.WithContext(withClaims(ctx, claims)))
		})
	}
}

func writeAuthzError(w http.ResponseWriter, err error, noCredentials bool) {
	var unauthenticated *UnauthenticatedError
	var denied *PermissionDeniedError

	status := http.StatusInternalServerError
	message := "internal error"

	switch {
	case errors.As(err, &unauthenticated):
		status = http.StatusUnauthorized
		message = err.Error()
		// RFC 6750 §3: no credentials presented gets a bare realm
		// challenge; a rejected token additionally carries
		// error="invalid_token".
		challenge := `Bearer realm="api", error="invalid_token"`
		if noCredentials {
			challenge = `Bearer realm="api"`
		}
		w.Header().Set("WWW-Authenticate", challenge)
	case errors.As(err, &denied):
		status = http.StatusForbidden
		message = err.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	body, _ := authzErrorCodec.Encode(authzErrorBody{Error: message})
	_, _ = w.Write(body)
}
