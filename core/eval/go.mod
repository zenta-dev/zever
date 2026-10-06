module github.com/zenta-dev/zever/core/eval

go 1.27.0

require (
	github.com/zenta-dev/zever/core/agent v0.5.3
	github.com/zenta-dev/zever/core/ai v0.5.3
)

require (
	github.com/zenta-dev/zever/shared/endpoint v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/mcpclient v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
)

replace github.com/zenta-dev/zever/core/agent => ../agent

replace github.com/zenta-dev/zever/core/ai => ../ai

replace github.com/zenta-dev/zever/shared/mcpclient => ../../shared/mcpclient
