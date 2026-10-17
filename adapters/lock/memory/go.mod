module github.com/zenta-dev/zever/adapters/lock/memory

go 1.27.2

require (
	github.com/zenta-dev/zever/core/lock v0.7.2
	github.com/zenta-dev/zever/shared/lrucache v0.7.2
)

require (
	github.com/zenta-dev/zever/shared/redisopt v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/ref v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.7.2 // indirect
)

replace github.com/zenta-dev/zever/core/lock => ../../../core/lock

replace github.com/zenta-dev/zever/shared/lrucache => ../../../shared/lrucache

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
