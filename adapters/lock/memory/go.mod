module github.com/zenta-dev/zever/adapters/lock/memory

go 1.27.0

require (
	github.com/zenta-dev/zever/core/lock v0.6.0
	github.com/zenta-dev/zever/shared/lrucache v0.6.0
)

require (
	github.com/zenta-dev/zever/shared/redisopt v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
)

replace github.com/zenta-dev/zever/core/lock => ../../../core/lock

replace github.com/zenta-dev/zever/shared/lrucache => ../../../shared/lrucache

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
