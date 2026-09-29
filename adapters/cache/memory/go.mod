module github.com/zenta-dev/zever/adapters/cache/memory

go 1.27.0

require github.com/zenta-dev/zever/core/cache v0.5.3

require (
	github.com/zenta-dev/zever/shared/codec v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
)

replace github.com/zenta-dev/zever/core/cache => ../../../core/cache

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
