module github.com/zenta-dev/zever/adapters/eventbus/memory

go 1.27.0

require github.com/zenta-dev/zever/core/eventbus v0.0.0

require (
	github.com/zenta-dev/zever/shared/redisopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
)

replace github.com/zenta-dev/zever/core/eventbus => ../../../core/eventbus

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
