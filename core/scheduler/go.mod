module github.com/zenta-dev/zever/core/scheduler

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/scheduler/embedded v0.6.1
	github.com/zenta-dev/zever/core/db v0.6.1
	github.com/zenta-dev/zever/core/job v0.6.1
	github.com/zenta-dev/zever/core/log v0.6.1
	github.com/zenta-dev/zever/core/queue v0.6.1
	github.com/zenta-dev/zever/shared/registry v0.6.1
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/zenta-dev/zever/adapters/log/noop v0.6.1 // indirect
	github.com/zenta-dev/zever/core/cache v0.6.1 // indirect
	github.com/zenta-dev/zever/core/observability v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/codec v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/msgspan v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/retry v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.6.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
)

replace github.com/zenta-dev/zever/core/db => ../db

replace github.com/zenta-dev/zever/core/job => ../job

replace github.com/zenta-dev/zever/core/log => ../log

replace github.com/zenta-dev/zever/core/queue => ../queue

replace github.com/zenta-dev/zever/shared/msgspan => ../../shared/msgspan

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/cache/memory => ../../adapters/cache/memory

replace github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop

replace github.com/zenta-dev/zever/adapters/queue/memory => ../../adapters/queue/memory

replace github.com/zenta-dev/zever/core/cache => ../cache

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec

replace github.com/zenta-dev/zever/shared/lrucache => ../../shared/lrucache

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/shared/retry => ../../shared/retry

replace github.com/zenta-dev/zever/shared/traceprop => ../../shared/traceprop

replace github.com/zenta-dev/zever/adapters/scheduler/embedded => ../../adapters/scheduler/embedded

replace github.com/zenta-dev/zever/core/observability => ../observability

replace github.com/zenta-dev/zever/adapters/observability/noop => ../../adapters/observability/noop
