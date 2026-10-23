module github.com/zenta-dev/zever/adapters/resilience/inproc

go 1.27.3

require (
	github.com/sony/gobreaker/v2 v2.4.0
	github.com/zenta-dev/zever/core/resilience v0.8.0
	github.com/zenta-dev/zever/shared/retry v0.8.0
	golang.org/x/sync v0.24.0
)

require (
	github.com/zenta-dev/zever/shared/redisopt v0.8.0 // indirect
	github.com/zenta-dev/zever/shared/ref v0.8.0 // indirect
)

replace github.com/zenta-dev/zever/core/resilience => ../../../core/resilience

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry

replace github.com/zenta-dev/zever/shared/redisopt => ../../../shared/redisopt

replace github.com/zenta-dev/zever/shared/ref => ../../../shared/ref
