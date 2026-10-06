module github.com/zenta-dev/zever/shared/providersclient

go 1.27.0

require (
	github.com/stripe/stripe-go/v82 v82.5.1
	github.com/zenta-dev/zever/shared/httpclient v0.5.3
	github.com/zenta-dev/zever/shared/traceprop v0.5.3
	go.opentelemetry.io/otel/trace v1.46.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
)

replace github.com/zenta-dev/zever/shared/httpclient => ../httpclient

replace github.com/zenta-dev/zever/shared/traceprop => ../../shared/traceprop
