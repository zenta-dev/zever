module github.com/zenta-dev/zever/adapters/router/stdhttp

go 1.27.3

require (
	github.com/zenta-dev/zever/adapters/log/noop v0.8.0
	github.com/zenta-dev/zever/core/log v0.8.0
	github.com/zenta-dev/zever/core/router v0.8.0
)

require (
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.8.0 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/log/noop => ../../log/noop
	github.com/zenta-dev/zever/core/log => ../../../core/log
	github.com/zenta-dev/zever/core/router => ../../../core/router
)

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
