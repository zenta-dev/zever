package grpcclient

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// TestRetryableCodeNameAll pins the canonical service-config name for every
// status code, including the CANCELLED spelling gRPC requires and the CODE(n)
// fallback for unknown values.
func TestRetryableCodeNameAll(t *testing.T) {
	t.Parallel()

	cases := []struct {
		code codes.Code
		want string
	}{
		{codes.OK, "OK"},
		{codes.Canceled, "CANCELLED"},
		{codes.Unknown, "UNKNOWN"},
		{codes.InvalidArgument, "INVALID_ARGUMENT"},
		{codes.DeadlineExceeded, "DEADLINE_EXCEEDED"},
		{codes.NotFound, "NOT_FOUND"},
		{codes.AlreadyExists, "ALREADY_EXISTS"},
		{codes.PermissionDenied, "PERMISSION_DENIED"},
		{codes.ResourceExhausted, "RESOURCE_EXHAUSTED"},
		{codes.FailedPrecondition, "FAILED_PRECONDITION"},
		{codes.Aborted, "ABORTED"},
		{codes.OutOfRange, "OUT_OF_RANGE"},
		{codes.Unimplemented, "UNIMPLEMENTED"},
		{codes.Internal, "INTERNAL"},
		{codes.Unavailable, "UNAVAILABLE"},
		{codes.DataLoss, "DATA_LOSS"},
		{codes.Unauthenticated, "UNAUTHENTICATED"},
		{codes.Code(99), "CODE(99)"},
	}

	for _, tc := range cases {
		if got := retryableCodeName(tc.code); got != tc.want {
			t.Errorf("retryableCodeName(%v) = %q, want %q", tc.code, got, tc.want)
		}
	}
}

// mustSelfSignedCertFiles generates a self-signed certificate and writes the
// cert, key, and CA (cert) PEMs to temp files, returning their paths.
func mustSelfSignedCertFiles(t *testing.T) (certFile, keyFile, caFile string) {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	template := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "grpcclient test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})

	certFile = mustWriteTempFile(t, "client.pem", string(certPEM))
	keyFile = mustWriteTempFile(t, "client.key", string(keyPEM))
	caFile = mustWriteTempFile(t, "ca.pem", string(certPEM))

	return certFile, keyFile, caFile
}

// TestNewTLSSuccess pins the full TLS success path: ServerName override,
// client certificate loading, and CA pool verification.
func TestNewTLSSuccess(t *testing.T) {
	t.Parallel()

	certFile, keyFile, caFile := mustSelfSignedCertFiles(t)

	conn, err := New(t.Context(), "dns:///localhost:50051", WithTLS(TLSOptions{
		CertFile:   certFile,
		KeyFile:    keyFile,
		CAFile:     caFile,
		ServerName: "example.com",
	}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// TestNewTLSBareSuccess pins that empty TLS options still yield credentials
// without touching the filesystem.
func TestNewTLSBareSuccess(t *testing.T) {
	t.Parallel()

	conn, err := New(t.Context(), "dns:///localhost:50051", WithTLS(TLSOptions{}))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

// TestNewDialError pins that a service config gRPC itself rejects surfaces as
// a dial error rather than a silent misconfiguration.
func TestNewDialError(t *testing.T) {
	t.Parallel()

	_, err := New(t.Context(), "dns:///localhost:50051",
		WithInsecure(),
		WithServiceConfigJSON(`{"loadBalancingConfig":[{"bogus":{}}]}`))
	if err == nil {
		t.Fatal("New succeeded with unsupported LB policy, want error")
	}

	if !strings.Contains(err.Error(), "dial") {
		t.Fatalf("error = %q, want dial mention", err.Error())
	}
}

// TestStreamInterceptorAppliesTimeout pins that the stream establishment path
// honors WithTimeout like the unary path does.
func TestStreamInterceptorAppliesTimeout(t *testing.T) {
	t.Parallel()

	cfg := &config{timeout: 25 * time.Millisecond, timeoutSet: true}
	streamer := func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, _ string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	_, err := cfg.streamInterceptor()(t.Context(), &grpc.StreamDesc{}, nil, "/svc/Method", streamer)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
}

// TestStreamInterceptorTimeoutWithGuard pins that timeout and guard compose on
// the stream path: the guard still wraps establishment under the deadline.
func TestStreamInterceptorTimeoutWithGuard(t *testing.T) {
	t.Parallel()

	g := &stubGuard{}
	cfg := &config{guard: g, timeout: time.Minute, timeoutSet: true}
	streamer := func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
		return stubClientStream{}, nil
	}

	if _, err := cfg.streamInterceptor()(t.Context(), &grpc.StreamDesc{}, nil, "/svc/Method", streamer); err != nil {
		t.Fatalf("interceptor: %v", err)
	}

	if g.calls != 1 {
		t.Fatalf("guard Execute calls = %d, want 1", g.calls)
	}
}

// TestUnaryInterceptorWithObservabilitySpan pins that the provider branch of
// startSpan runs on the unary path and the span ends with the call.
func TestUnaryInterceptorWithObservabilitySpan(t *testing.T) {
	t.Parallel()

	tr := &stubTracer{}
	cfg := &config{observability: &stubProvider{tracer: tr}}
	invoker, _ := captureInvoker(nil)

	if err := cfg.unaryInterceptor()(t.Context(), "/svc/Method", nil, nil, nil, invoker); err != nil {
		t.Fatalf("interceptor: %v", err)
	}

	if len(tr.spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(tr.spans))
	}

	if tr.spans[0].ended != 1 {
		t.Errorf("span ended %d times, want 1", tr.spans[0].ended)
	}
}

// TestStreamInterceptorWithObservabilitySpan pins the same provider branch on
// the stream path.
func TestStreamInterceptorWithObservabilitySpan(t *testing.T) {
	t.Parallel()

	tr := &stubTracer{}
	cfg := &config{observability: &stubProvider{tracer: tr}}
	streamer := func(context.Context, *grpc.StreamDesc, *grpc.ClientConn, string, ...grpc.CallOption) (grpc.ClientStream, error) {
		return stubClientStream{}, nil
	}

	if _, err := cfg.streamInterceptor()(t.Context(), &grpc.StreamDesc{}, nil, "/svc/Method", streamer); err != nil {
		t.Fatalf("interceptor: %v", err)
	}

	if len(tr.spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(tr.spans))
	}

	if tr.spans[0].ended != 1 {
		t.Errorf("span ended %d times, want 1", tr.spans[0].ended)
	}
}

// TestStartSpanNilProvider pins that without a provider the context passes
// through unchanged with a silent span.
func TestStartSpanNilProvider(t *testing.T) {
	t.Parallel()

	cfg := &config{}
	ctx := t.Context()

	gotCtx, span := cfg.startSpan(ctx, "/svc/Method")
	if gotCtx != ctx {
		t.Error("startSpan without provider changed the context")
	}

	span.SetAttributes()
	span.RecordError(errors.New("cover: ignored"))
	span.End()
}
