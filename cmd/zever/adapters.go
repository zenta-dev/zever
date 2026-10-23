package main

import (
	analyticslog "github.com/zenta-dev/zever/adapters/analytics/log"
	cachememory "github.com/zenta-dev/zever/adapters/cache/memory"
	cryptolocal "github.com/zenta-dev/zever/adapters/crypto/local"
	eventbusmemory "github.com/zenta-dev/zever/adapters/eventbus/memory"
	flagstatic "github.com/zenta-dev/zever/adapters/flag/static"
	geostatic "github.com/zenta-dev/zever/adapters/geo/static"
	i18nembed "github.com/zenta-dev/zever/adapters/i18n/embed"
	idempotencymemory "github.com/zenta-dev/zever/adapters/idempotency/memory"
	lockmemory "github.com/zenta-dev/zever/adapters/lock/memory"
	logslog "github.com/zenta-dev/zever/adapters/log/slog"
	mailerlog "github.com/zenta-dev/zever/adapters/mailer/log"
	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	observabilitystdout "github.com/zenta-dev/zever/adapters/observability/stdout"
	paymentstub "github.com/zenta-dev/zever/adapters/payment/stub"
	permissionnoop "github.com/zenta-dev/zever/adapters/permission/noop"
	queuememory "github.com/zenta-dev/zever/adapters/queue/memory"
	ratelimitmemory "github.com/zenta-dev/zever/adapters/ratelimit/memory"
	routerstdhttp "github.com/zenta-dev/zever/adapters/router/stdhttp"
	secretsenv "github.com/zenta-dev/zever/adapters/secrets/env"
	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	storagelocal "github.com/zenta-dev/zever/adapters/storage/local"
	tenantsingle "github.com/zenta-dev/zever/adapters/tenant/single"
	webhookhttp "github.com/zenta-dev/zever/adapters/webhook/http"
	workflowmemory "github.com/zenta-dev/zever/adapters/workflow/memory"
	"github.com/zenta-dev/zever/core/ai"
	"github.com/zenta-dev/zever/core/analytics"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/billing"
	"github.com/zenta-dev/zever/core/cache"
	"github.com/zenta-dev/zever/core/cdn"
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
	"github.com/zenta-dev/zever/core/outbox"
	"github.com/zenta-dev/zever/core/password"
	"github.com/zenta-dev/zever/core/payment"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/ratelimit"
	"github.com/zenta-dev/zever/core/resilience"
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

	aianthropic "github.com/zenta-dev/zever/adapters/ai/anthropic"
	authjwt "github.com/zenta-dev/zever/adapters/auth/jwt"
	billingstub "github.com/zenta-dev/zever/adapters/billing/stub"
	cdnnoop "github.com/zenta-dev/zever/adapters/cdn/noop"
	dbsqlite "github.com/zenta-dev/zever/adapters/db/sqlite"
	documentlocal "github.com/zenta-dev/zever/adapters/document/local"
	medialocal "github.com/zenta-dev/zever/adapters/media/local"
	outboxdb "github.com/zenta-dev/zever/adapters/outbox/db"
	passwordargon2 "github.com/zenta-dev/zever/adapters/password/argon2"
	resilienceinproc "github.com/zenta-dev/zever/adapters/resilience/inproc"
	schedulerembedded "github.com/zenta-dev/zever/adapters/scheduler/embedded"
	searchdb "github.com/zenta-dev/zever/adapters/search/db"
	vectorstoredb "github.com/zenta-dev/zever/adapters/vectorstore/db"
	"github.com/zenta-dev/zever/container"
)

// Adapters in this binary are wired explicitly: adapter packages expose
// constructors but never self-register, and the container resolves only
// what is Provided, so the composition root below wires every default
// adapter here, once, at startup. Wiring performs no I/O.
//
// This binary is developer tooling, so it wires every battery's default: it
// must check arbitrary user projects. Generated apps should Provide only
// the adapters they selected (see renderAppContent).
func cliProviders() []container.Provider {
	return []container.Provider{
		container.Provide(ai.DefaultRef(), aianthropic.New),
		container.Provide(analytics.DefaultRef(), analyticslog.New),
		container.Provide(auth.DefaultRef(), authjwt.New),
		container.Provide(billing.DefaultRef(), func(_ billing.Options) (billing.Billing, error) {
			return billingstub.New(), nil
		}),
		container.Provide(cache.DefaultRef(), cachememory.New),
		container.Provide(cdn.DefaultRef(), cdnnoop.New),
		container.Provide(crypto.DefaultRef(), cryptolocal.New),
		container.Provide(db.DefaultRef(), dbsqlite.New),
		container.Provide(document.DefaultRef(), documentlocal.New),
		container.Provide(eventbus.DefaultRef(), eventbusmemory.New),
		container.Provide(flag.DefaultRef(), flagstatic.New),
		container.Provide(geo.DefaultRef(), geostatic.New),
		container.Provide(i18n.DefaultRef(), i18nembed.New),
		container.Provide(idempotency.DefaultRef(), idempotencymemory.New),
		container.Provide(lock.DefaultRef(), lockmemory.New),
		container.Provide(log.DefaultRef(), func(o log.Options) (log.Logger, error) {
			return logslog.New(o), nil
		}),
		container.Provide(mailer.DefaultRef(), mailerlog.New),
		container.Provide(media.DefaultRef(), medialocal.New),
		container.Provide(notification.DefaultRef(), notificationlog.New),
		container.Provide(observability.DefaultRef(), observabilitystdout.New),
		container.Provide(outbox.DefaultRef(), func(o outbox.Options) (outbox.Store, error) {
			poolOpts := dbconn.SplitDSN(o.DSN)
			poolOpts.DedicatedPool = o.DedicatedPool
			return outboxdb.New(outboxdb.Options{
				Options:      poolOpts,
				Table:        o.Table,
				InboxTable:   o.InboxTable,
				Transport:    o.Publisher,
				PollInterval: o.PollInterval,
				BatchSize:    o.BatchSize,
				MaxAttempts:  o.MaxAttempts,
				Retry:        o.Retry,
				Retention:    o.Retention,
				LockSeconds:  o.LockSeconds,
				Provider:     o.Provider,
			})
		}),
		container.Provide(password.DefaultRef(), passwordargon2.New),
		container.Provide(payment.DefaultRef(), paymentstub.New),
		container.Provide(permission.DefaultRef(), permissionnoop.New),
		container.Provide(queue.DefaultRef(), queuememory.New),
		container.Provide(ratelimit.DefaultRef(), ratelimitmemory.New),
		container.Provide(resilience.DefaultRef(), resilienceinproc.New),
		container.Provide(router.DefaultRef(), routerstdhttp.New),
		container.Provide(scheduler.DefaultRef(), schedulerembedded.New),
		container.Provide(search.DefaultRef(), func(o search.Options) (search.Search, error) {
			poolOpts := dbconn.SplitDSN(o.DSN)
			poolOpts.DedicatedPool = o.DedicatedPool
			return searchdb.New(searchdb.Options{Options: poolOpts})
		}),
		container.Provide(secrets.DefaultRef(), func(o secrets.Options) (secrets.Secrets, error) {
			return secretsenv.New(secretsenv.Options{Prefix: o.Prefix})
		}),
		container.Provide(session.DefaultRef(), sessionmemory.New),
		container.Provide(storage.DefaultRef(), storagelocal.New),
		container.Provide(tenant.DefaultRef(), tenantsingle.New),
		container.Provide(vectorstore.DefaultRef(), vectorstoredb.New),
		container.Provide(webhook.DefaultRef(), webhookhttp.New),
		container.Provide(workflow.DefaultRef(), workflowmemory.New),
	}
}

// cliDriverNames lists every adapter this binary ships per service: the
// explain "drivers" column. The wired pick is cliDefaultDriver; anything
// else in the list needs app-level wiring (container.Provide + rebuild).
// Keep in sync with adapters/ directories when adding an adapter.
func cliDriverNames(service string) []string {
	switch service {
	case "ai":
		return []string{"anthropic", "gemini", "ollama", "openai"}
	case "analytics":
		return []string{"log", "posthog"}
	case "auth":
		return []string{"jwt", "oidc", "session"}
	case "billing":
		return []string{"paddle", "stripe", "stub"}
	case "cache":
		return []string{"db", "memory", "redis"}
	case "cdn":
		return []string{"cloudflare", "noop"}
	case "crypto":
		return []string{"kms", "local"}
	case "db":
		return []string{"postgres", "sqlite"}
	case "document":
		return []string{"latex", "local", "remote"}
	case "eventbus":
		return []string{"memory", "redis"}
	case "flag":
		return []string{"firebase", "static"}
	case "geo":
		return []string{"google", "osm", "static"}
	case "i18n":
		return []string{"embed", "remote"}
	case "idempotency":
		return []string{"db", "memory", "redis"}
	case "lock":
		return []string{"memory", "redis"}
	case "log":
		return []string{"noop", "pretty", "slog", "zerolog"}
	case "mailer":
		return []string{"log", "smtp"}
	case "media":
		return []string{"local", "s3"}
	case "notification":
		return []string{"fcm", "log", "twilio"}
	case "observability":
		return []string{"noop", "otlp", "stdout"}
	case "outbox":
		return []string{"cdc", "db", "memory"}
	case "password":
		return []string{"argon2id"}
	case "payment":
		return []string{"paddle", "stripe", "stub"}
	case "permission":
		return []string{"casbin", "noop", "rbac"}
	case "queue":
		return []string{"db", "memory", "redis"}
	case "ratelimit":
		return []string{"memory", "redis"}
	case "resilience":
		return []string{"memory", "redis"}
	case "router":
		return []string{"fiber", "stdhttp"}
	case "scheduler":
		return []string{"embedded", "postgres"}
	case "search":
		return []string{"db", "meilisearch"}
	case "secrets":
		return []string{"env", "vault"}
	case "session":
		return []string{"db", "memory", "redis"}
	case "storage":
		return []string{"local", "r2", "s3"}
	case "tenant":
		return []string{"header", "single"}
	case "vectorstore":
		return []string{"db", "qdrant"}
	case "webhook":
		return []string{"http", "queue"}
	case "workflow":
		return []string{"db", "memory"}
	default:
		return nil
	}
}

// cliDefaultDriver names the adapter cliProviders wires for service: the
// explain "driver" column. Changing it means editing cliProviders above and
// rebuilding.
func cliDefaultDriver(service string) string {
	switch service {
	case "ai":
		return "anthropic"
	case "analytics":
		return "log"
	case "auth":
		return "jwt"
	case "billing":
		return "stub"
	case "cache":
		return "memory"
	case "cdn":
		return "noop"
	case "crypto":
		return "local"
	case "db":
		return "sqlite"
	case "document":
		return "local"
	case "eventbus":
		return "memory"
	case "flag":
		return "static"
	case "geo":
		return "static"
	case "i18n":
		return "embed"
	case "idempotency":
		return "memory"
	case "lock":
		return "memory"
	case "log":
		return "slog"
	case "mailer":
		return "log"
	case "media":
		return "local"
	case "notification":
		return "log"
	case "observability":
		return "stdout"
	case "outbox":
		return "db"
	case "password":
		return "argon2id"
	case "payment":
		return "stub"
	case "permission":
		return "noop"
	case "queue":
		return "memory"
	case "ratelimit":
		return "memory"
	case "resilience":
		return "memory"
	case "router":
		return "stdhttp"
	case "scheduler":
		return "embedded"
	case "search":
		return "db"
	case "secrets":
		return "env"
	case "session":
		return "memory"
	case "storage":
		return "local"
	case "tenant":
		return "single"
	case "vectorstore":
		return "db"
	case "webhook":
		return "http"
	case "workflow":
		return "memory"
	default:
		return ""
	}
}
