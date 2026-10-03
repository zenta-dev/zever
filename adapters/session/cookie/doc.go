// Package cookie builds *http.Cookie values for a session ID with
// secure-by-default flags (Secure, HttpOnly, SameSite=Lax), so that
// downstream apps wiring a session.Store into an HTTP handler never have
// to reimplement safe cookie defaults from scratch.
//
// Options.Prefix opts into the browser-enforced __Host-/__Secure- name
// prefixes; New forces the attributes each prefix requires (Secure, and for
// __Host- Path=/ with no Domain) so the browser never rejects the cookie.
//
// The returned *http.Cookie is plain net/http, so it works unchanged with
// both router/stdhttp and router/fiber: both expose handlers as
// http.HandlerFunc, fiber included, via github.com/gofiber/adaptor/v2.
package cookie
