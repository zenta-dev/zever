package zevertest

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	analyticslog "github.com/zenta-dev/zever/adapters/analytics/log"
	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	eventbusmemory "github.com/zenta-dev/zever/adapters/eventbus/memory"
	flagstatic "github.com/zenta-dev/zever/adapters/flag/static"
	idempotencymemory "github.com/zenta-dev/zever/adapters/idempotency/memory"
	lockmemory "github.com/zenta-dev/zever/adapters/lock/memory"
	lognoop "github.com/zenta-dev/zever/adapters/log/noop"
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
	"github.com/zenta-dev/zever/core/analytics"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/core/session"
	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/core/tenant"
)

// TestJWTSecret is the HMAC secret NewTestContainer configures for the jwt
// auth adapter (32 bytes, the adapter's minimum). Test-only, never a
// production value.
const TestJWTSecret = "zevertest-jwt-secret-0123456789ab" //nolint:gosec // fixed test-only secret, documented as such

// closeTimeout bounds container shutdown at the end of a test.
const closeTimeout = 10 * time.Second

type settings struct {
	configure []func(*config.Config)
	extra     []container.Provider
}

// Option customizes NewTestContainer.
type Option func(*settings)

// WithConfig applies fn to the test config after the zevertest defaults, so a
// test can tweak options. Providers seed their Options from this config, so
// config edits apply.
func WithConfig(fn func(*config.Config)) Option {
	return func(s *settings) { s.configure = append(s.configure, fn) }
}

// WithProviders appends providers resolved alongside (after) the
// zero-infrastructure set NewTestContainer wires, so a test can override a
// battery or wire one zevertest leaves out (for example a hand-written fake).
// A duplicate Ref fails the container build; use it to replace, not repeat.
func WithProviders(ps ...container.Provider) Option {
	return func(s *settings) { s.extra = append(s.extra, ps...) }
}

// defaultProviders wires the zero-infrastructure adapters NewTestContainer
// resolves. Each entry names the compiled-in constructor directly: adding a
// battery here links it into the test binary, nothing else does. Options
// come from cfg (already the test's merged config): the applier overwrites
// the provider's env-seeded defaults so cfg stays authoritative, exactly as
// when the container resolved everything from cfg itself.
func defaultProviders(cfg *config.Config) []container.Provider {
	return []container.Provider{
		container.Provide(analytics.DefaultRef(), analyticslog.New,
			container.WithOptions(func(o *analytics.Options) { *o = cfg.Analytics.Options })),
		container.Provide(auth.DefaultRef(), authjwt.New,
			container.WithOptions(func(o *auth.Options) { *o = cfg.Auth.Options })),
		container.Provide(cache.DefaultRef(), cachememory.New,
			container.WithOptions(func(o *cache.Options) { *o = cfg.Cache.Options })),
		container.Provide(crypto.DefaultRef(), cryptolocal.New,
			container.WithOptions(func(o *crypto.Options) { *o = cfg.Crypto.Options })),
		container.Provide(db.DefaultRef(), dbsqlite.New,
			container.WithOptions(func(o *db.Options) { *o = cfg.DB.Options })),
		container.Provide(eventbus.DefaultRef(), eventbusmemory.New,
			container.WithOptions(func(o *eventbus.Options) { *o = cfg.EventBus.Options })),
		container.Provide(flag.DefaultRef(), flagstatic.New,
			container.WithOptions(func(o *flag.Options) { *o = cfg.Flag.Options })),
		container.Provide(idempotency.DefaultRef(), idempotencymemory.New,
			container.WithOptions(func(o *idempotency.Options) { *o = cfg.Idempotency.Options })),
		container.Provide(lock.DefaultRef(), lockmemory.New,
			container.WithOptions(func(o *lock.Options) { *o = cfg.Lock.Options })),
		container.Provide(log.DefaultRef(), container.WrapNoOpts[log.Logger, log.Options](lognoop.New)),
		container.Provide(mailer.DefaultRef(), mailerlog.New,
			container.WithOptions(func(o *mailer.Options) { *o = cfg.Mailer.Options })),
		container.Provide(notification.DefaultRef(), notificationlog.New,
			container.WithOptions(func(o *notification.Options) { *o = cfg.Notification.Options })),
		container.Provide(observability.DefaultRef(), container.WrapNoOpts[observability.Provider, observability.Options](observabilitynoop.New)),
		container.Provide(password.DefaultRef(), passwordargon2.New,
			container.WithOptions(func(o *password.Options) { *o = cfg.Password.Options })),
		container.Provide(permission.DefaultRef(), permissionnoop.New,
			container.WithOptions(func(o *permission.Options) { *o = cfg.Permission.Options })),
		container.Provide(queue.DefaultRef(), queuememory.New,
			container.WithOptions(func(o *queue.Options) { *o = cfg.Queue.Options })),
		container.Provide(ratelimit.DefaultRef(), ratelimitmemory.New,
			container.WithOptions(func(o *ratelimit.Options) { *o = cfg.RateLimit.Options })),
		container.Provide(router.DefaultRef(), routerstdhttp.New,
			container.WithOptions(func(o *router.Options) { *o = cfg.Router.Options })),
		container.Provide(session.DefaultRef(), sessionmemory.New,
			container.WithOptions(func(o *session.Options) { *o = cfg.Session.Options })),
		container.Provide(storage.DefaultRef(), storagelocal.New,
			container.WithOptions(func(o *storage.Options) { *o = cfg.Storage.Options })),
		container.Provide(tenant.DefaultRef(), tenantsingle.New,
			container.WithOptions(func(o *tenant.Options) { *o = cfg.Tenant.Options })),
	}
}

// NewTestContainer builds a container for t over zero-infrastructure
// adapters and closes it when the test ends.
//
// It starts from config.Default() and then isolates the test: sqlite and
// local storage live under tb.TempDir() (so parallel tests, -shuffle and
// -count=N never share state), logging and observability are no-ops (quiet
// output), and auth uses TestJWTSecret. Services still resolve lazily, so a
// test only pays for what it asks the container for. Use WithConfig and
// WithProviders to deviate.
//
// The returned container is safe for use from the test's goroutines; do not
// share it across tests.
func NewTestContainer(tb testing.TB, opts ...Option) *container.Container {
	tb.Helper()

	var s settings
	for _, o := range opts {
		o(&s)
	}

	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(tb.TempDir(), "test.db")
	cfg.Storage.Options.Root = filepath.Join(tb.TempDir(), "storage")
	cfg.Auth.Options.JWT.Secret = TestJWTSecret

	for _, fn := range s.configure {
		fn(cfg)
	}

	c := container.New(cfg, append(defaultProviders(cfg), s.extra...)...)

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
