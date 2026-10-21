module github.com/zenta-dev/zever/serve

go 1.27.3

require (
	github.com/zenta-dev/zever/config v0.8.0
	github.com/zenta-dev/zever/container v0.8.0
	github.com/zenta-dev/zever/core/auth v0.8.0
	github.com/zenta-dev/zever/core/authz v0.8.0
	github.com/zenta-dev/zever/core/crypto v0.8.0
	github.com/zenta-dev/zever/core/db v0.8.0
	github.com/zenta-dev/zever/core/log v0.8.0
	github.com/zenta-dev/zever/core/middleware v0.8.0
	github.com/zenta-dev/zever/core/observability v0.8.0
	github.com/zenta-dev/zever/core/permission v0.8.0
	github.com/zenta-dev/zever/core/queue v0.8.0
	github.com/zenta-dev/zever/core/ratelimit v0.8.0
	github.com/zenta-dev/zever/core/router v0.8.0
	github.com/zenta-dev/zever/core/scheduler v0.8.0
	go.uber.org/goleak v1.3.0
	google.golang.org/grpc v1.83.2
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/zenta-dev/zever/adapters/log/noop v0.8.0 // indirect
	github.com/zenta-dev/zever/core/agent v0.8.0 // indirect
	github.com/zenta-dev/zever/core/ai v0.8.0 // indirect
	github.com/zenta-dev/zever/core/analytics v0.8.0 // indirect
	github.com/zenta-dev/zever/core/billing v0.8.0 // indirect
	github.com/zenta-dev/zever/core/cache v0.8.0 // indirect
	github.com/zenta-dev/zever/core/cdn v0.8.0 // indirect
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
	github.com/zenta-dev/zever/core/notification v0.8.0 // indirect
	github.com/zenta-dev/zever/core/outbox v0.8.0 // indirect
	github.com/zenta-dev/zever/core/password v0.8.0 // indirect
	github.com/zenta-dev/zever/core/payment v0.8.0 // indirect
	github.com/zenta-dev/zever/core/rag v0.8.0 // indirect
	github.com/zenta-dev/zever/core/resilience v0.8.0 // indirect
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
	google.golang.org/protobuf v1.36.12 // indirect
)

replace github.com/zenta-dev/zever/config => ../config

replace github.com/zenta-dev/zever/container => ../container

replace github.com/zenta-dev/zever/core/authz => ../core/authz

replace github.com/zenta-dev/zever/core/crypto => ../core/crypto

replace github.com/zenta-dev/zever/core/db => ../core/db

replace github.com/zenta-dev/zever/core/log => ../core/log

replace github.com/zenta-dev/zever/core/middleware => ../core/middleware

replace github.com/zenta-dev/zever/core/observability => ../core/observability

replace github.com/zenta-dev/zever/core/ratelimit => ../core/ratelimit

replace github.com/zenta-dev/zever/core/router => ../core/router

replace github.com/zenta-dev/zever/shared/ref => ../shared/ref

replace github.com/zenta-dev/zever/core/auth => ../core/auth

replace github.com/zenta-dev/zever/core/permission => ../core/permission

replace github.com/zenta-dev/zever/core/queue => ../core/queue

replace github.com/zenta-dev/zever/core/scheduler => ../core/scheduler
