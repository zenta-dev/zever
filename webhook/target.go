package webhook

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"

	"github.com/zenta-dev/zever/internal/endpoint"
	"github.com/zenta-dev/zever/internal/httpclient"
)

// ValidateTarget validates target as an HTTPS webhook URL whose host does not
// resolve to a private address.
//
// Callers must re-validate the resolved address at dial time (see
// httpclient.SafeDialContext, wired in via NewSafeClient): DNS records can
// change between validation and delivery, so a target that is public now may
// resolve to a private address later.
// This is the classic DNS-rebinding gap; validation narrows but never closes it.
func ValidateTarget(target string) error {
	return ValidateTargetContext(context.Background(), target)
}

// ValidateTargetContext validates target like ValidateTarget using ctx for DNS.
func ValidateTargetContext(ctx context.Context, target string) error {
	if target == "" {
		return errors.New("webhook: target is empty")
	}
	raw, err := endpoint.ValidateURL(target)
	if err != nil {
		return mapShapeError(target, err)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("webhook: target %q is not a valid URL: %w", target, err)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("webhook: target has no host")
	}
	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return fmt.Errorf("webhook: host lookup failed: %w", err)
		}
		for _, a := range addrs {
			ips = append(ips, a.IP)
		}
	}
	for _, ip := range ips {
		if httpclient.IsPrivateIP(ip) {
			return errors.New("webhook: target resolves to private address")
		}
	}
	return nil
}

// ValidateTargetSyntax checks only the URL shape of target: it must parse with
// an http or https scheme and a host. It performs no DNS resolution, so it
// cannot tell whether the host is private; use ValidateTargetContext for that.
func ValidateTargetSyntax(target string) error {
	if target == "" {
		return errors.New("webhook: target is empty")
	}
	normalized, err := endpoint.ValidateURL(target, endpoint.WithAllowInsecure(true))
	if err != nil {
		switch {
		case errors.Is(err, endpoint.ErrParse):
			return fmt.Errorf("webhook: target %q is not a valid URL: %w", target, err)
		case errors.Is(err, endpoint.ErrNoHost):
			return errors.New("webhook: target has no host")
		default:
			return errors.New("webhook: target must use http or https scheme")
		}
	}
	u, err := url.Parse(normalized)
	if err != nil {
		return fmt.Errorf("webhook: target %q is not a valid URL: %w", target, err)
	}
	if u.Hostname() == "" {
		return errors.New("webhook: target has no host")
	}
	return nil
}

// mapShapeError maps endpoint shape failures to the historical
// ValidateTarget messages for the https-only target policy.
func mapShapeError(target string, err error) error {
	switch {
	case errors.Is(err, endpoint.ErrParse):
		return fmt.Errorf("webhook: target %q is not a valid URL: %w", target, err)
	case errors.Is(err, endpoint.ErrNoHost):
		return errors.New("webhook: target has no host")
	default:
		return errors.New("webhook: target must use https scheme")
	}
}

// IsPrivateIP reports whether ip falls in a private or otherwise non-routable
// range: 127.0.0.0/8 and ::1 (loopback), 10.0.0.0/8, 172.16.0.0/12,
// 192.168.0.0/16, 169.254.0.0/16 and fe80::/10 (link-local), ::/0 and 0.0.0.0
// (unspecified), and fc00::/7 (IPv6 unique local). Nil returns false.
func IsPrivateIP(ip net.IP) bool {
	return httpclient.IsPrivateIP(ip)
}
