package header

import (
	"context"
	"fmt"
	"net"
	"net/textproto"
	"regexp"
	"strings"

	"github.com/zenta-dev/zever/tenant"
)

// maxHostLength caps host length per DNS limits (253 chars textual representation).
const maxHostLength = 253

type adapter struct {
	header       string
	subdomainRe  *regexp.Regexp
	subdomainFmt string
}

// Resolve returns the tenant ID from the header, falling back to subdomain
// match. meta is trusted as-is; see the package doc for the required
// trust boundary (authenticate the caller and strip/overwrite the tenant
// header upstream of this code).
func (a *adapter) Resolve(_ context.Context, meta map[string]string) (string, error) {
	if meta == nil {
		return "", fmt.Errorf("%w: %w", ErrNilMeta, tenant.ErrNotFound)
	}

	hdr := textproto.CanonicalMIMEHeaderKey(a.header)

	if v, ok := lookupMeta(meta, hdr); ok {
		return v, nil
	}

	if a.subdomainRe != nil {
		if host, ok := lookupMeta(meta, "Host"); ok {
			if strings.Contains(host, ":") {
				h, _, splitErr := net.SplitHostPort(host)
				if splitErr == nil {
					host = h
				} else if !strings.Contains(splitErr.Error(), "missing port") {
					// Fragility note: relies on the "missing port" substring
					// because net.SplitHostPort exposes no sentinel error.
					return "", fmt.Errorf("%w: host %q: %w", ErrInvalidHost, host, splitErr)
				}
			}

			host = strings.ToLower(host)

			if len(host) > maxHostLength {
				return "", fmt.Errorf("%w: %d > %d", ErrHostTooLong, len(host), maxHostLength)
			}

			m := a.subdomainRe.FindStringSubmatch(host)
			if len(m) > 1 && m[1] != "" {
				return m[1], nil
			}

			return "", fmt.Errorf("header: tenant not found in header %q or subdomain %q: %w", a.header, a.subdomainFmt, tenant.ErrNotFound)
		}
	}

	return "", fmt.Errorf("header: tenant not found in header %q: %w", a.header, tenant.ErrNotFound)
}

func lookupMeta(meta map[string]string, key string) (string, bool) {
	for k, v := range meta {
		if textproto.CanonicalMIMEHeaderKey(k) == key && v != "" {
			return v, true
		}
	}

	return "", false
}

// Scoped returns a context carrying the given tenant ID.
func (a *adapter) Scoped(ctx context.Context, tenantID string) (context.Context, error) {
	return tenant.ContextWithTenant(ctx, tenantID), nil
}

// Close releases backend resources, of which header holds none.
func (a *adapter) Close() error {
	return nil
}

// New creates a header-based Tenant, defaulting empty header to DefaultHeader.
// See the package doc for the trust-boundary requirement: only deploy this
// behind a gateway that authenticates callers and owns the tenant header.
func New(o tenant.Options) (tenant.Tenant, error) {
	if err := o.Validate(); err != nil {
		return nil, fmt.Errorf("header: %w", err)
	}

	hdr := o.Header
	if hdr == "" {
		hdr = tenant.DefaultHeader
	}

	hdr = textproto.CanonicalMIMEHeaderKey(hdr)

	a := &adapter{header: hdr}

	if o.SubdomainRegex != "" {
		pattern := o.SubdomainRegex
		if isCatastrophicPattern(pattern) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidPattern, pattern)
		}

		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("%w: %q: %w", ErrInvalidPattern, pattern, err)
		}

		a.subdomainRe = re
		a.subdomainFmt = strings.TrimSpace(pattern)
	}

	return a, nil
}
