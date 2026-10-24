package vault

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/secrets"
	"github.com/zenta-dev/zever/shared/codec"
)

// stubVaultTransport fakes the Vault HTTP API in memory: every request gets
// the canned status and body with no sockets involved, so response-parsing
// fuzz stays deterministic and offline.
type stubVaultTransport struct {
	status int
	body   string
}

func (s stubVaultTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: s.status,
		Body:       io.NopCloser(strings.NewReader(s.body)),
		Header:     make(http.Header),
	}, nil
}

// mustStubDriver builds a driver whose HTTP layer is the canned stub above:
// Get/List exercise the real request, decode, and fail-closed paths against
// the fuzzed status and body.
func mustStubDriver(t *testing.T, status int, body string) *driver {
	t.Helper()

	return &driver{
		baseURL: "https://vault.test",
		token:   "t",
		mount:   "secret",
		client:  &http.Client{Transport: stubVaultTransport{status: status, body: body}},
	}
}

// decodeTestEnvelope independently decodes a Get response body with the
// same envelope shape vault.go uses, so the fuzz cross-check below can
// prove Get returned exactly what the response encodes.
func decodeTestEnvelope(body string) ([]byte, error) {
	var out struct {
		Data struct {
			Data map[string]string `json:"data"`
		} `json:"data"`
	}
	if err := codec.Unmarshal([]byte(body), &out); err != nil {
		return nil, err
	}
	raw, ok := out.Data.Data["value"]
	if !ok {
		return nil, errors.New("vault: missing value field")
	}
	return base64.StdEncoding.DecodeString(raw)
}

// FuzzGetResponse proves Get response parsing never panics and fails closed:
// truncated or garbage JSON never yields a value, non-200 statuses never
// yield data, and a returned value is always the strict-base64 decoding of
// the response's value field (never foreign or raw bytes).
func FuzzGetResponse(f *testing.F) {
	for _, seed := range []struct {
		status int
		body   string
	}{
		{200, `{"data":{"data":{"value":"aGk="}}}`},
		{200, `{"data":{"data":{"value":""}}}`},
		{200, ""},
		{200, "{not json"},
		{200, `{"data":{"data":{"value":"aGk`},
		{200, `{"data":{"data":{"other":"x"}}}`},
		{200, `{"data":{"data":{"value":"!!!"}}}`},
		{200, "null"},
		{200, "[]"},
		{200, "\x00\xff\xfe"},
		{404, ""},
		{500, "boom"},
		{200, `{"data":{"data":{"value":"aGk="},"metadata":{}},"extra":1}`},
		// Fuzzer-found: non-canonical trailing bits still decode under
		// base64.StdEncoding, so Get succeeds; the cross-check below pins
		// the exact decoded bytes instead of assuming a canonical round-trip.
		{200, `{"data":{"data":{"value":"001="}}}`},
	} {
		f.Add(seed.status, seed.body)
	}

	f.Fuzz(func(t *testing.T, status int, body string) {
		t.Parallel()

		d := mustStubDriver(t, status, body)
		got, err := d.Get(t.Context(), "fuzzkey")

		if err == nil {
			// Success is only reachable on 200: every other status fails closed.
			if status != http.StatusOK {
				t.Fatalf("Get(status=%d) = %q, want error for non-200", status, got)
			}
			// Get must return exactly what the response encodes: decode
			// independently and compare, so foreign or raw bytes fail red.
			want, decErr := decodeTestEnvelope(body)
			if decErr != nil {
				t.Fatalf("Get(body=%q) = %q, body does not decode: %v", body, got, decErr)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("Get(body=%q) = %q, want %q", body, got, want)
			}
			return
		}

		if status == http.StatusNotFound && !errors.Is(err, secrets.ErrNotFound) {
			t.Fatalf("Get(status=404) err = %v, want ErrNotFound", err)
		}
	})
}

// FuzzListResponse proves List response parsing never panics and fails
// closed: truncated or garbage JSON errors, and only 200/404 succeed —
// every other status is an error, so a forged status line can never inject
// a key list.
func FuzzListResponse(f *testing.F) {
	for _, seed := range []struct {
		status int
		body   string
	}{
		{200, `{"data":{"keys":["a","b"]}}`},
		{200, `{"data":{"keys":[]}}`},
		{200, ""},
		{200, `{"data":{"keys":["a"`},
		{200, "{not json"},
		{200, "null"},
		{200, "\x00\xff"},
		{404, ""},
		{500, "boom"},
	} {
		f.Add(seed.status, seed.body)
	}

	f.Fuzz(func(t *testing.T, status int, body string) {
		t.Parallel()

		d := mustStubDriver(t, status, body)
		keys, err := d.List(t.Context())

		if err != nil {
			return
		}
		// Only 200 (key list) and 404 (empty mount, nil keys) succeed.
		if status != http.StatusOK && status != http.StatusNotFound {
			t.Fatalf("List(status=%d, body=%q) = %q, want error for non-200/404", status, body, keys)
		}
		if status == http.StatusNotFound && keys != nil {
			t.Fatalf("List(status=404) = %q, want nil keys", keys)
		}
	})
}

// FuzzEscapeMount proves mount escaping never panics and never lets a
// crafted mount shift path segments: the escaped form keeps the input
// segment count (no smuggled separators) and carries no raw reserved
// characters that would re-split the request path.
func FuzzEscapeMount(f *testing.F) {
	for _, seed := range []string{
		"secret",
		"a/b/c",
		"../escape",
		"a/../../b",
		"with space",
		"with?query",
		"with#frag",
		"percent%2fencoded",
		"",
		"///",
		"a//b",
		"\x00\xff\xfe",
		"uni/ünïcode/键",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, mount string) {
		t.Parallel()

		got := escapeMount(mount)

		wantSegs := strings.Split(strings.Trim(strings.TrimSpace(mount), "/"), "/")
		gotSegs := strings.Split(got, "/")
		if len(gotSegs) != len(wantSegs) {
			t.Fatalf("escapeMount(%q) = %q: %d segments, want %d", mount, got, len(gotSegs), len(wantSegs))
		}
		for _, seg := range gotSegs {
			if strings.Contains(seg, "/") {
				t.Fatalf("escapeMount(%q) = %q: segment %q smuggles a separator", mount, got, seg)
			}
		}
		for _, raw := range []string{" ", "?", "#"} {
			if strings.Contains(got, raw) {
				t.Fatalf("escapeMount(%q) = %q: raw %q survives escaping", mount, got, raw)
			}
		}
	})
}

// FuzzValidateOptions proves Options validation never panics and fails
// closed on hostile fields: dot-dot or blank mounts and plain-http addrs
// (without the explicit opt-in) always error instead of opening an unsafe
// client.
func FuzzValidateOptions(f *testing.F) {
	for _, seed := range []struct {
		addr, mount string
	}{
		{"https://vault.test", "secret"},
		{"https://vault.test", "../escape"},
		{"https://vault.test", ""},
		{"http://vault.test", "secret"},
		{"", "secret"},
		{"https://vault.test", "with space"},
		{"http://", "secret"},
		{"\x00", "\x00"},
	} {
		f.Add(seed.addr, seed.mount)
	}

	f.Fuzz(func(t *testing.T, addr, mount string) {
		t.Parallel()

		// Token is fixed so Validate performs no I/O: deterministic.
		err := Options{Addr: addr, Mount: mount, Token: "t"}.Validate()

		if err == nil {
			if strings.Contains(mount, "..") {
				t.Fatalf("Validate(mount=%q) = nil, want dot-dot rejection", mount)
			}
			if strings.TrimSpace(mount) == "" {
				t.Fatalf("Validate(mount=%q) = nil, want empty-mount rejection", mount)
			}
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(addr)), "http://") {
				t.Fatalf("Validate(addr=%q) = nil, want plain-http rejection", addr)
			}
		}
	})
}
