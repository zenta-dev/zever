package redisopt

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// MaxPrefixLen is the maximum allowed Redis key prefix length in bytes.
const MaxPrefixLen = 64

// MaxPort is the maximum valid TCP port number.
const MaxPort = 65535

// ConnectOptions holds the shared Redis connection settings embedded by
// every battery's Redis options type.
type ConnectOptions struct {
	// Addr is the Redis address as host:port or redis:// and rediss:// URL.
	// Empty means the adapter default (localhost:6379).
	Addr string `json:"addr"`
	// Password is the Redis authentication password.
	Password string `json:"password"`
	// DB is the Redis database index.
	DB int `json:"db"`
	// TLS enables TLS with a TLS 1.2 version floor for plain addresses.
	//
	// TLS is opt-in; the zero value connects in plaintext, meaning session
	// data, cache data, and the Redis password itself are sent unencrypted
	// unless TLS is explicitly enabled here or via a rediss:// address. Set
	// RequireTLS to fail fast instead of silently connecting in plaintext.
	TLS bool `json:"tls"`
	// RequireTLS fails connection setup instead of silently connecting in
	// plaintext when the resolved address would not use TLS (TLS is false
	// and the address isn't rediss://). It defaults to false, preserving
	// the historical plaintext-allowed behavior.
	RequireTLS bool `json:"require_tls"`
}

// ValidateAddr checks a Redis host:port address.
// Empty means the adapter default and is valid.
//
// Every rejection names the offending value (with any embedded credentials
// masked by RedactAddr), says which part of the address is wrong, and
// states the shape the address must have, so the developer can fix the
// setting without reading the adapter's source.
func ValidateAddr(addr string) error {
	if addr == "" {
		return nil
	}

	// The address is echoed back, so mask any credentials first: an address
	// may be a redis:// URL carrying a password.
	shown := RedactAddr(addr)

	if strings.Contains(addr, "://") {
		return fmt.Errorf("redis: addr %q must be host:port without a scheme (drop the redis:// prefix, or set the url option instead)", shown)
	}

	if strings.ContainsAny(addr, "/?#") {
		return fmt.Errorf("redis: addr %q must be host:port (no path, query, or fragment)", shown)
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("redis: addr %q must be host:port (expected \"host:port\")", shown)
	}

	if host == "" {
		return fmt.Errorf("redis: addr %q must include a host", shown)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > MaxPort {
		return fmt.Errorf("redis: addr %q port must be 1-65535", shown)
	}

	return nil
}

// ValidatePrefix checks a Redis key prefix: at most MaxPrefixLen token
// characters. A rejection names the prefix, the limit it broke, and the
// character that was not a token character.
func ValidatePrefix(prefix string) error {
	if len(prefix) > MaxPrefixLen {
		return fmt.Errorf("redis: prefix %q is %d characters, max is %d", prefix, len(prefix), MaxPrefixLen)
	}

	for i := 0; i < len(prefix); i++ {
		if !IsTokenChar(prefix[i]) {
			return fmt.Errorf("redis: prefix %q must contain only token characters (found %q at position %d)", prefix, prefix[i], i)
		}
	}

	return nil
}

// IsTokenChar reports whether c is an RFC 7230 token character.
func IsTokenChar(c byte) bool {
	if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' {
		return true
	}

	switch c {
	case '!', '#', '$', '%', '&', '\'', '*', '+', '-', '.', '^', '_', '`', '|', '~':
		return true
	}

	return false
}

// RedactAddr masks any embedded userinfo credentials in addr, suitable for
// error messages. The password from options is never included.
//
// Both the URL form ("redis://user:pass@host:6379") and the scheme-less
// form ("user:pass@host:6379") are masked: url.Parse reads the scheme-less
// form's "user" as a scheme, so the userinfo segment is masked by hand.
func RedactAddr(addr string) string {
	raw := strings.TrimSpace(addr)

	u, err := url.Parse(raw)
	if err == nil && u.User != nil {
		u.User = url.UserPassword(u.User.Username(), "xxxxx")

		return u.String()
	}

	// Scheme-less userinfo: mask everything before the last "@" that
	// carries a "user:password" pair.
	if at := strings.LastIndex(raw, "@"); at > 0 {
		if colon := strings.LastIndex(raw[:at], ":"); colon >= 0 {
			return raw[:colon] + ":xxxxx" + raw[at:]
		}
	}

	return raw
}

// RedactEndpoint returns the URL-or-Addr endpoint with any embedded userinfo
// credentials masked. URL takes precedence when non-empty.
func RedactEndpoint(urlAddr, addr string) string {
	raw := strings.TrimSpace(urlAddr)
	if raw == "" {
		raw = strings.TrimSpace(addr)
	}

	return RedactAddr(raw)
}
