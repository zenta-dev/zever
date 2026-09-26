module github.com/zenta-dev/zever/adapters/storage/local

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/log/noop v0.0.0
	github.com/zenta-dev/zever/core/log v0.0.0
	github.com/zenta-dev/zever/core/storage v0.0.0
)

require github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect

replace (
	github.com/zenta-dev/zever/adapters/log/noop => ../../log/noop
	github.com/zenta-dev/zever/core/log => ../../../core/log
	github.com/zenta-dev/zever/core/storage => ../../../core/storage
)

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
