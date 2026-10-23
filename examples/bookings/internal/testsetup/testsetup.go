// Package testsetup wires every default adapter the bookings tests resolve.
// The container resolves only what is Provided, so tests build their
// container from Providers: one entry per battery naming the compiled-in
// constructor, exactly like the app's wiring.go. Options come from the
// passed config (the test's merged config), so per-test values (temp DB
// path, test JWT secret) apply.
package testsetup

import (
	aianthropic "github.com/zenta-dev/zever/adapters/ai/anthropic"
	analyticslog "github.com/zenta-dev/zever/adapters/analytics/log"
	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	billingstub "github.com/zenta-dev/zever/adapters/billing/stub"
	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	documentlocal "github.com/zenta-dev/zever/adapters/document/local"
	eventbusmemory "github.com/zenta-dev/zever/adapters/eventbus/memory"
	flagstatic "github.com/zenta-dev/zever/adapters/flag/static"
	geostatic "github.com/zenta-dev/zever/adapters/geo/static"
	i18nembed "github.com/zenta-dev/zever/adapters/i18n/embed"
	idempotencymemory "github.com/zenta-dev/zever/adapters/idempotency/memory"
	lockmemory "github.com/zenta-dev/zever/adapters/lock/memory"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	medialocal "github.com/zenta-dev/zever/adapters/media/local"
	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	observabilitystdout "github.com/zenta-dev/zever/adapters/observability/stdout"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	paymentstub "github.com/zenta-dev/zever/adapters/payment/stub"
	permissionrbac "github.com/zenta-dev/zever/adapters/permission/rbac"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	ratelimitmemory "github.com/zenta-dev/zever/adapters/ratelimit/memory"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	schedulerembedded "github.com/zenta-dev/zever/adapters/scheduler/embedded"
	searchdb "github.com/zenta-dev/zever/adapters/search/db"
	secretsenv "github.com/zenta-dev/zever/adapters/secrets/env"
	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	storagelocal "github.com/zenta-dev/zever/adapters/storage/local"
	tenantsingle "github.com/zenta-dev/zever/adapters/tenant/single"
	vectorstoredb "github.com/zenta-dev/zever/adapters/vectorstore/db"
	webhookhttp "github.com/zenta-dev/zever/adapters/webhook/http"
	workflowmemory "github.com/zenta-dev/zever/adapters/workflow/memory"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/analytics"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/document"
	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/media"
	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/core/scheduler"
	"github.com/zenta-dev/zever/core/search"
	"github.com/zenta-dev/zever/core/secrets"
	"github.com/zenta-dev/zever/core/session"
	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/core/tenant"
	"github.com/zenta-dev/zever/core/vectorstore"
	"github.com/zenta-dev/zever/core/webhook"
	"github.com/zenta-dev/zever/core/workflow"
	"github.com/zenta-dev/zever/shared/dbconn"
)

// Providers wires every default adapter the bookings tests resolve, seeded
// from cfg so per-test config (temp DB path, test JWT secret) applies.
// Build test containers with container.New(cfg, Providers(cfg)...).
func Providers(cfg *config.Config) []container.Provider {
	return []container.Provider{
		container.Provide(ai.DefaultRef(), aianthropic.New,
			container.WithOptions(func(o *ai.Options) { *o = cfg.AI.Options })),
		container.Provide(analytics.DefaultRef(), analyticslog.New,
			container.WithOptions(func(o *analytics.Options) { *o = cfg.Analytics.Options })),
		container.Provide(auth.DefaultRef(), authjwt.New,
			container.WithOptions(func(o *auth.Options) { *o = cfg.Auth.Options })),
		container.Provide(billing.DefaultRef(), func(o billing.Options) (billing.Billing, error) {
			return billingstub.New(), nil
		}),
		container.Provide(cache.DefaultRef(), cachememory.New,
			container.WithOptions(func(o *cache.Options) { *o = cfg.Cache.Options })),
		container.Provide(crypto.DefaultRef(), cryptolocal.New,
			container.WithOptions(func(o *crypto.Options) { *o = cfg.Crypto.Options })),
		container.Provide(db.DefaultRef(), dbsqlite.New,
			container.WithOptions(func(o *db.Options) { *o = cfg.DB.Options })),
		container.Provide(document.DefaultRef(), documentlocal.New,
			container.WithOptions(func(o *document.Options) { *o = cfg.Document.Options })),
		container.Provide(eventbus.DefaultRef(), eventbusmemory.New,
			container.WithOptions(func(o *eventbus.Options) { *o = cfg.EventBus.Options })),
		container.Provide(flag.DefaultRef(), flagstatic.New,
			container.WithOptions(func(o *flag.Options) { *o = cfg.Flag.Options })),
		container.Provide(geo.DefaultRef(), geostatic.New,
			container.WithOptions(func(o *geo.Options) { *o = cfg.Geo.Options })),
		container.Provide(i18n.DefaultRef(), i18nembed.New,
			container.WithOptions(func(o *i18n.Options) { *o = cfg.I18n.Options })),
		container.Provide(idempotency.DefaultRef(), idempotencymemory.New,
			container.WithOptions(func(o *idempotency.Options) { *o = cfg.Idempotency.Options })),
		container.Provide(lock.DefaultRef(), lockmemory.New,
			container.WithOptions(func(o *lock.Options) { *o = cfg.Lock.Options })),
		container.Provide(log.DefaultRef(), func(o log.Options) (log.Logger, error) {
			return logslog.New(o), nil
		}),
		container.Provide(mailer.DefaultRef(), mailerlog.New,
			container.WithOptions(func(o *mailer.Options) { *o = cfg.Mailer.Options })),
		container.Provide(media.DefaultRef(), medialocal.New,
			container.WithOptions(func(o *media.Options) { *o = cfg.Media.Options })),
		container.Provide(notification.DefaultRef(), notificationlog.New,
			container.WithOptions(func(o *notification.Options) { *o = cfg.Notification.Options })),
		container.Provide(observability.DefaultRef(), observabilitystdout.New,
			container.WithOptions(func(o *observability.Options) { *o = cfg.Observability.Options })),
		container.Provide(password.DefaultRef(), passwordargon2.New,
			container.WithOptions(func(o *password.Options) { *o = cfg.Password.Options })),
		container.Provide(payment.DefaultRef(), paymentstub.New),
		container.Provide(permission.DefaultRef(), permissionrbac.New,
			container.WithOptions(func(o *permission.Options) { *o = cfg.Permission.Options })),
		container.Provide(queue.DefaultRef(), queuememory.New,
			container.WithOptions(func(o *queue.Options) { *o = cfg.Queue.Options })),
		container.Provide(ratelimit.DefaultRef(), ratelimitmemory.New,
			container.WithOptions(func(o *ratelimit.Options) { *o = cfg.RateLimit.Options })),
		container.Provide(router.DefaultRef(), routerstdhttp.New,
			container.WithOptions(func(o *router.Options) { *o = cfg.Router.Options })),
		container.Provide(scheduler.DefaultRef(), schedulerembedded.New,
			container.WithOptions(func(o *scheduler.Options) { *o = cfg.Scheduler.Options })),
		container.Provide(search.DefaultRef(), func(o search.Options) (search.Search, error) {
			poolOpts := dbconn.SplitDSN(o.DSN)
			poolOpts.DedicatedPool = o.DedicatedPool
			return searchdb.New(searchdb.Options{Options: poolOpts})
		}),
		container.Provide(secrets.DefaultRef(), func(o secrets.Options) (secrets.Secrets, error) {
			return secretsenv.New(secretsenv.Options{Prefix: o.Prefix})
		}),
		container.Provide(session.DefaultRef(), sessionmemory.New,
			container.WithOptions(func(o *session.Options) { *o = cfg.Session.Options })),
		container.Provide(storage.DefaultRef(), storagelocal.New,
			container.WithOptions(func(o *storage.Options) { *o = cfg.Storage.Options })),
		container.Provide(tenant.DefaultRef(), tenantsingle.New,
			container.WithOptions(func(o *tenant.Options) { *o = cfg.Tenant.Options })),
		container.Provide(vectorstore.DefaultRef(), vectorstoredb.New,
			container.WithOptions(func(o *vectorstore.Options) { *o = cfg.VectorStore.Options })),
		container.Provide(webhook.DefaultRef(), webhookhttp.New,
			container.WithOptions(func(o *webhook.Options) { *o = cfg.Webhook.Options })),
		container.Provide(workflow.DefaultRef(), workflowmemory.New,
			container.WithOptions(func(o *workflow.Options) { *o = cfg.Workflow.Options })),
	}
}
