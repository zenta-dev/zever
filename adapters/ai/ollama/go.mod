module github.com/zenta-dev/zever/adapters/ai/ollama

go 1.27.0

require (
	github.com/zenta-dev/zever/core/ai v0.5.3
	github.com/zenta-dev/zever/shared/codec v0.5.3
	github.com/zenta-dev/zever/shared/endpoint v0.5.3
	github.com/zenta-dev/zever/shared/httpclient v0.5.3
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.5.3 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
)

replace (
	github.com/zenta-dev/zever/core/ai => ../../../core/ai
	github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
	github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint
	github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient
)

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop
