module github.com/zenta-dev/zever/core/payment

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/payment/stub v0.5.3
	github.com/zenta-dev/zever/core/idempotency v0.5.3
	github.com/zenta-dev/zever/shared/providersopt v0.5.3
	github.com/zenta-dev/zever/shared/registry v0.5.3
)

require (
	github.com/zenta-dev/zever/core/db v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.5.3 // indirect
)

replace github.com/zenta-dev/zever/core/idempotency => ../idempotency

replace github.com/zenta-dev/zever/shared/providersopt => ../../shared/providersopt

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/payment/stub => ../../adapters/payment/stub

replace github.com/zenta-dev/zever/adapters/idempotency/memory => ../../adapters/idempotency/memory

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/core/db => ../db
