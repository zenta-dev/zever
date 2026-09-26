module github.com/zenta-dev/zever/adapters/router/fiber

go 1.27.0

require (
	github.com/andybalholm/brotli v1.1.1 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/klauspost/compress v1.20.0 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/mattn/go-runewidth v0.0.27 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/tcplisten v1.0.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

require (
	github.com/gofiber/adaptor/v2 v2.2.1
	github.com/gofiber/fiber/v2 v2.52.15
	github.com/valyala/fasthttp v1.51.0
	github.com/zenta-dev/zever/adapters/log/noop v0.0.0
	github.com/zenta-dev/zever/core/log v0.0.0
	github.com/zenta-dev/zever/core/router v0.0.0
)

replace github.com/zenta-dev/zever/adapters/log/noop => ../../log/noop

replace github.com/zenta-dev/zever/core/log => ../../../core/log

replace github.com/zenta-dev/zever/core/router => ../../../core/router

replace github.com/zenta-dev/zever/adapters/router/stdhttp => ../stdhttp

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
