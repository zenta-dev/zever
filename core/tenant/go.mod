module github.com/zenta-dev/zever/core/tenant

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/tenant/single v0.0.0
	github.com/zenta-dev/zever/shared/registry v0.0.0
)

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/tenant/single => ../../adapters/tenant/single
