module github.com/zenta-dev/zever/core/storage

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/storage/local v0.6.1
	github.com/zenta-dev/zever/core/log v0.6.1
	github.com/zenta-dev/zever/shared/registry v0.6.1
)

require github.com/zenta-dev/zever/adapters/log/noop v0.6.1 // indirect

replace (
	github.com/zenta-dev/zever/core/log => ../log
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/adapters/storage/local => ../../adapters/storage/local

replace github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop
