module github.com/zenta-dev/zever/zevertest

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/analytics/log v0.7.1
	github.com/zenta-dev/zever/adapters/auth/jwt v0.7.1
	github.com/zenta-dev/zever/adapters/auth/session v0.7.1
	github.com/zenta-dev/zever/adapters/cache/memory v0.7.1
	github.com/zenta-dev/zever/adapters/crypto/local v0.7.1
	github.com/zenta-dev/zever/adapters/db/sqlite v0.7.1
	github.com/zenta-dev/zever/adapters/eventbus/memory v0.7.1
	github.com/zenta-dev/zever/adapters/flag/static v0.7.1
	github.com/zenta-dev/zever/adapters/idempotency/memory v0.7.1
	github.com/zenta-dev/zever/adapters/lock/memory v0.7.1
	github.com/zenta-dev/zever/adapters/log/noop v0.7.1
	github.com/zenta-dev/zever/adapters/log/slog v0.7.1
	github.com/zenta-dev/zever/adapters/mailer/log v0.7.1
	github.com/zenta-dev/zever/adapters/notification/log v0.7.1
	github.com/zenta-dev/zever/adapters/observability/noop v0.7.1
	github.com/zenta-dev/zever/adapters/password/argon2 v0.7.1
	github.com/zenta-dev/zever/adapters/permission/noop v0.7.1
	github.com/zenta-dev/zever/adapters/queue/memory v0.7.1
	github.com/zenta-dev/zever/adapters/ratelimit/memory v0.7.1
	github.com/zenta-dev/zever/adapters/router/stdhttp v0.7.1
	github.com/zenta-dev/zever/adapters/session/memory v0.7.1
	github.com/zenta-dev/zever/adapters/storage/local v0.7.1
	github.com/zenta-dev/zever/adapters/tenant/single v0.7.1
	github.com/zenta-dev/zever/config v0.7.1
	github.com/zenta-dev/zever/container v0.7.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/zenta-dev/zever/core/agent v0.7.1 // indirect
	github.com/zenta-dev/zever/core/ai v0.7.1 // indirect
	github.com/zenta-dev/zever/core/analytics v0.7.1 // indirect
	github.com/zenta-dev/zever/core/auth v0.7.1 // indirect
	github.com/zenta-dev/zever/core/billing v0.7.1 // indirect
	github.com/zenta-dev/zever/core/cache v0.7.1 // indirect
	github.com/zenta-dev/zever/core/cdn v0.7.1 // indirect
	github.com/zenta-dev/zever/core/crypto v0.7.1 // indirect
	github.com/zenta-dev/zever/core/db v0.7.1 // indirect
	github.com/zenta-dev/zever/core/document v0.7.1 // indirect
	github.com/zenta-dev/zever/core/eventbus v0.7.1 // indirect
	github.com/zenta-dev/zever/core/flag v0.7.1 // indirect
	github.com/zenta-dev/zever/core/geo v0.7.1 // indirect
	github.com/zenta-dev/zever/core/i18n v0.7.1 // indirect
	github.com/zenta-dev/zever/core/idempotency v0.7.1 // indirect
	github.com/zenta-dev/zever/core/job v0.7.1 // indirect
	github.com/zenta-dev/zever/core/lock v0.7.1 // indirect
	github.com/zenta-dev/zever/core/log v0.7.1 // indirect
	github.com/zenta-dev/zever/core/mailer v0.7.1 // indirect
	github.com/zenta-dev/zever/core/media v0.7.1 // indirect
	github.com/zenta-dev/zever/core/notification v0.7.1 // indirect
	github.com/zenta-dev/zever/core/observability v0.7.1 // indirect
	github.com/zenta-dev/zever/core/outbox v0.7.1 // indirect
	github.com/zenta-dev/zever/core/password v0.7.1 // indirect
	github.com/zenta-dev/zever/core/payment v0.7.1 // indirect
	github.com/zenta-dev/zever/core/permission v0.7.1 // indirect
	github.com/zenta-dev/zever/core/queue v0.7.1 // indirect
	github.com/zenta-dev/zever/core/rag v0.7.1 // indirect
	github.com/zenta-dev/zever/core/ratelimit v0.7.1 // indirect
	github.com/zenta-dev/zever/core/resilience v0.7.1 // indirect
	github.com/zenta-dev/zever/core/router v0.7.1 // indirect
	github.com/zenta-dev/zever/core/scheduler v0.7.1 // indirect
	github.com/zenta-dev/zever/core/search v0.7.1 // indirect
	github.com/zenta-dev/zever/core/secrets v0.7.1 // indirect
	github.com/zenta-dev/zever/core/session v0.7.1 // indirect
	github.com/zenta-dev/zever/core/storage v0.7.1 // indirect
	github.com/zenta-dev/zever/core/tenant v0.7.1 // indirect
	github.com/zenta-dev/zever/core/vectorstore v0.7.1 // indirect
	github.com/zenta-dev/zever/core/webhook v0.7.1 // indirect
	github.com/zenta-dev/zever/core/workflow v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/codec v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/dbconn v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/grpcclient v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/httpclient v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/lrucache v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/mcpclient v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/msgspan v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/outboxbridge v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/providersopt v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/retry v0.7.1 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.7.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/net v0.60.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/grpc v1.83.2 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	modernc.org/libc v1.77.1 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
	modernc.org/sqlite v1.60.1 // indirect
)

replace github.com/zenta-dev/zever/adapters/analytics/log => ../adapters/analytics/log

replace github.com/zenta-dev/zever/adapters/auth/jwt => ../adapters/auth/jwt

replace github.com/zenta-dev/zever/adapters/auth/session => ../adapters/auth/session

replace github.com/zenta-dev/zever/adapters/cache/memory => ../adapters/cache/memory

replace github.com/zenta-dev/zever/adapters/crypto/local => ../adapters/crypto/local

replace github.com/zenta-dev/zever/adapters/db/sqlite => ../adapters/db/sqlite

replace github.com/zenta-dev/zever/adapters/eventbus/memory => ../adapters/eventbus/memory

replace github.com/zenta-dev/zever/adapters/flag/static => ../adapters/flag/static

replace github.com/zenta-dev/zever/adapters/idempotency/memory => ../adapters/idempotency/memory

replace github.com/zenta-dev/zever/adapters/lock/memory => ../adapters/lock/memory

replace github.com/zenta-dev/zever/adapters/log/noop => ../adapters/log/noop

replace github.com/zenta-dev/zever/adapters/log/slog => ../adapters/log/slog

replace github.com/zenta-dev/zever/adapters/mailer/log => ../adapters/mailer/log

replace github.com/zenta-dev/zever/adapters/notification/log => ../adapters/notification/log

replace github.com/zenta-dev/zever/adapters/observability/noop => ../adapters/observability/noop

replace github.com/zenta-dev/zever/adapters/password/argon2 => ../adapters/password/argon2

replace github.com/zenta-dev/zever/adapters/permission/noop => ../adapters/permission/noop

replace github.com/zenta-dev/zever/adapters/queue/memory => ../adapters/queue/memory

replace github.com/zenta-dev/zever/adapters/ratelimit/memory => ../adapters/ratelimit/memory

replace github.com/zenta-dev/zever/adapters/router/stdhttp => ../adapters/router/stdhttp

replace github.com/zenta-dev/zever/adapters/session/memory => ../adapters/session/memory

replace github.com/zenta-dev/zever/adapters/storage/local => ../adapters/storage/local

replace github.com/zenta-dev/zever/adapters/tenant/single => ../adapters/tenant/single

replace github.com/zenta-dev/zever/config => ../config

replace github.com/zenta-dev/zever/container => ../container
