package remote

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/shared/lrucache"
)

// eventually polls cond until it holds or timeout elapses, failing the
// test on expiry. Fixed sleeps are banned here; all async waits go
// through this helper.
func eventually(t *testing.T, cond func() bool, msg string) {
	t.Helper()

	const timeout = 5 * time.Second
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		timer := time.NewTimer(5 * time.Millisecond)
		select {
		case <-t.Context().Done():
			timer.Stop()
			t.Fatalf("test context done waiting for %s", msg)
		case <-timer.C:
		}
	}
	t.Fatalf("timed out waiting for %s", msg)
}

// flightPresent reports whether key currently occupies a.flights.
func flightPresent(a *adapter, key string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.flights[key]
	return ok
}

// newTestAdapter builds a real adapter against endpoint. White-box: tests
// below poke a.cache/a.flights/a.order directly.
func newTestAdapter(t *testing.T, endpoint string) *adapter {
	t.Helper()
	ii, err := New(i18n.Options{Remote: i18n.RemoteOptions{Endpoint: endpoint, AllowInsecure: true}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	a, ok := ii.(*adapter)
	if !ok {
		t.Fatalf("New returned %T, want *adapter", ii)
	}
	return a
}

func TestCacheKey(t *testing.T) {
	t.Parallel()

	if got := cacheKey("loc", "key", nil); got != "loc\x00key\x00null" {
		t.Fatalf("nil args: got %q want %q", got, "loc\x00key\x00null")
	}
	if got := cacheKey("loc", "key", map[string]string{}); got != "loc\x00key\x00{}" {
		t.Fatalf("empty map: got %q", got)
	}

	a := map[string]string{"b": "2", "a": "1", "c": "3"}
	b := map[string]string{"c": "3", "a": "1", "b": "2"}
	ka := cacheKey("loc", "key", a)
	kb := cacheKey("loc", "key", b)
	if ka != kb {
		t.Fatalf("sorted determinism: %q != %q", ka, kb)
	}
	if got, want := ka, "loc\x00key\x00{\"a\":\"1\",\"b\":\"2\",\"c\":\"3\"}"; got != want {
		t.Fatalf("sorted json: got %q want %q", got, want)
	}

	// Quotes/newlines are round-tripped through json.Marshal escaping, and the
	// same args always map to the same key.
	esc := map[string]string{"q\"u\te\n": "v\"w", "z": "9"}
	e1 := cacheKey("l", "k", esc)
	e2 := cacheKey("l", "k", map[string]string{"z": "9", "q\"u\te\n": "v\"w"})
	if e1 != e2 {
		t.Fatalf("escaped determinism: %q != %q", e1, e2)
	}
	want := "l\x00k\x00{" + jstr("q\"u\te\n") + ":" + jstr("v\"w") + ",\"z\":\"9\"}"
	if e1 != want {
		t.Fatalf("escaped: got %q want %q", e1, want)
	}
}

func jstr(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func newLookupTestAdapter() *adapter {
	return &adapter{cache: lrucache.NewTTL[string, cacheValue](maxCacheEntries, posTTL)}
}

func TestLookup(t *testing.T) {
	t.Parallel()

	a := newLookupTestAdapter()
	a.store("a", cacheValue{val: "A"}, posTTL)
	a.store("b", cacheValue{val: "B"}, posTTL)

	// Hit.
	e, ok := a.lookup("b")
	if !ok || e.val != "B" {
		t.Fatalf("hit: got (%v,%v)", e, ok)
	}

	// Miss: absent key.
	if _, ok := a.lookup("absent"); ok {
		t.Fatal("miss: want not ok")
	}

	// Expiry: a short-TTL entry is lazily purged from the cache once past
	// its expiry, on the next lookup. Poll for the miss instead of a
	// fixed sleep: expiry is clock-driven and must surface promptly.
	a.store("short", cacheValue{val: "S"}, time.Millisecond)
	eventually(t, func() bool {
		_, ok := a.lookup("short")
		return !ok
	}, "short-TTL entry to expire")
}

func TestStoreOverwrite(t *testing.T) {
	t.Parallel()

	a := newLookupTestAdapter()
	a.store("k", cacheValue{val: "v1"}, posTTL)
	a.store("m", cacheValue{val: "m"}, posTTL)
	a.store("k", cacheValue{val: "v2"}, posTTL) // overwrite

	got, ok := a.lookup("k")
	if !ok || got.val != "v2" {
		t.Fatalf("lookup(k) = %v,%v, want v2,true", got, ok)
	}
	if a.cache.Len() != 2 {
		t.Fatalf("len = %d, want 2", a.cache.Len())
	}
}

func TestStoreEviction(t *testing.T) {
	t.Parallel()

	a := newLookupTestAdapter()
	for i := 0; i <= maxCacheEntries; i++ {
		a.store(fmt.Sprintf("k%d", i), cacheValue{val: "v"}, posTTL)
	}
	if a.cache.Len() != maxCacheEntries {
		t.Fatalf("len = %d, want %d", a.cache.Len(), maxCacheEntries)
	}
	if _, ok := a.lookup("k0"); ok {
		t.Fatal("oldest (k0) must be evicted")
	}
	if _, ok := a.lookup(fmt.Sprintf("k%d", maxCacheEntries)); !ok {
		t.Fatal("newest must be present")
	}
}

func TestDoEncodeError(t *testing.T) {
	t.Parallel()

	if _, err := (&adapter{}).do(t.Context(), "x", http.MethodGet, "http://example.com", make(chan int)); err == nil || !strings.Contains(err.Error(), "encode") {
		t.Fatalf("err = %v, want encode failure", err)
	}
}

func TestDoRequestError(t *testing.T) {
	t.Parallel()

	if _, err := (&adapter{}).do(t.Context(), "x", http.MethodGet, "://bad", nil); err == nil || !strings.Contains(err.Error(), "request") {
		t.Fatalf("err = %v, want request failure", err)
	}
}

func TestDoNon200(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv.URL)
	defer a.Close()

	_, err := a.do(t.Context(), "translate", http.MethodPost, srv.URL+"/translate", nil)
	if !errors.Is(err, i18n.ErrRemoteError) {
		t.Fatalf("err = %v, want ErrRemoteError", err)
	}
	if !strings.Contains(err.Error(), "translate: 500: boom") {
		t.Fatalf("err = %v, want status + body", err)
	}
}

func TestDoResponseTooLarge(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, maxRespBody+1024))
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv.URL)
	defer a.Close()

	_, err := a.do(t.Context(), "translate", http.MethodGet, srv.URL+"/t", nil)
	if !errors.Is(err, i18n.ErrRemoteError) {
		t.Fatalf("err = %v, want ErrRemoteError", err)
	}
	if !strings.Contains(err.Error(), "response body exceeds") {
		t.Fatalf("err = %v, want size guard", err)
	}
}

func TestDoBodyReadError(t *testing.T) {
	t.Parallel()

	// Declare a huge Content-Length, send a short body, then slam the
	// connection: the client body read fails with unexpected EOF.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "no hijack", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 100000\r\n\r\nshort"))
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv.URL)
	defer a.Close()

	if _, err := a.do(t.Context(), "translate", http.MethodGet, srv.URL+"/t", nil); err == nil || !strings.Contains(err.Error(), "remote: translate:") {
		t.Fatalf("err = %v, want body read failure", err)
	}
}

func TestFetchDecodeError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{bad`)
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv.URL)
	defer a.Close()

	if _, err := a.fetch(t.Context(), "en", "k", nil); err == nil || !strings.Contains(err.Error(), "remote: translate: decode") {
		t.Fatalf("err = %v, want translate decode failure", err)
	}
}

func TestLocalesDecodeError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{bad`)
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv.URL)
	defer a.Close()

	if _, err := a.Locales(t.Context()); err == nil || !strings.Contains(err.Error(), "remote: locales: decode") {
		t.Fatalf("err = %v, want locales decode failure", err)
	}
}

func TestLocalesRoundtripError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	srv.Close() // kill server: client roundtrip must fail
	a := newTestAdapter(t, srv.URL)
	defer a.Close()

	if _, err := a.Locales(t.Context()); err == nil || !strings.Contains(err.Error(), "remote: locales:") {
		t.Fatalf("err = %v, want client roundtrip failure", err)
	}
}

// slowHandler sleeps delay then answers status; it also honors request
// cancellation so Close aborts in-flight fetches promptly.
func slowHandler(delay time.Duration, status int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(delay):
			http.Error(w, "boom", status)
		case <-r.Context().Done():
		}
	}
}

func TestTranslateFlightErrWaiter(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(slowHandler(400*time.Millisecond, http.StatusInternalServerError))
	defer srv.Close()

	a := newTestAdapter(t, srv.URL)
	defer a.Close()

	leaderDone := make(chan error, 1)
	go func() {
		_, err := a.Translate(t.Context(), "en", "k", nil)
		leaderDone <- err
	}()
	// Poll for the leader's flight registration instead of a fixed sleep:
	// the entry proves the waiter below will join (not lead) the flight.
	eventually(t, func() bool { return flightPresent(a, cacheKey("en", "k", nil)) }, "leader to occupy the flight")

	waiterDone := make(chan error, 1)
	go func() {
		_, err := a.Translate(t.Context(), "en", "k", nil)
		waiterDone <- err
	}()

	leaderErr := <-leaderDone
	waiterErr := <-waiterDone
	if !errors.Is(waiterErr, i18n.ErrRemoteError) {
		t.Fatalf("waiter err = %v, want ErrRemoteError", waiterErr)
	}
	if waiterErr == nil || waiterErr.Error() != leaderErr.Error() {
		t.Fatalf("waiter err %v != leader err %v", waiterErr, leaderErr)
	}
}

func TestTranslateQuitWaiter(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(slowHandler(5*time.Second, http.StatusOK))
	defer srv.Close()

	a := newTestAdapter(t, srv.URL)

	leaderDone := make(chan struct{})
	go func() {
		defer close(leaderDone)
		_, _ = a.Translate(t.Context(), "en", "k", nil)
	}()
	eventually(t, func() bool { return flightPresent(a, cacheKey("en", "k", nil)) }, "leader to occupy the flight")

	waiterDone := make(chan error, 1)
	go func() {
		_, err := a.Translate(t.Context(), "en", "k", nil)
		waiterDone <- err
	}()
	// No fixed sleep for the waiter to park: Close delivers ErrClosed on
	// the quit channel whether the waiter has joined the flight or still
	// observes closed, so either interleaving asserts the same outcome.

	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if werr := <-waiterDone; !errors.Is(werr, i18n.ErrClosed) {
		t.Fatalf("waiter err = %v, want ErrClosed", werr)
	}
	<-leaderDone
}

func TestCloseCancelsFlight(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(slowHandler(5*time.Second, http.StatusOK))
	defer srv.Close()

	a := newTestAdapter(t, srv.URL)

	start := time.Now()
	leaderDone := make(chan error, 1)
	go func() {
		_, err := a.Translate(t.Context(), "en", "k", nil)
		leaderDone <- err
	}()
	eventually(t, func() bool { return flightPresent(a, cacheKey("en", "k", nil)) }, "leader to occupy the flight")

	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("Close took %v, want prompt cancel", d)
	}
	if lerr := <-leaderDone; lerr == nil {
		t.Fatal("leader err = nil, want canceled flight error")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("leader returned after %v, want promptly canceled", d)
	}
}

func TestTranslateEvictionAtMaxFlight(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"translations":{"new":"N"}}`)
	}))
	defer srv.Close()

	a := newTestAdapter(t, srv.URL)
	defer a.Close()

	victimKey := cacheKey("en", "victim", nil)
	a.maxFlight = 1
	a.mu.Lock()
	a.flights[victimKey] = &call{done: make(chan struct{})}
	a.orderElem[victimKey] = a.order.PushFront(victimKey)
	a.mu.Unlock()

	got, err := a.Translate(t.Context(), "en", "new", nil)
	if err != nil || got != "N" {
		t.Fatalf("Translate = %q, %v", got, err)
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, ok := a.flights[victimKey]; ok {
		t.Fatal("victim must be evicted from flights at maxFlight")
	}
}
