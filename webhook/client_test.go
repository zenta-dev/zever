package webhook

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSafeDialContext_blocksPrivateLiteral(t *testing.T) {
	t.Parallel()
	dial := SafeDialContext(false)
	for _, addr := range []string{"127.0.0.1:80", "127.0.0.1", "10.0.0.1:443"} {
		_, err := dial(t.Context(), "tcp", addr)
		if err == nil {
			t.Errorf("dial(%q) = nil, want private-address refusal", addr)
			continue
		}
		if !strings.Contains(err.Error(), `"`) {
			t.Errorf("dial(%q) err %q does not quote the host", addr, err.Error())
		}
	}
}

func TestSafeDialContext_blocksPrivateDNS(t *testing.T) {
	t.Parallel()
	dial := SafeDialContext(false)
	_, err := dial(t.Context(), "tcp", "localhost:80")
	if err == nil {
		t.Fatal("dial(localhost) = nil, want private-address refusal")
	}
	if !strings.Contains(err.Error(), `"localhost"`) {
		t.Errorf("dial(localhost) err %q does not quote the host", err.Error())
	}
}

func TestSafeDialContext_lookupFailure(t *testing.T) {
	t.Parallel()
	dial := SafeDialContext(false)
	_, err := dial(t.Context(), "tcp", "nonexistent.invalid:443")
	if err == nil {
		t.Fatal("dial(nonexistent.invalid) = nil, want lookup failure")
	}
	if !strings.Contains(err.Error(), `"nonexistent.invalid"`) {
		t.Errorf("dial err %q does not quote the host", err.Error())
	}
}

func TestSafeClient_allowPrivate_dialsLocalServer(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	client := NewSafeClient(5*time.Second, true)
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext err = %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("client.Do err = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestNewSafeClient_enforcesTLSAndRedirectPolicy(t *testing.T) {
	t.Parallel()
	client := NewSafeClient(5*time.Second, false)
	if client.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v want %v", client.Timeout, 5*time.Second)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport %T is not *http.Transport", client.Transport)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig is nil")
	}
	if transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %x want %x", transport.TLSClientConfig.MinVersion, tls.VersionTLS12)
	}
	if transport.DialContext == nil {
		t.Error("DialContext is nil")
	}
	if err := client.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Errorf("CheckRedirect err = %v want ErrUseLastResponse", err)
	}
}
