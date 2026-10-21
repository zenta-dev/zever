module github.com/zenta-dev/zever/adapters/lock/memory

go 1.27.3

require (
	github.com/zenta-dev/zever/core/lock v0.8.0
	github.com/zenta-dev/zever/shared/lrucache v0.8.0
)

require (
	github.com/zenta-dev/zever/shared/redisopt v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.8.0 // indirect
)

replace github.com/zenta-dev/zever/core/lock => ../../../core/lock

replace github.com/zenta-dev/zever/shared/lrucache => ../../../shared/lrucache

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
