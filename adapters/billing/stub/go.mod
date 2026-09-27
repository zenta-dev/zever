module github.com/zenta-dev/zever/adapters/billing/stub

go 1.27.0

require (
	github.com/oklog/ulid/v2 v2.1.2
	github.com/zenta-dev/zever/core/billing v0.0.0
)

require (
	github.com/zenta-dev/zever/shared/providersopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
)

replace github.com/zenta-dev/zever/core/billing => ../../../core/billing

replace github.com/zenta-dev/zever/shared/providersopt => ../../../shared/providersopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
