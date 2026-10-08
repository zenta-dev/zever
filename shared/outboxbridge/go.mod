module github.com/zenta-dev/zever/shared/outboxbridge

go 1.27.0

require (
	github.com/zenta-dev/zever/core/eventbus v0.6.0
	github.com/zenta-dev/zever/core/outbox v0.6.0
	github.com/zenta-dev/zever/core/queue v0.6.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/zenta-dev/zever/core/db v0.6.0 // indirect
	github.com/zenta-dev/zever/core/observability v0.6.0 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.6.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.0 // indirect
	github.com/zenta-dev/zever/shared/retry v0.6.0 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.6.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
)

replace (
	github.com/zenta-dev/zever/core/db => ../../core/db
	github.com/zenta-dev/zever/core/eventbus => ../../core/eventbus
	github.com/zenta-dev/zever/core/observability => ../../core/observability
	github.com/zenta-dev/zever/core/outbox => ../../core/outbox
	github.com/zenta-dev/zever/core/queue => ../../core/queue
	github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
	github.com/zenta-dev/zever/shared/retry => ../../shared/retry
	github.com/zenta-dev/zever/shared/traceprop => ../../shared/traceprop
)

replace github.com/zenta-dev/zever/adapters/eventbus/memory => ../../adapters/eventbus/memory

replace github.com/zenta-dev/zever/adapters/observability/noop => ../../adapters/observability/noop

replace github.com/zenta-dev/zever/adapters/queue/memory => ../../adapters/queue/memory

replace github.com/zenta-dev/zever/shared/msgspan => ../msgspan
