module github.com/zenta-dev/zever/core/lock

go 1.27.3

require (
	github.com/zenta-dev/zever/adapters/lock/memory v0.8.0
	github.com/zenta-dev/zever/shared/redisopt v0.8.0
	github.com/zenta-dev/zever/shared/ref v0.8.0
	github.com/zenta-dev/zever/shared/registry v0.8.0
)

require github.com/zenta-dev/zever/shared/lrucache v0.8.0 // indirect

replace (
	github.com/zenta-dev/zever/shared/lrucache => ../../shared/lrucache
	github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/adapters/lock/memory => ../../adapters/lock/memory

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref
