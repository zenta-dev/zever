module github.com/zenta-dev/zever/adapters/cache/memory

go 1.27.0

require github.com/zenta-dev/zever/core/cache v0.0.0

require (
	github.com/zenta-dev/zever/shared/codec v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
)

replace github.com/zenta-dev/zever/core/cache => ../../../core/cache

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
