module github.com/zenta-dev/zever/adapters/queue/redis

go 1.27.2

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	github.com/zenta-dev/zever/core/db v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.8.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/zenta-dev/zever/shared/redisclient => ../../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop

require (
	github.com/alicebob/miniredis/v2 v2.40.0
	github.com/redis/go-redis/v9 v9.23.0
	github.com/zenta-dev/zever/core/observability v0.8.0
	github.com/zenta-dev/zever/core/queue v0.8.0
	github.com/zenta-dev/zever/shared/msgspan v0.8.0
	github.com/zenta-dev/zever/shared/redisclient v0.8.0
	github.com/zenta-dev/zever/shared/redisopt v0.8.0
	github.com/zenta-dev/zever/shared/retry v0.8.0
	github.com/zenta-dev/zever/shared/traceprop v0.8.0
	go.opentelemetry.io/otel/trace v1.47.0
)

replace github.com/zenta-dev/zever/core/queue => ../../../core/queue

replace github.com/zenta-dev/zever/adapters/queue/memory => ../memory

replace github.com/zenta-dev/zever/adapters/queue/db => ../db

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/core/db => ../../../core/db

replace github.com/zenta-dev/zever/shared/msgspan => ../../../shared/msgspan

replace github.com/zenta-dev/zever/core/observability => ../../../core/observability

replace github.com/zenta-dev/zever/adapters/observability/noop => ../../observability/noop

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
