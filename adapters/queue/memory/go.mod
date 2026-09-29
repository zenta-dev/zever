module github.com/zenta-dev/zever/adapters/queue/memory

go 1.27.0

require github.com/zenta-dev/zever/core/queue v0.5.3

require (
	github.com/zenta-dev/zever/shared/redisopt v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
)

replace github.com/zenta-dev/zever/core/queue => ../../../core/queue

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
