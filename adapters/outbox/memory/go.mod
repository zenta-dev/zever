module github.com/zenta-dev/zever/adapters/outbox/memory

go 1.27.2

require (
	github.com/zenta-dev/zever/core/db v0.6.1
	github.com/zenta-dev/zever/core/outbox v0.6.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/zenta-dev/zever/core/observability v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/retry v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.6.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
)

replace github.com/zenta-dev/zever/core/db => ../../../core/db

replace github.com/zenta-dev/zever/core/observability => ../../../core/observability

replace github.com/zenta-dev/zever/core/outbox => ../../../core/outbox

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop

replace github.com/zenta-dev/zever/adapters/observability/noop => ../../observability/noop
