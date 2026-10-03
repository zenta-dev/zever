package cookie

import (
	"net/http"
	"time"
)

// DefaultName is the cookie name used when Options.Name is empty.
const DefaultName = "session_id"

// DefaultPath is the cookie Path used when Options.Path is empty.
const DefaultPath = "/"

// PrefixHost is the "__Host-" cookie-name prefix. Browsers only accept a
// __Host- cookie when it is Secure, scoped to Path=/, and host-only (no
// Domain attribute); New enforces all three.
const PrefixHost = "__Host-"

// PrefixSecure is the "__Secure-" cookie-name prefix. Browsers only accept
// a __Secure- cookie when it is Secure; New enforces that.
const PrefixSecure = "__Secure-"

// Options configures how New builds a session cookie. The zero Options
// produces secure defaults: Secure and HTTPOnly on, SameSite=Lax, a
// session cookie (no Max-Age/Expires) scoped to DefaultPath.
//
// Secure and HTTPOnly are *bool (nil means "use the secure default")
// rather than bool, because a plain bool cannot distinguish "unset" from
// an explicit false: the framework default is true, so an explicit
// opt-out must be expressible.
//
// Setting Prefix to PrefixHost or PrefixSecure prepends the browser-enforced
// cookie-name prefix and forces the attributes that prefix requires (see the
// Prefix field). The default is no prefix.
type Options struct {
	// Name is the cookie name. Empty defaults to DefaultName.
	Name string `json:"name" toml:"name" yaml:"name"`
	// Path is the cookie Path attribute. Empty defaults to DefaultPath.
	Path string `json:"path" toml:"path" yaml:"path"`
	// Domain is the cookie Domain attribute. Empty means no Domain
	// attribute is sent (host-only cookie). A non-empty Domain widens the
	// cookie's scope to the named host and all its subdomains, so it is
	// ignored (omitted) when Prefix is PrefixHost, which must stay
	// host-only to keep the prefix's guarantee.
	Domain string `json:"domain" toml:"domain" yaml:"domain"`
	// Prefix is the cookie-name security prefix: PrefixHost ("__Host-"),
	// PrefixSecure ("__Secure-"), or "" for none. When set, New prepends
	// it to Name and forces the prefix's required attributes: __Host-
	// forces Secure, Path=/, and no Domain (a Domain would widen the
	// cookie to subdomains, defeating the host-only guarantee); __Secure-
	// forces Secure. Any other value is ignored. SameSite is independent
	// of the prefix and keeps its own default.
	Prefix string `json:"prefix" toml:"prefix" yaml:"prefix"`
	// Secure controls the Secure attribute. Nil defaults to true
	// (HTTPS-only); set to a false pointer to explicitly allow the
	// cookie over plain HTTP, e.g. for local development.
	Secure *bool `json:"secure" toml:"secure" yaml:"secure"`
	// HTTPOnly controls the http.Cookie HttpOnly attribute. Nil defaults
	// to true (inaccessible to JavaScript); set to a false pointer to
	// explicitly expose the cookie to scripts.
	HTTPOnly *bool `json:"http_only" toml:"http_only" yaml:"http_only"`
	// SameSite controls the SameSite attribute. The zero value defaults
	// to http.SameSiteLaxMode; set explicitly to
	// http.SameSiteDefaultMode, http.SameSiteStrictMode, or
	// http.SameSiteNoneMode to override.
	SameSite http.SameSite `json:"same_site" toml:"same_site" yaml:"same_site"`
	// MaxAge is the cookie lifetime in seconds. Zero means a session
	// cookie: no Max-Age or Expires attribute is sent, and the browser
	// discards the cookie when it closes. A negative value deletes the
	// cookie immediately (Expires set in the past). A positive value
	// sets both Max-Age and Expires. int seconds (not time.Duration) is
	// required by net/http; every other duration in the repo is
	// time.Duration.
	MaxAge int `json:"max_age" toml:"max_age" yaml:"max_age"`
}

// Config aliases Options for compatibility.
type Config = Options

// New builds an *http.Cookie carrying sessionID as its value, applying
// cfg's secure defaults for any zero-valued field. The result is plain
// net/http and works unchanged with both router/stdhttp and router/fiber.
func New(sessionID string, cfg Options) *http.Cookie {
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

	domain := cfg.Domain

	// Enforce the browser rules for the requested cookie-name prefix. The
	// prefix's constraints win over cfg: a __Host- cookie that is not
	// Secure, not Path=/, or carries a Domain is rejected by the browser
	// outright, so honoring the conflicting field would silently drop the
	// cookie (or the prefix's protection) instead of failing loudly.
	switch cfg.Prefix {
	case PrefixHost:
		name = PrefixHost + name
		secure = true
		path = DefaultPath
		domain = ""
	case PrefixSecure:
		name = PrefixSecure + name
		secure = true
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
		Domain:   domain,
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
func Set(w http.ResponseWriter, sessionID string, cfg Options) {
	http.SetCookie(w, New(sessionID, cfg))
}
