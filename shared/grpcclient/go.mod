module github.com/zenta-dev/zever/shared/grpcclient

go 1.27.0

require (
	github.com/zenta-dev/zever/core/observability v0.6.0
	github.com/zenta-dev/zever/core/resilience v0.6.0
	github.com/zenta-dev/zever/shared/traceprop v0.6.0
	go.opentelemetry.io/otel/trace v1.46.0
	google.golang.org/grpc v1.83.2
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/retry v0.5.3 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260526163538-3dc84a4a5aaa // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace github.com/zenta-dev/zever/adapters/observability/noop => ../../adapters/observability/noop

replace github.com/zenta-dev/zever/core/observability => ../../core/observability

replace github.com/zenta-dev/zever/core/resilience => ../../core/resilience

replace github.com/zenta-dev/zever/shared/registry => ../registry

replace github.com/zenta-dev/zever/shared/traceprop => ../traceprop
