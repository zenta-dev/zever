package grpcclient

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/resilience"
)

func TestNewRequiresCredentials(t *testing.T) {
	t.Parallel()

	_, err := New(t.Context(), "dns:///localhost:50051")
	if !errors.Is(err, ErrNoCredentials) {
		t.Fatalf("error = %v, want ErrNoCredentials", err)
	}
}

func TestNewRejectsInsecureWithTLS(t *testing.T) {
	t.Parallel()

	_, err := New(t.Context(), "dns:///localhost:50051", WithInsecure(), WithTLS(TLSOptions{}))
	if !errors.Is(err, ErrInsecureAndTLS) {
		t.Fatalf("error = %v, want ErrInsecureAndTLS", err)
	}
}

func TestNewTLSMissingCAFile(t *testing.T) {
	t.Parallel()

	_, err := New(t.Context(), "dns:///localhost:50051", WithTLS(TLSOptions{CAFile: "/nonexistent/ca.pem"}))
	if err == nil {
		t.Fatal("New succeeded with missing CA file, want error")
	}
	if !strings.Contains(err.Error(), "CA file") {
		t.Fatalf("error = %q, want CA file mention", err.Error())
	}
}

func TestNewTLSClientCertWithoutKey(t *testing.T) {
	t.Parallel()

	_, err := New(t.Context(), "dns:///localhost:50051", WithTLS(TLSOptions{CertFile: "/nonexistent/cert.pem"}))
	if err == nil {
		t.Fatal("New succeeded with CertFile but no KeyFile, want error")
	}
	if !strings.Contains(err.Error(), "CertFile and KeyFile") {
		t.Fatalf("error = %q, want CertFile/KeyFile mention", err.Error())
	}
}

func TestNewNonPositiveTimeout(t *testing.T) {
	t.Parallel()

	_, err := New(t.Context(), "dns:///localhost:50051", WithInsecure(), WithTimeout(0))
	if err == nil {
		t.Fatal("New succeeded with zero timeout, want error")
	}
	if !strings.Contains(err.Error(), "timeout must be positive") {
		t.Fatalf("error = %q, want timeout mention", err.Error())
	}
}

func TestNewCanceledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := New(ctx, "dns:///localhost:50051", WithInsecure())
	if err == nil {
		t.Fatal("New succeeded with canceled context, want error")
	}
}

func TestNewDNSTargetAccepted(t *testing.T) {
	t.Parallel()

	conn, err := New(t.Context(), "dns:///localhost:50051", WithInsecure())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNewStaticTargetAccepted(t *testing.T) {
	t.Parallel()

	conn, err := New(t.Context(), "static:///users",
		WithInsecure(),
		WithStaticResolver(map[string][]string{"users": {"127.0.0.1:50051", "127.0.0.2:50051"}}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNewNilGuardIgnored(t *testing.T) {
	t.Parallel()

	conn, err := New(t.Context(), "dns:///localhost:50051", WithInsecure(), WithGuard(nil))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNewWithGuard(t *testing.T) {
	t.Parallel()

	conn, err := New(t.Context(), "dns:///localhost:50051", WithInsecure(), WithGuard(&stubGuard{}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestNewInvalidRetryPolicy(t *testing.T) {
	t.Parallel()

	_, err := New(t.Context(), "dns:///localhost:50051",
		WithInsecure(),
		WithRetryPolicy(RetryPolicy{MaxAttempts: 1}))
	if err == nil {
		t.Fatal("New succeeded with invalid retry policy, want error")
	}
	if !strings.HasPrefix(err.Error(), "grpcclient:") {
		t.Fatalf("error = %q, want grpcclient: prefix", err.Error())
	}
}

func TestNewInvalidServiceConfigJSON(t *testing.T) {
	t.Parallel()

	_, err := New(t.Context(), "dns:///localhost:50051",
		WithInsecure(),
		WithServiceConfigJSON(`{`))
	if err == nil {
		t.Fatal("New succeeded with invalid service config JSON, want error")
	}
	if !strings.HasPrefix(err.Error(), "grpcclient:") {
		t.Fatalf("error = %q, want grpcclient: prefix", err.Error())
	}
}

// compile-time assertion that stubGuard satisfies resilience.Guard.
var _ resilience.Guard = (*stubGuard)(nil)
