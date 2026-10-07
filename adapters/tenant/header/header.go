package header

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/textproto"
	"regexp"
	"strings"

	"github.com/zenta-dev/zever/core/tenant"
)

// maxHostLength caps host length per DNS limits (253 chars textual representation).
const maxHostLength = 253

// maxTenantIDLength bounds a resolved tenant ID.
const maxTenantIDLength = 128

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
		return "", fmt.Errorf("header: %w", errors.Join(ErrNilMeta, tenant.ErrNotFound))
	}

	hdr := textproto.CanonicalMIMEHeaderKey(a.header)

	if v, ok, err := lookupMeta(meta, hdr); err != nil {
		return "", err
	} else if ok {
		if err := validateTenantID(v); err != nil {
			return "", err
		}

		return v, nil
	}

	if a.subdomainRe != nil {
		if host, ok, err := lookupMeta(meta, "Host"); err != nil {
			return "", err
		} else if ok {
			if strings.Contains(host, ":") {
				h, _, splitErr := net.SplitHostPort(host)
				if splitErr == nil {
					host = h
				} else if !strings.Contains(splitErr.Error(), "missing port") {
					// Fragility note: relies on the "missing port" substring
					// because net.SplitHostPort exposes no sentinel error.
					return "", fmt.Errorf("%w: host %q: %s", ErrInvalidHost, host, splitErr)
				}
			}

			host = strings.ToLower(host)

			if len(host) > maxHostLength {
				return "", fmt.Errorf("%w: %d > %d", ErrHostTooLong, len(host), maxHostLength)
			}

			m := a.subdomainRe.FindStringSubmatch(host)
			if len(m) > 1 && m[1] != "" {
				if err := validateTenantID(m[1]); err != nil {
					return "", err
				}

				return m[1], nil
			}

			return "", fmt.Errorf("header: tenant not found in header %q or subdomain %q: %w", a.header, a.subdomainFmt, tenant.ErrNotFound)
		}
	}

	return "", fmt.Errorf("header: tenant not found in header %q: %w", a.header, tenant.ErrNotFound)
}

// lookupMeta returns the value for key, matching meta keys case-insensitively.
// Two case variants of key carrying different values are ambiguous and are
// rejected with ErrDuplicateHeader instead of resolving in nondeterministic
// map-iteration order; identical duplicates resolve to the shared value.
func lookupMeta(meta map[string]string, key string) (string, bool, error) {
	var found string

	var seen bool

	for k, v := range meta {
		if textproto.CanonicalMIMEHeaderKey(k) != key || v == "" {
			continue
		}

		if seen && v != found {
			return "", false, fmt.Errorf("%w: %q", ErrDuplicateHeader, key)
		}

		found, seen = v, true
	}

	return found, seen, nil
}

// validateTenantID checks a resolved tenant ID for shape and length: 1-128
// chars of ASCII letters, digits, '-', '_', or '.'.
func validateTenantID(id string) error {
	if id == "" || len(id) > maxTenantIDLength {
		return fmt.Errorf("%w: %q", ErrInvalidTenantID, id)
	}

	if strings.ContainsFunc(id, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' && r != '_' && r != '.'
	}) {
		return fmt.Errorf("%w: %q", ErrInvalidTenantID, id)
	}

	return nil
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
			return nil, fmt.Errorf("%w: %q: %s", ErrInvalidPattern, pattern, err)
		}

		a.subdomainRe = re
		a.subdomainFmt = strings.TrimSpace(pattern)
	}

	return a, nil
}
