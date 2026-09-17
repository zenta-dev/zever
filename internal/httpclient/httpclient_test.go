package httpclient

import (
	"bytes"
	"crypto/tls"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestNewClientTLSFloor(t *testing.T) {
	c := NewClient(5 * time.Second)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T, want *http.Transport", c.Transport)
	}
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion < tls.VersionTLS12 {
		t.Fatalf("TLS MinVersion = %v, want >= TLS1.2", tr.TLSClientConfig)
	}
	if c.Timeout != 5*time.Second {
		t.Fatalf("Timeout = %v", c.Timeout)
	}
}

func TestReadLimitedOverLimit(t *testing.T) {
	body := bytes.NewReader(bytes.Repeat([]byte("x"), 10))
	_, err := ReadLimited(body, 5)
	if err == nil {
		t.Fatalf("expected over-limit error")
	}
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("errors.Is = false, err = %v", err)
	}
	var tl *TooLargeError
	if !errors.As(err, &tl) {
		t.Fatalf("errors.As TooLargeError failed: %v", err)
	}
}

func TestReadLimitedWithinLimit(t *testing.T) {
	body := bytes.NewReader([]byte("hello"))
	got, err := ReadLimited(body, 10)
	if err != nil {
		t.Fatalf("ReadLimited: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestSafeDialGuardBlocksPrivate(t *testing.T) {
	c := NewSafeClient(time.Second, false)
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("Transport is %T", c.Transport)
	}
	if tr.DialContext == nil {
		t.Fatalf("DialContext nil")
	}
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1/", nil)
	_ = req
	// Dial should refuse loopback when allowPrivate=false.
	dial := tr.DialContext
	errCh := make(chan error, 1)
	go func() {
		// Use a short timeout context; SafeDial fails before dialing.
		conn, err := dial(t.Context(), "tcp", "127.0.0.1:80")
		if conn != nil {
			_ = conn.Close()
		}
		errCh <- err
	}()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatalf("expected private dial refusal")
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("dial hung")
	}
}

func TestNoRedirectPolicy(t *testing.T) {
	c := NewSafeClient(time.Second, false)
	if c.CheckRedirect == nil {
		t.Fatalf("CheckRedirect nil")
	}
	if err := c.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("CheckRedirect = %v", err)
	}
	plain := NewClient(time.Second)
	if plain.CheckRedirect != nil {
		t.Fatalf("plain client should follow redirects by default")
	}
}

func TestReadLimitedReadError(t *testing.T) {
	_, err := ReadLimited(errReader{}, 10)
	if err == nil || errors.Is(err, ErrTooLarge) {
		t.Fatalf("expected non-TooLarge read error, got %v", err)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

var _ = io.Discard
