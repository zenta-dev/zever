module github.com/zenta-dev/zever/adapters/ai/openai

go 1.27.0

require (
	github.com/openai/openai-go/v3 v3.66.0
	github.com/zenta-dev/zever/core/ai v0.5.2
	github.com/zenta-dev/zever/shared/endpoint v0.5.2
	github.com/zenta-dev/zever/shared/httpclient v0.5.2
	github.com/zenta-dev/zever/shared/retry v0.5.2
)

require (
	github.com/coder/websocket v1.8.15 // indirect
	github.com/tidwall/gjson v1.19.0 // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.2 // indirect
)

replace github.com/zenta-dev/zever/core/ai => ../../../core/ai

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
