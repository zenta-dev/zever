module github.com/zenta-dev/zever/examples/multidb

go 1.27.3

require (
	github.com/zenta-dev/zever/adapters/db/sqlite v0.8.0
	github.com/zenta-dev/zever/adapters/log/slog v0.8.0
	github.com/zenta-dev/zever/adapters/observability/stdout v0.8.0
	github.com/zenta-dev/zever/adapters/ratelimit/memory v0.8.0
	github.com/zenta-dev/zever/adapters/router/stdhttp v0.8.0
	github.com/zenta-dev/zever/container v0.8.0
	github.com/zenta-dev/zever/core/db v0.8.0
	github.com/zenta-dev/zever/core/log v0.8.0
	github.com/zenta-dev/zever/core/observability v0.8.0
	github.com/zenta-dev/zever/core/ratelimit v0.8.0
	github.com/zenta-dev/zever/core/router v0.8.0
	github.com/zenta-dev/zever/serve v0.8.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/zenta-dev/zever/adapters/log/noop v0.8.0 // indirect
	github.com/zenta-dev/zever/config v0.8.0 // indirect
	github.com/zenta-dev/zever/core/agent v0.8.0 // indirect
	github.com/zenta-dev/zever/core/ai v0.8.0 // indirect
	github.com/zenta-dev/zever/core/analytics v0.8.0 // indirect
	github.com/zenta-dev/zever/core/auth v0.8.0 // indirect
	github.com/zenta-dev/zever/core/authz v0.8.0 // indirect
	github.com/zenta-dev/zever/core/billing v0.8.0 // indirect
	github.com/zenta-dev/zever/core/cache v0.8.0 // indirect
	github.com/zenta-dev/zever/core/cdn v0.8.0 // indirect
	github.com/zenta-dev/zever/core/crypto v0.8.0 // indirect
	github.com/zenta-dev/zever/core/document v0.8.0 // indirect
	github.com/zenta-dev/zever/core/eventbus v0.8.0 // indirect
	github.com/zenta-dev/zever/core/flag v0.8.0 // indirect
	github.com/zenta-dev/zever/core/geo v0.8.0 // indirect
	github.com/zenta-dev/zever/core/i18n v0.8.0 // indirect
	github.com/zenta-dev/zever/core/idempotency v0.8.0 // indirect
	github.com/zenta-dev/zever/core/job v0.8.0 // indirect
	github.com/zenta-dev/zever/core/lock v0.8.0 // indirect
	github.com/zenta-dev/zever/core/mailer v0.8.0 // indirect
	github.com/zenta-dev/zever/core/media v0.8.0 // indirect
	github.com/zenta-dev/zever/core/middleware v0.8.0 // indirect
	github.com/zenta-dev/zever/core/notification v0.8.0 // indirect
	github.com/zenta-dev/zever/core/outbox v0.8.0 // indirect
	github.com/zenta-dev/zever/core/password v0.8.0 // indirect
	github.com/zenta-dev/zever/core/payment v0.8.0 // indirect
	github.com/zenta-dev/zever/core/permission v0.8.0 // indirect
	github.com/zenta-dev/zever/core/queue v0.8.0 // indirect
	github.com/zenta-dev/zever/core/rag v0.8.0 // indirect
	github.com/zenta-dev/zever/core/resilience v0.8.0 // indirect
	github.com/zenta-dev/zever/core/scheduler v0.8.0 // indirect
	github.com/zenta-dev/zever/core/search v0.8.0 // indirect
	github.com/zenta-dev/zever/core/secrets v0.8.0 // indirect
	github.com/zenta-dev/zever/core/session v0.8.0 // indirect
	github.com/zenta-dev/zever/core/storage v0.8.0 // indirect
	github.com/zenta-dev/zever/core/tenant v0.8.0 // indirect
	github.com/zenta-dev/zever/core/vectorstore v0.8.0 // indirect
	github.com/zenta-dev/zever/core/webhook v0.8.0 // indirect
	github.com/zenta-dev/zever/core/workflow v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/codec v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/dbconn v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/grpcclient v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/httpclient v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/lrucache v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/mcpclient v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/msgspan v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/outboxbridge v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/providersopt v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/retry v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.8.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260706201446-f0a921348800 // indirect
	google.golang.org/grpc v1.83.2 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	modernc.org/libc v1.77.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.60.1 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/db/sqlite => ../../adapters/db/sqlite
	github.com/zenta-dev/zever/adapters/log/slog => ../../adapters/log/slog
	github.com/zenta-dev/zever/adapters/observability/stdout => ../../adapters/observability/stdout
	github.com/zenta-dev/zever/adapters/ratelimit/memory => ../../adapters/ratelimit/memory
	github.com/zenta-dev/zever/adapters/router/stdhttp => ../../adapters/router/stdhttp
	github.com/zenta-dev/zever/config => ../../config
	github.com/zenta-dev/zever/container => ../../container
	github.com/zenta-dev/zever/core/db => ../../core/db
	github.com/zenta-dev/zever/core/log => ../../core/log
	github.com/zenta-dev/zever/core/observability => ../../core/observability
	github.com/zenta-dev/zever/core/ratelimit => ../../core/ratelimit
	github.com/zenta-dev/zever/core/router => ../../core/router
	github.com/zenta-dev/zever/serve => ../../serve
	github.com/zenta-dev/zever/shared/ref => ../../shared/ref
)
