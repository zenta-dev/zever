package zevertest

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	analyticslog "github.com/zenta-dev/zever/adapters/analytics/log"
	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	authsession "github.com/zenta-dev/zever/adapters/auth/session"
	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	eventbusmemory "github.com/zenta-dev/zever/adapters/eventbus/memory"
	flagstatic "github.com/zenta-dev/zever/adapters/flag/static"
	idempotencymemory "github.com/zenta-dev/zever/adapters/idempotency/memory"
	lockmemory "github.com/zenta-dev/zever/adapters/lock/memory"
	lognoop "github.com/zenta-dev/zever/adapters/log/noop"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	observabilitynoop "github.com/zenta-dev/zever/adapters/observability/noop"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	permissionnoop "github.com/zenta-dev/zever/adapters/permission/noop"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	ratelimitmemory "github.com/zenta-dev/zever/adapters/ratelimit/memory"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	storagelocal "github.com/zenta-dev/zever/adapters/storage/local"
	tenantsingle "github.com/zenta-dev/zever/adapters/tenant/single"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
)

// TestJWTSecret is the HMAC secret NewTestContainer configures for the jwt
// auth adapter (32 bytes, the adapter's minimum). Test-only, never a
// production value.
const TestJWTSecret = "zevertest-jwt-secret-0123456789ab" //nolint:gosec // fixed test-only secret, documented as such

// closeTimeout bounds container shutdown at the end of a test.
const closeTimeout = 10 * time.Second

type settings struct {
	configure []func(*config.Config)
	register  []func()
}

// Option customizes NewTestContainer.
type Option func(*settings)

// WithConfig applies fn to the test config after the zevertest defaults, so a
// test can switch an adapter or tweak options, for example
// cfg.Cache.Adapter = "redis" against a registered fake.
func WithConfig(fn func(*config.Config)) Option {
	return func(s *settings) { s.configure = append(s.configure, fn) }
}

// WithRegister runs fn before the container is built, for registering
// adapters beyond the zero-infrastructure set NewTestContainer registers
// (for example a hand-written fake battery).
func WithRegister(fn func()) Option {
	return func(s *settings) { s.register = append(s.register, fn) }
}

// registerDefaults registers the zero-infrastructure adapters
// NewTestContainer wires. Registration is idempotent: a repeated call (a
// second test, -count=N) leaves the registries unchanged.
func registerDefaults() {
	analyticslog.Register()
	authjwt.Register()
	authsession.Register()
	cachememory.Register()
	cryptolocal.Register()
	dbsqlite.Register()
	eventbusmemory.Register()
	flagstatic.Register()
	idempotencymemory.Register()
	lockmemory.Register()
	lognoop.Register()
	logslog.Register()
	mailerlog.Register()
	notificationlog.Register()
	observabilitynoop.Register()
	passwordargon2.Register()
	permissionnoop.Register()
	queuememory.Register()
	ratelimitmemory.Register()
	routerstdhttp.Register()
	sessionmemory.Register()
	storagelocal.Register()
	tenantsingle.Register()
}

// NewTestContainer builds a container for t over zero-infrastructure
// adapters and closes it when the test ends.
//
// It starts from config.Default() and then isolates the test: sqlite and
// local storage live under tb.TempDir() (so parallel tests, -shuffle and
// -count=N never share state), logging and observability are no-ops (quiet
// output), and auth uses TestJWTSecret. Services still resolve lazily, so a
// test only pays for what it asks the container for. Use WithConfig and
// WithRegister to deviate.
//
// The returned container is safe for use from the test's goroutines; do not
// share it across tests.
func NewTestContainer(tb testing.TB, opts ...Option) *container.Container {
	tb.Helper()

	var s settings
	for _, o := range opts {
		o(&s)
	}

	registerDefaults()

	for _, fn := range s.register {
		fn()
	}

	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(tb.TempDir(), "test.db")
	cfg.Storage.Options.Root = filepath.Join(tb.TempDir(), "storage")
	cfg.Log.Adapter = "noop"
	cfg.Observability.Adapter = "noop"
	cfg.Auth.Options.JWT.Secret = TestJWTSecret

	for _, fn := range s.configure {
		fn(cfg)
	}

	c := container.New(cfg)

	tb.Cleanup(func() {
		// t.Context() is canceled just before cleanups run, so closing needs
		// its own bounded context.
		ctx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()

		if err := c.Close(ctx); err != nil {
			tb.Errorf("zevertest: close container: %v", err)
		}
	})

	return c
}
