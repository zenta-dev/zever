package cookie

import (
	"net/http"
	"time"
)

// DefaultName is the cookie name used when Config.Name is empty.
const DefaultName = "session_id"

// DefaultPath is the cookie Path used when Config.Path is empty.
const DefaultPath = "/"

// Config configures how New builds a session cookie. The zero Config
// produces secure defaults: Secure and HTTPOnly on, SameSite=Lax, a
// session cookie (no Max-Age/Expires) scoped to DefaultPath.
//
// Secure and HTTPOnly are *bool (nil means "use the secure default")
// rather than bool, because a plain bool cannot distinguish "unset" from
// an explicit false: the framework default is true, so an explicit
// opt-out must be expressible. This mirrors log/pretty.Options.Color.
type Config struct {
	// Name is the cookie name. Empty defaults to DefaultName.
	Name string
	// Path is the cookie Path attribute. Empty defaults to DefaultPath.
	Path string
	// Domain is the cookie Domain attribute. Empty means no Domain
	// attribute is sent (host-only cookie).
	Domain string
	// Secure controls the Secure attribute. Nil defaults to true
	// (HTTPS-only); set to a false pointer to explicitly allow the
	// cookie over plain HTTP, e.g. for local development.
	Secure *bool
	// HTTPOnly controls the http.Cookie HttpOnly attribute. Nil defaults
	// to true (inaccessible to JavaScript); set to a false pointer to
	// explicitly expose the cookie to scripts.
	HTTPOnly *bool
	// SameSite controls the SameSite attribute. The zero value defaults
	// to http.SameSiteLaxMode; set explicitly to
	// http.SameSiteDefaultMode, http.SameSiteStrictMode, or
	// http.SameSiteNoneMode to override.
	SameSite http.SameSite
	// MaxAge is the cookie lifetime in seconds. Zero means a session
	// cookie: no Max-Age or Expires attribute is sent, and the browser
	// discards the cookie when it closes. A negative value deletes the
	// cookie immediately (Expires set in the past). A positive value
	// sets both Max-Age and Expires.
	MaxAge int
}

// New builds an *http.Cookie carrying sessionID as its value, applying
// cfg's secure defaults for any zero-valued field. The result is plain
// net/http and works unchanged with both router/stdhttp and router/fiber.
func New(sessionID string, cfg Config) *http.Cookie {
	name := cfg.Name
	if name == "" {
		name = DefaultName
	}

	path := cfg.Path
	if path == "" {
		path = DefaultPath
	}

	secure := true
	if cfg.Secure != nil {
		secure = *cfg.Secure
	}

	httpOnly := true
	if cfg.HTTPOnly != nil {
		httpOnly = *cfg.HTTPOnly
	}

	// cfg.SameSite's zero value is neither a valid http.SameSite constant
	// nor http.SameSiteDefaultMode (SameSite's iota starts at 1), so it
	// unambiguously means "unset" here.
	sameSite := cfg.SameSite
	if sameSite == 0 {
		sameSite = http.SameSiteLaxMode
	}

	// Secure, HttpOnly and SameSite are all set above, defaulted to their
	// secure values when cfg leaves them unset; gosec's G124 cannot see
	// that through the local variables, hence the nosec.
	c := &http.Cookie{ //nolint:gosec // G124: secure/httpOnly/sameSite defaulted above
		Name:     name,
		Value:    sessionID,
		Path:     path,
		Domain:   cfg.Domain,
		Secure:   secure,
		HttpOnly: httpOnly,
		SameSite: sameSite,
	}

	if cfg.MaxAge != 0 {
		c.MaxAge = cfg.MaxAge
		c.Expires = time.Now().Add(time.Duration(cfg.MaxAge) * time.Second)
	}

	return c
}

// Set builds a cookie for sessionID via New and writes it to w as a
// Set-Cookie header.
func Set(w http.ResponseWriter, sessionID string, cfg Config) {
	http.SetCookie(w, New(sessionID, cfg))
}
