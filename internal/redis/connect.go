package redis

import (
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
)

// MaxPrefixLen is the maximum allowed Redis key prefix length in bytes.
const MaxPrefixLen = 64

// ConnectOptions holds the shared Redis connection settings embedded by
// every battery's Redis options type.
type ConnectOptions struct {
	// Addr is the Redis address as host:port or redis:// and rediss:// URL.
	// Empty means the adapter default (localhost:6379).
	Addr string `json:"addr" toml:"addr" yaml:"addr"`
	// Password is the Redis authentication password.
	Password string `json:"password" toml:"password" yaml:"password"`
	// DB is the Redis database index.
	DB int `json:"db" toml:"db" yaml:"db"`
	// TLS enables TLS with a TLS 1.2 version floor for plain addresses.
	TLS bool `json:"tls" toml:"tls" yaml:"tls"`
}

// ValidateAddr checks a Redis host:port address.
// Empty means the adapter default and is valid.
func ValidateAddr(addr string) error {
	if addr == "" {
		return nil
	}

	if strings.Contains(addr, "://") {
		return errors.New("redis addr must be host:port without scheme")
	}

	if strings.ContainsAny(addr, "/?#") {
		return errors.New("redis addr must be host:port")
	}

	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return errors.New("redis addr must be host:port")
	}

	if host == "" {
		return errors.New("redis addr host must be non-empty")
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return errors.New("redis addr port must be 1-65535")
	}

	return nil
}

// ValidatePrefix checks a Redis key prefix: at most MaxPrefixLen token characters.
func ValidatePrefix(prefix string) error {
	if len(prefix) > MaxPrefixLen {
		return errors.New("redis prefix must be at most 64 characters")
	}

	for i := 0; i < len(prefix); i++ {
		if !IsTokenChar(prefix[i]) {
			return errors.New("redis prefix must contain only token characters")
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
func RedactAddr(addr string) string {
	raw := strings.TrimSpace(addr)

	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}

	u.User = url.UserPassword(u.User.Username(), "xxxxx")

	return u.String()
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
