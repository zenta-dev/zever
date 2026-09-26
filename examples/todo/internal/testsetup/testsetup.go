// Package testsetup registers every default adapter the todo tests
// resolve. The container wires nothing by itself: each Register call below
// matches one config.Default() adapter pick, so resolving any default
// battery in tests never fails with "unknown adapter (forgotten import?)".
// Registration is idempotent; calling it from multiple setup helpers is safe.
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
	searchsqlite "github.com/zenta-dev/zever/adapters/search/sqlite"
	secretsenv "github.com/zenta-dev/zever/adapters/secrets/env"
	sessionmemory "github.com/zenta-dev/zever/adapters/session/memory"
	storagelocal "github.com/zenta-dev/zever/adapters/storage/local"
	tenantsingle "github.com/zenta-dev/zever/adapters/tenant/single"
	vectorsqlite "github.com/zenta-dev/zever/adapters/vectorstore/sqlite"
	webhookhttp "github.com/zenta-dev/zever/adapters/webhook/http"
	workflowmemory "github.com/zenta-dev/zever/adapters/workflow/memory"
)

// RegisterDefaults registers every config.Default() adapter pick.
func RegisterDefaults() {
	aianthropic.Register()
	analyticslog.Register()
	authjwt.Register()
	billingstub.Register()
	cachememory.Register()
	cryptolocal.Register()
	dbsqlite.Register()
	documentlocal.Register()
	eventbusmemory.Register()
	flagstatic.Register()
	geostatic.Register()
	i18nembed.Register()
	idempotencymemory.Register()
	lockmemory.Register()
	logslog.Register()
	mailerlog.Register()
	medialocal.Register()
	notificationlog.Register()
	observabilitystdout.Register()
	passwordargon2.Register()
	paymentstub.Register()
	permissionrbac.Register()
	queuememory.Register()
	ratelimitmemory.Register()
	routerstdhttp.Register()
	schedulerembedded.Register()
	searchsqlite.Register()
	secretsenv.Register()
	sessionmemory.Register()
	storagelocal.Register()
	tenantsingle.Register()
	vectorsqlite.Register()
	webhookhttp.Register()
	workflowmemory.Register()
}
