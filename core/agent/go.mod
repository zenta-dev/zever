module github.com/zenta-dev/zever/core/agent

go 1.27.0

require (
	github.com/zenta-dev/zever/core/ai v0.6.0
	github.com/zenta-dev/zever/shared/mcpclient v0.6.0
)

require (
	github.com/zenta-dev/zever/shared/endpoint v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
)

replace github.com/zenta-dev/zever/core/ai => ../ai

replace github.com/zenta-dev/zever/shared/mcpclient => ../../shared/mcpclient

replace github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry
