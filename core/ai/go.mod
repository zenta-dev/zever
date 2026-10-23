module github.com/zenta-dev/zever/core/ai

go 1.27.3

require (
	github.com/zenta-dev/zever/shared/endpoint v0.8.0
	github.com/zenta-dev/zever/shared/ref v0.8.0
)

replace (
	github.com/zenta-dev/zever/shared/endpoint => ../../shared/endpoint
	github.com/zenta-dev/zever/shared/registry => ../../shared/registry
)

replace github.com/zenta-dev/zever/shared/ref => ../../shared/ref
