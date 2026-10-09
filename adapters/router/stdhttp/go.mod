module github.com/zenta-dev/zever/adapters/router/stdhttp

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/log/noop v0.6.1
	github.com/zenta-dev/zever/core/log v0.6.1
	github.com/zenta-dev/zever/core/router v0.6.1
)

require github.com/zenta-dev/zever/shared/registry v0.6.1 // indirect

replace (
	github.com/zenta-dev/zever/adapters/log/noop => ../../log/noop
	github.com/zenta-dev/zever/core/log => ../../../core/log
	github.com/zenta-dev/zever/core/router => ../../../core/router
)

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
