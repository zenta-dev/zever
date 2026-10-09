module github.com/zenta-dev/zever/core/eval

go 1.27.2

require (
	github.com/zenta-dev/zever/core/agent v0.6.1
	github.com/zenta-dev/zever/core/ai v0.6.1
)

require (
	github.com/zenta-dev/zever/shared/endpoint v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/mcpclient v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
)

replace github.com/zenta-dev/zever/core/agent => ../agent

replace github.com/zenta-dev/zever/core/ai => ../ai

replace github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint

replace github.com/zenta-dev/zever/shared/mcpclient => ../../shared/mcpclient

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry
