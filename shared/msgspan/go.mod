module github.com/zenta-dev/zever/shared/msgspan

go 1.27.0

require (
	github.com/zenta-dev/zever/core/observability v0.5.3
	github.com/zenta-dev/zever/shared/traceprop v0.5.3
	go.opentelemetry.io/otel/trace v1.46.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
)

replace github.com/zenta-dev/zever/core/observability => ../../core/observability

replace github.com/zenta-dev/zever/shared/traceprop => ../traceprop
