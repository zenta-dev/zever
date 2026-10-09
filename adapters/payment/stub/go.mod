module github.com/zenta-dev/zever/adapters/payment/stub

go 1.27.2

require github.com/zenta-dev/zever/core/payment v0.6.1

require (
	github.com/zenta-dev/zever/core/db v0.6.1 // indirect
	github.com/zenta-dev/zever/core/idempotency v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/providersopt v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.6.1 // indirect
	github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect
)

replace github.com/zenta-dev/zever/core/payment => ../../../core/payment

replace github.com/zenta-dev/zever/adapters/idempotency/memory => ../../idempotency/memory

replace github.com/zenta-dev/zever/core/idempotency => ../../../core/idempotency

replace github.com/zenta-dev/zever/shared/providersopt => ../../../shared/providersopt

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/core/db => ../../../core/db
