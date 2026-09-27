module github.com/zenta-dev/zever/core/scheduler

go 1.27.0

require (
	github.com/zenta-dev/zever/core/job v0.0.0
	github.com/zenta-dev/zever/core/log v0.0.0
	github.com/zenta-dev/zever/core/queue v0.0.0
	github.com/zenta-dev/zever/shared/registry v0.0.0
)

require (
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/zenta-dev/zever/adapters/log/noop v0.0.0 // indirect
	github.com/zenta-dev/zever/core/cache v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/codec v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/retry v0.0.0 // indirect
)

replace github.com/zenta-dev/zever/core/job => ../job

replace github.com/zenta-dev/zever/core/log => ../log

replace github.com/zenta-dev/zever/core/queue => ../queue

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/adapters/cache/memory => ../../adapters/cache/memory

replace github.com/zenta-dev/zever/adapters/log/noop => ../../adapters/log/noop

replace github.com/zenta-dev/zever/adapters/queue/memory => ../../adapters/queue/memory

replace github.com/zenta-dev/zever/core/cache => ../cache

replace github.com/zenta-dev/zever/shared/codec => ../../shared/codec

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/shared/retry => ../../shared/retry
