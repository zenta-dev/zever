package header

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/tenant"
)

func TestResolveFromHeader(t *testing.T) {
	t.Parallel()

	a := &adapter{header: "X-Tenant-ID"}
	meta := map[string]string{"X-Tenant-ID": "acme"}

	id, err := a.Resolve(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}

	if id != "acme" {
		t.Fatalf("want %q, got %q", "acme", id)
	}
}

func TestResolveHeaderCaseInsensitive(t *testing.T) {
	t.Parallel()

	a := &adapter{header: "X-Tenant-ID"}
	meta := map[string]string{"x-tenant-id": "acme"}

	id, err := a.Resolve(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}

	if id != "acme" {
		t.Fatalf("want %q, got %q", "acme", id)
	}
}

func TestResolveHeaderNonCanonicalMetaKey(t *testing.T) {
	t.Parallel()

	a := &adapter{header: "X-Tenant-ID"}
	meta := map[string]string{"X-TENANT-ID": "acme"}

	id, err := a.Resolve(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}

	if id != "acme" {
		t.Fatalf("want %q, got %q", "acme", id)
	}
}

func TestOpenCaseInsensitiveHeaderOpt(t *testing.T) {
	t.Parallel()

	tt, err := Open(tenant.Options{Header: "x-tenant-id"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	meta := map[string]string{"X-Tenant-ID": "acme"}

	id, err := tt.Resolve(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}

	if id != "acme" {
		t.Fatalf("want %q, got %q", "acme", id)
	}
}

func TestResolveMissingHeader(t *testing.T) {
	t.Parallel()

	a := &adapter{header: "X-Tenant-ID"}
	meta := map[string]string{"Other": "val"}

	_, err := a.Resolve(context.Background(), meta)
	if err == nil {
		t.Fatal("want error, got nil")
	}

	if !errors.Is(err, tenant.ErrNotFound) {
		t.Fatalf("want tenant.ErrNotFound, got %v", err)
	}
}

func TestResolveNilMeta(t *testing.T) {
	t.Parallel()

	a := &adapter{header: "X-Tenant-ID"}

	_, err := a.Resolve(context.Background(), nil)
	if err == nil {
		t.Fatal("want error, got nil")
	}

	if !errors.Is(err, ErrNilMeta) {
		t.Fatalf("want ErrNilMeta, got %v", err)
	}

	if !errors.Is(err, tenant.ErrNotFound) {
		t.Fatalf("want tenant.ErrNotFound, got %v", err)
	}
}

func TestResolveFromSubdomain(t *testing.T) {
	t.Parallel()

	re := mustCompile(t, `^([a-z0-9-]+)\.example\.com$`)
	a := &adapter{header: "X-Tenant-ID", subdomainRe: re}
	meta := map[string]string{"Host": "acme.example.com"}

	id, err := a.Resolve(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}

	if id != "acme" {
		t.Fatalf("want %q, got %q", "acme", id)
	}
}

func TestResolveFromSubdomainWithPort(t *testing.T) {
	t.Parallel()

	re := mustCompile(t, `^([a-z0-9-]+)\.example\.com$`)
	a := &adapter{header: "X-Tenant-ID", subdomainRe: re}
	meta := map[string]string{"Host": "acme.example.com:8080"}

	id, err := a.Resolve(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}

	if id != "acme" {
		t.Fatalf("want %q, got %q", "acme", id)
	}
}

func TestResolveSubdomainLowercaseHostKey(t *testing.T) {
	t.Parallel()

	re := mustCompile(t, `^([a-z0-9-]+)\.example\.com$`)
	a := &adapter{header: "X-Tenant-ID", subdomainRe: re}
	meta := map[string]string{"host": "acme.example.com"}

	id, err := a.Resolve(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}

	if id != "acme" {
		t.Fatalf("want %q, got %q", "acme", id)
	}
}

func TestResolveSubdomainMixedCaseHostKey(t *testing.T) {
	t.Parallel()

	re := mustCompile(t, `^([a-z0-9-]+)\.example\.com$`)
	a := &adapter{header: "X-Tenant-ID", subdomainRe: re}
	meta := map[string]string{"HoSt": "acme.example.com"}

	id, err := a.Resolve(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}

	if id != "acme" {
		t.Fatalf("want %q, got %q", "acme", id)
	}
}

func TestResolveHeaderTakesPrecedence(t *testing.T) {
	t.Parallel()

	re := mustCompile(t, `^([a-z0-9-]+)\.example\.com$`)
	a := &adapter{header: "X-Tenant-ID", subdomainRe: re}
	meta := map[string]string{
		"X-Tenant-ID": "from-header",
		"Host":        "from-sub.example.com",
	}

	id, err := a.Resolve(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}

	if id != "from-header" {
		t.Fatalf("want %q, got %q", "from-header", id)
	}
}

func TestResolveSubdomainMixedCaseHostValue(t *testing.T) {
	t.Parallel()

	re := mustCompile(t, `^([a-z0-9-]+)\.example\.com$`)
	a := &adapter{header: "X-Tenant-ID", subdomainRe: re}
	meta := map[string]string{"Host": "Acme.Example.Com"}

	id, err := a.Resolve(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}

	if id != "acme" {
		t.Fatalf("want %q, got %q", "acme", id)
	}
}

func TestResolveSubdomainMixedCaseHostValueWithPort(t *testing.T) {
	t.Parallel()

	re := mustCompile(t, `^([a-z0-9-]+)\.example\.com$`)
	a := &adapter{header: "X-Tenant-ID", subdomainRe: re}
	meta := map[string]string{"Host": "Acme.Example.Com:8080"}

	id, err := a.Resolve(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}

	if id != "acme" {
		t.Fatalf("want %q, got %q", "acme", id)
	}
}

func TestResolveSubdomainUpperCaseApex(t *testing.T) {
	t.Parallel()

	re := mustCompile(t, `^([a-z0-9-]+)\.example\.com$`)
	a := &adapter{header: "X-Tenant-ID", subdomainRe: re}
	meta := map[string]string{"Host": "acme.example.COM"}

	id, err := a.Resolve(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}

	if id != "acme" {
		t.Fatalf("want %q, got %q", "acme", id)
	}
}

func TestResolveSubdomainNoMatch(t *testing.T) {
	t.Parallel()

	re := mustCompile(t, `^([a-z0-9-]+)\.example\.com$`)
	a := &adapter{header: "X-Tenant-ID", subdomainRe: re, subdomainFmt: `^([a-z0-9-]+)\.example\.com$`}
	meta := map[string]string{"Host": "www.other.com"}

	_, err := a.Resolve(context.Background(), meta)
	if err == nil {
		t.Fatal("want error, got nil")
	}

	if !errors.Is(err, tenant.ErrNotFound) {
		t.Fatalf("want tenant.ErrNotFound, got %v", err)
	}
}

func TestResolveSubdomainEmptyCapture(t *testing.T) {
	t.Parallel()

	re := mustCompile(t, `^([^.]*)\.example\.com$`)
	a := &adapter{header: "X-Tenant-ID", subdomainRe: re}
	meta := map[string]string{"Host": "example.com"}

	id, err := a.Resolve(context.Background(), meta)
	if err == nil {
		t.Fatalf("want error for empty subdomain capture, got %q", id)
	}

	if !errors.Is(err, tenant.ErrNotFound) {
		t.Fatalf("want tenant.ErrNotFound, got %v", err)
	}
}

func TestResolveBadHost(t *testing.T) {
	t.Parallel()

	tt, err := Open(tenant.Options{
		Header:         "X-Tenant-ID",
		SubdomainRegex: `^([a-z0-9-]+)\.example\.com$`,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	_, err = tt.Resolve(context.Background(), map[string]string{"Host": "a:b:c"})
	if err == nil {
		t.Fatal("want error, got nil")
	}

	if !errors.Is(err, ErrInvalidHost) {
		t.Fatalf("want ErrInvalidHost, got %v", err)
	}
}

func TestResolveHostTooLong(t *testing.T) {
	t.Parallel()

	tt, err := Open(tenant.Options{
		Header:         "X-Tenant-ID",
		SubdomainRegex: `^([a-z0-9-]+)\.example\.com$`,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	host := strings.Repeat("a", 254)
	_, err = tt.Resolve(context.Background(), map[string]string{"Host": host})
	if err == nil {
		t.Fatal("want error, got nil")
	}

	if !errors.Is(err, ErrHostTooLong) {
		t.Fatalf("want ErrHostTooLong, got %v", err)
	}
}

func TestOpenCatastrophicPattern(t *testing.T) {
	t.Parallel()

	_, err := Open(tenant.Options{Header: "X-Tenant-ID", SubdomainRegex: `^(a+)+$`})
	if err == nil {
		t.Fatal("want error, got nil")
	}

	if !errors.Is(err, ErrInvalidPattern) {
		t.Fatalf("want ErrInvalidPattern, got %v", err)
	}
}

func TestOpenUncompilablePattern(t *testing.T) {
	t.Parallel()

	_, err := Open(tenant.Options{Header: "X-Tenant-ID", SubdomainRegex: `(unclosed`})
	if err == nil {
		t.Fatal("want error, got nil")
	}

	if !errors.Is(err, ErrInvalidPattern) {
		t.Fatalf("want ErrInvalidPattern, got %v", err)
	}
}

func TestOpenInvalidOptions(t *testing.T) {
	t.Parallel()

	_, err := Open(tenant.Options{Header: "X Bad"})
	if err == nil {
		t.Fatal("want error, got nil")
	}

	if !errors.Is(err, tenant.ErrInvalidOptions) {
		t.Fatalf("want ErrInvalidOptions, got %v", err)
	}
}

func TestScopedSetsContext(t *testing.T) {
	t.Parallel()

	a := &adapter{header: "X-Tenant-ID"}

	ctx, err := a.Scoped(context.Background(), "t-99")
	if err != nil {
		t.Fatal(err)
	}

	got, ok := tenant.FromContext(ctx)
	if !ok {
		t.Fatal("want ok")
	}

	if got != "t-99" {
		t.Fatalf("want %q, got %q", "t-99", got)
	}
}

func TestCloseNoop(t *testing.T) {
	t.Parallel()

	a := &adapter{header: "X-Tenant-ID"}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultHeaderName(t *testing.T) {
	t.Parallel()

	tt, err := Open(tenant.Options{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	meta := map[string]string{"X-Tenant-ID": "default-hdr"}

	id, err := tt.Resolve(context.Background(), meta)
	if err != nil {
		t.Fatal(err)
	}

	if id != "default-hdr" {
		t.Fatalf("want %q, got %q", "default-hdr", id)
	}
}

func mustCompile(t *testing.T, pattern string) *regexp.Regexp {
	t.Helper()

	re, err := regexp.Compile(pattern)
	if err != nil {
		t.Fatalf("compile %q: %v", pattern, err)
	}

	return re
}
