module github.com/zenta-dev/zever/core/ratelimit

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/ratelimit/memory v0.6.1
	github.com/zenta-dev/zever/shared/redisopt v0.6.1
	github.com/zenta-dev/zever/shared/registry v0.6.1
)

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/ratelimit/memory => ../../adapters/ratelimit/memory
