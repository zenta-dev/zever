module github.com/zenta-dev/zever/adapters/session/memory

go 1.27.0

require github.com/zenta-dev/zever/core/session v0.6.0

require (
	github.com/zenta-dev/zever/core/db v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
)

replace github.com/zenta-dev/zever/core/session => ../../../core/session

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/core/db => ../../../core/db
