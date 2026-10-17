module github.com/zenta-dev/zever/core/session

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/session/memory v0.7.2
	github.com/zenta-dev/zever/core/db v0.7.2
	github.com/zenta-dev/zever/shared/redisopt v0.7.2
	github.com/zenta-dev/zever/shared/registry v0.7.2
)

replace (
	github.com/zenta-dev/zever/core/db => ../db
	github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/adapters/session/memory => ../../adapters/session/memory
