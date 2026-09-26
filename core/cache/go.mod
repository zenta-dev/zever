module github.com/zenta-dev/zever/core/cache

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/cache/memory v0.0.0
	github.com/zenta-dev/zever/shared/codec v0.0.0
	github.com/zenta-dev/zever/shared/redisopt v0.0.0
	github.com/zenta-dev/zever/shared/registry v0.0.0
)

replace (
	github.com/zenta-dev/zever/adapters/cache/memory => ../../adapters/cache/memory
	github.com/zenta-dev/zever/shared/codec => ../../shared/codec
	github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)
