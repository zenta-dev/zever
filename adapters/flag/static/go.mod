module github.com/zenta-dev/zever/adapters/flag/static

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/log/noop v0.7.2
	github.com/zenta-dev/zever/core/flag v0.7.2
	github.com/zenta-dev/zever/core/log v0.7.2
)

require (
	github.com/zenta-dev/zever/shared/ref v0.7.2 // indirect
	github.com/zenta-dev/zever/shared/registry v0.7.2 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/log/noop => ../../log/noop
	github.com/zenta-dev/zever/core/flag => ../../../core/flag
	github.com/zenta-dev/zever/core/log => ../../../core/log
)

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
