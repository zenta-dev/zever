module github.com/zenta-dev/zever/core/flag

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/flag/static v0.6.1
	github.com/zenta-dev/zever/core/log v0.6.1
	github.com/zenta-dev/zever/shared/registry v0.6.1
)

require github.com/zenta-dev/zever/adapters/log/noop v0.6.1 // indirect

replace (
	github.com/zenta-dev/zever/adapters/flag/static => ../../adapters/flag/static
	github.com/zenta-dev/zever/core/log => ../log
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop
