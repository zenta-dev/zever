module github.com/zenta-dev/zever/adapters/resilience/inproc

go 1.27.0

require (
	github.com/sony/gobreaker/v2 v2.4.0
	github.com/zenta-dev/zever/core/resilience v0.5.3
	github.com/zenta-dev/zever/shared/retry v0.5.3
	golang.org/x/sync v0.23.0
)

require (
	github.com/zenta-dev/zever/shared/redisopt v0.5.3 // indirect
	github.com/zenta-dev/zever/shared/registry v0.5.3 // indirect
)

replace github.com/zenta-dev/zever/core/resilience => ../../../core/resilience

replace github.com/zenta-dev/zever/shared/registry => ../../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../../shared/retry
