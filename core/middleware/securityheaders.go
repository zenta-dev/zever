package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// DefaultHSTSMaxAge is the Strict-Transport-Security max-age the generated
// server passes when TLS is on: one year, per the spec's recommended floor
// for includeSubDomains commitments.
const DefaultHSTSMaxAge = 365 * 24 * time.Hour

// DefaultSecurityHeadersPermissionsPolicy locks down the high-risk
// features a rendered page could otherwise invoke without the app opting
// in: no camera, microphone, or geolocation in any frame.
const DefaultSecurityHeadersPermissionsPolicy = "camera=(), microphone=(), geolocation=()"

// DefaultSecurityHeadersReferrerPolicy trims cross-origin referrers to the
// origin while keeping full same-origin URLs for analytics: the least
// surprising default for an API that also serves a console.
const DefaultSecurityHeadersReferrerPolicy = "strict-origin-when-cross-origin"

// DefaultSecurityHeadersFrameOptions keeps clickjacking protection on for
// apps that never set a policy: same-origin framing only.
const DefaultSecurityHeadersFrameOptions = "SAMEORIGIN"

// SecurityHeadersOptions configures SecurityHeaders.
//
// Tags are snake_case so apps can decode their own YAML/TOML/JSON config
// block straight into it; the framework itself owns no security-headers
// config section. The generated server builds these in code, setting
// HSTSMaxAge only when TLS is on (HSTS on plaintext is a spec violation
// that can brick the host). Timeouts stay time.Duration per repo
// convention and render to max-age seconds on the wire.
type SecurityHeadersOptions struct {
	// HSTSMaxAge emits Strict-Transport-Security with this max-age when
	// positive. Zero (default) omits the header: set it only behind TLS.
	HSTSMaxAge time.Duration `json:"hsts_max_age"`
	// HSTSIncludeSubDomains appends includeSubDomains to the HSTS value.
	// It defaults true because an API host without it leaves sibling
	// hosts downgradeable; set false only when a sibling must stay HTTP.
	HSTSIncludeSubDomains *bool `json:"hsts_include_subdomains"`
	// HSTSPreload appends preload to the HSTS value. It defaults false:
	// preload is a one-way commitment via the browser preload list.
	HSTSPreload bool `json:"hsts_preload"`
	// FrameOptions sets X-Frame-Options. It defaults to SAMEORIGIN; empty
	// omits the header (for apps that enforce framing via CSP frame-ancestors
	// instead, since Carson-era browsers prefer CSP on conflict).
	FrameOptions string `json:"frame_options"`
	// ContentSecurityPolicy sets Content-Security-Policy. Empty (default)
	// omits the header: a policy that fits every app does not exist, so
	// the app opts in explicitly.
	ContentSecurityPolicy string `json:"content_security_policy"`
	// ReferrerPolicy sets Referrer-Policy. It defaults to
	// strict-origin-when-cross-origin; empty omits the header.
	ReferrerPolicy string `json:"referrer_policy"`
	// PermissionsPolicy sets Permissions-Policy. It defaults to a locked-down
	// camera/microphone/geolocation denial; empty omits the header.
	PermissionsPolicy string `json:"permissions_policy"`
}

// Validate rejects fail-open shapes: a negative HSTS max-age.
func (o SecurityHeadersOptions) Validate() error {
	if o.HSTSMaxAge < 0 {
		return errors.New("middleware: security headers hsts max age must not be negative")
	}

	return nil
}

// hstsValue renders the Strict-Transport-Security value for a positive
// max-age: whole seconds on the wire, sub-second values truncating down
// (anything below one second still emits max-age=0, which clears a stale
// HSTS pin rather than setting a useless one).
func (o SecurityHeadersOptions) hstsValue() string {
	value := fmt.Sprintf("max-age=%d", int64(o.HSTSMaxAge/time.Second))
	if o.HSTSIncludeSubDomains == nil || *o.HSTSIncludeSubDomains {
		value += "; includeSubDomains"
	}
	if o.HSTSPreload {
		value += "; preload"
	}

	return value
}

// SecurityHeaders returns HTTP middleware setting baseline response
// security headers before the handler runs, so error responses carry them
// too. Always set: X-Content-Type-Options nosniff, Referrer-Policy, and
// Permissions-Policy (each defaulted, each overridable, empty omits).
// X-Frame-Options defaults to SAMEORIGIN and Content-Security-Policy is
// opt-in. Strict-Transport-Security is emitted only for a positive
// HSTSMaxAge -- pass DefaultHSTSMaxAge when serving TLS, leave zero on
// plaintext. Invalid options fail closed with 500 and the fixed error
// envelope instead of running open.
//
// Wire inside r.Use (after Recover/RequestLogger/Tracing in the generated
// server) so the headers land before any handler WriteHeader. HTTP-only:
// gRPC has no header concept to protect.
func SecurityHeaders(opts SecurityHeadersOptions) func(http.Handler) http.Handler {
	if err := opts.Validate(); err != nil {
		return func(http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				if data, encErr := errorBodyCodec.Encode(errorBody{Error: "internal error"}); encErr == nil {
					_, _ = w.Write(append(data, '\n'))
				}
			})
		}
	}

	frameOptions := opts.FrameOptions
	switch {
	case frameOptions != "":
		// Explicit choice wins.
	case opts.ContentSecurityPolicy != "":
		// CSP owns framing via frame-ancestors: omit X-Frame-Options
		// rather than emit a header browsers deprioritize on conflict.
		frameOptions = ""
	default:
		frameOptions = DefaultSecurityHeadersFrameOptions
	}

	referrerPolicy := opts.ReferrerPolicy
	if referrerPolicy == "" {
		referrerPolicy = DefaultSecurityHeadersReferrerPolicy
	}

	permissionsPolicy := opts.PermissionsPolicy
	if permissionsPolicy == "" {
		permissionsPolicy = DefaultSecurityHeadersPermissionsPolicy
	}

	var hsts string
	if opts.HSTSMaxAge > 0 {
		hsts = opts.hstsValue()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := w.Header()
			header.Set("X-Content-Type-Options", "nosniff")
			header.Set("Referrer-Policy", referrerPolicy)
			header.Set("Permissions-Policy", permissionsPolicy)
			if frameOptions != "" {
				header.Set("X-Frame-Options", frameOptions)
			}
			if opts.ContentSecurityPolicy != "" {
				header.Set("Content-Security-Policy", opts.ContentSecurityPolicy)
			}
			if hsts != "" {
				header.Set("Strict-Transport-Security", hsts)
			}
			next.ServeHTTP(w, r)
		})
	}
}
