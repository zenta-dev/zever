module github.com/zenta-dev/zever/adapters/cache/memory

go 1.27.2

require (
	github.com/zenta-dev/zever/core/cache v0.6.1
	github.com/zenta-dev/zever/shared/lrucache v0.6.1
)

require (
	github.com/zenta-dev/zever/core/db v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/codec v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
)

replace github.com/zenta-dev/zever/core/cache => ../../../core/cache

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/lrucache => ../../../shared/lrucache

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/core/db => ../../../core/db
