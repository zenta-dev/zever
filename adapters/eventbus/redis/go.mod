module github.com/zenta-dev/zever/adapters/eventbus/redis

go 1.27.0

require (
	github.com/alicebob/miniredis/v2 v2.39.0
	github.com/redis/go-redis/v9 v9.22.0
	github.com/zenta-dev/zever/core/eventbus v0.5.3
	github.com/zenta-dev/zever/core/observability v0.5.3
	github.com/zenta-dev/zever/shared/codec v0.5.3
	github.com/zenta-dev/zever/shared/msgspan v0.5.3
	github.com/zenta-dev/zever/shared/redisclient v0.5.3
	github.com/zenta-dev/zever/shared/redisopt v0.5.3
	github.com/zenta-dev/zever/shared/traceprop v0.5.3
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/yuin/gopher-lua v1.1.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.uber.org/atomic v1.11.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/redisclient => ../../../shared/redisclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop

replace github.com/zenta-dev/zever/core/eventbus => ../../../core/eventbus

replace github.com/zenta-dev/zever/adapters/eventbus/memory => ../memory

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/msgspan => ../../../shared/msgspan

replace github.com/zenta-dev/zever/core/observability => ../../../core/observability
