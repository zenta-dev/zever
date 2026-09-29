module github.com/zenta-dev/zever/core/webhook

go 1.27.0

require (
	github.com/zenta-dev/zever/adapters/webhook/http v0.5.2
	github.com/zenta-dev/zever/core/log v0.5.2
	github.com/zenta-dev/zever/core/queue v0.5.2
	github.com/zenta-dev/zever/shared/endpoint v0.5.2
	github.com/zenta-dev/zever/shared/httpclient v0.5.2
	github.com/zenta-dev/zever/shared/registry v0.5.2
)

require (
	github.com/zenta-dev/zever/shared/redisopt v0.5.2 // indirect
	github.com/zenta-dev/zever/shared/retry v0.5.2 // indirect
)

replace (
	github.com/zenta-dev/zever/core/log => ../log
	github.com/zenta-dev/zever/core/queue => ../queue
	github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint
	github.com/zenta-dev/zever/shared/httpclient => ../../shared/httpclient
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/adapters/webhook/http => ../../adapters/webhook/http

replace github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop

replace github.com/zenta-dev/zever/adapters/queue/memory => ../../adapters/queue/memory

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/shared/retry => ../../shared/retry
