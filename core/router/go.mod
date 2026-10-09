module github.com/zenta-dev/zever/core/router

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/router/stdhttp v0.6.1
	github.com/zenta-dev/zever/core/log v0.6.1
	github.com/zenta-dev/zever/shared/registry v0.6.1
)

require github.com/zenta-dev/zever/adapters/log/noop v0.6.1 // indirect

replace github.com/zenta-dev/zever/core/log => ../log

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/router/stdhttp => ../../adapters/router/stdhttp

replace github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop
