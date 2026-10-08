module github.com/zenta-dev/zever/adapters/lock/memory

go 1.27.0

require (
	github.com/zenta-dev/zever/core/lock v0.6.1
	github.com/zenta-dev/zever/shared/lrucache v0.6.1
)

require (
	github.com/zenta-dev/zever/shared/redisopt v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
)

replace github.com/zenta-dev/zever/core/lock => ../../../core/lock

replace github.com/zenta-dev/zever/shared/lrucache => ../../../shared/lrucache

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
