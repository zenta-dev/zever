package webhook

import (
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
