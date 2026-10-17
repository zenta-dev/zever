module github.com/zenta-dev/zever/core/eval

go 1.27.2

require (
	github.com/zenta-dev/zever/core/agent v0.7.2
	github.com/zenta-dev/zever/core/ai v0.7.2
)

require (
	github.com/zenta-dev/zever/shared/endpoint v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/mcpclient v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.7.2 // indirect
)

replace github.com/zenta-dev/zever/core/agent => ../agent

replace github.com/zenta-dev/zever/core/ai => ../ai

replace github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint

replace github.com/zenta-dev/zever/shared/mcpclient => ../../shared/mcpclient

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry
