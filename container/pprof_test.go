package container

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/core/log"
)

func TestStartPprofOffByDefault(t *testing.T) {
	t.Parallel()

	p, err := StartPprof("", nil)
	if err != nil || p != nil {
		t.Fatalf("StartPprof(\"\") = (%v, %v), want (nil, nil)", p, err)
	}

	if p.Addr() != nil {
		t.Error("nil server Addr() != nil")
	}

	if err := p.Shutdown(t.Context()); err != nil {
		t.Errorf("nil server Shutdown() = %v", err)
	}
}

func TestStartPprofRejectsNonLoopback(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{":6060", "0.0.0.0:6060", "[::]:6060", "10.0.0.5:6060", "example.com:6060", "6060", "garbage"} {
		p, err := StartPprof(addr, nil)
		if err == nil || p != nil {
			t.Errorf("StartPprof(%q) = (%v, %v), want error", addr, p, err)
		}
	}
}

func TestStartPprofServesAndStops(t *testing.T) {
	t.Parallel()

	p, err := StartPprof("127.0.0.1:0", &stubLogger{min: log.LevelInfo})
	if err != nil {
		t.Fatalf("StartPprof: %v", err)
	}

	base := "http://" + p.Addr().String()

	get := func(path string) (int, string) {
		req, reqErr := http.NewRequestWithContext(t.Context(), http.MethodGet, base+path, nil)
		if reqErr != nil {
			t.Fatalf("request: %v", reqErr)
		}

		resp, doErr := http.DefaultClient.Do(req)
		if doErr != nil {
			t.Fatalf("GET %s: %v", path, doErr)
		}

		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)

		return resp.StatusCode, string(body)
	}

	if code, body := get("/debug/pprof/"); code != http.StatusOK || !strings.Contains(body, "goroutine") {
		t.Errorf("index = %d %q, want 200 listing profiles", code, body)
	}

	if code, _ := get("/debug/pprof/goroutine?debug=1"); code != http.StatusOK {
		t.Errorf("goroutine = %d, want 200", code)
	}

	if code, body := get("/debug/pprof/heap?debug=1"); code != http.StatusOK || !strings.Contains(body, "heap profile") {
		t.Errorf("heap = %d %q, want 200 heap profile", code, body)
	}

	if code, _ := get("/debug/pprof/nope"); code != http.StatusNotFound {
		t.Errorf("unknown profile = %d, want 404", code)
	}

	for _, path := range []string{"/debug/pprof/profile?seconds=0", "/debug/pprof/profile?seconds=999", "/debug/pprof/trace?seconds=abc"} {
		if code, _ := get(path); code != http.StatusBadRequest {
			t.Errorf("GET %s = %d, want 400", path, code)
		}
	}

	if code, _ := get("/debug/pprof/cmdline"); code != http.StatusNotFound {
		t.Errorf("cmdline = %d, want 404 (argv can hold secrets)", code)
	}

	if code, _ := get("/healthz"); code != http.StatusNotFound {
		t.Errorf("non-pprof path = %d, want 404", code)
	}

	if err := p.Shutdown(t.Context()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, base+"/debug/pprof/", nil)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
		t.Error("server still answering after Shutdown")
	}
}

func TestPprofHandlerDoesNotTouchDefaultMux(t *testing.T) {
	t.Parallel()

	_, pattern := http.DefaultServeMux.Handler(mustReq(t, "/debug/pprof/"))
	if pattern != "" {
		t.Errorf("DefaultServeMux has pattern %q for /debug/pprof/; PprofHandler must use a private mux", pattern)
	}
}

func mustReq(t *testing.T, path string) *http.Request {
	t.Helper()

	r, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://x"+path, nil)
	if err != nil {
		t.Fatal(err)
	}

	return r
}
