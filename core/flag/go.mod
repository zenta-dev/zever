module github.com/zenta-dev/zever/core/flag

go 1.27.3

require (
	github.com/zenta-dev/zever/adapters/flag/static v0.8.0
	github.com/zenta-dev/zever/core/log v0.8.0
	github.com/zenta-dev/zever/shared/ref v0.8.0
)

require github.com/zenta-dev/zever/adapters/log/noop v0.8.0 // indirect

replace (
	github.com/zenta-dev/zever/adapters/flag/static => ../../adapters/flag/static
	github.com/zenta-dev/zever/core/log => ../log
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref
