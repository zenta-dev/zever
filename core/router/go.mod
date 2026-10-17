module github.com/zenta-dev/zever/core/router

go 1.27.2

require (
	github.com/zenta-dev/zever/adapters/router/stdhttp v0.7.2
	github.com/zenta-dev/zever/core/log v0.7.2
	github.com/zenta-dev/zever/shared/registry v0.7.2
)

require github.com/zenta-dev/zever/adapters/log/noop v0.7.2 // indirect

replace github.com/zenta-dev/zever/core/log => ../log

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/router/stdhttp => ../../adapters/router/stdhttp

replace github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop
