package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultCORSMaxAge disables the Access-Control-Max-Age header when MaxAge
// is unset, forcing browsers to re-run preflight per request.
const DefaultCORSMaxAge time.Duration = 0

// Options configures CORS middleware.
//
// Tags are snake_case so apps can decode their own YAML/TOML/JSON config
// block straight into it; the framework itself owns no cors config section.
// Example app-level YAML:
//
//	cors:
//	  allowed_origins: ["https://example.com"]
//	  allowed_methods: ["GET", "POST"]
//	  allowed_headers: ["Content-Type"]
//	  allow_credentials: true
//	  max_age: 10m
//
// Env mapping is app-level too (e.g. CORS_ALLOWED_ORIGINS), parsed in main
// and passed here. Timeouts stay time.Duration per repo convention and render
// to delay-seconds on the wire.
type Options struct {
	AllowedOrigins   []string `json:"allowed_origins" toml:"allowed_origins" yaml:"allowed_origins"`
	AllowedMethods   []string `json:"allowed_methods" toml:"allowed_methods" yaml:"allowed_methods"`
	AllowedHeaders   []string `json:"allowed_headers" toml:"allowed_headers" yaml:"allowed_headers"`
	ExposedHeaders   []string `json:"exposed_headers" toml:"exposed_headers" yaml:"exposed_headers"`
	AllowCredentials bool     `json:"allow_credentials" toml:"allow_credentials" yaml:"allow_credentials"`
	// MaxAge bounds preflight caching in whole seconds on the wire;
	// sub-second values truncate down (anything below one second omits
	// the header, same as unset).
	MaxAge time.Duration `json:"max_age" toml:"max_age" yaml:"max_age"`
}

// Validate rejects fail-open shapes: empty origin/method lists, blank entries,
// wildcard origin combined with credentials, and negative MaxAge.
func (o Options) Validate() error {
	if len(o.AllowedOrigins) == 0 {
		return errors.New("middleware: cors requires at least one allowed origin")
	}
	if len(o.AllowedMethods) == 0 {
		return errors.New("middleware: cors requires at least one allowed method")
	}
	for _, origin := range o.AllowedOrigins {
		if strings.TrimSpace(origin) == "" {
			return errors.New("middleware: cors allowed origin must not be blank")
		}
	}
	for _, method := range o.AllowedMethods {
		if strings.TrimSpace(method) == "" {
			return errors.New("middleware: cors allowed method must not be blank")
		}
	}
	if o.AllowCredentials {
		for _, origin := range o.AllowedOrigins {
			if origin == "*" {
				return errors.New("middleware: cors wildcard origin must not combine with allow credentials")
			}
		}
	}
	if o.MaxAge < 0 {
		return errors.New("middleware: cors max age must not be negative")
	}

	return nil
}

// CORS returns HTTP middleware enforcing strict cross-origin semantics.
// Non-CORS requests (no Origin header) pass through untouched. A disallowed
// origin is never reflected: simple requests pass through without CORS
// headers and preflights fall through to the next handler. Preflight denials
// on an allowed origin (method/header mismatch) answer 403. Invalid Options
// fail closed with 500 and the fixed error envelope instead of running open.
//
// Wire outermost via r.Use before any Handle call, so preflights short-
// circuit without logging or rate-limit side effects (stdhttp snapshots
// middleware per route at registration).
func CORS(opts Options) func(http.Handler) http.Handler {
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

	allowedOrigins := make(map[string]bool, len(opts.AllowedOrigins))
	wildcard := false
	for _, origin := range opts.AllowedOrigins {
		if origin == "*" {
			wildcard = true
			continue
		}
		allowedOrigins[origin] = true
	}
	allowedMethods := make(map[string]bool, len(opts.AllowedMethods))
	for _, method := range opts.AllowedMethods {
		allowedMethods[strings.ToUpper(strings.TrimSpace(method))] = true
	}
	allowedHeaders := make(map[string]bool, len(opts.AllowedHeaders))
	for _, header := range opts.AllowedHeaders {
		allowedHeaders[strings.ToLower(strings.TrimSpace(header))] = true
	}
	allowMethodsValue := strings.Join(normalizeMethods(opts.AllowedMethods), ", ")
	maxAgeValue := ""
	if opts.MaxAge > 0 {
		maxAgeValue = strconv.Itoa(int(opts.MaxAge.Seconds()))
	}
	exposeValue := strings.Join(opts.ExposedHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin == "" {
				next.ServeHTTP(w, r)

				return
			}

			originAllowed := wildcard || allowedOrigins[origin]
			if !originAllowed {
				next.ServeHTTP(w, r)

				return
			}

			allowOrigin := origin
			if wildcard {
				allowOrigin = "*"
			}

			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				reqMethod := strings.ToUpper(strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")))
				if !allowedMethods[reqMethod] {
					addVary(w, "Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers")
					w.WriteHeader(http.StatusForbidden)

					return
				}
				if !corsHeadersAllowed(r.Header.Get("Access-Control-Request-Headers"), allowedHeaders) {
					addVary(w, "Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers")
					w.WriteHeader(http.StatusForbidden)

					return
				}

				w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
				w.Header().Set("Access-Control-Allow-Methods", allowMethodsValue)
				if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); strings.TrimSpace(reqHeaders) != "" {
					w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
				} else if len(opts.AllowedHeaders) > 0 {
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(opts.AllowedHeaders, ", "))
				}
				if opts.AllowCredentials && !wildcard {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				if maxAgeValue != "" {
					w.Header().Set("Access-Control-Max-Age", maxAgeValue)
				}
				addVary(w, "Origin", "Access-Control-Request-Method", "Access-Control-Request-Headers")
				w.WriteHeader(http.StatusNoContent)

				return
			}

			w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			if opts.AllowCredentials && !wildcard {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			if exposeValue != "" {
				w.Header().Set("Access-Control-Expose-Headers", exposeValue)
			}
			addVary(w, "Origin")
			next.ServeHTTP(w, r)
		})
	}
}

// normalizeMethods upper-cases and trims methods for the Allow-Methods header.
func normalizeMethods(methods []string) []string {
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		out = append(out, strings.ToUpper(strings.TrimSpace(method)))
	}

	return out
}

// corsHeadersAllowed reports whether every comma-separated requested header is
// in the allowlist (case-insensitive). Empty requests always pass.
func corsHeadersAllowed(requested string, allowed map[string]bool) bool {
	if strings.TrimSpace(requested) == "" {
		return true
	}
	for _, header := range strings.Split(requested, ",") {
		name := strings.ToLower(strings.TrimSpace(header))
		if name == "" {
			continue
		}
		if !allowed[name] {
			return false
		}
	}

	return true
}

// addVary appends names to the Vary header without duplicating existing ones.
func addVary(w http.ResponseWriter, names ...string) {
	existing := w.Header().Get("Vary")
	seen := make(map[string]bool)
	var parts []string
	for _, part := range strings.Split(existing, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		seen[strings.ToLower(name)] = true
		parts = append(parts, name)
	}
	for _, name := range names {
		if seen[strings.ToLower(name)] {
			continue
		}
		seen[strings.ToLower(name)] = true
		parts = append(parts, name)
	}
	w.Header().Set("Vary", strings.Join(parts, ", "))
}
