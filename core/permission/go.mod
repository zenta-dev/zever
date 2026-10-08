module github.com/zenta-dev/zever/core/permission

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/permission/noop v0.6.1
	github.com/zenta-dev/zever/shared/registry v0.6.1
)

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/permission/noop => ../../adapters/permission/noop
