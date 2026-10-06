package grpcclient

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/resolver"
)

func TestNewNegativeTimeout(t *testing.T) {
	t.Parallel()

	_, err := New(t.Context(), "dns:///localhost:50051", WithInsecure(), WithTimeout(-time.Second))
	if err == nil {
		t.Fatal("New succeeded with negative timeout, want error")
	}

	if !strings.Contains(err.Error(), "timeout must be positive") {
		t.Fatalf("error = %q, want timeout mention", err.Error())
	}
}

func TestNewTLSKeyWithoutCert(t *testing.T) {
	t.Parallel()

	_, err := New(t.Context(), "dns:///localhost:50051", WithTLS(TLSOptions{KeyFile: "/nonexistent/key.pem"}))
	if err == nil {
		t.Fatal("New succeeded with KeyFile but no CertFile, want error")
	}

	if !strings.Contains(err.Error(), "CertFile and KeyFile") {
		t.Fatalf("error = %q, want CertFile/KeyFile mention", err.Error())
	}
}

func TestNewTLSMissingClientCertFiles(t *testing.T) {
	t.Parallel()

	_, err := New(t.Context(), "dns:///localhost:50051", WithTLS(TLSOptions{
		CertFile: "/nonexistent/cert.pem",
		KeyFile:  "/nonexistent/key.pem",
	}))
	if err == nil {
		t.Fatal("New succeeded with missing client cert files, want error")
	}

	if !strings.Contains(err.Error(), "load client certificate") {
		t.Fatalf("error = %q, want load client certificate mention", err.Error())
	}
}

func mustWriteTempFile(t *testing.T, name, data string) string {
	t.Helper()

	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", p, err)
	}

	return p
}

func TestNewTLSInvalidCAPEM(t *testing.T) {
	t.Parallel()

	ca := mustWriteTempFile(t, "ca.pem", "not-a-pem-bundle")

	_, err := New(t.Context(), "dns:///localhost:50051", WithTLS(TLSOptions{CAFile: ca}))
	if err == nil {
		t.Fatal("New succeeded with invalid CA PEM, want error")
	}

	if !strings.Contains(err.Error(), "no certificates found in CA file") {
		t.Fatalf("error = %q, want no-certificates mention", err.Error())
	}
}

func TestRetryableCodeNameUnknown(t *testing.T) {
	t.Parallel()

	if got := retryableCodeName(codes.Code(99)); got != "CODE(99)" {
		t.Errorf("retryableCodeName(99) = %q, want CODE(99)", got)
	}

	if got := retryableCodeName(codes.Unavailable); got != "UNAVAILABLE" {
		t.Errorf("retryableCodeName(Unavailable) = %q, want UNAVAILABLE", got)
	}
}

func TestGrpcDurationFormats(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{10 * time.Millisecond, "0.010s"},
		{100 * time.Millisecond, "0.100s"},
		{2 * time.Second, "2s"},
		{1500 * time.Millisecond, "1.500s"},
	}

	for _, c := range cases {
		if got := grpcDuration(c.in); got != c.want {
			t.Errorf("grpcDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

// errResolverConn fails UpdateState so the resolver error path is covered.
type errResolverConn struct {
	stubResolverClientConn
}

// UpdateState returns a sentinel error.
func (e *errResolverConn) UpdateState(resolver.State) error {
	return errors.New("options-edge: update failed")
}

func TestStaticResolverHostFallback(t *testing.T) {
	t.Parallel()

	b := &staticResolverBuilder{addrs: map[string][]string{"users": {"127.0.0.1:50051"}}}
	target := resolver.Target{URL: url.URL{Scheme: "static", Host: "users"}}
	cc := &stubResolverClientConn{}

	r, err := b.Build(target, cc, resolver.BuildOptions{})
	if err != nil {
		t.Fatalf("Build(host fallback) error = %v", err)
	}

	if r == nil {
		t.Fatal("Build returned nil resolver")
	}

	if len(cc.state.Addresses) != 1 || cc.state.Addresses[0].Addr != "127.0.0.1:50051" {
		t.Fatalf("addresses = %v, want [127.0.0.1:50051]", cc.state.Addresses)
	}
}

func TestStaticResolverUpdateStateError(t *testing.T) {
	t.Parallel()

	b := &staticResolverBuilder{addrs: map[string][]string{"users": {"127.0.0.1:50051"}}}
	target := resolver.Target{URL: url.URL{Scheme: "static", Path: "/users"}}
	cc := &errResolverConn{}

	_, err := b.Build(target, cc, resolver.BuildOptions{})
	if err == nil {
		t.Fatal("Build succeeded despite UpdateState failure, want error")
	}

	if !strings.Contains(err.Error(), "update state") {
		t.Fatalf("error = %q, want update state mention", err.Error())
	}
}

func TestTransportCredentialsConflict(t *testing.T) {
	t.Parallel()

	cfg := &config{insecure: true, tls: &TLSOptions{}}

	_, err := cfg.transportCredentials()
	if !errors.Is(err, ErrInsecureAndTLS) {
		t.Fatalf("transportCredentials(both) err = %v, want ErrInsecureAndTLS", err)
	}
}

func TestNewWithMetadataNilMap(t *testing.T) {
	t.Parallel()

	conn, err := New(t.Context(), "dns:///localhost:50051", WithInsecure(), WithMetadata(nil))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestNewWithEmptyServiceConfigObject(t *testing.T) {
	t.Parallel()

	conn, err := New(t.Context(), "dns:///localhost:50051", WithInsecure(), WithServiceConfigJSON("{}"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestNewConcurrent(t *testing.T) {
	t.Parallel()

	const n = 8

	var wg sync.WaitGroup

	wg.Add(n)

	for range n {
		go func() {
			defer wg.Done()

			conn, err := New(t.Context(), "dns:///localhost:50051", WithInsecure())
			if err != nil {
				t.Errorf("New() error = %v", err)
				return
			}

			if err := conn.Close(); err != nil {
				t.Errorf("Close() error = %v", err)
			}
		}()
	}

	wg.Wait()
}
