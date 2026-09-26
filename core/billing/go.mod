module github.com/zenta-dev/zever/core/billing

go 1.27.0

require (
	github.com/zenta-dev/zever/shared/providersopt v0.0.0
	github.com/zenta-dev/zever/shared/registry v0.0.0
)

replace (
	github.com/zenta-dev/zever/shared/providersopt => ../../shared/providersopt
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)
