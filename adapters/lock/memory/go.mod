module github.com/zenta-dev/zever/adapters/lock/memory

go 1.27.0

require github.com/zenta-dev/zever/core/lock v0.5.0

require (
	github.com/zenta-dev/zever/shared/redisopt v0.5.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.0 // indirect
)

replace github.com/zenta-dev/zever/core/lock => ../../../core/lock

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
