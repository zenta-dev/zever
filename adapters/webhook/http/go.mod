module github.com/zenta-dev/zever/adapters/webhook/http

go 1.27.3

require (
	github.com/zenta-dev/zever/core/webhook v0.8.0
	github.com/zenta-dev/zever/shared/retry v0.8.0
	github.com/zenta-dev/zever/shared/traceprop v0.8.0
	go.opentelemetry.io/otel/trace v1.47.0
)

replace (
	github.com/zenta-dev/zever/core/webhook => ../../../core/webhook
	github.com/zenta-dev/zever/shared/retry => ../../../shared/retry
)

replace github.com/zenta-dev/zever/adapters/log/noop => ../../log/noop

replace github.com/zenta-dev/zever/adapters/queue/memory => ../../queue/memory

replace github.com/zenta-dev/zever/adapters/queue/db => ../../queue/db

replace github.com/zenta-dev/zever/core/log => ../../../core/log

replace github.com/zenta-dev/zever/core/queue => ../../../core/queue

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/shared/msgspan => ../../../shared/msgspan

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/zenta-dev/zever/core/db v0.8.0 // indirect
	github.com/zenta-dev/zever/core/log v0.8.0 // indirect
	github.com/zenta-dev/zever/core/observability v0.8.0 // indirect
	github.com/zenta-dev/zever/core/queue v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/httpclient v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
)

replace github.com/zenta-dev/zever/core/db => ../../../core/db

replace github.com/zenta-dev/zever/core/observability => ../../../core/observability

replace github.com/zenta-dev/zever/adapters/observability/noop => ../../observability/noop

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
