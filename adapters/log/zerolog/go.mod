module github.com/zenta-dev/zever/adapters/log/zerolog

go 1.27.0

require (
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

require (
	github.com/rs/zerolog v1.35.1
	github.com/zenta-dev/zever/core/log v0.0.0
)

replace github.com/zenta-dev/zever/core/log => ../../../core/log

replace github.com/zenta-dev/zever/adapters/log/noop => ../noop

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
