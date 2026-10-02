module github.com/zenta-dev/zever/core/billing

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/billing/stub v0.5.3
	github.com/zenta-dev/zever/shared/providersopt v0.5.3
	github.com/zenta-dev/zever/shared/registry v0.5.3
)

require github.com/oklog/ulid/v2 v2.1.2 // indirect

replace (
	github.com/zenta-dev/zever/shared/providersopt => ../../shared/providersopt
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/adapters/billing/stub => ../../adapters/billing/stub
