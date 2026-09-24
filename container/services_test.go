package container

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"google.golang.org/grpc"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/i18n"
)

// testConfig returns Default plus the dev values Default leaves empty:
// the AI adapter requires an API key, JWT auth requires a secret plus
// issuer, the static geo adapter requires a cities file, and the embed
// i18n adapter requires a catalog FS. All fixtures are local and
// deterministic; nothing touches the network.
func testConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg := config.Default()
	cfg.AI.Options.APIKey = "test-key"
	cfg.Auth.Options.JWT.Secret = "test-secret-0123456789abcdef01234567"
	cfg.Auth.Options.JWT.Issuer = "zever-test"

	cities := `[{"name":"Testville","lat":1.0,"lng":2.0}]`
	path := filepath.Join(t.TempDir(), "cities.json")
	if err := os.WriteFile(path, []byte(cities), 0o600); err != nil {
		t.Fatalf("write cities fixture: %v", err)
	}

	cfg.Geo.Options.Path = path
	cfg.DB.Options.Path = ":memory:"
	cfg.I18n.Options.Embed = i18n.EmbedOptions{
		FS:       fstest.MapFS{"en.json": {Data: []byte(`{"hello":"Hello"}`)}},
		Dir:      ".",
		Fallback: "en",
	}

	return cfg
}

// closeContainer shuts down resolved services so the package goleak check
// sees no leftover background work (cache janitors, queues, servers).
func closeContainer(t *testing.T, c *Container) {
	t.Helper()

	t.Cleanup(func() {
		if err := c.Close(t.Context()); err != nil {
			t.Logf("close: %v", err)
		}
	})
}

// sameInstance fails the test when two resolutions of one accessor are not
// the same singleton instance.
func sameInstance(t *testing.T, a, b any) {
	t.Helper()

	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	if !va.IsValid() || !vb.IsValid() {
		t.Fatal("nil service instance")
	}

	if va.Type() != vb.Type() {
		t.Fatalf("type changed between calls: %T vs %T", a, b)
	}

	if va.Kind() == reflect.Pointer {
		if va.Pointer() != vb.Pointer() {
			t.Error("singleton identity violated: distinct pointers")
		}

		return
	}

	if a != b {
		t.Error("singleton identity violated: distinct values")
	}
}

func TestAccessors_resolve(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	tests := []struct {
		name    string
		resolve func() (any, error)
	}{
		{"ai", func() (any, error) { return c.AI() }},
		{"analytics", func() (any, error) { return c.Analytics() }},
		{"auth", func() (any, error) { return c.Auth() }},
		{"billing", func() (any, error) { return c.Billing() }},
		{"cache", func() (any, error) { return c.Cache() }},
		{"crypto", func() (any, error) { return c.Crypto() }},
		{"db", func() (any, error) { return c.DB() }},
		{"document", func() (any, error) { return c.Document() }},
		{"eventbus", func() (any, error) { return c.EventBus() }},
		{"flag", func() (any, error) { return c.Flag() }},
		{"geo", func() (any, error) { return c.Geo() }},
		{"i18n", func() (any, error) { return c.I18n() }},
		{"idempotency", func() (any, error) { return c.Idempotency() }},
		{"lock", func() (any, error) { return c.Lock() }},
		{"log", func() (any, error) { return c.Log() }},
		{"mailer", func() (any, error) { return c.Mailer() }},
		{"media", func() (any, error) { return c.Media() }},
		{"notification", func() (any, error) { return c.Notification() }},
		{"observability", func() (any, error) { return c.Observability() }},
		{"password", func() (any, error) { return c.Password() }},
		{"payment", func() (any, error) { return c.Payment() }},
		{"permission", func() (any, error) { return c.Permission() }},
		{"queue", func() (any, error) { return c.Queue() }},
		{"ratelimit", func() (any, error) { return c.RateLimit() }},
		{"router", func() (any, error) { return c.Router() }},
		{"search", func() (any, error) { return c.Search() }},
		{"secrets", func() (any, error) { return c.Secrets() }},
		{"session", func() (any, error) { return c.Session() }},
		{"storage", func() (any, error) { return c.Storage() }},
		{"tenant", func() (any, error) { return c.Tenant() }},
		{"vectorstore", func() (any, error) { return c.VectorStore() }},
		{"webhook", func() (any, error) { return c.Webhook() }},
		{"workflow", func() (any, error) { return c.Workflow() }},
	}

	// Sequential: subtests share one container, and the parent's cleanup
	// Close must run after all of them finish.
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v, err := tt.resolve()
			if err != nil {
				t.Fatalf("resolve failed: %v", err)
			}

			if v == nil {
				t.Fatal("resolve returned nil instance")
			}

			sameInstance(t, v, mustResolve(t, tt.resolve))
		})
	}
}

func TestAccessors_alternateAdapters(t *testing.T) {
	t.Parallel()

	// Covers the infallible factory wrappers (log noop/zerolog,
	// observability noop) that defaults never exercise.
	tests := []struct {
		name    string
		mutate  func(cfg *config.Config)
		resolve func(c *Container) (any, error)
	}{
		{"log/noop", func(cfg *config.Config) { cfg.Log.Adapter = "noop" }, func(c *Container) (any, error) { return c.Log() }},
		{"log/zerolog", func(cfg *config.Config) { cfg.Log.Adapter = "zerolog" }, func(c *Container) (any, error) { return c.Log() }},
		{"observability/noop", func(cfg *config.Config) { cfg.Observability.Adapter = "noop" }, func(c *Container) (any, error) { return c.Observability() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := testConfig(t)
			tt.mutate(cfg)

			c := New(cfg)
			closeContainer(t, c)

			v, err := tt.resolve(c)
			if err != nil {
				t.Fatalf("resolve failed: %v", err)
			}

			if v == nil {
				t.Fatal("resolve returned nil instance")
			}
		})
	}
}

func mustResolve(t *testing.T, resolve func() (any, error)) any {
	t.Helper()

	v, err := resolve()
	if err != nil {
		t.Fatalf("second resolve failed: %v", err)
	}

	return v
}

func TestAccessors_lazy(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	if c.ai.resolved() || c.db.resolved() || c.cache.resolved() {
		t.Fatal("fresh container has resolved services")
	}

	if _, err := c.AI(); err != nil {
		t.Fatalf("AI() failed: %v", err)
	}

	if !c.ai.resolved() {
		t.Error("AI() did not mark ai resolved")
	}

	if c.db.resolved() || c.cache.resolved() {
		t.Error("resolving AI resolved unrelated services")
	}
}

func TestAccessors_badAdapterThenRetry(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	c.cfg.DB.Adapter = "bogus-adapter"

	if _, err := c.DB(); err == nil {
		t.Fatal("expected error for bogus db adapter, got nil")
	}

	if c.db.resolved() {
		t.Fatal("failed build must not mark the service resolved")
	}

	c.cfg.DB.Adapter = "sqlite"

	if _, err := c.DB(); err != nil {
		t.Fatalf("retry after config fix failed: %v", err)
	}
}

func TestAccessors_badAdapterError(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	c.cfg.Cache.Adapter = "bogus-adapter"

	_, err := c.Cache()
	if err == nil {
		t.Fatal("expected error for bogus cache adapter, got nil")
	}

	if !strings.Contains(err.Error(), "cache") {
		t.Errorf("error should name the service, got: %v", err)
	}
}

func TestCrypto_failureCarriesNoSecretText(t *testing.T) {
	t.Parallel()

	const marker = "s3cr3t-marker-zz9"

	c := New(testConfig(t))
	closeContainer(t, c)

	c.cfg.Crypto.Options.Key = "NOT-BASE64!!-" + marker

	_, err := c.Crypto()
	if err == nil {
		t.Fatal("expected error for invalid crypto key, got nil")
	}

	if strings.Contains(err.Error(), marker) {
		t.Errorf("error leaks secret text: %v", err)
	}
}

func TestAccessors_concurrent(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	const n = 8

	results := make([]any, n)
	errs := make([]error, n)

	var wg sync.WaitGroup

	for i := range n {
		wg.Add(1)

		go func() {
			defer wg.Done()

			v, err := c.Cache()
			results[i] = v
			errs[i] = err
		}()
	}

	wg.Wait()

	for i := range n {
		if errs[i] != nil {
			t.Fatalf("goroutine %d failed: %v", i, errs[i])
		}

		sameInstance(t, results[0], results[i])
	}
}

func TestJob_sharesQueue(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	q, err := c.Queue()
	if err != nil {
		t.Fatalf("Queue() failed: %v", err)
	}

	d, err := c.Job()
	if err != nil {
		t.Fatalf("Job() failed: %v", err)
	}

	if d == nil {
		t.Fatal("Job() returned nil dispatcher")
	}

	if d.Q != q {
		t.Error("dispatcher does not share the resolved queue instance")
	}

	again, err := c.Job()
	if err != nil {
		t.Fatalf("second Job() failed: %v", err)
	}

	if again != d {
		t.Error("Job() is not a singleton")
	}
}

func TestJob_queueFailure(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	c.cfg.Queue.Adapter = "bogus-adapter"

	if _, err := c.Job(); err == nil {
		t.Fatal("expected Job error for bogus queue adapter, got nil")
	}
}

func TestScheduler_resolve(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	s, err := c.Scheduler()
	if err != nil {
		t.Fatalf("Scheduler() failed: %v", err)
	}

	if s == nil {
		t.Fatal("Scheduler() returned nil")
	}

	if got := s.Name(); got != "embedded" {
		t.Errorf("Name() = %q, want %q", got, "embedded")
	}

	again, err := c.Scheduler()
	if err != nil {
		t.Fatalf("second Scheduler() failed: %v", err)
	}

	if again != s {
		t.Error("Scheduler() is not a singleton")
	}
}

func TestScheduler_injectsJobDispatcher(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	cfg.Scheduler.Options.Dispatcher = nil

	c := New(cfg)
	closeContainer(t, c)

	s, err := c.Scheduler()
	if err != nil {
		t.Fatalf("Scheduler() with nil dispatcher failed: %v", err)
	}

	if s == nil {
		t.Fatal("Scheduler() returned nil")
	}

	if !c.job.resolved() {
		t.Error("nil dispatcher should resolve the shared job dispatcher")
	}
}

func TestScheduler_jobFailure(t *testing.T) {
	t.Parallel()

	cfg := testConfig(t)
	cfg.Scheduler.Options.Dispatcher = nil
	cfg.Queue.Adapter = "bogus-adapter"

	c := New(cfg)
	closeContainer(t, c)

	if _, err := c.Scheduler(); err == nil {
		t.Fatal("expected Scheduler error for bogus queue adapter, got nil")
	}
}

func TestScheduler_badAdapter(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	c.cfg.Scheduler.Adapter = "bogus-adapter"

	if _, err := c.Scheduler(); err == nil {
		t.Fatal("expected error for bogus scheduler adapter, got nil")
	}
}

func TestTransactor_ok(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	tr, err := c.Transactor()
	if err != nil {
		t.Fatalf("Transactor() failed: %v", err)
	}

	if tr == nil {
		t.Fatal("Transactor() returned nil")
	}
}

func TestTransactor_dbFailure(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	c.cfg.DB.Adapter = "bogus-adapter"

	if _, err := c.Transactor(); err == nil {
		t.Fatal("expected Transactor error for bogus db adapter, got nil")
	}
}

// stubDB implements db.DB by embedding it but never Transactor, so
// Transactor must reject it. Seeded through lazy.get, which is the only
// same-package seam the frozen lazy contract exposes.
type stubDB struct {
	db.DB
}

func TestTransactor_unsupported(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	if _, err := c.db.get(func() (db.DB, error) { return stubDB{}, nil }); err != nil {
		t.Fatalf("seeding stub db failed: %v", err)
	}

	_, err := c.Transactor()
	if err == nil {
		t.Fatal("expected Transactor error for non-transactor db, got nil")
	}

	var terr TransactorError
	if !errors.As(err, &terr) {
		t.Fatalf("error is %T, want TransactorError", err)
	}

	if !strings.Contains(terr.Actual, "stubDB") {
		t.Errorf("TransactorError.Actual = %q, want it to name stubDB", terr.Actual)
	}

	if !errors.Is(err, ErrTransactorUnsupported) {
		t.Errorf("error should unwrap to ErrTransactorUnsupported, got: %v", err)
	}
}

func TestGRPC_singletonOptsIgnored(t *testing.T) {
	t.Parallel()

	c := New(testConfig(t))
	closeContainer(t, c)

	if c.grpcServer.resolved() {
		t.Fatal("fresh container has grpc server resolved")
	}

	first, err := c.GRPC()
	if err != nil {
		t.Fatalf("GRPC() failed: %v", err)
	}

	if first == nil {
		t.Fatal("GRPC() returned nil server")
	}

	second, err := c.GRPC(grpc.MaxRecvMsgSize(1))
	if err != nil {
		t.Fatalf("second GRPC() failed: %v", err)
	}

	if second != first {
		t.Error("GRPC() is not a singleton: later options built a new server")
	}
}
