module github.com/zenta-dev/zever/core/resilience

go 1.27.3

require (
	github.com/zenta-dev/zever/shared/redisopt v0.8.0
	github.com/zenta-dev/zever/shared/ref v0.8.0
	github.com/zenta-dev/zever/shared/retry v0.8.0
)

replace github.com/zenta-dev/zever/shared/redisopt => ../../shared/redisopt

replace github.com/zenta-dev/zever/shared/registry => ../../shared/registry

replace github.com/zenta-dev/zever/shared/retry => ../../shared/retry

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref
