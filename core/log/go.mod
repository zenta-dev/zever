module github.com/zenta-dev/zever/core/log

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/log/noop v0.6.1
	github.com/zenta-dev/zever/shared/registry v0.6.1
)

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop
