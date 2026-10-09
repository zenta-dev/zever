module github.com/zenta-dev/zever/adapters/ratelimit/memory

go 1.27.2

require github.com/zenta-dev/zever/core/ratelimit v0.6.1

require (
	github.com/zenta-dev/zever/shared/redisopt v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
)

replace github.com/zenta-dev/zever/core/ratelimit => ../../../core/ratelimit

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
