package ratelimit

import (
	"fmt"
	"net/netip"
	"strings"
)

// ParseTrustedProxies parses entries as trusted-proxy CIDRs for
// ClientIPKey-style X-Forwarded-For handling. Each entry is either a CIDR
// prefix ("10.0.0.0/8", "2001:db8::/32") or a bare IP address, which maps
// to its single-host prefix (an IPv4 address to /32, IPv6 to /128).
// Blank entries and unparsable values fail with InvalidOptionsError.
// Returned prefixes are masked to their routing form.
func ParseTrustedProxies(entries []string) ([]netip.Prefix, error) {
	out := make([]netip.Prefix, 0, len(entries))

	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			return nil, InvalidOptionsError{Reason: "trusted_proxies must not contain blank entries"}
		}

		if prefix, err := netip.ParsePrefix(entry); err == nil {
			out = append(out, prefix.Masked())

			continue
		}

		addr, err := netip.ParseAddr(strings.Trim(entry, "[]"))
		if err != nil {
			return nil, InvalidOptionsError{Reason: fmt.Sprintf("trusted_proxies %q is not a CIDR or IP address", entry)}
		}

		addr = addr.Unmap().WithZone("")
		bits := 128
		if addr.Is4() {
			bits = 32
		}

		out = append(out, netip.PrefixFrom(addr, bits))
	}

	return out, nil
}
