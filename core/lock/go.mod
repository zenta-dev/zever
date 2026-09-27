module github.com/zenta-dev/zever/core/lock

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/lock/memory v0.0.0
	github.com/zenta-dev/zever/shared/redisopt v0.0.0
	github.com/zenta-dev/zever/shared/registry v0.0.0
)

replace (
	github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/adapters/lock/memory => ../../adapters/lock/memory
