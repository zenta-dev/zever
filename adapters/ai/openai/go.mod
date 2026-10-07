module github.com/zenta-dev/zever/adapters/ai/openai

go 1.27.0

require (
	github.com/openai/openai-go/v3 v3.70.0
	github.com/zenta-dev/zever/core/ai v0.6.0
	github.com/zenta-dev/zever/shared/endpoint v0.6.0
	github.com/zenta-dev/zever/shared/httpclient v0.6.0
	github.com/zenta-dev/zever/shared/retry v0.6.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/coder/websocket v1.8.15 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/tidwall/gjson v1.19.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.0 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.6.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.46.0 // indirect
	go.opentelemetry.io/otel/metric v1.46.0 // indirect
	go.opentelemetry.io/otel/trace v1.46.0 // indirect
)

replace github.com/zenta-dev/zever/core/ai => ../../../core/ai

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop
