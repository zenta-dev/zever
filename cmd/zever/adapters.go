package main

import (
	aiollama "github.com/zenta-dev/zever/adapters/ai/ollama"
	analyticslog "github.com/zenta-dev/zever/adapters/analytics/log"
	authsession "github.com/zenta-dev/zever/adapters/auth/session"
	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	documentlatex "github.com/zenta-dev/zever/adapters/document/latex"
	documentremote "github.com/zenta-dev/zever/adapters/document/remote"
	eventbusmemory "github.com/zenta-dev/zever/adapters/eventbus/memory"
	flagstatic "github.com/zenta-dev/zever/adapters/flag/static"
	geoosm "github.com/zenta-dev/zever/adapters/geo/osm"
	geostatic "github.com/zenta-dev/zever/adapters/geo/static"
	i18nembed "github.com/zenta-dev/zever/adapters/i18n/embed"
	i18nremote "github.com/zenta-dev/zever/adapters/i18n/remote"
	idempotencymemory "github.com/zenta-dev/zever/adapters/idempotency/memory"
	lockmemory "github.com/zenta-dev/zever/adapters/lock/memory"
	lognoop "github.com/zenta-dev/zever/adapters/log/noop"
	logpretty "github.com/zenta-dev/zever/adapters/log/pretty"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	mailersmtp "github.com/zenta-dev/zever/adapters/mailer/smtp"
	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	observabilitynoop "github.com/zenta-dev/zever/adapters/observability/noop"
	observabilitystdout "github.com/zenta-dev/zever/adapters/observability/stdout"
	paymentstub "github.com/zenta-dev/zever/adapters/payment/stub"
	permissionnoop "github.com/zenta-dev/zever/adapters/permission/noop"
	permissionrbac "github.com/zenta-dev/zever/adapters/permission/rbac"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	ratelimitmemory "github.com/zenta-dev/zever/adapters/ratelimit/memory"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	secretsenv "github.com/zenta-dev/zever/adapters/secrets/env"
	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	storagelocal "github.com/zenta-dev/zever/adapters/storage/local"
	tenantheader "github.com/zenta-dev/zever/adapters/tenant/header"
	tenantsingle "github.com/zenta-dev/zever/adapters/tenant/single"
	webhookhttp "github.com/zenta-dev/zever/adapters/webhook/http"
	webhookqueue "github.com/zenta-dev/zever/adapters/webhook/queue"
	workflowmemory "github.com/zenta-dev/zever/adapters/workflow/memory"
	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/analytics"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/crypto"
	"github.com/zenta-dev/zever/core/document"
	"github.com/zenta-dev/zever/core/eventbus"
	"github.com/zenta-dev/zever/core/flag"
	"github.com/zenta-dev/zever/core/geo"
	"github.com/zenta-dev/zever/core/i18n"
	"github.com/zenta-dev/zever/core/idempotency"
	"github.com/zenta-dev/zever/core/lock"
	"github.com/zenta-dev/zever/core/log"
	"github.com/zenta-dev/zever/core/mailer"
	"github.com/zenta-dev/zever/core/notification"
	"github.com/zenta-dev/zever/core/observability"
	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/core/secrets"
	"github.com/zenta-dev/zever/core/session"
	"github.com/zenta-dev/zever/core/storage"
	"github.com/zenta-dev/zever/core/tenant"
	"github.com/zenta-dev/zever/core/webhook"
	"github.com/zenta-dev/zever/core/workflow"

	aianthropic "github.com/zenta-dev/zever/adapters/ai/anthropic"
	aigemini "github.com/zenta-dev/zever/adapters/ai/gemini"
	aiopenai "github.com/zenta-dev/zever/adapters/ai/openai"
	analyticsposthog "github.com/zenta-dev/zever/adapters/analytics/posthog"
	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	authoidc "github.com/zenta-dev/zever/adapters/auth/oidc"
	billingpaddle "github.com/zenta-dev/zever/adapters/billing/paddle"
	billingstripe "github.com/zenta-dev/zever/adapters/billing/stripe"
	billingstub "github.com/zenta-dev/zever/adapters/billing/stub"
	cacheredis "github.com/zenta-dev/zever/adapters/cache/redis"
	dbpostgres "github.com/zenta-dev/zever/adapters/db/postgres"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	documentlocal "github.com/zenta-dev/zever/adapters/document/local"
	eventbusredis "github.com/zenta-dev/zever/adapters/eventbus/redis"
	flagfirebase "github.com/zenta-dev/zever/adapters/flag/firebase"
	geogoogle "github.com/zenta-dev/zever/adapters/geo/google"
	idempotencyredis "github.com/zenta-dev/zever/adapters/idempotency/redis"
	lockredis "github.com/zenta-dev/zever/adapters/lock/redis"
	logzerolog "github.com/zenta-dev/zever/adapters/log/zerolog"
	medialocal "github.com/zenta-dev/zever/adapters/media/local"
	medias3 "github.com/zenta-dev/zever/adapters/media/s3"
	notificationfcm "github.com/zenta-dev/zever/adapters/notification/fcm"
	notificationtwilio "github.com/zenta-dev/zever/adapters/notification/twilio"
	observabilityotlp "github.com/zenta-dev/zever/adapters/observability/otlp"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	paymentpaddle "github.com/zenta-dev/zever/adapters/payment/paddle"
	paymentstripe "github.com/zenta-dev/zever/adapters/payment/stripe"
	permissioncasbin "github.com/zenta-dev/zever/adapters/permission/casbin"
	queueredis "github.com/zenta-dev/zever/adapters/queue/redis"
	ratelimitredis "github.com/zenta-dev/zever/adapters/ratelimit/redis"
	routerfiber "github.com/zenta-dev/zever/adapters/router/fiber"
	schedulerembedded "github.com/zenta-dev/zever/adapters/scheduler/embedded"
	searchmeilisearch "github.com/zenta-dev/zever/adapters/search/meilisearch"
	searchpostgres "github.com/zenta-dev/zever/adapters/search/postgres"
	searchsqlite "github.com/zenta-dev/zever/adapters/search/sqlite"
	sessionredis "github.com/zenta-dev/zever/adapters/session/redis"
	storager2 "github.com/zenta-dev/zever/adapters/storage/r2"
	storages3 "github.com/zenta-dev/zever/adapters/storage/s3"
	vectorstorepgvector "github.com/zenta-dev/zever/adapters/vectorstore/pgvector"
	vectorstoreqdrant "github.com/zenta-dev/zever/adapters/vectorstore/qdrant"
	vectorstoresqlite "github.com/zenta-dev/zever/adapters/vectorstore/sqlite"
	webhooksqlite "github.com/zenta-dev/zever/adapters/webhook/sqlite"
)

// Adapters in this binary are registered explicitly: adapter packages
// expose constructors but never self-register, and the container wires
// nothing itself, so the composition root registers every adapter here,
// once, at startup (the composition root). Registration performs no I/O.
//
// This binary is developer tooling, so it registers every adapter: it must
// handle arbitrary user projects. Generated apps should import and register
// only the adapters they selected (see renderAppContent).
func init() {
	_ = ai.Register(ai.Ollama, aiollama.New)
	_ = analytics.Register(analytics.Log, analyticslog.New)
	_ = auth.Register(auth.Session, authsession.New)
	_ = cache.Register(cache.Memory, cachememory.New)
	_ = crypto.Register(crypto.AdapterLocal, cryptolocal.New)
	_ = document.Register(document.Remote, documentremote.New)
	_ = document.Register(document.Latex, documentlatex.New)
	_ = eventbus.Register(eventbus.Memory, eventbusmemory.New)
	_ = flag.Register(flag.Static, flagstatic.New)
	_ = geo.Register(geo.Static, geostatic.New)
	_ = geo.Register(geo.OSM, geoosm.New)
	_ = i18n.Register(i18n.Embed, i18nembed.New)
	_ = i18n.Register(i18n.Remote, i18nremote.New)
	_ = idempotency.Register(idempotency.Memory, idempotencymemory.New)
	_ = lock.Register(lock.Memory, lockmemory.New)
	_ = log.Register(log.Noop, func(log.Options) (log.Logger, error) { return lognoop.New(), nil })
	_ = log.Register(log.Slog, func(o log.Options) (log.Logger, error) { return logslog.New(o), nil })
	_ = log.Register(log.Pretty, func(o log.Options) (log.Logger, error) { return logpretty.New(o), nil })
	_ = mailer.Register(mailer.Log, mailerlog.New)
	_ = mailer.Register(mailer.SMTP, mailersmtp.New)
	_ = notification.Register(notification.Log, notificationlog.New)
	_ = observability.Register(observability.Noop, func(observability.Options) (observability.Provider, error) {
		return observabilitynoop.New(), nil
	})
	_ = observability.Register(observability.Stdout, observabilitystdout.New)
	_ = payment.Register(payment.Stub, paymentstub.New)
	_ = permission.Register(permission.Noop, permissionnoop.New)
	_ = permission.Register(permission.RBAC, permissionrbac.New)
	_ = queue.Register(queue.Memory, queuememory.New)
	_ = ratelimit.Register(ratelimit.Memory, ratelimitmemory.New)
	_ = router.Register(router.AdapterStdHTTP, routerstdhttp.New)
	_ = secrets.Register(secrets.Env, func(o secrets.Options) (secrets.Secrets, error) {
		return secretsenv.New(secretsenv.Options{Prefix: o.Prefix})
	})
	_ = session.Register(session.Memory, sessionmemory.New)
	_ = storage.Register(storage.AdapterLocal, storagelocal.New)
	_ = tenant.Register(tenant.Single, tenantsingle.New)
	_ = tenant.Register(tenant.Header, tenantheader.New)
	_ = webhook.Register(webhook.AdapterHTTP, webhookhttp.New)
	_ = webhook.Register(webhook.AdapterQueue, webhookqueue.New)
	_ = workflow.Register(workflow.Memory, workflowmemory.New)

	aianthropic.Register()
	aigemini.Register()
	aiopenai.Register()
	analyticsposthog.Register()
	authjwt.Register()
	authoidc.Register()
	billingpaddle.Register()
	billingstripe.Register()
	billingstub.Register()
	cacheredis.Register()
	dbpostgres.Register()
	dbsqlite.Register()
	documentlocal.Register()
	eventbusredis.Register()
	flagfirebase.Register()
	geogoogle.Register()
	idempotencyredis.Register()
	lockredis.Register()
	logzerolog.Register()
	medialocal.Register()
	medias3.Register()
	notificationfcm.Register()
	notificationtwilio.Register()
	observabilityotlp.Register()
	passwordargon2.Register()
	paymentpaddle.Register()
	paymentstripe.Register()
	permissioncasbin.Register()
	queueredis.Register()
	ratelimitredis.Register()
	routerfiber.Register()
	schedulerembedded.Register()
	searchmeilisearch.Register()
	searchpostgres.Register()
	searchsqlite.Register()
	sessionredis.Register()
	storager2.Register()
	storages3.Register()
	vectorstorepgvector.Register()
	vectorstoreqdrant.Register()
	vectorstoresqlite.Register()
	webhooksqlite.Register()
}
