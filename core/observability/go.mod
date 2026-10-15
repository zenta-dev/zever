module github.com/zenta-dev/zever/core/observability

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/observability/noop v0.7.1
	github.com/zenta-dev/zever/shared/registry v0.7.1
)

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/observability/noop => ../../adapters/observability/noop
