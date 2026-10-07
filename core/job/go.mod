module github.com/zenta-dev/zever/core/job

go 1.27.0

require (
	github.com/robfig/cron/v3 v3.0.1
	github.com/zenta-dev/zever/adapters/cache/memory v0.5.3
	github.com/zenta-dev/zever/adapters/log/noop v0.5.3
	github.com/zenta-dev/zever/adapters/queue/memory v0.5.3
	github.com/zenta-dev/zever/core/cache v0.5.3
	github.com/zenta-dev/zever/core/log v0.5.3
	github.com/zenta-dev/zever/core/observability v0.5.3
	github.com/zenta-dev/zever/core/queue v0.5.3
	github.com/zenta-dev/zever/shared/msgspan v0.5.3
	github.com/zenta-dev/zever/shared/retry v0.5.3
	github.com/zenta-dev/zever/shared/traceprop v0.5.3
	go.opentelemetry.io/otel v1.46.0
	go.opentelemetry.io/otel/trace v1.46.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/zenta-dev/zever/core/db v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/codec v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/lrucache v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
)

replace (
	github.com/zenta-dev/zever/core/cache => ../cache
	github.com/zenta-dev/zever/core/log => ../log
	github.com/zenta-dev/zever/core/queue => ../queue
	github.com/zenta-dev/zever/shared/retry => ../../shared/retry
)

replace github.com/zenta-dev/zever/adapters/cache/memory => ../../adapters/cache/memory

replace github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop

replace github.com/zenta-dev/zever/adapters/queue/memory => ../../adapters/queue/memory

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec

replace github.com/zenta-dev/zever/shared/lrucache => ../../shared/lrucache

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/shared/traceprop => ../../shared/traceprop

replace github.com/zenta-dev/zever/core/db => ../db

replace github.com/zenta-dev/zever/shared/msgspan => ../../shared/msgspan

replace github.com/zenta-dev/zever/core/observability => ../observability
