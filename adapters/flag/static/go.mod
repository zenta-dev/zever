module github.com/zenta-dev/zever/adapters/flag/static

go 1.27.3

require (
	github.com/zenta-dev/zever/adapters/log/noop v0.8.0
	github.com/zenta-dev/zever/core/flag v0.8.0
	github.com/zenta-dev/zever/core/log v0.8.0
	github.com/zenta-dev/zever/shared/codec v0.8.0
)

require (
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.8.0 // indirect
)

replace (
	github.com/zenta-dev/zever/adapters/log/noop => ../../log/noop
	github.com/zenta-dev/zever/core/flag => ../../../core/flag
	github.com/zenta-dev/zever/core/log => ../../../core/log
)

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec
