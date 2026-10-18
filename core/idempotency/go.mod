module github.com/zenta-dev/zever/core/idempotency

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/idempotency/memory v0.8.0
	github.com/zenta-dev/zever/core/db v0.8.0
	github.com/zenta-dev/zever/shared/redisopt v0.8.0
	github.com/zenta-dev/zever/shared/ref v0.8.0
	github.com/zenta-dev/zever/shared/registry v0.8.0
)

replace (
	github.com/zenta-dev/zever/core/db => ../db
	github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/adapters/idempotency/memory => ../../adapters/idempotency/memory

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref
