// Package remote tests render documents through an HTTP render service.
package remote

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/zenta-dev/zever/core/document"
	"github.com/zenta-dev/zever/shared/traceprop"
)

func TestRenderRoundtripJSON(t *testing.T) {
	t.Parallel()

	want := []byte("rendered-data")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/render" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("missing content-type")
		}

		var req renderRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode: %v", err)
		}

		if req.Format != document.FormatPDF {
			t.Errorf("format: got %q, want %q", req.Format, document.FormatPDF)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(renderResponse{Data: want})
	}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	got, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if string(got) != string(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderPDFBytes(t *testing.T) {
	t.Parallel()

	want := []byte("pdf-bytes")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(want)
	}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	got, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if string(got) != "pdf-bytes" {
		t.Fatalf("got %q, want pdf-bytes", got)
	}
}

func TestRenderEmptyDataErrors(t *testing.T) {
	t.Parallel()

	for _, body := range []string{`{"data":null}`, `{}`, `{"data":""}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(body))
		}))

		doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
		if err != nil {
			srv.Close()
			t.Fatalf("Open: %v", err)
		}

		_, err = doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)
		_ = doc.Close()
		srv.Close()

		if err == nil {
			t.Fatalf("expected error for body %s, got nil", body)
		}
	}
}

func TestRenderUnexpectedContentTypeRejected(t *testing.T) {
	t.Parallel()

	err := renderWithContentType(t, "text/html", []byte(`{"data":"cmVuZGVyZWQ="}`))
	if err == nil {
		t.Fatal("expected error for text/html content type")
	}
}

func TestRenderInvalidContentType(t *testing.T) {
	t.Parallel()

	err := renderWithContentType(t, "text/", []byte("junk"))
	if err == nil {
		t.Fatal("expected error for invalid content-type")
	} else if !strings.Contains(err.Error(), "invalid content-type") {
		t.Fatalf("error should mention invalid content-type: %v", err)
	}
}

func TestRenderBinarySniffPassThrough(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ct   string
		body []byte
	}{
		{"pdf-x-pdf", "application/x-pdf", []byte("%PDF-1.7\n\x01\x02binary")},
		{"pdf-vnd-fdf", "application/vnd.fdf", []byte("%PDF-1.7\n\x01\x02binary")},
		{"pdf-force-download", "application/force-download", []byte("%PDF-1.7\n\x01\x02binary")},
		{"png-x-png", "application/x-png", []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")},
		{"jpeg-x-jpeg", "application/x-jpeg", []byte("\xff\xd8\xff\xe0\x00\x10JFIF\x00\x01")},
		{"webp-x-webp", "application/x-webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 \x00\x00\x00\x00")},
		{"gif87a", "application/x-gif", []byte("GIF87a\x01\x00\x01\x00\x80\x00\x00")},
		{"gif89a", "application/x-gif", []byte("GIF89a\x01\x00\x01\x00\x80\x00\x00")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.ct != "" {
					w.Header().Set("Content-Type", tc.ct)
				}

				_, _ = w.Write(tc.body)
			}))
			defer srv.Close()

			doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
			if err != nil {
				t.Fatalf("Open: %v", err)
			}

			defer func() { _ = doc.Close() }()

			got, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}

			if !bytes.Equal(got, tc.body) {
				t.Fatalf("got %q, want %q", got, tc.body)
			}
		})
	}
}

func renderWithContentType(t *testing.T, contentType string, body []byte) error {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	_, err = doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)

	return err
}

// serveRawHijack serves a raw HTTP response without server sniffing or
// content-type inference, by hijacking the connection.
func serveRawHijack(t *testing.T, raw string) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("w does not implement http.Hijacker")
			return
		}

		conn, _, err := hj.Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}

		defer func() { _ = conn.Close() }()

		_, _ = conn.Write([]byte(raw))
	}))
}

func TestRenderNoContentTypeBinarySniffPassThrough(t *testing.T) {
	t.Parallel()

	want := []byte("%PDF-1.7\n\x01\x02binary")
	rawResp := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", len(want)) + string(want)

	srv := serveRawHijack(t, rawResp)
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	got, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderNoContentTypeNoMagicErrors(t *testing.T) {
	t.Parallel()

	body := []byte("hello world, not binary, not json")
	rawResp := fmt.Sprintf("HTTP/1.1 200 OK\r\nContent-Length: %d\r\n\r\n", len(body)) + string(body)

	srv := serveRawHijack(t, rawResp)
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	if _, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF); err == nil {
		t.Fatal("expected error for non-binary body without content-type")
	}
}

func TestRenderBinaryContentTypePassThrough(t *testing.T) {
	t.Parallel()

	want := []byte("\x25PDF-1.7\x00binary\xff\xfe\x01raw")

	for _, ct := range []string{
		"application/pdf",
		"image/png",
		"image/jpeg",
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", ct)
			_, _ = w.Write(want)
		}))

		doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
		if err != nil {
			srv.Close()
			t.Fatalf("Open: %v", err)
		}

		got, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)
		_ = doc.Close()
		srv.Close()

		if err != nil {
			t.Fatalf("Render (%s): %v", ct, err)
		}

		if !bytes.Equal(got, want) {
			t.Fatalf("Render (%s): got %q, want %q", ct, got, want)
		}
	}
}

func TestRenderOctetStreamWithMagicPassThrough(t *testing.T) {
	t.Parallel()

	want := []byte("%PDF-1.7 binary")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(want)
	}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	got, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}

	if !bytes.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderJSONContentTypeEnvelope(t *testing.T) {
	t.Parallel()

	want := []byte("rendered-data")

	for _, ct := range []string{"application/json", "text/json"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", ct)
			_ = json.NewEncoder(w).Encode(renderResponse{Data: want})
		}))

		doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
		if err != nil {
			srv.Close()
			t.Fatalf("Open: %v", err)
		}

		got, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)
		_ = doc.Close()
		srv.Close()

		if err != nil {
			t.Fatalf("Render (%s): %v", ct, err)
		}

		if string(got) != string(want) {
			t.Fatalf("Render (%s): got %q, want %q", ct, got, want)
		}
	}
}

func TestRenderJSONDecodeError(t *testing.T) {
	t.Parallel()

	err := renderWithContentType(t, "application/json", []byte("not json {"))
	if err == nil {
		t.Fatal("expected decode error")
	} else if !strings.Contains(err.Error(), "decode") {
		t.Fatalf("error should mention decode: %v", err)
	}
}

func TestRenderBinaryEmptyBodyErrors(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	if _, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF); err == nil {
		t.Fatal("expected error for empty binary body")
	}
}

func TestRenderTextPlainNoMagicRejected(t *testing.T) {
	t.Parallel()

	if err := renderWithContentType(t, "text/plain", []byte("sorry, upstream is down")); err == nil {
		t.Fatal("expected error for text/plain body without magic")
	}
}

func TestRenderOctetStreamNoMagicRejected(t *testing.T) {
	t.Parallel()

	if err := renderWithContentType(t, "application/octet-stream", []byte("hello world, not binary, not json")); err == nil {
		t.Fatal("expected error for octet-stream body without magic")
	}
}

func TestRenderRejectsUnsupportedFormat(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("request must not reach server")
	}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	_, err = doc.Render(t.Context(), []byte("<html>"), document.OutputFormat("webp"))
	if err == nil {
		t.Fatal("expected error for unsupported format")
	}

	var uf document.UnsupportedFormatError
	if !errors.As(err, &uf) {
		t.Fatalf("want UnsupportedFormatError, got %T: %v", err, err)
	}

	if !errors.Is(err, document.ErrUnsupportedFormat) {
		t.Fatalf("want ErrUnsupportedFormat, got %v", err)
	}
}

func TestMissingEndpoint(t *testing.T) {
	t.Parallel()

	_, err := New(document.Options{})
	if err == nil {
		t.Fatal("expected error for missing endpoint")
	}

	if !errors.Is(err, document.ErrMissingEndpoint) {
		t.Fatalf("want ErrMissingEndpoint, got %v", err)
	}
}

func TestInvalidEndpoint(t *testing.T) {
	t.Parallel()

	cases := []document.Options{
		{Endpoint: "notaurl"},
		{Endpoint: "ftp://example.com"},
		{Endpoint: "http://"},
		{Endpoint: "http://[::1"},
		{Endpoint: "://bad-url"},
	}
	for _, opts := range cases {
		if _, err := New(opts); err == nil {
			t.Fatalf("expected error for %v", opts)
		}
	}
}

func TestInvalidOptionsRejected(t *testing.T) {
	t.Parallel()

	_, err := New(document.Options{Endpoint: "https://example.com", Timeout: -time.Second})
	if err == nil {
		t.Fatal("expected error for negative timeout")
	}

	if !errors.Is(err, document.ErrInvalidOptions) {
		t.Fatalf("want ErrInvalidOptions, got %v", err)
	}
}

func TestQueryFragmentEndpointRejected(t *testing.T) {
	t.Parallel()

	for _, ep := range []string{
		"https://example.com?token=1",
		"https://example.com/path?token=1#frag",
		"https://example.com/#frag",
	} {
		if _, err := New(document.Options{Endpoint: ep}); err == nil {
			t.Fatalf("expected error for endpoint %q", ep)
		}
	}
}

func TestInsecureEndpointRejectedByDefault(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if _, err := New(document.Options{Endpoint: srv.URL}); err == nil {
		t.Fatal("expected error for http endpoint without AllowInsecure")
	}

	if _, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true}); err != nil {
		t.Fatalf("AllowInsecure should permit http endpoint: %v", err)
	}
}

func TestTrailingSlashEndpoint(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/base/render" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(renderResponse{Data: []byte("ok")})
	}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL + "/base/", AllowInsecure: true})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = doc.Close() }()

	if _, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF); err != nil {
		t.Fatal(err)
	}
}

func TestRenderOversizedBodyRejected(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(bytes.Repeat([]byte("x"), 5000))
	}))
	defer srv.Close()

	doc, err := New(document.Options{
		Endpoint:       srv.URL,
		AllowInsecure:  true,
		MaxOutputBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = doc.Close() }()

	_, err = doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)
	if err == nil {
		t.Fatal("expected error for oversized body")
	}

	var sl document.SizeLimitError
	if !errors.As(err, &sl) {
		t.Fatalf("want SizeLimitError, got %T: %v", err, err)
	}
}

func TestRenderMaxOutputBytesBoundary(t *testing.T) {
	t.Parallel()

	body := bytes.Repeat([]byte("y"), 1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	doc, err := New(document.Options{
		Endpoint:       srv.URL,
		AllowInsecure:  true,
		MaxOutputBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = doc.Close() }()

	got, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)
	if err != nil {
		t.Fatalf("Render at exact limit: %v", err)
	}

	if !bytes.Equal(got, body) {
		t.Fatal("body mismatch at exact limit")
	}
}

func TestRenderCapPlusOneRejected(t *testing.T) {
	t.Parallel()

	body := bytes.Repeat([]byte("y"), 1025)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	doc, err := New(document.Options{
		Endpoint:       srv.URL,
		AllowInsecure:  true,
		MaxOutputBytes: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = doc.Close() }()

	if _, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF); err == nil {
		t.Fatal("expected error for cap+1 body")
	}
}

func TestErrorBodyTruncated(t *testing.T) {
	t.Parallel()

	blob := strings.Repeat("x", 2000)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, blob, http.StatusInternalServerError)
	}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = doc.Close() }()

	_, err = doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)
	if err == nil {
		t.Fatal("expected error")
	}

	msg := err.Error()
	if strings.Contains(msg, strings.Repeat("x", 600)) {
		t.Fatal("error embeds full response body")
	}

	if !strings.Contains(msg, strings.Repeat("x", 100)) {
		t.Fatal("error missing body prefix")
	}
}

func TestBearerHeaderAuthed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer secret-key" {
			t.Errorf("Authorization = %q, want Bearer secret-key", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(renderResponse{Data: []byte("ok")})
	}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, APIKey: "secret-key", AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	if _, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF); err != nil {
		t.Fatalf("Render: %v", err)
	}
}

func TestBearerHeaderAnonymous(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization = %q, want empty", got)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(renderResponse{Data: []byte("ok")})
	}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	if _, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF); err != nil {
		t.Fatalf("Render: %v", err)
	}
}

// withDefaultTransport points http.DefaultTransport at tr for the duration of
// the test. It mutates a global, so the caller must not be parallel.
func withDefaultTransport(t *testing.T, tr http.RoundTripper) {
	t.Helper()

	prev := http.DefaultTransport
	http.DefaultTransport = tr
	t.Cleanup(func() { http.DefaultTransport = prev })
}

// legacyTLSServer starts an HTTPS test server that negotiates at most TLS 1.1,
// so a client enforcing a TLS 1.2 floor cannot complete the handshake. The
// failure happens on version negotiation, before certificate verification, so
// the server's self-signed certificate is irrelevant.
func legacyTLSServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(renderResponse{Data: []byte("ok")})
	}))
	//nolint:gosec // test-only legacy TLS range to observe the client's TLS floor.
	srv.TLS = &tls.Config{MinVersion: tls.VersionTLS10, MaxVersion: tls.VersionTLS11}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	return srv
}

// openAgainst asserts a driver pointed at srv refuses to render: the TLS 1.2
// floor is asserted behaviorally because the driver's transport is wrapped for
// trace injection and can no longer be inspected directly.
func openAgainst(t *testing.T, srv *httptest.Server) document.Document {
	t.Helper()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Cleanup(func() { _ = doc.Close() })

	return doc
}

func requireTLSFloor(t *testing.T, doc document.Document) {
	t.Helper()

	if _, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF); err == nil {
		t.Fatal("TLS 1.1 server accepted; TLS 1.2 floor not enforced")
	}
}

func TestTLSMinVersionEnforced(t *testing.T) {
	// Mutates http.DefaultTransport: must not be parallel.
	withDefaultTransport(t, &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS10}}) //nolint:gosec // legacy floor the driver must upgrade

	requireTLSFloor(t, openAgainst(t, legacyTLSServer(t)))
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestTransportNonStandardDefault(t *testing.T) {
	// Mutates http.DefaultTransport: must not be parallel.
	withDefaultTransport(t, roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return http.DefaultClient.Transport.RoundTrip(r)
	}))

	requireTLSFloor(t, openAgainst(t, legacyTLSServer(t)))
}

func TestTransportUpgradesLegacyTLS(t *testing.T) {
	// Mutates http.DefaultTransport: must not be parallel.
	withDefaultTransport(t, &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS10}}) //nolint:gosec // legacy floor the driver must upgrade

	requireTLSFloor(t, openAgainst(t, legacyTLSServer(t)))
}

func TestTimeoutDefaultAndCustom(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(renderResponse{Data: []byte("ok")})
	}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	d, ok := doc.(*driver)
	if !ok {
		t.Fatalf("want *driver, got %T", doc)
	}

	if d.client.Timeout != document.DefaultTimeout {
		t.Fatalf("Timeout = %v, want %v", d.client.Timeout, document.DefaultTimeout)
	}

	_ = doc.Close()

	if d.maxOutput != document.DefaultMaxOutputBytes {
		t.Fatalf("maxOutput = %d, want %d", d.maxOutput, document.DefaultMaxOutputBytes)
	}

	custom, err := New(document.Options{Endpoint: srv.URL, Timeout: 7 * time.Second, MaxOutputBytes: 1 << 20, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open custom: %v", err)
	}

	defer func() { _ = custom.Close() }()

	cd, ok := custom.(*driver)
	if !ok {
		t.Fatalf("want *driver, got %T", custom)
	}

	if cd.client.Timeout != 7*time.Second {
		t.Fatalf("Timeout = %v, want 7s", cd.client.Timeout)
	}

	if cd.maxOutput != 1<<20 {
		t.Fatalf("maxOutput = %d, want 1MB", cd.maxOutput)
	}
}

func TestRenderTimeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		// Stall past the 50ms client timeout without a fixed sleep: return
		// early if the client goes away, else fall back to a bounded wait.
		select {
		case <-r.Context().Done():
		case <-time.After(300 * time.Millisecond):
		}
	}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, Timeout: 50 * time.Millisecond, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	if _, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF); err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestRenderNonOKStatus(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	_, err = doc.Render(t.Context(), []byte("<html>"), document.FormatPDF)
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}

	if !strings.Contains(err.Error(), "502") {
		t.Fatalf("error should mention status: %v", err)
	}
}

func TestRenderRequestBuildError(t *testing.T) {
	t.Parallel()

	d := &driver{
		endpoint:  "://bad-endpoint",
		maxOutput: 1 << 20,
		client:    http.DefaultClient,
	}

	if _, err := d.Render(t.Context(), []byte("<html>"), document.FormatPDF); err == nil {
		t.Fatal("expected request-build error")
	} else if !strings.Contains(err.Error(), "request") {
		t.Fatalf("error should mention request: %v", err)
	}
}

func TestRenderErrorBodyReadError(t *testing.T) {
	t.Parallel()

	// Declare more bytes than sent, then close: client sees unexpected EOF.
	rawResp := "HTTP/1.1 500 Internal Server Error\r\nContent-Length: 100\r\nConnection: close\r\n\r\nshort"

	srv := serveRawHijack(t, rawResp)
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	if _, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF); err == nil {
		t.Fatal("expected error for truncated error body")
	}
}

func TestRenderBodyReadError(t *testing.T) {
	t.Parallel()

	rawResp := "HTTP/1.1 200 OK\r\nContent-Type: application/pdf\r\nContent-Length: 100\r\nConnection: close\r\n\r\nshort"

	srv := serveRawHijack(t, rawResp)
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	if _, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF); err == nil {
		t.Fatal("expected error for truncated body")
	}
}

func TestCloseNil(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	doc, err := New(document.Options{Endpoint: srv.URL, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	if err := doc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// renderTraceServer serves one render response and reports the traceparent
// header the driver sent.
func renderTraceServer(t *testing.T) (url string, seen func() string) {
	t.Helper()

	var got atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get(traceprop.TraceParentHeader))
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(renderResponse{Data: []byte("ok")})
	}))
	t.Cleanup(srv.Close)

	return srv.URL, func() string {
		v, _ := got.Load().(string)
		return v
	}
}

func TestRenderInjectsTraceHeaders(t *testing.T) {
	t.Parallel()

	endpoint, seenTraceparent := renderTraceServer(t)

	doc, err := New(document.Options{Endpoint: endpoint, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	ctx := ctxWithRemoteTrace(t, "4bf92f3577b34da6a3ce929d0e0e4736")
	if _, err := doc.Render(ctx, []byte("<html>"), document.FormatPDF); err != nil {
		t.Fatalf("Render: %v", err)
	}

	got := seenTraceparent()
	if got == "" {
		t.Fatal("traceparent header missing")
	}

	back := traceprop.Extract(t.Context(), map[string]string{traceprop.TraceParentHeader: got})
	sc := trace.SpanContextFromContext(back)
	if sc.TraceID().String() != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("TraceID = %s, want 4bf92f3577b34da6a3ce929d0e0e4736", sc.TraceID())
	}
}

func TestRenderNoSpanNoTraceHeaders(t *testing.T) {
	t.Parallel()

	endpoint, seenTraceparent := renderTraceServer(t)

	doc, err := New(document.Options{Endpoint: endpoint, AllowInsecure: true})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	defer func() { _ = doc.Close() }()

	if _, err := doc.Render(t.Context(), []byte("<html>"), document.FormatPDF); err != nil {
		t.Fatalf("Render: %v", err)
	}

	if got := seenTraceparent(); got != "" {
		t.Errorf("traceparent = %q, want empty without a span", got)
	}
}

// ctxWithRemoteTrace returns a context carrying a valid remote span.
func ctxWithRemoteTrace(t *testing.T, traceHex string) context.Context {
	t.Helper()

	tid, err := trace.TraceIDFromHex(traceHex)
	if err != nil {
		t.Fatalf("TraceIDFromHex: %v", err)
	}

	sid, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatalf("SpanIDFromHex: %v", err)
	}

	return trace.ContextWithSpanContext(t.Context(), trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
	}))
}
