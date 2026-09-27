module github.com/zenta-dev/zever/adapters/webhook/http

go 1.27.0

require (
	github.com/zenta-dev/zever/core/webhook v0.0.0
	github.com/zenta-dev/zever/shared/retry v0.0.0
)

require (
	github.com/zenta-dev/zever/core/log v0.0.0 // indirect
	github.com/zenta-dev/zever/core/queue v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/endpoint v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/httpclient v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
)

replace (
	github.com/zenta-dev/zever/core/webhook => ../../../core/webhook
	github.com/zenta-dev/zever/shared/retry => ../../../shared/retry
)

replace github.com/zenta-dev/zever/adapters/log/noop => ../../log/noop

replace github.com/zenta-dev/zever/adapters/queue/memory => ../../queue/memory

replace github.com/zenta-dev/zever/core/log => ../../../core/log

replace github.com/zenta-dev/zever/core/queue => ../../../core/queue

replace github.com/zenta-dev/zever/shared/endpoint => ../../../shared/endpoint

replace github.com/zenta-dev/zever/shared/httpclient => ../../../shared/httpclient

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry
