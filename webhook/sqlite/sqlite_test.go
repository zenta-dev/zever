package sqlite

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "modernc.org/sqlite" // register sqlite driver for raw-SQL fixtures

	"github.com/zenta-dev/zever/webhook"
)

func testOptions(dsn string) webhook.Options {
	return webhook.Options{
		Timeout:             5 * time.Second,
		MaxRetries:          1,
		AllowPrivateTargets: true,
		DSN:                 dsn,
	}
}

func openTest(t *testing.T, o webhook.Options) webhook.Webhook {
	t.Helper()

	w, err := New(o)
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	t.Cleanup(func() {
		if err := w.Close(); err != nil {
			t.Errorf("Close err = %v", err)
		}
	})

	return w
}

// checkSignature validates that got is a well-formed "t=<ts>,v1=<hex>"
// signature envelope (see webhook/http.sign) whose HMAC matches secret and
// payload.
func checkSignature(t *testing.T, secret string, payload []byte, got string) {
	t.Helper()

	tsPart, macPart, ok := strings.Cut(got, ",")
	if !ok {
		t.Fatalf("signature %q has no ',' separator", got)
	}

	tsStr, ok := strings.CutPrefix(tsPart, "t=")
	if !ok {
		t.Fatalf("signature %q missing t= field", got)
	}

	macHex, ok := strings.CutPrefix(macPart, "v1=")
	if !ok {
		t.Fatalf("signature %q missing v1= field", got)
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(tsStr))
	mac.Write([]byte{'.'})
	mac.Write(payload)
	want := hex.EncodeToString(mac.Sum(nil))

	if macHex != want {
		t.Errorf("signature mac = %q want %q (full signature %q)", macHex, want, got)
	}
}

func TestOpen_invalidOptions(t *testing.T) {
	t.Parallel()

	w, err := New(webhook.Options{Timeout: -time.Second})
	if !errors.Is(err, webhook.ErrInvalidOptions) {
		t.Fatalf("Open err = %v, want ErrInvalidOptions", err)
	}

	if w != nil {
		t.Fatalf("Open webhook = %v, want nil", w)
	}

	if !strings.Contains(err.Error(), "sqlite: ") {
		t.Errorf("Open err %q missing %q prefix", err.Error(), "sqlite: ")
	}
}

func TestOpen_badDSN(t *testing.T) {
	t.Parallel()

	for _, dsn := range []string{"x;DROP TABLE", "a\nb", "a\rb", "a\x00b"} {
		t.Run("dsn:"+strings.ReplaceAll(dsn, "\x00", "<nul>"), func(t *testing.T) {
			t.Parallel()

			w, err := New(testOptions(dsn))
			if err == nil {
				_ = w.Close()
				t.Fatalf("New(%q) = nil, want error", dsn)
			}

			if w != nil {
				t.Fatalf("New(%q) webhook = %v, want nil", dsn, w)
			}
		})
	}
}

func TestOpen_malformedDSN(t *testing.T) {
	t.Parallel()

	// Passes validateDSN but fails eager NewConnector parsing.
	w, err := New(testOptions("file:x?%zz"))
	if err == nil {
		_ = w.Close()
		t.Fatal("New(malformed) = nil, want error")
	}

	if w != nil {
		t.Fatalf("New(malformed) webhook = %v, want nil", w)
	}
}

func TestOpen_unwritablePath(t *testing.T) {
	t.Parallel()

	dsn := filepath.Join(t.TempDir(), "no-such-dir", "w.db")

	w, err := New(testOptions(dsn))
	if err == nil {
		_ = w.Close()
		t.Fatalf("New(%q) = nil, want error", dsn)
	}

	if w != nil {
		t.Fatalf("Open webhook = %v, want nil", w)
	}
}

func TestOpen_corruptDBFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "w.db")
	if err := os.WriteFile(path, []byte("this is not a sqlite database"), 0o600); err != nil {
		t.Fatalf("WriteFile err = %v", err)
	}

	w, err := New(testOptions(path))
	if err == nil {
		_ = w.Close()
		t.Fatal("New(corrupt) = nil, want error")
	}

	if w != nil {
		t.Fatalf("New(corrupt) webhook = %v, want nil", w)
	}
}

func TestOpen_readOnlyDB(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "w.db")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open err = %v", err)
	}

	// A valid database deliberately missing webhook_targets, so Open's DDL
	// must attempt a write (IF NOT EXISTS is a no-op on an existing table).
	if _, err = raw.ExecContext(t.Context(), `CREATE TABLE other (id TEXT)`); err != nil {
		t.Fatalf("raw CREATE TABLE err = %v", err)
	}

	if err = raw.Close(); err != nil {
		t.Fatalf("raw Close err = %v", err)
	}

	if err = os.Chmod(path, 0o444); err != nil {
		t.Fatalf("Chmod err = %v", err)
	}

	w, err := New(testOptions(path))
	if err == nil {
		_ = w.Close()
		t.Fatal("New(readonly) = nil, want DDL failure")
	}

	if w != nil {
		t.Fatalf("New(readonly) webhook = %v, want nil", w)
	}
}

func TestOpen_memoryIsolated(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	a := openTest(t, testOptions(""))
	if err := a.Register(t.Context(), "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	b := openTest(t, testOptions(""))
	if err := b.Deliver(t.Context(), "e", []byte(`{}`)); !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("Deliver on fresh memory DB err = %v, want ErrNotFound", err)
	}
}

func TestOpen_rejectsStalePrivateRow(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "w.db")

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("sql.Open err = %v", err)
	}

	if _, err = raw.ExecContext(t.Context(), schema); err != nil {
		t.Fatalf("raw schema err = %v", err)
	}

	if _, err = raw.ExecContext(t.Context(), `INSERT INTO webhook_targets (event, target, secret) VALUES (?, ?, ?)`,
		"e", "http://127.0.0.1:9/hook", "s"); err != nil {
		t.Fatalf("raw INSERT err = %v", err)
	}

	if err = raw.Close(); err != nil {
		t.Fatalf("raw Close err = %v", err)
	}

	w, err := New(webhook.Options{Timeout: 5 * time.Second, DSN: path})
	if err == nil {
		_ = w.Close()
		t.Fatal("Open with stale private row = nil, want loud failure")
	}

	if w != nil {
		t.Fatalf("Open webhook = %v, want nil", w)
	}
}

func TestRegister_ok(t *testing.T) {
	t.Parallel()

	var gotBody atomic.Value

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody.Store(string(body))
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openTest(t, testOptions(""))
	if err := w.Register(t.Context(), "order.created", srv.URL, "s3cr3t"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err := w.Deliver(t.Context(), "order.created", []byte(`{"id":1}`)); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}

	if got, _ := gotBody.Load().(string); got != `{"id":1}` {
		t.Fatalf("body = %q want %q", got, `{"id":1}`)
	}
}

func TestRegister_empty(t *testing.T) {
	t.Parallel()

	w := openTest(t, testOptions(""))

	for _, tc := range []struct {
		name   string
		event  string
		target string
	}{
		{"empty event", "", "http://127.0.0.1:9/hook"},
		{"empty target", "e", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if err := w.Register(t.Context(), tc.event, tc.target, "s"); err == nil {
				t.Fatal("Register = nil, want error")
			}
		})
	}
}

func TestRegister_invalidTarget(t *testing.T) {
	t.Parallel()

	w := openTest(t, testOptions(""))

	for _, target := range []string{"ftp://example.com/hook", "://bad-url", "https:///path"} {
		if err := w.Register(t.Context(), "e", target, "s"); err == nil {
			t.Errorf("Register(%q) = nil, want error", target)
		}
	}

	strict := openTest(t, webhook.Options{Timeout: 5 * time.Second, DSN: ""})
	if err := strict.Register(t.Context(), "e", "https://127.0.0.1/hook", "s"); err == nil {
		t.Error("Register(private, strict) = nil, want SSRF refusal")
	}
}

func TestRegister_dbFailureRollsBackInner(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w, err := New(testOptions(""))
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	t.Cleanup(func() { _ = w.Close() })

	if err := w.Register(t.Context(), "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	a, ok := w.(*adapter)
	if !ok {
		t.Fatalf("webhook %T is not *adapter", w)
	}

	if err := a.db.Close(); err != nil {
		t.Fatalf("db.Close err = %v", err)
	}

	if err := w.Register(t.Context(), "e2", srv.URL+"/2", "s2"); err == nil {
		t.Fatal("Register with closed DB = nil, want error")
	} else if !strings.Contains(err.Error(), "sqlite: register: ") {
		t.Errorf("Register err %q missing %q prefix", err.Error(), "sqlite: register: ")
	}

	if err := w.Deliver(t.Context(), "e2", []byte(`{}`)); !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("Deliver(e2) err = %v, want ErrNotFound (inner rollback)", err)
	}

	if err := w.Deliver(t.Context(), "e", []byte(`{}`)); err != nil {
		t.Fatalf("Deliver(e) err = %v, want nil (rollback scoped to failed Register)", err)
	}

	if hits.Load() != 1 {
		t.Fatalf("hits = %d want 1", hits.Load())
	}
}

func TestUnregister_ok(t *testing.T) {
	t.Parallel()

	w := openTest(t, testOptions(""))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	if err := w.Register(t.Context(), "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err := w.Unregister(t.Context(), "e", srv.URL); err != nil {
		t.Fatalf("Unregister err = %v", err)
	}

	if err := w.Deliver(t.Context(), "e", []byte(`{}`)); !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("Deliver after Unregister err = %v, want ErrNotFound", err)
	}
}

func TestUnregister_missing(t *testing.T) {
	t.Parallel()

	w := openTest(t, testOptions(""))

	if err := w.Unregister(t.Context(), "nope", "http://127.0.0.1:9/x"); !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("Unregister err = %v, want ErrNotFound", err)
	}
}

func TestUnregister_double(t *testing.T) {
	t.Parallel()

	w := openTest(t, testOptions(""))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	if err := w.Register(t.Context(), "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err := w.Unregister(t.Context(), "e", srv.URL); err != nil {
		t.Fatalf("first Unregister err = %v", err)
	}

	if err := w.Unregister(t.Context(), "e", srv.URL); !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("second Unregister err = %v, want ErrNotFound", err)
	}
}

func TestDeliver_successHeadersAndSignature(t *testing.T) {
	t.Parallel()

	const secret = "topsecret"

	var (
		mu      sync.Mutex
		headers http.Header
		body    []byte
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)

		mu.Lock()
		headers = r.Header.Clone()
		body = b
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openTest(t, testOptions(""))
	if err := w.Register(t.Context(), "order.created", srv.URL, secret); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	payload := []byte(`{"id":7}`)
	if err := w.Deliver(t.Context(), "order.created", payload); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if string(body) != string(payload) {
		t.Errorf("body = %q want %q", body, payload)
	}

	if got := headers.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q want %q", got, "application/json")
	}

	checkSignature(t, secret, payload, headers.Get("X-Hub-Signature-256"))
}

func TestDeliver_emptySecretOmitsSignature(t *testing.T) {
	t.Parallel()

	var (
		mu  sync.Mutex
		sig string
		hit bool
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sig = r.Header.Get("X-Hub-Signature-256")
		hit = true
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openTest(t, testOptions(""))
	if err := w.Register(t.Context(), "e", srv.URL, ""); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err := w.Deliver(t.Context(), "e", []byte(`{}`)); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if !hit {
		t.Fatal("server was not hit")
	}

	if sig != "" {
		t.Errorf("X-Hub-Signature-256 = %q want empty (no secret)", sig)
	}
}

func TestDeliver_overwriteSecret(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex

	var sig string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		sig = r.Header.Get("X-Hub-Signature-256")
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openTest(t, testOptions(""))
	if err := w.Register(t.Context(), "e", srv.URL, "old"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err := w.Register(t.Context(), "e", srv.URL, "new"); err != nil {
		t.Fatalf("re-Register err = %v", err)
	}

	payload := []byte(`{}`)
	if err := w.Deliver(t.Context(), "e", payload); err != nil {
		t.Fatalf("Deliver err = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	checkSignature(t, "new", payload, sig)
}

func TestDeliver_retry(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	o := testOptions("")
	o.MaxRetries = 3

	w := openTest(t, o)
	if err := w.Register(t.Context(), "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err := w.Deliver(t.Context(), "e", []byte(`{}`)); err != nil {
		t.Fatalf("Deliver err = %v, want success after retry", err)
	}

	if hits.Load() != 2 {
		t.Fatalf("hits = %d want 2", hits.Load())
	}
}

func TestDeliver_unknownEvent(t *testing.T) {
	t.Parallel()

	w := openTest(t, testOptions(""))

	if err := w.Deliver(t.Context(), "nope", []byte(`{}`)); !errors.Is(err, webhook.ErrNotFound) {
		t.Fatalf("Deliver err = %v, want ErrNotFound", err)
	}
}

func TestPersistence_reopenDelivers(t *testing.T) {
	t.Parallel()

	const secret = "persistsecret"

	payload := []byte(`{"durable":true}`)

	var (
		mu  sync.Mutex
		sig string
		got []byte
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)

		mu.Lock()
		sig = r.Header.Get("X-Hub-Signature-256")
		got = b
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	path := filepath.Join(t.TempDir(), "w.db")
	o := testOptions(path)

	w, err := New(o)
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	if err = w.Register(t.Context(), "e", srv.URL, secret); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	if err = w.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	reopened, err := New(o)
	if err != nil {
		t.Fatalf("reopen err = %v", err)
	}

	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("Close err = %v", err)
		}
	})

	if err := reopened.Deliver(t.Context(), "e", payload); err != nil {
		t.Fatalf("Deliver after reopen err = %v (durability broken)", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if string(got) != string(payload) {
		t.Errorf("body = %q want %q", got, payload)
	}

	checkSignature(t, secret, payload, sig)
}

func TestUnregister_dbFailure(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w, err := New(testOptions(""))
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	t.Cleanup(func() { _ = w.Close() })

	if err := w.Register(t.Context(), "e", srv.URL, "s"); err != nil {
		t.Fatalf("Register err = %v", err)
	}

	a, ok := w.(*adapter)
	if !ok {
		t.Fatalf("webhook %T is not *adapter", w)
	}

	if err := a.db.Close(); err != nil {
		t.Fatalf("db.Close err = %v", err)
	}

	if err := w.Unregister(t.Context(), "e", srv.URL); err == nil {
		t.Fatal("Unregister with closed DB = nil, want error")
	} else if !strings.Contains(err.Error(), "sqlite: unregister: ") {
		t.Errorf("Unregister err %q missing %q prefix", err.Error(), "sqlite: unregister: ")
	}
}

func TestLoad_queryFailure(t *testing.T) {
	t.Parallel()

	w, err := New(testOptions(""))
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	t.Cleanup(func() { _ = w.Close() })

	a, ok := w.(*adapter)
	if !ok {
		t.Fatalf("webhook %T is not *adapter", w)
	}

	if err := a.db.Close(); err != nil {
		t.Fatalf("db.Close err = %v", err)
	}

	if err := a.load(); err == nil {
		t.Fatal("load on closed DB = nil, want error")
	}
}

func TestClose_double(t *testing.T) {
	t.Parallel()

	w, err := New(testOptions(""))
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("first Close err = %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("second Close err = %v, want nil (idempotent)", err)
	}
}

func TestConcurrent_registerDeliver(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	w := openTest(t, testOptions(""))

	const n = 10

	var wg sync.WaitGroup

	errs := make(chan error, 2*n)

	for i := range n {
		wg.Add(1)

		go func() {
			defer wg.Done()

			event := "event-" + strings.Repeat("x", i+1)
			if err := w.Register(t.Context(), event, srv.URL, "s"); err != nil {
				errs <- err
				return
			}

			if err := w.Deliver(t.Context(), event, []byte(`{}`)); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent err = %v", err)
	}
}

var errLoadBoom = errors.New("boom")

// stubInner is a no-op webhook.Webhook for white-box load tests.
type stubInner struct{}

func (stubInner) Register(context.Context, string, string, string) error { return nil }
func (stubInner) Unregister(context.Context, string, string) error       { return nil }
func (stubInner) Deliver(context.Context, string, []byte) error          { return nil }
func (stubInner) Close() error                                           { return nil }

// loadConnector builds *sql.DB instances backed by a scripted driver so
// load's row-iteration failure branches stay covered without driver faults.
type loadConnector struct {
	nextErr error
	oneCol  bool
}

func (c loadConnector) Connect(context.Context) (driver.Conn, error) {
	return loadConn(c), nil
}

func (c loadConnector) Driver() driver.Driver { return loadDriver{} }

type loadDriver struct{}

func (loadDriver) Open(string) (driver.Conn, error) { return loadConn{}, nil }

type loadConn struct {
	nextErr error
	oneCol  bool
}

func (c loadConn) Prepare(string) (driver.Stmt, error) { return nil, errLoadBoom }
func (c loadConn) Close() error                        { return nil }
func (c loadConn) Begin() (driver.Tx, error)           { return nil, errLoadBoom }

func (c loadConn) QueryContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	return &loadRows{nextErr: c.nextErr, oneCol: c.oneCol}, nil
}

type loadRows struct {
	nextErr error
	oneCol  bool
	done    bool
}

func (r *loadRows) Columns() []string {
	if r.oneCol {
		return []string{"c"}
	}

	return []string{"event", "target", "secret"}
}

func (r *loadRows) Close() error { return nil }

func (r *loadRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}

	r.done = true

	if r.nextErr != nil {
		return r.nextErr
	}

	if len(dest) > 0 {
		dest[0] = "e"
	}

	if len(dest) > 1 {
		dest[1] = "t"
	}

	if len(dest) > 2 {
		dest[2] = "s"
	}

	return nil
}

func TestLoad_rowsErr(t *testing.T) {
	t.Parallel()

	a := &adapter{db: sql.OpenDB(loadConnector{nextErr: errLoadBoom}), inner: stubInner{}}

	if err := a.load(); !errors.Is(err, errLoadBoom) {
		t.Fatalf("load err = %v, want boom", err)
	}
}

func TestLoad_scanMismatch(t *testing.T) {
	t.Parallel()

	// One-column rows against a three-destination Scan: database/sql rejects
	// the arity mismatch deterministically.
	a := &adapter{db: sql.OpenDB(loadConnector{oneCol: true}), inner: stubInner{}}

	if err := a.load(); err == nil {
		t.Fatal("load = nil, want scan error")
	}
}

func TestLoad_ok(t *testing.T) {
	t.Parallel()

	a := &adapter{db: sql.OpenDB(loadConnector{}), inner: stubInner{}}

	if err := a.load(); err != nil {
		t.Fatalf("load err = %v", err)
	}
}
