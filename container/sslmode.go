package container

import (
	"net"
	"net/url"
	"strings"
)

// postgresSSLModeWarning returns a warning when dsn (URL or key=value form,
// with PGSSLMODE/PGHOST consulted when the DSN is silent) leaves TLS
// optional toward a non-local host, and "" otherwise. sslmode prefer and
// allow both connect in plaintext when the server does not offer TLS, so a
// network attacker can strip encryption; require or verify-full close that.
func postgresSSLModeWarning(dsn string, getenv func(string) string) string {
	params := parsePostgresDSN(dsn)

	mode := strings.ToLower(params["sslmode"])
	if mode == "" {
		mode = strings.ToLower(getenv("PGSSLMODE"))
	}

	if mode != "" && mode != "prefer" && mode != "allow" {
		return ""
	}

	host := params["host"]
	if host == "" {
		host = getenv("PGHOST")
	}

	if !hasRemoteHost(host) {
		return ""
	}

	shown := mode
	if shown == "" {
		shown = "unset (defaults to prefer)"
	}

	return "postgres: sslmode " + shown + " allows an unencrypted fallback to a remote host; " +
		"set sslmode=require or verify-full in the DSN (or sslmode=disable explicitly for a trusted network)"
}

// parsePostgresDSN extracts the host and sslmode settings from either DSN
// form. Unparseable input yields an empty map.
func parsePostgresDSN(dsn string) map[string]string {
	out := map[string]string{}
	dsn = strings.TrimSpace(dsn)

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		u, err := url.Parse(dsn)
		if err != nil {
			return out
		}

		out["host"] = u.Host
		if q := u.Query(); q.Get("host") != "" {
			out["host"] = q.Get("host")
		}

		out["sslmode"] = u.Query().Get("sslmode")

		return out
	}

	for _, field := range strings.Fields(dsn) {
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}

		switch k {
		case "host", "sslmode":
			out[k] = strings.Trim(v, `'"`)
		}
	}

	return out
}

// hasRemoteHost reports whether a comma-separated host list contains any
// host other than a unix socket path, localhost or a loopback address. An
// empty list means the libpq default (local socket), which is not remote.
func hasRemoteHost(hosts string) bool {
	for _, h := range strings.Split(hosts, ",") {
		h = strings.TrimSpace(h)
		if h == "" || strings.HasPrefix(h, "/") {
			continue
		}

		if host, _, err := net.SplitHostPort(h); err == nil {
			h = host
		}

		h = strings.Trim(h, "[]")
		if strings.EqualFold(h, "localhost") {
			continue
		}

		if ip := net.ParseIP(h); ip != nil && ip.IsLoopback() {
			continue
		}

		return true
	}

	return false
}
