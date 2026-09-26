module github.com/zenta-dev/zever/adapters/scheduler/embedded

go 1.27.0

require (
	github.com/robfig/cron/v3 v3.0.1
	github.com/zenta-dev/zever/adapters/log/noop v0.0.0
	github.com/zenta-dev/zever/adapters/queue/memory v0.0.0
	github.com/zenta-dev/zever/core/job v0.0.0
	github.com/zenta-dev/zever/core/log v0.0.0
	github.com/zenta-dev/zever/core/queue v0.0.0
	github.com/zenta-dev/zever/core/scheduler v0.0.0
	github.com/zenta-dev/zever/shared/codec v0.0.0
)

require (
	github.com/zenta-dev/zever/core/cache v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/redisopt v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/registry v0.0.0 // indirect
	github.com/zenta-dev/zever/shared/retry v0.0.0 // indirect
)

replace github.com/zenta-dev/zever/adapters/log/noop => ../../log/noop

replace github.com/zenta-dev/zever/adapters/queue/memory => ../../queue/memory

replace github.com/zenta-dev/zever/core/job => ../../../core/job

replace github.com/zenta-dev/zever/core/log => ../../../core/log

replace github.com/zenta-dev/zever/core/queue => ../../../core/queue

replace github.com/zenta-dev/zever/core/scheduler => ../../../core/scheduler

replace github.com/zenta-dev/zever/shared/codec => ../../../shared/codec

replace github.com/zenta-dev/zever/adapters/cache/memory => ../../cache/memory

replace github.com/zenta-dev/zever/core/cache => ../../../core/cache

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry
