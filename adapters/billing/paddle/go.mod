module github.com/zenta-dev/zever/adapters/billing/paddle

go 1.27.0

require (
	github.com/PaddleHQ/paddle-go-sdk/v5 v5.2.0
	github.com/zenta-dev/zever/core/billing v0.6.0
	github.com/zenta-dev/zever/shared/httpclient v0.6.0
	github.com/zenta-dev/zever/shared/providersopt v0.6.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/ggicci/httpin v0.20.3 // indirect
	github.com/ggicci/owl v0.8.2 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.0 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.6.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
)

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/shared/providersopt => ../../../shared/providersopt

replace github.com/zenta-dev/zever/core/billing => ../../../core/billing

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop

replace github.com/zenta-dev/zever/adapters/billing/stub => ../stub
