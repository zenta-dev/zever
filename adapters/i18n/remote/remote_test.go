package remote_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zenta-dev/zever/i18n"
	"github.com/zenta-dev/zever/i18n/remote"
)

type stubServer struct {
	translateHits atomic.Int64
	localesHits   atomic.Int64
	authSeen      atomic.Value // string

	delay           time.Duration
	translateStatus int
	values          map[string]string // locale+"\x00"+key -> translation
	locales         []string
}

func (f *stubServer) serve(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/translate", func(w http.ResponseWriter, r *http.Request) {
		f.translateHits.Add(1)
		f.authSeen.Store(r.Header.Get("Authorization"))
		if f.delay > 0 {
			select {
			case <-time.After(f.delay):
			case <-r.Context().Done():
				return
			}
		}
		if f.translateStatus != 0 {
			http.Error(w, "boom", f.translateStatus)
			return
		}
		var req struct {
			Locale string            `json:"locale"`
			Key    string            `json:"key"`
			Args   map[string]string `json:"args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		out := map[string]string{}
		if v, ok := f.values[req.Locale+"\x00"+req.Key]; ok {
			out[req.Key] = v
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"translations": out})
	})
	mux.HandleFunc("/locales", func(w http.ResponseWriter, _ *http.Request) {
		f.localesHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"locales": f.locales})
	})
	return httptest.NewServer(mux)
}

func optsFor(url string) i18n.Options {
	return i18n.Options{Remote: i18n.RemoteOptions{Endpoint: url, AllowInsecure: true}}
}

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

func TestTranslateRoundtrip(t *testing.T) {
	t.Parallel()
	f := &stubServer{values: map[string]string{"en\x00hello": "Hello"}}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ad.Close()

	got, err := ad.Translate(t.Context(), "en", "hello", nil)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if got != "Hello" {
		t.Fatalf("got %q want %q", got, "Hello")
	}
}

func TestTranslateMissCached(t *testing.T) {
	t.Parallel()
	f := &stubServer{values: map[string]string{}}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ad.Close()

	ctx := t.Context()
	if _, err := ad.Translate(ctx, "en", "missing", nil); !errors.Is(err, i18n.ErrKeyNotFound) {
		t.Fatalf("first: got %v want ErrKeyNotFound", err)
	}
	if _, err := ad.Translate(ctx, "en", "missing", nil); !errors.Is(err, i18n.ErrKeyNotFound) {
		t.Fatalf("second: got %v want ErrKeyNotFound", err)
	}
	if n := f.translateHits.Load(); n != 1 {
		t.Fatalf("hits = %d want 1 (negative miss must cache)", n)
	}
}

func TestTranslateHitCached(t *testing.T) {
	t.Parallel()
	f := &stubServer{values: map[string]string{"en\x00hi": "Hi"}}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ad.Close()

	ctx := t.Context()
	for range 2 {
		got, err := ad.Translate(ctx, "en", "hi", nil)
		if err != nil || got != "Hi" {
			t.Fatalf("got %q,%v", got, err)
		}
	}
	if n := f.translateHits.Load(); n != 1 {
		t.Fatalf("hits = %d want 1", n)
	}
}

func TestTranslateSingleflight(t *testing.T) {
	t.Parallel()
	f := &stubServer{values: map[string]string{"en\x00slow": "Slow"}, delay: 150 * time.Millisecond}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ad.Close()

	const n = 20
	start := make(chan struct{})
	var wg sync.WaitGroup
	vals := make([]string, n)
	errs := make([]error, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			vals[i], errs[i] = ad.Translate(t.Context(), "en", "slow", nil)
		}(i)
	}
	close(start)
	wg.Wait()
	for i := range n {
		if errs[i] != nil || vals[i] != "Slow" {
			t.Fatalf("g%d: got %q,%v", i, vals[i], errs[i])
		}
	}
	if hits := f.translateHits.Load(); hits != 1 {
		t.Fatalf("hits = %d want 1 (coalesced)", hits)
	}
}

func TestTranslateTimeout(t *testing.T) {
	t.Parallel()
	f := &stubServer{values: map[string]string{"en\x00slow": "Slow"}, delay: 500 * time.Millisecond}
	srv := f.serve(t)
	defer srv.Close()

	o := optsFor(srv.URL)
	o.Remote.Timeout = 50 * time.Millisecond
	ad, err := remote.New(o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ad.Close()

	if _, err := ad.Translate(t.Context(), "en", "slow", nil); err == nil {
		t.Fatal("want timeout error, got nil")
	}
}

func TestTranslateNon200(t *testing.T) {
	t.Parallel()
	f := &stubServer{translateStatus: http.StatusInternalServerError}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ad.Close()

	_, err = ad.Translate(t.Context(), "en", "k", nil)
	if err == nil {
		t.Fatal("want non-200 error, got nil")
	}
}

func TestTranslateEmptyLocale(t *testing.T) {
	t.Parallel()
	f := &stubServer{}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ad.Close()

	if _, err := ad.Translate(t.Context(), "", "k", nil); !errors.Is(err, i18n.ErrLocaleNotFound) {
		t.Fatalf("got %v want ErrLocaleNotFound", err)
	}
}

func TestLocales(t *testing.T) {
	t.Parallel()
	f := &stubServer{locales: []string{"en", "de"}}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ad.Close()

	ctx := t.Context()
	got, err := ad.Locales(ctx)
	if err != nil {
		t.Fatalf("Locales: %v", err)
	}
	if len(got) != 2 || got[0] != "en" || got[1] != "de" {
		t.Fatalf("got %v", got)
	}
	got[0] = "MUT"
	got2, err := ad.Locales(ctx)
	if err != nil {
		t.Fatalf("Locales 2: %v", err)
	}
	if got2[0] != "en" {
		t.Fatalf("cache mutated: %v", got2)
	}
	if n := f.localesHits.Load(); n != 1 {
		t.Fatalf("hits = %d want 1 (cached)", n)
	}
}

func TestBearerAuth(t *testing.T) {
	t.Parallel()
	f := &stubServer{values: map[string]string{"en\x00hi": "Hi"}}
	srv := f.serve(t)
	defer srv.Close()

	o := optsFor(srv.URL)
	o.Remote.APIKey = "secret"
	ad, err := remote.New(o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ad.Close()

	if _, err := ad.Translate(t.Context(), "en", "hi", nil); err != nil {
		t.Fatalf("Translate: %v", err)
	}
	if got, _ := f.authSeen.Load().(string); got != "Bearer secret" {
		t.Fatalf("auth header = %q want %q", got, "Bearer secret")
	}
}

func TestNewBadOptions(t *testing.T) {
	t.Parallel()
	for name, mut := range map[string]func(*i18n.Options){
		"empty endpoint": func(_ *i18n.Options) {},
		"no scheme":      func(o *i18n.Options) { o.Remote.Endpoint = "example.com/x" },
		"http blocked":   func(o *i18n.Options) { o.Remote.Endpoint = "http://example.com" },
		"neg timeout":    func(o *i18n.Options) { o.Remote.Timeout = -time.Second },
		"neg inflight":   func(o *i18n.Options) { o.Remote.MaxInFlight = -1 },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			o := i18n.Options{Remote: i18n.RemoteOptions{AllowInsecure: true}}
			mut(&o)
			if name == "http blocked" {
				o.Remote.AllowInsecure = false
				o.Remote.Endpoint = "http://example.com"
			}
			if _, err := remote.New(o); err == nil {
				t.Fatalf("%s: want error, got nil", name)
			}
		})
	}
}

func TestNewTrimsSlash(t *testing.T) {
	t.Parallel()
	f := &stubServer{values: map[string]string{"en\x00hi": "Hi"}}
	srv := f.serve(t)
	defer srv.Close()

	o := optsFor(srv.URL + "/")
	ad, err := remote.New(o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ad.Close()

	if _, err := ad.Translate(t.Context(), "en", "hi", nil); err != nil {
		t.Fatalf("Translate: %v", err)
	}
}

func TestAfterClose(t *testing.T) {
	t.Parallel()
	f := &stubServer{
		values:  map[string]string{"en\x00hi": "Hi"},
		locales: []string{"en"},
	}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := ad.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := ad.Close(); err != nil {
		t.Fatalf("Close 2: %v", err)
	}
	if _, err := ad.Translate(t.Context(), "en", "hi", nil); !errors.Is(err, i18n.ErrClosed) {
		t.Fatalf("Translate: got %v want ErrClosed", err)
	}
	if _, err := ad.Locales(t.Context()); !errors.Is(err, i18n.ErrClosed) {
		t.Fatalf("Locales: got %v want ErrClosed", err)
	}
}

func TestWaiterCtxCancel(t *testing.T) {
	t.Parallel()
	f := &stubServer{values: map[string]string{"en\x00slow": "Slow"}, delay: 300 * time.Millisecond}
	srv := f.serve(t)
	defer srv.Close()

	ad, err := remote.New(optsFor(srv.URL))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer ad.Close()

	// Occupy the singleflight with a leader, then cancel a waiter.
	leaderDone := make(chan struct{})
	go func() {
		defer close(leaderDone)
		_, _ = ad.Translate(t.Context(), "en", "slow", nil)
	}()
	// Poll for the leader's arrival at the stubServer server instead of a fixed
	// sleep: one translate hit proves the flight is occupied.
	eventually(t, func() bool { return f.translateHits.Load() == 1 }, "leader to reach stubServer server")

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := ad.Translate(ctx, "en", "slow", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v want context.Canceled", err)
	}
	<-leaderDone
}
