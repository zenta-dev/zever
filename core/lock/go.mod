module github.com/zenta-dev/zever/core/lock

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/lock/memory v0.6.1
	github.com/zenta-dev/zever/shared/redisopt v0.6.1
	github.com/zenta-dev/zever/shared/registry v0.6.1
)

require github.com/zenta-dev/zever/shared/lrucache v0.6.1 // indirect

replace (
	github.com/zenta-dev/zever/shared/lrucache => ../../shared/lrucache
	github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/adapters/lock/memory => ../../adapters/lock/memory
