module github.com/zenta-dev/zever/adapters/payment/paddle

go 1.27.0

require (
	github.com/ggicci/httpin v0.20.3 // indirect
	github.com/ggicci/owl v0.8.2 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
)

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/shared/providersopt => ../../../shared/providersopt

require (
	github.com/PaddleHQ/paddle-go-sdk/v5 v5.2.0
	github.com/zenta-dev/zever/core/idempotency v0.0.0
	github.com/zenta-dev/zever/core/payment v0.0.0
	github.com/zenta-dev/zever/shared/httpclient v0.0.0
	github.com/zenta-dev/zever/shared/providersopt v0.0.0
)

replace github.com/zenta-dev/zever/core/idempotency => ../../../core/idempotency

replace github.com/zenta-dev/zever/core/payment => ../../../core/payment

replace github.com/zenta-dev/zever/adapters/idempotency/memory => ../../idempotency/memory

replace github.com/zenta-dev/zever/adapters/payment/stub => ../stub

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
