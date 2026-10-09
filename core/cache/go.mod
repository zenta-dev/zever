module github.com/zenta-dev/zever/core/cache

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/cache/memory v0.6.1
	github.com/zenta-dev/zever/core/db v0.6.1
	github.com/zenta-dev/zever/shared/codec v0.6.1
	github.com/zenta-dev/zever/shared/redisopt v0.6.1
	github.com/zenta-dev/zever/shared/registry v0.6.1
)

require github.com/zenta-dev/zever/shared/lrucache v0.6.1 // indirect

replace (
	github.com/zenta-dev/zever/adapters/cache/memory => ../../adapters/cache/memory
	github.com/zenta-dev/zever/core/db => ../db
	github.com/zenta-dev/zever/shared/codec => ../../shared/codec
	github.com/zenta-dev/zever/shared/lrucache => ../../shared/lrucache
	github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)
