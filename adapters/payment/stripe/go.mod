module github.com/zenta-dev/zever/adapters/payment/stripe

go 1.27.0

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/zenta-dev/zever/core/db v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/providersopt v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/traceprop v0.5.3 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
)

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/shared/providersclient => ../../../shared/providersclient

replace github.com/zenta-dev/zever/shared/providersopt => ../../../shared/providersopt

require (
	github.com/stripe/stripe-go/v82 v82.5.1
	github.com/zenta-dev/zever/core/idempotency v0.5.3
	github.com/zenta-dev/zever/core/payment v0.5.3
	github.com/zenta-dev/zever/shared/httpclient v0.5.3
	github.com/zenta-dev/zever/shared/providersclient v0.5.3
)

replace github.com/zenta-dev/zever/core/idempotency => ../../../core/idempotency

replace github.com/zenta-dev/zever/core/payment => ../../../core/payment

replace github.com/zenta-dev/zever/adapters/idempotency/memory => ../../idempotency/memory

replace github.com/zenta-dev/zever/adapters/payment/stub => ../stub

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/core/db => ../../../core/db

replace github.com/zenta-dev/zever/shared/traceprop => ../../../shared/traceprop
