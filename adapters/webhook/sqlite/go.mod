module github.com/zenta-dev/zever/adapters/webhook/sqlite

go 1.27.0

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/mattn/go-isatty v0.0.24 // indirect
	github.com/ncruces/go-strftime v1.0.0 // indirect
	github.com/remyoudompheng/bigfft v0.0.0-20230129092748-24d4a6f8daec // indirect
	github.com/zenta-dev/zever/core/log v0.0.0 // indirect
	github.com/zenta-dev/zever/core/queue v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/httpclient v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/retry v0.0.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	modernc.org/libc v1.75.7 // indirect
	modernc.org/mathutil v1.7.1 // indirect
	modernc.org/memory v1.12.1 // indirect
)

require (
	github.com/zenta-dev/zever/adapters/webhook/http v0.0.0
	github.com/zenta-dev/zever/core/webhook v0.0.0
	modernc.org/sqlite v1.59.0
)

replace github.com/zenta-dev/zever/core/webhook => ../../../core/webhook

replace github.com/zenta-dev/zever/adapters/webhook/http => ../http

replace github.com/zenta-dev/zever/adapters/log/noop => ../../log/noop

replace github.com/zenta-dev/zever/adapters/queue/memory => ../../queue/memory

replace github.com/zenta-dev/zever/core/log => ../../../core/log

replace github.com/zenta-dev/zever/core/queue => ../../../core/queue

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry
